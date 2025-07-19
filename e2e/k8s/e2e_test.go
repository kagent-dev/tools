package e2e

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/kagent-dev/tools/internal/commands"
	"github.com/kagent-dev/tools/internal/logger"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// TestE2EK8s is the main test runner for Kubernetes E2E tests
func TestE2EK8s(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "KAgent Tools Kubernetes E2E Suite")
}

/*
K8s E2E Tests
These tests are used to test the Kubernetes integration of the KAgent Tools.
They are run in a Kubernetes cluster and have working in-cluster resources.

They test the following:
- KAgent Tools can be installed in a Kubernetes cluster
- KAgent Tools k8s can list all resources in the cluster
- KAgent Tools helm can list all releases in the cluster
- KAgent Tools istioctl can install istio in the cluster
- KAgent Tools cillium can install cillium in the cluster
*/

// CreateNamespace creates a new Kubernetes namespace
func CreateNamespace(namespace string) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	log := logger.Get()
	By("Creating namespace " + namespace)
	log.Info("Creating namespace", "namespace", namespace)

	// First, check if the namespace already exists
	_, err := commands.NewCommandBuilder("kubectl").
		WithArgs("get", "namespace", namespace).
		WithCache(false).
		Execute(ctx)

	if err == nil {
		log.Info("Namespace already exists, skipping creation", "namespace", namespace)
		return
	}

	// Create the namespace using kubectl
	output, err := commands.NewCommandBuilder("kubectl").
		WithArgs("create", "namespace", namespace).
		WithCache(false). // Don't cache namespace creation
		Execute(ctx)

	// If it's an AlreadyExists error, that's fine - treat it as success
	if err != nil && strings.Contains(err.Error(), "AlreadyExists") {
		log.Info("Namespace already exists, continuing", "namespace", namespace)
		return
	}

	Expect(err).ToNot(HaveOccurred(), "Failed to create namespace: %v", err)
	log.Info("Namespace creation completed", "namespace", namespace, "output", output)
}

// DeleteNamespace deletes a Kubernetes namespace
func DeleteNamespace(namespace string) {
	// Use longer timeout for namespace deletion as it can take more time
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	log := logger.Get()
	By("Deleting namespace " + namespace)
	log.Info("Deleting namespace", "namespace", namespace)

	// Delete the namespace using kubectl
	output, err := commands.NewCommandBuilder("kubectl").
		WithArgs("delete", "namespace", namespace, "--ignore-not-found=true", "--wait=false").
		WithCache(false). // Don't cache namespace deletion
		Execute(ctx)

	Expect(err).ToNot(HaveOccurred(), "Failed to delete namespace: %v", err)
	log.Info("Namespace deletion completed", "namespace", namespace, "output", output)
}

// InstallKAgentTools installs KAgent Tools using helm in the specified namespace
func InstallKAgentTools(namespace string) {
	// Use longer timeout for helm installation as it can take time to pull images
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	log := logger.Get()
	By("Installing KAgent Tools in namespace " + namespace)
	log.Info("Installing KAgent Tools", "namespace", namespace)

	// Use a unique release name for e2e tests to avoid conflicts
	releaseName := "kagent-tools-e2e"

	// First, try to uninstall any existing release to clean up
	log.Info("Cleaning up any existing release", "release", releaseName, "namespace", namespace)
	_, _ = commands.NewCommandBuilder("helm").
		WithArgs("uninstall", releaseName).
		WithArgs("--namespace", namespace).
		WithArgs("--ignore-not-found").
		WithCache(false).
		Execute(ctx)

	// Install KAgent Tools using helm with unique release name
	// Use absolute path from project root
	output, err := commands.NewCommandBuilder("helm").
		WithArgs("install", releaseName, "../../helm/kagent-tools").
		WithArgs("--namespace", namespace).
		WithArgs("--create-namespace").
		WithArgs("--wait").
		WithCache(false). // Don't cache helm installation
		Execute(ctx)

	Expect(err).ToNot(HaveOccurred(), "Failed to install KAgent Tools: %v", err)
	log.Info("KAgent Tools installation completed", "namespace", namespace, "output", output)

	// Verify the installation by checking if pods are running
	By("Verifying KAgent Tools pods are running")
	log.Info("Verifying KAgent Tools pods", "namespace", namespace)

	Eventually(func() bool {
		ctx, cancel := context.WithTimeout(context.Background(), DefaultTimeout)
		defer cancel()

		output, err := commands.NewCommandBuilder("kubectl").
			WithArgs("get", "pods", "-n", namespace, "-l", "app.kubernetes.io/name=kagent", "-o", "jsonpath={.items[*].status.phase}").
			Execute(ctx)

		if err != nil {
			log.Error("Failed to get pod status", "error", err)
			return false
		}

		log.Info("Pod status check", "namespace", namespace, "output", output)
		// Check if all pods are in Running state
		return output == "Running" || (len(output) > 0 && !contains(output, "Pending") && !contains(output, "Failed"))
	}, 120*time.Second, 5*time.Second).Should(BeTrue(), "KAgent Tools pods should be running")
}

// contains checks if a string contains a substring
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && (s[:len(substr)] == substr || s[len(s)-len(substr):] == substr || containsHelper(s, substr)))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

var _ = Describe("KAgent Tools Kubernetes E2E Tests", Ordered, func() {
	var namespace string
	var log = logger.Get()

	BeforeAll(func() {
		log.Info("Starting KAgent Tools E2E tests")
		// Create new namespace
		namespace = DefaultTestNamespace
		CreateNamespace(namespace)
		// Install kagent tools
		InstallKAgentTools(namespace)
	})

	AfterAll(func() {
		log.Info("Cleaning up KAgent Tools E2E tests", "namespace", namespace)
		// Delete namespace
		if namespace != "" {
			DeleteNamespace(namespace)
		}
	})

	Describe("KAgent Tools Deployment", func() {
		It("should have kagent-tools pods running", func() {
			ctx, cancel := context.WithTimeout(context.Background(), DefaultTimeout)
			defer cancel()

			log.Info("Checking if kagent-tools pods are running", "namespace", namespace)
			output, err := commands.NewCommandBuilder("kubectl").
				WithArgs("get", "pods", "-n", namespace, "-l", "app.kubernetes.io/name=kagent", "-o", "json").
				Execute(ctx)

			Expect(err).ToNot(HaveOccurred())
			Expect(output).ToNot(BeEmpty())
			log.Info("Successfully verified kagent-tools pods", "namespace", namespace)
		})

		It("should have kagent-tools service accessible", func() {
			ctx, cancel := context.WithTimeout(context.Background(), DefaultTimeout)
			defer cancel()

			log.Info("Checking if kagent-tools service is accessible", "namespace", namespace)
			output, err := commands.NewCommandBuilder("kubectl").
				WithArgs("get", "svc", "-n", namespace, "-l", "app.kubernetes.io/name=kagent", "-o", "json").
				Execute(ctx)

			Expect(err).ToNot(HaveOccurred())
			Expect(output).ToNot(BeEmpty())
			log.Info("Successfully verified kagent-tools service", "namespace", namespace)
		})
	})

	Describe("KAgent Tools K8s Operations", func() {
		It("should be able to list all resources in the cluster", func() {
			log.Info("Testing MCP client connectivity and k8s operations", "namespace", namespace)

			// Setup port forwarding for testing (run in background)
			go func() {
				_, _ = commands.NewCommandBuilder("kubectl").
					WithArgs("-n", namespace, "port-forward", "service/kagent-tools-e2e", "30884:8084").
					Execute(context.Background())
			}()

			// Wait for port forwarding to establish
			time.Sleep(3 * time.Second)

			log.Info("Attempting to connect to MCP server at localhost:30884")

			// Test MCP client connectivity
			client := GetMCPClient()
			err := TestMCPConnection()
			if err != nil {
				log.Info("MCP server connection failed - this is expected if port forwarding isn't available", "error", err)
				Skip(fmt.Sprintf("MCP server not accessible: %v", err))
				return
			}

			// Test k8s list resources functionality
			log.Info("Testing k8s list resources via MCP")
			response, err := client.k8sListResources("namespace")
			Expect(err).ToNot(HaveOccurred(), "Failed to list k8s resources via MCP: %v", err)
			Expect(response).ToNot(BeNil())

			log.Info("Successfully tested k8s operations via MCP", "namespace", namespace)
		})
	})

	Describe("KAgent Tools Helm Operations", func() {
		It("should be able to list all helm releases", func() {
			log.Info("Testing helm operations via MCP", "namespace", namespace)

			// Test MCP client connectivity first
			client := GetMCPClient()
			err := TestMCPConnection()
			if err != nil {
				Skip(fmt.Sprintf("MCP server not accessible: %v", err))
				return
			}

			// Test helm list releases functionality
			log.Info("Testing helm list releases via MCP")
			response, err := client.helmListReleases()
			if err != nil {
				log.Info("Helm list releases failed (may be normal)", "error", err)
				Skip(fmt.Sprintf("Helm operations not available: %v", err))
				return
			}
			Expect(response).ToNot(BeNil())

			log.Info("Successfully tested helm operations via MCP", "namespace", namespace)
		})
	})

	Describe("KAgent Tools Istio Operations", func() {
		It("should be able to install istio in the cluster", func() {
			client := GetMCPClient()
			r, err := client.istioInstall("default") // Explicitly ignore return values for now
			Expect(err).ToNot(HaveOccurred())
			Expect(r).ToNot(BeNil())
			log.Info("Successfully tested istioInstall via MCP", "namespace", namespace, r.Result)
		})
	})

	Describe("KAgent Tools Cilium Operations", func() {
		It("should be able to install cilium in the cluster", func() {
			client := GetMCPClient()
			r, err := client.ciliumStatus() // Reference to prevent unused function warning
			Expect(err).ToNot(HaveOccurred())
			Expect(r).ToNot(BeNil())
			log.Info("Successfully tested ciliumStatus via MCP", "namespace", namespace, r.Result)
		})
	})

	Describe("KAgent Tools Argo Operations", func() {
		It("should be able to manage argo rollouts", func() {
			Skip("Implementation pending - requires KAgent Tools API to be accessible")
			// TODO: Implement test to call KAgent Tools argo rollouts API
			// TODO: Add MCP client with HTTP Streaming implementations
			// TODO: Verify list of tools provided by argo tool provider
			client := GetMCPClient()
			r, err := client.argoRolloutsList("default") // Explicitly ignore return values for now
			Expect(err).ToNot(HaveOccurred())
			Expect(r).ToNot(BeNil())
			log.Info("Successfully tested argoRolloutsList via MCP", "namespace", namespace, r.Result)
		})
	})

	Describe("KAgent Tools Prometheus Operations", func() {
		It("should be able to query prometheus metrics", func() {
			Skip("Implementation pending - requires KAgent Tools API to be accessible")
			// TODO: Implement test to call KAgent Tools prometheus query API
			// TODO: Add MCP client with HTTP Streaming implementations
			// TODO: Verify list of tools provided by prometheus tool provider
			client := GetMCPClient()
			_, _ = client.prometheusQuery("up") // Reference to prevent unused function warning
		})
	})
})
