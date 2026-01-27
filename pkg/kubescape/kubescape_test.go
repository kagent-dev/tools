package kubescape

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/kubescape/storage/pkg/apis/softwarecomposition/v1beta1"
	kubescapefake "github.com/kubescape/storage/pkg/generated/clientset/versioned/fake"
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
		RegisterTools(s, "")
	})

	// Verify tools are registered by checking the server has tools
	// NOTE: SBOM tools are disabled (too large for LLM context), so we expect 10 tools
	tools := s.ListTools()
	assert.Len(t, tools, 10)

	expectedTools := map[string]bool{
		"kubescape_check_health":                 false,
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

	spdxClient := kubescapefake.NewSimpleClientset(
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
	spdxClient := kubescapefake.NewSimpleClientset()

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
	spdxClient := kubescapefake.NewSimpleClientset()

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
	spdxClient := kubescapefake.NewSimpleClientset()

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
	spdxClient := kubescapefake.NewSimpleClientset()

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
	spdxClient := kubescapefake.NewSimpleClientset() // No vulnerability manifests

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
	spdxClient := kubescapefake.NewSimpleClientset()

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
	spdxClient := kubescapefake.NewSimpleClientset()

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
	spdxClient := kubescapefake.NewSimpleClientset(
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
	spdxClient := kubescapefake.NewSimpleClientset(
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
	spdxClient := kubescapefake.NewSimpleClientset()
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
	spdxClient := kubescapefake.NewSimpleClientset(
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
	spdxClient := kubescapefake.NewSimpleClientset()
	tool := NewKubescapeToolWithClients(nil, nil, spdxClient.SpdxV1beta1())

	result, err := tool.HandleListVulnerabilitiesInManifest(context.Background(), makeRequest(nil))
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.IsError)
	assert.Contains(t, getResultText(result), "manifest_name parameter is required")
}

func TestHandleListVulnerabilitiesInManifest_ManifestNotFound(t *testing.T) {
	spdxClient := kubescapefake.NewSimpleClientset()
	tool := NewKubescapeToolWithClients(nil, nil, spdxClient.SpdxV1beta1())

	result, err := tool.HandleListVulnerabilitiesInManifest(context.Background(), makeRequest(map[string]interface{}{
		"manifest_name": "nonexistent",
	}))
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.IsError)
}

func TestHandleGetVulnerabilityDetails_Success(t *testing.T) {
	spdxClient := kubescapefake.NewSimpleClientset(
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
	spdxClient := kubescapefake.NewSimpleClientset()
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
	spdxClient := kubescapefake.NewSimpleClientset()
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
	spdxClient := kubescapefake.NewSimpleClientset(
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
	spdxClient := kubescapefake.NewSimpleClientset(
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
	spdxClient := kubescapefake.NewSimpleClientset(
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
	spdxClient := kubescapefake.NewSimpleClientset()
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
	spdxClient := kubescapefake.NewSimpleClientset(
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
	spdxClient := kubescapefake.NewSimpleClientset()
	tool := NewKubescapeToolWithClients(nil, nil, spdxClient.SpdxV1beta1())

	result, err := tool.HandleGetConfigurationScan(context.Background(), makeRequest(nil))
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.IsError)
	assert.Contains(t, getResultText(result), "manifest_name parameter is required")
}

func TestHandleGetConfigurationScan_NotFound(t *testing.T) {
	spdxClient := kubescapefake.NewSimpleClientset()
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
	spdxClient := kubescapefake.NewSimpleClientset()
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
	spdxClient := kubescapefake.NewSimpleClientset(
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
	spdxClient := kubescapefake.NewSimpleClientset(
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
	spdxClient := kubescapefake.NewSimpleClientset()
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
	spdxClient := kubescapefake.NewSimpleClientset(
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
	spdxClient := kubescapefake.NewSimpleClientset()
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
	spdxClient := kubescapefake.NewSimpleClientset()
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
	spdxClient := kubescapefake.NewSimpleClientset()
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
	spdxClient := kubescapefake.NewSimpleClientset(
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
	spdxClient := kubescapefake.NewSimpleClientset(
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
	spdxClient := kubescapefake.NewSimpleClientset()
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
	spdxClient := kubescapefake.NewSimpleClientset(
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
	spdxClient := kubescapefake.NewSimpleClientset()
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
	spdxClient := kubescapefake.NewSimpleClientset()
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
	spdxClient := kubescapefake.NewSimpleClientset()
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
