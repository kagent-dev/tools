package kubescape

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	helpersv1 "github.com/kubescape/k8s-interface/instanceidhandler/v1/helpers"
	"github.com/kubescape/storage/pkg/apis/softwarecomposition/v1beta1"
	kubescapefake "github.com/kubescape/storage/pkg/generated/clientset/versioned/fake"
	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	kubefake "k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
)

// Helper function to extract text content from MCP result
func getResultText(result *sdkmcp.CallToolResult) string {
	if result == nil || len(result.Content) == 0 {
		return ""
	}
	if textContent, ok := result.Content[0].(*sdkmcp.TextContent); ok {
		return textContent.Text
	}
	return ""
}

func TestRegisterTools(t *testing.T) {
	s := sdkmcp.NewServer(&sdkmcp.Implementation{Name: "test", Version: "1.0.0"}, nil)
	assert.NotPanics(t, func() {
		RegisterTools(s, "", false)
	})
}

func TestHandleCheckHealth_AllComponentsHealthy(t *testing.T) {
	// Setup fake clients with all components healthy
	//nolint:staticcheck // NewSimpleClientset is deprecated but NewClientset requires generated apply configs
	k8sClient := kubefake.NewSimpleClientset(
		// Namespace
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "kubescape"}},
		// Operator pods
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "kubescape-operator-123",
				Namespace: "kubescape",
				Labels:    map[string]string{"app.kubernetes.io/name": "kubescape-operator", "app": "operator"},
			},
			Status: corev1.PodStatus{Phase: corev1.PodRunning},
		},
		// Storage pods
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "storage-123",
				Namespace: "kubescape",
				Labels:    map[string]string{"app.kubernetes.io/name": "kubescape-operator", "app": "storage"},
			},
			Status: corev1.PodStatus{Phase: corev1.PodRunning},
		},
	)

	spdxClient := kubescapefake.NewClientset(
		&v1beta1.VulnerabilityManifest{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-manifest",
				Namespace: "kubescape",
			},
		},
		&v1beta1.WorkloadConfigurationScan{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-config-scan",
				Namespace: "kubescape",
			},
		},
		&v1beta1.ApplicationProfile{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-app-profile",
				Namespace: "kubescape",
			},
		},
		&v1beta1.NetworkNeighborhood{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-network-neighborhood",
				Namespace: "kubescape",
			},
		},
		// NOTE: SBOM data check is disabled (SBOM tools are too large for LLM context)
	)

	tool := NewKubescapeToolWithClients(k8sClient, spdxClient.SpdxV1beta1())

	result, _, err := tool.HandleCheckHealth(context.Background(), checkHealthInput{})
	require.NoError(t, err)
	require.NotNil(t, result)

	// Parse the response
	var health HealthCheckResult
	err = json.Unmarshal([]byte(getResultText(result)), &health)
	require.NoError(t, err)

	assert.True(t, health.Healthy)
	assert.Equal(t, "ok", health.Checks["namespace"].Status)
	assert.Equal(t, "ok", health.Checks["operator_pods"].Status)
	assert.Equal(t, "ok", health.Checks["storage_pods"].Status)
	assert.Equal(t, "ok", health.Checks["vulnerability_crd"].Status)
	assert.Equal(t, "ok", health.Checks["configuration_crd"].Status)
	assert.Equal(t, "ok", health.Checks["vulnerability_scan_data"].Status)
	assert.Equal(t, "ok", health.Checks["configuration_scan_data"].Status)
	assert.Equal(t, "ok", health.Checks["application_profiles_crd"].Status)
	assert.Equal(t, "ok", health.Checks["application_profiles_data"].Status)
	assert.Equal(t, "ok", health.Checks["network_neighborhoods_crd"].Status)
	assert.Equal(t, "ok", health.Checks["network_neighborhoods_data"].Status)
	// NOTE: SBOM checks are disabled (SBOM tools are too large for LLM context)
	// assert.Equal(t, "ok", health.Checks["sbom_crd"].Status)
	// assert.Equal(t, "ok", health.Checks["sbom_data"].Status)
	assert.Equal(t, "Kubescape is fully operational", health.Summary)
}

func TestHandleCheckHealth_NamespaceNotFound(t *testing.T) {
	//nolint:staticcheck // NewSimpleClientset is deprecated but NewClientset requires generated apply configs
	k8sClient := kubefake.NewSimpleClientset() // No namespace
	spdxClient := kubescapefake.NewClientset()

	tool := NewKubescapeToolWithClients(k8sClient, spdxClient.SpdxV1beta1())

	result, _, err := tool.HandleCheckHealth(context.Background(), checkHealthInput{})
	require.NoError(t, err)
	require.NotNil(t, result)

	var health HealthCheckResult
	err = json.Unmarshal([]byte(getResultText(result)), &health)
	require.NoError(t, err)

	assert.False(t, health.Healthy)
	assert.Equal(t, "error", health.Checks["namespace"].Status)
	assert.Contains(t, health.Checks["namespace"].Message, "not found")
}

func TestHandleCheckHealth_OperatorPodsNotRunning(t *testing.T) {
	//nolint:staticcheck // NewSimpleClientset is deprecated but NewClientset requires generated apply configs
	k8sClient := kubefake.NewSimpleClientset(
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "kubescape"}},
		// No operator pods
	)
	spdxClient := kubescapefake.NewClientset()

	tool := NewKubescapeToolWithClients(k8sClient, spdxClient.SpdxV1beta1())

	result, _, err := tool.HandleCheckHealth(context.Background(), checkHealthInput{})
	require.NoError(t, err)
	require.NotNil(t, result)

	var health HealthCheckResult
	err = json.Unmarshal([]byte(getResultText(result)), &health)
	require.NoError(t, err)

	assert.False(t, health.Healthy)
	assert.Equal(t, "error", health.Checks["operator_pods"].Status)
	assert.Contains(t, health.Checks["operator_pods"].Message, "No operator pods found")
	assert.Contains(t, health.Recommendations, "Install Kubescape operator: helm upgrade --install kubescape kubescape/kubescape-operator -n kubescape --create-namespace")
}

func TestHandleCheckHealth_OperatorPodsUnhealthy(t *testing.T) {
	//nolint:staticcheck // NewSimpleClientset is deprecated but NewClientset requires generated apply configs
	k8sClient := kubefake.NewSimpleClientset(
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "kubescape"}},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "kubescape-operator-123",
				Namespace: "kubescape",
				Labels:    map[string]string{"app.kubernetes.io/name": "kubescape-operator", "app": "operator"},
			},
			Status: corev1.PodStatus{Phase: corev1.PodPending}, // Not running
		},
	)
	spdxClient := kubescapefake.NewClientset()

	tool := NewKubescapeToolWithClients(k8sClient, spdxClient.SpdxV1beta1())

	result, _, err := tool.HandleCheckHealth(context.Background(), checkHealthInput{})
	require.NoError(t, err)
	require.NotNil(t, result)

	var health HealthCheckResult
	err = json.Unmarshal([]byte(getResultText(result)), &health)
	require.NoError(t, err)

	assert.False(t, health.Healthy)
	assert.Equal(t, "warning", health.Checks["operator_pods"].Status)
	assert.Contains(t, health.Checks["operator_pods"].Message, "0/1 pods running")
}

// One resource being unreachable must fail only that resource's checks -- a
// partially degraded storage API should still report what does work.
func TestHandleCheckHealth_VulnerabilityAPIUnavailable(t *testing.T) {
	//nolint:staticcheck // NewSimpleClientset is deprecated but NewClientset requires generated apply configs
	k8sClient := kubefake.NewSimpleClientset(
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "kubescape"}},
	)
	spdxClient := kubescapefake.NewClientset()
	spdxClient.PrependReactor("list", "vulnerabilitymanifests", func(action k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, k8serrors.NewServiceUnavailable("no response from storage service")
	})

	tool := NewKubescapeToolWithClients(k8sClient, spdxClient.SpdxV1beta1())

	result, _, err := tool.HandleCheckHealth(context.Background(), checkHealthInput{})
	require.NoError(t, err)
	require.NotNil(t, result)

	var health HealthCheckResult
	err = json.Unmarshal([]byte(getResultText(result)), &health)
	require.NoError(t, err)

	assert.False(t, health.Healthy)
	assert.Equal(t, "error", health.Checks["vulnerability_crd"].Status)
	assert.Contains(t, health.Checks["vulnerability_crd"].Message, "not available")
	// The configuration API is still reachable, so it must still report ok.
	assert.Equal(t, "ok", health.Checks["configuration_crd"].Status)
}

func TestHandleCheckHealth_NoScanData(t *testing.T) {
	//nolint:staticcheck // NewSimpleClientset is deprecated but NewClientset requires generated apply configs
	k8sClient := kubefake.NewSimpleClientset(
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "kubescape"}},
	)
	spdxClient := kubescapefake.NewClientset() // No vulnerability manifests

	tool := NewKubescapeToolWithClients(k8sClient, spdxClient.SpdxV1beta1())

	result, _, err := tool.HandleCheckHealth(context.Background(), checkHealthInput{})
	require.NoError(t, err)
	require.NotNil(t, result)

	var health HealthCheckResult
	err = json.Unmarshal([]byte(getResultText(result)), &health)
	require.NoError(t, err)

	// Warning for no scan data
	assert.Equal(t, "warning", health.Checks["vulnerability_scan_data"].Status)
	assert.Contains(t, health.Checks["vulnerability_scan_data"].Message, "No vulnerability manifests found")
}

// Runtime observability is optional: its resources being unreachable warns and
// recommends enabling the capability, but does not fail the health check.
func TestHandleCheckHealth_RuntimeObservabilityAPIUnavailable(t *testing.T) {
	//nolint:staticcheck // NewSimpleClientset is deprecated but NewClientset requires generated apply configs
	k8sClient := kubefake.NewSimpleClientset(
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "kubescape"}},
	)
	spdxClient := kubescapefake.NewClientset()
	for _, resource := range []string{"applicationprofiles", "networkneighborhoods"} {
		spdxClient.PrependReactor("list", resource, func(action k8stesting.Action) (bool, runtime.Object, error) {
			return true, nil, k8serrors.NewServiceUnavailable("no response from storage service")
		})
	}

	tool := NewKubescapeToolWithClients(k8sClient, spdxClient.SpdxV1beta1())

	result, _, err := tool.HandleCheckHealth(context.Background(), checkHealthInput{})
	require.NoError(t, err)
	require.NotNil(t, result)

	var health HealthCheckResult
	err = json.Unmarshal([]byte(getResultText(result)), &health)
	require.NoError(t, err)

	assert.Equal(t, "warning", health.Checks["application_profiles_crd"].Status)
	assert.Contains(t, health.Checks["application_profiles_crd"].Message, "not available")
	assert.Equal(t, "warning", health.Checks["network_neighborhoods_crd"].Status)
	assert.Contains(t, health.Checks["network_neighborhoods_crd"].Message, "not available")

	// Should have recommendation to enable runtime observability
	foundRuntimeRecommendation := false
	for _, r := range health.Recommendations {
		if contains(r, "runtimeObservability") {
			foundRuntimeRecommendation = true
			break
		}
	}
	assert.True(t, foundRuntimeRecommendation, "Expected recommendation to enable runtimeObservability")
}

// Helper function for test
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func TestHandleCheckHealth_CustomNamespace(t *testing.T) {
	//nolint:staticcheck // NewSimpleClientset is deprecated but NewClientset requires generated apply configs
	k8sClient := kubefake.NewSimpleClientset(
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "custom-ns"}},
	)
	spdxClient := kubescapefake.NewClientset()

	tool := NewKubescapeToolWithClients(k8sClient, spdxClient.SpdxV1beta1())

	result, _, err := tool.HandleCheckHealth(context.Background(), checkHealthInput{
		Namespace: "custom-ns",
	})
	require.NoError(t, err)
	require.NotNil(t, result)

	var health HealthCheckResult
	err = json.Unmarshal([]byte(getResultText(result)), &health)
	require.NoError(t, err)

	assert.Equal(t, "ok", health.Checks["namespace"].Status)
	assert.Contains(t, health.Checks["namespace"].Message, "custom-ns")
}

func TestHandleCheckHealth_InitError(t *testing.T) {
	tool := NewKubescapeToolWithError(errors.New("failed to connect"))

	result, _, err := tool.HandleCheckHealth(context.Background(), checkHealthInput{})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.IsError)
}

func TestHandleListVulnerabilityManifests_Success(t *testing.T) {
	spdxClient := kubescapefake.NewClientset(
		&v1beta1.VulnerabilityManifest{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "manifest-1",
				Namespace: "default",
				Annotations: map[string]string{
					"kubescape.io/image-id":  "sha256:abc123",
					"kubescape.io/image-tag": "nginx:1.19",
				},
			},
			Spec: v1beta1.VulnerabilityManifestSpec{
				Payload: v1beta1.GrypeDocument{
					Matches: []v1beta1.Match{
						{Vulnerability: v1beta1.Vulnerability{VulnerabilityMetadata: v1beta1.VulnerabilityMetadata{ID: "CVE-2021-1234"}}},
					},
				},
			},
		},
		&v1beta1.VulnerabilityManifest{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "manifest-2",
				Namespace: "kubescape",
			},
		},
	)

	tool := NewKubescapeToolWithClients(nil, spdxClient.SpdxV1beta1())

	result, _, err := tool.HandleListVulnerabilityManifests(context.Background(), listVulnerabilityManifestsInput{})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.False(t, result.IsError)

	var response listVulnerabilityManifestsOutput
	err = json.Unmarshal([]byte(getResultText(result)), &response)
	require.NoError(t, err)

	assert.Equal(t, 2, response.TotalCount)
	assert.Len(t, response.VulnerabilityManifests, 2)
}

func TestHandleListVulnerabilityManifests_FilterByNamespace(t *testing.T) {
	spdxClient := kubescapefake.NewClientset(
		&v1beta1.VulnerabilityManifest{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "manifest-1",
				Namespace: "default",
			},
		},
	)

	tool := NewKubescapeToolWithClients(nil, spdxClient.SpdxV1beta1())

	result, _, err := tool.HandleListVulnerabilityManifests(context.Background(), listVulnerabilityManifestsInput{
		Namespace: "default",
	})
	require.NoError(t, err)
	require.NotNil(t, result)

	var response listVulnerabilityManifestsOutput
	err = json.Unmarshal([]byte(getResultText(result)), &response)
	require.NoError(t, err)

	assert.Equal(t, 1, response.TotalCount)
}

func TestHandleListVulnerabilityManifests_EmptyResults(t *testing.T) {
	spdxClient := kubescapefake.NewClientset()
	tool := NewKubescapeToolWithClients(nil, spdxClient.SpdxV1beta1())

	result, _, err := tool.HandleListVulnerabilityManifests(context.Background(), listVulnerabilityManifestsInput{})
	require.NoError(t, err)
	require.NotNil(t, result)

	var response listVulnerabilityManifestsOutput
	err = json.Unmarshal([]byte(getResultText(result)), &response)
	require.NoError(t, err)

	assert.Equal(t, 0, response.TotalCount)
}

func TestHandleListVulnerabilityManifests_InitError(t *testing.T) {
	tool := NewKubescapeToolWithError(errors.New("failed to connect"))

	result, _, err := tool.HandleListVulnerabilityManifests(context.Background(), listVulnerabilityManifestsInput{})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.IsError)
}

func TestHandleListVulnerabilitiesInManifest_Success(t *testing.T) {
	spdxClient := kubescapefake.NewClientset(
		&v1beta1.VulnerabilityManifest{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-manifest",
				Namespace: "kubescape",
			},
			Spec: v1beta1.VulnerabilityManifestSpec{
				Payload: v1beta1.GrypeDocument{
					Matches: []v1beta1.Match{
						{
							Vulnerability: v1beta1.Vulnerability{
								VulnerabilityMetadata: v1beta1.VulnerabilityMetadata{
									ID:          "CVE-2021-1234",
									Severity:    "Critical",
									Description: "Test vulnerability",
								},
							},
						},
						{
							Vulnerability: v1beta1.Vulnerability{
								VulnerabilityMetadata: v1beta1.VulnerabilityMetadata{
									ID:       "CVE-2021-5678",
									Severity: "High",
								},
							},
						},
					},
				},
			},
		},
	)

	tool := NewKubescapeToolWithClients(nil, spdxClient.SpdxV1beta1())

	result, _, err := tool.HandleListVulnerabilitiesInManifest(context.Background(), listVulnerabilitiesInManifestInput{
		ManifestName: "test-manifest",
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.False(t, result.IsError)

	var response listVulnerabilitiesInManifestOutput
	err = json.Unmarshal([]byte(getResultText(result)), &response)
	require.NoError(t, err)

	assert.Equal(t, 2, response.TotalCount)
	assert.Equal(t, 1, response.SeveritySummary.Critical)
	assert.Equal(t, 1, response.SeveritySummary.High)
}

func TestHandleListVulnerabilitiesInManifest_MissingManifestName(t *testing.T) {
	spdxClient := kubescapefake.NewClientset()
	tool := NewKubescapeToolWithClients(nil, spdxClient.SpdxV1beta1())

	result, _, err := tool.HandleListVulnerabilitiesInManifest(context.Background(), listVulnerabilitiesInManifestInput{})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.IsError)
	assert.Contains(t, getResultText(result), "manifest_name parameter is required")
}

func TestHandleListVulnerabilitiesInManifest_ManifestNotFound(t *testing.T) {
	spdxClient := kubescapefake.NewClientset()
	tool := NewKubescapeToolWithClients(nil, spdxClient.SpdxV1beta1())

	result, _, err := tool.HandleListVulnerabilitiesInManifest(context.Background(), listVulnerabilitiesInManifestInput{
		ManifestName: "nonexistent",
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.IsError)
}

func TestHandleGetVulnerabilityDetails_Success(t *testing.T) {
	spdxClient := kubescapefake.NewClientset(
		&v1beta1.VulnerabilityManifest{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-manifest",
				Namespace: "kubescape",
			},
			Spec: v1beta1.VulnerabilityManifestSpec{
				Payload: v1beta1.GrypeDocument{
					Matches: []v1beta1.Match{
						{
							Vulnerability: v1beta1.Vulnerability{
								VulnerabilityMetadata: v1beta1.VulnerabilityMetadata{
									ID:          "CVE-2021-1234",
									Severity:    "Critical",
									Description: "Test vulnerability",
								},
								Fix: v1beta1.Fix{
									State:    "fixed",
									Versions: []string{"1.2.3"},
								},
							},
						},
					},
				},
			},
		},
	)

	tool := NewKubescapeToolWithClients(nil, spdxClient.SpdxV1beta1())

	// The handler returns the matches as its typed Out and leaves the tool result
	// for the SDK to synthesise. For an array Out the SDK serialises the value
	// into a TextContent block itself, so setting Content in the handler as well
	// would emit the list twice.
	result, matches, err := tool.HandleGetVulnerabilityDetails(context.Background(), getVulnerabilityDetailsInput{
		ManifestName: "test-manifest",
		CveID:        "CVE-2021-1234",
	})
	require.NoError(t, err)
	assert.Nil(t, result, "handler should leave result to the SDK for an array Out")

	require.Len(t, matches, 1)
	assert.Equal(t, "CVE-2021-1234", matches[0].Vulnerability.ID)
}

func TestHandleGetVulnerabilityDetails_MissingManifestName(t *testing.T) {
	spdxClient := kubescapefake.NewClientset()
	tool := NewKubescapeToolWithClients(nil, spdxClient.SpdxV1beta1())

	result, _, err := tool.HandleGetVulnerabilityDetails(context.Background(), getVulnerabilityDetailsInput{
		CveID: "CVE-2021-1234",
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.IsError)
	assert.Contains(t, getResultText(result), "manifest_name parameter is required")
}

func TestHandleGetVulnerabilityDetails_MissingCveId(t *testing.T) {
	spdxClient := kubescapefake.NewClientset()
	tool := NewKubescapeToolWithClients(nil, spdxClient.SpdxV1beta1())

	result, _, err := tool.HandleGetVulnerabilityDetails(context.Background(), getVulnerabilityDetailsInput{
		ManifestName: "test-manifest",
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.IsError)
	assert.Contains(t, getResultText(result), "cve_id parameter is required")
}

func TestHandleGetVulnerabilityDetails_CveNotFound(t *testing.T) {
	spdxClient := kubescapefake.NewClientset(
		&v1beta1.VulnerabilityManifest{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-manifest",
				Namespace: "kubescape",
			},
			Spec: v1beta1.VulnerabilityManifestSpec{
				Payload: v1beta1.GrypeDocument{
					Matches: []v1beta1.Match{},
				},
			},
		},
	)

	tool := NewKubescapeToolWithClients(nil, spdxClient.SpdxV1beta1())

	result, _, err := tool.HandleGetVulnerabilityDetails(context.Background(), getVulnerabilityDetailsInput{
		ManifestName: "test-manifest",
		CveID:        "CVE-2021-1234",
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.IsError)
	assert.Contains(t, getResultText(result), "CVE CVE-2021-1234 not found")
}

func TestHandleListConfigurationScans_Success(t *testing.T) {
	spdxClient := kubescapefake.NewClientset(
		&v1beta1.WorkloadConfigurationScan{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "scan-1",
				Namespace: "default",
			},
		},
		&v1beta1.WorkloadConfigurationScan{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "scan-2",
				Namespace: "kubescape",
			},
		},
	)

	tool := NewKubescapeToolWithClients(nil, spdxClient.SpdxV1beta1())

	result, _, err := tool.HandleListConfigurationScans(context.Background(), listConfigurationScansInput{})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.False(t, result.IsError)

	var response listConfigurationScansOutput
	err = json.Unmarshal([]byte(getResultText(result)), &response)
	require.NoError(t, err)

	assert.Equal(t, 2, response.TotalCount)
}

func TestHandleListConfigurationScans_FilterByNamespace(t *testing.T) {
	spdxClient := kubescapefake.NewClientset(
		&v1beta1.WorkloadConfigurationScan{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "scan-1",
				Namespace: "default",
			},
		},
	)

	tool := NewKubescapeToolWithClients(nil, spdxClient.SpdxV1beta1())

	result, _, err := tool.HandleListConfigurationScans(context.Background(), listConfigurationScansInput{
		Namespace: "default",
	})
	require.NoError(t, err)
	require.NotNil(t, result)

	var response listConfigurationScansOutput
	err = json.Unmarshal([]byte(getResultText(result)), &response)
	require.NoError(t, err)

	assert.Equal(t, 1, response.TotalCount)
}

func TestHandleListConfigurationScans_EmptyResults(t *testing.T) {
	spdxClient := kubescapefake.NewClientset()
	tool := NewKubescapeToolWithClients(nil, spdxClient.SpdxV1beta1())

	result, _, err := tool.HandleListConfigurationScans(context.Background(), listConfigurationScansInput{})
	require.NoError(t, err)
	require.NotNil(t, result)

	var response listConfigurationScansOutput
	err = json.Unmarshal([]byte(getResultText(result)), &response)
	require.NoError(t, err)

	assert.Equal(t, 0, response.TotalCount)
}

func TestHandleGetConfigurationScan_Success(t *testing.T) {
	spdxClient := kubescapefake.NewClientset(
		&v1beta1.WorkloadConfigurationScan{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-scan",
				Namespace: "kubescape",
			},
		},
	)

	tool := NewKubescapeToolWithClients(nil, spdxClient.SpdxV1beta1())

	result, _, err := tool.HandleGetConfigurationScan(context.Background(), getConfigurationScanInput{
		ManifestName: "test-scan",
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.False(t, result.IsError)
}

func TestHandleGetConfigurationScan_MissingManifestName(t *testing.T) {
	spdxClient := kubescapefake.NewClientset()
	tool := NewKubescapeToolWithClients(nil, spdxClient.SpdxV1beta1())

	result, _, err := tool.HandleGetConfigurationScan(context.Background(), getConfigurationScanInput{})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.IsError)
	assert.Contains(t, getResultText(result), "manifest_name parameter is required")
}

func TestHandleGetConfigurationScan_NotFound(t *testing.T) {
	spdxClient := kubescapefake.NewClientset()
	tool := NewKubescapeToolWithClients(nil, spdxClient.SpdxV1beta1())

	result, _, err := tool.HandleGetConfigurationScan(context.Background(), getConfigurationScanInput{
		ManifestName: "nonexistent",
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.IsError)
}

func TestTruncateString(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		maxLen   int
		expected string
	}{
		{"short string", "hello", 10, "hello"},
		{"exact length", "hello", 5, "hello"},
		{"truncated", "hello world", 5, "hello..."},
		{"empty string", "", 10, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := truncateString(tt.input, tt.maxLen)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestNilArgumentsHandling(t *testing.T) {
	spdxClient := kubescapefake.NewClientset()
	tool := NewKubescapeToolWithClients(nil, spdxClient.SpdxV1beta1())

	// Empty input should use defaults
	result, _, err := tool.HandleListVulnerabilityManifests(context.Background(), listVulnerabilityManifestsInput{})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.False(t, result.IsError)
}

// Tests for ApplicationProfile handlers

func TestHandleListApplicationProfiles_Success(t *testing.T) {
	spdxClient := kubescapefake.NewClientset(
		&v1beta1.ApplicationProfile{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "profile-1",
				Namespace: "default",
			},
			Spec: v1beta1.ApplicationProfileSpec{
				Containers: []v1beta1.ApplicationProfileContainer{
					{
						Name: "container-1",
						Execs: []v1beta1.ExecCalls{
							{Path: "/bin/bash"},
						},
						Opens: []v1beta1.OpenCalls{
							{Path: "/etc/passwd"},
						},
						Syscalls: []string{"read", "write"},
					},
				},
			},
		},
		&v1beta1.ApplicationProfile{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "profile-2",
				Namespace: "kubescape",
			},
		},
	)

	tool := NewKubescapeToolWithClients(nil, spdxClient.SpdxV1beta1())

	result, _, err := tool.HandleListApplicationProfiles(context.Background(), listApplicationProfilesInput{})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.False(t, result.IsError)

	var response listApplicationProfilesOutput
	err = json.Unmarshal([]byte(getResultText(result)), &response)
	require.NoError(t, err)

	assert.Equal(t, 2, response.TotalCount)
	assert.Contains(t, response.Description, "ApplicationProfiles capture runtime behavior")
	assert.Len(t, response.ApplicationProfiles, 2)
}

func TestHandleListApplicationProfiles_FilterByNamespace(t *testing.T) {
	spdxClient := kubescapefake.NewClientset(
		&v1beta1.ApplicationProfile{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "profile-1",
				Namespace: "default",
			},
		},
	)

	tool := NewKubescapeToolWithClients(nil, spdxClient.SpdxV1beta1())

	result, _, err := tool.HandleListApplicationProfiles(context.Background(), listApplicationProfilesInput{
		Namespace: "default",
	})
	require.NoError(t, err)
	require.NotNil(t, result)

	var response listApplicationProfilesOutput
	err = json.Unmarshal([]byte(getResultText(result)), &response)
	require.NoError(t, err)

	assert.Equal(t, 1, response.TotalCount)
}

func TestHandleListApplicationProfiles_EmptyResults(t *testing.T) {
	spdxClient := kubescapefake.NewClientset()
	tool := NewKubescapeToolWithClients(nil, spdxClient.SpdxV1beta1())

	result, _, err := tool.HandleListApplicationProfiles(context.Background(), listApplicationProfilesInput{})
	require.NoError(t, err)
	require.NotNil(t, result)

	var response listApplicationProfilesOutput
	err = json.Unmarshal([]byte(getResultText(result)), &response)
	require.NoError(t, err)

	assert.Equal(t, 0, response.TotalCount)
}

func TestHandleListApplicationProfiles_InitError(t *testing.T) {
	tool := NewKubescapeToolWithError(errors.New("failed to connect"))

	result, _, err := tool.HandleListApplicationProfiles(context.Background(), listApplicationProfilesInput{})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.IsError)
}

func TestHandleGetApplicationProfile_Success(t *testing.T) {
	spdxClient := kubescapefake.NewClientset(
		&v1beta1.ApplicationProfile{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-profile",
				Namespace: "default",
			},
			Spec: v1beta1.ApplicationProfileSpec{
				Containers: []v1beta1.ApplicationProfileContainer{
					{
						Name: "container-1",
						Execs: []v1beta1.ExecCalls{
							{Path: "/bin/bash"},
						},
						Opens: []v1beta1.OpenCalls{
							{Path: "/etc/passwd"},
						},
						Syscalls:     []string{"read", "write"},
						Capabilities: []string{"NET_ADMIN"},
					},
				},
			},
		},
	)

	tool := NewKubescapeToolWithClients(nil, spdxClient.SpdxV1beta1())

	result, _, err := tool.HandleGetApplicationProfile(context.Background(), getApplicationProfileInput{
		Namespace: "default",
		Name:      "test-profile",
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.False(t, result.IsError)

	var response getApplicationProfileOutput
	err = json.Unmarshal([]byte(getResultText(result)), &response)
	require.NoError(t, err)

	assert.Equal(t, "default", response.Namespace)
	assert.Equal(t, "test-profile", response.Name)
	assert.Contains(t, response.Description, "ApplicationProfile shows what the workload containers actually execute")
}

func TestHandleGetApplicationProfile_MissingName(t *testing.T) {
	spdxClient := kubescapefake.NewClientset()
	tool := NewKubescapeToolWithClients(nil, spdxClient.SpdxV1beta1())

	result, _, err := tool.HandleGetApplicationProfile(context.Background(), getApplicationProfileInput{
		Namespace: "default",
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.IsError)
	assert.Contains(t, getResultText(result), "name parameter is required")
}

func TestHandleGetApplicationProfile_MissingNamespace(t *testing.T) {
	spdxClient := kubescapefake.NewClientset()
	tool := NewKubescapeToolWithClients(nil, spdxClient.SpdxV1beta1())

	result, _, err := tool.HandleGetApplicationProfile(context.Background(), getApplicationProfileInput{
		Name: "test-profile",
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.IsError)
	assert.Contains(t, getResultText(result), "namespace parameter is required")
}

func TestHandleGetApplicationProfile_NotFound(t *testing.T) {
	spdxClient := kubescapefake.NewClientset()
	tool := NewKubescapeToolWithClients(nil, spdxClient.SpdxV1beta1())

	result, _, err := tool.HandleGetApplicationProfile(context.Background(), getApplicationProfileInput{
		Namespace: "default",
		Name:      "nonexistent",
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.IsError)
}

// Tests for NetworkNeighborhood handlers

func TestHandleListNetworkNeighborhoods_Success(t *testing.T) {
	spdxClient := kubescapefake.NewClientset(
		&v1beta1.NetworkNeighborhood{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "nn-1",
				Namespace: "default",
			},
			Spec: v1beta1.NetworkNeighborhoodSpec{
				Containers: []v1beta1.NetworkNeighborhoodContainer{
					{
						Name: "container-1",
						Ingress: []v1beta1.NetworkNeighbor{
							{Identifier: "pod-1", Type: "internal"},
						},
						Egress: []v1beta1.NetworkNeighbor{
							{Identifier: "api.example.com", Type: "external", DNS: "api.example.com"},
						},
					},
				},
			},
		},
		&v1beta1.NetworkNeighborhood{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "nn-2",
				Namespace: "kubescape",
			},
		},
	)

	tool := NewKubescapeToolWithClients(nil, spdxClient.SpdxV1beta1())

	result, _, err := tool.HandleListNetworkNeighborhoods(context.Background(), listNetworkNeighborhoodsInput{})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.False(t, result.IsError)

	var response listNetworkNeighborhoodsOutput
	err = json.Unmarshal([]byte(getResultText(result)), &response)
	require.NoError(t, err)

	assert.Equal(t, 2, response.TotalCount)
	assert.Contains(t, response.Description, "NetworkNeighborhoods capture actual network communication patterns")
	assert.Len(t, response.NetworkNeighborhoods, 2)
}

func TestHandleListNetworkNeighborhoods_FilterByNamespace(t *testing.T) {
	spdxClient := kubescapefake.NewClientset(
		&v1beta1.NetworkNeighborhood{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "nn-1",
				Namespace: "default",
			},
		},
	)

	tool := NewKubescapeToolWithClients(nil, spdxClient.SpdxV1beta1())

	result, _, err := tool.HandleListNetworkNeighborhoods(context.Background(), listNetworkNeighborhoodsInput{
		Namespace: "default",
	})
	require.NoError(t, err)
	require.NotNil(t, result)

	var response listNetworkNeighborhoodsOutput
	err = json.Unmarshal([]byte(getResultText(result)), &response)
	require.NoError(t, err)

	assert.Equal(t, 1, response.TotalCount)
}

func TestHandleListNetworkNeighborhoods_EmptyResults(t *testing.T) {
	spdxClient := kubescapefake.NewClientset()
	tool := NewKubescapeToolWithClients(nil, spdxClient.SpdxV1beta1())

	result, _, err := tool.HandleListNetworkNeighborhoods(context.Background(), listNetworkNeighborhoodsInput{})
	require.NoError(t, err)
	require.NotNil(t, result)

	var response listNetworkNeighborhoodsOutput
	err = json.Unmarshal([]byte(getResultText(result)), &response)
	require.NoError(t, err)

	assert.Equal(t, 0, response.TotalCount)
}

func TestHandleListNetworkNeighborhoods_InitError(t *testing.T) {
	tool := NewKubescapeToolWithError(errors.New("failed to connect"))

	result, _, err := tool.HandleListNetworkNeighborhoods(context.Background(), listNetworkNeighborhoodsInput{})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.IsError)
}

func TestHandleGetNetworkNeighborhood_Success(t *testing.T) {
	spdxClient := kubescapefake.NewClientset(
		&v1beta1.NetworkNeighborhood{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-nn",
				Namespace: "default",
			},
			Spec: v1beta1.NetworkNeighborhoodSpec{
				Containers: []v1beta1.NetworkNeighborhoodContainer{
					{
						Name: "container-1",
						Ingress: []v1beta1.NetworkNeighbor{
							{Identifier: "pod-1", Type: "internal"},
						},
						Egress: []v1beta1.NetworkNeighbor{
							{Identifier: "api.example.com", Type: "external", DNS: "api.example.com"},
						},
					},
				},
			},
		},
	)

	tool := NewKubescapeToolWithClients(nil, spdxClient.SpdxV1beta1())

	result, _, err := tool.HandleGetNetworkNeighborhood(context.Background(), getNetworkNeighborhoodInput{
		Namespace: "default",
		Name:      "test-nn",
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.False(t, result.IsError)

	var response getNetworkNeighborhoodOutput
	err = json.Unmarshal([]byte(getResultText(result)), &response)
	require.NoError(t, err)

	assert.Equal(t, "default", response.Namespace)
	assert.Equal(t, "test-nn", response.Name)
	assert.Contains(t, response.Description, "NetworkNeighborhood shows actual network connections")
}

func TestHandleGetNetworkNeighborhood_MissingName(t *testing.T) {
	spdxClient := kubescapefake.NewClientset()
	tool := NewKubescapeToolWithClients(nil, spdxClient.SpdxV1beta1())

	result, _, err := tool.HandleGetNetworkNeighborhood(context.Background(), getNetworkNeighborhoodInput{
		Namespace: "default",
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.IsError)
	assert.Contains(t, getResultText(result), "name parameter is required")
}

func TestHandleGetNetworkNeighborhood_MissingNamespace(t *testing.T) {
	spdxClient := kubescapefake.NewClientset()
	tool := NewKubescapeToolWithClients(nil, spdxClient.SpdxV1beta1())

	result, _, err := tool.HandleGetNetworkNeighborhood(context.Background(), getNetworkNeighborhoodInput{
		Name: "test-nn",
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.IsError)
	assert.Contains(t, getResultText(result), "namespace parameter is required")
}

func TestHandleGetNetworkNeighborhood_NotFound(t *testing.T) {
	spdxClient := kubescapefake.NewClientset()
	tool := NewKubescapeToolWithClients(nil, spdxClient.SpdxV1beta1())

	result, _, err := tool.HandleGetNetworkNeighborhood(context.Background(), getNetworkNeighborhoodInput{
		Namespace: "default",
		Name:      "nonexistent",
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.IsError)
}

// NOTE: SBOM tests are disabled as SBOM tools are too large for LLM context windows.
// The handlers still exist in the code but are not registered or exported.
//
// Tests for SBOM handlers - DISABLED
//
// func TestHandleListSBOMs_Success(t *testing.T) { ... }
// func TestHandleListSBOMs_FilterByNamespace(t *testing.T) { ... }
// func TestHandleListSBOMs_EmptyResults(t *testing.T) { ... }
// func TestHandleListSBOMs_InitError(t *testing.T) { ... }
// func TestHandleGetSBOM_Success(t *testing.T) { ... }
// func TestHandleGetSBOM_MissingName(t *testing.T) { ... }
// func TestHandleGetSBOM_MissingNamespace(t *testing.T) { ... }
// func TestHandleGetSBOM_NotFound(t *testing.T) { ... }

// TestHandleListVulnerabilityManifests_OmitsVulnerabilityCount is the regression
// test for the bug behind PR #76. The aggregated API strips spec.payload.matches
// on LIST and serves it only on GET, so len(Matches) was 0 for every manifest in
// every cluster: the tool reported "vulnerability_count": 0 for images with
// hundreds of CVEs, and an agent reading that data correctly concluded the
// cluster was clean. The count must not appear in the list response at all - an
// absent field cannot be mistaken for a measured zero.
func TestHandleListVulnerabilityManifests_OmitsVulnerabilityCount(t *testing.T) {
	// Matches is nil here exactly as the aggregated API returns it on LIST, even
	// though this image really does have CVEs.
	spdxClient := kubescapefake.NewClientset(
		&v1beta1.VulnerabilityManifest{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "docker.io-library-nginx-1.14.0-e34030",
				Namespace: "kubescape",
				Annotations: map[string]string{
					"kubescape.io/image-tag": "docker.io/library/nginx:1.14.0",
				},
			},
		},
	)

	tool := NewKubescapeToolWithClients(nil, spdxClient.SpdxV1beta1())

	result, _, err := tool.HandleListVulnerabilityManifests(context.Background(), listVulnerabilityManifestsInput{})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.False(t, result.IsError)

	// The field must be absent, not present-and-zero: a zero reads as "measured
	// clean", which is the failure mode this guards.
	assert.NotContains(t, getResultText(result), "vulnerability_count",
		"list response must not report a count the aggregated API cannot supply")

	// The useful metadata is still there.
	assert.Contains(t, getResultText(result), "nginx:1.14.0")
}

// ---------------------------------------------------------------------------
// PR-1 regression tests (P1: aggregated-API availability, P2: pod selectors,
// P3: client-side level filter)
// ---------------------------------------------------------------------------

// realWorldKubescapePods returns pods labelled the way the kubescape-operator
// Helm chart actually labels them: every pod carries the chart-wide
// app.kubernetes.io/name=kubescape-operator label, and per-component identity
// lives in the plain `app` label.
func realWorldKubescapePods() []runtime.Object {
	pod := func(name, app string, phase corev1.PodPhase) *corev1.Pod {
		return &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: "kubescape",
				Labels: map[string]string{
					"app.kubernetes.io/name": "kubescape-operator",
					"app":                    app,
				},
			},
			Status: corev1.PodStatus{Phase: phase},
		}
	}
	return []runtime.Object{
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "kubescape"}},
		pod("operator-5d854fdc8f-j75sb", "operator", corev1.PodRunning),
		pod("storage-5ff6f76c7f-ngb2d", "storage", corev1.PodRunning),
		pod("node-agent-7vfq2", "node-agent", corev1.PodRunning),
		pod("kubevuln-c6bb59f9c-v5xlq", "kubevuln", corev1.PodRunning),
	}
}

func allSpdxResources() []runtime.Object {
	return []runtime.Object{
		&v1beta1.VulnerabilityManifest{ObjectMeta: metav1.ObjectMeta{Name: "vm", Namespace: "kubescape"}},
		&v1beta1.WorkloadConfigurationScan{ObjectMeta: metav1.ObjectMeta{Name: "cs", Namespace: "kubescape"}},
		&v1beta1.ApplicationProfile{ObjectMeta: metav1.ObjectMeta{Name: "ap", Namespace: "kubescape"}},
		&v1beta1.NetworkNeighborhood{ObjectMeta: metav1.ObjectMeta{Name: "nn", Namespace: "kubescape"}},
	}
}

func healthOf(t *testing.T, tool *KubescapeTool) HealthCheckResult {
	t.Helper()
	result, _, err := tool.HandleCheckHealth(context.Background(), checkHealthInput{})
	require.NoError(t, err)
	require.NotNil(t, result)
	var health HealthCheckResult
	require.NoError(t, json.Unmarshal([]byte(getResultText(result)), &health))
	return health
}

// P1: a working standalone install has NO CRDs for the spdx group -- the
// resources are served by an aggregated API server. Health must report healthy.
func TestHandleCheckHealth_HealthyWithAggregatedAPIAndNoCRDs(t *testing.T) {
	//nolint:staticcheck // NewSimpleClientset is deprecated but NewClientset requires generated apply configs
	k8sClient := kubefake.NewSimpleClientset(realWorldKubescapePods()...)
	spdxClient := kubescapefake.NewClientset(allSpdxResources()...)

	tool := NewKubescapeToolWithClients(k8sClient, spdxClient.SpdxV1beta1())

	health := healthOf(t, tool)

	assert.True(t, health.Healthy, "healthy should be true on a working install with no CRDs; recommendations=%v", health.Recommendations)
	assert.Equal(t, "ok", health.Checks["vulnerability_crd"].Status)
	assert.Equal(t, "ok", health.Checks["configuration_crd"].Status)
	assert.Equal(t, "ok", health.Checks["application_profiles_crd"].Status)
	assert.Equal(t, "ok", health.Checks["network_neighborhoods_crd"].Status)
	assert.Equal(t, "Kubescape is fully operational", health.Summary)
}

// P1: when the aggregated API is down, the storage-backed checks must fail and
// say so in the product's own vocabulary -- not "CRD not installed".
func TestHandleCheckHealth_AggregatedAPIUnavailable(t *testing.T) {
	//nolint:staticcheck // NewSimpleClientset is deprecated but NewClientset requires generated apply configs
	k8sClient := kubefake.NewSimpleClientset(realWorldKubescapePods()...)
	spdxClient := kubescapefake.NewClientset()
	spdxClient.PrependReactor("list", "*", func(action k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, k8serrors.NewServiceUnavailable("no response from storage service")
	})

	tool := NewKubescapeToolWithClients(k8sClient, spdxClient.SpdxV1beta1())

	health := healthOf(t, tool)

	assert.False(t, health.Healthy)
	assert.Equal(t, "error", health.Checks["vulnerability_crd"].Status)
	assert.Equal(t, "error", health.Checks["configuration_crd"].Status)
	// runtime observability stays a warning, as before
	assert.Equal(t, "warning", health.Checks["application_profiles_crd"].Status)
	assert.Equal(t, "warning", health.Checks["network_neighborhoods_crd"].Status)
	assert.NotContains(t, health.Checks["vulnerability_crd"].Message, "CRD")
	assert.Contains(t, health.Checks["vulnerability_crd"].Message, "not available")
}

// P2: operator_pods must count the operator Deployment, not every pod the chart
// created.
func TestHandleCheckHealth_OperatorPodsCountsOnlyOperator(t *testing.T) {
	//nolint:staticcheck // NewSimpleClientset is deprecated but NewClientset requires generated apply configs
	k8sClient := kubefake.NewSimpleClientset(realWorldKubescapePods()...)
	spdxClient := kubescapefake.NewClientset(allSpdxResources()...)

	tool := NewKubescapeToolWithClients(k8sClient, spdxClient.SpdxV1beta1())

	health := healthOf(t, tool)

	assert.Equal(t, "ok", health.Checks["operator_pods"].Status)
	assert.Equal(t, "1/1 pods running", health.Checks["operator_pods"].Message)
}

// P2: the storage pod is found via app=storage, not app.kubernetes.io/name=storage.
func TestHandleCheckHealth_StoragePodFound(t *testing.T) {
	//nolint:staticcheck // NewSimpleClientset is deprecated but NewClientset requires generated apply configs
	k8sClient := kubefake.NewSimpleClientset(realWorldKubescapePods()...)
	spdxClient := kubescapefake.NewClientset(allSpdxResources()...)

	tool := NewKubescapeToolWithClients(k8sClient, spdxClient.SpdxV1beta1())

	health := healthOf(t, tool)

	assert.Equal(t, "ok", health.Checks["storage_pods"].Status)
	assert.Equal(t, "1/1 pods running", health.Checks["storage_pods"].Message)
}

// P2: nothing works without storage, so its absence is an error, not a warning.
func TestHandleCheckHealth_StoragePodMissingIsError(t *testing.T) {
	objs := []runtime.Object{}
	for _, o := range realWorldKubescapePods() {
		if pod, ok := o.(*corev1.Pod); ok && pod.Labels["app"] == "storage" {
			continue
		}
		objs = append(objs, o)
	}
	//nolint:staticcheck // NewSimpleClientset is deprecated but NewClientset requires generated apply configs
	k8sClient := kubefake.NewSimpleClientset(objs...)
	spdxClient := kubescapefake.NewClientset(allSpdxResources()...)

	tool := NewKubescapeToolWithClients(k8sClient, spdxClient.SpdxV1beta1())

	health := healthOf(t, tool)

	assert.False(t, health.Healthy)
	assert.Equal(t, "error", health.Checks["storage_pods"].Status)
	assert.Contains(t, health.Checks["storage_pods"].Message, "No storage pods found")
}

// P3: the advertised `level` filter must actually filter. The storage API
// ignores labelSelector, so filtering has to happen client-side.
func TestHandleListVulnerabilityManifests_LevelFilter(t *testing.T) {
	imageLevel := &v1beta1.VulnerabilityManifest{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "docker.io-library-nginx-1.14.0-e34030",
			Namespace: "kubescape",
			Labels:    map[string]string{"kubescape.io/context": "non-filtered"},
		},
	}
	workloadLevel := &v1beta1.VulnerabilityManifest{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "replicaset-chatty-client-nginx-1234",
			Namespace: "kubescape",
			Labels:    map[string]string{"kubescape.io/context": "filtered"},
			Annotations: map[string]string{
				helpersv1.WlidMetadataKey: "wlid://cluster-test/namespace-default/deployment-chatty-client",
			},
		},
	}

	tests := []struct {
		name      string
		in        listVulnerabilityManifestsInput
		wantNames []string
	}{
		{"no level returns both", listVulnerabilityManifestsInput{}, []string{imageLevel.Name, workloadLevel.Name}},
		{"level=both returns both", listVulnerabilityManifestsInput{Level: "both"}, []string{imageLevel.Name, workloadLevel.Name}},
		{"level=image returns image-level only", listVulnerabilityManifestsInput{Level: "image"}, []string{imageLevel.Name}},
		{"level=workload returns workload-level only", listVulnerabilityManifestsInput{Level: "workload"}, []string{workloadLevel.Name}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			spdxClient := kubescapefake.NewClientset(imageLevel, workloadLevel)

			// Storage servers before v0.0.305 IGNORE labelSelector on list --
			// they return every object no matter what is asked for -- and the
			// kubescape-operator chart still pins v0.0.298. The fake clientset
			// honours selectors, which would let a server-side filter pass this
			// test while returning unfiltered results against the storage
			// version actually deployed. This reactor reproduces that server's
			// behaviour, and records what the provider asked for.
			var sentSelector string
			spdxClient.PrependReactor("list", "vulnerabilitymanifests", func(action k8stesting.Action) (bool, runtime.Object, error) {
				sentSelector = action.(k8stesting.ListAction).GetListRestrictions().Labels.String()
				return true, &v1beta1.VulnerabilityManifestList{
					Items: []v1beta1.VulnerabilityManifest{*imageLevel, *workloadLevel},
				}, nil
			})

			//nolint:staticcheck // NewSimpleClientset is deprecated but NewClientset requires generated apply configs
			tool := NewKubescapeToolWithClients(kubefake.NewSimpleClientset(), spdxClient.SpdxV1beta1())

			result, _, err := tool.HandleListVulnerabilityManifests(context.Background(), tt.in)
			require.NoError(t, err)

			assert.Empty(t, sentSelector, "must not rely on a server-side label selector: the storage API ignores it")

			var payload struct {
				Manifests []struct {
					ManifestName string `json:"manifest_name"`
				} `json:"vulnerability_manifests"`
				TotalCount int `json:"total_count"`
			}
			require.NoError(t, json.Unmarshal([]byte(getResultText(result)), &payload))

			got := []string{}
			for _, m := range payload.Manifests {
				got = append(got, m.ManifestName)
			}
			assert.ElementsMatch(t, tt.wantNames, got)
			assert.Equal(t, len(tt.wantNames), payload.TotalCount)
		})
	}
}

// P3: an unrecognised level is a caller error, not a silent "both".
func TestHandleListVulnerabilityManifests_InvalidLevel(t *testing.T) {
	spdxClient := kubescapefake.NewClientset()
	//nolint:staticcheck // NewSimpleClientset is deprecated but NewClientset requires generated apply configs
	tool := NewKubescapeToolWithClients(kubefake.NewSimpleClientset(), spdxClient.SpdxV1beta1())

	result, _, err := tool.HandleListVulnerabilityManifests(context.Background(), listVulnerabilityManifestsInput{Level: "bogus"})
	require.NoError(t, err)
	assert.Contains(t, getResultText(result), "level")
	assert.True(t, result.IsError)
}
