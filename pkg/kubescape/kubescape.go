package kubescape

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/kagent-dev/tools/internal/errors"
	mcp "github.com/kagent-dev/tools/internal/mcp"
	helpersv1 "github.com/kubescape/k8s-interface/instanceidhandler/v1/helpers"
	"github.com/kubescape/storage/pkg/apis/softwarecomposition/v1beta1"
	spdxv1beta1 "github.com/kubescape/storage/pkg/generated/clientset/versioned/typed/softwarecomposition/v1beta1"
	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
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

// limitOrDefault resolves an optional limit argument: absent means def, and
// zero or a negative value means no limit.
func limitOrDefault(limit *int, def int) int {
	if limit == nil {
		return def
	}
	if *limit < 0 {
		return 0
	}
	return *limit
}

// kubescapeErrResult adapts ToolError to an MCP error result.
func kubescapeErrResult(toolErr *errors.ToolError) *sdkmcp.CallToolResult {
	return toolErr.ToMCPResult()
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
	Checks          map[string]CheckStatus `json:"checks,omitempty"`
	Summary         string                 `json:"summary"`
	Recommendations []string               `json:"recommendations,omitempty"`
}

// CheckStatus represents the status of a single check
type CheckStatus struct {
	Status  string          `json:"status"`
	Message string          `json:"message"`
	Details []PodCheckEntry `json:"details,omitempty"`
}

// PodCheckEntry reports the name and phase of a pod inspected by a health check.
type PodCheckEntry struct {
	Name   string `json:"name"`
	Status string `json:"status"`
}

type checkHealthInput struct {
	Namespace string `json:"namespace" jsonschema:"Namespace to check (default: kubescape)"`
}

type listVulnerabilityManifestsInput struct {
	Namespace string `json:"namespace" jsonschema:"Filter by namespace (optional, defaults to all namespaces)"`
	Level     string `json:"level" jsonschema:"Type of manifests to list: 'image', 'workload', or 'both' (default: both)"`
}

type vulnerabilityOverviewInput struct {
	Namespace string `json:"namespace" jsonschema:"Restrict to one namespace (optional; omit for the whole cluster)"`
}

type listVulnerableWorkloadsInput struct {
	Namespace          string `json:"namespace" jsonschema:"Restrict to one namespace (optional, defaults to all namespaces)"`
	Limit              *int   `json:"limit" jsonschema:"Maximum workloads to return (default: 20). Use 0 for no limit."`
	KubescapeNamespace string `json:"kubescape_namespace" jsonschema:"Namespace the Kubescape operator is installed in, where vulnerability manifests are stored (default: kubescape)"`
}

type listVulnerabilitiesInManifestInput struct {
	Namespace    string `json:"namespace" jsonschema:"Namespace of the manifest (default: kubescape)"`
	ManifestName string `json:"manifest_name" jsonschema:"Name of the vulnerability manifest"`
	Severity     string `json:"severity" jsonschema:"Only return CVEs of this severity: 'Critical', 'High', 'Medium', 'Low', 'Negligible' or 'Unknown'. severity_summary still covers the whole manifest."`
	FixableOnly  bool   `json:"fixable_only" jsonschema:"Only return CVEs that have a fix available (default: false)"`
	Limit        *int   `json:"limit" jsonschema:"Maximum CVEs to return (default: 20). Use 0 for no limit -- a single image can hold hundreds of CVEs and overflow the context window."`
}

type getVulnerabilityDetailsInput struct {
	Namespace    string `json:"namespace" jsonschema:"Namespace of the manifest (default: kubescape)"`
	ManifestName string `json:"manifest_name" jsonschema:"Name of the vulnerability manifest"`
	CveID        string `json:"cve_id" jsonschema:"CVE identifier (e.g., CVE-2023-12345)"`
}

type listConfigurationScansInput struct {
	Namespace string `json:"namespace" jsonschema:"Filter by namespace (optional, defaults to all namespaces)"`
}

type getConfigurationScanInput struct {
	Namespace    string `json:"namespace" jsonschema:"Namespace of the scan (default: kubescape)"`
	ManifestName string `json:"manifest_name" jsonschema:"Name of the configuration scan manifest"`
}

type listApplicationProfilesInput struct {
	Namespace string `json:"namespace" jsonschema:"Filter by namespace (optional, defaults to all namespaces)"`
}

type getApplicationProfileInput struct {
	Namespace string `json:"namespace" jsonschema:"Namespace of the profile"`
	Name      string `json:"name" jsonschema:"Name of the application profile"`
}

type listNetworkNeighborhoodsInput struct {
	Namespace string `json:"namespace" jsonschema:"Filter by namespace (optional, defaults to all namespaces)"`
}

type getNetworkNeighborhoodInput struct {
	Namespace string `json:"namespace" jsonschema:"Namespace of the network neighborhood"`
	Name      string `json:"name" jsonschema:"Name of the network neighborhood"`
}

// Typed response shapes for the read-only scan/report tools. Each is returned
// directly as the handler's typed Out value, so the SDK derives an output schema
// and populates CallToolResult.StructuredContent from a concrete Go type.

type vulnerabilityManifestSummary struct {
	Namespace             string `json:"namespace"`
	ManifestName          string `json:"manifest_name"`
	ImageLevel            bool   `json:"image_level"`
	WorkloadLevel         bool   `json:"workload_level"`
	ImageID               string `json:"image_id"`
	ImageTag              string `json:"image_tag"`
	WorkloadID            string `json:"workload_id"`
	WorkloadContainerName string `json:"workload_container_name"`
}

type listVulnerabilityManifestsOutput struct {
	VulnerabilityManifests []vulnerabilityManifestSummary `json:"vulnerability_manifests"`
	TotalCount             int                            `json:"total_count"`
}

type severitySummary struct {
	Critical   int `json:"Critical"`
	High       int `json:"High"`
	Medium     int `json:"Medium"`
	Low        int `json:"Low"`
	Negligible int `json:"Negligible"`
	Unknown    int `json:"Unknown"`
}

// bucket returns the counter for a severity already passed through
// normaliseSeverity. Every entry of severityOrder has its own bucket.
func (s *severitySummary) bucket(severity string) *int {
	switch severity {
	case "Critical":
		return &s.Critical
	case "High":
		return &s.High
	case "Medium":
		return &s.Medium
	case "Low":
		return &s.Low
	case "Negligible":
		return &s.Negligible
	default:
		return &s.Unknown
	}
}

// vulnerabilitySummary carries only the fields an agent needs to decide what to
// look at next. description (56% of the old payload) was truncated mid-word and
// is available in full from kubescape_get_vulnerability_details, which is also
// where data_source and fix_versions live.
type vulnerabilitySummary struct {
	ID                string `json:"id"`
	Severity          string `json:"severity"`
	FixState          string `json:"fix_state"`
	AffectedArtifacts int    `json:"affected_artifacts"`
}

type vulnerabilityFilters struct {
	Severity    string `json:"severity"`
	FixableOnly bool   `json:"fixable_only"`
}

type listVulnerabilitiesInManifestOutput struct {
	ManifestName    string                 `json:"manifest_name"`
	Namespace       string                 `json:"namespace"`
	SeveritySummary severitySummary        `json:"severity_summary"`
	TotalCount      int                    `json:"total_count"`
	ReturnedCount   int                    `json:"returned_count"`
	Truncated       bool                   `json:"truncated"`
	Filters         vulnerabilityFilters   `json:"filters"`
	Vulnerabilities []vulnerabilitySummary `json:"vulnerabilities"`
}

// severityCount is one severity's count from a server-side summary: every CVE,
// plus those whose code node-agent observed loaded at runtime.
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
type severityCount struct {
	All      int64 `json:"all"`
	Relevant int64 `json:"relevant,omitempty"`
}

// severityCounts renders a server-side SeveritySummary for output.
type severityCounts struct {
	Critical   severityCount `json:"Critical"`
	High       severityCount `json:"High"`
	Medium     severityCount `json:"Medium"`
	Low        severityCount `json:"Low"`
	Negligible severityCount `json:"Negligible"`
	Unknown    severityCount `json:"Unknown"`
}

func newSeverityCounts(s v1beta1.SeveritySummary) severityCounts {
	c := func(v v1beta1.VulnerabilityCounters) severityCount {
		return severityCount{All: v.All, Relevant: v.Relevant}
	}
	return severityCounts{
		Critical:   c(s.Critical),
		High:       c(s.High),
		Medium:     c(s.Medium),
		Low:        c(s.Low),
		Negligible: c(s.Negligible),
		Unknown:    c(s.Unknown),
	}
}

func (c *severityCounts) buckets() []*severityCount {
	return []*severityCount{&c.Critical, &c.High, &c.Medium, &c.Low, &c.Negligible, &c.Unknown}
}

// add accumulates other into c. A relevant count that is absent everywhere
// stays absent in the total.
func (c *severityCounts) add(other severityCounts) {
	src := other.buckets()
	for i, dst := range c.buckets() {
		dst.All += src[i].All
		dst.Relevant += src[i].Relevant
	}
}

func (c severityCounts) total() int64 {
	var t int64
	for _, b := range c.buckets() {
		t += b.All
	}
	return t
}

// worseThan orders by critical count, then by total, worst first.
func (c severityCounts) worseThan(other severityCounts) bool {
	if c.Critical.All != other.Critical.All {
		return c.Critical.All > other.Critical.All
	}
	return c.total() > other.total()
}

type namespaceVulnerabilities struct {
	Namespace     string         `json:"namespace"`
	WorkloadCount int            `json:"workload_count"`
	Severities    severityCounts `json:"severities"`
}

type vulnerabilityOverviewOutput struct {
	Scope         string                     `json:"scope"`
	ClusterTotals severityCounts             `json:"cluster_totals"`
	Namespaces    []namespaceVulnerabilities `json:"namespaces"`
	NextStep      string                     `json:"next_step"`
	Warning       string                     `json:"warning,omitempty"`
}

type vulnerableWorkload struct {
	Namespace            string         `json:"namespace"`
	WorkloadSummary      string         `json:"workload_summary"`
	Workload             string         `json:"workload,omitempty"`
	Container            string         `json:"container,omitempty"`
	Image                string         `json:"image,omitempty"`
	Severities           severityCounts `json:"severities"`
	ScanStatus           string         `json:"scan_status"`
	ManifestName         string         `json:"manifest_name,omitempty"`
	ManifestNamespace    string         `json:"manifest_namespace,omitempty"`
	WorkloadManifestName string         `json:"workload_manifest_name,omitempty"`
}

type listVulnerableWorkloadsOutput struct {
	Namespace      string               `json:"namespace"`
	TotalWorkloads int                  `json:"total_workloads"`
	Returned       int                  `json:"returned"`
	Truncated      bool                 `json:"truncated"`
	Workloads      []vulnerableWorkload `json:"workloads"`
	NextStep       string               `json:"next_step"`
}

type configurationScanSummary struct {
	Namespace    string `json:"namespace"`
	ManifestName string `json:"manifest_name"`
	CreatedAt    string `json:"created_at"`
}

type listConfigurationScansOutput struct {
	ConfigurationScans []configurationScanSummary `json:"configuration_scans"`
	TotalCount         int                        `json:"total_count"`
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

type listApplicationProfilesOutput struct {
	ApplicationProfiles []applicationProfileSummary `json:"application_profiles"`
	TotalCount          int                         `json:"total_count"`
	Description         string                      `json:"description"`
}

type containerBehavior struct {
	Name           string                        `json:"name"`
	Execs          []v1beta1.ExecCalls           `json:"execs"`
	Opens          []v1beta1.OpenCalls           `json:"opens"`
	Syscalls       []string                      `json:"syscalls"`
	Capabilities   []string                      `json:"capabilities"`
	Endpoints      []v1beta1.HTTPEndpoint        `json:"endpoints"`
	SeccompProfile *v1beta1.SingleSeccompProfile `json:"seccomp_profile,omitempty"`
}

type getApplicationProfileOutput struct {
	Namespace      string              `json:"namespace"`
	Name           string              `json:"name"`
	Containers     []containerBehavior `json:"containers"`
	InitContainers []containerBehavior `json:"init_containers"`
	Annotations    map[string]string   `json:"annotations,omitempty"`
	Labels         map[string]string   `json:"labels,omitempty"`
	Description    string              `json:"description"`
}

type networkNeighborhoodSummary struct {
	Namespace       string `json:"namespace"`
	Name            string `json:"name"`
	ContainersCount int    `json:"containers_count"`
	TotalIngress    int    `json:"total_ingress"`
	TotalEgress     int    `json:"total_egress"`
	CreatedAt       string `json:"created_at"`
}

type listNetworkNeighborhoodsOutput struct {
	NetworkNeighborhoods []networkNeighborhoodSummary `json:"network_neighborhoods"`
	TotalCount           int                          `json:"total_count"`
	Description          string                       `json:"description"`
}

type networkConnection struct {
	Identifier        string                    `json:"identifier"`
	Type              v1beta1.CommunicationType `json:"type"`
	DNS               string                    `json:"dns,omitempty"`
	Ports             []v1beta1.NetworkPort     `json:"ports,omitempty"`
	IPAddress         string                    `json:"ip_address,omitempty"`
	PodSelector       *metav1.LabelSelector     `json:"pod_selector,omitempty"`
	NamespaceSelector *metav1.LabelSelector     `json:"namespace_selector,omitempty"`
}

type networkContainer struct {
	Name    string              `json:"name"`
	Ingress []networkConnection `json:"ingress"`
	Egress  []networkConnection `json:"egress"`
}

type getNetworkNeighborhoodOutput struct {
	Namespace   string             `json:"namespace"`
	Name        string             `json:"name"`
	Containers  []networkContainer `json:"containers"`
	Annotations map[string]string  `json:"annotations,omitempty"`
	Labels      map[string]string  `json:"labels,omitempty"`
	Description string             `json:"description"`
}

// handleCheckHealth verifies Kubescape operator installation and readiness
func (k *KubescapeTool) handleCheckHealth(ctx context.Context, request *sdkmcp.CallToolRequest, in checkHealthInput) (*sdkmcp.CallToolResult, HealthCheckResult, error) {
	if k.initError != nil {
		toolErr := errors.NewKubescapeError("check_health", k.initError)
		return kubescapeErrResult(toolErr), HealthCheckResult{}, nil
	}

	namespace := in.Namespace
	if namespace == "" {
		namespace = defaultKubescapeNamespace
	}

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
		podDetails := []PodCheckEntry{}
		for _, pod := range operatorPods.Items {
			status := string(pod.Status.Phase)
			if pod.Status.Phase == corev1.PodRunning {
				runningCount++
			}
			podDetails = append(podDetails, PodCheckEntry{
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
		return mcp.NewToolResultError(fmt.Sprintf("failed to marshal result: %v", err)), HealthCheckResult{}, nil
	}

	return mcp.NewToolResultText(string(content)), result, nil
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

// handleVulnerabilityOverview reports cluster or namespace vulnerability posture
// from the server-side aggregated summaries.
func (k *KubescapeTool) handleVulnerabilityOverview(ctx context.Context, request *sdkmcp.CallToolRequest, in vulnerabilityOverviewInput) (*sdkmcp.CallToolResult, vulnerabilityOverviewOutput, error) {
	if k.initError != nil {
		toolErr := errors.NewKubescapeError("vulnerability_overview", k.initError)
		return kubescapeErrResult(toolErr), vulnerabilityOverviewOutput{}, nil
	}

	namespace := in.Namespace

	// VulnerabilitySummary is cluster-scoped and each object's NAME is a
	// namespace, so listing returns one aggregate per namespace.
	summaries, err := k.spdxClient.VulnerabilitySummaries(metav1.NamespaceAll).List(ctx, fullSpecList())
	if err != nil {
		toolErr := errors.NewKubescapeError("vulnerability_overview", err).
			WithContext("namespace", namespace)
		return kubescapeErrResult(toolErr), vulnerabilityOverviewOutput{}, nil
	}

	clusterTotals := severityCounts{}
	namespaces := []namespaceVulnerabilities{}
	for _, summary := range summaries.Items {
		if namespace != "" && summary.Name != namespace {
			continue
		}
		counts := newSeverityCounts(summary.Spec.Severities)
		clusterTotals.add(counts)
		namespaces = append(namespaces, namespaceVulnerabilities{
			Namespace:     summary.Name,
			WorkloadCount: len(summary.Spec.WorkloadVulnerabilitiesObj),
			Severities:    counts,
		})
	}

	// Worst namespace first, so the agent's next call is obvious.
	sort.SliceStable(namespaces, func(i, j int) bool {
		return namespaces[i].Severities.worseThan(namespaces[j].Severities)
	})

	scope := "cluster"
	if namespace != "" {
		scope = "namespace"
	}

	result := vulnerabilityOverviewOutput{
		Scope:         scope,
		ClusterTotals: clusterTotals,
		Namespaces:    namespaces,
		NextStep:      "call kubescape_list_vulnerable_workloads with a namespace to rank its workloads",
	}

	// A cluster with summaries but no counts anywhere is far more likely to mean
	// the spec was stripped than that every image is clean. Never present that
	// as good news.
	if len(namespaces) > 0 && clusterTotals.total() == 0 {
		result.Warning = "Every namespace reported zero vulnerabilities, which could not be confirmed as a genuinely clean cluster: " +
			"the Kubescape storage API returns zeroed counts when it strips object specs from list responses. " +
			"Verify with kubescape_list_vulnerabilities on a specific manifest before concluding there are no vulnerabilities."
	}

	content, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to marshal result: %v", err)), vulnerabilityOverviewOutput{}, nil
	}
	return mcp.NewToolResultText(string(content)), result, nil
}

// handleListVulnerableWorkloads ranks workloads by vulnerability severity and
// hands back the manifest name needed to drill into each one.
func (k *KubescapeTool) handleListVulnerableWorkloads(ctx context.Context, request *sdkmcp.CallToolRequest, in listVulnerableWorkloadsInput) (*sdkmcp.CallToolResult, listVulnerableWorkloadsOutput, error) {
	if k.initError != nil {
		toolErr := errors.NewKubescapeError("list_vulnerable_workloads", k.initError)
		return kubescapeErrResult(toolErr), listVulnerableWorkloadsOutput{}, nil
	}

	namespace := in.Namespace
	// Where the VulnerabilityManifest objects live, which is not what the
	// summaries' vulnerabilitiesRef reports -- see the comment at its use below.
	manifestNamespace := in.KubescapeNamespace
	if manifestNamespace == "" {
		manifestNamespace = defaultKubescapeNamespace
	}
	limit := limitOrDefault(in.Limit, defaultWorkloadLimit)

	queryNamespace := metav1.NamespaceAll
	if namespace != "" {
		queryNamespace = namespace
	}

	summaries, err := k.spdxClient.VulnerabilityManifestSummaries(queryNamespace).List(ctx, fullSpecList())
	if err != nil {
		toolErr := errors.NewKubescapeError("list_vulnerable_workloads", err).
			WithContext("namespace", namespace)
		return kubescapeErrResult(toolErr), listVulnerableWorkloadsOutput{}, nil
	}

	workloads := []vulnerableWorkload{}
	for _, summary := range summaries.Items {
		entry := vulnerableWorkload{
			Namespace:       summary.Namespace,
			WorkloadSummary: summary.Name,
			Severities:      newSeverityCounts(summary.Spec.Severities),
			ScanStatus:      summary.Annotations["kubescape.io/status"],
			Container:       summary.Labels["kubescape.io/workload-container-name"],
			Image:           summary.Annotations["kubescape.io/image-tag"],
		}
		if kind, name := summary.Labels["kubescape.io/workload-kind"], summary.Labels["kubescape.io/workload-name"]; kind != "" && name != "" {
			entry.Workload = kind + "/" + name
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
			entry.ManifestName = ref.Name
			entry.ManifestNamespace = manifestNamespace
		}
		if ref := summary.Spec.Vulnerabilities.WorkloadVulnerabilitiesObj; ref.Name != "" {
			entry.WorkloadManifestName = ref.Name
		}
		workloads = append(workloads, entry)
	}

	sort.SliceStable(workloads, func(i, j int) bool {
		return workloads[i].Severities.worseThan(workloads[j].Severities)
	})

	total := len(workloads)
	truncated := false
	if limit > 0 && total > limit {
		workloads = workloads[:limit]
		truncated = true
	}

	result := listVulnerableWorkloadsOutput{
		Namespace:      namespace,
		TotalWorkloads: total,
		Returned:       len(workloads),
		Truncated:      truncated,
		Workloads:      workloads,
		NextStep:       "call kubescape_list_vulnerabilities with a workload's manifest_name to see its CVEs",
	}

	content, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to marshal result: %v", err)), listVulnerableWorkloadsOutput{}, nil
	}
	return mcp.NewToolResultText(string(content)), result, nil
}

// handleListVulnerabilityManifests lists vulnerability manifests at image and workload levels
func (k *KubescapeTool) handleListVulnerabilityManifests(ctx context.Context, request *sdkmcp.CallToolRequest, in listVulnerabilityManifestsInput) (*sdkmcp.CallToolResult, listVulnerabilityManifestsOutput, error) {
	if k.initError != nil {
		toolErr := errors.NewKubescapeError("list_vulnerability_manifests", k.initError)
		return kubescapeErrResult(toolErr), listVulnerabilityManifestsOutput{}, nil
	}

	namespace := in.Namespace
	level := in.Level
	if level == "" {
		level = "both"
	}

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
		return kubescapeErrResult(toolErr), listVulnerabilityManifestsOutput{}, nil
	}

	// Build response
	vulnerabilityManifests := []vulnerabilityManifestSummary{}
	// No vulnerability count is reported here. The aggregated API strips
	// spec.payload.matches on LIST and serves it only on GET, so the field is
	// nil on every listed object: counting it would report 0 for every image in
	// every cluster, and an agent reading that data would conclude the cluster
	// is clean. An absent field cannot be mistaken for a measured zero. Use
	// kubescape_list_vulnerabilities with a manifest_name for real counts.
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
		})
	}

	result := listVulnerabilityManifestsOutput{
		VulnerabilityManifests: vulnerabilityManifests,
		TotalCount:             len(vulnerabilityManifests),
	}

	content, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to marshal result: %v", err)), listVulnerabilityManifestsOutput{}, nil
	}

	return mcp.NewToolResultText(string(content)), result, nil
}

// handleListVulnerabilitiesInManifest lists all CVEs in a specific manifest
func (k *KubescapeTool) handleListVulnerabilitiesInManifest(ctx context.Context, request *sdkmcp.CallToolRequest, in listVulnerabilitiesInManifestInput) (*sdkmcp.CallToolResult, listVulnerabilitiesInManifestOutput, error) {
	if k.initError != nil {
		toolErr := errors.NewKubescapeError("list_vulnerabilities", k.initError)
		return kubescapeErrResult(toolErr), listVulnerabilitiesInManifestOutput{}, nil
	}

	namespace := in.Namespace
	if namespace == "" {
		namespace = defaultKubescapeNamespace
	}
	manifestName := in.ManifestName

	if manifestName == "" {
		return mcp.NewToolResultError("manifest_name parameter is required"), listVulnerabilitiesInManifestOutput{}, nil
	}

	severityFilter := in.Severity
	if severityFilter != "" {
		if !isKnownSeverity(severityFilter) {
			toolErr := errors.NewKubescapeError("list_vulnerabilities",
				fmt.Errorf("invalid severity %q: must be one of %v", severityFilter, severityOrder))
			return kubescapeErrResult(toolErr), listVulnerabilitiesInManifestOutput{}, nil
		}
		severityFilter = normaliseSeverity(severityFilter)
	}
	fixableOnly := in.FixableOnly
	limit := limitOrDefault(in.Limit, defaultVulnerabilityLimit)

	manifest, err := k.spdxClient.VulnerabilityManifests(namespace).Get(ctx, manifestName, metav1.GetOptions{})
	if err != nil {
		toolErr := errors.NewKubescapeError("get_vulnerability_manifest", err).
			WithContext("namespace", namespace).
			WithContext("manifest_name", manifestName)
		return kubescapeErrResult(toolErr), listVulnerabilitiesInManifestOutput{}, nil
	}

	// Count every match by severity first. The summary describes the WHOLE
	// manifest even when the returned array is filtered or truncated, so a
	// bounded response still carries the aggregate answer.
	severityCounts := severitySummary{}

	// Grype reports one Match per affected package, so the same CVE recurs.
	// Collapse them by ID -- without the package fields the rows would be
	// indistinguishable and duplicates would consume the limit budget -- while
	// keeping the number of affected artifacts visible.
	matched := []*vulnerabilitySummary{}
	byID := map[string]*vulnerabilitySummary{}
	for _, match := range manifest.Spec.Payload.Matches {
		vuln := match.Vulnerability
		severity := normaliseSeverity(string(vuln.Severity))
		*severityCounts.bucket(severity)++

		if severityFilter != "" && severity != severityFilter {
			continue
		}
		if fixableOnly && vuln.Fix.State != fixStateFixed {
			continue
		}

		if existing, seen := byID[vuln.ID]; seen {
			existing.AffectedArtifacts++
			// A CVE unfixed in any package is not actionable as "fixed".
			if vuln.Fix.State != fixStateFixed {
				existing.FixState = vuln.Fix.State
			}
			continue
		}

		entry := &vulnerabilitySummary{
			ID:                vuln.ID,
			Severity:          severity,
			FixState:          vuln.Fix.State,
			AffectedArtifacts: 1,
		}
		byID[vuln.ID] = entry
		matched = append(matched, entry)
	}

	// Worst first, so a truncated array is the half that matters. Ties break on
	// id to keep responses stable between calls.
	sort.SliceStable(matched, func(i, j int) bool {
		ri, rj := severityRank(matched[i].Severity), severityRank(matched[j].Severity)
		if ri != rj {
			return ri < rj
		}
		return matched[i].ID < matched[j].ID
	})

	totalMatching := len(matched)
	truncated := false
	if limit > 0 && totalMatching > limit {
		matched = matched[:limit]
		truncated = true
	}

	vulnerabilities := make([]vulnerabilitySummary, 0, len(matched))
	for _, v := range matched {
		vulnerabilities = append(vulnerabilities, *v)
	}

	result := listVulnerabilitiesInManifestOutput{
		ManifestName:    manifestName,
		Namespace:       namespace,
		SeveritySummary: severityCounts,
		TotalCount:      totalMatching,
		ReturnedCount:   len(vulnerabilities),
		Truncated:       truncated,
		Filters: vulnerabilityFilters{
			Severity:    severityFilter,
			FixableOnly: fixableOnly,
		},
		Vulnerabilities: vulnerabilities,
	}

	content, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to marshal result: %v", err)), listVulnerabilitiesInManifestOutput{}, nil
	}

	return mcp.NewToolResultText(string(content)), result, nil
}

// handleGetVulnerabilityDetails gets detailed info about a specific CVE in a manifest
func (k *KubescapeTool) handleGetVulnerabilityDetails(ctx context.Context, request *sdkmcp.CallToolRequest, in getVulnerabilityDetailsInput) (*sdkmcp.CallToolResult, []v1beta1.Match, error) {
	if k.initError != nil {
		toolErr := errors.NewKubescapeError("get_vulnerability_details", k.initError)
		return kubescapeErrResult(toolErr), nil, nil
	}

	namespace := in.Namespace
	if namespace == "" {
		namespace = defaultKubescapeNamespace
	}
	manifestName := in.ManifestName
	cveID := in.CveID

	if manifestName == "" {
		return mcp.NewToolResultError("manifest_name parameter is required"), nil, nil
	}
	if cveID == "" {
		return mcp.NewToolResultError("cve_id parameter is required"), nil, nil
	}

	manifest, err := k.spdxClient.VulnerabilityManifests(namespace).Get(ctx, manifestName, metav1.GetOptions{})
	if err != nil {
		toolErr := errors.NewKubescapeError("get_vulnerability_manifest", err).
			WithContext("namespace", namespace).
			WithContext("manifest_name", manifestName)
		return kubescapeErrResult(toolErr), nil, nil
	}

	// Find matching CVE entries
	var matches []v1beta1.Match
	for _, m := range manifest.Spec.Payload.Matches {
		if m.Vulnerability.ID == cveID {
			matches = append(matches, m)
		}
	}

	if len(matches) == 0 {
		return mcp.NewToolResultError(fmt.Sprintf("CVE %s not found in manifest %s", cveID, manifestName)), nil, nil
	}

	// Return the matches as the typed Out and leave Content unset. For an array
	// (non-object) output the SDK serialises the value into a TextContent block
	// itself; setting Content here as well would emit the same list twice for
	// clients that read Content.
	return nil, matches, nil
}

// handleListConfigurationScans lists configuration security scan results
func (k *KubescapeTool) handleListConfigurationScans(ctx context.Context, request *sdkmcp.CallToolRequest, in listConfigurationScansInput) (*sdkmcp.CallToolResult, listConfigurationScansOutput, error) {
	if k.initError != nil {
		toolErr := errors.NewKubescapeError("list_configuration_scans", k.initError)
		return kubescapeErrResult(toolErr), listConfigurationScansOutput{}, nil
	}

	namespace := in.Namespace

	queryNamespace := metav1.NamespaceAll
	if namespace != "" {
		queryNamespace = namespace
	}

	manifests, err := k.spdxClient.WorkloadConfigurationScans(queryNamespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		toolErr := errors.NewKubescapeError("list_configuration_scans", err).
			WithContext("namespace", namespace)
		return kubescapeErrResult(toolErr), listConfigurationScansOutput{}, nil
	}

	configManifests := []configurationScanSummary{}
	for _, manifest := range manifests.Items {
		configManifests = append(configManifests, configurationScanSummary{
			Namespace:    manifest.Namespace,
			ManifestName: manifest.Name,
			CreatedAt:    manifest.CreationTimestamp.Format(time.RFC3339),
		})
	}

	result := listConfigurationScansOutput{
		ConfigurationScans: configManifests,
		TotalCount:         len(configManifests),
	}

	content, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to marshal result: %v", err)), listConfigurationScansOutput{}, nil
	}

	return mcp.NewToolResultText(string(content)), result, nil
}

// handleGetConfigurationScan gets details of a specific configuration scan
func (k *KubescapeTool) handleGetConfigurationScan(ctx context.Context, request *sdkmcp.CallToolRequest, in getConfigurationScanInput) (*sdkmcp.CallToolResult, mcp.TextOutput, error) {
	if k.initError != nil {
		toolErr := errors.NewKubescapeError("get_configuration_scan", k.initError)
		res := kubescapeErrResult(toolErr)
		return res, mcp.TextOf(res), nil
	}

	namespace := in.Namespace
	if namespace == "" {
		namespace = defaultKubescapeNamespace
	}
	manifestName := in.ManifestName

	if manifestName == "" {
		return mcp.TextError("manifest_name parameter is required")
	}

	manifest, err := k.spdxClient.WorkloadConfigurationScans(namespace).Get(ctx, manifestName, metav1.GetOptions{})
	if err != nil {
		toolErr := errors.NewKubescapeError("get_configuration_scan", err).
			WithContext("namespace", namespace).
			WithContext("manifest_name", manifestName)
		res := kubescapeErrResult(toolErr)
		return res, mcp.TextOf(res), nil
	}

	content, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return mcp.TextError(fmt.Sprintf("failed to marshal result: %v", err))
	}

	return mcp.TextResult(string(content))
}

// handleListApplicationProfiles lists application profiles showing runtime behavior data
func (k *KubescapeTool) handleListApplicationProfiles(ctx context.Context, request *sdkmcp.CallToolRequest, in listApplicationProfilesInput) (*sdkmcp.CallToolResult, listApplicationProfilesOutput, error) {
	if k.initError != nil {
		toolErr := errors.NewKubescapeError("list_application_profiles", k.initError)
		return kubescapeErrResult(toolErr), listApplicationProfilesOutput{}, nil
	}

	namespace := in.Namespace

	queryNamespace := metav1.NamespaceAll
	if namespace != "" {
		queryNamespace = namespace
	}

	profiles, err := k.spdxClient.ApplicationProfiles(queryNamespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		toolErr := errors.NewKubescapeError("list_application_profiles", err).
			WithContext("namespace", namespace)
		return kubescapeErrResult(toolErr), listApplicationProfilesOutput{}, nil
	}

	profileList := []applicationProfileSummary{}
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

	result := listApplicationProfilesOutput{
		ApplicationProfiles: profileList,
		TotalCount:          len(profileList),
		Description: "ApplicationProfiles capture runtime behavior of workloads including: " +
			"executed processes (Execs), file access patterns (Opens), system calls (Syscalls), " +
			"Linux capabilities used, and HTTP endpoints accessed. " +
			"Use this data to prioritize vulnerabilities - a CVE in an unused package is lower priority than one in an actively running process.",
	}

	content, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to marshal result: %v", err)), listApplicationProfilesOutput{}, nil
	}

	return mcp.NewToolResultText(string(content)), result, nil
}

// handleGetApplicationProfile gets detailed runtime behavior for a specific workload
func (k *KubescapeTool) handleGetApplicationProfile(ctx context.Context, request *sdkmcp.CallToolRequest, in getApplicationProfileInput) (*sdkmcp.CallToolResult, getApplicationProfileOutput, error) {
	if k.initError != nil {
		toolErr := errors.NewKubescapeError("get_application_profile", k.initError)
		return kubescapeErrResult(toolErr), getApplicationProfileOutput{}, nil
	}

	namespace := in.Namespace
	name := in.Name

	if name == "" {
		return mcp.NewToolResultError("name parameter is required"), getApplicationProfileOutput{}, nil
	}
	if namespace == "" {
		return mcp.NewToolResultError("namespace parameter is required"), getApplicationProfileOutput{}, nil
	}

	profile, err := k.spdxClient.ApplicationProfiles(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		toolErr := errors.NewKubescapeError("get_application_profile", err).
			WithContext("namespace", namespace).
			WithContext("name", name)
		return kubescapeErrResult(toolErr), getApplicationProfileOutput{}, nil
	}

	// Build detailed response with container behaviors
	containers := []containerBehavior{}
	for _, c := range profile.Spec.Containers {
		containerInfo := containerBehavior{
			Name:         c.Name,
			Execs:        c.Execs,
			Opens:        c.Opens,
			Syscalls:     c.Syscalls,
			Capabilities: c.Capabilities,
			Endpoints:    c.Endpoints,
		}
		if c.SeccompProfile.Name != "" || c.SeccompProfile.Path != "" {
			seccompProfile := c.SeccompProfile
			containerInfo.SeccompProfile = &seccompProfile
		}
		containers = append(containers, containerInfo)
	}

	initContainers := []containerBehavior{}
	for _, c := range profile.Spec.InitContainers {
		initContainers = append(initContainers, containerBehavior{
			Name:         c.Name,
			Execs:        c.Execs,
			Opens:        c.Opens,
			Syscalls:     c.Syscalls,
			Capabilities: c.Capabilities,
			Endpoints:    c.Endpoints,
		})
	}

	result := getApplicationProfileOutput{
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
		return mcp.NewToolResultError(fmt.Sprintf("failed to marshal result: %v", err)), getApplicationProfileOutput{}, nil
	}

	return mcp.NewToolResultText(string(content)), result, nil
}

// handleListNetworkNeighborhoods lists network communication patterns for workloads
func (k *KubescapeTool) handleListNetworkNeighborhoods(ctx context.Context, request *sdkmcp.CallToolRequest, in listNetworkNeighborhoodsInput) (*sdkmcp.CallToolResult, listNetworkNeighborhoodsOutput, error) {
	if k.initError != nil {
		toolErr := errors.NewKubescapeError("list_network_neighborhoods", k.initError)
		return kubescapeErrResult(toolErr), listNetworkNeighborhoodsOutput{}, nil
	}

	namespace := in.Namespace

	queryNamespace := metav1.NamespaceAll
	if namespace != "" {
		queryNamespace = namespace
	}

	neighborhoods, err := k.spdxClient.NetworkNeighborhoods(queryNamespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		toolErr := errors.NewKubescapeError("list_network_neighborhoods", err).
			WithContext("namespace", namespace)
		return kubescapeErrResult(toolErr), listNetworkNeighborhoodsOutput{}, nil
	}

	neighborhoodList := []networkNeighborhoodSummary{}
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

	result := listNetworkNeighborhoodsOutput{
		NetworkNeighborhoods: neighborhoodList,
		TotalCount:           len(neighborhoodList),
		Description: "NetworkNeighborhoods capture actual network communication patterns of workloads. " +
			"Ingress: connections coming INTO the workload; Egress: connections going OUT from the workload. " +
			"Includes DNS names, IP addresses, ports, and protocols. " +
			"Use this data to understand attack surface and prioritize network-related security findings.",
	}

	content, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to marshal result: %v", err)), listNetworkNeighborhoodsOutput{}, nil
	}

	return mcp.NewToolResultText(string(content)), result, nil
}

// handleGetNetworkNeighborhood gets detailed network connections for a specific workload
func (k *KubescapeTool) handleGetNetworkNeighborhood(ctx context.Context, request *sdkmcp.CallToolRequest, in getNetworkNeighborhoodInput) (*sdkmcp.CallToolResult, getNetworkNeighborhoodOutput, error) {
	if k.initError != nil {
		toolErr := errors.NewKubescapeError("get_network_neighborhood", k.initError)
		return kubescapeErrResult(toolErr), getNetworkNeighborhoodOutput{}, nil
	}

	namespace := in.Namespace
	name := in.Name

	if name == "" {
		return mcp.NewToolResultError("name parameter is required"), getNetworkNeighborhoodOutput{}, nil
	}
	if namespace == "" {
		return mcp.NewToolResultError("namespace parameter is required"), getNetworkNeighborhoodOutput{}, nil
	}

	nn, err := k.spdxClient.NetworkNeighborhoods(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		toolErr := errors.NewKubescapeError("get_network_neighborhood", err).
			WithContext("namespace", namespace).
			WithContext("name", name)
		return kubescapeErrResult(toolErr), getNetworkNeighborhoodOutput{}, nil
	}

	// Build detailed response with container network data
	containers := []networkContainer{}
	for _, c := range nn.Spec.Containers {
		// Format ingress connections
		ingressList := []networkConnection{}
		for _, ing := range c.Ingress {
			ingressInfo := networkConnection{
				Identifier: ing.Identifier,
				Type:       ing.Type,
			}
			if ing.DNS != "" {
				ingressInfo.DNS = ing.DNS
			}
			if len(ing.Ports) > 0 {
				ingressInfo.Ports = ing.Ports
			}
			if len(ing.IPAddress) > 0 {
				ingressInfo.IPAddress = ing.IPAddress
			}
			if ing.PodSelector != nil {
				ingressInfo.PodSelector = ing.PodSelector
			}
			if ing.NamespaceSelector != nil {
				ingressInfo.NamespaceSelector = ing.NamespaceSelector
			}
			ingressList = append(ingressList, ingressInfo)
		}

		// Format egress connections
		egressList := []networkConnection{}
		for _, egr := range c.Egress {
			egressInfo := networkConnection{
				Identifier: egr.Identifier,
				Type:       egr.Type,
			}
			if egr.DNS != "" {
				egressInfo.DNS = egr.DNS
			}
			if len(egr.Ports) > 0 {
				egressInfo.Ports = egr.Ports
			}
			if len(egr.IPAddress) > 0 {
				egressInfo.IPAddress = egr.IPAddress
			}
			if egr.PodSelector != nil {
				egressInfo.PodSelector = egr.PodSelector
			}
			if egr.NamespaceSelector != nil {
				egressInfo.NamespaceSelector = egr.NamespaceSelector
			}
			egressList = append(egressList, egressInfo)
		}

		containers = append(containers, networkContainer{
			Name:    c.Name,
			Ingress: ingressList,
			Egress:  egressList,
		})
	}

	result := getNetworkNeighborhoodOutput{
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
		return mcp.NewToolResultError(fmt.Sprintf("failed to marshal result: %v", err)), getNetworkNeighborhoodOutput{}, nil
	}

	return mcp.NewToolResultText(string(content)), result, nil
}

// Helper function to truncate strings
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// RegisterTools registers all Kubescape tools with the MCP server
func RegisterTools(s *sdkmcp.Server, kubeconfig string, readOnly bool) {
	tool := NewKubescapeTool(kubeconfig)
	_ = readOnly // all kubescape tools are read-only

	mcp.AddTool(s, "kubescape", &sdkmcp.Tool{
		Name:        "kubescape_check_health",
		Description: "Check if Kubescape operator is installed and operational. Verifies namespace, operator pods, storage pods, CRDs, and scan data availability.",
	}, tool.handleCheckHealth)

	// Cluster / namespace vulnerability posture -- the cheapest entry point
	mcp.AddTool(s, "kubescape", &sdkmcp.Tool{
		Name: "kubescape_vulnerability_overview",
		Description: "START HERE for any question about cluster-wide or namespace-wide vulnerabilities. " +
			"Returns severity totals per namespace from Kubescape's server-side aggregates in a single cheap call, worst namespace first. " +
			"Counts are 'all' plus 'relevant' (the vulnerable code was observed loaded at runtime). " +
			"'relevant' is reported only when greater than zero; when it is absent that means EITHER no runtime-relevant CVEs OR that relevancy " +
			"has not been computed for those workloads yet, and the two cannot be distinguished here -- so do not report an absent 'relevant' as a measured zero. " +
			"Then narrow with kubescape_list_vulnerable_workloads.",
	}, tool.handleVulnerabilityOverview)

	// Rank workloads and hand back the manifest to drill into
	mcp.AddTool(s, "kubescape", &sdkmcp.Tool{
		Name: "kubescape_list_vulnerable_workloads",
		Description: "Rank workloads by vulnerability severity, worst first, and return the 'manifest_name' needed to inspect each one's CVEs. " +
			"Use after kubescape_vulnerability_overview to find which workloads matter, then pass a returned manifest_name to kubescape_list_vulnerabilities.",
	}, tool.handleListVulnerableWorkloads)

	mcp.AddTool(s, "kubescape", &sdkmcp.Tool{
		Name: "kubescape_list_vulnerability_manifests",
		Description: "List vulnerability manifests from Kubescape operator, at image or workload level. " +
			"This is an index only: it does NOT report how many vulnerabilities each manifest contains, " +
			"and an entry appearing here says nothing about whether that image is clean. " +
			"To get vulnerability counts and severities for a manifest, call kubescape_list_vulnerabilities " +
			"with its manifest_name.",
	}, tool.handleListVulnerabilityManifests)

	mcp.AddTool(s, "kubescape", &sdkmcp.Tool{
		Name: "kubescape_list_vulnerabilities",
		Description: "List CVEs in a specific vulnerability manifest. Always returns severity_summary, which counts EVERY CVE in the manifest. " +
			"The 'vulnerabilities' array is capped at 'limit' (default 20), ordered worst severity first; check 'truncated' and 'total_count' to see whether more exist. " +
			"Each entry carries only id, severity and fix_state -- use kubescape_get_vulnerability_details for the description, data source and fix versions of one CVE.",
	}, tool.handleListVulnerabilitiesInManifest)

	mcp.AddTool(s, "kubescape", &sdkmcp.Tool{
		Name:        "kubescape_get_vulnerability_details",
		Description: "Get detailed information about a specific CVE in a vulnerability manifest, including affected packages and fix information.",
	}, tool.handleGetVulnerabilityDetails)

	mcp.AddTool(s, "kubescape", &sdkmcp.Tool{
		Name:        "kubescape_list_configuration_scans",
		Description: "List configuration security scan results from Kubescape operator. Shows workloads that have been scanned for security misconfigurations.",
	}, tool.handleListConfigurationScans)

	mcp.AddTool(s, "kubescape", &sdkmcp.Tool{
		Name:        "kubescape_get_configuration_scan",
		Description: "Get detailed configuration security scan results for a specific workload, including failed controls and remediation guidance.",
	}, tool.handleGetConfigurationScan)

	mcp.AddTool(s, "kubescape", &sdkmcp.Tool{
		Name: "kubescape_list_application_profiles",
		Description: "List ApplicationProfiles showing runtime behavior of workloads. These profiles capture: " +
			"executed processes (Execs), file access patterns (Opens), system calls (Syscalls), Linux capabilities used, and HTTP endpoints. " +
			"Use this data to prioritize vulnerability findings - a CVE in an unused package is lower priority than one in an actively running process. " +
			"Requires 'capabilities.runtimeObservability=enable' in Kubescape Helm chart.",
	}, tool.handleListApplicationProfiles)

	mcp.AddTool(s, "kubescape", &sdkmcp.Tool{
		Name: "kubescape_get_application_profile",
		Description: "Get detailed runtime behavior profile for a specific workload. Shows what processes run, what files are accessed, " +
			"what system calls are made, and what capabilities are used per container. " +
			"Compare with CVE findings to prioritize remediation - focus on vulnerabilities affecting actively used components.",
	}, tool.handleGetApplicationProfile)

	mcp.AddTool(s, "kubescape", &sdkmcp.Tool{
		Name: "kubescape_list_network_neighborhoods",
		Description: "List NetworkNeighborhoods showing actual network communication patterns of workloads. " +
			"These capture: ingress connections (who talks TO the workload), egress connections (who the workload talks TO), " +
			"including DNS names, IP addresses, ports, and protocols. " +
			"Use this to understand attack surface and prioritize network-related security findings. " +
			"Requires 'capabilities.runtimeObservability=enable' in Kubescape Helm chart.",
	}, tool.handleListNetworkNeighborhoods)

	mcp.AddTool(s, "kubescape", &sdkmcp.Tool{
		Name: "kubescape_get_network_neighborhood",
		Description: "Get detailed network connections for a specific workload. Shows all observed ingress and egress traffic " +
			"with DNS names, IPs, ports, and protocols. Use this to verify if a workload with a vulnerability is actually exposed to the network.",
	}, tool.handleGetNetworkNeighborhood)

	// NOTE: SBOM tools are disabled as they return too much data for LLM context windows.
}

// Interfaces for testing - allows mocking the Kubernetes clients
type KubescapeToolInterface interface {
	HandleCheckHealth(ctx context.Context, in checkHealthInput) (*sdkmcp.CallToolResult, HealthCheckResult, error)
	HandleVulnerabilityOverview(ctx context.Context, in vulnerabilityOverviewInput) (*sdkmcp.CallToolResult, vulnerabilityOverviewOutput, error)
	HandleListVulnerableWorkloads(ctx context.Context, in listVulnerableWorkloadsInput) (*sdkmcp.CallToolResult, listVulnerableWorkloadsOutput, error)
	HandleListVulnerabilityManifests(ctx context.Context, in listVulnerabilityManifestsInput) (*sdkmcp.CallToolResult, listVulnerabilityManifestsOutput, error)
	HandleListVulnerabilitiesInManifest(ctx context.Context, in listVulnerabilitiesInManifestInput) (*sdkmcp.CallToolResult, listVulnerabilitiesInManifestOutput, error)
	HandleGetVulnerabilityDetails(ctx context.Context, in getVulnerabilityDetailsInput) (*sdkmcp.CallToolResult, []v1beta1.Match, error)
	HandleListConfigurationScans(ctx context.Context, in listConfigurationScansInput) (*sdkmcp.CallToolResult, listConfigurationScansOutput, error)
	HandleGetConfigurationScan(ctx context.Context, in getConfigurationScanInput) (*sdkmcp.CallToolResult, mcp.TextOutput, error)
	HandleListApplicationProfiles(ctx context.Context, in listApplicationProfilesInput) (*sdkmcp.CallToolResult, listApplicationProfilesOutput, error)
	HandleGetApplicationProfile(ctx context.Context, in getApplicationProfileInput) (*sdkmcp.CallToolResult, getApplicationProfileOutput, error)
	HandleListNetworkNeighborhoods(ctx context.Context, in listNetworkNeighborhoodsInput) (*sdkmcp.CallToolResult, listNetworkNeighborhoodsOutput, error)
	HandleGetNetworkNeighborhood(ctx context.Context, in getNetworkNeighborhoodInput) (*sdkmcp.CallToolResult, getNetworkNeighborhoodOutput, error)
}

// Ensure KubescapeTool implements the interface
var _ KubescapeToolInterface = (*KubescapeTool)(nil)

// Export handler methods for testing
func (k *KubescapeTool) HandleCheckHealth(ctx context.Context, in checkHealthInput) (*sdkmcp.CallToolResult, HealthCheckResult, error) {
	return k.handleCheckHealth(ctx, &sdkmcp.CallToolRequest{}, in)
}

func (k *KubescapeTool) HandleVulnerabilityOverview(ctx context.Context, in vulnerabilityOverviewInput) (*sdkmcp.CallToolResult, vulnerabilityOverviewOutput, error) {
	return k.handleVulnerabilityOverview(ctx, &sdkmcp.CallToolRequest{}, in)
}

func (k *KubescapeTool) HandleListVulnerableWorkloads(ctx context.Context, in listVulnerableWorkloadsInput) (*sdkmcp.CallToolResult, listVulnerableWorkloadsOutput, error) {
	return k.handleListVulnerableWorkloads(ctx, &sdkmcp.CallToolRequest{}, in)
}

func (k *KubescapeTool) HandleListVulnerabilityManifests(ctx context.Context, in listVulnerabilityManifestsInput) (*sdkmcp.CallToolResult, listVulnerabilityManifestsOutput, error) {
	return k.handleListVulnerabilityManifests(ctx, &sdkmcp.CallToolRequest{}, in)
}

func (k *KubescapeTool) HandleListVulnerabilitiesInManifest(ctx context.Context, in listVulnerabilitiesInManifestInput) (*sdkmcp.CallToolResult, listVulnerabilitiesInManifestOutput, error) {
	return k.handleListVulnerabilitiesInManifest(ctx, &sdkmcp.CallToolRequest{}, in)
}

func (k *KubescapeTool) HandleGetVulnerabilityDetails(ctx context.Context, in getVulnerabilityDetailsInput) (*sdkmcp.CallToolResult, []v1beta1.Match, error) {
	return k.handleGetVulnerabilityDetails(ctx, &sdkmcp.CallToolRequest{}, in)
}

func (k *KubescapeTool) HandleListConfigurationScans(ctx context.Context, in listConfigurationScansInput) (*sdkmcp.CallToolResult, listConfigurationScansOutput, error) {
	return k.handleListConfigurationScans(ctx, &sdkmcp.CallToolRequest{}, in)
}

func (k *KubescapeTool) HandleGetConfigurationScan(ctx context.Context, in getConfigurationScanInput) (*sdkmcp.CallToolResult, mcp.TextOutput, error) {
	return k.handleGetConfigurationScan(ctx, &sdkmcp.CallToolRequest{}, in)
}

func (k *KubescapeTool) HandleListApplicationProfiles(ctx context.Context, in listApplicationProfilesInput) (*sdkmcp.CallToolResult, listApplicationProfilesOutput, error) {
	return k.handleListApplicationProfiles(ctx, &sdkmcp.CallToolRequest{}, in)
}

func (k *KubescapeTool) HandleGetApplicationProfile(ctx context.Context, in getApplicationProfileInput) (*sdkmcp.CallToolResult, getApplicationProfileOutput, error) {
	return k.handleGetApplicationProfile(ctx, &sdkmcp.CallToolRequest{}, in)
}

func (k *KubescapeTool) HandleListNetworkNeighborhoods(ctx context.Context, in listNetworkNeighborhoodsInput) (*sdkmcp.CallToolResult, listNetworkNeighborhoodsOutput, error) {
	return k.handleListNetworkNeighborhoods(ctx, &sdkmcp.CallToolRequest{}, in)
}

func (k *KubescapeTool) HandleGetNetworkNeighborhood(ctx context.Context, in getNetworkNeighborhoodInput) (*sdkmcp.CallToolResult, getNetworkNeighborhoodOutput, error) {
	return k.handleGetNetworkNeighborhood(ctx, &sdkmcp.CallToolRequest{}, in)
}
