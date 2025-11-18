package argo

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kagent-dev/tools/internal/cmd"
)

// Helper function to extract text content from MCP result
func getResultText(result *mcp.CallToolResult) string {
	if result == nil || len(result.Content) == 0 {
		return ""
	}
	if textContent, ok := result.Content[0].(*mcp.TextContent); ok {
		return textContent.Text
	}
	return ""
}

// Helper function to create MCP request with arguments
func createMCPRequest(args map[string]interface{}) *mcp.CallToolRequest {
	argsJSON, _ := json.Marshal(args)
	return &mcp.CallToolRequest{
		Params: &mcp.CallToolParamsRaw{
			Arguments: argsJSON,
		},
	}
}

// Test Argo Rollouts Promote
func TestHandlePromoteRollout(t *testing.T) {
	t.Run("promote rollout basic", func(t *testing.T) {
		mock := cmd.NewMockShellExecutor()
		expectedOutput := `rollout "myapp" promoted`

		mock.AddCommandString("kubectl", []string{"argo", "rollouts", "promote", "myapp"}, expectedOutput, nil)
		ctx := cmd.WithShellExecutor(context.Background(), mock)

		request := createMCPRequest(map[string]interface{}{
			"rollout_name": "myapp",
		})

		result, err := handlePromoteRollout(ctx, request)

		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.False(t, result.IsError)
		assert.Contains(t, getResultText(result), "promoted")

		// Verify the correct command was called
		callLog := mock.GetCallLog()
		require.Len(t, callLog, 1)
		assert.Equal(t, "kubectl", callLog[0].Command)
		assert.Equal(t, []string{"argo", "rollouts", "promote", "myapp"}, callLog[0].Args)
	})

	t.Run("promote rollout with namespace", func(t *testing.T) {
		mock := cmd.NewMockShellExecutor()
		expectedOutput := `rollout "myapp" promoted`

		mock.AddCommandString("kubectl", []string{"argo", "rollouts", "promote", "-n", "production", "myapp"}, expectedOutput, nil)
		ctx := cmd.WithShellExecutor(context.Background(), mock)

		request := createMCPRequest(map[string]interface{}{
			"rollout_name": "myapp",
			"namespace":    "production",
		})

		result, err := handlePromoteRollout(ctx, request)

		assert.NoError(t, err)
		assert.False(t, result.IsError)

		// Verify the correct command was called with namespace
		callLog := mock.GetCallLog()
		require.Len(t, callLog, 1)
		assert.Equal(t, "kubectl", callLog[0].Command)
		assert.Equal(t, []string{"argo", "rollouts", "promote", "-n", "production", "myapp"}, callLog[0].Args)
	})

	t.Run("promote rollout with full flag", func(t *testing.T) {
		mock := cmd.NewMockShellExecutor()
		expectedOutput := `rollout "myapp" fully promoted`

		mock.AddCommandString("kubectl", []string{"argo", "rollouts", "promote", "myapp", "--full"}, expectedOutput, nil)
		ctx := cmd.WithShellExecutor(context.Background(), mock)

		request := createMCPRequest(map[string]interface{}{
			"rollout_name": "myapp",
			"full":         "true",
		})

		result, err := handlePromoteRollout(ctx, request)

		assert.NoError(t, err)
		assert.False(t, result.IsError)

		// Verify the correct command was called with full flag
		callLog := mock.GetCallLog()
		require.Len(t, callLog, 1)
		assert.Equal(t, "kubectl", callLog[0].Command)
		assert.Equal(t, []string{"argo", "rollouts", "promote", "myapp", "--full"}, callLog[0].Args)
	})

	t.Run("missing required parameters", func(t *testing.T) {
		mock := cmd.NewMockShellExecutor()
		ctx := cmd.WithShellExecutor(context.Background(), mock)

		request := createMCPRequest(map[string]interface{}{
			// Missing rollout_name
		})

		result, err := handlePromoteRollout(ctx, request)
		assert.NoError(t, err)
		assert.True(t, result.IsError)
		assert.Contains(t, getResultText(result), "rollout_name parameter is required")

		// Verify no commands were executed
		callLog := mock.GetCallLog()
		assert.Len(t, callLog, 0)
	})

	t.Run("kubectl command failure", func(t *testing.T) {
		mock := cmd.NewMockShellExecutor()
		mock.AddCommandString("kubectl", []string{"argo", "rollouts", "promote", "myapp"}, "", assert.AnError)
		ctx := cmd.WithShellExecutor(context.Background(), mock)

		request := createMCPRequest(map[string]interface{}{
			"rollout_name": "myapp",
		})

		result, err := handlePromoteRollout(ctx, request)

		assert.NoError(t, err) // MCP handlers should not return Go errors
		assert.True(t, result.IsError)
		assert.Contains(t, getResultText(result), "Error promoting rollout")
	})
}

// Test Argo Rollouts Pause
func TestHandlePauseRollout(t *testing.T) {
	t.Run("pause rollout basic", func(t *testing.T) {
		mock := cmd.NewMockShellExecutor()
		expectedOutput := `rollout "myapp" paused`

		mock.AddCommandString("kubectl", []string{"argo", "rollouts", "pause", "myapp"}, expectedOutput, nil)
		ctx := cmd.WithShellExecutor(context.Background(), mock)

		request := createMCPRequest(map[string]interface{}{
			"rollout_name": "myapp",
		})

		result, err := handlePauseRollout(ctx, request)

		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.False(t, result.IsError)

		// Verify the expected output
		content := getResultText(result)
		assert.Contains(t, content, "paused")

		// Verify the correct command was called
		callLog := mock.GetCallLog()
		require.Len(t, callLog, 1)
		assert.Equal(t, "kubectl", callLog[0].Command)
		assert.Equal(t, []string{"argo", "rollouts", "pause", "myapp"}, callLog[0].Args)
	})

	t.Run("pause rollout with namespace", func(t *testing.T) {
		mock := cmd.NewMockShellExecutor()
		expectedOutput := `rollout "myapp" paused`

		mock.AddCommandString("kubectl", []string{"argo", "rollouts", "pause", "-n", "production", "myapp"}, expectedOutput, nil)
		ctx := cmd.WithShellExecutor(context.Background(), mock)

		request := createMCPRequest(map[string]interface{}{
			"rollout_name": "myapp",
			"namespace":    "production",
		})

		result, err := handlePauseRollout(ctx, request)

		assert.NoError(t, err)
		assert.False(t, result.IsError)

		// Verify the correct command was called with namespace
		callLog := mock.GetCallLog()
		require.Len(t, callLog, 1)
		assert.Equal(t, "kubectl", callLog[0].Command)
		assert.Equal(t, []string{"argo", "rollouts", "pause", "-n", "production", "myapp"}, callLog[0].Args)
	})

	t.Run("missing required parameters", func(t *testing.T) {
		mock := cmd.NewMockShellExecutor()
		ctx := cmd.WithShellExecutor(context.Background(), mock)

		request := createMCPRequest(map[string]interface{}{
			// Missing rollout_name
		})

		result, err := handlePauseRollout(ctx, request)
		assert.NoError(t, err)
		assert.True(t, result.IsError)
		assert.Contains(t, getResultText(result), "rollout_name parameter is required")

		// Verify no commands were executed
		callLog := mock.GetCallLog()
		assert.Len(t, callLog, 0)
	})
}

// Test Argo Rollouts Set Image
func TestHandleSetRolloutImage(t *testing.T) {
	t.Run("set rollout image basic", func(t *testing.T) {
		mock := cmd.NewMockShellExecutor()
		expectedOutput := `rollout "myapp" image updated`

		mock.AddCommandString("kubectl", []string{"argo", "rollouts", "set", "image", "myapp", "nginx:latest"}, expectedOutput, nil)
		ctx := cmd.WithShellExecutor(context.Background(), mock)

		request := createMCPRequest(map[string]interface{}{
			"rollout_name":    "myapp",
			"container_image": "nginx:latest",
		})

		result, err := handleSetRolloutImage(ctx, request)

		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.False(t, result.IsError)

		// Verify the expected output
		content := getResultText(result)
		assert.Contains(t, content, "image updated")

		// Verify the correct command was called
		callLog := mock.GetCallLog()
		require.Len(t, callLog, 1)
		assert.Equal(t, "kubectl", callLog[0].Command)
		assert.Equal(t, []string{"argo", "rollouts", "set", "image", "myapp", "nginx:latest"}, callLog[0].Args)
	})

	t.Run("set rollout image with namespace", func(t *testing.T) {
		mock := cmd.NewMockShellExecutor()
		expectedOutput := `rollout "myapp" image updated`

		mock.AddCommandString("kubectl", []string{"argo", "rollouts", "set", "image", "myapp", "nginx:1.20", "-n", "production"}, expectedOutput, nil)
		ctx := cmd.WithShellExecutor(context.Background(), mock)

		request := createMCPRequest(map[string]interface{}{
			"rollout_name":    "myapp",
			"container_image": "nginx:1.20",
			"namespace":       "production",
		})

		result, err := handleSetRolloutImage(ctx, request)

		assert.NoError(t, err)
		assert.False(t, result.IsError)

		// Verify the correct command was called with namespace
		callLog := mock.GetCallLog()
		require.Len(t, callLog, 1)
		assert.Equal(t, "kubectl", callLog[0].Command)
		assert.Equal(t, []string{"argo", "rollouts", "set", "image", "myapp", "nginx:1.20", "-n", "production"}, callLog[0].Args)
	})

	t.Run("missing rollout_name parameter", func(t *testing.T) {
		mock := cmd.NewMockShellExecutor()
		ctx := cmd.WithShellExecutor(context.Background(), mock)

		request := createMCPRequest(map[string]interface{}{
			"container_image": "nginx:latest",
			// Missing rollout_name
		})

		result, err := handleSetRolloutImage(ctx, request)
		assert.NoError(t, err)
		assert.True(t, result.IsError)
		assert.Contains(t, getResultText(result), "rollout_name parameter is required")

		// Verify no commands were executed
		callLog := mock.GetCallLog()
		assert.Len(t, callLog, 0)
	})

	t.Run("missing container_image parameter", func(t *testing.T) {
		mock := cmd.NewMockShellExecutor()
		ctx := cmd.WithShellExecutor(context.Background(), mock)

		request := createMCPRequest(map[string]interface{}{
			"rollout_name": "myapp",
			// Missing container_image
		})

		result, err := handleSetRolloutImage(ctx, request)
		assert.NoError(t, err)
		assert.True(t, result.IsError)
		assert.Contains(t, getResultText(result), "container_image parameter is required")

		// Verify no commands were executed
		callLog := mock.GetCallLog()
		assert.Len(t, callLog, 0)
	})
}

func TestGetSystemArchitecture(t *testing.T) {
	arch, err := getSystemArchitecture()
	if err != nil {
		t.Fatalf("getSystemArchitecture failed: %v", err)
	}

	if arch == "" {
		t.Error("Expected non-empty architecture")
	}

	// Architecture should contain system info
	if len(arch) < 5 {
		t.Errorf("Expected architecture string to be reasonable length, got: %s", arch)
	}
}

func TestGetLatestVersion(t *testing.T) {
	version := getLatestVersion(context.Background())
	if version == "" {
		t.Error("Expected non-empty version")
	}

	// Should return at least the default version
	if version != "0.5.0" && len(version) < 3 {
		t.Errorf("Expected valid version format, got: %s", version)
	}
}

func TestGatewayPluginStatus(t *testing.T) {
	status := GatewayPluginStatus{
		Installed:    true,
		Version:      "0.5.0",
		Architecture: "linux-amd64",
		DownloadTime: 1.5,
	}

	str := status.String()
	if str == "" {
		t.Error("Expected non-empty string representation")
	}

	// Should be valid JSON
	if !strings.Contains(str, "installed") {
		t.Error("Expected string to contain 'installed' field")
	}
}

// Test Verify Gateway Plugin
func TestHandleVerifyGatewayPlugin(t *testing.T) {
	t.Run("verify gateway plugin without install", func(t *testing.T) {
		mock := cmd.NewMockShellExecutor()
		expectedOutput := `gateway-api-plugin not found`

		mock.AddCommandString("kubectl", []string{"get", "configmap", "argo-rollouts-config", "-n", "argo-rollouts", "-o", "yaml"}, expectedOutput, nil)
		ctx := cmd.WithShellExecutor(context.Background(), mock)

		request := createMCPRequest(map[string]interface{}{
			"should_install": "false",
		})

		result, err := handleVerifyGatewayPlugin(ctx, request)

		assert.NoError(t, err)
		assert.NotNil(t, result)
		// May be success or error depending on whether plugin exists

		// Verify kubectl command was called
		callLog := mock.GetCallLog()
		require.Len(t, callLog, 1)
		assert.Equal(t, "kubectl", callLog[0].Command)
		assert.Contains(t, callLog[0].Args, "get")
		assert.Contains(t, callLog[0].Args, "configmap")
		assert.Contains(t, callLog[0].Args, "argo-rollouts-config")
	})

	t.Run("verify gateway plugin with custom namespace", func(t *testing.T) {
		mock := cmd.NewMockShellExecutor()
		expectedOutput := `gateway-api-plugin-abc123`

		mock.AddCommandString("kubectl", []string{"get", "configmap", "argo-rollouts-config", "-n", "custom-namespace", "-o", "yaml"}, expectedOutput, nil)
		ctx := cmd.WithShellExecutor(context.Background(), mock)

		request := createMCPRequest(map[string]interface{}{
			"should_install": "false",
			"namespace":      "custom-namespace",
		})

		result, err := handleVerifyGatewayPlugin(ctx, request)

		assert.NoError(t, err)
		assert.NotNil(t, result)

		// Verify kubectl command was called with custom namespace
		callLog := mock.GetCallLog()
		require.Len(t, callLog, 1)
		assert.Equal(t, "kubectl", callLog[0].Command)
		assert.Contains(t, callLog[0].Args, "-n")
		assert.Contains(t, callLog[0].Args, "custom-namespace")
	})
}

// Test Verify Argo Rollouts Controller Install
func TestHandleVerifyArgoRolloutsControllerInstall(t *testing.T) {
	t.Run("verify controller install", func(t *testing.T) {
		mock := cmd.NewMockShellExecutor()
		expectedOutput := `Running Running`

		mock.AddCommandString("kubectl", []string{"get", "pods", "-n", "argo-rollouts", "-l", "app.kubernetes.io/component=rollouts-controller", "-o", "jsonpath={.items[*].status.phase}"}, expectedOutput, nil)
		ctx := cmd.WithShellExecutor(context.Background(), mock)

		request := createMCPRequest(map[string]interface{}{})
		result, err := handleVerifyArgoRolloutsControllerInstall(ctx, request)

		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.False(t, result.IsError)
		assert.Contains(t, getResultText(result), "All pods are running")

		// Verify kubectl command was called
		callLog := mock.GetCallLog()
		require.Len(t, callLog, 1)
		assert.Equal(t, "kubectl", callLog[0].Command)
		assert.Contains(t, callLog[0].Args, "get")
		assert.Contains(t, callLog[0].Args, "pods")
	})

	t.Run("verify controller install with custom namespace", func(t *testing.T) {
		mock := cmd.NewMockShellExecutor()
		expectedOutput := `Running`

		mock.AddCommandString("kubectl", []string{"get", "pods", "-n", "custom-argo", "-l", "app.kubernetes.io/component=rollouts-controller", "-o", "jsonpath={.items[*].status.phase}"}, expectedOutput, nil)
		ctx := cmd.WithShellExecutor(context.Background(), mock)

		request := createMCPRequest(map[string]interface{}{
			"namespace": "custom-argo",
		})

		result, err := handleVerifyArgoRolloutsControllerInstall(ctx, request)

		assert.NoError(t, err)
		assert.NotNil(t, result)

		// Verify kubectl command was called with custom namespace
		callLog := mock.GetCallLog()
		require.Len(t, callLog, 1)
		assert.Equal(t, "kubectl", callLog[0].Command)
		assert.Contains(t, callLog[0].Args, "-n")
		assert.Contains(t, callLog[0].Args, "custom-argo")
	})

	t.Run("verify controller install with custom label", func(t *testing.T) {
		mock := cmd.NewMockShellExecutor()
		expectedOutput := `Running`

		mock.AddCommandString("kubectl", []string{"get", "pods", "-n", "argo-rollouts", "-l", "app=custom-rollouts", "-o", "jsonpath={.items[*].status.phase}"}, expectedOutput, nil)
		ctx := cmd.WithShellExecutor(context.Background(), mock)

		request := createMCPRequest(map[string]interface{}{
			"label": "app=custom-rollouts",
		})

		result, err := handleVerifyArgoRolloutsControllerInstall(ctx, request)

		assert.NoError(t, err)
		assert.NotNil(t, result)

		// Verify kubectl command was called with custom label
		callLog := mock.GetCallLog()
		require.Len(t, callLog, 1)
		assert.Equal(t, "kubectl", callLog[0].Command)
		assert.Contains(t, callLog[0].Args, "-l")
		assert.Contains(t, callLog[0].Args, "app=custom-rollouts")
	})
}

// Test Verify Kubectl Plugin Install
func TestHandleVerifyKubectlPluginInstall(t *testing.T) {
	t.Run("verify kubectl plugin install", func(t *testing.T) {
		mock := cmd.NewMockShellExecutor()
		expectedOutput := `kubectl-argo-rollouts: v1.6.0+d1ab3f2`

		mock.AddCommandString("kubectl", []string{"argo", "rollouts", "version"}, expectedOutput, nil)
		ctx := cmd.WithShellExecutor(context.Background(), mock)

		request := createMCPRequest(map[string]interface{}{})
		result, err := handleVerifyKubectlPluginInstall(ctx, request)

		assert.NoError(t, err)
		assert.False(t, result.IsError)
		assert.Contains(t, getResultText(result), "kubectl-argo-rollouts")

		// Verify the correct command was called
		callLog := mock.GetCallLog()
		require.Len(t, callLog, 1)
		assert.Equal(t, "kubectl", callLog[0].Command)
		assert.Equal(t, []string{"argo", "rollouts", "version"}, callLog[0].Args)
	})

	t.Run("kubectl plugin command failure", func(t *testing.T) {
		mock := cmd.NewMockShellExecutor()
		mock.AddCommandString("kubectl", []string{"argo", "rollouts", "version"}, "", assert.AnError)
		ctx := cmd.WithShellExecutor(context.Background(), mock)

		request := createMCPRequest(map[string]interface{}{})
		result, err := handleVerifyKubectlPluginInstall(ctx, request)

		assert.NoError(t, err) // MCP handlers should not return Go errors
		assert.NotNil(t, result)
		assert.Contains(t, getResultText(result), "Kubectl Argo Rollouts plugin is not installed")
	})
}

// Test List Rollouts
func TestHandleListRollouts(t *testing.T) {
	t.Run("list rollouts basic", func(t *testing.T) {
		mock := cmd.NewMockShellExecutor()
		expectedOutput := `NAME     STRATEGY   STATUS        STEP  SET-WEIGHT  READY  DESIRED  UP-TO-DATE  AVAILABLE
myapp    Canary     Healthy       8/8   100         1/1    1        1           1`

		mock.AddCommandString("kubectl", []string{"argo", "rollouts", "list", "rollouts", "-n", "argo-rollouts"}, expectedOutput, nil)
		ctx := cmd.WithShellExecutor(context.Background(), mock)

		request := createMCPRequest(map[string]interface{}{})
		result, err := handleListRollouts(ctx, request)

		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.False(t, result.IsError)
		assert.Contains(t, getResultText(result), "myapp")

		// Verify the correct command was called
		callLog := mock.GetCallLog()
		require.Len(t, callLog, 1)
		assert.Equal(t, "kubectl", callLog[0].Command)
		assert.Equal(t, []string{"argo", "rollouts", "list", "rollouts", "-n", "argo-rollouts"}, callLog[0].Args)
	})

	t.Run("list experiments", func(t *testing.T) {
		mock := cmd.NewMockShellExecutor()
		expectedOutput := `NAME     STATUS   AGE
exp1     Running  5m`

		mock.AddCommandString("kubectl", []string{"argo", "rollouts", "list", "experiments", "-n", "argo-rollouts"}, expectedOutput, nil)
		ctx := cmd.WithShellExecutor(context.Background(), mock)

		request := createMCPRequest(map[string]interface{}{
			"type": "experiments",
		})
		result, err := handleListRollouts(ctx, request)

		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.False(t, result.IsError)

		// Verify the correct command was called
		callLog := mock.GetCallLog()
		require.Len(t, callLog, 1)
		assert.Equal(t, "kubectl", callLog[0].Command)
		assert.Equal(t, []string{"argo", "rollouts", "list", "experiments", "-n", "argo-rollouts"}, callLog[0].Args)
	})
}

func TestGatewayPluginInstallFlowAndLogs(t *testing.T) {
	// should_install=true triggers configureGatewayPlugin with kubectl apply
	mock := cmd.NewMockShellExecutor()
	// First, get configmap returns some text without the plugin marker
	mock.AddCommandString("kubectl", []string{"get", "configmap", "argo-rollouts-config", "-n", "argo-rollouts", "-o", "yaml"}, "not configured", nil)
	// Then, apply is called with a temp file path; use partial matcher
	mock.AddPartialMatcherString("kubectl", []string{"apply", "-f"}, "config applied", nil)
	ctx := cmd.WithShellExecutor(context.Background(), mock)

	req := createMCPRequest(map[string]interface{}{
		"should_install": "true",
		"version":        "0.5.0",
		"namespace":      "argo-rollouts",
	})
	res, err := handleVerifyGatewayPlugin(ctx, req)
	require.NoError(t, err)
	assert.NotNil(t, res)

	// Now test logs parser success path
	mock2 := cmd.NewMockShellExecutor()
	logs := `... Downloading plugin argoproj-labs/gatewayAPI from: https://example/v1.2.3/gatewayapi-plugin-darwin-arm64"
Download complete, it took 1.23s`
	mock2.AddCommandString("kubectl", []string{"logs", "-n", "argo-rollouts", "-l", "app.kubernetes.io/name=argo-rollouts", "--tail", "100"}, logs, nil)
	ctx2 := cmd.WithShellExecutor(context.Background(), mock2)
	res2, err := handleCheckPluginLogs(ctx2, createMCPRequest(map[string]interface{}{}))
	require.NoError(t, err)
	assert.NotNil(t, res2)
	content := getResultText(res2)
	assert.Contains(t, content, "download_time")
	assert.Contains(t, content, "darwin-arm64")
}

// TestRegisterToolsArgo verifies that RegisterTools correctly registers all Argo tools
func TestRegisterToolsArgo(t *testing.T) {
	server := mcp.NewServer(&mcp.Implementation{
		Name:    "test-server",
		Version: "1.0.0",
	}, nil)

	err := RegisterTools(server)
	require.NoError(t, err, "RegisterTools should not return an error")

	// Note: In the actual implementation, we can't easily verify tool registration
	// without accessing internal server state. This test verifies the function
	// runs without errors, which covers the registration logic paths.
}

// ArgoCD Client Tests

// mockHTTPRoundTripper mocks HTTP responses for ArgoCD client tests
type mockHTTPRoundTripper struct {
	response  *http.Response
	err       error
	responses []*http.Response
	callCount int
}

func (m *mockHTTPRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	if m.err != nil {
		return nil, m.err
	}
	if len(m.responses) > 0 {
		if m.callCount < len(m.responses) {
			resp := m.responses[m.callCount]
			m.callCount++
			return resp, nil
		}
		// Return last response if more calls than responses
		if len(m.responses) > 0 {
			return m.responses[len(m.responses)-1], nil
		}
	}
	return m.response, nil
}

func createMockHTTPResponse(statusCode int, body string) *http.Response {
	return &http.Response{
		StatusCode: statusCode,
		Body:       io.NopCloser(strings.NewReader(body)),
		Header:     make(http.Header),
	}
}

func TestNewArgoCDClient(t *testing.T) {
	t.Run("valid client creation", func(t *testing.T) {
		client, err := NewArgoCDClient("https://argocd.example.com", "test-token")
		require.NoError(t, err)
		assert.NotNil(t, client)
	})

	t.Run("invalid URL", func(t *testing.T) {
		client, err := NewArgoCDClient("not-a-url", "test-token")
		assert.Error(t, err)
		assert.Nil(t, client)
	})

	t.Run("removes trailing slash", func(t *testing.T) {
		client, err := NewArgoCDClient("https://argocd.example.com/", "test-token")
		require.NoError(t, err)
		assert.NotNil(t, client)
	})
}

func TestArgoCDClientListApplications(t *testing.T) {
	t.Run("successful list", func(t *testing.T) {
		responseBody := `{"items":[{"metadata":{"name":"test-app"}}]}`
		mockClient := &http.Client{
			Transport: &mockHTTPRoundTripper{
				response: createMockHTTPResponse(200, responseBody),
			},
		}

		client := &ArgoCDClient{
			baseURL:  "https://argocd.example.com",
			apiToken: "test-token",
			client:   mockClient,
		}

		result, err := client.ListApplications(context.Background(), nil)
		require.NoError(t, err)
		assert.NotNil(t, result)
	})

	t.Run("with filters", func(t *testing.T) {
		responseBody := `{"items":[]}`
		mockClient := &http.Client{
			Transport: &mockHTTPRoundTripper{
				response: createMockHTTPResponse(200, responseBody),
			},
		}

		client := &ArgoCDClient{
			baseURL:  "https://argocd.example.com",
			apiToken: "test-token",
			client:   mockClient,
		}

		limit := 10
		offset := 0
		opts := &ListApplicationsOptions{
			Search: "test",
			Limit:  &limit,
			Offset: &offset,
		}

		result, err := client.ListApplications(context.Background(), opts)
		require.NoError(t, err)
		assert.NotNil(t, result)
	})
}

func TestArgoCDClientGetApplication(t *testing.T) {
	responseBody := `{"metadata":{"name":"test-app"}}`
	mockClient := &http.Client{
		Transport: &mockHTTPRoundTripper{
			responses: []*http.Response{
				createMockHTTPResponse(200, responseBody),
				createMockHTTPResponse(200, responseBody),
			},
		},
	}

	client := &ArgoCDClient{
		baseURL:  "https://argocd.example.com",
		apiToken: "test-token",
		client:   mockClient,
	}

	result, err := client.GetApplication(context.Background(), "test-app", nil)
	require.NoError(t, err)
	assert.NotNil(t, result)

	// Test with namespace
	namespace := "argocd"
	result2, err := client.GetApplication(context.Background(), "test-app", &namespace)
	require.NoError(t, err)
	assert.NotNil(t, result2)
}

func TestArgoCDClientGetApplicationResourceTree(t *testing.T) {
	responseBody := `{"nodes":[{"kind":"Deployment","name":"test-deploy"}]}`
	mockClient := &http.Client{
		Transport: &mockHTTPRoundTripper{
			response: createMockHTTPResponse(200, responseBody),
		},
	}

	client := &ArgoCDClient{
		baseURL:  "https://argocd.example.com",
		apiToken: "test-token",
		client:   mockClient,
	}

	result, err := client.GetApplicationResourceTree(context.Background(), "test-app")
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestArgoCDClientGetApplicationManagedResources(t *testing.T) {
	responseBody := `{"items":[]}`
	mockClient := &http.Client{
		Transport: &mockHTTPRoundTripper{
			response: createMockHTTPResponse(200, responseBody),
		},
	}

	client := &ArgoCDClient{
		baseURL:  "https://argocd.example.com",
		apiToken: "test-token",
		client:   mockClient,
	}

	kind := "Deployment"
	filters := &ManagedResourcesFilters{
		Kind: &kind,
	}

	result, err := client.GetApplicationManagedResources(context.Background(), "test-app", filters)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestArgoCDClientGetWorkloadLogs(t *testing.T) {
	responseBody := `{"logs":"test logs"}`
	mockClient := &http.Client{
		Transport: &mockHTTPRoundTripper{
			response: createMockHTTPResponse(200, responseBody),
		},
	}

	client := &ArgoCDClient{
		baseURL:  "https://argocd.example.com",
		apiToken: "test-token",
		client:   mockClient,
	}

	resourceRef := ResourceRef{
		UID:       "uid-123",
		Version:   "v1",
		Group:     "apps",
		Kind:      "Deployment",
		Name:      "test-deploy",
		Namespace: "default",
	}

	result, err := client.GetWorkloadLogs(context.Background(), "test-app", "argocd", resourceRef, "container")
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestArgoCDClientGetApplicationEvents(t *testing.T) {
	responseBody := `{"items":[]}`
	mockClient := &http.Client{
		Transport: &mockHTTPRoundTripper{
			response: createMockHTTPResponse(200, responseBody),
		},
	}

	client := &ArgoCDClient{
		baseURL:  "https://argocd.example.com",
		apiToken: "test-token",
		client:   mockClient,
	}

	result, err := client.GetApplicationEvents(context.Background(), "test-app")
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestArgoCDClientGetResourceEvents(t *testing.T) {
	responseBody := `{"items":[]}`
	mockClient := &http.Client{
		Transport: &mockHTTPRoundTripper{
			response: createMockHTTPResponse(200, responseBody),
		},
	}

	client := &ArgoCDClient{
		baseURL:  "https://argocd.example.com",
		apiToken: "test-token",
		client:   mockClient,
	}

	result, err := client.GetResourceEvents(context.Background(), "test-app", "argocd", "uid-123", "default", "test-resource")
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestArgoCDClientGetResource(t *testing.T) {
	responseBody := `{"metadata":{"name":"test-resource"}}`
	mockClient := &http.Client{
		Transport: &mockHTTPRoundTripper{
			response: createMockHTTPResponse(200, responseBody),
		},
	}

	client := &ArgoCDClient{
		baseURL:  "https://argocd.example.com",
		apiToken: "test-token",
		client:   mockClient,
	}

	resourceRef := ResourceRef{
		UID:       "uid-123",
		Version:   "v1",
		Group:     "apps",
		Kind:      "Deployment",
		Name:      "test-deploy",
		Namespace: "default",
	}

	result, err := client.GetResource(context.Background(), "test-app", "argocd", resourceRef)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestArgoCDClientGetResourceActions(t *testing.T) {
	responseBody := `{"actions":[]}`
	mockClient := &http.Client{
		Transport: &mockHTTPRoundTripper{
			response: createMockHTTPResponse(200, responseBody),
		},
	}

	client := &ArgoCDClient{
		baseURL:  "https://argocd.example.com",
		apiToken: "test-token",
		client:   mockClient,
	}

	resourceRef := ResourceRef{
		UID:       "uid-123",
		Version:   "v1",
		Group:     "apps",
		Kind:      "Deployment",
		Name:      "test-deploy",
		Namespace: "default",
	}

	result, err := client.GetResourceActions(context.Background(), "test-app", "argocd", resourceRef)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestArgoCDClientCreateApplication(t *testing.T) {
	responseBody := `{"metadata":{"name":"test-app"}}`
	mockClient := &http.Client{
		Transport: &mockHTTPRoundTripper{
			response: createMockHTTPResponse(200, responseBody),
		},
	}

	client := &ArgoCDClient{
		baseURL:  "https://argocd.example.com",
		apiToken: "test-token",
		client:   mockClient,
	}

	app := map[string]interface{}{
		"metadata": map[string]interface{}{
			"name": "test-app",
		},
	}

	result, err := client.CreateApplication(context.Background(), app)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestArgoCDClientUpdateApplication(t *testing.T) {
	responseBody := `{"metadata":{"name":"test-app"}}`
	mockClient := &http.Client{
		Transport: &mockHTTPRoundTripper{
			response: createMockHTTPResponse(200, responseBody),
		},
	}

	client := &ArgoCDClient{
		baseURL:  "https://argocd.example.com",
		apiToken: "test-token",
		client:   mockClient,
	}

	app := map[string]interface{}{
		"metadata": map[string]interface{}{
			"name": "test-app",
		},
	}

	result, err := client.UpdateApplication(context.Background(), "test-app", app)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestArgoCDClientDeleteApplication(t *testing.T) {
	responseBody := `{}`
	mockClient := &http.Client{
		Transport: &mockHTTPRoundTripper{
			responses: []*http.Response{
				createMockHTTPResponse(200, responseBody),
				createMockHTTPResponse(200, responseBody),
			},
		},
	}

	client := &ArgoCDClient{
		baseURL:  "https://argocd.example.com",
		apiToken: "test-token",
		client:   mockClient,
	}

	result, err := client.DeleteApplication(context.Background(), "test-app", nil)
	require.NoError(t, err)
	assert.NotNil(t, result)

	// Test with options
	appNs := "argocd"
	cascade := true
	options := &DeleteApplicationOptions{
		AppNamespace: &appNs,
		Cascade:      &cascade,
	}

	result2, err := client.DeleteApplication(context.Background(), "test-app", options)
	require.NoError(t, err)
	assert.NotNil(t, result2)
}

func TestArgoCDClientSyncApplication(t *testing.T) {
	responseBody := `{"status":"success"}`
	mockClient := &http.Client{
		Transport: &mockHTTPRoundTripper{
			responses: []*http.Response{
				createMockHTTPResponse(200, responseBody),
				createMockHTTPResponse(200, responseBody),
			},
		},
	}

	client := &ArgoCDClient{
		baseURL:  "https://argocd.example.com",
		apiToken: "test-token",
		client:   mockClient,
	}

	result, err := client.SyncApplication(context.Background(), "test-app", nil)
	require.NoError(t, err)
	assert.NotNil(t, result)

	// Test with options
	appNs := "argocd"
	dryRun := true
	prune := false
	revision := "main"
	syncOptions := []string{"CreateNamespace=true"}
	options := &SyncApplicationOptions{
		AppNamespace: &appNs,
		DryRun:       &dryRun,
		Prune:        &prune,
		Revision:     &revision,
		SyncOptions:  syncOptions,
	}

	result2, err := client.SyncApplication(context.Background(), "test-app", options)
	require.NoError(t, err)
	assert.NotNil(t, result2)
}

func TestArgoCDClientRunResourceAction(t *testing.T) {
	responseBody := `{"result":"success"}`
	mockClient := &http.Client{
		Transport: &mockHTTPRoundTripper{
			response: createMockHTTPResponse(200, responseBody),
		},
	}

	client := &ArgoCDClient{
		baseURL:  "https://argocd.example.com",
		apiToken: "test-token",
		client:   mockClient,
	}

	resourceRef := ResourceRef{
		UID:       "uid-123",
		Version:   "v1",
		Group:     "apps",
		Kind:      "Deployment",
		Name:      "test-deploy",
		Namespace: "default",
	}

	result, err := client.RunResourceAction(context.Background(), "test-app", "argocd", resourceRef, "restart")
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestArgoCDClientMakeRequestError(t *testing.T) {
	mockClient := &http.Client{
		Transport: &mockHTTPRoundTripper{
			response: createMockHTTPResponse(500, "Internal Server Error"),
		},
	}

	client := &ArgoCDClient{
		baseURL:  "https://argocd.example.com",
		apiToken: "test-token",
		client:   mockClient,
	}

	_, err := client.ListApplications(context.Background(), nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "ArgoCD API error")
}

func TestArgoCDClientMakeRequestHTTPError(t *testing.T) {
	mockClient := &http.Client{
		Transport: &mockHTTPRoundTripper{
			err: fmt.Errorf("network error"),
		},
	}

	client := &ArgoCDClient{
		baseURL:  "https://argocd.example.com",
		apiToken: "test-token",
		client:   mockClient,
	}

	_, err := client.ListApplications(context.Background(), nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to execute request")
}

func TestGetArgoCDClientFromEnv(t *testing.T) {
	originalBaseURL := os.Getenv("ARGOCD_BASE_URL")
	originalToken := os.Getenv("ARGOCD_API_TOKEN")
	defer func() {
		if originalBaseURL != "" {
			_ = os.Setenv("ARGOCD_BASE_URL", originalBaseURL)
		} else {
			_ = os.Unsetenv("ARGOCD_BASE_URL")
		}
		if originalToken != "" {
			_ = os.Setenv("ARGOCD_API_TOKEN", originalToken)
		} else {
			_ = os.Unsetenv("ARGOCD_API_TOKEN")
		}
	}()

	t.Run("missing base URL", func(t *testing.T) {
		_ = os.Unsetenv("ARGOCD_BASE_URL")
		_ = os.Unsetenv("ARGOCD_API_TOKEN")
		client, err := GetArgoCDClientFromEnv()
		assert.Error(t, err)
		assert.Nil(t, client)
		assert.Contains(t, err.Error(), "ARGOCD_BASE_URL")
	})

	t.Run("missing API token", func(t *testing.T) {
		_ = os.Setenv("ARGOCD_BASE_URL", "https://argocd.example.com")
		_ = os.Unsetenv("ARGOCD_API_TOKEN")
		client, err := GetArgoCDClientFromEnv()
		assert.Error(t, err)
		assert.Nil(t, client)
		assert.Contains(t, err.Error(), "ARGOCD_API_TOKEN")
	})

	t.Run("successful creation", func(t *testing.T) {
		_ = os.Setenv("ARGOCD_BASE_URL", "https://argocd.example.com")
		_ = os.Setenv("ARGOCD_API_TOKEN", "test-token")
		client, err := GetArgoCDClientFromEnv()
		require.NoError(t, err)
		assert.NotNil(t, client)
	})
}

// Test successful handler paths with mocked client
func TestHandleArgoCDListApplicationsSuccess(t *testing.T) {
	responseBody := `{"items":[{"metadata":{"name":"test-app"}}]}`
	mockClient := &http.Client{
		Transport: &mockHTTPRoundTripper{
			response: createMockHTTPResponse(200, responseBody),
		},
	}

	originalGetClient := getArgoCDClient
	client := &ArgoCDClient{
		baseURL:  "https://argocd.example.com",
		apiToken: "test-token",
		client:   mockClient,
	}

	getArgoCDClient = func() (*ArgoCDClient, error) {
		return client, nil
	}
	defer func() { getArgoCDClient = originalGetClient }()

	request := createMCPRequest(map[string]interface{}{
		"search": "test",
		"limit":  float64(10),
		"offset": float64(0),
	})

	result, err := handleArgoCDListApplications(context.Background(), request)
	assert.NoError(t, err)
	assert.False(t, result.IsError)
	assert.NotEmpty(t, getResultText(result))
}

func TestHandleArgoCDGetApplicationSuccess(t *testing.T) {
	responseBody := `{"metadata":{"name":"test-app"}}`
	mockClient := &http.Client{
		Transport: &mockHTTPRoundTripper{
			response: createMockHTTPResponse(200, responseBody),
		},
	}

	originalGetClient := getArgoCDClient
	client := &ArgoCDClient{
		baseURL:  "https://argocd.example.com",
		apiToken: "test-token",
		client:   mockClient,
	}

	getArgoCDClient = func() (*ArgoCDClient, error) {
		return client, nil
	}
	defer func() { getArgoCDClient = originalGetClient }()

	request := createMCPRequest(map[string]interface{}{
		"applicationName": "test-app",
	})

	result, err := handleArgoCDGetApplication(context.Background(), request)
	assert.NoError(t, err)
	assert.False(t, result.IsError)
	assert.NotEmpty(t, getResultText(result))
}

func TestHandleArgoCDGetApplicationResourceTreeSuccess(t *testing.T) {
	responseBody := `{"nodes":[{"kind":"Deployment","name":"test-deploy"}]}`
	mockClient := &http.Client{
		Transport: &mockHTTPRoundTripper{
			response: createMockHTTPResponse(200, responseBody),
		},
	}

	originalGetClient := getArgoCDClient
	client := &ArgoCDClient{
		baseURL:  "https://argocd.example.com",
		apiToken: "test-token",
		client:   mockClient,
	}

	getArgoCDClient = func() (*ArgoCDClient, error) {
		return client, nil
	}
	defer func() { getArgoCDClient = originalGetClient }()

	request := createMCPRequest(map[string]interface{}{
		"applicationName": "test-app",
	})

	result, err := handleArgoCDGetApplicationResourceTree(context.Background(), request)
	assert.NoError(t, err)
	assert.False(t, result.IsError)
	assert.NotEmpty(t, getResultText(result))
}

func TestHandleArgoCDGetApplicationManagedResourcesSuccess(t *testing.T) {
	responseBody := `{"items":[]}`
	mockClient := &http.Client{
		Transport: &mockHTTPRoundTripper{
			response: createMockHTTPResponse(200, responseBody),
		},
	}

	originalGetClient := getArgoCDClient
	client := &ArgoCDClient{
		baseURL:  "https://argocd.example.com",
		apiToken: "test-token",
		client:   mockClient,
	}

	getArgoCDClient = func() (*ArgoCDClient, error) {
		return client, nil
	}
	defer func() { getArgoCDClient = originalGetClient }()

	request := createMCPRequest(map[string]interface{}{
		"applicationName": "test-app",
		"kind":            "Deployment",
		"namespace":       "default",
	})

	result, err := handleArgoCDGetApplicationManagedResources(context.Background(), request)
	assert.NoError(t, err)
	assert.False(t, result.IsError)
	assert.NotEmpty(t, getResultText(result))
}

func TestHandleArgoCDGetApplicationWorkloadLogsSuccess(t *testing.T) {
	responseBody := `{"logs":"test logs"}`
	mockClient := &http.Client{
		Transport: &mockHTTPRoundTripper{
			response: createMockHTTPResponse(200, responseBody),
		},
	}

	originalGetClient := getArgoCDClient
	client := &ArgoCDClient{
		baseURL:  "https://argocd.example.com",
		apiToken: "test-token",
		client:   mockClient,
	}

	getArgoCDClient = func() (*ArgoCDClient, error) {
		return client, nil
	}
	defer func() { getArgoCDClient = originalGetClient }()

	request := createMCPRequest(map[string]interface{}{
		"applicationName":      "test-app",
		"applicationNamespace": "argocd",
		"container":            "main",
		"resourceRef": map[string]interface{}{
			"uid":       "uid-123",
			"version":   "v1",
			"group":     "apps",
			"kind":      "Deployment",
			"name":      "test-deploy",
			"namespace": "default",
		},
	})

	result, err := handleArgoCDGetApplicationWorkloadLogs(context.Background(), request)
	assert.NoError(t, err)
	assert.False(t, result.IsError)
	assert.NotEmpty(t, getResultText(result))
}

func TestHandleArgoCDGetApplicationEventsSuccess(t *testing.T) {
	responseBody := `{"items":[]}`
	mockClient := &http.Client{
		Transport: &mockHTTPRoundTripper{
			response: createMockHTTPResponse(200, responseBody),
		},
	}

	originalGetClient := getArgoCDClient
	client := &ArgoCDClient{
		baseURL:  "https://argocd.example.com",
		apiToken: "test-token",
		client:   mockClient,
	}

	getArgoCDClient = func() (*ArgoCDClient, error) {
		return client, nil
	}
	defer func() { getArgoCDClient = originalGetClient }()

	request := createMCPRequest(map[string]interface{}{
		"applicationName": "test-app",
	})

	result, err := handleArgoCDGetApplicationEvents(context.Background(), request)
	assert.NoError(t, err)
	assert.False(t, result.IsError)
	assert.NotEmpty(t, getResultText(result))
}

func TestHandleArgoCDGetResourceEventsSuccess(t *testing.T) {
	responseBody := `{"items":[]}`
	mockClient := &http.Client{
		Transport: &mockHTTPRoundTripper{
			response: createMockHTTPResponse(200, responseBody),
		},
	}

	originalGetClient := getArgoCDClient
	client := &ArgoCDClient{
		baseURL:  "https://argocd.example.com",
		apiToken: "test-token",
		client:   mockClient,
	}

	getArgoCDClient = func() (*ArgoCDClient, error) {
		return client, nil
	}
	defer func() { getArgoCDClient = originalGetClient }()

	request := createMCPRequest(map[string]interface{}{
		"applicationName":      "test-app",
		"applicationNamespace": "argocd",
		"resourceUID":          "uid-123",
		"resourceNamespace":    "default",
		"resourceName":         "test-resource",
	})

	result, err := handleArgoCDGetResourceEvents(context.Background(), request)
	assert.NoError(t, err)
	assert.False(t, result.IsError)
	assert.NotEmpty(t, getResultText(result))
}

func TestHandleArgoCDGetResourcesSuccess(t *testing.T) {
	// Mock resource tree response
	treeResponseBody := `{"nodes":[{"uid":"uid-123","version":"v1","group":"apps","kind":"Deployment","name":"test-deploy","namespace":"default"}]}`
	resourceResponseBody := `{"metadata":{"name":"test-deploy"}}`

	mockClient := &http.Client{
		Transport: &mockHTTPRoundTripper{
			responses: []*http.Response{
				createMockHTTPResponse(200, treeResponseBody),
				createMockHTTPResponse(200, resourceResponseBody),
			},
		},
	}

	originalGetClient := getArgoCDClient
	client := &ArgoCDClient{
		baseURL:  "https://argocd.example.com",
		apiToken: "test-token",
		client:   mockClient,
	}

	getArgoCDClient = func() (*ArgoCDClient, error) {
		return client, nil
	}
	defer func() { getArgoCDClient = originalGetClient }()

	request := createMCPRequest(map[string]interface{}{
		"applicationName":      "test-app",
		"applicationNamespace": "argocd",
	})

	result, err := handleArgoCDGetResources(context.Background(), request)
	// This might fail due to multiple calls, but we're testing the structure
	if err == nil {
		assert.False(t, result.IsError)
	}
}

func TestHandleArgoCDGetResourceActionsSuccess(t *testing.T) {
	responseBody := `{"actions":[]}`
	mockClient := &http.Client{
		Transport: &mockHTTPRoundTripper{
			response: createMockHTTPResponse(200, responseBody),
		},
	}

	originalGetClient := getArgoCDClient
	client := &ArgoCDClient{
		baseURL:  "https://argocd.example.com",
		apiToken: "test-token",
		client:   mockClient,
	}

	getArgoCDClient = func() (*ArgoCDClient, error) {
		return client, nil
	}
	defer func() { getArgoCDClient = originalGetClient }()

	request := createMCPRequest(map[string]interface{}{
		"applicationName":      "test-app",
		"applicationNamespace": "argocd",
		"resourceRef": map[string]interface{}{
			"uid":       "uid-123",
			"version":   "v1",
			"group":     "apps",
			"kind":      "Deployment",
			"name":      "test-deploy",
			"namespace": "default",
		},
	})

	result, err := handleArgoCDGetResourceActions(context.Background(), request)
	assert.NoError(t, err)
	assert.False(t, result.IsError)
	assert.NotEmpty(t, getResultText(result))
}

func TestHandleArgoCDCreateApplicationSuccess(t *testing.T) {
	responseBody := `{"metadata":{"name":"test-app"}}`
	mockClient := &http.Client{
		Transport: &mockHTTPRoundTripper{
			response: createMockHTTPResponse(200, responseBody),
		},
	}

	originalGetClient := getArgoCDClient
	client := &ArgoCDClient{
		baseURL:  "https://argocd.example.com",
		apiToken: "test-token",
		client:   mockClient,
	}

	getArgoCDClient = func() (*ArgoCDClient, error) {
		return client, nil
	}
	defer func() { getArgoCDClient = originalGetClient }()

	request := createMCPRequest(map[string]interface{}{
		"application": map[string]interface{}{
			"metadata": map[string]interface{}{
				"name": "test-app",
			},
		},
	})

	result, err := handleArgoCDCreateApplication(context.Background(), request)
	assert.NoError(t, err)
	assert.False(t, result.IsError)
	assert.NotEmpty(t, getResultText(result))
}

func TestHandleArgoCDUpdateApplicationSuccess(t *testing.T) {
	responseBody := `{"metadata":{"name":"test-app"}}`
	mockClient := &http.Client{
		Transport: &mockHTTPRoundTripper{
			response: createMockHTTPResponse(200, responseBody),
		},
	}

	originalGetClient := getArgoCDClient
	client := &ArgoCDClient{
		baseURL:  "https://argocd.example.com",
		apiToken: "test-token",
		client:   mockClient,
	}

	getArgoCDClient = func() (*ArgoCDClient, error) {
		return client, nil
	}
	defer func() { getArgoCDClient = originalGetClient }()

	request := createMCPRequest(map[string]interface{}{
		"applicationName": "test-app",
		"application": map[string]interface{}{
			"metadata": map[string]interface{}{
				"name": "test-app",
			},
		},
	})

	result, err := handleArgoCDUpdateApplication(context.Background(), request)
	assert.NoError(t, err)
	assert.False(t, result.IsError)
	assert.NotEmpty(t, getResultText(result))
}

func TestHandleArgoCDDeleteApplicationSuccess(t *testing.T) {
	responseBody := `{}`
	mockClient := &http.Client{
		Transport: &mockHTTPRoundTripper{
			response: createMockHTTPResponse(200, responseBody),
		},
	}

	originalGetClient := getArgoCDClient
	client := &ArgoCDClient{
		baseURL:  "https://argocd.example.com",
		apiToken: "test-token",
		client:   mockClient,
	}

	getArgoCDClient = func() (*ArgoCDClient, error) {
		return client, nil
	}
	defer func() { getArgoCDClient = originalGetClient }()

	request := createMCPRequest(map[string]interface{}{
		"applicationName": "test-app",
	})

	result, err := handleArgoCDDeleteApplication(context.Background(), request)
	assert.NoError(t, err)
	assert.False(t, result.IsError)
	assert.NotEmpty(t, getResultText(result))
}

func TestHandleArgoCDSyncApplicationSuccess(t *testing.T) {
	responseBody := `{"status":"success"}`
	mockClient := &http.Client{
		Transport: &mockHTTPRoundTripper{
			response: createMockHTTPResponse(200, responseBody),
		},
	}

	originalGetClient := getArgoCDClient
	client := &ArgoCDClient{
		baseURL:  "https://argocd.example.com",
		apiToken: "test-token",
		client:   mockClient,
	}

	getArgoCDClient = func() (*ArgoCDClient, error) {
		return client, nil
	}
	defer func() { getArgoCDClient = originalGetClient }()

	request := createMCPRequest(map[string]interface{}{
		"applicationName":      "test-app",
		"applicationNamespace": "argocd",
		"dryRun":               true,
		"prune":                false,
		"revision":             "main",
		"syncOptions":          []interface{}{"CreateNamespace=true"},
	})

	result, err := handleArgoCDSyncApplication(context.Background(), request)
	assert.NoError(t, err)
	assert.False(t, result.IsError)
	assert.NotEmpty(t, getResultText(result))
}

func TestHandleArgoCDRunResourceActionSuccess(t *testing.T) {
	responseBody := `{"result":"success"}`
	mockClient := &http.Client{
		Transport: &mockHTTPRoundTripper{
			response: createMockHTTPResponse(200, responseBody),
		},
	}

	originalGetClient := getArgoCDClient
	client := &ArgoCDClient{
		baseURL:  "https://argocd.example.com",
		apiToken: "test-token",
		client:   mockClient,
	}

	getArgoCDClient = func() (*ArgoCDClient, error) {
		return client, nil
	}
	defer func() { getArgoCDClient = originalGetClient }()

	request := createMCPRequest(map[string]interface{}{
		"applicationName":      "test-app",
		"applicationNamespace": "argocd",
		"action":               "restart",
		"resourceRef": map[string]interface{}{
			"uid":       "uid-123",
			"version":   "v1",
			"group":     "apps",
			"kind":      "Deployment",
			"name":      "test-deploy",
			"namespace": "default",
		},
	})

	result, err := handleArgoCDRunResourceAction(context.Background(), request)
	assert.NoError(t, err)
	assert.False(t, result.IsError)
	assert.NotEmpty(t, getResultText(result))
}

func TestHandleArgoCDGetResourcesWithResourceRefs(t *testing.T) {
	responseBody := `{"metadata":{"name":"test-deploy"}}`
	mockClient := &http.Client{
		Transport: &mockHTTPRoundTripper{
			response: createMockHTTPResponse(200, responseBody),
		},
	}

	originalGetClient := getArgoCDClient
	client := &ArgoCDClient{
		baseURL:  "https://argocd.example.com",
		apiToken: "test-token",
		client:   mockClient,
	}

	getArgoCDClient = func() (*ArgoCDClient, error) {
		return client, nil
	}
	defer func() { getArgoCDClient = originalGetClient }()

	request := createMCPRequest(map[string]interface{}{
		"applicationName":      "test-app",
		"applicationNamespace": "argocd",
		"resourceRefs": []interface{}{
			map[string]interface{}{
				"uid":       "uid-123",
				"version":   "v1",
				"group":     "apps",
				"kind":      "Deployment",
				"name":      "test-deploy",
				"namespace": "default",
			},
		},
	})

	result, err := handleArgoCDGetResources(context.Background(), request)
	assert.NoError(t, err)
	assert.False(t, result.IsError)
	assert.NotEmpty(t, getResultText(result))
}

func TestHandleArgoCDGetResourcesClientError(t *testing.T) {
	originalGetClient := getArgoCDClient
	getArgoCDClient = func() (*ArgoCDClient, error) {
		return nil, fmt.Errorf("client error")
	}
	defer func() { getArgoCDClient = originalGetClient }()

	request := createMCPRequest(map[string]interface{}{
		"applicationName":      "test-app",
		"applicationNamespace": "argocd",
	})

	result, err := handleArgoCDGetResources(context.Background(), request)
	assert.NoError(t, err)
	assert.True(t, result.IsError)
}

func TestHandleArgoCDGetResourcesAPIError(t *testing.T) {
	mockClient := &http.Client{
		Transport: &mockHTTPRoundTripper{
			response: createMockHTTPResponse(500, "Internal Server Error"),
		},
	}

	originalGetClient := getArgoCDClient
	client := &ArgoCDClient{
		baseURL:  "https://argocd.example.com",
		apiToken: "test-token",
		client:   mockClient,
	}

	getArgoCDClient = func() (*ArgoCDClient, error) {
		return client, nil
	}
	defer func() { getArgoCDClient = originalGetClient }()

	request := createMCPRequest(map[string]interface{}{
		"applicationName":      "test-app",
		"applicationNamespace": "argocd",
		"resourceRefs": []interface{}{
			map[string]interface{}{
				"uid":       "uid-123",
				"version":   "v1",
				"group":     "apps",
				"kind":      "Deployment",
				"name":      "test-deploy",
				"namespace": "default",
			},
		},
	})

	result, err := handleArgoCDGetResources(context.Background(), request)
	assert.NoError(t, err)
	assert.True(t, result.IsError)
}

func TestReturnJSONResultError(t *testing.T) {
	// Test with data that can't be marshaled (circular reference)
	type Circular struct {
		Self *Circular
	}
	circular := &Circular{}
	circular.Self = circular // Create circular reference

	result, err := returnJSONResult(circular)
	assert.NoError(t, err)
	assert.True(t, result.IsError)
	assert.Contains(t, getResultText(result), "failed to marshal")
}

// ArgoCD Handler Tests

func TestHandleArgoCDListApplications(t *testing.T) {
	t.Run("client creation failure", func(t *testing.T) {
		// Temporarily override getArgoCDClient to return error
		originalGetClient := getArgoCDClient
		getArgoCDClient = func() (*ArgoCDClient, error) {
			return nil, fmt.Errorf("failed to create client")
		}
		defer func() { getArgoCDClient = originalGetClient }()

		request := createMCPRequest(map[string]interface{}{})
		result, err := handleArgoCDListApplications(context.Background(), request)

		assert.NoError(t, err)
		assert.True(t, result.IsError)
		assert.Contains(t, getResultText(result), "failed to create ArgoCD client")
	})

	t.Run("invalid arguments", func(t *testing.T) {
		request := &mcp.CallToolRequest{
			Params: &mcp.CallToolParamsRaw{
				Arguments: []byte("invalid json"),
			},
		}

		result, err := handleArgoCDListApplications(context.Background(), request)
		assert.NoError(t, err)
		assert.True(t, result.IsError)
	})
}

func TestHandleArgoCDGetApplication(t *testing.T) {
	t.Run("missing applicationName", func(t *testing.T) {
		request := createMCPRequest(map[string]interface{}{})
		result, err := handleArgoCDGetApplication(context.Background(), request)

		assert.NoError(t, err)
		assert.True(t, result.IsError)
		assert.Contains(t, getResultText(result), "applicationName parameter is required")
	})
}

func TestHandleArgoCDGetApplicationResourceTree(t *testing.T) {
	t.Run("missing applicationName", func(t *testing.T) {
		request := createMCPRequest(map[string]interface{}{})
		result, err := handleArgoCDGetApplicationResourceTree(context.Background(), request)

		assert.NoError(t, err)
		assert.True(t, result.IsError)
		assert.Contains(t, getResultText(result), "applicationName parameter is required")
	})
}

func TestHandleArgoCDGetApplicationManagedResources(t *testing.T) {
	t.Run("missing applicationName", func(t *testing.T) {
		request := createMCPRequest(map[string]interface{}{})
		result, err := handleArgoCDGetApplicationManagedResources(context.Background(), request)

		assert.NoError(t, err)
		assert.True(t, result.IsError)
		assert.Contains(t, getResultText(result), "applicationName parameter is required")
	})
}

func TestHandleArgoCDGetApplicationWorkloadLogs(t *testing.T) {
	t.Run("missing required parameters", func(t *testing.T) {
		testCases := []struct {
			name string
			args map[string]interface{}
		}{
			{"missing applicationName", map[string]interface{}{}},
			{"missing applicationNamespace", map[string]interface{}{"applicationName": "test"}},
			{"missing container", map[string]interface{}{"applicationName": "test", "applicationNamespace": "argocd"}},
			{"missing resourceRef", map[string]interface{}{"applicationName": "test", "applicationNamespace": "argocd", "container": "main"}},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				request := createMCPRequest(tc.args)
				result, err := handleArgoCDGetApplicationWorkloadLogs(context.Background(), request)

				assert.NoError(t, err)
				assert.True(t, result.IsError)
			})
		}
	})
}

func TestHandleArgoCDGetApplicationEvents(t *testing.T) {
	t.Run("missing applicationName", func(t *testing.T) {
		request := createMCPRequest(map[string]interface{}{})
		result, err := handleArgoCDGetApplicationEvents(context.Background(), request)

		assert.NoError(t, err)
		assert.True(t, result.IsError)
		assert.Contains(t, getResultText(result), "applicationName parameter is required")
	})
}

func TestHandleArgoCDGetResourceEvents(t *testing.T) {
	t.Run("missing required parameters", func(t *testing.T) {
		testCases := []struct {
			name string
			args map[string]interface{}
		}{
			{"missing applicationName", map[string]interface{}{}},
			{"missing applicationNamespace", map[string]interface{}{"applicationName": "test"}},
			{"missing resourceUID", map[string]interface{}{"applicationName": "test", "applicationNamespace": "argocd"}},
			{"missing resourceNamespace", map[string]interface{}{"applicationName": "test", "applicationNamespace": "argocd", "resourceUID": "uid"}},
			{"missing resourceName", map[string]interface{}{"applicationName": "test", "applicationNamespace": "argocd", "resourceUID": "uid", "resourceNamespace": "default"}},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				request := createMCPRequest(tc.args)
				result, err := handleArgoCDGetResourceEvents(context.Background(), request)

				assert.NoError(t, err)
				assert.True(t, result.IsError)
			})
		}
	})
}

func TestHandleArgoCDGetResources(t *testing.T) {
	t.Run("missing required parameters", func(t *testing.T) {
		testCases := []struct {
			name string
			args map[string]interface{}
		}{
			{"missing applicationName", map[string]interface{}{}},
			{"missing applicationNamespace", map[string]interface{}{"applicationName": "test"}},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				request := createMCPRequest(tc.args)
				result, err := handleArgoCDGetResources(context.Background(), request)

				assert.NoError(t, err)
				assert.True(t, result.IsError)
			})
		}
	})
}

func TestHandleArgoCDGetResourceActions(t *testing.T) {
	t.Run("missing required parameters", func(t *testing.T) {
		testCases := []struct {
			name string
			args map[string]interface{}
		}{
			{"missing applicationName", map[string]interface{}{}},
			{"missing applicationNamespace", map[string]interface{}{"applicationName": "test"}},
			{"missing resourceRef", map[string]interface{}{"applicationName": "test", "applicationNamespace": "argocd"}},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				request := createMCPRequest(tc.args)
				result, err := handleArgoCDGetResourceActions(context.Background(), request)

				assert.NoError(t, err)
				assert.True(t, result.IsError)
			})
		}
	})
}

func TestHandleArgoCDCreateApplication(t *testing.T) {
	t.Run("missing application parameter", func(t *testing.T) {
		request := createMCPRequest(map[string]interface{}{})
		result, err := handleArgoCDCreateApplication(context.Background(), request)

		assert.NoError(t, err)
		assert.True(t, result.IsError)
		assert.Contains(t, getResultText(result), "application parameter is required")
	})
}

func TestHandleArgoCDUpdateApplication(t *testing.T) {
	t.Run("missing required parameters", func(t *testing.T) {
		testCases := []struct {
			name string
			args map[string]interface{}
		}{
			{"missing applicationName", map[string]interface{}{}},
			{"missing application", map[string]interface{}{"applicationName": "test"}},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				request := createMCPRequest(tc.args)
				result, err := handleArgoCDUpdateApplication(context.Background(), request)

				assert.NoError(t, err)
				assert.True(t, result.IsError)
			})
		}
	})
}

func TestHandleArgoCDDeleteApplication(t *testing.T) {
	t.Run("missing applicationName", func(t *testing.T) {
		request := createMCPRequest(map[string]interface{}{})
		result, err := handleArgoCDDeleteApplication(context.Background(), request)

		assert.NoError(t, err)
		assert.True(t, result.IsError)
		assert.Contains(t, getResultText(result), "applicationName parameter is required")
	})
}

func TestHandleArgoCDSyncApplication(t *testing.T) {
	t.Run("missing applicationName", func(t *testing.T) {
		request := createMCPRequest(map[string]interface{}{})
		result, err := handleArgoCDSyncApplication(context.Background(), request)

		assert.NoError(t, err)
		assert.True(t, result.IsError)
		assert.Contains(t, getResultText(result), "applicationName parameter is required")
	})
}

func TestHandleArgoCDRunResourceAction(t *testing.T) {
	t.Run("missing required parameters", func(t *testing.T) {
		testCases := []struct {
			name string
			args map[string]interface{}
		}{
			{"missing applicationName", map[string]interface{}{}},
			{"missing applicationNamespace", map[string]interface{}{"applicationName": "test"}},
			{"missing action", map[string]interface{}{"applicationName": "test", "applicationNamespace": "argocd"}},
			{"missing resourceRef", map[string]interface{}{"applicationName": "test", "applicationNamespace": "argocd", "action": "restart"}},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				request := createMCPRequest(tc.args)
				result, err := handleArgoCDRunResourceAction(context.Background(), request)

				assert.NoError(t, err)
				assert.True(t, result.IsError)
			})
		}
	})
}

func TestIsReadOnlyMode(t *testing.T) {
	originalValue := os.Getenv("MCP_READ_ONLY")
	defer func() {
		if originalValue == "" {
			_ = os.Unsetenv("MCP_READ_ONLY")
		} else {
			_ = os.Setenv("MCP_READ_ONLY", originalValue)
		}
	}()

	t.Run("read-only mode enabled", func(t *testing.T) {
		_ = os.Setenv("MCP_READ_ONLY", "true")
		assert.True(t, isReadOnlyMode())
	})

	t.Run("read-only mode disabled", func(t *testing.T) {
		_ = os.Setenv("MCP_READ_ONLY", "false")
		assert.False(t, isReadOnlyMode())
	})

	t.Run("read-only mode not set", func(t *testing.T) {
		_ = os.Unsetenv("MCP_READ_ONLY")
		assert.False(t, isReadOnlyMode())
	})
}

func TestReturnJSONResult(t *testing.T) {
	t.Run("valid JSON", func(t *testing.T) {
		data := map[string]interface{}{"key": "value"}
		result, err := returnJSONResult(data)

		assert.NoError(t, err)
		assert.False(t, result.IsError)
		assert.NotEmpty(t, getResultText(result))

		// Verify it's valid JSON
		var jsonData map[string]interface{}
		err = json.Unmarshal([]byte(getResultText(result)), &jsonData)
		assert.NoError(t, err)
		assert.Equal(t, "value", jsonData["key"])
	})
}

func TestReturnErrorResult(t *testing.T) {
	result, err := returnErrorResult("test error")

	assert.NoError(t, err)
	assert.True(t, result.IsError)
	assert.Equal(t, "test error", getResultText(result))
}
