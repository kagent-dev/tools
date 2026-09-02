package kubescape

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/kagent-dev/tools/internal/errors"
	"github.com/kagent-dev/tools/internal/telemetry"
	helpersv1 "github.com/kubescape/k8s-interface/instanceidhandler/v1/helpers"
	"github.com/kubescape/storage/pkg/apis/softwarecomposition/v1beta1"
	spdxv1beta1 "github.com/kubescape/storage/pkg/generated/clientset/versioned/typed/softwarecomposition/v1beta1"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
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

	// defaultVulnerabilityLimit bounds kubescape_list_vulnerabilities. A single
	// image measured 466 CVEs / 200 KB unbounded, which no agent can consume;
	// the severity summary carries the aggregate answer instead.
	defaultVulnerabilityLimit = 20

	// defaultWorkloadLimit bounds kubescape_list_vulnerable_workloads. Each
	// workload summary renders small, but a large cluster has many of them.
	defaultWorkloadLimit = 20

	// fixStateFixed is the Grype fix state meaning an upgrade is available.
	fixStateFixed = "fixed"

	severityUnknown = "Unknown"
)

// severityOrder lists Grype severities worst first. It is both the ranking used
// to sort results and the set of buckets the severity summary reports, so a
// severity can never be silently dropped into the wrong bucket.
var severityOrder = []string{"Critical", "High", "Medium", "Low", "Negligible", severityUnknown}

// normaliseSeverity maps a Grype severity onto a known bucket, case-insensitively.
// Anything unrecognised becomes Unknown -- but Negligible is a real severity and
// must not land there: 102 Negligible CVEs were previously counted as Unknown.
func normaliseSeverity(s string) string {
	for _, known := range severityOrder {
		if strings.EqualFold(s, known) {
			return known
		}
	}
	return severityUnknown
}

func isKnownSeverity(s string) bool {
	for _, known := range severityOrder {
		if strings.EqualFold(s, known) {
			return true
		}
	}
	return false
}

// severityRank returns the sort position of a severity, worst first.
func severityRank(s string) int {
	for i, known := range severityOrder {
		if s == known {
			return i
		}
	}
	return len(severityOrder)
}

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

// storageFullSpec is the sentinel the Kubescape storage API server requires in
// ListOptions.ResourceVersion to return object specs on LIST.
//
// By default the server strips spec from EVERY list response, so severity
// counters come back as zero and vulnerabilitiesRef comes back empty. That is
// not an error and is indistinguishable from a healthy cluster with no
// vulnerabilities -- listing the summary resources without this sentinel
// reports a vulnerable cluster as clean. Measured on storage v0.0.298:
// nginx:1.14.0 has 76 critical CVEs and a default LIST reports 0.
//
// Defined as ResourceVersionFullSpec in
// github.com/kubescape/storage/pkg/apis/softwarecomposition/register.go.
const storageFullSpec = "fullSpec"

// fullSpecList returns the ListOptions required to read specs from the storage
// API. Use it for every list of an aggregate resource.
func fullSpecList() metav1.ListOptions {
	return metav1.ListOptions{ResourceVersion: storageFullSpec}
}

// severityCounts renders a SeveritySummary for output.
//
// `relevant` is `json:"relevant,omitempty"` upstream, so a zero is
// indistinguishable from "relevancy was never computed" -- node-agent needs a
// learning period before it reports anything. Emitting a zero there would tell
// an agent that no vulnerability is runtime-reachable when the truth may be that
// nobody has looked yet, so the key is omitted unless it holds a real value and
// the tool description spells out that its absence is ambiguous.
//
// There is deliberately no derived "relevancy: available|unavailable" field.
// The obvious signal for one does not work: workloads carrying
// kubescape.io/status=ready were measured with `relevant` absent, so the
// annotation says nothing about whether relevancy was computed. Reporting a
// confident availability verdict from it would be a guess dressed as a fact.
func severityCounts(s v1beta1.SeveritySummary) map[string]map[string]int64 {
	out := map[string]map[string]int64{}
	for name, c := range map[string]v1beta1.VulnerabilityCounters{
		"Critical":      s.Critical,
		"High":          s.High,
		"Medium":        s.Medium,
		"Low":           s.Low,
		"Negligible":    s.Negligible,
		severityUnknown: s.Unknown,
	} {
		entry := map[string]int64{"all": c.All}
		if c.Relevant > 0 {
			entry["relevant"] = c.Relevant
		}
		out[name] = entry
	}
	return out
}

// addCounters accumulates src into dst, preserving the omit-when-absent rule.
func addCounters(dst map[string]map[string]int64, src v1beta1.SeveritySummary) {
	for name, entry := range severityCounts(src) {
		if dst[name] == nil {
			dst[name] = map[string]int64{"all": 0}
		}
		dst[name]["all"] += entry["all"]
		if rel, ok := entry["relevant"]; ok {
			dst[name]["relevant"] += rel
		}
	}
}

func totalAll(counts map[string]map[string]int64) int64 {
	var t int64
	for _, c := range counts {
		t += c["all"]
	}
	return t
}

// handleVulnerabilityOverview reports cluster or namespace vulnerability posture
// from the server-side aggregated summaries.
func (k *KubescapeTool) handleVulnerabilityOverview(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	if k.initError != nil {
		toolErr := errors.NewKubescapeError("vulnerability_overview", k.initError)
		return toolErr.ToMCPResult(), nil
	}

	namespace := mcp.ParseString(request, "namespace", "")

	// VulnerabilitySummary is cluster-scoped and each object's NAME is a
	// namespace, so listing returns one aggregate per namespace.
	summaries, err := k.spdxClient.VulnerabilitySummaries(metav1.NamespaceAll).List(ctx, fullSpecList())
	if err != nil {
		toolErr := errors.NewKubescapeError("vulnerability_overview", err).
			WithContext("namespace", namespace)
		return toolErr.ToMCPResult(), nil
	}

	clusterTotals := map[string]map[string]int64{}
	namespaces := []map[string]interface{}{}
	for _, summary := range summaries.Items {
		if namespace != "" && summary.Name != namespace {
			continue
		}
		addCounters(clusterTotals, summary.Spec.Severities)
		namespaces = append(namespaces, map[string]interface{}{
			"namespace":      summary.Name,
			"workload_count": len(summary.Spec.WorkloadVulnerabilitiesObj),
			"severities":     severityCounts(summary.Spec.Severities),
		})
	}

	// Worst namespace first, so the agent's next call is obvious.
	sort.SliceStable(namespaces, func(i, j int) bool {
		si := namespaces[i]["severities"].(map[string]map[string]int64)
		sj := namespaces[j]["severities"].(map[string]map[string]int64)
		if si["Critical"]["all"] != sj["Critical"]["all"] {
			return si["Critical"]["all"] > sj["Critical"]["all"]
		}
		return totalAll(si) > totalAll(sj)
	})

	scope := "cluster"
	if namespace != "" {
		scope = "namespace"
	}

	result := map[string]interface{}{
		"scope":          scope,
		"cluster_totals": clusterTotals,
		"namespaces":     namespaces,
		"next_step":      "call kubescape_list_vulnerable_workloads with a namespace to rank its workloads",
	}

	// A cluster with summaries but no counts anywhere is far more likely to mean
	// the spec was stripped than that every image is clean. Never present that
	// as good news.
	if len(namespaces) > 0 && totalAll(clusterTotals) == 0 {
		result["warning"] = "Every namespace reported zero vulnerabilities, which could not be confirmed as a genuinely clean cluster: " +
			"the Kubescape storage API returns zeroed counts when it strips object specs from list responses. " +
			"Verify with kubescape_list_vulnerabilities on a specific manifest before concluding there are no vulnerabilities."
	}

	content, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to marshal result: %v", err)), nil
	}
	return mcp.NewToolResultText(string(content)), nil
}

// handleListVulnerableWorkloads ranks workloads by vulnerability severity and
// hands back the manifest name needed to drill into each one.
func (k *KubescapeTool) handleListVulnerableWorkloads(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	if k.initError != nil {
		toolErr := errors.NewKubescapeError("list_vulnerable_workloads", k.initError)
		return toolErr.ToMCPResult(), nil
	}

	namespace := mcp.ParseString(request, "namespace", "")
	// Where the VulnerabilityManifest objects live, which is not what the
	// summaries' vulnerabilitiesRef reports -- see the comment at its use below.
	manifestNamespace := mcp.ParseString(request, "kubescape_namespace", defaultKubescapeNamespace)
	limit := int(mcp.ParseFloat64(request, "limit", defaultWorkloadLimit))
	if limit < 0 {
		limit = 0
	}

	queryNamespace := metav1.NamespaceAll
	if namespace != "" {
		queryNamespace = namespace
	}

	summaries, err := k.spdxClient.VulnerabilityManifestSummaries(queryNamespace).List(ctx, fullSpecList())
	if err != nil {
		toolErr := errors.NewKubescapeError("list_vulnerable_workloads", err).
			WithContext("namespace", namespace)
		return toolErr.ToMCPResult(), nil
	}

	workloads := []map[string]interface{}{}
	for _, summary := range summaries.Items {
		counts := severityCounts(summary.Spec.Severities)

		entry := map[string]interface{}{
			"namespace":        summary.Namespace,
			"workload_summary": summary.Name,
			"severities":       counts,
			"scan_status":      summary.Annotations["kubescape.io/status"],
		}
		if kind, name := summary.Labels["kubescape.io/workload-kind"], summary.Labels["kubescape.io/workload-name"]; kind != "" && name != "" {
			entry["workload"] = kind + "/" + name
		}
		if container := summary.Labels["kubescape.io/workload-container-name"]; container != "" {
			entry["container"] = container
		}
		if image := summary.Annotations["kubescape.io/image-tag"]; image != "" {
			entry["image"] = image
		}
		// vulnerabilitiesRef points straight at the manifests holding the CVEs:
		// `all` is the image-level manifest, `relevant` the workload-filtered
		// one. These names are exactly what kubescape_list_vulnerabilities takes.
		//
		// Only the NAME from the ref is usable. The server fills the ref's
		// namespace with the workload's namespace, but the manifests live in the
		// Kubescape namespace -- verified on a live cluster, where a GET of
		// docker.io-library-nginx-1.14.0-e34030 in the referenced namespace
		// returns NotFound while the same GET in `kubescape` succeeds. Passing
		// the ref's namespace on would hand the agent a pointer that 404s.
		if ref := summary.Spec.Vulnerabilities.ImageVulnerabilitiesObj; ref.Name != "" {
			entry["manifest_name"] = ref.Name
			entry["manifest_namespace"] = manifestNamespace
		}
		if ref := summary.Spec.Vulnerabilities.WorkloadVulnerabilitiesObj; ref.Name != "" {
			entry["workload_manifest_name"] = ref.Name
		}
		workloads = append(workloads, entry)
	}

	sort.SliceStable(workloads, func(i, j int) bool {
		si := workloads[i]["severities"].(map[string]map[string]int64)
		sj := workloads[j]["severities"].(map[string]map[string]int64)
		if si["Critical"]["all"] != sj["Critical"]["all"] {
			return si["Critical"]["all"] > sj["Critical"]["all"]
		}
		return totalAll(si) > totalAll(sj)
	})

	total := len(workloads)
	truncated := false
	if limit > 0 && total > limit {
		workloads = workloads[:limit]
		truncated = true
	}

	result := map[string]interface{}{
		"namespace":       namespace,
		"total_workloads": total,
		"returned":        len(workloads),
		"truncated":       truncated,
		"workloads":       workloads,
		"next_step":       "call kubescape_list_vulnerabilities with a workload's manifest_name to see its CVEs",
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
	vulnerabilityManifests := []map[string]interface{}{}
	for _, manifest := range manifests.Items {
		isImageLevel := manifest.Annotations[helpersv1.WlidMetadataKey] == ""
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

	severityFilter := mcp.ParseString(request, "severity", "")
	if severityFilter != "" {
		if !isKnownSeverity(severityFilter) {
			toolErr := errors.NewKubescapeError("list_vulnerabilities",
				fmt.Errorf("invalid severity %q: must be one of %v", severityFilter, severityOrder))
			return toolErr.ToMCPResult(), nil
		}
		severityFilter = normaliseSeverity(severityFilter)
	}
	fixableOnly := mcp.ParseBoolean(request, "fixable_only", false)
	limit := int(mcp.ParseFloat64(request, "limit", defaultVulnerabilityLimit))
	if limit < 0 {
		limit = 0
	}

	manifest, err := k.spdxClient.VulnerabilityManifests(namespace).Get(ctx, manifestName, metav1.GetOptions{})
	if err != nil {
		toolErr := errors.NewKubescapeError("get_vulnerability_manifest", err).
			WithContext("namespace", namespace).
			WithContext("manifest_name", manifestName)
		return toolErr.ToMCPResult(), nil
	}

	// Count every match by severity first. The summary describes the WHOLE
	// manifest even when the returned array is filtered or truncated, so a
	// bounded response still carries the aggregate answer.
	severityCounts := map[string]int{}
	for _, name := range severityOrder {
		severityCounts[name] = 0
	}

	// Grype reports one Match per affected package, so the same CVE recurs.
	// Collapse them by ID -- without the package fields the rows would be
	// indistinguishable and duplicates would consume the limit budget -- while
	// keeping the number of affected artifacts visible.
	matched := []map[string]interface{}{}
	byID := map[string]map[string]interface{}{}
	for _, match := range manifest.Spec.Payload.Matches {
		vuln := match.Vulnerability
		severity := normaliseSeverity(string(vuln.Severity))
		severityCounts[severity]++

		if severityFilter != "" && severity != severityFilter {
			continue
		}
		if fixableOnly && vuln.Fix.State != fixStateFixed {
			continue
		}

		if existing, seen := byID[vuln.ID]; seen {
			existing["affected_artifacts"] = existing["affected_artifacts"].(int) + 1
			// A CVE unfixed in any package is not actionable as "fixed".
			if vuln.Fix.State != fixStateFixed {
				existing["fix_state"] = vuln.Fix.State
			}
			continue
		}

		// Only the fields an agent needs to decide what to look at next.
		// description (56% of the old payload) is truncated mid-word and is
		// available in full from kubescape_get_vulnerability_details, which is
		// also where data_source and fix_versions live.
		entry := map[string]interface{}{
			"id":                 vuln.ID,
			"severity":           severity,
			"fix_state":          vuln.Fix.State,
			"affected_artifacts": 1,
		}
		byID[vuln.ID] = entry
		matched = append(matched, entry)
	}

	// Worst first, so a truncated array is the half that matters. Ties break on
	// id to keep responses stable between calls.
	sort.SliceStable(matched, func(i, j int) bool {
		ri, rj := severityRank(matched[i]["severity"].(string)), severityRank(matched[j]["severity"].(string))
		if ri != rj {
			return ri < rj
		}
		return matched[i]["id"].(string) < matched[j]["id"].(string)
	})

	totalMatching := len(matched)
	truncated := false
	if limit > 0 && totalMatching > limit {
		matched = matched[:limit]
		truncated = true
	}

	result := map[string]interface{}{
		"manifest_name":    manifestName,
		"namespace":        namespace,
		"severity_summary": severityCounts,
		"total_count":      totalMatching,
		"returned_count":   len(matched),
		"truncated":        truncated,
		"filters": map[string]interface{}{
			"severity":     severityFilter,
			"fixable_only": fixableOnly,
		},
		"vulnerabilities": matched,
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

	// Cluster / namespace vulnerability posture -- the cheapest entry point
	s.AddTool(mcp.NewTool("kubescape_vulnerability_overview",
		mcp.WithDescription("START HERE for any question about cluster-wide or namespace-wide vulnerabilities. "+
			"Returns severity totals per namespace from Kubescape's server-side aggregates in a single cheap call, worst namespace first. "+
			"Counts are 'all' plus 'relevant' (the vulnerable code was observed loaded at runtime). "+
			"'relevant' is reported only when greater than zero; when it is absent that means EITHER no runtime-relevant CVEs OR that relevancy "+
			"has not been computed for those workloads yet, and the two cannot be distinguished here -- so do not report an absent 'relevant' as a measured zero. "+
			"Then narrow with kubescape_list_vulnerable_workloads."),
		mcp.WithString("namespace", mcp.Description("Restrict to one namespace (optional; omit for the whole cluster)")),
	), telemetry.AdaptToolHandler(telemetry.WithTracing("kubescape_vulnerability_overview", tool.handleVulnerabilityOverview)))

	// Rank workloads and hand back the manifest to drill into
	s.AddTool(mcp.NewTool("kubescape_list_vulnerable_workloads",
		mcp.WithDescription("Rank workloads by vulnerability severity, worst first, and return the 'manifest_name' needed to inspect each one's CVEs. "+
			"Use after kubescape_vulnerability_overview to find which workloads matter, then pass a returned manifest_name to kubescape_list_vulnerabilities."),
		mcp.WithString("namespace", mcp.Description("Restrict to one namespace (optional, defaults to all namespaces)")),
		mcp.WithNumber("limit", mcp.Description("Maximum workloads to return (default: 20). Use 0 for no limit.")),
		mcp.WithString("kubescape_namespace", mcp.Description("Namespace the Kubescape operator is installed in, where vulnerability manifests are stored (default: kubescape)")),
	), telemetry.AdaptToolHandler(telemetry.WithTracing("kubescape_list_vulnerable_workloads", tool.handleListVulnerableWorkloads)))

	// List vulnerability manifests
	s.AddTool(mcp.NewTool("kubescape_list_vulnerability_manifests",
		mcp.WithDescription("List vulnerability manifests from Kubescape operator. Returns vulnerability scan results at image or workload level."),
		mcp.WithString("namespace", mcp.Description("Filter by namespace (optional, defaults to all namespaces)")),
		mcp.WithString("level", mcp.Description("Type of manifests to list: 'image', 'workload', or 'both' (default: both)")),
	), telemetry.AdaptToolHandler(telemetry.WithTracing("kubescape_list_vulnerability_manifests", tool.handleListVulnerabilityManifests)))

	// List vulnerabilities in a manifest
	s.AddTool(mcp.NewTool("kubescape_list_vulnerabilities",
		mcp.WithDescription("List CVEs in a specific vulnerability manifest. Always returns severity_summary, which counts EVERY CVE in the manifest. "+
			"The 'vulnerabilities' array is capped at 'limit' (default 20), ordered worst severity first; check 'truncated' and 'total_count' to see whether more exist. "+
			"Each entry carries only id, severity and fix_state -- use kubescape_get_vulnerability_details for the description, data source and fix versions of one CVE."),
		mcp.WithString("namespace", mcp.Description("Namespace of the manifest (default: kubescape)")),
		mcp.WithString("manifest_name", mcp.Description("Name of the vulnerability manifest"), mcp.Required()),
		mcp.WithString("severity", mcp.Description("Only return CVEs of this severity: 'Critical', 'High', 'Medium', 'Low', 'Negligible' or 'Unknown'. severity_summary still covers the whole manifest.")),
		mcp.WithBoolean("fixable_only", mcp.Description("Only return CVEs that have a fix available (default: false)")),
		mcp.WithNumber("limit", mcp.Description("Maximum CVEs to return (default: 20). Use 0 for no limit -- a single image can hold hundreds of CVEs and overflow the context window.")),
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
	HandleVulnerabilityOverview(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error)
	HandleListVulnerableWorkloads(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error)
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

func (k *KubescapeTool) HandleVulnerabilityOverview(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return k.handleVulnerabilityOverview(ctx, request)
}

func (k *KubescapeTool) HandleListVulnerableWorkloads(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return k.handleListVulnerableWorkloads(ctx, request)
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
