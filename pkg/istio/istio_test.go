package istio

import (
	"context"
	"testing"

	"github.com/kagent-dev/tools/internal/cmd"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRegisterTools(t *testing.T) {
	s := server.NewMCPServer("test-server", "v0.0.1")
	RegisterTools(s)
}

func TestHandleIstioProxyStatus(t *testing.T) {
	ctx := context.Background()

	t.Run("basic proxy status", func(t *testing.T) {
		mock := cmd.NewMockShellExecutor()
		mock.AddCommandString("istioctl", []string{"proxy-status", "--timeout", "30s"}, "Proxy status output", nil)

		ctx = cmd.WithShellExecutor(ctx, mock)

		result, err := handleIstioProxyStatus(ctx, mcp.CallToolRequest{})

		require.NoError(t, err)
		assert.NotNil(t, result)
		assert.False(t, result.IsError)
	})

	t.Run("proxy status with namespace", func(t *testing.T) {
		mock := cmd.NewMockShellExecutor()
		mock.AddCommandString("istioctl", []string{"proxy-status", "-n", "istio-system", "--timeout", "30s"}, "Proxy status output", nil)

		ctx = cmd.WithShellExecutor(ctx, mock)

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
		mock := cmd.NewMockShellExecutor()
		mock.AddCommandString("istioctl", []string{"proxy-status", "-n", "default", "test-pod", "--timeout", "30s"}, "Proxy status output", nil)

		ctx = cmd.WithShellExecutor(ctx, mock)

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
		mock := cmd.NewMockShellExecutor()
		mock.AddCommandString("istioctl", []string{"proxy-config", "all", "test-pod", "--timeout", "30s"}, "Proxy config output", nil)

		ctx = cmd.WithShellExecutor(ctx, mock)

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
		mock := cmd.NewMockShellExecutor()
		mock.AddCommandString("istioctl", []string{"proxy-config", "cluster", "test-pod.default", "--timeout", "30s"}, "Proxy config output", nil)

		ctx = cmd.WithShellExecutor(ctx, mock)

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
		mock := cmd.NewMockShellExecutor()
		mock.AddCommandString("istioctl", []string{"install", "--set", "profile=default", "-y", "--timeout", "30s"}, "Install completed", nil)

		ctx = cmd.WithShellExecutor(ctx, mock)

		result, err := handleIstioInstall(ctx, mcp.CallToolRequest{})

		require.NoError(t, err)
		assert.NotNil(t, result)
		assert.False(t, result.IsError)
	})

	t.Run("install with custom profile", func(t *testing.T) {
		mock := cmd.NewMockShellExecutor()
		mock.AddCommandString("istioctl", []string{"install", "--set", "profile=demo", "-y", "--timeout", "30s"}, "Install completed", nil)

		ctx = cmd.WithShellExecutor(ctx, mock)

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
	mock := cmd.NewMockShellExecutor()

	mock.AddCommandString("istioctl", []string{"manifest", "generate", "--set", "profile=minimal", "--timeout", "30s"}, "Generated manifest", nil)

	ctx = cmd.WithShellExecutor(ctx, mock)

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
		mock := cmd.NewMockShellExecutor()
		mock.AddCommandString("istioctl", []string{"analyze", "-A", "--timeout", "30s"}, "Analysis output", nil)

		ctx = cmd.WithShellExecutor(ctx, mock)

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
		mock := cmd.NewMockShellExecutor()
		mock.AddCommandString("istioctl", []string{"analyze", "-n", "default", "--timeout", "30s"}, "Analysis output", nil)

		ctx = cmd.WithShellExecutor(ctx, mock)

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
		mock := cmd.NewMockShellExecutor()
		mock.AddCommandString("istioctl", []string{"version", "--timeout", "30s"}, "Version output", nil)

		ctx = cmd.WithShellExecutor(ctx, mock)

		result, err := handleIstioVersion(ctx, mcp.CallToolRequest{})

		require.NoError(t, err)
		assert.NotNil(t, result)
		assert.False(t, result.IsError)
	})

	t.Run("version short", func(t *testing.T) {
		mock := cmd.NewMockShellExecutor()
		mock.AddCommandString("istioctl", []string{"version", "--short", "--timeout", "30s"}, "1.18.0", nil)

		ctx = cmd.WithShellExecutor(ctx, mock)

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
	mock := cmd.NewMockShellExecutor()

	mock.AddCommandString("istioctl", []string{"remote-clusters", "--timeout", "30s"}, "Remote clusters output", nil)

	ctx = cmd.WithShellExecutor(ctx, mock)

	result, err := handleIstioRemoteClusters(ctx, mcp.CallToolRequest{})

	require.NoError(t, err)
	assert.NotNil(t, result)
	assert.False(t, result.IsError)
}

func TestHandleWaypointList(t *testing.T) {
	ctx := context.Background()

	t.Run("list waypoints in all namespaces", func(t *testing.T) {
		mock := cmd.NewMockShellExecutor()
		mock.AddCommandString("istioctl", []string{"waypoint", "list", "-A", "--timeout", "30s"}, "Waypoints list", nil)

		ctx = cmd.WithShellExecutor(ctx, mock)

		request := mcp.CallToolRequest{}
		request.Params.Arguments = map[string]interface{}{
			"all_namespaces": "true",
		}

		result, err := handleWaypointList(ctx, request)

		require.NoError(t, err)
		assert.NotNil(t, result)
		assert.False(t, result.IsError)
	})

	t.Run("list waypoints in a specific namespace", func(t *testing.T) {
		mock := cmd.NewMockShellExecutor()
		mock.AddCommandString("istioctl", []string{"waypoint", "list", "-n", "default", "--timeout", "30s"}, "Waypoints list", nil)

		ctx = cmd.WithShellExecutor(ctx, mock)

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

	t.Run("generate waypoint with namespace", func(t *testing.T) {
		mock := cmd.NewMockShellExecutor()
		mock.AddCommandString("istioctl", []string{"waypoint", "generate", "waypoint", "-n", "default", "--for", "all", "--timeout", "30s"}, "Generated waypoint", nil)

		ctx = cmd.WithShellExecutor(ctx, mock)

		request := mcp.CallToolRequest{}
		request.Params.Arguments = map[string]interface{}{
			"namespace":    "default",
			"name":         "waypoint",
			"traffic_type": "all",
		}

		result, err := handleWaypointGenerate(ctx, request)

		require.NoError(t, err)
		assert.NotNil(t, result)
		assert.False(t, result.IsError)
	})
}

func TestRunIstioCtl(t *testing.T) {
	t.Run("run istioctl with context", func(t *testing.T) {
		mock := cmd.NewMockShellExecutor()
		mock.AddCommandString("istioctl", []string{"version", "--timeout", "30s"}, "1.18.0", nil)
		ctx := cmd.WithShellExecutor(context.Background(), mock)

		result, err := runIstioCtl(ctx, []string{"version"})

		require.NoError(t, err)
		assert.Equal(t, "1.18.0", result)
	})
}

func TestIstioErrorHandling(t *testing.T) {
	t.Run("istioctl command failure", func(t *testing.T) {
		mock := cmd.NewMockShellExecutor()
		mock.AddCommandString("istioctl", []string{"proxy-status"}, "", assert.AnError)
		ctx := cmd.WithShellExecutor(context.Background(), mock)

		result, err := handleIstioProxyStatus(ctx, mcp.CallToolRequest{})

		require.NoError(t, err)
		assert.NotNil(t, result)
		assert.True(t, result.IsError)
	})
}
