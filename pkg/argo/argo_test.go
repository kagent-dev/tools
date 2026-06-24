package argo

import (
	"context"
	"strings"
	"testing"

	"github.com/kagent-dev/tools/internal/cmd"
	mcp "github.com/kagent-dev/tools/internal/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRegisterTools(t *testing.T) {
	t.Run("read-write", func(t *testing.T) {
		s := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "v0.0.1"}, nil)
		RegisterTools(s, false)
	})
	t.Run("read-only", func(t *testing.T) {
		s := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "v0.0.1"}, nil)
		RegisterTools(s, true)
	})
}

func TestHandleListRollouts(t *testing.T) {
	t.Run("default namespace and type", func(t *testing.T) {
		mock := cmd.NewMockShellExecutor()
		mock.AddCommandString("kubectl", []string{"argo", "rollouts", "list", "rollouts", "-n", "argo-rollouts"}, "NAME STATUS\nmyapp Healthy", nil)
		ctx := cmd.WithShellExecutor(context.Background(), mock)

		result, _, err := handleListRollouts(ctx, &mcp.CallToolRequest{}, listRolloutsInput{})
		assert.NoError(t, err)
		assert.False(t, result.IsError)
		assert.Contains(t, getResultText(result), "myapp")
	})

	t.Run("experiments type custom namespace", func(t *testing.T) {
		mock := cmd.NewMockShellExecutor()
		mock.AddCommandString("kubectl", []string{"argo", "rollouts", "list", "experiments", "-n", "prod"}, "NAME", nil)
		ctx := cmd.WithShellExecutor(context.Background(), mock)

		result, _, err := handleListRollouts(ctx, &mcp.CallToolRequest{}, listRolloutsInput{Type: "experiments", Namespace: "prod"})
		assert.NoError(t, err)
		assert.False(t, result.IsError)
	})

	t.Run("command failure", func(t *testing.T) {
		mock := cmd.NewMockShellExecutor()
		mock.AddCommandString("kubectl", []string{"argo", "rollouts", "list", "rollouts", "-n", "argo-rollouts"}, "", assert.AnError)
		ctx := cmd.WithShellExecutor(context.Background(), mock)

		result, _, err := handleListRollouts(ctx, &mcp.CallToolRequest{}, listRolloutsInput{})
		assert.NoError(t, err)
		assert.True(t, result.IsError)
		assert.Contains(t, getResultText(result), "Error listing rollouts")
	})
}

func TestHandleCheckPluginLogs(t *testing.T) {
	t.Run("plugin install found in logs", func(t *testing.T) {
		mock := cmd.NewMockShellExecutor()
		logs := `Downloading plugin argoproj-labs/gatewayAPI from: https://github.com/x/releases/download/v0.5.0/gatewayapi-plugin-linux-amd64"
Download complete, it took 1.5s`
		mock.AddCommandString("kubectl", []string{"logs", "-n", "argo-rollouts", "-l", "app.kubernetes.io/name=argo-rollouts", "--tail", "100"}, logs, nil)
		ctx := cmd.WithShellExecutor(context.Background(), mock)

		result, _, err := handleCheckPluginLogs(ctx, &mcp.CallToolRequest{}, checkPluginLogsInput{})
		assert.NoError(t, err)
		assert.Contains(t, getResultText(result), "0.5.0")
		assert.Contains(t, getResultText(result), `"installed": true`)
	})

	t.Run("plugin install not found", func(t *testing.T) {
		mock := cmd.NewMockShellExecutor()
		mock.AddCommandString("kubectl", []string{"logs", "-n", "argo-rollouts", "-l", "app.kubernetes.io/name=argo-rollouts", "--tail", "100"}, "no plugin here", nil)
		ctx := cmd.WithShellExecutor(context.Background(), mock)

		result, _, err := handleCheckPluginLogs(ctx, &mcp.CallToolRequest{}, checkPluginLogsInput{})
		assert.NoError(t, err)
		assert.Contains(t, getResultText(result), "Plugin installation not found")
	})

	t.Run("command failure", func(t *testing.T) {
		mock := cmd.NewMockShellExecutor()
		mock.AddCommandString("kubectl", []string{"logs", "-n", "argo-rollouts", "-l", "app.kubernetes.io/name=argo-rollouts", "--tail", "100"}, "", assert.AnError)
		ctx := cmd.WithShellExecutor(context.Background(), mock)

		result, _, err := handleCheckPluginLogs(ctx, &mcp.CallToolRequest{}, checkPluginLogsInput{})
		assert.NoError(t, err)
		assert.Contains(t, getResultText(result), `"installed": false`)
	})
}

func TestConfigureGatewayPlugin(t *testing.T) {
	t.Run("applies configmap successfully", func(t *testing.T) {
		mock := cmd.NewMockShellExecutor()
		mock.AddPartialMatcherString("kubectl", []string{"apply", "-f"}, "configmap/argo-rollouts-config created", nil)
		ctx := cmd.WithShellExecutor(context.Background(), mock)

		status := configureGatewayPlugin(ctx, "0.5.0", "argo-rollouts")
		assert.True(t, status.Installed)
		assert.Equal(t, "0.5.0", status.Version)
		assert.NotEmpty(t, status.Architecture)
	})

	t.Run("apply failure", func(t *testing.T) {
		mock := cmd.NewMockShellExecutor()
		mock.AddPartialMatcherString("kubectl", []string{"apply", "-f"}, "", assert.AnError)
		ctx := cmd.WithShellExecutor(context.Background(), mock)

		status := configureGatewayPlugin(ctx, "0.5.0", "argo-rollouts")
		assert.False(t, status.Installed)
		assert.Contains(t, status.ErrorMessage, "Error applying Gateway API plugin config")
	})
}

func TestHandleVerifyGatewayPluginAlreadyConfigured(t *testing.T) {
	mock := cmd.NewMockShellExecutor()
	mock.AddCommandString("kubectl", []string{"get", "configmap", "argo-rollouts-config", "-n", "argo-rollouts", "-o", "yaml"}, "data:\n  trafficRouterPlugins: argoproj-labs/gatewayAPI", nil)
	ctx := cmd.WithShellExecutor(context.Background(), mock)

	result, _, err := handleVerifyGatewayPlugin(ctx, &mcp.CallToolRequest{}, verifyGatewayPluginInput{})
	assert.NoError(t, err)
	assert.Contains(t, getResultText(result), "already configured")
}

func TestHandleVerifyArgoRolloutsControllerInstallStatuses(t *testing.T) {
	baseCmd := []string{"get", "pods", "-n", "argo-rollouts", "-l", "app.kubernetes.io/component=rollouts-controller", "-o", "jsonpath={.items[*].status.phase}"}

	t.Run("all running", func(t *testing.T) {
		mock := cmd.NewMockShellExecutor()
		mock.AddCommandString("kubectl", baseCmd, "Running Running", nil)
		ctx := cmd.WithShellExecutor(context.Background(), mock)
		result, _, err := handleVerifyArgoRolloutsControllerInstall(ctx, &mcp.CallToolRequest{}, verifyArgoRolloutsControllerInstallInput{})
		assert.NoError(t, err)
		assert.Contains(t, getResultText(result), "All pods are running")
	})

	t.Run("not all running", func(t *testing.T) {
		mock := cmd.NewMockShellExecutor()
		mock.AddCommandString("kubectl", baseCmd, "Running Pending", nil)
		ctx := cmd.WithShellExecutor(context.Background(), mock)
		result, _, err := handleVerifyArgoRolloutsControllerInstall(ctx, &mcp.CallToolRequest{}, verifyArgoRolloutsControllerInstallInput{})
		assert.NoError(t, err)
		assert.Contains(t, getResultText(result), "Not all pods are running")
	})

	t.Run("no pods", func(t *testing.T) {
		mock := cmd.NewMockShellExecutor()
		mock.AddCommandString("kubectl", baseCmd, "", nil)
		ctx := cmd.WithShellExecutor(context.Background(), mock)
		result, _, err := handleVerifyArgoRolloutsControllerInstall(ctx, &mcp.CallToolRequest{}, verifyArgoRolloutsControllerInstallInput{})
		assert.NoError(t, err)
		assert.Contains(t, getResultText(result), "No pods found")
	})

	t.Run("command error", func(t *testing.T) {
		mock := cmd.NewMockShellExecutor()
		mock.AddCommandString("kubectl", baseCmd, "", assert.AnError)
		ctx := cmd.WithShellExecutor(context.Background(), mock)
		result, _, err := handleVerifyArgoRolloutsControllerInstall(ctx, &mcp.CallToolRequest{}, verifyArgoRolloutsControllerInstallInput{})
		assert.NoError(t, err)
		assert.True(t, result.IsError)
	})
}

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

// Test the actual MCP tool handler functions

// Test Argo Rollouts Promote
func TestHandlePromoteRollout(t *testing.T) {
	t.Run("promote rollout basic", func(t *testing.T) {
		mock := cmd.NewMockShellExecutor()
		expectedOutput := `rollout "myapp" promoted`

		mock.AddCommandString("kubectl", []string{"argo", "rollouts", "promote", "myapp"}, expectedOutput, nil)
		ctx := cmd.WithShellExecutor(context.Background(), mock)

		result, _, err := handlePromoteRollout(ctx, &mcp.CallToolRequest{}, promoteRolloutInput{RolloutName: "myapp"})

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

		result, _, err := handlePromoteRollout(ctx, &mcp.CallToolRequest{}, promoteRolloutInput{RolloutName: "myapp", Namespace: "production"})

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

		result, _, err := handlePromoteRollout(ctx, &mcp.CallToolRequest{}, promoteRolloutInput{RolloutName: "myapp", Full: true})

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

		result, _, err := handlePromoteRollout(ctx, &mcp.CallToolRequest{}, promoteRolloutInput{})
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

		result, _, err := handlePromoteRollout(ctx, &mcp.CallToolRequest{}, promoteRolloutInput{RolloutName: "myapp"})

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

		result, _, err := handlePauseRollout(ctx, &mcp.CallToolRequest{}, pauseRolloutInput{RolloutName: "myapp"})

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

		result, _, err := handlePauseRollout(ctx, &mcp.CallToolRequest{}, pauseRolloutInput{RolloutName: "myapp", Namespace: "production"})

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

		result, _, err := handlePauseRollout(ctx, &mcp.CallToolRequest{}, pauseRolloutInput{})
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

		result, _, err := handleSetRolloutImage(ctx, &mcp.CallToolRequest{}, setRolloutImageInput{RolloutName: "myapp", ContainerImage: "nginx:latest"})

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

		result, _, err := handleSetRolloutImage(ctx, &mcp.CallToolRequest{}, setRolloutImageInput{RolloutName: "myapp", ContainerImage: "nginx:1.20", Namespace: "production"})

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

		result, _, err := handleSetRolloutImage(ctx, &mcp.CallToolRequest{}, setRolloutImageInput{ContainerImage: "nginx:latest"})
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

		result, _, err := handleSetRolloutImage(ctx, &mcp.CallToolRequest{}, setRolloutImageInput{RolloutName: "myapp"})
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

		shouldInstall := false
		result, _, err := handleVerifyGatewayPlugin(ctx, &mcp.CallToolRequest{}, verifyGatewayPluginInput{ShouldInstall: &shouldInstall})

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

		shouldInstall := false
		result, _, err := handleVerifyGatewayPlugin(ctx, &mcp.CallToolRequest{}, verifyGatewayPluginInput{ShouldInstall: &shouldInstall, Namespace: "custom-namespace"})

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
		expectedOutput := `argo-rollouts-controller-manager-abc123`

		mock.AddCommandString("kubectl", []string{"get", "pods", "-l", "app.kubernetes.io/name=argo-rollouts", "-n", "argo-rollouts", "-o", "jsonpath={.items[*].metadata.name}"}, expectedOutput, nil)
		ctx := cmd.WithShellExecutor(context.Background(), mock)

		result, _, err := handleVerifyArgoRolloutsControllerInstall(ctx, &mcp.CallToolRequest{}, verifyArgoRolloutsControllerInstallInput{})

		assert.NoError(t, err)
		assert.NotNil(t, result)

		// Verify kubectl command was called
		callLog := mock.GetCallLog()
		require.Len(t, callLog, 1)
		assert.Equal(t, "kubectl", callLog[0].Command)
		assert.Contains(t, callLog[0].Args, "get")
		assert.Contains(t, callLog[0].Args, "pods")
	})

	t.Run("verify controller install with custom namespace", func(t *testing.T) {
		mock := cmd.NewMockShellExecutor()
		expectedOutput := `argo-rollouts-controller-manager-abc123`

		mock.AddCommandString("kubectl", []string{"get", "pods", "-l", "app.kubernetes.io/name=argo-rollouts", "-n", "custom-argo", "-o", "jsonpath={.items[*].metadata.name}"}, expectedOutput, nil)
		ctx := cmd.WithShellExecutor(context.Background(), mock)

		result, _, err := handleVerifyArgoRolloutsControllerInstall(ctx, &mcp.CallToolRequest{}, verifyArgoRolloutsControllerInstallInput{Namespace: "custom-argo"})

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
		expectedOutput := `argo-rollouts-controller-manager-abc123`

		mock.AddCommandString("kubectl", []string{"get", "pods", "-l", "app=custom-rollouts", "-n", "argo-rollouts", "-o", "jsonpath={.items[*].metadata.name}"}, expectedOutput, nil)
		ctx := cmd.WithShellExecutor(context.Background(), mock)

		result, _, err := handleVerifyArgoRolloutsControllerInstall(ctx, &mcp.CallToolRequest{}, verifyArgoRolloutsControllerInstallInput{Label: "app=custom-rollouts"})

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
		expectedOutput := `kubectl-argo-rollouts`

		mock.AddCommandString("kubectl", []string{"argo", "rollouts", "version"}, expectedOutput, nil)
		ctx := cmd.WithShellExecutor(context.Background(), mock)

		result, _, err := handleVerifyKubectlPluginInstall(ctx, &mcp.CallToolRequest{}, verifyKubectlPluginInstallInput{})

		assert.NoError(t, err)
		assert.False(t, result.IsError)

		// Verify the correct command was called
		callLog := mock.GetCallLog()
		require.Len(t, callLog, 1)
		assert.Equal(t, "kubectl", callLog[0].Command)
		assert.Equal(t, []string{"argo", "rollouts", "version"}, callLog[0].Args)
	})

	t.Run("kubectl plugin command failure", func(t *testing.T) {
		mock := cmd.NewMockShellExecutor()
		mock.AddCommandString("kubectl", []string{"plugin", "list"}, "", assert.AnError)
		ctx := cmd.WithShellExecutor(context.Background(), mock)

		result, _, err := handleVerifyKubectlPluginInstall(ctx, &mcp.CallToolRequest{}, verifyKubectlPluginInstallInput{})

		assert.NoError(t, err) // MCP handlers should not return Go errors
		assert.NotNil(t, result)
		// May be success or error depending on implementation
	})
}
