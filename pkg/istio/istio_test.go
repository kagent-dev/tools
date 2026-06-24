package istio

import (
	"context"
	"testing"

	"github.com/kagent-dev/tools/internal/cmd"
	mcp "github.com/kagent-dev/tools/internal/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRegisterTools(t *testing.T) {
	s := mcp.NewServer(&mcp.Implementation{Name: "test-server", Version: "v0.0.1"}, nil)
	RegisterTools(s, false) // false = enable all tools including write operations
}

func TestHandleIstioProxyStatus(t *testing.T) {
	ctx := context.Background()

	t.Run("basic proxy status", func(t *testing.T) {
		mock := cmd.NewMockShellExecutor()
		mock.AddCommandString("istioctl", []string{"proxy-status"}, "Proxy status output", nil)

		ctx = cmd.WithShellExecutor(ctx, mock)

		result, _, err := handleIstioProxyStatus(ctx, &mcp.CallToolRequest{}, istioProxyStatusInput{})

		require.NoError(t, err)
		assert.NotNil(t, result)
		assert.False(t, result.IsError)
	})

	t.Run("proxy status with namespace", func(t *testing.T) {
		mock := cmd.NewMockShellExecutor()
		mock.AddCommandString("istioctl", []string{"proxy-status", "-n", "istio-system"}, "Proxy status output", nil)

		ctx = cmd.WithShellExecutor(ctx, mock)

		result, _, err := handleIstioProxyStatus(ctx, &mcp.CallToolRequest{}, istioProxyStatusInput{
			Namespace: "istio-system",
		})

		require.NoError(t, err)
		assert.NotNil(t, result)
		assert.False(t, result.IsError)
	})

	t.Run("proxy status with pod name", func(t *testing.T) {
		mock := cmd.NewMockShellExecutor()
		mock.AddCommandString("istioctl", []string{"proxy-status", "-n", "default", "test-pod"}, "Proxy status output", nil)

		ctx = cmd.WithShellExecutor(ctx, mock)

		result, _, err := handleIstioProxyStatus(ctx, &mcp.CallToolRequest{}, istioProxyStatusInput{
			PodName:   "test-pod",
			Namespace: "default",
		})

		require.NoError(t, err)
		assert.NotNil(t, result)
		assert.False(t, result.IsError)
	})
}

func TestHandleIstioProxyConfig(t *testing.T) {
	ctx := context.Background()

	t.Run("missing pod_name parameter", func(t *testing.T) {
		result, _, err := handleIstioProxyConfig(ctx, &mcp.CallToolRequest{}, istioProxyConfigInput{})

		require.NoError(t, err)
		assert.NotNil(t, result)
		assert.True(t, result.IsError)
	})

	t.Run("proxy config with pod name", func(t *testing.T) {
		mock := cmd.NewMockShellExecutor()
		mock.AddCommandString("istioctl", []string{"proxy-config", "all", "test-pod"}, "Proxy config output", nil)

		ctx = cmd.WithShellExecutor(ctx, mock)

		result, _, err := handleIstioProxyConfig(ctx, &mcp.CallToolRequest{}, istioProxyConfigInput{
			PodName: "test-pod",
		})

		require.NoError(t, err)
		assert.NotNil(t, result)
		assert.False(t, result.IsError)
	})

	t.Run("proxy config with namespace", func(t *testing.T) {
		mock := cmd.NewMockShellExecutor()
		mock.AddCommandString("istioctl", []string{"proxy-config", "cluster", "test-pod.default"}, "Proxy config output", nil)

		ctx = cmd.WithShellExecutor(ctx, mock)

		result, _, err := handleIstioProxyConfig(ctx, &mcp.CallToolRequest{}, istioProxyConfigInput{
			PodName:    "test-pod",
			Namespace:  "default",
			ConfigType: "cluster",
		})

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

		result, _, err := handleIstioInstall(ctx, &mcp.CallToolRequest{}, istioInstallInput{})

		require.NoError(t, err)
		assert.NotNil(t, result)
		assert.False(t, result.IsError)
	})

	t.Run("install with custom profile", func(t *testing.T) {
		mock := cmd.NewMockShellExecutor()
		mock.AddCommandString("istioctl", []string{"install", "--set", "profile=demo", "-y"}, "Install completed", nil)

		ctx = cmd.WithShellExecutor(ctx, mock)

		result, _, err := handleIstioInstall(ctx, &mcp.CallToolRequest{}, istioInstallInput{
			Profile: "demo",
		})

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

	result, _, err := handleIstioGenerateManifest(ctx, &mcp.CallToolRequest{}, istioGenerateManifestInput{
		Profile: "minimal",
	})

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

		result, _, err := handleIstioAnalyzeClusterConfiguration(ctx, &mcp.CallToolRequest{}, istioAnalyzeClusterConfigurationInput{
			AllNamespaces: true,
		})

		require.NoError(t, err)
		assert.NotNil(t, result)
		assert.False(t, result.IsError)
	})

	t.Run("analyze specific namespace", func(t *testing.T) {
		mock := cmd.NewMockShellExecutor()
		mock.AddCommandString("istioctl", []string{"analyze", "-n", "default"}, "Analysis output", nil)

		ctx = cmd.WithShellExecutor(ctx, mock)

		result, _, err := handleIstioAnalyzeClusterConfiguration(ctx, &mcp.CallToolRequest{}, istioAnalyzeClusterConfigurationInput{
			Namespace: "default",
		})

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

		result, _, err := handleIstioVersion(ctx, &mcp.CallToolRequest{}, istioVersionInput{})

		require.NoError(t, err)
		assert.NotNil(t, result)
		assert.False(t, result.IsError)
	})

	t.Run("version short", func(t *testing.T) {
		mock := cmd.NewMockShellExecutor()
		mock.AddCommandString("istioctl", []string{"version", "--short"}, "1.18.0", nil)

		ctx = cmd.WithShellExecutor(ctx, mock)

		result, _, err := handleIstioVersion(ctx, &mcp.CallToolRequest{}, istioVersionInput{
			Short: true,
		})

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

	result, _, err := handleIstioRemoteClusters(ctx, &mcp.CallToolRequest{}, istioRemoteClustersInput{})

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

		result, _, err := handleWaypointList(ctx, &mcp.CallToolRequest{}, waypointListInput{
			AllNamespaces: true,
		})

		require.NoError(t, err)
		assert.NotNil(t, result)
		assert.False(t, result.IsError)
	})

	t.Run("list waypoints in a specific namespace", func(t *testing.T) {
		mock := cmd.NewMockShellExecutor()
		mock.AddCommandString("istioctl", []string{"waypoint", "list", "-n", "default"}, "Waypoints list", nil)

		ctx = cmd.WithShellExecutor(ctx, mock)

		result, _, err := handleWaypointList(ctx, &mcp.CallToolRequest{}, waypointListInput{
			Namespace: "default",
		})

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

		result, _, err := handleWaypointGenerate(ctx, &mcp.CallToolRequest{}, waypointGenerateInput{
			Namespace:   "default",
			Name:        "waypoint",
			TrafficType: "all",
		})

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

		result, _, err := handleIstioProxyStatus(ctx, &mcp.CallToolRequest{}, istioProxyStatusInput{})

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

		result, _, err := handleWaypointApply(ctx, &mcp.CallToolRequest{}, waypointApplyInput{Namespace: "default"})
		require.NoError(t, err)
		assert.False(t, result.IsError)
	})

	t.Run("apply with enroll-namespace", func(t *testing.T) {
		mock := cmd.NewMockShellExecutor()
		mock.AddCommandString("istioctl", []string{"waypoint", "apply", "-n", "default", "--enroll-namespace"}, "applied", nil)
		ctx := cmd.WithShellExecutor(context.Background(), mock)

		result, _, err := handleWaypointApply(ctx, &mcp.CallToolRequest{}, waypointApplyInput{
			Namespace:       "default",
			EnrollNamespace: true,
		})
		require.NoError(t, err)
		assert.False(t, result.IsError)
	})

	t.Run("missing namespace", func(t *testing.T) {
		mock := cmd.NewMockShellExecutor()
		ctx := cmd.WithShellExecutor(context.Background(), mock)
		result, _, err := handleWaypointApply(ctx, &mcp.CallToolRequest{}, waypointApplyInput{})
		require.NoError(t, err)
		assert.True(t, result.IsError)
	})

	t.Run("command failure", func(t *testing.T) {
		mock := cmd.NewMockShellExecutor()
		mock.AddCommandString("istioctl", []string{"waypoint", "apply", "-n", "default"}, "", assert.AnError)
		ctx := cmd.WithShellExecutor(context.Background(), mock)
		result, _, err := handleWaypointApply(ctx, &mcp.CallToolRequest{}, waypointApplyInput{Namespace: "default"})
		require.NoError(t, err)
		assert.True(t, result.IsError)
	})
}

func TestHandleWaypointDelete(t *testing.T) {
	t.Run("delete all", func(t *testing.T) {
		mock := cmd.NewMockShellExecutor()
		mock.AddCommandString("istioctl", []string{"waypoint", "delete", "--all", "-n", "default"}, "deleted", nil)
		ctx := cmd.WithShellExecutor(context.Background(), mock)
		result, _, err := handleWaypointDelete(ctx, &mcp.CallToolRequest{}, waypointDeleteInput{
			Namespace: "default",
			All:       true,
		})
		require.NoError(t, err)
		assert.False(t, result.IsError)
	})

	t.Run("delete by names", func(t *testing.T) {
		mock := cmd.NewMockShellExecutor()
		mock.AddCommandString("istioctl", []string{"waypoint", "delete", "wp1", "wp2", "-n", "default"}, "deleted", nil)
		ctx := cmd.WithShellExecutor(context.Background(), mock)
		result, _, err := handleWaypointDelete(ctx, &mcp.CallToolRequest{}, waypointDeleteInput{
			Namespace: "default",
			Names:     "wp1, wp2",
		})
		require.NoError(t, err)
		assert.False(t, result.IsError)
	})

	t.Run("missing namespace", func(t *testing.T) {
		mock := cmd.NewMockShellExecutor()
		ctx := cmd.WithShellExecutor(context.Background(), mock)
		result, _, err := handleWaypointDelete(ctx, &mcp.CallToolRequest{}, waypointDeleteInput{})
		require.NoError(t, err)
		assert.True(t, result.IsError)
	})
}

func TestHandleWaypointStatus(t *testing.T) {
	t.Run("status with name", func(t *testing.T) {
		mock := cmd.NewMockShellExecutor()
		mock.AddCommandString("istioctl", []string{"waypoint", "status", "wp1", "-n", "default"}, "status", nil)
		ctx := cmd.WithShellExecutor(context.Background(), mock)
		result, _, err := handleWaypointStatus(ctx, &mcp.CallToolRequest{}, waypointStatusInput{
			Namespace: "default",
			Name:      "wp1",
		})
		require.NoError(t, err)
		assert.False(t, result.IsError)
	})

	t.Run("status without name", func(t *testing.T) {
		mock := cmd.NewMockShellExecutor()
		mock.AddCommandString("istioctl", []string{"waypoint", "status", "-n", "default"}, "status", nil)
		ctx := cmd.WithShellExecutor(context.Background(), mock)
		result, _, err := handleWaypointStatus(ctx, &mcp.CallToolRequest{}, waypointStatusInput{
			Namespace: "default",
		})
		require.NoError(t, err)
		assert.False(t, result.IsError)
	})

	t.Run("missing namespace", func(t *testing.T) {
		mock := cmd.NewMockShellExecutor()
		ctx := cmd.WithShellExecutor(context.Background(), mock)
		result, _, err := handleWaypointStatus(ctx, &mcp.CallToolRequest{}, waypointStatusInput{})
		require.NoError(t, err)
		assert.True(t, result.IsError)
	})
}

func TestHandleZtunnelConfig(t *testing.T) {
	t.Run("default config type", func(t *testing.T) {
		mock := cmd.NewMockShellExecutor()
		mock.AddCommandString("istioctl", []string{"ztunnel", "config", "all"}, "ztunnel config", nil)
		ctx := cmd.WithShellExecutor(context.Background(), mock)
		result, _, err := handleZtunnelConfig(ctx, &mcp.CallToolRequest{}, ztunnelConfigInput{})
		require.NoError(t, err)
		assert.False(t, result.IsError)
	})

	t.Run("with namespace and config type", func(t *testing.T) {
		mock := cmd.NewMockShellExecutor()
		mock.AddCommandString("istioctl", []string{"ztunnel", "config", "workloads", "-n", "istio-system"}, "ztunnel config", nil)
		ctx := cmd.WithShellExecutor(context.Background(), mock)
		result, _, err := handleZtunnelConfig(ctx, &mcp.CallToolRequest{}, ztunnelConfigInput{
			ConfigType: "workloads",
			Namespace:  "istio-system",
		})
		require.NoError(t, err)
		assert.False(t, result.IsError)
	})

	t.Run("command failure", func(t *testing.T) {
		mock := cmd.NewMockShellExecutor()
		mock.AddCommandString("istioctl", []string{"ztunnel", "config", "all"}, "", assert.AnError)
		ctx := cmd.WithShellExecutor(context.Background(), mock)
		result, _, err := handleZtunnelConfig(ctx, &mcp.CallToolRequest{}, ztunnelConfigInput{})
		require.NoError(t, err)
		assert.True(t, result.IsError)
	})
}
