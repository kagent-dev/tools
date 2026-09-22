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
	RegisterTools(s, false) // false = enable all tools including write operations
}

func TestHandleIstioProxyStatus(t *testing.T) {
	ctx := context.Background()

	t.Run("basic proxy status", func(t *testing.T) {
		mock := cmd.NewMockShellExecutor()
		mock.AddCommandString("istioctl", []string{"proxy-status"}, "Proxy status output", nil)

		ctx = cmd.WithShellExecutor(ctx, mock)

		result, err := handleIstioProxyStatus(ctx, mcp.CallToolRequest{})

		require.NoError(t, err)
		assert.NotNil(t, result)
		assert.False(t, result.IsError)
	})

	t.Run("proxy status with namespace", func(t *testing.T) {
		mock := cmd.NewMockShellExecutor()
		mock.AddCommandString("istioctl", []string{"proxy-status", "-n", "istio-system"}, "Proxy status output", nil)

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
		mock.AddCommandString("istioctl", []string{"proxy-status", "-n", "default", "test-pod"}, "Proxy status output", nil)

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
		mock.AddCommandString("istioctl", []string{"proxy-config", "all", "test-pod"}, "Proxy config output", nil)

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
		mock.AddCommandString("istioctl", []string{"proxy-config", "cluster", "test-pod.default"}, "Proxy config output", nil)

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
		mock.AddCommandString("istioctl", []string{"install", "--set", "profile=default", "-y"}, "Install completed", nil)

		ctx = cmd.WithShellExecutor(ctx, mock)

		result, err := handleIstioInstall(ctx, mcp.CallToolRequest{})

		require.NoError(t, err)
		assert.NotNil(t, result)
		assert.False(t, result.IsError)
	})

	t.Run("install with custom profile", func(t *testing.T) {
		mock := cmd.NewMockShellExecutor()
		mock.AddCommandString("istioctl", []string{"install", "--set", "profile=demo", "-y"}, "Install completed", nil)

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

	mock.AddCommandString("istioctl", []string{"manifest", "generate", "--set", "profile=minimal"}, "Generated manifest", nil)

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
		mock.AddCommandString("istioctl", []string{"analyze", "-A"}, "Analysis output", nil)

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
		mock.AddCommandString("istioctl", []string{"analyze", "-n", "default"}, "Analysis output", nil)

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
		mock.AddCommandString("istioctl", []string{"version"}, "Version output", nil)

		ctx = cmd.WithShellExecutor(ctx, mock)

		result, err := handleIstioVersion(ctx, mcp.CallToolRequest{})

		require.NoError(t, err)
		assert.NotNil(t, result)
		assert.False(t, result.IsError)
	})

	t.Run("version short", func(t *testing.T) {
		mock := cmd.NewMockShellExecutor()
		mock.AddCommandString("istioctl", []string{"version", "--short"}, "1.18.0", nil)

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

	mock.AddCommandString("istioctl", []string{"remote-clusters"}, "Remote clusters output", nil)

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
		mock.AddCommandString("istioctl", []string{"waypoint", "list", "-A"}, "Waypoints list", nil)

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
		mock.AddCommandString("istioctl", []string{"waypoint", "list", "-n", "default"}, "Waypoints list", nil)

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
		mock.AddCommandString("istioctl", []string{"waypoint", "generate", "waypoint", "-n", "default", "--for", "all"}, "Generated waypoint", nil)

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
		mock.AddCommandString("istioctl", []string{"version"}, "1.18.0", nil)
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

func TestHandleWaypointApply(t *testing.T) {
	t.Run("basic apply", func(t *testing.T) {
		mock := cmd.NewMockShellExecutor()
		mock.AddCommandString("istioctl", []string{"waypoint", "apply", "-n", "default"}, "applied", nil)
		ctx := cmd.WithShellExecutor(context.Background(), mock)

		req := mcp.CallToolRequest{}
		req.Params.Arguments = map[string]interface{}{"namespace": "default"}
		result, err := handleWaypointApply(ctx, req)
		require.NoError(t, err)
		assert.False(t, result.IsError)
	})

	t.Run("apply with enroll-namespace", func(t *testing.T) {
		mock := cmd.NewMockShellExecutor()
		mock.AddCommandString("istioctl", []string{"waypoint", "apply", "-n", "default", "--enroll-namespace"}, "applied", nil)
		ctx := cmd.WithShellExecutor(context.Background(), mock)

		req := mcp.CallToolRequest{}
		req.Params.Arguments = map[string]interface{}{"namespace": "default", "enroll_namespace": "true"}
		result, err := handleWaypointApply(ctx, req)
		require.NoError(t, err)
		assert.False(t, result.IsError)
	})

	t.Run("missing namespace", func(t *testing.T) {
		mock := cmd.NewMockShellExecutor()
		ctx := cmd.WithShellExecutor(context.Background(), mock)
		result, err := handleWaypointApply(ctx, mcp.CallToolRequest{})
		require.NoError(t, err)
		assert.True(t, result.IsError)
	})

	t.Run("command failure", func(t *testing.T) {
		mock := cmd.NewMockShellExecutor()
		mock.AddCommandString("istioctl", []string{"waypoint", "apply", "-n", "default"}, "", assert.AnError)
		ctx := cmd.WithShellExecutor(context.Background(), mock)
		req := mcp.CallToolRequest{}
		req.Params.Arguments = map[string]interface{}{"namespace": "default"}
		result, err := handleWaypointApply(ctx, req)
		require.NoError(t, err)
		assert.True(t, result.IsError)
	})
}

func TestHandleWaypointDelete(t *testing.T) {
	t.Run("delete all", func(t *testing.T) {
		mock := cmd.NewMockShellExecutor()
		mock.AddCommandString("istioctl", []string{"waypoint", "delete", "--all", "-n", "default"}, "deleted", nil)
		ctx := cmd.WithShellExecutor(context.Background(), mock)
		req := mcp.CallToolRequest{}
		req.Params.Arguments = map[string]interface{}{"namespace": "default", "all": "true"}
		result, err := handleWaypointDelete(ctx, req)
		require.NoError(t, err)
		assert.False(t, result.IsError)
	})

	t.Run("delete by names", func(t *testing.T) {
		mock := cmd.NewMockShellExecutor()
		mock.AddCommandString("istioctl", []string{"waypoint", "delete", "wp1", "wp2", "-n", "default"}, "deleted", nil)
		ctx := cmd.WithShellExecutor(context.Background(), mock)
		req := mcp.CallToolRequest{}
		req.Params.Arguments = map[string]interface{}{"namespace": "default", "names": "wp1, wp2"}
		result, err := handleWaypointDelete(ctx, req)
		require.NoError(t, err)
		assert.False(t, result.IsError)
	})

	t.Run("missing namespace", func(t *testing.T) {
		mock := cmd.NewMockShellExecutor()
		ctx := cmd.WithShellExecutor(context.Background(), mock)
		result, err := handleWaypointDelete(ctx, mcp.CallToolRequest{})
		require.NoError(t, err)
		assert.True(t, result.IsError)
	})
}

func TestHandleWaypointStatus(t *testing.T) {
	t.Run("status with name", func(t *testing.T) {
		mock := cmd.NewMockShellExecutor()
		mock.AddCommandString("istioctl", []string{"waypoint", "status", "wp1", "-n", "default"}, "status", nil)
		ctx := cmd.WithShellExecutor(context.Background(), mock)
		req := mcp.CallToolRequest{}
		req.Params.Arguments = map[string]interface{}{"namespace": "default", "name": "wp1"}
		result, err := handleWaypointStatus(ctx, req)
		require.NoError(t, err)
		assert.False(t, result.IsError)
	})

	t.Run("status without name", func(t *testing.T) {
		mock := cmd.NewMockShellExecutor()
		mock.AddCommandString("istioctl", []string{"waypoint", "status", "-n", "default"}, "status", nil)
		ctx := cmd.WithShellExecutor(context.Background(), mock)
		req := mcp.CallToolRequest{}
		req.Params.Arguments = map[string]interface{}{"namespace": "default"}
		result, err := handleWaypointStatus(ctx, req)
		require.NoError(t, err)
		assert.False(t, result.IsError)
	})

	t.Run("missing namespace", func(t *testing.T) {
		mock := cmd.NewMockShellExecutor()
		ctx := cmd.WithShellExecutor(context.Background(), mock)
		result, err := handleWaypointStatus(ctx, mcp.CallToolRequest{})
		require.NoError(t, err)
		assert.True(t, result.IsError)
	})
}

func TestHandleZtunnelConfig(t *testing.T) {
	t.Run("default config type", func(t *testing.T) {
		mock := cmd.NewMockShellExecutor()
		mock.AddCommandString("istioctl", []string{"ztunnel", "config", "all"}, "ztunnel config", nil)
		ctx := cmd.WithShellExecutor(context.Background(), mock)
		result, err := handleZtunnelConfig(ctx, mcp.CallToolRequest{})
		require.NoError(t, err)
		assert.False(t, result.IsError)
	})

	t.Run("with namespace and config type", func(t *testing.T) {
		mock := cmd.NewMockShellExecutor()
		mock.AddCommandString("istioctl", []string{"ztunnel", "config", "workloads", "-n", "istio-system"}, "ztunnel config", nil)
		ctx := cmd.WithShellExecutor(context.Background(), mock)
		req := mcp.CallToolRequest{}
		req.Params.Arguments = map[string]interface{}{"config_type": "workloads", "namespace": "istio-system"}
		result, err := handleZtunnelConfig(ctx, req)
		require.NoError(t, err)
		assert.False(t, result.IsError)
	})

	t.Run("command failure", func(t *testing.T) {
		mock := cmd.NewMockShellExecutor()
		mock.AddCommandString("istioctl", []string{"ztunnel", "config", "all"}, "", assert.AnError)
		ctx := cmd.WithShellExecutor(context.Background(), mock)
		result, err := handleZtunnelConfig(ctx, mcp.CallToolRequest{})
		require.NoError(t, err)
		assert.True(t, result.IsError)
	})
}
