package istio

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/kagent-dev/tools/internal/cmd"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRegisterTools(t *testing.T) {
	server := mcp.NewServer(&mcp.Implementation{Name: "test"}, nil)
	err := RegisterTools(server)
	require.NoError(t, err)
}

func TestHandleIstioProxyStatus(t *testing.T) {
	ctx := context.Background()

	t.Run("basic proxy status", func(t *testing.T) {
		mock := cmd.NewMockShellExecutor()
		mock.AddCommandString("istioctl", []string{"proxy-status"}, "Proxy status output", nil)

		ctx = cmd.WithShellExecutor(ctx, mock)

		request := &mcp.CallToolRequest{
			Params: &mcp.CallToolParamsRaw{
				Arguments: json.RawMessage(`{}`),
			},
		}

		result, err := handleIstioProxyStatus(ctx, request)

		require.NoError(t, err)
		assert.NotNil(t, result)
		assert.False(t, result.IsError)
	})

	t.Run("proxy status with namespace", func(t *testing.T) {
		mock := cmd.NewMockShellExecutor()
		mock.AddCommandString("istioctl", []string{"proxy-status", "-n", "istio-system"}, "Proxy status output", nil)

		ctx = cmd.WithShellExecutor(ctx, mock)

		args := map[string]interface{}{
			"namespace": "istio-system",
		}
		argsJSON, _ := json.Marshal(args)

		request := &mcp.CallToolRequest{
			Params: &mcp.CallToolParamsRaw{
				Arguments: argsJSON,
			},
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

		args := map[string]interface{}{
			"pod_name":  "test-pod",
			"namespace": "default",
		}
		argsJSON, _ := json.Marshal(args)

		request := &mcp.CallToolRequest{
			Params: &mcp.CallToolParamsRaw{
				Arguments: argsJSON,
			},
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
		request := &mcp.CallToolRequest{
			Params: &mcp.CallToolParamsRaw{
				Arguments: json.RawMessage(`{}`),
			},
		}

		result, err := handleIstioProxyConfig(ctx, request)

		require.NoError(t, err)
		assert.NotNil(t, result)
		assert.True(t, result.IsError)
	})

	t.Run("proxy config with pod name", func(t *testing.T) {
		mock := cmd.NewMockShellExecutor()
		mock.AddCommandString("istioctl", []string{"proxy-config", "all", "test-pod"}, "Proxy config output", nil)

		ctx = cmd.WithShellExecutor(ctx, mock)

		args := map[string]interface{}{
			"pod_name": "test-pod",
		}
		argsJSON, _ := json.Marshal(args)

		request := &mcp.CallToolRequest{
			Params: &mcp.CallToolParamsRaw{
				Arguments: argsJSON,
			},
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

		args := map[string]interface{}{
			"pod_name":    "test-pod",
			"namespace":   "default",
			"config_type": "cluster",
		}
		argsJSON, _ := json.Marshal(args)

		request := &mcp.CallToolRequest{
			Params: &mcp.CallToolParamsRaw{
				Arguments: argsJSON,
			},
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

		request := &mcp.CallToolRequest{
			Params: &mcp.CallToolParamsRaw{
				Arguments: json.RawMessage(`{}`),
			},
		}

		result, err := handleIstioInstall(ctx, request)

		require.NoError(t, err)
		assert.NotNil(t, result)
		assert.False(t, result.IsError)
	})

	t.Run("install with custom profile", func(t *testing.T) {
		mock := cmd.NewMockShellExecutor()
		mock.AddCommandString("istioctl", []string{"install", "--set", "profile=demo", "-y"}, "Install completed", nil)

		ctx = cmd.WithShellExecutor(ctx, mock)

		args := map[string]interface{}{
			"profile": "demo",
		}
		argsJSON, _ := json.Marshal(args)

		request := &mcp.CallToolRequest{
			Params: &mcp.CallToolParamsRaw{
				Arguments: argsJSON,
			},
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

	args := map[string]interface{}{
		"profile": "minimal",
	}
	argsJSON, _ := json.Marshal(args)

	request := &mcp.CallToolRequest{
		Params: &mcp.CallToolParamsRaw{
			Arguments: argsJSON,
		},
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

		args := map[string]interface{}{
			"all_namespaces": "true",
		}
		argsJSON, _ := json.Marshal(args)

		request := &mcp.CallToolRequest{
			Params: &mcp.CallToolParamsRaw{
				Arguments: argsJSON,
			},
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

		args := map[string]interface{}{
			"namespace": "default",
		}
		argsJSON, _ := json.Marshal(args)

		request := &mcp.CallToolRequest{
			Params: &mcp.CallToolParamsRaw{
				Arguments: argsJSON,
			},
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

		request := &mcp.CallToolRequest{
			Params: &mcp.CallToolParamsRaw{
				Arguments: json.RawMessage(`{}`),
			},
		}

		result, err := handleIstioVersion(ctx, request)

		require.NoError(t, err)
		assert.NotNil(t, result)
		assert.False(t, result.IsError)
	})

	t.Run("version short", func(t *testing.T) {
		mock := cmd.NewMockShellExecutor()
		mock.AddCommandString("istioctl", []string{"version", "--short"}, "1.18.0", nil)

		ctx = cmd.WithShellExecutor(ctx, mock)

		args := map[string]interface{}{
			"short": "true",
		}
		argsJSON, _ := json.Marshal(args)

		request := &mcp.CallToolRequest{
			Params: &mcp.CallToolParamsRaw{
				Arguments: argsJSON,
			},
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

	request := &mcp.CallToolRequest{
		Params: &mcp.CallToolParamsRaw{
			Arguments: json.RawMessage(`{}`),
		},
	}

	result, err := handleIstioRemoteClusters(ctx, request)

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

		args := map[string]interface{}{
			"all_namespaces": "true",
		}
		argsJSON, _ := json.Marshal(args)

		request := &mcp.CallToolRequest{
			Params: &mcp.CallToolParamsRaw{
				Arguments: argsJSON,
			},
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

		args := map[string]interface{}{
			"namespace": "default",
		}
		argsJSON, _ := json.Marshal(args)

		request := &mcp.CallToolRequest{
			Params: &mcp.CallToolParamsRaw{
				Arguments: argsJSON,
			},
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

		args := map[string]interface{}{
			"namespace":    "default",
			"name":         "waypoint",
			"traffic_type": "all",
		}
		argsJSON, _ := json.Marshal(args)

		request := &mcp.CallToolRequest{
			Params: &mcp.CallToolParamsRaw{
				Arguments: argsJSON,
			},
		}

		result, err := handleWaypointGenerate(ctx, request)

		require.NoError(t, err)
		assert.NotNil(t, result)
		assert.False(t, result.IsError)
	})

	t.Run("missing namespace parameter", func(t *testing.T) {
		request := &mcp.CallToolRequest{
			Params: &mcp.CallToolParamsRaw{
				Arguments: json.RawMessage(`{}`),
			},
		}

		result, err := handleWaypointGenerate(ctx, request)

		require.NoError(t, err)
		assert.NotNil(t, result)
		assert.True(t, result.IsError)
	})
}

func TestHandleWaypointApply(t *testing.T) {
	ctx := context.Background()

	t.Run("apply waypoint with namespace", func(t *testing.T) {
		mock := cmd.NewMockShellExecutor()
		mock.AddCommandString("istioctl", []string{"waypoint", "apply", "-n", "default"}, "Waypoint applied", nil)

		ctx = cmd.WithShellExecutor(ctx, mock)

		args := map[string]interface{}{
			"namespace": "default",
		}
		argsJSON, _ := json.Marshal(args)

		request := &mcp.CallToolRequest{
			Params: &mcp.CallToolParamsRaw{
				Arguments: argsJSON,
			},
		}

		result, err := handleWaypointApply(ctx, request)

		require.NoError(t, err)
		assert.NotNil(t, result)
		assert.False(t, result.IsError)
	})

	t.Run("missing namespace parameter", func(t *testing.T) {
		request := &mcp.CallToolRequest{
			Params: &mcp.CallToolParamsRaw{
				Arguments: json.RawMessage(`{}`),
			},
		}

		result, err := handleWaypointApply(ctx, request)

		require.NoError(t, err)
		assert.NotNil(t, result)
		assert.True(t, result.IsError)
	})

	t.Run("apply waypoint with enroll namespace", func(t *testing.T) {
		mock := cmd.NewMockShellExecutor()
		mock.AddCommandString("istioctl", []string{"waypoint", "apply", "-n", "default", "--enroll-namespace"}, "Waypoint applied and enrolled", nil)

		ctx = cmd.WithShellExecutor(ctx, mock)

		args := map[string]interface{}{
			"namespace":        "default",
			"enroll_namespace": "true",
		}
		argsJSON, _ := json.Marshal(args)

		request := &mcp.CallToolRequest{
			Params: &mcp.CallToolParamsRaw{
				Arguments: argsJSON,
			},
		}

		result, err := handleWaypointApply(ctx, request)

		require.NoError(t, err)
		assert.NotNil(t, result)
		assert.False(t, result.IsError)
	})

	t.Run("istioctl command failure", func(t *testing.T) {
		mock := cmd.NewMockShellExecutor()
		mock.AddCommandString("istioctl", []string{"waypoint", "apply", "-n", "default"}, "", errors.New("istioctl failed"))

		ctx = cmd.WithShellExecutor(ctx, mock)

		args := map[string]interface{}{
			"namespace": "default",
		}
		argsJSON, _ := json.Marshal(args)

		request := &mcp.CallToolRequest{
			Params: &mcp.CallToolParamsRaw{
				Arguments: argsJSON,
			},
		}

		result, err := handleWaypointApply(ctx, request)

		require.NoError(t, err)
		assert.NotNil(t, result)
		assert.True(t, result.IsError)
	})
}

func TestHandleWaypointDelete(t *testing.T) {
	ctx := context.Background()

	t.Run("delete waypoint with names", func(t *testing.T) {
		mock := cmd.NewMockShellExecutor()
		mock.AddCommandString("istioctl", []string{"waypoint", "delete", "waypoint1", "waypoint2", "-n", "default"}, "Waypoints deleted", nil)

		ctx = cmd.WithShellExecutor(ctx, mock)

		args := map[string]interface{}{
			"namespace": "default",
			"names":     "waypoint1,waypoint2",
		}
		argsJSON, _ := json.Marshal(args)

		request := &mcp.CallToolRequest{
			Params: &mcp.CallToolParamsRaw{
				Arguments: argsJSON,
			},
		}

		result, err := handleWaypointDelete(ctx, request)

		require.NoError(t, err)
		assert.NotNil(t, result)
		assert.False(t, result.IsError)
	})

	t.Run("delete all waypoints", func(t *testing.T) {
		mock := cmd.NewMockShellExecutor()
		mock.AddCommandString("istioctl", []string{"waypoint", "delete", "--all", "-n", "default"}, "All waypoints deleted", nil)

		ctx = cmd.WithShellExecutor(ctx, mock)

		args := map[string]interface{}{
			"namespace": "default",
			"all":       "true",
		}
		argsJSON, _ := json.Marshal(args)

		request := &mcp.CallToolRequest{
			Params: &mcp.CallToolParamsRaw{
				Arguments: argsJSON,
			},
		}

		result, err := handleWaypointDelete(ctx, request)

		require.NoError(t, err)
		assert.NotNil(t, result)
		assert.False(t, result.IsError)
	})

	t.Run("missing namespace parameter", func(t *testing.T) {
		request := &mcp.CallToolRequest{
			Params: &mcp.CallToolParamsRaw{
				Arguments: json.RawMessage(`{}`),
			},
		}

		result, err := handleWaypointDelete(ctx, request)

		require.NoError(t, err)
		assert.NotNil(t, result)
		assert.True(t, result.IsError)
	})
}

func TestHandleWaypointStatus(t *testing.T) {
	ctx := context.Background()

	t.Run("waypoint status with name", func(t *testing.T) {
		mock := cmd.NewMockShellExecutor()
		mock.AddCommandString("istioctl", []string{"waypoint", "status", "waypoint", "-n", "default"}, "Waypoint status", nil)

		ctx = cmd.WithShellExecutor(ctx, mock)

		args := map[string]interface{}{
			"namespace": "default",
			"name":      "waypoint",
		}
		argsJSON, _ := json.Marshal(args)

		request := &mcp.CallToolRequest{
			Params: &mcp.CallToolParamsRaw{
				Arguments: argsJSON,
			},
		}

		result, err := handleWaypointStatus(ctx, request)

		require.NoError(t, err)
		assert.NotNil(t, result)
		assert.False(t, result.IsError)
	})

	t.Run("missing namespace parameter", func(t *testing.T) {
		request := &mcp.CallToolRequest{
			Params: &mcp.CallToolParamsRaw{
				Arguments: json.RawMessage(`{}`),
			},
		}

		result, err := handleWaypointStatus(ctx, request)

		require.NoError(t, err)
		assert.NotNil(t, result)
		assert.True(t, result.IsError)
	})
}

func TestHandleZtunnelConfig(t *testing.T) {
	ctx := context.Background()

	t.Run("ztunnel config with namespace", func(t *testing.T) {
		mock := cmd.NewMockShellExecutor()
		mock.AddCommandString("istioctl", []string{"ztunnel", "config", "workload", "-n", "default"}, "Ztunnel config", nil)

		ctx = cmd.WithShellExecutor(ctx, mock)

		args := map[string]interface{}{
			"namespace":   "default",
			"config_type": "workload",
		}
		argsJSON, _ := json.Marshal(args)

		request := &mcp.CallToolRequest{
			Params: &mcp.CallToolParamsRaw{
				Arguments: argsJSON,
			},
		}

		result, err := handleZtunnelConfig(ctx, request)

		require.NoError(t, err)
		assert.NotNil(t, result)
		assert.False(t, result.IsError)
	})

	t.Run("ztunnel config without namespace", func(t *testing.T) {
		mock := cmd.NewMockShellExecutor()
		mock.AddCommandString("istioctl", []string{"ztunnel", "config", "all"}, "Ztunnel config", nil)

		ctx = cmd.WithShellExecutor(ctx, mock)

		request := &mcp.CallToolRequest{
			Params: &mcp.CallToolParamsRaw{
				Arguments: json.RawMessage(`{}`),
			},
		}

		result, err := handleZtunnelConfig(ctx, request)

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

		request := &mcp.CallToolRequest{
			Params: &mcp.CallToolParamsRaw{
				Arguments: json.RawMessage(`{}`),
			},
		}

		result, err := handleIstioProxyStatus(ctx, request)

		require.NoError(t, err)
		assert.NotNil(t, result)
		assert.True(t, result.IsError)
	})

	t.Run("invalid JSON arguments", func(t *testing.T) {
		request := &mcp.CallToolRequest{
			Params: &mcp.CallToolParamsRaw{
				Arguments: json.RawMessage(`invalid json`),
			},
		}

		result, err := handleIstioProxyStatus(context.Background(), request)

		require.NoError(t, err)
		assert.NotNil(t, result)
		assert.True(t, result.IsError)
	})
}
