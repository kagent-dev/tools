package cilium

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/kagent-dev/tools/internal/cmd"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Helper function to create MCP request with arguments
func createMCPRequest(args map[string]interface{}) *mcp.CallToolRequest {
	argsJSON, _ := json.Marshal(args)
	return &mcp.CallToolRequest{
		Params: &mcp.CallToolParamsRaw{
			Arguments: argsJSON,
		},
	}
}

// Note: RegisterTools test is skipped as it requires a properly initialized server

func TestHandleCiliumStatusAndVersion(t *testing.T) {
	ctx := context.Background()
	mock := cmd.NewMockShellExecutor()
	mock.AddCommandString("cilium", []string{"status"}, "Cilium status: OK", nil)
	mock.AddCommandString("cilium", []string{"version"}, "cilium version 1.14.0", nil)

	ctx = cmd.WithShellExecutor(ctx, mock)

	request := createMCPRequest(map[string]interface{}{})

	result, err := handleCiliumStatusAndVersion(ctx, request)
	require.NoError(t, err)
	assert.NotNil(t, result)
	assert.False(t, result.IsError)

	var textContent *mcp.TextContent
	var ok bool
	for _, content := range result.Content {
		if textContent, ok = content.(*mcp.TextContent); ok {
			break
		}
	}
	require.True(t, ok, "no text content in result")

	assert.Contains(t, textContent.Text, "Cilium status: OK")
	assert.Contains(t, textContent.Text, "cilium version 1.14.0")
}

func TestHandleCiliumStatusAndVersionError(t *testing.T) {
	ctx := context.Background()
	mock := cmd.NewMockShellExecutor()
	mock.AddCommandString("cilium", []string{"status"}, "", errors.New("command failed"))
	mock.AddCommandString("cilium", []string{"version"}, "cilium version 1.14.0", nil)

	ctx = cmd.WithShellExecutor(ctx, mock)

	request := createMCPRequest(map[string]interface{}{})

	result, err := handleCiliumStatusAndVersion(ctx, request)
	require.NoError(t, err)
	assert.NotNil(t, result)
	assert.True(t, result.IsError)
	assert.Contains(t, getResultText(result), "Error getting Cilium status")
}

func TestHandleInstallCilium(t *testing.T) {
	ctx := context.Background()
	mock := cmd.NewMockShellExecutor()
	mock.AddCommandString("cilium", []string{"install"}, "✓ Cilium was successfully installed!", nil)

	ctx = cmd.WithShellExecutor(ctx, mock)

	request := createMCPRequest(map[string]interface{}{})

	result, err := handleInstallCilium(ctx, request)
	require.NoError(t, err)
	assert.NotNil(t, result)
	assert.False(t, result.IsError)
	assert.Contains(t, getResultText(result), "✓ Cilium was successfully installed!")
}

func TestHandleUninstallCilium(t *testing.T) {
	ctx := context.Background()
	mock := cmd.NewMockShellExecutor()
	mock.AddCommandString("cilium", []string{"uninstall"}, "✓ Cilium was successfully uninstalled!", nil)

	ctx = cmd.WithShellExecutor(ctx, mock)

	request := createMCPRequest(map[string]interface{}{})

	result, err := handleUninstallCilium(ctx, request)
	require.NoError(t, err)
	assert.NotNil(t, result)
	assert.False(t, result.IsError)
	assert.Contains(t, getResultText(result), "✓ Cilium was successfully uninstalled!")
}

func TestHandleUpgradeCilium(t *testing.T) {
	ctx := context.Background()
	mock := cmd.NewMockShellExecutor()
	mock.AddCommandString("cilium", []string{"upgrade"}, "✓ Cilium was successfully upgraded!", nil)

	ctx = cmd.WithShellExecutor(ctx, mock)

	request := createMCPRequest(map[string]interface{}{})

	result, err := handleUpgradeCilium(ctx, request)
	require.NoError(t, err)
	assert.NotNil(t, result)
	assert.False(t, result.IsError)
	assert.Contains(t, getResultText(result), "✓ Cilium was successfully upgraded!")
}

func TestHandleConnectToRemoteCluster(t *testing.T) {
	ctx := context.Background()

	t.Run("success", func(t *testing.T) {
		mock := cmd.NewMockShellExecutor()
		mock.AddCommandString("cilium", []string{"clustermesh", "connect", "--destination-cluster", "my-cluster"}, "✓ Connected to cluster my-cluster!", nil)
		ctx = cmd.WithShellExecutor(ctx, mock)

		request := createMCPRequest(map[string]interface{}{
			"cluster_name": "my-cluster",
		})

		result, err := handleConnectToRemoteCluster(ctx, request)
		require.NoError(t, err)
		assert.NotNil(t, result)
		assert.False(t, result.IsError)
		assert.Contains(t, getResultText(result), "✓ Connected to cluster my-cluster!")
	})

	t.Run("missing cluster_name", func(t *testing.T) {
		request := createMCPRequest(map[string]interface{}{})
		result, err := handleConnectToRemoteCluster(ctx, request)
		require.NoError(t, err)
		assert.NotNil(t, result)
		assert.True(t, result.IsError)
		assert.Contains(t, getResultText(result), "cluster_name parameter is required")
	})
}

func TestHandleDisconnectFromRemoteCluster(t *testing.T) {
	ctx := context.Background()

	t.Run("success", func(t *testing.T) {
		mock := cmd.NewMockShellExecutor()
		mock.AddCommandString("cilium", []string{"clustermesh", "disconnect", "--destination-cluster", "my-cluster"}, "✓ Disconnected from cluster my-cluster!", nil)
		ctx = cmd.WithShellExecutor(ctx, mock)

		request := createMCPRequest(map[string]interface{}{
			"cluster_name": "my-cluster",
		})

		result, err := handleDisconnectRemoteCluster(ctx, request)
		require.NoError(t, err)
		assert.NotNil(t, result)
		assert.False(t, result.IsError)
		assert.Contains(t, getResultText(result), "✓ Disconnected from cluster my-cluster!")
	})

	t.Run("missing cluster_name", func(t *testing.T) {
		request := createMCPRequest(map[string]interface{}{})
		result, err := handleDisconnectRemoteCluster(ctx, request)
		require.NoError(t, err)
		assert.NotNil(t, result)
		assert.True(t, result.IsError)
		assert.Contains(t, getResultText(result), "cluster_name parameter is required")
	})
}

func TestHandleEnableHubble(t *testing.T) {
	ctx := context.Background()
	mock := cmd.NewMockShellExecutor()
	mock.AddCommandString("cilium", []string{"hubble", "enable"}, "✓ Hubble was successfully enabled!", nil)
	ctx = cmd.WithShellExecutor(ctx, mock)

	request := createMCPRequest(map[string]interface{}{
		"enable": "true",
	})

	result, err := handleToggleHubble(ctx, request)
	require.NoError(t, err)
	assert.NotNil(t, result)
	assert.False(t, result.IsError)
	assert.Contains(t, getResultText(result), "✓ Hubble was successfully enabled!")
}

func TestHandleDisableHubble(t *testing.T) {
	ctx := context.Background()
	mock := cmd.NewMockShellExecutor()
	mock.AddCommandString("cilium", []string{"hubble", "disable"}, "✓ Hubble was successfully disabled!", nil)
	ctx = cmd.WithShellExecutor(ctx, mock)

	request := createMCPRequest(map[string]interface{}{
		"enable": "false",
	})

	result, err := handleToggleHubble(ctx, request)
	require.NoError(t, err)
	assert.NotNil(t, result)
	assert.False(t, result.IsError)
	assert.Contains(t, getResultText(result), "✓ Hubble was successfully disabled!")
}

func TestHandleListBGPPeers(t *testing.T) {
	ctx := context.Background()
	mock := cmd.NewMockShellExecutor()
	mock.AddCommandString("cilium", []string{"bgp", "peers"}, "listing BGP peers", nil)
	ctx = cmd.WithShellExecutor(ctx, mock)

	request := createMCPRequest(map[string]interface{}{})

	result, err := handleListBGPPeers(ctx, request)
	require.NoError(t, err)
	assert.NotNil(t, result)
	assert.False(t, result.IsError)
	assert.Contains(t, getResultText(result), "listing BGP peers")
}

func TestHandleListBGPRoutes(t *testing.T) {
	ctx := context.Background()
	mock := cmd.NewMockShellExecutor()
	mock.AddCommandString("cilium", []string{"bgp", "routes"}, "listing BGP routes", nil)
	ctx = cmd.WithShellExecutor(ctx, mock)

	request := createMCPRequest(map[string]interface{}{})

	result, err := handleListBGPRoutes(ctx, request)
	require.NoError(t, err)
	assert.NotNil(t, result)
	assert.False(t, result.IsError)
	assert.Contains(t, getResultText(result), "listing BGP routes")
}

func TestRunCiliumCliWithContext(t *testing.T) {
	ctx := context.Background()
	t.Run("success", func(t *testing.T) {
		mock := cmd.NewMockShellExecutor()
		mock.AddCommandString("cilium", []string{"test"}, "success", nil)
		ctx = cmd.WithShellExecutor(ctx, mock)
		result, err := runCiliumCliWithContext(ctx, "test")
		require.NoError(t, err)
		assert.Equal(t, "success", result)
	})
	t.Run("error", func(t *testing.T) {
		mock := cmd.NewMockShellExecutor()
		mock.AddCommandString("cilium", []string{"test"}, "", fmt.Errorf("test error"))
		ctx = cmd.WithShellExecutor(ctx, mock)
		_, err := runCiliumCliWithContext(ctx, "test")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "test error")
	})
}

func getResultText(r *mcp.CallToolResult) string {
	if r == nil || len(r.Content) == 0 {
		return ""
	}
	if textContent, ok := r.Content[0].(*mcp.TextContent); ok {
		return strings.TrimSpace(textContent.Text)
	}
	return ""
}
