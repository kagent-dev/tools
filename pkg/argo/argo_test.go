package argo

import (
	"context"
	"encoding/json"
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
