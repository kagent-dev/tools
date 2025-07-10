package istio

import (
	"context"
	"testing"

	"github.com/kagent-dev/tools/pkg/utils"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHandleIstioProxyStatus(t *testing.T) {
	ctx := context.Background()

	t.Run("basic proxy status", func(t *testing.T) {
		mock := utils.NewMockShellExecutor()
		mock.AddCommandString("istioctl", []string{"proxy-status"}, "Proxy status output", nil)

		ctx = utils.WithShellExecutor(ctx, mock)

		result, err := handleIstioProxyStatus(ctx, mcp.CallToolRequest{})

		require.NoError(t, err)
		assert.NotNil(t, result)
		assert.False(t, result.IsError)
	})

	t.Run("proxy status with namespace", func(t *testing.T) {
		mock := utils.NewMockShellExecutor()
		mock.AddCommandString("istioctl", []string{"proxy-status", "-n", "istio-system"}, "Proxy status output", nil)

		ctx = utils.WithShellExecutor(ctx, mock)

		request := mcp.CallToolRequest{}
		request.Params.Arguments = map[string]interface{}{
			"namespace": "istio-system",
		}

		result, err := handleIstioProxyStatus(ctx, request)

		require.NoError(t, err)
		assert.NotNil(t, result)
		assert.False(t, result.IsError)
	})

	t.Run("proxy status with pod name", func(t *testing.T) {
		mock := utils.NewMockShellExecutor()
		mock.AddCommandString("istioctl", []string{"proxy-status", "-n", "default", "test-pod"}, "Proxy status output", nil)

		ctx = utils.WithShellExecutor(ctx, mock)

		request := mcp.CallToolRequest{}
		request.Params.Arguments = map[string]interface{}{
			"pod_name":  "test-pod",
			"namespace": "default",
		}

		result, err := handleIstioProxyStatus(ctx, request)

		require.NoError(t, err)
		assert.NotNil(t, result)
		assert.False(t, result.IsError)
	})
}

func TestHandleIstioProxyConfig(t *testing.T) {
	ctx := context.Background()

	t.Run("missing pod_name parameter", func(t *testing.T) {
		result, err := handleIstioProxyConfig(ctx, mcp.CallToolRequest{})

		require.NoError(t, err)
		assert.NotNil(t, result)
		assert.True(t, result.IsError)
	})

	t.Run("proxy config with pod name", func(t *testing.T) {
		mock := utils.NewMockShellExecutor()
		mock.AddCommandString("istioctl", []string{"proxy-config", "all", "test-pod"}, "Proxy config output", nil)

		ctx = utils.WithShellExecutor(ctx, mock)

		request := mcp.CallToolRequest{}
		request.Params.Arguments = map[string]interface{}{
			"pod_name": "test-pod",
		}

		result, err := handleIstioProxyConfig(ctx, request)

		require.NoError(t, err)
		assert.NotNil(t, result)
		assert.False(t, result.IsError)
	})

	t.Run("proxy config with namespace", func(t *testing.T) {
		mock := utils.NewMockShellExecutor()
		mock.AddCommandString("istioctl", []string{"proxy-config", "cluster", "test-pod.default"}, "Proxy config output", nil)

		ctx = utils.WithShellExecutor(ctx, mock)

		request := mcp.CallToolRequest{}
		request.Params.Arguments = map[string]interface{}{
			"pod_name":    "test-pod",
			"namespace":   "default",
			"config_type": "cluster",
		}

		result, err := handleIstioProxyConfig(ctx, request)

		require.NoError(t, err)
		assert.NotNil(t, result)
		assert.False(t, result.IsError)
	})
}

func TestHandleIstioInstall(t *testing.T) {
	ctx := context.Background()

	t.Run("install with default profile", func(t *testing.T) {
		mock := utils.NewMockShellExecutor()
		mock.AddCommandString("istioctl", []string{"install", "--set", "profile=default", "-y"}, "Install completed", nil)

		ctx = utils.WithShellExecutor(ctx, mock)

		result, err := handleIstioInstall(ctx, mcp.CallToolRequest{})

		require.NoError(t, err)
		assert.NotNil(t, result)
		assert.False(t, result.IsError)
	})

	t.Run("install with custom profile", func(t *testing.T) {
		mock := utils.NewMockShellExecutor()
		mock.AddCommandString("istioctl", []string{"install", "--set", "profile=demo", "-y"}, "Install completed", nil)

		ctx = utils.WithShellExecutor(ctx, mock)

		request := mcp.CallToolRequest{}
		request.Params.Arguments = map[string]interface{}{
			"profile": "demo",
		}

		result, err := handleIstioInstall(ctx, request)

		require.NoError(t, err)
		assert.NotNil(t, result)
		assert.False(t, result.IsError)
	})
}

func TestHandleIstioGenerateManifest(t *testing.T) {
	ctx := context.Background()
	mock := utils.NewMockShellExecutor()

	mock.AddCommandString("istioctl", []string{"manifest", "generate", "--set", "profile=minimal"}, "Generated manifest", nil)

	ctx = utils.WithShellExecutor(ctx, mock)

	request := mcp.CallToolRequest{}
	request.Params.Arguments = map[string]interface{}{
		"profile": "minimal",
	}

	result, err := handleIstioGenerateManifest(ctx, request)

	require.NoError(t, err)
	assert.NotNil(t, result)
	assert.False(t, result.IsError)
}

func TestHandleIstioAnalyzeClusterConfiguration(t *testing.T) {
	ctx := context.Background()

	t.Run("analyze all namespaces", func(t *testing.T) {
		mock := utils.NewMockShellExecutor()
		mock.AddCommandString("istioctl", []string{"analyze", "-A"}, "Analysis output", nil)

		ctx = utils.WithShellExecutor(ctx, mock)

		request := mcp.CallToolRequest{}
		request.Params.Arguments = map[string]interface{}{
			"all_namespaces": "true",
		}

		result, err := handleIstioAnalyzeClusterConfiguration(ctx, request)

		require.NoError(t, err)
		assert.NotNil(t, result)
		assert.False(t, result.IsError)
	})

	t.Run("analyze specific namespace", func(t *testing.T) {
		mock := utils.NewMockShellExecutor()
		mock.AddCommandString("istioctl", []string{"analyze", "-n", "default"}, "Analysis output", nil)

		ctx = utils.WithShellExecutor(ctx, mock)

		request := mcp.CallToolRequest{}
		request.Params.Arguments = map[string]interface{}{
			"namespace": "default",
		}

		result, err := handleIstioAnalyzeClusterConfiguration(ctx, request)

		require.NoError(t, err)
		assert.NotNil(t, result)
		assert.False(t, result.IsError)
	})
}

func TestHandleIstioVersion(t *testing.T) {
	ctx := context.Background()

	t.Run("version full", func(t *testing.T) {
		mock := utils.NewMockShellExecutor()
		mock.AddCommandString("istioctl", []string{"version"}, "Version output", nil)

		ctx = utils.WithShellExecutor(ctx, mock)

		result, err := handleIstioVersion(ctx, mcp.CallToolRequest{})

		require.NoError(t, err)
		assert.NotNil(t, result)
		assert.False(t, result.IsError)
	})

	t.Run("version short", func(t *testing.T) {
		mock := utils.NewMockShellExecutor()
		mock.AddCommandString("istioctl", []string{"version", "--short"}, "1.18.0", nil)

		ctx = utils.WithShellExecutor(ctx, mock)

		request := mcp.CallToolRequest{}
		request.Params.Arguments = map[string]interface{}{
			"short": "true",
		}

		result, err := handleIstioVersion(ctx, request)

		require.NoError(t, err)
		assert.NotNil(t, result)
		assert.False(t, result.IsError)
	})
}

func TestHandleIstioRemoteClusters(t *testing.T) {
	ctx := context.Background()
	mock := utils.NewMockShellExecutor()

	mock.AddCommandString("istioctl", []string{"remote-clusters"}, "Remote clusters output", nil)

	ctx = utils.WithShellExecutor(ctx, mock)

	result, err := handleIstioRemoteClusters(ctx, mcp.CallToolRequest{})

	require.NoError(t, err)
	assert.NotNil(t, result)
	assert.False(t, result.IsError)
}

func TestHandleWaypointList(t *testing.T) {
	ctx := context.Background()

	t.Run("list all namespaces", func(t *testing.T) {
		mock := utils.NewMockShellExecutor()
		mock.AddCommandString("istioctl", []string{"waypoint", "list", "-A"}, "Waypoint list output", nil)

		ctx = utils.WithShellExecutor(ctx, mock)

		request := mcp.CallToolRequest{}
		request.Params.Arguments = map[string]interface{}{
			"all_namespaces": "true",
		}

		result, err := handleWaypointList(ctx, request)

		require.NoError(t, err)
		assert.NotNil(t, result)
		assert.False(t, result.IsError)
	})

	t.Run("list specific namespace", func(t *testing.T) {
		mock := utils.NewMockShellExecutor()
		mock.AddCommandString("istioctl", []string{"waypoint", "list", "-n", "default"}, "Waypoint list output", nil)

		ctx = utils.WithShellExecutor(ctx, mock)

		request := mcp.CallToolRequest{}
		request.Params.Arguments = map[string]interface{}{
			"namespace": "default",
		}

		result, err := handleWaypointList(ctx, request)

		require.NoError(t, err)
		assert.NotNil(t, result)
		assert.False(t, result.IsError)
	})
}

func TestHandleWaypointGenerate(t *testing.T) {
	ctx := context.Background()

	t.Run("missing namespace parameter", func(t *testing.T) {
		result, err := handleWaypointGenerate(ctx, mcp.CallToolRequest{})

		require.NoError(t, err)
		assert.NotNil(t, result)
		assert.True(t, result.IsError)
	})

	t.Run("generate waypoint", func(t *testing.T) {
		mock := utils.NewMockShellExecutor()
		mock.AddCommandString("istioctl", []string{"waypoint", "generate", "test-waypoint", "-n", "default", "--for", "service"}, "Waypoint generated", nil)

		ctx = utils.WithShellExecutor(ctx, mock)

		request := mcp.CallToolRequest{}
		request.Params.Arguments = map[string]interface{}{
			"namespace":    "default",
			"name":         "test-waypoint",
			"traffic_type": "service",
		}

		result, err := handleWaypointGenerate(ctx, request)

		require.NoError(t, err)
		assert.NotNil(t, result)
		assert.False(t, result.IsError)
	})
}

func TestRunIstioCtl(t *testing.T) {
	ctx := context.Background()
	mock := utils.NewMockShellExecutor()

	mock.AddCommandString("istioctl", []string{"version"}, "Version output", nil)

	ctx = utils.WithShellExecutor(ctx, mock)

	result, err := runIstioCtl(ctx, []string{"version"})

	require.NoError(t, err)
	assert.Equal(t, "Version output", result)
}

func TestIstioErrorHandling(t *testing.T) {
	ctx := context.Background()
	mock := utils.NewMockShellExecutor()

	mock.AddCommandString("istioctl", []string{"version"}, "", assert.AnError)

	ctx = utils.WithShellExecutor(ctx, mock)

	result, err := handleIstioVersion(ctx, mcp.CallToolRequest{})

	require.NoError(t, err)
	assert.NotNil(t, result)
	assert.True(t, result.IsError)
}
