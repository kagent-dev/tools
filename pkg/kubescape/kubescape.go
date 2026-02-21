package kubescape

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/kagent-dev/tools/internal/errors"
	"github.com/kagent-dev/tools/internal/mcpcompat"
	"github.com/kagent-dev/tools/internal/mcpcompat/server"
	"github.com/kagent-dev/tools/internal/telemetry"
	helpersv1 "github.com/kubescape/k8s-interface/instanceidhandler/v1/helpers"
	"github.com/kubescape/storage/pkg/apis/softwarecomposition/v1beta1"
	spdxv1beta1 "github.com/kubescape/storage/pkg/generated/clientset/versioned/typed/softwarecomposition/v1beta1"
	corev1 "k8s.io/api/core/v1"
	apiextensionsclientset "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

const (
	defaultKubescapeNamespace = "kubescape"

	// CRD names
	vulnerabilityManifestsCRD     = "vulnerabilitymanifests.spdx.softwarecomposition.kubescape.io"
	workloadConfigurationScansCRD = "workloadconfigurationscans.spdx.softwarecomposition.kubescape.io"
	applicationProfilesCRD        = "applicationprofiles.spdx.softwarecomposition.kubescape.io"
	networkNeighborhoodsCRD       = "networkneighborhoods.spdx.softwarecomposition.kubescape.io"
	sbomSyftsCRD                  = "sbomsyfts.spdx.softwarecomposition.kubescape.io"

	// Pod labels
	operatorPodLabel = "app.kubernetes.io/name=kubescape-operator"
	storagePodLabel  = "app.kubernetes.io/name=storage"
)

// KubescapeTool holds the clients for Kubescape and Kubernetes APIs
type KubescapeTool struct {
	spdxClient   spdxv1beta1.SpdxV1beta1Interface
	k8sClient    kubernetes.Interface
	apiExtClient apiextensionsclientset.Interface
	initError    error
}

// NewKubescapeTool creates a new KubescapeTool with Kubernetes clients
func NewKubescapeTool(kubeconfig string) *KubescapeTool {
	tool := &KubescapeTool{}

	config, err := getKubeConfig(kubeconfig)
	if err != nil {
		tool.initError = fmt.Errorf("failed to create kubernetes config: %w", err)
		return tool
	}

	// Create standard Kubernetes client
	k8sClient, err := kubernetes.NewForConfig(config)
	if err != nil {
		tool.initError = fmt.Errorf("failed to create kubernetes client: %w", err)
		return tool
	}
	tool.k8sClient = k8sClient

	// Create API extensions client for CRD checks
	apiExtClient, err := apiextensionsclientset.NewForConfig(config)
	if err != nil {
		tool.initError = fmt.Errorf("failed to create apiextensions client: %w", err)
		return tool
	}
	tool.apiExtClient = apiExtClient

	// Create Kubescape storage client
	spdxClient, err := spdxv1beta1.NewForConfig(config)
	if err != nil {
		tool.initError = fmt.Errorf("failed to create kubescape client: %w", err)
		return tool
	}
	tool.spdxClient = spdxClient

	return tool
}

func getKubeConfig(kubeconfig string) (*rest.Config, error) {
	if kubeconfig != "" {
		return clientcmd.BuildConfigFromFlags("", kubeconfig)
	}
	// Try in-cluster config first, then fall back to default kubeconfig location
	config, err := rest.InClusterConfig()
	if err != nil {
		// Fall back to default kubeconfig
		loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
		configOverrides := &clientcmd.ConfigOverrides{}
		kubeConfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, configOverrides)
		return kubeConfig.ClientConfig()
	}
	return config, nil
}

// HealthCheckResult represents the result of a health check
type HealthCheckResult struct {
	Healthy         bool                   `json:"healthy"`
	Checks          map[string]CheckStatus `json:"checks"`
	Summary         string                 `json:"summary"`
	Recommendations []string               `json:"recommendations,omitempty"`
}

// CheckStatus represents the status of a single check
type CheckStatus struct {
	Status  string      `json:"status"`
	Message string      `json:"message"`
	Details interface{} `json:"details,omitempty"`
}

// Typed response models for MCP payloads.
type vulnerabilityManifestSummary struct {
	Namespace             string `json:"namespace"`
	ManifestName          string `json:"manifest_name"`
	ImageLevel            bool   `json:"image_level"`
	WorkloadLevel         bool   `json:"workload_level"`
	ImageID               string `json:"image_id,omitempty"`
	ImageTag              string `json:"image_tag,omitempty"`
	WorkloadID            string `json:"workload_id,omitempty"`
	WorkloadContainerName string `json:"workload_container_name,omitempty"`
	VulnerabilityCount    int    `json:"vulnerability_count"`
}

type listVulnerabilityManifestsResponse struct {
	VulnerabilityManifests []vulnerabilityManifestSummary `json:"vulnerability_manifests"`
	TotalCount             int                            `json:"total_count"`
}

type vulnerabilitySummary struct {
	ID          string   `json:"id"`
	Severity    string   `json:"severity"`
	Description string   `json:"description"`
	DataSource  string   `json:"data_source"`
	FixState    string   `json:"fix_state,omitempty"`
	FixVersions []string `json:"fix_versions,omitempty"`
}

type listVulnerabilitiesResponse struct {
	ManifestName    string                 `json:"manifest_name"`
	Namespace       string                 `json:"namespace"`
	TotalCount      int                    `json:"total_count"`
	SeveritySummary map[string]int         `json:"severity_summary"`
	Vulnerabilities []vulnerabilitySummary `json:"vulnerabilities"`
}

type configurationScanSummary struct {
	Namespace    string `json:"namespace"`
	ManifestName string `json:"manifest_name"`
	CreatedAt    string `json:"created_at"`
}

type listConfigurationScansResponse struct {
	ConfigurationScans []configurationScanSummary `json:"configuration_scans"`
	TotalCount         int                        `json:"total_count"`
}

type podStatusDetail struct {
	Name   string `json:"name"`
	Status string `json:"status"`
}

type applicationProfileSummary struct {
	Namespace                string `json:"namespace"`
	Name                     string `json:"name"`
	ContainersCount          int    `json:"containers_count"`
	InitContainersCount      int    `json:"init_containers_count"`
	EphemeralContainersCount int    `json:"ephemeral_containers_count"`
	TotalExecs               int    `json:"total_execs"`
	TotalOpens               int    `json:"total_opens"`
	TotalSyscalls            int    `json:"total_syscalls"`
	TotalCapabilities        int    `json:"total_capabilities"`
	TotalEndpoints           int    `json:"total_endpoints"`
	CreatedAt                string `json:"created_at"`
}

type listApplicationProfilesResponse struct {
	ApplicationProfiles []applicationProfileSummary `json:"application_profiles"`
	TotalCount          int                         `json:"total_count"`
	Description         string                      `json:"description"`
}

type profileContainerBehavior struct {
	Name           string                        `json:"name"`
	Execs          []v1beta1.ExecCalls           `json:"execs"`
	Opens          []v1beta1.OpenCalls           `json:"opens"`
	Syscalls       []string                      `json:"syscalls"`
	Capabilities   []string                      `json:"capabilities"`
	Endpoints      []v1beta1.HTTPEndpoint        `json:"endpoints"`
	SeccompProfile *v1beta1.SingleSeccompProfile `json:"seccomp_profile,omitempty"`
}

type getApplicationProfileResponse struct {
	Namespace      string                     `json:"namespace"`
	Name           string                     `json:"name"`
	Containers     []profileContainerBehavior `json:"containers"`
	InitContainers []profileContainerBehavior `json:"init_containers"`
	Annotations    map[string]string          `json:"annotations,omitempty"`
	Labels         map[string]string          `json:"labels,omitempty"`
	Description    string                     `json:"description"`
}

type networkNeighborhoodSummary struct {
	Namespace       string `json:"namespace"`
	Name            string `json:"name"`
	ContainersCount int    `json:"containers_count"`
	TotalIngress    int    `json:"total_ingress"`
	TotalEgress     int    `json:"total_egress"`
	CreatedAt       string `json:"created_at"`
}

type listNetworkNeighborhoodsResponse struct {
	NetworkNeighborhoods []networkNeighborhoodSummary `json:"network_neighborhoods"`
	TotalCount           int                          `json:"total_count"`
	Description          string                       `json:"description"`
}

type networkPeer struct {
	Identifier        string                    `json:"identifier"`
	Type              v1beta1.CommunicationType `json:"type"`
	DNS               string                    `json:"dns,omitempty"`
	Ports             []v1beta1.NetworkPort     `json:"ports,omitempty"`
	IPAddress         string                    `json:"ip_address,omitempty"`
	PodSelector       *metav1.LabelSelector     `json:"pod_selector,omitempty"`
	NamespaceSelector *metav1.LabelSelector     `json:"namespace_selector,omitempty"`
}

type networkContainerConnections struct {
	Name    string        `json:"name"`
	Ingress []networkPeer `json:"ingress"`
	Egress  []networkPeer `json:"egress"`
}

type getNetworkNeighborhoodResponse struct {
	Namespace   string                        `json:"namespace"`
	Name        string                        `json:"name"`
	Containers  []networkContainerConnections `json:"containers"`
	Annotations map[string]string             `json:"annotations,omitempty"`
	Labels      map[string]string             `json:"labels,omitempty"`
	Description string                        `json:"description"`
}

// handleCheckHealth verifies Kubescape operator installation and readiness
func (k *KubescapeTool) handleCheckHealth(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	if k.initError != nil {
		toolErr := errors.NewKubescapeError("check_health", k.initError)
		return toolErr.ToMCPResult(), nil
	}

	namespace := mcp.ParseString(request, "namespace", defaultKubescapeNamespace)

	result := HealthCheckResult{
		Healthy: true,
		Checks:  make(map[string]CheckStatus),
	}
	var recommendations []string

	// Check 1: Namespace exists
	_, err := k.k8sClient.CoreV1().Namespaces().Get(ctx, namespace, metav1.GetOptions{})
	if err != nil {
		if k8serrors.IsNotFound(err) {
			result.Checks["namespace"] = CheckStatus{
				Status:  "error",
				Message: fmt.Sprintf("Namespace '%s' not found", namespace),
			}
			result.Healthy = false
			recommendations = append(recommendations, fmt.Sprintf("Create the namespace: kubectl create namespace %s", namespace))
		} else {
			result.Checks["namespace"] = CheckStatus{
				Status:  "error",
				Message: fmt.Sprintf("Failed to check namespace: %v", err),
			}
			result.Healthy = false
		}
	} else {
		result.Checks["namespace"] = CheckStatus{
			Status:  "ok",
			Message: fmt.Sprintf("Namespace '%s' exists", namespace),
		}
	}

	// Check 2: Operator pods running
	operatorPods, err := k.k8sClient.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{
		LabelSelector: operatorPodLabel,
	})
	if err != nil {
		result.Checks["operator_pods"] = CheckStatus{
			Status:  "error",
			Message: fmt.Sprintf("Failed to list operator pods: %v", err),
		}
		result.Healthy = false
	} else if len(operatorPods.Items) == 0 {
		result.Checks["operator_pods"] = CheckStatus{
			Status:  "error",
			Message: "No operator pods found",
		}
		result.Healthy = false
		recommendations = append(recommendations, "Install Kubescape operator: helm upgrade --install kubescape kubescape/kubescape-operator -n kubescape --create-namespace")
	} else {
		runningCount := 0
		podDetails := make([]podStatusDetail, 0, len(operatorPods.Items))
		for _, pod := range operatorPods.Items {
			status := string(pod.Status.Phase)
			if pod.Status.Phase == corev1.PodRunning {
				runningCount++
			}
			podDetails = append(podDetails, podStatusDetail{
				Name:   pod.Name,
				Status: status,
			})
		}
		if runningCount == len(operatorPods.Items) {
			result.Checks["operator_pods"] = CheckStatus{
				Status:  "ok",
				Message: fmt.Sprintf("%d/%d pods running", runningCount, len(operatorPods.Items)),
				Details: podDetails,
			}
		} else {
			result.Checks["operator_pods"] = CheckStatus{
				Status:  "warning",
				Message: fmt.Sprintf("%d/%d pods running", runningCount, len(operatorPods.Items)),
				Details: podDetails,
			}
			result.Healthy = false
			recommendations = append(recommendations, fmt.Sprintf("Check operator logs: kubectl logs -n %s -l %s", namespace, operatorPodLabel))
		}
	}

	// Check 3: Storage pods running
	storagePods, err := k.k8sClient.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{
		LabelSelector: storagePodLabel,
	})
	if err != nil {
		result.Checks["storage_pods"] = CheckStatus{
			Status:  "error",
			Message: fmt.Sprintf("Failed to list storage pods: %v", err),
		}
		result.Healthy = false
	} else if len(storagePods.Items) == 0 {
		result.Checks["storage_pods"] = CheckStatus{
			Status:  "warning",
			Message: "No storage pods found (may be using external storage)",
		}
	} else {
		runningCount := 0
		for _, pod := range storagePods.Items {
			if pod.Status.Phase == corev1.PodRunning {
				runningCount++
			}
		}
		if runningCount == len(storagePods.Items) {
			result.Checks["storage_pods"] = CheckStatus{
				Status:  "ok",
				Message: fmt.Sprintf("%d/%d pods running", runningCount, len(storagePods.Items)),
			}
		} else {
			result.Checks["storage_pods"] = CheckStatus{
				Status:  "warning",
				Message: fmt.Sprintf("%d/%d pods running", runningCount, len(storagePods.Items)),
			}
		}
	}

	// Check 4: VulnerabilityManifests CRD exists
	_, err = k.apiExtClient.ApiextensionsV1().CustomResourceDefinitions().Get(ctx, vulnerabilityManifestsCRD, metav1.GetOptions{})
	if err != nil {
		if k8serrors.IsNotFound(err) {
			result.Checks["vulnerability_crd"] = CheckStatus{
				Status:  "error",
				Message: "VulnerabilityManifests CRD not installed - vulnerability scanning may not be enabled",
			}
			result.Healthy = false
			recommendations = append(recommendations,
				"Enable vulnerability scanning in Kubescape Helm chart: helm upgrade --install kubescape kubescape/kubescape-operator -n kubescape --set capabilities.vulnerabilityScan=enable")
		} else {
			result.Checks["vulnerability_crd"] = CheckStatus{
				Status:  "error",
				Message: fmt.Sprintf("Failed to check CRD: %v", err),
			}
			result.Healthy = false
		}
	} else {
		result.Checks["vulnerability_crd"] = CheckStatus{
			Status:  "ok",
			Message: "CRD installed",
		}
	}

	// Check 5: WorkloadConfigurationScans CRD exists
	_, err = k.apiExtClient.ApiextensionsV1().CustomResourceDefinitions().Get(ctx, workloadConfigurationScansCRD, metav1.GetOptions{})
	if err != nil {
		if k8serrors.IsNotFound(err) {
			result.Checks["configuration_crd"] = CheckStatus{
				Status:  "error",
				Message: "WorkloadConfigurationScans CRD not installed - configuration scanning may not be enabled",
			}
			result.Healthy = false
			recommendations = append(recommendations,
				"Enable configuration scanning in Kubescape Helm chart: helm upgrade --install kubescape kubescape/kubescape-operator -n kubescape --set capabilities.continuousScan=enable")
		} else {
			result.Checks["configuration_crd"] = CheckStatus{
				Status:  "error",
				Message: fmt.Sprintf("Failed to check CRD: %v", err),
			}
			result.Healthy = false
		}
	} else {
		result.Checks["configuration_crd"] = CheckStatus{
			Status:  "ok",
			Message: "CRD installed",
		}
	}

	// Check 6: Vulnerability scan data available
	manifests, err := k.spdxClient.VulnerabilityManifests(metav1.NamespaceAll).List(ctx, metav1.ListOptions{Limit: 1})
	if err != nil {
		result.Checks["vulnerability_scan_data"] = CheckStatus{
			Status:  "warning",
			Message: fmt.Sprintf("Failed to list vulnerability manifests: %v", err),
		}
	} else if len(manifests.Items) == 0 {
		result.Checks["vulnerability_scan_data"] = CheckStatus{
			Status:  "warning",
			Message: "No vulnerability manifests found - scans may not have completed yet or vulnerability scanning may be disabled",
		}
		recommendations = append(recommendations,
			"If vulnerability scanning is not working, ensure it is enabled: helm upgrade kubescape kubescape/kubescape-operator -n kubescape --set capabilities.vulnerabilityScan=enable")
	} else {
		// Get actual count
		allManifests, _ := k.spdxClient.VulnerabilityManifests(metav1.NamespaceAll).List(ctx, metav1.ListOptions{})
		count := 0
		if allManifests != nil {
			count = len(allManifests.Items)
		}
		result.Checks["vulnerability_scan_data"] = CheckStatus{
			Status:  "ok",
			Message: fmt.Sprintf("%d vulnerability manifests found", count),
		}
	}

	// Check 7: Configuration scan data available
	configScans, err := k.spdxClient.WorkloadConfigurationScans(metav1.NamespaceAll).List(ctx, metav1.ListOptions{Limit: 1})
	if err != nil {
		result.Checks["configuration_scan_data"] = CheckStatus{
			Status:  "warning",
			Message: fmt.Sprintf("Failed to list configuration scans: %v", err),
		}
	} else if len(configScans.Items) == 0 {
		result.Checks["configuration_scan_data"] = CheckStatus{
			Status:  "warning",
			Message: "No configuration scans found - scans may not have completed yet or continuous scanning may be disabled",
		}
		recommendations = append(recommendations,
			"If configuration scanning is not working, ensure it is enabled: helm upgrade kubescape kubescape/kubescape-operator -n kubescape --set capabilities.continuousScan=enable")
	} else {
		// Get actual count
		allConfigScans, _ := k.spdxClient.WorkloadConfigurationScans(metav1.NamespaceAll).List(ctx, metav1.ListOptions{})
		count := 0
		if allConfigScans != nil {
			count = len(allConfigScans.Items)
		}
		result.Checks["configuration_scan_data"] = CheckStatus{
			Status:  "ok",
			Message: fmt.Sprintf("%d configuration scans found", count),
		}
	}

	// Check 8: ApplicationProfiles CRD exists (runtime observability)
	_, err = k.apiExtClient.ApiextensionsV1().CustomResourceDefinitions().Get(ctx, applicationProfilesCRD, metav1.GetOptions{})
	if err != nil {
		if k8serrors.IsNotFound(err) {
			result.Checks["application_profiles_crd"] = CheckStatus{
				Status:  "warning",
				Message: "ApplicationProfiles CRD not installed - runtime observability may not be enabled",
			}
			recommendations = append(recommendations,
				"Enable runtime observability for workload behavior analysis: helm upgrade kubescape kubescape/kubescape-operator -n kubescape --set capabilities.runtimeObservability=enable")
		} else {
			result.Checks["application_profiles_crd"] = CheckStatus{
				Status:  "error",
				Message: fmt.Sprintf("Failed to check CRD: %v", err),
			}
		}
	} else {
		result.Checks["application_profiles_crd"] = CheckStatus{
			Status:  "ok",
			Message: "CRD installed",
		}

		// Check for ApplicationProfile data
		profiles, listErr := k.spdxClient.ApplicationProfiles(metav1.NamespaceAll).List(ctx, metav1.ListOptions{Limit: 1})
		if listErr != nil {
			result.Checks["application_profiles_data"] = CheckStatus{
				Status:  "warning",
				Message: fmt.Sprintf("Failed to list application profiles: %v", listErr),
			}
		} else if len(profiles.Items) == 0 {
			result.Checks["application_profiles_data"] = CheckStatus{
				Status:  "warning",
				Message: "No application profiles found - runtime learning may not have completed yet",
			}
		} else {
			allProfiles, _ := k.spdxClient.ApplicationProfiles(metav1.NamespaceAll).List(ctx, metav1.ListOptions{})
			count := 0
			if allProfiles != nil {
				count = len(allProfiles.Items)
			}
			result.Checks["application_profiles_data"] = CheckStatus{
				Status:  "ok",
				Message: fmt.Sprintf("%d application profiles found", count),
			}
		}
	}

	// Check 9: NetworkNeighborhoods CRD exists (runtime observability)
	_, err = k.apiExtClient.ApiextensionsV1().CustomResourceDefinitions().Get(ctx, networkNeighborhoodsCRD, metav1.GetOptions{})
	if err != nil {
		if k8serrors.IsNotFound(err) {
			result.Checks["network_neighborhoods_crd"] = CheckStatus{
				Status:  "warning",
				Message: "NetworkNeighborhoods CRD not installed - runtime observability may not be enabled",
			}
			// Only add recommendation if not already added from ApplicationProfiles check
			hasRuntimeRecommendation := false
			for _, r := range recommendations {
				if strings.Contains(r, "runtimeObservability") {
					hasRuntimeRecommendation = true
					break
				}
			}
			if !hasRuntimeRecommendation {
				recommendations = append(recommendations,
					"Enable runtime observability for network analysis: helm upgrade kubescape kubescape/kubescape-operator -n kubescape --set capabilities.runtimeObservability=enable")
			}
		} else {
			result.Checks["network_neighborhoods_crd"] = CheckStatus{
				Status:  "error",
				Message: fmt.Sprintf("Failed to check CRD: %v", err),
			}
		}
	} else {
		result.Checks["network_neighborhoods_crd"] = CheckStatus{
			Status:  "ok",
			Message: "CRD installed",
		}

		// Check for NetworkNeighborhood data
		neighborhoods, listErr := k.spdxClient.NetworkNeighborhoods(metav1.NamespaceAll).List(ctx, metav1.ListOptions{Limit: 1})
		if listErr != nil {
			result.Checks["network_neighborhoods_data"] = CheckStatus{
				Status:  "warning",
				Message: fmt.Sprintf("Failed to list network neighborhoods: %v", listErr),
			}
		} else if len(neighborhoods.Items) == 0 {
			result.Checks["network_neighborhoods_data"] = CheckStatus{
				Status:  "warning",
				Message: "No network neighborhoods found - runtime learning may not have completed yet",
			}
		} else {
			allNeighborhoods, _ := k.spdxClient.NetworkNeighborhoods(metav1.NamespaceAll).List(ctx, metav1.ListOptions{})
			count := 0
			if allNeighborhoods != nil {
				count = len(allNeighborhoods.Items)
			}
			result.Checks["network_neighborhoods_data"] = CheckStatus{
				Status:  "ok",
				Message: fmt.Sprintf("%d network neighborhoods found", count),
			}
		}
	}

	// NOTE: SBOM checks are disabled as SBOM tools are disabled (too large for LLM context)
	// Check 10: SBOMSyfts CRD exists - DISABLED

	// Set summary
	if result.Healthy {
		result.Summary = "Kubescape is fully operational"
	} else {
		result.Summary = "Kubescape has issues that need attention"
		result.Recommendations = recommendations
	}

	content, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to marshal result: %v", err)), nil
	}

	return mcp.NewToolResultText(string(content)), nil
}

// handleListVulnerabilityManifests lists vulnerability manifests at image and workload levels
func (k *KubescapeTool) handleListVulnerabilityManifests(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	if k.initError != nil {
		toolErr := errors.NewKubescapeError("list_vulnerability_manifests", k.initError)
		return toolErr.ToMCPResult(), nil
	}

	namespace := mcp.ParseString(request, "namespace", "")
	level := mcp.ParseString(request, "level", "both")

	// Build label selector based on level
	labelSelector := ""
	switch level {
	case "workload":
		labelSelector = "kubescape.io/context=filtered"
	case "image":
		labelSelector = "kubescape.io/context=non-filtered"
	}

	// Determine namespace to query
	queryNamespace := metav1.NamespaceAll
	if namespace != "" {
		queryNamespace = namespace
	}

	// List manifests
	listOpts := metav1.ListOptions{}
	if labelSelector != "" {
		listOpts.LabelSelector = labelSelector
	}

	manifests, err := k.spdxClient.VulnerabilityManifests(queryNamespace).List(ctx, listOpts)
	if err != nil {
		toolErr := errors.NewKubescapeError("list_vulnerability_manifests", err).
			WithContext("namespace", namespace).
			WithContext("level", level)
		return toolErr.ToMCPResult(), nil
	}

	// Build response
	vulnerabilityManifests := make([]vulnerabilityManifestSummary, 0, len(manifests.Items))
	for _, manifest := range manifests.Items {
		isImageLevel := manifest.Annotations[helpersv1.WlidMetadataKey] == ""
		vulnerabilityManifests = append(vulnerabilityManifests, vulnerabilityManifestSummary{
			Namespace:             manifest.Namespace,
			ManifestName:          manifest.Name,
			ImageLevel:            isImageLevel,
			WorkloadLevel:         !isImageLevel,
			ImageID:               manifest.Annotations[helpersv1.ImageIDMetadataKey],
			ImageTag:              manifest.Annotations[helpersv1.ImageTagMetadataKey],
			WorkloadID:            manifest.Annotations[helpersv1.WlidMetadataKey],
			WorkloadContainerName: manifest.Annotations[helpersv1.ContainerNameMetadataKey],
			VulnerabilityCount:    len(manifest.Spec.Payload.Matches),
		})
	}

	result := listVulnerabilityManifestsResponse{
		VulnerabilityManifests: vulnerabilityManifests,
		TotalCount:             len(vulnerabilityManifests),
	}

	content, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to marshal result: %v", err)), nil
	}

	return mcp.NewToolResultText(string(content)), nil
}

// handleListVulnerabilitiesInManifest lists all CVEs in a specific manifest
func (k *KubescapeTool) handleListVulnerabilitiesInManifest(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	if k.initError != nil {
		toolErr := errors.NewKubescapeError("list_vulnerabilities", k.initError)
		return toolErr.ToMCPResult(), nil
	}

	namespace := mcp.ParseString(request, "namespace", defaultKubescapeNamespace)
	manifestName := mcp.ParseString(request, "manifest_name", "")

	if manifestName == "" {
		return mcp.NewToolResultError("manifest_name parameter is required"), nil
	}

	manifest, err := k.spdxClient.VulnerabilityManifests(namespace).Get(ctx, manifestName, metav1.GetOptions{})
	if err != nil {
		toolErr := errors.NewKubescapeError("get_vulnerability_manifest", err).
			WithContext("namespace", namespace).
			WithContext("manifest_name", manifestName)
		return toolErr.ToMCPResult(), nil
	}

	// Extract vulnerabilities with summary info
	vulnerabilities := make([]vulnerabilitySummary, 0, len(manifest.Spec.Payload.Matches))
	severityCounts := map[string]int{
		"Critical": 0,
		"High":     0,
		"Medium":   0,
		"Low":      0,
		"Unknown":  0,
	}

	for _, match := range manifest.Spec.Payload.Matches {
		vuln := match.Vulnerability
		severity := string(vuln.Severity)
		if _, exists := severityCounts[severity]; exists {
			severityCounts[severity]++
		} else {
			severityCounts["Unknown"]++
		}

		summary := vulnerabilitySummary{
			ID:          vuln.ID,
			Severity:    severity,
			Description: truncateString(vuln.Description, 200),
			DataSource:  vuln.DataSource,
		}

		if vuln.Fix.State != "" {
			summary.FixState = vuln.Fix.State
			summary.FixVersions = vuln.Fix.Versions
		}

		vulnerabilities = append(vulnerabilities, summary)
	}

	result := listVulnerabilitiesResponse{
		ManifestName:    manifestName,
		Namespace:       namespace,
		TotalCount:      len(vulnerabilities),
		SeveritySummary: severityCounts,
		Vulnerabilities: vulnerabilities,
	}

	content, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to marshal result: %v", err)), nil
	}

	return mcp.NewToolResultText(string(content)), nil
}

// handleGetVulnerabilityDetails gets detailed info about a specific CVE in a manifest
func (k *KubescapeTool) handleGetVulnerabilityDetails(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	if k.initError != nil {
		toolErr := errors.NewKubescapeError("get_vulnerability_details", k.initError)
		return toolErr.ToMCPResult(), nil
	}

	namespace := mcp.ParseString(request, "namespace", defaultKubescapeNamespace)
	manifestName := mcp.ParseString(request, "manifest_name", "")
	cveID := mcp.ParseString(request, "cve_id", "")

	if manifestName == "" {
		return mcp.NewToolResultError("manifest_name parameter is required"), nil
	}
	if cveID == "" {
		return mcp.NewToolResultError("cve_id parameter is required"), nil
	}

	manifest, err := k.spdxClient.VulnerabilityManifests(namespace).Get(ctx, manifestName, metav1.GetOptions{})
	if err != nil {
		toolErr := errors.NewKubescapeError("get_vulnerability_manifest", err).
			WithContext("namespace", namespace).
			WithContext("manifest_name", manifestName)
		return toolErr.ToMCPResult(), nil
	}

	// Find matching CVE entries
	var matches []v1beta1.Match
	for _, m := range manifest.Spec.Payload.Matches {
		if m.Vulnerability.ID == cveID {
			matches = append(matches, m)
		}
	}

	if len(matches) == 0 {
		return mcp.NewToolResultError(fmt.Sprintf("CVE %s not found in manifest %s", cveID, manifestName)), nil
	}

	content, err := json.MarshalIndent(matches, "", "  ")
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to marshal result: %v", err)), nil
	}

	return mcp.NewToolResultText(string(content)), nil
}

// handleListConfigurationScans lists configuration security scan results
func (k *KubescapeTool) handleListConfigurationScans(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	if k.initError != nil {
		toolErr := errors.NewKubescapeError("list_configuration_scans", k.initError)
		return toolErr.ToMCPResult(), nil
	}

	namespace := mcp.ParseString(request, "namespace", "")

	queryNamespace := metav1.NamespaceAll
	if namespace != "" {
		queryNamespace = namespace
	}

	manifests, err := k.spdxClient.WorkloadConfigurationScans(queryNamespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		toolErr := errors.NewKubescapeError("list_configuration_scans", err).
			WithContext("namespace", namespace)
		return toolErr.ToMCPResult(), nil
	}

	configManifests := make([]configurationScanSummary, 0, len(manifests.Items))
	for _, manifest := range manifests.Items {
		configManifests = append(configManifests, configurationScanSummary{
			Namespace:    manifest.Namespace,
			ManifestName: manifest.Name,
			CreatedAt:    manifest.CreationTimestamp.Format(time.RFC3339),
		})
	}

	result := listConfigurationScansResponse{
		ConfigurationScans: configManifests,
		TotalCount:         len(configManifests),
	}

	content, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to marshal result: %v", err)), nil
	}

	return mcp.NewToolResultText(string(content)), nil
}

// handleGetConfigurationScan gets details of a specific configuration scan
func (k *KubescapeTool) handleGetConfigurationScan(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	if k.initError != nil {
		toolErr := errors.NewKubescapeError("get_configuration_scan", k.initError)
		return toolErr.ToMCPResult(), nil
	}

	namespace := mcp.ParseString(request, "namespace", defaultKubescapeNamespace)
	manifestName := mcp.ParseString(request, "manifest_name", "")

	if manifestName == "" {
		return mcp.NewToolResultError("manifest_name parameter is required"), nil
	}

	manifest, err := k.spdxClient.WorkloadConfigurationScans(namespace).Get(ctx, manifestName, metav1.GetOptions{})
	if err != nil {
		toolErr := errors.NewKubescapeError("get_configuration_scan", err).
			WithContext("namespace", namespace).
			WithContext("manifest_name", manifestName)
		return toolErr.ToMCPResult(), nil
	}

	content, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to marshal result: %v", err)), nil
	}

	return mcp.NewToolResultText(string(content)), nil
}

// handleListApplicationProfiles lists application profiles showing runtime behavior data
func (k *KubescapeTool) handleListApplicationProfiles(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	if k.initError != nil {
		toolErr := errors.NewKubescapeError("list_application_profiles", k.initError)
		return toolErr.ToMCPResult(), nil
	}

	namespace := mcp.ParseString(request, "namespace", "")

	queryNamespace := metav1.NamespaceAll
	if namespace != "" {
		queryNamespace = namespace
	}

	profiles, err := k.spdxClient.ApplicationProfiles(queryNamespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		toolErr := errors.NewKubescapeError("list_application_profiles", err).
			WithContext("namespace", namespace)
		return toolErr.ToMCPResult(), nil
	}

	profileList := make([]applicationProfileSummary, 0, len(profiles.Items))
	for _, profile := range profiles.Items {
		// Summarize what data is captured per container
		containersCount := len(profile.Spec.Containers)
		initContainersCount := len(profile.Spec.InitContainers)
		ephemeralContainersCount := len(profile.Spec.EphemeralContainers)

		totalExecs := 0
		totalOpens := 0
		totalSyscalls := 0
		totalCapabilities := 0
		totalEndpoints := 0

		for _, c := range profile.Spec.Containers {
			totalExecs += len(c.Execs)
			totalOpens += len(c.Opens)
			totalSyscalls += len(c.Syscalls)
			totalCapabilities += len(c.Capabilities)
			totalEndpoints += len(c.Endpoints)
		}

		profileList = append(profileList, applicationProfileSummary{
			Namespace:                profile.Namespace,
			Name:                     profile.Name,
			ContainersCount:          containersCount,
			InitContainersCount:      initContainersCount,
			EphemeralContainersCount: ephemeralContainersCount,
			TotalExecs:               totalExecs,
			TotalOpens:               totalOpens,
			TotalSyscalls:            totalSyscalls,
			TotalCapabilities:        totalCapabilities,
			TotalEndpoints:           totalEndpoints,
			CreatedAt:                profile.CreationTimestamp.Format(time.RFC3339),
		})
	}

	result := listApplicationProfilesResponse{
		ApplicationProfiles: profileList,
		TotalCount:          len(profileList),
		Description: "ApplicationProfiles capture runtime behavior of workloads including: " +
			"executed processes (Execs), file access patterns (Opens), system calls (Syscalls), " +
			"Linux capabilities used, and HTTP endpoints accessed. " +
			"Use this data to prioritize vulnerabilities - a CVE in an unused package is lower priority than one in an actively running process.",
	}

	content, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to marshal result: %v", err)), nil
	}

	return mcp.NewToolResultText(string(content)), nil
}

// handleGetApplicationProfile gets detailed runtime behavior for a specific workload
func (k *KubescapeTool) handleGetApplicationProfile(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	if k.initError != nil {
		toolErr := errors.NewKubescapeError("get_application_profile", k.initError)
		return toolErr.ToMCPResult(), nil
	}

	namespace := mcp.ParseString(request, "namespace", "")
	name := mcp.ParseString(request, "name", "")

	if name == "" {
		return mcp.NewToolResultError("name parameter is required"), nil
	}
	if namespace == "" {
		return mcp.NewToolResultError("namespace parameter is required"), nil
	}

	profile, err := k.spdxClient.ApplicationProfiles(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		toolErr := errors.NewKubescapeError("get_application_profile", err).
			WithContext("namespace", namespace).
			WithContext("name", name)
		return toolErr.ToMCPResult(), nil
	}

	// Build detailed response with container behaviors
	containers := make([]profileContainerBehavior, 0, len(profile.Spec.Containers))
	for _, c := range profile.Spec.Containers {
		containerInfo := profileContainerBehavior{
			Name:         c.Name,
			Execs:        c.Execs,
			Opens:        c.Opens,
			Syscalls:     c.Syscalls,
			Capabilities: c.Capabilities,
			Endpoints:    c.Endpoints,
		}
		if c.SeccompProfile.Name != "" || c.SeccompProfile.Path != "" {
			seccomp := c.SeccompProfile
			containerInfo.SeccompProfile = &seccomp
		}
		containers = append(containers, containerInfo)
	}

	initContainers := make([]profileContainerBehavior, 0, len(profile.Spec.InitContainers))
	for _, c := range profile.Spec.InitContainers {
		initContainers = append(initContainers, profileContainerBehavior{
			Name:         c.Name,
			Execs:        c.Execs,
			Opens:        c.Opens,
			Syscalls:     c.Syscalls,
			Capabilities: c.Capabilities,
			Endpoints:    c.Endpoints,
		})
	}

	result := getApplicationProfileResponse{
		Namespace:      namespace,
		Name:           name,
		Containers:     containers,
		InitContainers: initContainers,
		Annotations:    profile.Annotations,
		Labels:         profile.Labels,
		Description: "This ApplicationProfile shows what the workload containers actually execute at runtime. " +
			"Execs: processes that run; Opens: files read/written; Syscalls: kernel-level operations; " +
			"Capabilities: special Linux privileges; Endpoints: HTTP APIs called. " +
			"Compare this with vulnerability findings to prioritize remediation - focus on CVEs affecting actively used components.",
	}

	content, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to marshal result: %v", err)), nil
	}

	return mcp.NewToolResultText(string(content)), nil
}

// handleListNetworkNeighborhoods lists network communication patterns for workloads
func (k *KubescapeTool) handleListNetworkNeighborhoods(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	if k.initError != nil {
		toolErr := errors.NewKubescapeError("list_network_neighborhoods", k.initError)
		return toolErr.ToMCPResult(), nil
	}

	namespace := mcp.ParseString(request, "namespace", "")

	queryNamespace := metav1.NamespaceAll
	if namespace != "" {
		queryNamespace = namespace
	}

	neighborhoods, err := k.spdxClient.NetworkNeighborhoods(queryNamespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		toolErr := errors.NewKubescapeError("list_network_neighborhoods", err).
			WithContext("namespace", namespace)
		return toolErr.ToMCPResult(), nil
	}

	neighborhoodList := make([]networkNeighborhoodSummary, 0, len(neighborhoods.Items))
	for _, nn := range neighborhoods.Items {
		totalIngress := 0
		totalEgress := 0
		for _, c := range nn.Spec.Containers {
			totalIngress += len(c.Ingress)
			totalEgress += len(c.Egress)
		}

		neighborhoodList = append(neighborhoodList, networkNeighborhoodSummary{
			Namespace:       nn.Namespace,
			Name:            nn.Name,
			ContainersCount: len(nn.Spec.Containers),
			TotalIngress:    totalIngress,
			TotalEgress:     totalEgress,
			CreatedAt:       nn.CreationTimestamp.Format(time.RFC3339),
		})
	}

	result := listNetworkNeighborhoodsResponse{
		NetworkNeighborhoods: neighborhoodList,
		TotalCount:           len(neighborhoodList),
		Description: "NetworkNeighborhoods capture actual network communication patterns of workloads. " +
			"Ingress: connections coming INTO the workload; Egress: connections going OUT from the workload. " +
			"Includes DNS names, IP addresses, ports, and protocols. " +
			"Use this data to understand attack surface and prioritize network-related security findings.",
	}

	content, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to marshal result: %v", err)), nil
	}

	return mcp.NewToolResultText(string(content)), nil
}

// handleGetNetworkNeighborhood gets detailed network connections for a specific workload
func (k *KubescapeTool) handleGetNetworkNeighborhood(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	if k.initError != nil {
		toolErr := errors.NewKubescapeError("get_network_neighborhood", k.initError)
		return toolErr.ToMCPResult(), nil
	}

	namespace := mcp.ParseString(request, "namespace", "")
	name := mcp.ParseString(request, "name", "")

	if name == "" {
		return mcp.NewToolResultError("name parameter is required"), nil
	}
	if namespace == "" {
		return mcp.NewToolResultError("namespace parameter is required"), nil
	}

	nn, err := k.spdxClient.NetworkNeighborhoods(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		toolErr := errors.NewKubescapeError("get_network_neighborhood", err).
			WithContext("namespace", namespace).
			WithContext("name", name)
		return toolErr.ToMCPResult(), nil
	}

	// Build detailed response with container network data
	containers := make([]networkContainerConnections, 0, len(nn.Spec.Containers))
	for _, c := range nn.Spec.Containers {
		// Format ingress connections
		ingressList := make([]networkPeer, 0, len(c.Ingress))
		for _, ing := range c.Ingress {
			ingressList = append(ingressList, networkPeer{
				Identifier:        ing.Identifier,
				Type:              ing.Type,
				DNS:               ing.DNS,
				Ports:             ing.Ports,
				IPAddress:         ing.IPAddress,
				PodSelector:       ing.PodSelector,
				NamespaceSelector: ing.NamespaceSelector,
			})
		}

		// Format egress connections
		egressList := make([]networkPeer, 0, len(c.Egress))
		for _, egr := range c.Egress {
			egressList = append(egressList, networkPeer{
				Identifier:        egr.Identifier,
				Type:              egr.Type,
				DNS:               egr.DNS,
				Ports:             egr.Ports,
				IPAddress:         egr.IPAddress,
				PodSelector:       egr.PodSelector,
				NamespaceSelector: egr.NamespaceSelector,
			})
		}

		containers = append(containers, networkContainerConnections{
			Name:    c.Name,
			Ingress: ingressList,
			Egress:  egressList,
		})
	}

	result := getNetworkNeighborhoodResponse{
		Namespace:   namespace,
		Name:        name,
		Containers:  containers,
		Annotations: nn.Annotations,
		Labels:      nn.Labels,
		Description: "This NetworkNeighborhood shows actual network connections observed for this workload. " +
			"Ingress connections show what talks TO this workload. Egress connections show what this workload talks TO. " +
			"Use this to verify if a workload with a vulnerability is actually exposed to the network.",
	}

	content, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to marshal result: %v", err)), nil
	}

	return mcp.NewToolResultText(string(content)), nil
}

// Helper function to truncate strings
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// RegisterTools registers all Kubescape tools with the MCP server
func RegisterTools(s *server.MCPServer, kubeconfig string, readOnly bool) {
	tool := NewKubescapeTool(kubeconfig)

	// Health check tool
	s.AddTool(mcp.NewTool("kubescape_check_health",
		mcp.WithDescription("Check if Kubescape operator is installed and operational. Verifies namespace, operator pods, storage pods, CRDs, and scan data availability."),
		mcp.WithString("namespace", mcp.Description("Namespace to check (default: kubescape)")),
	), telemetry.AdaptToolHandler(telemetry.WithTracing("kubescape_check_health", tool.handleCheckHealth)))

	// List vulnerability manifests
	s.AddTool(mcp.NewTool("kubescape_list_vulnerability_manifests",
		mcp.WithDescription("List vulnerability manifests from Kubescape operator. Returns vulnerability scan results at image or workload level."),
		mcp.WithString("namespace", mcp.Description("Filter by namespace (optional, defaults to all namespaces)")),
		mcp.WithString("level", mcp.Description("Type of manifests to list: 'image', 'workload', or 'both' (default: both)")),
	), telemetry.AdaptToolHandler(telemetry.WithTracing("kubescape_list_vulnerability_manifests", tool.handleListVulnerabilityManifests)))

	// List vulnerabilities in a manifest
	s.AddTool(mcp.NewTool("kubescape_list_vulnerabilities",
		mcp.WithDescription("List all CVEs/vulnerabilities found in a specific vulnerability manifest. Returns severity summary and vulnerability details."),
		mcp.WithString("namespace", mcp.Description("Namespace of the manifest (default: kubescape)")),
		mcp.WithString("manifest_name", mcp.Description("Name of the vulnerability manifest"), mcp.Required()),
	), telemetry.AdaptToolHandler(telemetry.WithTracing("kubescape_list_vulnerabilities", tool.handleListVulnerabilitiesInManifest)))

	// Get detailed vulnerability info
	s.AddTool(mcp.NewTool("kubescape_get_vulnerability_details",
		mcp.WithDescription("Get detailed information about a specific CVE in a vulnerability manifest, including affected packages and fix information."),
		mcp.WithString("namespace", mcp.Description("Namespace of the manifest (default: kubescape)")),
		mcp.WithString("manifest_name", mcp.Description("Name of the vulnerability manifest"), mcp.Required()),
		mcp.WithString("cve_id", mcp.Description("CVE identifier (e.g., CVE-2023-12345)"), mcp.Required()),
	), telemetry.AdaptToolHandler(telemetry.WithTracing("kubescape_get_vulnerability_details", tool.handleGetVulnerabilityDetails)))

	// List configuration scans
	s.AddTool(mcp.NewTool("kubescape_list_configuration_scans",
		mcp.WithDescription("List configuration security scan results from Kubescape operator. Shows workloads that have been scanned for security misconfigurations."),
		mcp.WithString("namespace", mcp.Description("Filter by namespace (optional, defaults to all namespaces)")),
	), telemetry.AdaptToolHandler(telemetry.WithTracing("kubescape_list_configuration_scans", tool.handleListConfigurationScans)))

	// Get configuration scan details
	s.AddTool(mcp.NewTool("kubescape_get_configuration_scan",
		mcp.WithDescription("Get detailed configuration security scan results for a specific workload, including failed controls and remediation guidance."),
		mcp.WithString("namespace", mcp.Description("Namespace of the scan (default: kubescape)")),
		mcp.WithString("manifest_name", mcp.Description("Name of the configuration scan manifest"), mcp.Required()),
	), telemetry.AdaptToolHandler(telemetry.WithTracing("kubescape_get_configuration_scan", tool.handleGetConfigurationScan)))

	// List application profiles (runtime observability)
	s.AddTool(mcp.NewTool("kubescape_list_application_profiles",
		mcp.WithDescription("List ApplicationProfiles showing runtime behavior of workloads. These profiles capture: "+
			"executed processes (Execs), file access patterns (Opens), system calls (Syscalls), Linux capabilities used, and HTTP endpoints. "+
			"Use this data to prioritize vulnerability findings - a CVE in an unused package is lower priority than one in an actively running process. "+
			"Requires 'capabilities.runtimeObservability=enable' in Kubescape Helm chart."),
		mcp.WithString("namespace", mcp.Description("Filter by namespace (optional, defaults to all namespaces)")),
	), telemetry.AdaptToolHandler(telemetry.WithTracing("kubescape_list_application_profiles", tool.handleListApplicationProfiles)))

	// Get application profile details
	s.AddTool(mcp.NewTool("kubescape_get_application_profile",
		mcp.WithDescription("Get detailed runtime behavior profile for a specific workload. Shows what processes run, what files are accessed, "+
			"what system calls are made, and what capabilities are used per container. "+
			"Compare with CVE findings to prioritize remediation - focus on vulnerabilities affecting actively used components."),
		mcp.WithString("namespace", mcp.Description("Namespace of the profile"), mcp.Required()),
		mcp.WithString("name", mcp.Description("Name of the application profile"), mcp.Required()),
	), telemetry.AdaptToolHandler(telemetry.WithTracing("kubescape_get_application_profile", tool.handleGetApplicationProfile)))

	// List network neighborhoods (runtime observability)
	s.AddTool(mcp.NewTool("kubescape_list_network_neighborhoods",
		mcp.WithDescription("List NetworkNeighborhoods showing actual network communication patterns of workloads. "+
			"These capture: ingress connections (who talks TO the workload), egress connections (who the workload talks TO), "+
			"including DNS names, IP addresses, ports, and protocols. "+
			"Use this to understand attack surface and prioritize network-related security findings. "+
			"Requires 'capabilities.runtimeObservability=enable' in Kubescape Helm chart."),
		mcp.WithString("namespace", mcp.Description("Filter by namespace (optional, defaults to all namespaces)")),
	), telemetry.AdaptToolHandler(telemetry.WithTracing("kubescape_list_network_neighborhoods", tool.handleListNetworkNeighborhoods)))

	// Get network neighborhood details
	s.AddTool(mcp.NewTool("kubescape_get_network_neighborhood",
		mcp.WithDescription("Get detailed network connections for a specific workload. Shows all observed ingress and egress traffic "+
			"with DNS names, IPs, ports, and protocols. Use this to verify if a workload with a vulnerability is actually exposed to the network."),
		mcp.WithString("namespace", mcp.Description("Namespace of the network neighborhood"), mcp.Required()),
		mcp.WithString("name", mcp.Description("Name of the network neighborhood"), mcp.Required()),
	), telemetry.AdaptToolHandler(telemetry.WithTracing("kubescape_get_network_neighborhood", tool.handleGetNetworkNeighborhood)))

	// NOTE: SBOM tools are disabled as they return too much data for LLM context windows.
	// SBOMs contain detailed package information that can be very large.
	// To enable in the future, uncomment the handlers and tool registrations below.
	//
	// s.AddTool(mcp.NewTool("kubescape_list_sboms", ...))
	// s.AddTool(mcp.NewTool("kubescape_get_sbom", ...))
}

// Interfaces for testing - allows mocking the Kubernetes clients
type KubescapeToolInterface interface {
	HandleCheckHealth(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error)
	HandleListVulnerabilityManifests(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error)
	HandleListVulnerabilitiesInManifest(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error)
	HandleGetVulnerabilityDetails(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error)
	HandleListConfigurationScans(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error)
	HandleGetConfigurationScan(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error)
	HandleListApplicationProfiles(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error)
	HandleGetApplicationProfile(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error)
	HandleListNetworkNeighborhoods(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error)
	HandleGetNetworkNeighborhood(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error)
	// NOTE: SBOM handlers are disabled as they return too much data for LLM context
	// HandleListSBOMs(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error)
	// HandleGetSBOM(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error)
}

// Ensure KubescapeTool implements the interface
var _ KubescapeToolInterface = (*KubescapeTool)(nil)

// Export handler methods for testing
func (k *KubescapeTool) HandleCheckHealth(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return k.handleCheckHealth(ctx, request)
}

func (k *KubescapeTool) HandleListVulnerabilityManifests(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return k.handleListVulnerabilityManifests(ctx, request)
}

func (k *KubescapeTool) HandleListVulnerabilitiesInManifest(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return k.handleListVulnerabilitiesInManifest(ctx, request)
}

func (k *KubescapeTool) HandleGetVulnerabilityDetails(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return k.handleGetVulnerabilityDetails(ctx, request)
}

func (k *KubescapeTool) HandleListConfigurationScans(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return k.handleListConfigurationScans(ctx, request)
}

func (k *KubescapeTool) HandleGetConfigurationScan(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return k.handleGetConfigurationScan(ctx, request)
}

func (k *KubescapeTool) HandleListApplicationProfiles(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return k.handleListApplicationProfiles(ctx, request)
}

func (k *KubescapeTool) HandleGetApplicationProfile(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return k.handleGetApplicationProfile(ctx, request)
}

func (k *KubescapeTool) HandleListNetworkNeighborhoods(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return k.handleListNetworkNeighborhoods(ctx, request)
}

func (k *KubescapeTool) HandleGetNetworkNeighborhood(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return k.handleGetNetworkNeighborhood(ctx, request)
}

// NOTE: SBOM handlers are disabled as they return too much data for LLM context
// func (k *KubescapeTool) HandleListSBOMs(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
// 	return k.handleListSBOMs(ctx, request)
// }
//
// func (k *KubescapeTool) HandleGetSBOM(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
// 	return k.handleGetSBOM(ctx, request)
// }
