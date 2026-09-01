package kubescape

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/kagent-dev/tools/internal/errors"
	"github.com/kagent-dev/tools/internal/telemetry"
	helpersv1 "github.com/kubescape/k8s-interface/instanceidhandler/v1/helpers"
	"github.com/kubescape/storage/pkg/apis/softwarecomposition/v1beta1"
	spdxv1beta1 "github.com/kubescape/storage/pkg/generated/clientset/versioned/typed/softwarecomposition/v1beta1"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

const (
	defaultKubescapeNamespace = "kubescape"

	// Pod labels.
	//
	// Every pod the kubescape-operator chart creates carries the chart-wide
	// label app.kubernetes.io/name=kubescape-operator, so it cannot identify a
	// single component. Per-component identity lives in the plain `app` label.
	operatorPodLabel = "app=operator"
	storagePodLabel  = "app=storage"

	// Vulnerability manifest levels accepted by the `level` argument of
	// kubescape_list_vulnerability_manifests.
	levelImage    = "image"
	levelWorkload = "workload"
	levelBoth     = "both"

	// Helm remediation offered when a capability looks disabled.
	enableVulnerabilityScan    = "Enable vulnerability scanning: helm upgrade --install kubescape kubescape/kubescape-operator -n kubescape --set capabilities.vulnerabilityScan=enable"
	enableContinuousScan       = "Enable configuration scanning: helm upgrade --install kubescape kubescape/kubescape-operator -n kubescape --set capabilities.continuousScan=enable"
	enableRuntimeObservability = "Enable runtime observability for workload behavior and network analysis: helm upgrade kubescape kubescape/kubescape-operator -n kubescape --set capabilities.runtimeObservability=enable"
)

// KubescapeTool holds the clients for Kubescape and Kubernetes APIs
type KubescapeTool struct {
	spdxClient spdxv1beta1.SpdxV1beta1Interface
	k8sClient  kubernetes.Interface
	initError  error
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

// storageResource describes one resource served by the Kubescape storage
// service through the aggregated API server, and how check_health reports it.
type storageResource struct {
	// apiCheckKey and dataCheckKey are the keys this resource contributes to
	// the health result. They keep their historical *_crd names so existing
	// consumers of this tool's output are unaffected; the resources themselves
	// are not CRDs.
	apiCheckKey  string
	dataCheckKey string
	// displayName is the resource kind as users see it, e.g. "VulnerabilityManifests".
	displayName string
	// dataNoun names the objects in prose, e.g. "vulnerability manifests".
	dataNoun string
	// capability is the Kubescape capability that produces this data.
	capability string
	// required marks resources whose absence makes Kubescape unusable and so
	// fails the health check. Runtime-observability resources only warn.
	required bool
	// apiRecommendation is offered when the API itself is unreachable,
	// dataRecommendation when it responds but holds no data.
	apiRecommendation  string
	dataRecommendation string
	// list reports how many objects exist, or why they could not be read.
	list func(ctx context.Context) (int, error)
}

// storageResources returns the resources check_health probes, in report order.
func (k *KubescapeTool) storageResources() []storageResource {
	return []storageResource{
		{
			apiCheckKey:        "vulnerability_crd",
			dataCheckKey:       "vulnerability_scan_data",
			displayName:        "VulnerabilityManifests",
			dataNoun:           "vulnerability manifests",
			capability:         "vulnerability scanning",
			required:           true,
			apiRecommendation:  enableVulnerabilityScan,
			dataRecommendation: enableVulnerabilityScan,
			list: func(ctx context.Context) (int, error) {
				list, err := k.spdxClient.VulnerabilityManifests(metav1.NamespaceAll).List(ctx, metav1.ListOptions{})
				if err != nil {
					return 0, err
				}
				return len(list.Items), nil
			},
		},
		{
			apiCheckKey:        "configuration_crd",
			dataCheckKey:       "configuration_scan_data",
			displayName:        "WorkloadConfigurationScans",
			dataNoun:           "configuration scans",
			capability:         "configuration scanning",
			required:           true,
			apiRecommendation:  enableContinuousScan,
			dataRecommendation: enableContinuousScan,
			list: func(ctx context.Context) (int, error) {
				list, err := k.spdxClient.WorkloadConfigurationScans(metav1.NamespaceAll).List(ctx, metav1.ListOptions{})
				if err != nil {
					return 0, err
				}
				return len(list.Items), nil
			},
		},
		{
			apiCheckKey:       "application_profiles_crd",
			dataCheckKey:      "application_profiles_data",
			displayName:       "ApplicationProfiles",
			dataNoun:          "application profiles",
			capability:        "runtime observability",
			apiRecommendation: enableRuntimeObservability,
			list: func(ctx context.Context) (int, error) {
				list, err := k.spdxClient.ApplicationProfiles(metav1.NamespaceAll).List(ctx, metav1.ListOptions{})
				if err != nil {
					return 0, err
				}
				return len(list.Items), nil
			},
		},
		{
			apiCheckKey:       "network_neighborhoods_crd",
			dataCheckKey:      "network_neighborhoods_data",
			displayName:       "NetworkNeighborhoods",
			dataNoun:          "network neighborhoods",
			capability:        "runtime observability",
			apiRecommendation: enableRuntimeObservability,
			list: func(ctx context.Context) (int, error) {
				list, err := k.spdxClient.NetworkNeighborhoods(metav1.NamespaceAll).List(ctx, metav1.ListOptions{})
				if err != nil {
					return 0, err
				}
				return len(list.Items), nil
			},
		},
	}
}

// isStorageAPIUnavailable reports whether err means the aggregated API could
// not be reached at all, as opposed to a request that reached it and failed.
// The API server returns 503 when the storage service is down, and 404 when the
// API group is not registered.
func isStorageAPIUnavailable(err error) bool {
	return k8serrors.IsNotFound(err) ||
		k8serrors.IsServiceUnavailable(err) ||
		meta.IsNoMatchError(err)
}

// appendUnique adds rec unless it is already present, so resources that share a
// capability do not recommend the same fix twice.
func appendUnique(recommendations []string, rec string) []string {
	for _, existing := range recommendations {
		if existing == rec {
			return recommendations
		}
	}
	return append(recommendations, rec)
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
		podDetails := []map[string]string{}
		for _, pod := range operatorPods.Items {
			status := string(pod.Status.Phase)
			if pod.Status.Phase == corev1.PodRunning {
				runningCount++
			}
			podDetails = append(podDetails, map[string]string{
				"name":   pod.Name,
				"status": status,
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
		// The storage service backs every read this provider makes, so its
		// absence is fatal rather than advisory.
		result.Checks["storage_pods"] = CheckStatus{
			Status:  "error",
			Message: "No storage pods found - every Kubescape data tool depends on the storage service",
		}
		result.Healthy = false
		recommendations = append(recommendations, fmt.Sprintf("Check the storage deployment: kubectl get pods -n %s -l %s", namespace, storagePodLabel))
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

	// Checks 4-9: the storage-backed resources.
	//
	// These resources are served by the Kubescape storage service through an
	// aggregated API server, NOT by CRDs -- so their availability is probed by
	// listing them, which is also exactly what the data tools do. A single list
	// per resource answers both "is the API there?" and "is there any data?".
	for _, res := range k.storageResources() {
		count, listErr := res.list(ctx)
		switch {
		case listErr == nil:
			result.Checks[res.apiCheckKey] = CheckStatus{
				Status:  "ok",
				Message: fmt.Sprintf("%s API available", res.displayName),
			}
			if count == 0 {
				result.Checks[res.dataCheckKey] = CheckStatus{
					Status:  "warning",
					Message: fmt.Sprintf("No %s found - scans may not have completed yet", res.dataNoun),
				}
				if res.dataRecommendation != "" {
					recommendations = appendUnique(recommendations, res.dataRecommendation)
				}
			} else {
				result.Checks[res.dataCheckKey] = CheckStatus{
					Status:  "ok",
					Message: fmt.Sprintf("%d %s found", count, res.dataNoun),
				}
			}

		case isStorageAPIUnavailable(listErr):
			status := "warning"
			if res.required {
				status = "error"
				result.Healthy = false
			}
			result.Checks[res.apiCheckKey] = CheckStatus{
				Status: status,
				Message: fmt.Sprintf(
					"%s API not available - the Kubescape storage service may be unavailable, or %s may not be enabled",
					res.displayName, res.capability),
			}
			result.Checks[res.dataCheckKey] = CheckStatus{
				Status:  status,
				Message: fmt.Sprintf("Cannot read %s while the %s API is unavailable", res.dataNoun, res.displayName),
			}
			if res.apiRecommendation != "" {
				recommendations = appendUnique(recommendations, res.apiRecommendation)
			}

		default:
			result.Checks[res.apiCheckKey] = CheckStatus{
				Status:  "error",
				Message: fmt.Sprintf("Failed to query %s: %v", res.displayName, listErr),
			}
			result.Checks[res.dataCheckKey] = CheckStatus{
				Status:  "error",
				Message: fmt.Sprintf("Failed to list %s: %v", res.dataNoun, listErr),
			}
			if res.required {
				result.Healthy = false
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

	switch level {
	case levelImage, levelWorkload, levelBoth:
	default:
		toolErr := errors.NewKubescapeError("list_vulnerability_manifests",
			fmt.Errorf("invalid level %q: must be one of %q, %q or %q", level, levelImage, levelWorkload, levelBoth))
		return toolErr.ToMCPResult(), nil
	}

	// Determine namespace to query
	queryNamespace := metav1.NamespaceAll
	if namespace != "" {
		queryNamespace = namespace
	}

	// Filtering is done client-side below rather than with a labelSelector: the
	// Kubescape storage API server ignores labelSelector on list and returns
	// every object regardless (kubescape/storage#363), so a server-side filter
	// silently returns unfiltered results.
	manifests, err := k.spdxClient.VulnerabilityManifests(queryNamespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		toolErr := errors.NewKubescapeError("list_vulnerability_manifests", err).
			WithContext("namespace", namespace).
			WithContext("level", level)
		return toolErr.ToMCPResult(), nil
	}

	// Build response
	vulnerabilityManifests := []map[string]interface{}{}
	for _, manifest := range manifests.Items {
		// A workload-level manifest carries the workload it was filtered for;
		// an image-level one does not. This is the same predicate reported as
		// image_level/workload_level below, so the filter and the output can
		// never disagree.
		isImageLevel := manifest.Annotations[helpersv1.WlidMetadataKey] == ""
		if (level == levelImage && !isImageLevel) || (level == levelWorkload && isImageLevel) {
			continue
		}
		manifestMap := map[string]interface{}{
			"namespace":               manifest.Namespace,
			"manifest_name":           manifest.Name,
			"image_level":             isImageLevel,
			"workload_level":          !isImageLevel,
			"image_id":                manifest.Annotations[helpersv1.ImageIDMetadataKey],
			"image_tag":               manifest.Annotations[helpersv1.ImageTagMetadataKey],
			"workload_id":             manifest.Annotations[helpersv1.WlidMetadataKey],
			"workload_container_name": manifest.Annotations[helpersv1.ContainerNameMetadataKey],
			"vulnerability_count":     len(manifest.Spec.Payload.Matches),
		}
		vulnerabilityManifests = append(vulnerabilityManifests, manifestMap)
	}

	result := map[string]interface{}{
		"vulnerability_manifests": vulnerabilityManifests,
		"total_count":             len(vulnerabilityManifests),
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
	vulnerabilities := []map[string]interface{}{}
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

		vulnInfo := map[string]interface{}{
			"id":          vuln.ID,
			"severity":    severity,
			"description": truncateString(vuln.Description, 200),
			"data_source": vuln.DataSource,
		}

		if vuln.Fix.State != "" {
			vulnInfo["fix_state"] = vuln.Fix.State
			vulnInfo["fix_versions"] = vuln.Fix.Versions
		}

		vulnerabilities = append(vulnerabilities, vulnInfo)
	}

	result := map[string]interface{}{
		"manifest_name":    manifestName,
		"namespace":        namespace,
		"total_count":      len(vulnerabilities),
		"severity_summary": severityCounts,
		"vulnerabilities":  vulnerabilities,
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

	configManifests := []map[string]interface{}{}
	for _, manifest := range manifests.Items {
		item := map[string]interface{}{
			"namespace":     manifest.Namespace,
			"manifest_name": manifest.Name,
			"created_at":    manifest.CreationTimestamp.Format(time.RFC3339),
		}
		configManifests = append(configManifests, item)
	}

	result := map[string]interface{}{
		"configuration_scans": configManifests,
		"total_count":         len(configManifests),
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

	profileList := []map[string]interface{}{}
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

		profileMap := map[string]interface{}{
			"namespace":                  profile.Namespace,
			"name":                       profile.Name,
			"containers_count":           containersCount,
			"init_containers_count":      initContainersCount,
			"ephemeral_containers_count": ephemeralContainersCount,
			"total_execs":                totalExecs,
			"total_opens":                totalOpens,
			"total_syscalls":             totalSyscalls,
			"total_capabilities":         totalCapabilities,
			"total_endpoints":            totalEndpoints,
			"created_at":                 profile.CreationTimestamp.Format(time.RFC3339),
		}
		profileList = append(profileList, profileMap)
	}

	result := map[string]interface{}{
		"application_profiles": profileList,
		"total_count":          len(profileList),
		"description": "ApplicationProfiles capture runtime behavior of workloads including: " +
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
	containers := []map[string]interface{}{}
	for _, c := range profile.Spec.Containers {
		containerInfo := map[string]interface{}{
			"name":         c.Name,
			"execs":        c.Execs,
			"opens":        c.Opens,
			"syscalls":     c.Syscalls,
			"capabilities": c.Capabilities,
			"endpoints":    c.Endpoints,
		}
		if c.SeccompProfile.Name != "" || c.SeccompProfile.Path != "" {
			containerInfo["seccomp_profile"] = c.SeccompProfile
		}
		containers = append(containers, containerInfo)
	}

	initContainers := []map[string]interface{}{}
	for _, c := range profile.Spec.InitContainers {
		containerInfo := map[string]interface{}{
			"name":         c.Name,
			"execs":        c.Execs,
			"opens":        c.Opens,
			"syscalls":     c.Syscalls,
			"capabilities": c.Capabilities,
			"endpoints":    c.Endpoints,
		}
		initContainers = append(initContainers, containerInfo)
	}

	result := map[string]interface{}{
		"namespace":       namespace,
		"name":            name,
		"containers":      containers,
		"init_containers": initContainers,
		"annotations":     profile.Annotations,
		"labels":          profile.Labels,
		"description": "This ApplicationProfile shows what the workload containers actually execute at runtime. " +
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

	neighborhoodList := []map[string]interface{}{}
	for _, nn := range neighborhoods.Items {
		totalIngress := 0
		totalEgress := 0
		for _, c := range nn.Spec.Containers {
			totalIngress += len(c.Ingress)
			totalEgress += len(c.Egress)
		}

		nnMap := map[string]interface{}{
			"namespace":        nn.Namespace,
			"name":             nn.Name,
			"containers_count": len(nn.Spec.Containers),
			"total_ingress":    totalIngress,
			"total_egress":     totalEgress,
			"created_at":       nn.CreationTimestamp.Format(time.RFC3339),
		}
		neighborhoodList = append(neighborhoodList, nnMap)
	}

	result := map[string]interface{}{
		"network_neighborhoods": neighborhoodList,
		"total_count":           len(neighborhoodList),
		"description": "NetworkNeighborhoods capture actual network communication patterns of workloads. " +
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
	containers := []map[string]interface{}{}
	for _, c := range nn.Spec.Containers {
		// Format ingress connections
		ingressList := []map[string]interface{}{}
		for _, ing := range c.Ingress {
			ingressInfo := map[string]interface{}{
				"identifier": ing.Identifier,
				"type":       ing.Type,
			}
			if ing.DNS != "" {
				ingressInfo["dns"] = ing.DNS
			}
			if len(ing.Ports) > 0 {
				ingressInfo["ports"] = ing.Ports
			}
			if len(ing.IPAddress) > 0 {
				ingressInfo["ip_address"] = ing.IPAddress
			}
			if ing.PodSelector != nil {
				ingressInfo["pod_selector"] = ing.PodSelector
			}
			if ing.NamespaceSelector != nil {
				ingressInfo["namespace_selector"] = ing.NamespaceSelector
			}
			ingressList = append(ingressList, ingressInfo)
		}

		// Format egress connections
		egressList := []map[string]interface{}{}
		for _, egr := range c.Egress {
			egressInfo := map[string]interface{}{
				"identifier": egr.Identifier,
				"type":       egr.Type,
			}
			if egr.DNS != "" {
				egressInfo["dns"] = egr.DNS
			}
			if len(egr.Ports) > 0 {
				egressInfo["ports"] = egr.Ports
			}
			if len(egr.IPAddress) > 0 {
				egressInfo["ip_address"] = egr.IPAddress
			}
			if egr.PodSelector != nil {
				egressInfo["pod_selector"] = egr.PodSelector
			}
			if egr.NamespaceSelector != nil {
				egressInfo["namespace_selector"] = egr.NamespaceSelector
			}
			egressList = append(egressList, egressInfo)
		}

		containerInfo := map[string]interface{}{
			"name":    c.Name,
			"ingress": ingressList,
			"egress":  egressList,
		}
		containers = append(containers, containerInfo)
	}

	result := map[string]interface{}{
		"namespace":   namespace,
		"name":        name,
		"containers":  containers,
		"annotations": nn.Annotations,
		"labels":      nn.Labels,
		"description": "This NetworkNeighborhood shows actual network connections observed for this workload. " +
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
