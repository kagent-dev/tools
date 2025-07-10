package cilium

import (
	"context"
	"testing"

	"github.com/kagent-dev/tools/pkg/utils"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCiliumStatusAndVersion(t *testing.T) {
	ctx := context.Background()
	mock := utils.NewMockShellExecutor()

	// Mock the cilium status and version commands
	mock.AddCommandString("cilium", []string{"status"}, "Cilium status: OK", nil)
	mock.AddCommandString("cilium", []string{"version"}, "cilium version 1.14.0", nil)

	ctx = utils.WithShellExecutor(ctx, mock)

	result, err := handleCiliumStatusAndVersion(ctx, mcp.CallToolRequest{})

	require.NoError(t, err)
	assert.NotNil(t, result)
	assert.False(t, result.IsError)

	// Verify the output contains expected content
	if len(result.Content) > 0 {
		if textContent, ok := result.Content[0].(mcp.TextContent); ok {
			assert.Contains(t, textContent.Text, "Cilium status: OK")
			assert.Contains(t, textContent.Text, "cilium version 1.14.0")
		}
	}
}

func TestUpgradeCilium(t *testing.T) {
	ctx := context.Background()
	mock := utils.NewMockShellExecutor()

	mock.AddCommandString("cilium", []string{"upgrade"}, "Cilium upgrade completed", nil)

	ctx = utils.WithShellExecutor(ctx, mock)

	result, err := handleUpgradeCilium(ctx, mcp.CallToolRequest{})

	require.NoError(t, err)
	assert.NotNil(t, result)
	assert.False(t, result.IsError)
}

func TestInstallCilium(t *testing.T) {
	ctx := context.Background()
	mock := utils.NewMockShellExecutor()

	mock.AddCommandString("cilium", []string{"install"}, "Cilium install completed", nil)

	ctx = utils.WithShellExecutor(ctx, mock)

	result, err := handleInstallCilium(ctx, mcp.CallToolRequest{})

	require.NoError(t, err)
	assert.NotNil(t, result)
	assert.False(t, result.IsError)
}

func TestUninstallCilium(t *testing.T) {
	ctx := context.Background()
	mock := utils.NewMockShellExecutor()

	mock.AddCommandString("cilium", []string{"uninstall"}, "Cilium uninstall completed", nil)

	ctx = utils.WithShellExecutor(ctx, mock)

	result, err := handleUninstallCilium(ctx, mcp.CallToolRequest{})

	require.NoError(t, err)
	assert.NotNil(t, result)
	assert.False(t, result.IsError)
}

func TestConnectToRemoteCluster(t *testing.T) {
	ctx := context.Background()

	t.Run("missing cluster_name parameter", func(t *testing.T) {
		result, err := handleConnectToRemoteCluster(ctx, mcp.CallToolRequest{})

		require.NoError(t, err)
		assert.NotNil(t, result)
		assert.True(t, result.IsError)
	})

	t.Run("connect with cluster name", func(t *testing.T) {
		mock := utils.NewMockShellExecutor()
		mock.AddCommandString("cilium", []string{"clustermesh", "connect", "--destination-cluster", "remote-cluster"}, "Connected to remote cluster", nil)

		ctx = utils.WithShellExecutor(ctx, mock)

		request := mcp.CallToolRequest{}
		request.Params.Arguments = map[string]interface{}{
			"cluster_name": "remote-cluster",
		}

		result, err := handleConnectToRemoteCluster(ctx, request)

		require.NoError(t, err)
		assert.NotNil(t, result)
		assert.False(t, result.IsError)
	})
}

func TestListBGPPeers(t *testing.T) {
	ctx := context.Background()
	mock := utils.NewMockShellExecutor()

	mock.AddCommandString("cilium", []string{"bgp", "peers"}, "BGP peers list", nil)

	ctx = utils.WithShellExecutor(ctx, mock)

	result, err := handleListBGPPeers(ctx, mcp.CallToolRequest{})

	require.NoError(t, err)
	assert.NotNil(t, result)
	assert.False(t, result.IsError)
}

func TestListBGPRoutes(t *testing.T) {
	ctx := context.Background()
	mock := utils.NewMockShellExecutor()

	mock.AddCommandString("cilium", []string{"bgp", "routes"}, "BGP routes list", nil)

	ctx = utils.WithShellExecutor(ctx, mock)

	result, err := handleListBGPRoutes(ctx, mcp.CallToolRequest{})

	require.NoError(t, err)
	assert.NotNil(t, result)
	assert.False(t, result.IsError)
}

func TestToggleHubble(t *testing.T) {
	ctx := context.Background()
	mock := utils.NewMockShellExecutor()

	mock.AddCommandString("cilium", []string{"hubble", "enable"}, "Hubble enabled", nil)

	ctx = utils.WithShellExecutor(ctx, mock)

	request := mcp.CallToolRequest{}
	request.Params.Arguments = map[string]interface{}{
		"enable": "true",
	}

	result, err := handleToggleHubble(ctx, request)

	require.NoError(t, err)
	assert.NotNil(t, result)
	assert.False(t, result.IsError)
}

func TestRunCiliumCliWithContext(t *testing.T) {
	ctx := context.Background()
	mock := utils.NewMockShellExecutor()

	mock.AddCommandString("cilium", []string{"status"}, "Cilium status", nil)

	ctx = utils.WithShellExecutor(ctx, mock)

	result, err := runCiliumCliWithContext(ctx, "status")

	require.NoError(t, err)
	assert.Equal(t, "Cilium status", result)
}

func TestCiliumErrorHandling(t *testing.T) {
	ctx := context.Background()
	mock := utils.NewMockShellExecutor()

	mock.AddCommandString("cilium", []string{"status"}, "", assert.AnError)

	ctx = utils.WithShellExecutor(ctx, mock)

	result, err := handleCiliumStatusAndVersion(ctx, mcp.CallToolRequest{})

	require.NoError(t, err)
	assert.NotNil(t, result)
	assert.True(t, result.IsError)
}
