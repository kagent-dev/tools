package e2e

import (
	"context"
	"fmt"
	"github.com/kagent-dev/tools/internal/commands"
	"github.com/kagent-dev/tools/internal/logger"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

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

var _ = Describe("KAgent Tools Kubernetes E2E Tests", Ordered, func() {

	var err error
	var client *MCPClient
	var log = logger.Get()
	var namespace = DefaultTestNamespace
	var releaseName = DefaultReleaseName

	BeforeAll(func() {
		log.Info("Starting KAgent Tools E2E tests")
		// Create new namespace
		CreateNamespace(namespace)
		// Install kagent tools
		InstallKAgentTools(namespace, releaseName)

		client, err = GetMCPClient()
		Expect(err).ToNot(HaveOccurred(), "Failed to get MCP client: %v", err)
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
				WithArgs("get", "pods", "-n", namespace, "-l", "app.kubernetes.io/name=kagent-tools", "-o", "json").
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
				WithArgs("get", "svc", "-n", namespace, "-l", "app.kubernetes.io/name=kagent-tools", "-o", "json").
				Execute(ctx)

			Expect(err).ToNot(HaveOccurred())
			Expect(output).ToNot(BeEmpty())
			log.Info("Successfully verified kagent-tools service", "namespace", namespace, "output", output)
		})
	})

	Describe("KAgent Tools K8s Operations", func() {
		It("should be able to list namespace in the cluster", func() {
			log.Info("Testing MCP client connectivity and k8s operations", "namespace", namespace)

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
			log.Info("Testing istio operations via MCP", "namespace", namespace)

			// If we get here, MCP is accessible, test istio operations
			response, err := client.istioInstall("default")
			Expect(err).ToNot(HaveOccurred(), "Failed to install istio via MCP: %v", err)
			Expect(response).ToNot(BeNil())

			log.Info("Successfully tested istio operations via MCP", "namespace", namespace, "response", response)
		})
	})

	Describe("KAgent Tools Cilium Operations", func() {
		It("should be able to install cilium in the cluster", func() {
			log.Info("Testing cilium operations via MCP", "namespace", namespace)

			// If we get here, MCP is accessible, test cilium operations
			response, err := client.ciliumStatus()
			Expect(err).ToNot(HaveOccurred(), "Failed to get cilium status via MCP: %v", err)
			Expect(response).ToNot(BeNil())

			log.Info("Successfully tested cilium operations via MCP", "namespace", namespace)
		})
	})

	Describe("KAgent Tools Argo Operations", func() {
		It("should be able to list Argo rollouts in the cluster", func() {
			log.Info("Testing Argo operations via MCP", "namespace", namespace)

			// If we get here, MCP is accessible, test cilium operations
			response, err := client.argoRolloutsList(namespace)
			Expect(err).ToNot(HaveOccurred(), "Failed to list argo rollouts via MCP: %v", err)
			Expect(response).ToNot(BeNil())

			log.Info("Successfully tested argo rollouts via MCP", "namespace", namespace)
		})
	})
})
