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
	tools := s.ListTools()
	assert.Len(t, tools, 6)

	expectedTools := map[string]bool{
		"kubescape_check_health":                 false,
		"kubescape_list_vulnerability_manifests": false,
		"kubescape_list_vulnerabilities":         false,
		"kubescape_get_vulnerability_details":    false,
		"kubescape_list_configuration_scans":     false,
		"kubescape_get_configuration_scan":       false,
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

	apiExtClient := apiextensionsfake.NewSimpleClientset(
		&apiextensionsv1.CustomResourceDefinition{
			ObjectMeta: metav1.ObjectMeta{Name: vulnerabilityManifestsCRD},
		},
		&apiextensionsv1.CustomResourceDefinition{
			ObjectMeta: metav1.ObjectMeta{Name: workloadConfigurationScansCRD},
		},
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
	assert.Equal(t, "Kubescape is fully operational", health.Summary)
}

func TestHandleCheckHealth_NamespaceNotFound(t *testing.T) {
	k8sClient := kubefake.NewSimpleClientset() // No namespace
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
	k8sClient := kubefake.NewSimpleClientset(
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "kubescape"}},
		// No operator pods
	)
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
	k8sClient := kubefake.NewSimpleClientset(
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "kubescape"}},
	)
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
	k8sClient := kubefake.NewSimpleClientset(
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "kubescape"}},
	)
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

func TestHandleCheckHealth_CustomNamespace(t *testing.T) {
	k8sClient := kubefake.NewSimpleClientset(
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "custom-ns"}},
	)
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
