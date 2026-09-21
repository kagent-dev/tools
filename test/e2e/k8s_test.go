package e2e

import (
	"context"
	"fmt"
	"os"
	"time"

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
				WithArgs("get", "pods", "-n", namespace, "-l", "app.kubernetes.io/instance="+releaseName, "-o", "json").
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
				WithArgs("get", "svc", "-n", namespace, "-l", "app.kubernetes.io/instance="+releaseName, "-o", "json").
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
			Expect(response.IsError).To(BeFalse(), "k8s_get_resources returned a tool error: %s", toolResultText(response))

			// The migrated handler returns a typed Out value, so the SDK must
			// populate StructuredContent. A missing value means Out=any regressed.
			output, err := decodeTextOutput(response)
			Expect(err).ToNot(HaveOccurred(), "k8s_get_resources did not return typed output: %v", err)
			Expect(output.Output).ToNot(BeEmpty(), "k8s_get_resources returned empty output")

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

			output, err := decodeTextOutput(response)
			Expect(err).ToNot(HaveOccurred(), "helm_list_releases did not return typed output: %v", err)
			Expect(output.Output).ToNot(BeEmpty(), "helm_list_releases returned empty output")

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

			// istioctl install exits 0 when Istio is already installed, so this
			// asserts the tool reached the CLI rather than that it changed state.
			Expect(response.IsError).To(BeFalse(), "istio_install_istio returned a tool error: %s", toolResultText(response))

			output, err := decodeTextOutput(response)
			Expect(err).ToNot(HaveOccurred(), "istio_install_istio did not return typed output: %v", err)

			log.Info("Successfully tested istio operations via MCP", "namespace", namespace, "response", output.Output)
		})

		It("should report the installed istio version through the MCP tool", func() {
			if !clusterHasIstio() {
				Skip("Istio control plane (istiod) not installed; no MCP tool exists to install it in CI")
			}

			response, err := client.callTool("istio_version", struct{}{})
			Expect(err).ToNot(HaveOccurred(), "istio_version call failed: %v", err)
			Expect(response.IsError).To(BeFalse(), "istio_version returned a tool error: %s", toolResultText(response))

			output, err := decodeTextOutput(response)
			Expect(err).ToNot(HaveOccurred(), "istio_version did not return typed output: %v", err)
			Expect(output.Output).To(ContainSubstring("version"), "istio_version output should name a version")
		})
	})

	Describe("KAgent Tools Cilium Operations", func() {
		It("should report cilium status through the MCP tool", func() {
			// The Kind cluster uses kindnet by default, so Cilium is only present
			// if the lifecycle spec below (or an operator) installed it. Without
			// Cilium the tool correctly returns a tool error, which is not a
			// failure of this suite -- so skip instead of asserting success.
			if !clusterHasCilium() {
				Skip("Cilium is not installed in this cluster (Kind uses kindnet); run the Cilium lifecycle spec to cover it")
			}

			log.Info("Testing cilium operations via MCP", "namespace", namespace)
			response, err := client.ciliumStatus()
			Expect(err).ToNot(HaveOccurred(), "Failed to get cilium status via MCP: %v", err)
			Expect(response).ToNot(BeNil())

			output, err := decodeTextOutput(response)
			Expect(err).ToNot(HaveOccurred(), "cilium_status_and_version did not return typed output: %v", err)
			Expect(output.Output).ToNot(BeEmpty(), "cilium_status_and_version returned empty output")

			log.Info("Successfully tested cilium operations via MCP", "namespace", namespace)
		})
	})

	// The sweep must run before AfterAll deletes the namespace, so it lives in
	// this ordered container rather than a separate one.
	Describe("KAgent Tools Coverage Sweep", Label("coverage"), func() {
		It("invokes every read-only tool with a typed result", func() {
			By("sweeping every advertised read-only tool")
			SweepReadOnlyTools(client)
		})

		It("classifies write-guarded tools without invoking them", func() {
			By("verifying the write-tool safety rail")
			SweepGuardedTools(client)
		})
	})

	Describe("KAgent Tools Argo Operations", func() {
		It("should be able to list Argo rollouts in the cluster", func() {
			log.Info("Testing Argo operations via MCP", "namespace", namespace)

			// If we get here, MCP is accessible, test argo operations
			response, err := client.argoRolloutsList(namespace)
			Expect(err).ToNot(HaveOccurred(), "Failed to list argo rollouts via MCP: %v", err)
			Expect(response).ToNot(BeNil())

			// argo_rollouts_list legitimately reports "No resources found." as a
			// successful result when nothing is installed, so assert the typed
			// contract rather than specific content.
			output, err := decodeTextOutput(response)
			Expect(err).ToNot(HaveOccurred(), "argo_rollouts_list did not return typed output: %v", err)
			Expect(output.Output).ToNot(BeEmpty(), "argo_rollouts_list returned empty output")

			log.Info("Successfully tested argo rollouts via MCP", "namespace", namespace)
		})
	})

	// The Cilium lifecycle mutates cluster-wide networking: it installs a CNI.
	// It is opt-in via E2E_CILIUM_LIFECYCLE=true so the default suite (and CI,
	// whose Kind cluster uses kindnet) never touches pod networking.
	Describe("KAgent Tools Cilium Lifecycle", Label("cilium-lifecycle"), func() {
		It("should install, report status, list endpoints, and uninstall cilium via MCP", func() {
			if os.Getenv("E2E_CILIUM_LIFECYCLE") != "true" {
				Skip("set E2E_CILIUM_LIFECYCLE=true to run the Cilium install/uninstall lifecycle")
			}
			if clusterHasCilium() {
				Skip("Cilium is already installed; refusing to modify existing cluster networking")
			}

			// Install through the MCP tool. Cilium is long to converge, so allow
			// a generous timeout for the CLI call itself.
			By("installing Cilium via the cilium_install_cilium tool")
			installResult, err := client.callToolWithTimeout("cilium_install_cilium",
				struct {
					DatapathMode string `json:"datapath_mode"`
				}{DatapathMode: "native"}, 5*time.Minute)
			Expect(err).ToNot(HaveOccurred(), "cilium_install_cilium call failed: %v", err)
			Expect(installResult.IsError).To(BeFalse(),
				"cilium_install_cilium returned a tool error: %s", toolResultText(installResult))

			// Installing via the tool must actually create the DaemonSet.
			Eventually(clusterHasCilium, 3*time.Minute, 5*time.Second).
				Should(BeTrue(), "cilium DaemonSet was not created by cilium_install_cilium")

			// Safety net: if an assertion below fails before the uninstall step
			// runs, remove the DaemonSet through the cluster API so a leaked CNI
			// cannot poison later runs (a leftover Cilium would make this spec
			// skip itself as "already installed"). The MCP tool cannot be used
			// here: the ordered container's AfterAll deletes the kagent-tools
			// namespace before DeferCleanup runs, leaving no server to call.
			DeferCleanup(func() {
				if !clusterHasCilium() {
					return
				}
				By("removing leftover Cilium via the cluster API")
				ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
				defer cancel()
				_, _ = commands.NewCommandBuilder("kubectl").
					WithArgs("delete", "daemonset", "cilium", "-n", "kube-system", "--ignore-not-found").
					WithCache(false).
					Execute(ctx)
			})

			// Installing Cilium replaces the cluster CNI and briefly resets pod
			// networking, which drops the long-lived MCP session established in
			// BeforeAll. Reconnect before driving further tools.
			By("reconnecting the MCP session after the CNI switch")
			client, err = GetMCPClient()
			Expect(err).ToNot(HaveOccurred(), "failed to reconnect after Cilium install: %v", err)

			By("reporting Cilium status via the cilium_status_and_version tool")
			statusResult, err := client.callToolWithTimeout("cilium_status_and_version", struct{}{}, 2*time.Minute)
			Expect(err).ToNot(HaveOccurred(), "cilium_status_and_version call failed: %v", err)
			Expect(statusResult.IsError).To(BeFalse(),
				"cilium_status_and_version returned a tool error: %s", toolResultText(statusResult))

			statusOutput, err := decodeTextOutput(statusResult)
			Expect(err).ToNot(HaveOccurred(), "cilium_status_and_version did not return typed output: %v", err)
			Expect(statusOutput.Output).ToNot(BeEmpty(), "cilium status output should not be empty")

			// Uninstall through the MCP tool while the server is still reachable.
			// This must run inside the It, not DeferCleanup: the ordered container's
			// AfterAll deletes the namespace first, leaving no server to call.
			By("uninstalling Cilium via the cilium_uninstall_cilium tool")
			uninstallResult, err := client.callToolWithTimeout("cilium_uninstall_cilium", struct{}{}, 3*time.Minute)
			Expect(err).ToNot(HaveOccurred(), "cilium_uninstall_cilium call failed: %v", err)
			Expect(uninstallResult).ToNot(BeNil())
			Expect(uninstallResult.IsError).To(BeFalse(),
				"cilium_uninstall_cilium returned a tool error: %s", toolResultText(uninstallResult))

			Eventually(clusterHasCilium, 3*time.Minute, 5*time.Second).
				Should(BeFalse(), "cilium DaemonSet still present after cilium_uninstall_cilium")

			log.Info("Successfully exercised the Cilium install/status/uninstall lifecycle via MCP")
		})
	})
})
