package kubescape

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	"github.com/kubescape/storage/pkg/apis/softwarecomposition/v1beta1"
	kubescapefake "github.com/kubescape/storage/pkg/generated/clientset/versioned/fake"
	spdxv1beta1 "github.com/kubescape/storage/pkg/generated/clientset/versioned/typed/softwarecomposition/v1beta1"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apiextensionsfake "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset/fake"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kubefake "k8s.io/client-go/kubernetes/fake"
)

// Helper function to create a CallToolRequest with arguments
func makeRequest(args map[string]interface{}) mcp.CallToolRequest {
	request := mcp.CallToolRequest{}
	request.Params.Arguments = args
	return request
}

// Helper function to extract text content from MCP result
func getResultText(result *mcp.CallToolResult) string {
	if result == nil || len(result.Content) == 0 {
		return ""
	}
	if textContent, ok := result.Content[0].(mcp.TextContent); ok {
		return textContent.Text
	}
	return ""
}

func TestRegisterTools(t *testing.T) {
	s := server.NewMCPServer("test", "1.0.0")

	// Should not panic
	assert.NotPanics(t, func() {
		RegisterTools(s, "", false)
	})

	// Verify tools are registered by checking the server has tools
	// NOTE: SBOM tools are disabled (too large for LLM context), so we expect 12 tools
	tools := s.ListTools()
	assert.Len(t, tools, 12)

	expectedTools := map[string]bool{
		"kubescape_check_health":                 false,
		"kubescape_vulnerability_overview":       false,
		"kubescape_list_vulnerable_workloads":    false,
		"kubescape_list_vulnerability_manifests": false,
		"kubescape_list_vulnerabilities":         false,
		"kubescape_get_vulnerability_details":    false,
		"kubescape_list_configuration_scans":     false,
		"kubescape_get_configuration_scan":       false,
		"kubescape_list_application_profiles":    false,
		"kubescape_get_application_profile":      false,
		"kubescape_list_network_neighborhoods":   false,
		"kubescape_get_network_neighborhood":     false,
		// NOTE: SBOM tools disabled - too large for LLM context
		// "kubescape_list_sboms":                   false,
		// "kubescape_get_sbom":                     false,
	}

	for name := range tools {
		if _, exists := expectedTools[name]; exists {
			expectedTools[name] = true
		}
	}

	for name, found := range expectedTools {
		assert.True(t, found, "Tool %s not found", name)
	}
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
				Labels:    map[string]string{"app.kubernetes.io/name": "kubescape-operator"},
			},
			Status: corev1.PodStatus{Phase: corev1.PodRunning},
		},
		// Storage pods
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "storage-123",
				Namespace: "kubescape",
				Labels:    map[string]string{"app.kubernetes.io/name": "storage"},
			},
			Status: corev1.PodStatus{Phase: corev1.PodRunning},
		},
	)

	//nolint:staticcheck // NewSimpleClientset is deprecated but NewClientset requires generated apply configs
	apiExtClient := apiextensionsfake.NewSimpleClientset(
		&apiextensionsv1.CustomResourceDefinition{
			ObjectMeta: metav1.ObjectMeta{Name: vulnerabilityManifestsCRD},
		},
		&apiextensionsv1.CustomResourceDefinition{
			ObjectMeta: metav1.ObjectMeta{Name: workloadConfigurationScansCRD},
		},
		&apiextensionsv1.CustomResourceDefinition{
			ObjectMeta: metav1.ObjectMeta{Name: applicationProfilesCRD},
		},
		&apiextensionsv1.CustomResourceDefinition{
			ObjectMeta: metav1.ObjectMeta{Name: networkNeighborhoodsCRD},
		},
		// NOTE: SBOM CRD check is disabled (SBOM tools are too large for LLM context)
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

	tool := NewKubescapeToolWithClients(k8sClient, apiExtClient, spdxClient.SpdxV1beta1())

	result, err := tool.HandleCheckHealth(context.Background(), makeRequest(nil))
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
	//nolint:staticcheck // NewSimpleClientset is deprecated but NewClientset requires generated apply configs
	apiExtClient := apiextensionsfake.NewSimpleClientset()
	spdxClient := kubescapefake.NewClientset()

	tool := NewKubescapeToolWithClients(k8sClient, apiExtClient, spdxClient.SpdxV1beta1())

	result, err := tool.HandleCheckHealth(context.Background(), makeRequest(nil))
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
	//nolint:staticcheck // NewSimpleClientset is deprecated but NewClientset requires generated apply configs
	apiExtClient := apiextensionsfake.NewSimpleClientset()
	spdxClient := kubescapefake.NewClientset()

	tool := NewKubescapeToolWithClients(k8sClient, apiExtClient, spdxClient.SpdxV1beta1())

	result, err := tool.HandleCheckHealth(context.Background(), makeRequest(nil))
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
				Labels:    map[string]string{"app.kubernetes.io/name": "kubescape-operator"},
			},
			Status: corev1.PodStatus{Phase: corev1.PodPending}, // Not running
		},
	)
	//nolint:staticcheck // NewSimpleClientset is deprecated but NewClientset requires generated apply configs
	apiExtClient := apiextensionsfake.NewSimpleClientset()
	spdxClient := kubescapefake.NewClientset()

	tool := NewKubescapeToolWithClients(k8sClient, apiExtClient, spdxClient.SpdxV1beta1())

	result, err := tool.HandleCheckHealth(context.Background(), makeRequest(nil))
	require.NoError(t, err)
	require.NotNil(t, result)

	var health HealthCheckResult
	err = json.Unmarshal([]byte(getResultText(result)), &health)
	require.NoError(t, err)

	assert.False(t, health.Healthy)
	assert.Equal(t, "warning", health.Checks["operator_pods"].Status)
	assert.Contains(t, health.Checks["operator_pods"].Message, "0/1 pods running")
}

func TestHandleCheckHealth_VulnerabilityCRDMissing(t *testing.T) {
	//nolint:staticcheck // NewSimpleClientset is deprecated but NewClientset requires generated apply configs
	k8sClient := kubefake.NewSimpleClientset(
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "kubescape"}},
	)
	//nolint:staticcheck // NewSimpleClientset is deprecated but NewClientset requires generated apply configs
	apiExtClient := apiextensionsfake.NewSimpleClientset() // No CRDs
	spdxClient := kubescapefake.NewClientset()

	tool := NewKubescapeToolWithClients(k8sClient, apiExtClient, spdxClient.SpdxV1beta1())

	result, err := tool.HandleCheckHealth(context.Background(), makeRequest(nil))
	require.NoError(t, err)
	require.NotNil(t, result)

	var health HealthCheckResult
	err = json.Unmarshal([]byte(getResultText(result)), &health)
	require.NoError(t, err)

	assert.False(t, health.Healthy)
	assert.Equal(t, "error", health.Checks["vulnerability_crd"].Status)
	assert.Contains(t, health.Checks["vulnerability_crd"].Message, "not installed")
}

func TestHandleCheckHealth_NoScanData(t *testing.T) {
	//nolint:staticcheck // NewSimpleClientset is deprecated but NewClientset requires generated apply configs
	k8sClient := kubefake.NewSimpleClientset(
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "kubescape"}},
	)
	//nolint:staticcheck // NewSimpleClientset is deprecated but NewClientset requires generated apply configs
	apiExtClient := apiextensionsfake.NewSimpleClientset(
		&apiextensionsv1.CustomResourceDefinition{
			ObjectMeta: metav1.ObjectMeta{Name: vulnerabilityManifestsCRD},
		},
		&apiextensionsv1.CustomResourceDefinition{
			ObjectMeta: metav1.ObjectMeta{Name: workloadConfigurationScansCRD},
		},
	)
	spdxClient := kubescapefake.NewClientset() // No vulnerability manifests

	tool := NewKubescapeToolWithClients(k8sClient, apiExtClient, spdxClient.SpdxV1beta1())

	result, err := tool.HandleCheckHealth(context.Background(), makeRequest(nil))
	require.NoError(t, err)
	require.NotNil(t, result)

	var health HealthCheckResult
	err = json.Unmarshal([]byte(getResultText(result)), &health)
	require.NoError(t, err)

	// Warning for no scan data
	assert.Equal(t, "warning", health.Checks["vulnerability_scan_data"].Status)
	assert.Contains(t, health.Checks["vulnerability_scan_data"].Message, "No vulnerability manifests found")
}

func TestHandleCheckHealth_RuntimeObservabilityCRDsMissing(t *testing.T) {
	//nolint:staticcheck // NewSimpleClientset is deprecated but NewClientset requires generated apply configs
	k8sClient := kubefake.NewSimpleClientset(
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "kubescape"}},
	)
	//nolint:staticcheck // NewSimpleClientset is deprecated but NewClientset requires generated apply configs
	apiExtClient := apiextensionsfake.NewSimpleClientset(
		&apiextensionsv1.CustomResourceDefinition{
			ObjectMeta: metav1.ObjectMeta{Name: vulnerabilityManifestsCRD},
		},
		&apiextensionsv1.CustomResourceDefinition{
			ObjectMeta: metav1.ObjectMeta{Name: workloadConfigurationScansCRD},
		},
		// No runtime observability CRDs (applicationprofiles, networkneighborhoods)
	)
	spdxClient := kubescapefake.NewClientset()

	tool := NewKubescapeToolWithClients(k8sClient, apiExtClient, spdxClient.SpdxV1beta1())

	result, err := tool.HandleCheckHealth(context.Background(), makeRequest(nil))
	require.NoError(t, err)
	require.NotNil(t, result)

	var health HealthCheckResult
	err = json.Unmarshal([]byte(getResultText(result)), &health)
	require.NoError(t, err)

	// Warning for missing runtime observability CRDs
	assert.Equal(t, "warning", health.Checks["application_profiles_crd"].Status)
	assert.Contains(t, health.Checks["application_profiles_crd"].Message, "not installed")
	assert.Equal(t, "warning", health.Checks["network_neighborhoods_crd"].Status)
	assert.Contains(t, health.Checks["network_neighborhoods_crd"].Message, "not installed")

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
	//nolint:staticcheck // NewSimpleClientset is deprecated but NewClientset requires generated apply configs
	apiExtClient := apiextensionsfake.NewSimpleClientset()
	spdxClient := kubescapefake.NewClientset()

	tool := NewKubescapeToolWithClients(k8sClient, apiExtClient, spdxClient.SpdxV1beta1())

	result, err := tool.HandleCheckHealth(context.Background(), makeRequest(map[string]interface{}{
		"namespace": "custom-ns",
	}))
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

	result, err := tool.HandleCheckHealth(context.Background(), makeRequest(nil))
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

	tool := NewKubescapeToolWithClients(nil, nil, spdxClient.SpdxV1beta1())

	result, err := tool.HandleListVulnerabilityManifests(context.Background(), makeRequest(nil))
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.False(t, result.IsError)

	var response map[string]interface{}
	err = json.Unmarshal([]byte(getResultText(result)), &response)
	require.NoError(t, err)

	assert.Equal(t, float64(2), response["total_count"])
	manifests := response["vulnerability_manifests"].([]interface{})
	assert.Len(t, manifests, 2)
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

	tool := NewKubescapeToolWithClients(nil, nil, spdxClient.SpdxV1beta1())

	result, err := tool.HandleListVulnerabilityManifests(context.Background(), makeRequest(map[string]interface{}{
		"namespace": "default",
	}))
	require.NoError(t, err)
	require.NotNil(t, result)

	var response map[string]interface{}
	err = json.Unmarshal([]byte(getResultText(result)), &response)
	require.NoError(t, err)

	assert.Equal(t, float64(1), response["total_count"])
}

func TestHandleListVulnerabilityManifests_EmptyResults(t *testing.T) {
	spdxClient := kubescapefake.NewClientset()
	tool := NewKubescapeToolWithClients(nil, nil, spdxClient.SpdxV1beta1())

	result, err := tool.HandleListVulnerabilityManifests(context.Background(), makeRequest(nil))
	require.NoError(t, err)
	require.NotNil(t, result)

	var response map[string]interface{}
	err = json.Unmarshal([]byte(getResultText(result)), &response)
	require.NoError(t, err)

	assert.Equal(t, float64(0), response["total_count"])
}

func TestHandleListVulnerabilityManifests_InitError(t *testing.T) {
	tool := NewKubescapeToolWithError(errors.New("failed to connect"))

	result, err := tool.HandleListVulnerabilityManifests(context.Background(), makeRequest(nil))
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

	tool := NewKubescapeToolWithClients(nil, nil, spdxClient.SpdxV1beta1())

	result, err := tool.HandleListVulnerabilitiesInManifest(context.Background(), makeRequest(map[string]interface{}{
		"manifest_name": "test-manifest",
	}))
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.False(t, result.IsError)

	var response map[string]interface{}
	err = json.Unmarshal([]byte(getResultText(result)), &response)
	require.NoError(t, err)

	assert.Equal(t, float64(2), response["total_count"])
	severitySummary := response["severity_summary"].(map[string]interface{})
	assert.Equal(t, float64(1), severitySummary["Critical"])
	assert.Equal(t, float64(1), severitySummary["High"])
}

func TestHandleListVulnerabilitiesInManifest_MissingManifestName(t *testing.T) {
	spdxClient := kubescapefake.NewClientset()
	tool := NewKubescapeToolWithClients(nil, nil, spdxClient.SpdxV1beta1())

	result, err := tool.HandleListVulnerabilitiesInManifest(context.Background(), makeRequest(nil))
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.IsError)
	assert.Contains(t, getResultText(result), "manifest_name parameter is required")
}

func TestHandleListVulnerabilitiesInManifest_ManifestNotFound(t *testing.T) {
	spdxClient := kubescapefake.NewClientset()
	tool := NewKubescapeToolWithClients(nil, nil, spdxClient.SpdxV1beta1())

	result, err := tool.HandleListVulnerabilitiesInManifest(context.Background(), makeRequest(map[string]interface{}{
		"manifest_name": "nonexistent",
	}))
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

	tool := NewKubescapeToolWithClients(nil, nil, spdxClient.SpdxV1beta1())

	result, err := tool.HandleGetVulnerabilityDetails(context.Background(), makeRequest(map[string]interface{}{
		"manifest_name": "test-manifest",
		"cve_id":        "CVE-2021-1234",
	}))
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.False(t, result.IsError)

	var matches []v1beta1.Match
	err = json.Unmarshal([]byte(getResultText(result)), &matches)
	require.NoError(t, err)

	assert.Len(t, matches, 1)
	assert.Equal(t, "CVE-2021-1234", matches[0].Vulnerability.ID)
}

func TestHandleGetVulnerabilityDetails_MissingManifestName(t *testing.T) {
	spdxClient := kubescapefake.NewClientset()
	tool := NewKubescapeToolWithClients(nil, nil, spdxClient.SpdxV1beta1())

	result, err := tool.HandleGetVulnerabilityDetails(context.Background(), makeRequest(map[string]interface{}{
		"cve_id": "CVE-2021-1234",
	}))
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.IsError)
	assert.Contains(t, getResultText(result), "manifest_name parameter is required")
}

func TestHandleGetVulnerabilityDetails_MissingCveId(t *testing.T) {
	spdxClient := kubescapefake.NewClientset()
	tool := NewKubescapeToolWithClients(nil, nil, spdxClient.SpdxV1beta1())

	result, err := tool.HandleGetVulnerabilityDetails(context.Background(), makeRequest(map[string]interface{}{
		"manifest_name": "test-manifest",
	}))
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

	tool := NewKubescapeToolWithClients(nil, nil, spdxClient.SpdxV1beta1())

	result, err := tool.HandleGetVulnerabilityDetails(context.Background(), makeRequest(map[string]interface{}{
		"manifest_name": "test-manifest",
		"cve_id":        "CVE-2021-1234",
	}))
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

	tool := NewKubescapeToolWithClients(nil, nil, spdxClient.SpdxV1beta1())

	result, err := tool.HandleListConfigurationScans(context.Background(), makeRequest(nil))
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.False(t, result.IsError)

	var response map[string]interface{}
	err = json.Unmarshal([]byte(getResultText(result)), &response)
	require.NoError(t, err)

	assert.Equal(t, float64(2), response["total_count"])
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

	tool := NewKubescapeToolWithClients(nil, nil, spdxClient.SpdxV1beta1())

	result, err := tool.HandleListConfigurationScans(context.Background(), makeRequest(map[string]interface{}{
		"namespace": "default",
	}))
	require.NoError(t, err)
	require.NotNil(t, result)

	var response map[string]interface{}
	err = json.Unmarshal([]byte(getResultText(result)), &response)
	require.NoError(t, err)

	assert.Equal(t, float64(1), response["total_count"])
}

func TestHandleListConfigurationScans_EmptyResults(t *testing.T) {
	spdxClient := kubescapefake.NewClientset()
	tool := NewKubescapeToolWithClients(nil, nil, spdxClient.SpdxV1beta1())

	result, err := tool.HandleListConfigurationScans(context.Background(), makeRequest(nil))
	require.NoError(t, err)
	require.NotNil(t, result)

	var response map[string]interface{}
	err = json.Unmarshal([]byte(getResultText(result)), &response)
	require.NoError(t, err)

	assert.Equal(t, float64(0), response["total_count"])
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

	tool := NewKubescapeToolWithClients(nil, nil, spdxClient.SpdxV1beta1())

	result, err := tool.HandleGetConfigurationScan(context.Background(), makeRequest(map[string]interface{}{
		"manifest_name": "test-scan",
	}))
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.False(t, result.IsError)
}

func TestHandleGetConfigurationScan_MissingManifestName(t *testing.T) {
	spdxClient := kubescapefake.NewClientset()
	tool := NewKubescapeToolWithClients(nil, nil, spdxClient.SpdxV1beta1())

	result, err := tool.HandleGetConfigurationScan(context.Background(), makeRequest(nil))
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.IsError)
	assert.Contains(t, getResultText(result), "manifest_name parameter is required")
}

func TestHandleGetConfigurationScan_NotFound(t *testing.T) {
	spdxClient := kubescapefake.NewClientset()
	tool := NewKubescapeToolWithClients(nil, nil, spdxClient.SpdxV1beta1())

	result, err := tool.HandleGetConfigurationScan(context.Background(), makeRequest(map[string]interface{}{
		"manifest_name": "nonexistent",
	}))
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
	tool := NewKubescapeToolWithClients(nil, nil, spdxClient.SpdxV1beta1())

	// Test with nil arguments map - should use defaults
	request := mcp.CallToolRequest{}
	request.Params.Arguments = nil

	result, err := tool.HandleListVulnerabilityManifests(context.Background(), request)
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

	tool := NewKubescapeToolWithClients(nil, nil, spdxClient.SpdxV1beta1())

	result, err := tool.HandleListApplicationProfiles(context.Background(), makeRequest(nil))
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.False(t, result.IsError)

	var response map[string]interface{}
	err = json.Unmarshal([]byte(getResultText(result)), &response)
	require.NoError(t, err)

	assert.Equal(t, float64(2), response["total_count"])
	assert.Contains(t, response["description"], "ApplicationProfiles capture runtime behavior")
	profiles := response["application_profiles"].([]interface{})
	assert.Len(t, profiles, 2)
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

	tool := NewKubescapeToolWithClients(nil, nil, spdxClient.SpdxV1beta1())

	result, err := tool.HandleListApplicationProfiles(context.Background(), makeRequest(map[string]interface{}{
		"namespace": "default",
	}))
	require.NoError(t, err)
	require.NotNil(t, result)

	var response map[string]interface{}
	err = json.Unmarshal([]byte(getResultText(result)), &response)
	require.NoError(t, err)

	assert.Equal(t, float64(1), response["total_count"])
}

func TestHandleListApplicationProfiles_EmptyResults(t *testing.T) {
	spdxClient := kubescapefake.NewClientset()
	tool := NewKubescapeToolWithClients(nil, nil, spdxClient.SpdxV1beta1())

	result, err := tool.HandleListApplicationProfiles(context.Background(), makeRequest(nil))
	require.NoError(t, err)
	require.NotNil(t, result)

	var response map[string]interface{}
	err = json.Unmarshal([]byte(getResultText(result)), &response)
	require.NoError(t, err)

	assert.Equal(t, float64(0), response["total_count"])
}

func TestHandleListApplicationProfiles_InitError(t *testing.T) {
	tool := NewKubescapeToolWithError(errors.New("failed to connect"))

	result, err := tool.HandleListApplicationProfiles(context.Background(), makeRequest(nil))
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

	tool := NewKubescapeToolWithClients(nil, nil, spdxClient.SpdxV1beta1())

	result, err := tool.HandleGetApplicationProfile(context.Background(), makeRequest(map[string]interface{}{
		"namespace": "default",
		"name":      "test-profile",
	}))
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.False(t, result.IsError)

	var response map[string]interface{}
	err = json.Unmarshal([]byte(getResultText(result)), &response)
	require.NoError(t, err)

	assert.Equal(t, "default", response["namespace"])
	assert.Equal(t, "test-profile", response["name"])
	assert.Contains(t, response["description"], "ApplicationProfile shows what the workload containers actually execute")
}

func TestHandleGetApplicationProfile_MissingName(t *testing.T) {
	spdxClient := kubescapefake.NewClientset()
	tool := NewKubescapeToolWithClients(nil, nil, spdxClient.SpdxV1beta1())

	result, err := tool.HandleGetApplicationProfile(context.Background(), makeRequest(map[string]interface{}{
		"namespace": "default",
	}))
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.IsError)
	assert.Contains(t, getResultText(result), "name parameter is required")
}

func TestHandleGetApplicationProfile_MissingNamespace(t *testing.T) {
	spdxClient := kubescapefake.NewClientset()
	tool := NewKubescapeToolWithClients(nil, nil, spdxClient.SpdxV1beta1())

	result, err := tool.HandleGetApplicationProfile(context.Background(), makeRequest(map[string]interface{}{
		"name": "test-profile",
	}))
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.IsError)
	assert.Contains(t, getResultText(result), "namespace parameter is required")
}

func TestHandleGetApplicationProfile_NotFound(t *testing.T) {
	spdxClient := kubescapefake.NewClientset()
	tool := NewKubescapeToolWithClients(nil, nil, spdxClient.SpdxV1beta1())

	result, err := tool.HandleGetApplicationProfile(context.Background(), makeRequest(map[string]interface{}{
		"namespace": "default",
		"name":      "nonexistent",
	}))
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

	tool := NewKubescapeToolWithClients(nil, nil, spdxClient.SpdxV1beta1())

	result, err := tool.HandleListNetworkNeighborhoods(context.Background(), makeRequest(nil))
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.False(t, result.IsError)

	var response map[string]interface{}
	err = json.Unmarshal([]byte(getResultText(result)), &response)
	require.NoError(t, err)

	assert.Equal(t, float64(2), response["total_count"])
	assert.Contains(t, response["description"], "NetworkNeighborhoods capture actual network communication patterns")
	neighborhoods := response["network_neighborhoods"].([]interface{})
	assert.Len(t, neighborhoods, 2)
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

	tool := NewKubescapeToolWithClients(nil, nil, spdxClient.SpdxV1beta1())

	result, err := tool.HandleListNetworkNeighborhoods(context.Background(), makeRequest(map[string]interface{}{
		"namespace": "default",
	}))
	require.NoError(t, err)
	require.NotNil(t, result)

	var response map[string]interface{}
	err = json.Unmarshal([]byte(getResultText(result)), &response)
	require.NoError(t, err)

	assert.Equal(t, float64(1), response["total_count"])
}

func TestHandleListNetworkNeighborhoods_EmptyResults(t *testing.T) {
	spdxClient := kubescapefake.NewClientset()
	tool := NewKubescapeToolWithClients(nil, nil, spdxClient.SpdxV1beta1())

	result, err := tool.HandleListNetworkNeighborhoods(context.Background(), makeRequest(nil))
	require.NoError(t, err)
	require.NotNil(t, result)

	var response map[string]interface{}
	err = json.Unmarshal([]byte(getResultText(result)), &response)
	require.NoError(t, err)

	assert.Equal(t, float64(0), response["total_count"])
}

func TestHandleListNetworkNeighborhoods_InitError(t *testing.T) {
	tool := NewKubescapeToolWithError(errors.New("failed to connect"))

	result, err := tool.HandleListNetworkNeighborhoods(context.Background(), makeRequest(nil))
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

	tool := NewKubescapeToolWithClients(nil, nil, spdxClient.SpdxV1beta1())

	result, err := tool.HandleGetNetworkNeighborhood(context.Background(), makeRequest(map[string]interface{}{
		"namespace": "default",
		"name":      "test-nn",
	}))
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.False(t, result.IsError)

	var response map[string]interface{}
	err = json.Unmarshal([]byte(getResultText(result)), &response)
	require.NoError(t, err)

	assert.Equal(t, "default", response["namespace"])
	assert.Equal(t, "test-nn", response["name"])
	assert.Contains(t, response["description"], "NetworkNeighborhood shows actual network connections")
}

func TestHandleGetNetworkNeighborhood_MissingName(t *testing.T) {
	spdxClient := kubescapefake.NewClientset()
	tool := NewKubescapeToolWithClients(nil, nil, spdxClient.SpdxV1beta1())

	result, err := tool.HandleGetNetworkNeighborhood(context.Background(), makeRequest(map[string]interface{}{
		"namespace": "default",
	}))
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.IsError)
	assert.Contains(t, getResultText(result), "name parameter is required")
}

func TestHandleGetNetworkNeighborhood_MissingNamespace(t *testing.T) {
	spdxClient := kubescapefake.NewClientset()
	tool := NewKubescapeToolWithClients(nil, nil, spdxClient.SpdxV1beta1())

	result, err := tool.HandleGetNetworkNeighborhood(context.Background(), makeRequest(map[string]interface{}{
		"name": "test-nn",
	}))
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.IsError)
	assert.Contains(t, getResultText(result), "namespace parameter is required")
}

func TestHandleGetNetworkNeighborhood_NotFound(t *testing.T) {
	spdxClient := kubescapefake.NewClientset()
	tool := NewKubescapeToolWithClients(nil, nil, spdxClient.SpdxV1beta1())

	result, err := tool.HandleGetNetworkNeighborhood(context.Background(), makeRequest(map[string]interface{}{
		"namespace": "default",
		"name":      "nonexistent",
	}))
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

// ---------------------------------------------------------------------------
// T3: list_vulnerabilities -- bounded output, truthful summary (design 01b)
// ---------------------------------------------------------------------------

// manifestWithMatches builds a manifest holding n CVEs of the given severity,
// alternating fix state so fixable filtering can be exercised.
func manifestWithMatches(name string, perSeverity map[string]int) *v1beta1.VulnerabilityManifest {
	matches := []v1beta1.Match{}
	i := 0
	for severity, n := range perSeverity {
		for j := 0; j < n; j++ {
			fixState := "not-fixed"
			if i%2 == 0 {
				fixState = "fixed"
			}
			matches = append(matches, v1beta1.Match{
				Vulnerability: v1beta1.Vulnerability{
					VulnerabilityMetadata: v1beta1.VulnerabilityMetadata{
						ID:          fmt.Sprintf("CVE-2024-%s-%04d", severity, j),
						Severity:    severity,
						Description: "a description long enough to matter for payload size, repeated over and over",
						DataSource:  "https://security-tracker.debian.org/tracker/CVE-2024-0000",
					},
					Fix: v1beta1.Fix{State: fixState, Versions: []string{"1.2.3"}},
				},
			})
			i++
		}
	}
	return &v1beta1.VulnerabilityManifest{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "kubescape"},
		Spec:       v1beta1.VulnerabilityManifestSpec{Payload: v1beta1.GrypeDocument{Matches: matches}},
	}
}

type vulnListResponse struct {
	ManifestName    string                   `json:"manifest_name"`
	SeveritySummary map[string]int           `json:"severity_summary"`
	TotalCount      int                      `json:"total_count"`
	ReturnedCount   int                      `json:"returned_count"`
	Truncated       bool                     `json:"truncated"`
	Vulnerabilities []map[string]interface{} `json:"vulnerabilities"`
}

func listVulns(t *testing.T, manifest *v1beta1.VulnerabilityManifest, args map[string]interface{}) vulnListResponse {
	t.Helper()
	spdxClient := kubescapefake.NewClientset(manifest)
	tool := NewKubescapeToolWithClients(nil, nil, spdxClient.SpdxV1beta1())

	if args == nil {
		args = map[string]interface{}{}
	}
	args["manifest_name"] = manifest.Name

	result, err := tool.HandleListVulnerabilitiesInManifest(context.Background(), makeRequest(args))
	require.NoError(t, err)
	require.False(t, result.IsError, getResultText(result))

	var resp vulnListResponse
	require.NoError(t, json.Unmarshal([]byte(getResultText(result)), &resp))
	return resp
}

// The whole point of P4: a manifest with hundreds of CVEs must not return
// hundreds of records. nginx:1.14.0 measured 200,793 B / 466 matches.
func TestHandleListVulnerabilities_BoundsOutputByDefault(t *testing.T) {
	m := manifestWithMatches("big", map[string]int{
		"Critical": 76, "High": 133, "Medium": 99, "Low": 56, "Negligible": 102,
	})

	resp := listVulns(t, m, nil)

	assert.Equal(t, 466, resp.TotalCount, "total_count must report every CVE, not just the returned ones")
	assert.Equal(t, 20, resp.ReturnedCount, "default limit should bound the array")
	assert.Len(t, resp.Vulnerabilities, 20)
	assert.True(t, resp.Truncated, "the agent must be told it received partial data")
}

// The severity summary is the cheap answer and must describe the WHOLE manifest
// even when the array is truncated.
func TestHandleListVulnerabilities_SummaryCoversAllMatchesNotJustReturned(t *testing.T) {
	m := manifestWithMatches("big", map[string]int{
		"Critical": 76, "High": 133, "Medium": 99, "Low": 56, "Negligible": 102,
	})

	resp := listVulns(t, m, nil)

	assert.Equal(t, 76, resp.SeveritySummary["Critical"])
	assert.Equal(t, 133, resp.SeveritySummary["High"])
	assert.Equal(t, 99, resp.SeveritySummary["Medium"])
	assert.Equal(t, 56, resp.SeveritySummary["Low"])
}

// Measured live: 102 Negligible CVEs were reported as "Unknown": 102 because
// severityCounts had no Negligible bucket.
func TestHandleListVulnerabilities_CountsNegligibleNotUnknown(t *testing.T) {
	m := manifestWithMatches("negl", map[string]int{"Negligible": 102})

	resp := listVulns(t, m, nil)

	assert.Equal(t, 102, resp.SeveritySummary["Negligible"], "Negligible must have its own bucket")
	assert.Equal(t, 0, resp.SeveritySummary["Unknown"], "Negligible must not be miscounted as Unknown")
}

// A genuinely unrecognised severity still lands in Unknown.
func TestHandleListVulnerabilities_UnrecognisedSeverityCountsAsUnknown(t *testing.T) {
	m := manifestWithMatches("weird", map[string]int{"Bogus": 3})

	resp := listVulns(t, m, nil)

	assert.Equal(t, 3, resp.SeveritySummary["Unknown"])
}

// Per-record fields: description (56.1% of the payload) and data_source (19.5%)
// are dropped; description is truncated mid-word anyway and full text lives in
// get_vulnerability_details.
func TestHandleListVulnerabilities_RecordCarriesOnlyIdSeverityFixState(t *testing.T) {
	m := manifestWithMatches("fields", map[string]int{"Critical": 1})

	resp := listVulns(t, m, nil)
	require.Len(t, resp.Vulnerabilities, 1)

	entry := resp.Vulnerabilities[0]
	assert.ElementsMatch(t, []string{"id", "severity", "fix_state", "affected_artifacts"}, keysOf(entry))
}

func keysOf(m map[string]interface{}) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

// Worst-first, so a truncated array is the part that matters.
func TestHandleListVulnerabilities_SortsBySeverityDescending(t *testing.T) {
	m := manifestWithMatches("order", map[string]int{
		"Negligible": 5, "Critical": 2, "Medium": 3, "High": 4, "Low": 1,
	})

	resp := listVulns(t, m, map[string]interface{}{"limit": float64(6)})

	got := []string{}
	for _, v := range resp.Vulnerabilities {
		got = append(got, v["severity"].(string))
	}
	assert.Equal(t, []string{"Critical", "Critical", "High", "High", "High", "High"}, got)
}

func TestHandleListVulnerabilities_FiltersBySeverity(t *testing.T) {
	m := manifestWithMatches("filter", map[string]int{"Critical": 3, "Low": 7})

	resp := listVulns(t, m, map[string]interface{}{"severity": "Critical"})

	assert.Equal(t, 3, resp.TotalCount, "total_count reflects the filter")
	assert.Len(t, resp.Vulnerabilities, 3)
	for _, v := range resp.Vulnerabilities {
		assert.Equal(t, "Critical", v["severity"])
	}
	// The summary still describes the whole manifest, so the agent keeps context.
	assert.Equal(t, 7, resp.SeveritySummary["Low"])
}

func TestHandleListVulnerabilities_FiltersFixableOnly(t *testing.T) {
	m := manifestWithMatches("fixable", map[string]int{"Critical": 10})

	resp := listVulns(t, m, map[string]interface{}{"fixable_only": true})

	assert.NotZero(t, len(resp.Vulnerabilities))
	for _, v := range resp.Vulnerabilities {
		assert.Equal(t, "fixed", v["fix_state"])
	}
	assert.Less(t, resp.TotalCount, 10, "fixable_only must actually exclude the unfixed ones")
}

// An explicit limit above the match count must not claim truncation.
func TestHandleListVulnerabilities_NotTruncatedWhenLimitExceedsMatches(t *testing.T) {
	m := manifestWithMatches("small", map[string]int{"High": 3})

	resp := listVulns(t, m, map[string]interface{}{"limit": float64(100)})

	assert.Equal(t, 3, resp.TotalCount)
	assert.Equal(t, 3, resp.ReturnedCount)
	assert.False(t, resp.Truncated)
}

func TestHandleListVulnerabilities_RejectsInvalidSeverity(t *testing.T) {
	m := manifestWithMatches("bad", map[string]int{"High": 1})
	spdxClient := kubescapefake.NewClientset(m)
	tool := NewKubescapeToolWithClients(nil, nil, spdxClient.SpdxV1beta1())

	result, err := tool.HandleListVulnerabilitiesInManifest(context.Background(), makeRequest(map[string]interface{}{
		"manifest_name": "bad",
		"severity":      "VeryBad",
	}))
	require.NoError(t, err)
	assert.True(t, result.IsError)
	assert.Contains(t, getResultText(result), "severity")
}

// ---------------------------------------------------------------------------
// T1/T2: overview and workload ranking (design 01b)
//
// These read the aggregate resources through the storage API. That API strips
// spec from every LIST unless ResourceVersion is the "fullSpec" sentinel, so a
// default LIST returns all-zero counts -- the P0 failure shape at cluster
// scale. The fake clientset ignores ResourceVersion and returns whatever was
// seeded, so it CANNOT reproduce the stripping. The invariant is therefore
// pinned by asserting the outgoing ListOptions, not by observing the response.
// ---------------------------------------------------------------------------

func counters(all, relevant int64, withRelevant bool) v1beta1.VulnerabilityCounters {
	c := v1beta1.VulnerabilityCounters{All: all}
	if withRelevant {
		c.Relevant = relevant
	}
	return c
}

func nsSummary(namespace string, crit, high int64, refs ...string) *v1beta1.VulnerabilitySummary {
	objRefs := []v1beta1.VulnerabilitiesObjScope{}
	for _, r := range refs {
		objRefs = append(objRefs, v1beta1.VulnerabilitiesObjScope{
			Namespace: namespace, Name: r, Kind: "vulnerabilitymanifestsummary",
		})
	}
	return &v1beta1.VulnerabilitySummary{
		ObjectMeta: metav1.ObjectMeta{Name: namespace},
		Spec: v1beta1.VulnerabilitySummarySpec{
			Severities: v1beta1.SeveritySummary{
				Critical: counters(crit, crit/2, true),
				High:     counters(high, 0, false),
			},
			WorkloadVulnerabilitiesObj: objRefs,
		},
	}
}

func workloadSummary(namespace, name, imageTag, manifestName string, crit, high int64) *v1beta1.VulnerabilityManifestSummary {
	return &v1beta1.VulnerabilityManifestSummary{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Annotations: map[string]string{
				"kubescape.io/image-tag": imageTag,
				"kubescape.io/status":    "ready",
			},
			Labels: map[string]string{
				"kubescape.io/workload-kind":           "deployment",
				"kubescape.io/workload-name":           "app",
				"kubescape.io/workload-container-name": "main",
			},
		},
		Spec: v1beta1.VulnerabilityManifestSummarySpec{
			Severities: v1beta1.SeveritySummary{
				Critical: counters(crit, crit/2, true),
				High:     counters(high, 0, false),
			},
			Vulnerabilities: v1beta1.VulnerabilitiesComponents{
				ImageVulnerabilitiesObj: v1beta1.VulnerabilitiesObjScope{
					// The server reports the WORKLOAD's namespace here, but the
					// manifest actually lives in the Kubescape namespace, so
					// this value is unusable. Verified on a live cluster: a GET
					// in this namespace returns NotFound.
					Namespace: namespace, Name: manifestName, Kind: "vulnerabilitymanifests",
				},
			},
		},
	}
}

// The fake clientset's recorded actions drop ResourceVersion entirely, so a
// reactor cannot see it. These thin decorators wrap the typed client and record
// the ListOptions the handler actually passes, which is the thing under test.

type recordingSpdx struct {
	spdxv1beta1.SpdxV1beta1Interface
	recorded *[]metav1.ListOptions
}

func (r recordingSpdx) VulnerabilitySummaries(ns string) spdxv1beta1.VulnerabilitySummaryInterface {
	return recordingVulnSummaries{r.SpdxV1beta1Interface.VulnerabilitySummaries(ns), r.recorded}
}

func (r recordingSpdx) VulnerabilityManifestSummaries(ns string) spdxv1beta1.VulnerabilityManifestSummaryInterface {
	return recordingManifestSummaries{r.SpdxV1beta1Interface.VulnerabilityManifestSummaries(ns), r.recorded}
}

type recordingVulnSummaries struct {
	spdxv1beta1.VulnerabilitySummaryInterface
	recorded *[]metav1.ListOptions
}

func (r recordingVulnSummaries) List(ctx context.Context, opts metav1.ListOptions) (*v1beta1.VulnerabilitySummaryList, error) {
	*r.recorded = append(*r.recorded, opts)
	return r.VulnerabilitySummaryInterface.List(ctx, opts)
}

type recordingManifestSummaries struct {
	spdxv1beta1.VulnerabilityManifestSummaryInterface
	recorded *[]metav1.ListOptions
}

func (r recordingManifestSummaries) List(ctx context.Context, opts metav1.ListOptions) (*v1beta1.VulnerabilityManifestSummaryList, error) {
	*r.recorded = append(*r.recorded, opts)
	return r.VulnerabilityManifestSummaryInterface.List(ctx, opts)
}

func recordingClient(c *kubescapefake.Clientset, sink *[]metav1.ListOptions) spdxv1beta1.SpdxV1beta1Interface {
	return recordingSpdx{c.SpdxV1beta1(), sink}
}

// THE LINCHPIN TEST. Without ResourceVersion="fullSpec" the storage server
// returns spec-stripped objects and every count reads zero -- an agent would be
// told a vulnerable cluster is clean. A fake cannot show that, so assert the
// request instead.
func TestHandleVulnerabilityOverview_RequestsFullSpec(t *testing.T) {
	spdxClient := kubescapefake.NewClientset(nsSummary("verify-targets", 99, 242, "deployment-vuln-nginx-nginx"))
	var seen []metav1.ListOptions

	tool := NewKubescapeToolWithClients(nil, nil, recordingClient(spdxClient, &seen))
	_, err := tool.HandleVulnerabilityOverview(context.Background(), makeRequest(nil))
	require.NoError(t, err)

	require.Len(t, seen, 1)
	assert.Equal(t, storageFullSpec, seen[0].ResourceVersion,
		"must request fullSpec: a default LIST returns all-zero counts and would report a vulnerable cluster as clean")
}

func TestHandleListVulnerableWorkloads_RequestsFullSpec(t *testing.T) {
	spdxClient := kubescapefake.NewClientset(
		workloadSummary("verify-targets", "deployment-vuln-nginx-nginx",
			"docker.io/library/nginx:1.14.0", "docker.io-library-nginx-1.14.0-e34030", 76, 133))
	var seen []metav1.ListOptions

	tool := NewKubescapeToolWithClients(nil, nil, recordingClient(spdxClient, &seen))
	_, err := tool.HandleListVulnerableWorkloads(context.Background(), makeRequest(nil))
	require.NoError(t, err)

	require.Len(t, seen, 1)
	assert.Equal(t, storageFullSpec, seen[0].ResourceVersion)
}

func TestHandleVulnerabilityOverview_AggregatesNamespaces(t *testing.T) {
	spdxClient := kubescapefake.NewClientset(
		nsSummary("verify-targets", 99, 242, "a", "b"),
		nsSummary("other", 1, 2, "c"),
	)
	tool := NewKubescapeToolWithClients(nil, nil, spdxClient.SpdxV1beta1())

	result, err := tool.HandleVulnerabilityOverview(context.Background(), makeRequest(nil))
	require.NoError(t, err)
	require.False(t, result.IsError, getResultText(result))

	var resp struct {
		Scope         string                            `json:"scope"`
		ClusterTotals map[string]map[string]interface{} `json:"cluster_totals"`
		Namespaces    []struct {
			Namespace     string `json:"namespace"`
			WorkloadCount int    `json:"workload_count"`
		} `json:"namespaces"`
	}
	require.NoError(t, json.Unmarshal([]byte(getResultText(result)), &resp))

	assert.Equal(t, "cluster", resp.Scope)
	assert.Equal(t, float64(100), resp.ClusterTotals["Critical"]["all"], "cluster total sums namespaces")
	require.Len(t, resp.Namespaces, 2)
	assert.Equal(t, "verify-targets", resp.Namespaces[0].Namespace, "worst namespace first")
	assert.Equal(t, 2, resp.Namespaces[0].WorkloadCount)
}

// relevant is `json:"relevant,omitempty"`, so a zero is indistinguishable from
// "relevancy not computed". Never render it as a measured zero.
func TestHandleVulnerabilityOverview_OmitsRelevantWhenAbsent(t *testing.T) {
	spdxClient := kubescapefake.NewClientset(nsSummary("verify-targets", 99, 242, "a"))
	tool := NewKubescapeToolWithClients(nil, nil, spdxClient.SpdxV1beta1())

	result, err := tool.HandleVulnerabilityOverview(context.Background(), makeRequest(nil))
	require.NoError(t, err)

	var resp struct {
		ClusterTotals map[string]map[string]interface{} `json:"cluster_totals"`
	}
	require.NoError(t, json.Unmarshal([]byte(getResultText(result)), &resp))

	// High was built with no relevant value at all.
	_, present := resp.ClusterTotals["High"]["relevant"]
	assert.False(t, present, "an absent relevant count must not be reported as 0")
	// Critical had one, so it survives.
	assert.Equal(t, float64(49), resp.ClusterTotals["Critical"]["relevant"])
}

func TestHandleListVulnerableWorkloads_CarriesManifestPointer(t *testing.T) {
	spdxClient := kubescapefake.NewClientset(
		workloadSummary("verify-targets", "deployment-vuln-nginx-nginx",
			"docker.io/library/nginx:1.14.0", "docker.io-library-nginx-1.14.0-e34030", 76, 133))
	tool := NewKubescapeToolWithClients(nil, nil, spdxClient.SpdxV1beta1())

	result, err := tool.HandleListVulnerableWorkloads(context.Background(), makeRequest(nil))
	require.NoError(t, err)
	require.False(t, result.IsError, getResultText(result))

	var resp struct {
		Workloads []map[string]interface{} `json:"workloads"`
	}
	require.NoError(t, json.Unmarshal([]byte(getResultText(result)), &resp))
	require.Len(t, resp.Workloads, 1)

	w := resp.Workloads[0]
	// The whole point of the ladder: the next call is handed over, never guessed.
	assert.Equal(t, "docker.io-library-nginx-1.14.0-e34030", w["manifest_name"])
	assert.Equal(t, "docker.io/library/nginx:1.14.0", w["image"])
	// The summary was built with the workload's namespace in the ref, which is
	// what the real server sends and where the manifest is NOT. Reporting it
	// verbatim hands the agent a pointer that 404s.
	assert.Equal(t, "kubescape", w["manifest_namespace"],
		"manifest_namespace must be where manifests actually live, not the unusable value in vulnerabilitiesRef")
}

func TestHandleListVulnerableWorkloads_SortsAndLimits(t *testing.T) {
	spdxClient := kubescapefake.NewClientset(
		workloadSummary("ns", "low-risk", "img:a", "manifest-a", 1, 0),
		workloadSummary("ns", "high-risk", "img:b", "manifest-b", 76, 0),
		workloadSummary("ns", "mid-risk", "img:c", "manifest-c", 12, 0),
	)
	tool := NewKubescapeToolWithClients(nil, nil, spdxClient.SpdxV1beta1())

	result, err := tool.HandleListVulnerableWorkloads(context.Background(), makeRequest(map[string]interface{}{
		"limit": float64(2),
	}))
	require.NoError(t, err)

	var resp struct {
		TotalWorkloads int                      `json:"total_workloads"`
		Returned       int                      `json:"returned"`
		Truncated      bool                     `json:"truncated"`
		Workloads      []map[string]interface{} `json:"workloads"`
	}
	require.NoError(t, json.Unmarshal([]byte(getResultText(result)), &resp))

	assert.Equal(t, 3, resp.TotalWorkloads)
	assert.Equal(t, 2, resp.Returned)
	assert.True(t, resp.Truncated)
	require.Len(t, resp.Workloads, 2)
	assert.Equal(t, "high-risk", resp.Workloads[0]["workload_summary"])
	assert.Equal(t, "mid-risk", resp.Workloads[1]["workload_summary"])
}

// If every namespace reports zero, we cannot tell "clean cluster" from
// "fullSpec stopped working". Say so rather than reporting good news.
func TestHandleVulnerabilityOverview_FlagsAllZeroAsUnverified(t *testing.T) {
	spdxClient := kubescapefake.NewClientset(nsSummary("verify-targets", 0, 0, "a"))
	tool := NewKubescapeToolWithClients(nil, nil, spdxClient.SpdxV1beta1())

	result, err := tool.HandleVulnerabilityOverview(context.Background(), makeRequest(nil))
	require.NoError(t, err)

	assert.Contains(t, getResultText(result), "could not be confirmed",
		"an all-zero result must be reported as unconfirmed, never as a clean cluster")
}

// Grype emits one Match per affected package, so the same CVE ID recurs. Once
// the package fields are trimmed away those rows are byte-identical and would
// burn the limit budget on duplicates -- measured live, the first 20 records for
// nginx:1.14.0 contained CVE-2017-12424 and CVE-2017-15670 twice each.
func TestHandleListVulnerabilities_DeduplicatesByCVE(t *testing.T) {
	dup := func(id, severity, fixState string) v1beta1.Match {
		return v1beta1.Match{Vulnerability: v1beta1.Vulnerability{
			VulnerabilityMetadata: v1beta1.VulnerabilityMetadata{ID: id, Severity: severity},
			Fix:                   v1beta1.Fix{State: fixState},
		}}
	}
	m := &v1beta1.VulnerabilityManifest{
		ObjectMeta: metav1.ObjectMeta{Name: "dupes", Namespace: "kubescape"},
		Spec: v1beta1.VulnerabilityManifestSpec{Payload: v1beta1.GrypeDocument{Matches: []v1beta1.Match{
			dup("CVE-2017-12424", "Critical", "fixed"),
			dup("CVE-2017-12424", "Critical", "fixed"),
			dup("CVE-2017-12424", "Critical", "fixed"),
			dup("CVE-2020-0001", "High", "not-fixed"),
		}}},
	}

	resp := listVulns(t, m, nil)

	// The summary counts matches, matching the numbers Kubescape itself reports.
	assert.Equal(t, 3, resp.SeveritySummary["Critical"])
	assert.Equal(t, 1, resp.SeveritySummary["High"])

	// The array carries distinct CVEs.
	require.Len(t, resp.Vulnerabilities, 2)
	assert.Equal(t, 2, resp.TotalCount, "total_count counts distinct CVEs in the array")

	assert.Equal(t, "CVE-2017-12424", resp.Vulnerabilities[0]["id"])
	assert.Equal(t, float64(3), resp.Vulnerabilities[0]["affected_artifacts"],
		"the collapsed matches must still be visible as a count")
	assert.Equal(t, float64(1), resp.Vulnerabilities[1]["affected_artifacts"])
}

// A CVE fixed in one package but not another is only actionable if the list says
// so, and "not-fixed" is the safer thing to surface.
func TestHandleListVulnerabilities_DedupeKeepsWorstFixState(t *testing.T) {
	mk := func(fixState string) v1beta1.Match {
		return v1beta1.Match{Vulnerability: v1beta1.Vulnerability{
			VulnerabilityMetadata: v1beta1.VulnerabilityMetadata{ID: "CVE-2021-1", Severity: "High"},
			Fix:                   v1beta1.Fix{State: fixState},
		}}
	}
	m := &v1beta1.VulnerabilityManifest{
		ObjectMeta: metav1.ObjectMeta{Name: "mixed", Namespace: "kubescape"},
		Spec:       v1beta1.VulnerabilityManifestSpec{Payload: v1beta1.GrypeDocument{Matches: []v1beta1.Match{mk("fixed"), mk("not-fixed")}}},
	}

	resp := listVulns(t, m, nil)

	require.Len(t, resp.Vulnerabilities, 1)
	assert.Equal(t, "not-fixed", resp.Vulnerabilities[0]["fix_state"],
		"a CVE unfixed in any package must not be reported as fixed")
}
