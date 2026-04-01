package cilium

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/kagent-dev/tools/internal/cmd"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRegisterCiliumTools(t *testing.T) {
	s := server.NewMCPServer("test-server", "v0.0.1")
	RegisterTools(s, false) // false = enable all tools including write operations
	// We can't directly check the tools, but we can ensure the call doesn't panic
}

func TestHandleCiliumStatusAndVersion(t *testing.T) {
	ctx := context.Background()
	mock := cmd.NewMockShellExecutor()
	mock.AddCommandString("cilium", []string{"status"}, "Cilium status: OK", nil)
	mock.AddCommandString("cilium", []string{"version"}, "cilium version 1.14.0", nil)

	ctx = cmd.WithShellExecutor(ctx, mock)

	result, err := handleCiliumStatusAndVersion(ctx, mcp.CallToolRequest{})
	require.NoError(t, err)
	assert.NotNil(t, result)
	assert.False(t, result.IsError)

	var textContent mcp.TextContent
	var ok bool
	for _, content := range result.Content {
		if textContent, ok = content.(mcp.TextContent); ok {
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

	result, err := handleCiliumStatusAndVersion(ctx, mcp.CallToolRequest{})
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

	result, err := handleInstallCilium(ctx, mcp.CallToolRequest{})
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

	result, err := handleUninstallCilium(ctx, mcp.CallToolRequest{})
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

	result, err := handleUpgradeCilium(ctx, mcp.CallToolRequest{})
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
		req := mcp.CallToolRequest{
			Params: mcp.CallToolParams{
				Arguments: map[string]any{
					"cluster_name": "my-cluster",
				},
			},
		}

		result, err := handleConnectToRemoteCluster(ctx, req)
		require.NoError(t, err)
		assert.NotNil(t, result)
		assert.False(t, result.IsError)
		assert.Contains(t, getResultText(result), "✓ Connected to cluster my-cluster!")
	})

	t.Run("missing cluster_name", func(t *testing.T) {
		req := mcp.CallToolRequest{
			Params: mcp.CallToolParams{
				Arguments: map[string]any{},
			},
		}
		result, err := handleConnectToRemoteCluster(ctx, req)
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
		req := mcp.CallToolRequest{
			Params: mcp.CallToolParams{
				Arguments: map[string]any{
					"cluster_name": "my-cluster",
				},
			},
		}

		result, err := handleDisconnectRemoteCluster(ctx, req)
		require.NoError(t, err)
		assert.NotNil(t, result)
		assert.False(t, result.IsError)
		assert.Contains(t, getResultText(result), "✓ Disconnected from cluster my-cluster!")
	})

	t.Run("missing cluster_name", func(t *testing.T) {
		req := mcp.CallToolRequest{
			Params: mcp.CallToolParams{
				Arguments: map[string]any{},
			},
		}
		result, err := handleDisconnectRemoteCluster(ctx, req)
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
	req := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Arguments: map[string]any{
				"enable": true,
			},
		},
	}

	result, err := handleToggleHubble(ctx, req)
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
	req := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Arguments: map[string]any{
				"enable": false,
			},
		},
	}
	result, err := handleToggleHubble(ctx, req)
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
	result, err := handleListBGPPeers(ctx, mcp.CallToolRequest{})
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
	result, err := handleListBGPRoutes(ctx, mcp.CallToolRequest{})
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

// mockCiliumDbgCommand sets up the mock for a cilium-dbg command executed via kubectl exec.
// It mocks: (1) kubectl get pods to resolve the cilium pod name, (2) kubectl exec to run cilium-dbg.
func mockCiliumDbgCommand(mock *cmd.MockShellExecutor, dbgArgs []string, output string, err error) {
	// Mock the pod name lookup
	mock.AddCommandString("kubectl", []string{
		"get", "pods", "-n", "kube-system",
		"--selector=k8s-app=cilium",
		"--field-selector=spec.nodeName=test-node",
		"-o", "jsonpath={.items[0].metadata.name}",
	}, "cilium-abc123", nil)

	// Mock the kubectl exec call
	execArgs := []string{"exec", "-n", "kube-system", "cilium-abc123", "--", "cilium-dbg"}
	execArgs = append(execArgs, dbgArgs...)
	mock.AddCommandString("kubectl", execArgs, output, err)
}

func newRequestWithArgs(args map[string]any) mcp.CallToolRequest {
	return mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Arguments: args,
		},
	}
}

func TestHandleGetEndpointsList(t *testing.T) {
	ctx := context.Background()
	mock := cmd.NewMockShellExecutor()
	mockCiliumDbgCommand(mock, []string{"endpoint", "list"}, "ENDPOINT   POLICY\n34   Disabled", nil)
	ctx = cmd.WithShellExecutor(ctx, mock)

	req := newRequestWithArgs(map[string]any{"node_name": "test-node"})
	result, err := handleGetEndpointsList(ctx, req)
	require.NoError(t, err)
	assert.False(t, result.IsError)
	assert.Contains(t, getResultText(result), "ENDPOINT")
}

func TestHandleGetEndpointDetails(t *testing.T) {
	ctx := context.Background()
	mock := cmd.NewMockShellExecutor()
	mockCiliumDbgCommand(mock, []string{"endpoint", "get", "34", "-o", "json"}, `{"id": 34}`, nil)
	ctx = cmd.WithShellExecutor(ctx, mock)

	req := newRequestWithArgs(map[string]any{"endpoint_id": "34", "node_name": "test-node"})
	result, err := handleGetEndpointDetails(ctx, req)
	require.NoError(t, err)
	assert.False(t, result.IsError)
	assert.Contains(t, getResultText(result), `"id": 34`)
}

func TestHandleGetEndpointLogs(t *testing.T) {
	ctx := context.Background()
	mock := cmd.NewMockShellExecutor()
	mockCiliumDbgCommand(mock, []string{"endpoint", "logs", "34"}, "endpoint log output", nil)
	ctx = cmd.WithShellExecutor(ctx, mock)

	req := newRequestWithArgs(map[string]any{"endpoint_id": "34", "node_name": "test-node"})
	result, err := handleGetEndpointLogs(ctx, req)
	require.NoError(t, err)
	assert.False(t, result.IsError)
	assert.Contains(t, getResultText(result), "endpoint log output")
}

func TestHandleGetEndpointHealth(t *testing.T) {
	ctx := context.Background()
	mock := cmd.NewMockShellExecutor()
	mockCiliumDbgCommand(mock, []string{"endpoint", "health", "34"}, "endpoint health OK", nil)
	ctx = cmd.WithShellExecutor(ctx, mock)

	req := newRequestWithArgs(map[string]any{"endpoint_id": "34", "node_name": "test-node"})
	result, err := handleGetEndpointHealth(ctx, req)
	require.NoError(t, err)
	assert.False(t, result.IsError)
	assert.Contains(t, getResultText(result), "endpoint health OK")
}

func TestHandleShowConfigurationOptions(t *testing.T) {
	t.Run("default", func(t *testing.T) {
		ctx := context.Background()
		mock := cmd.NewMockShellExecutor()
		mockCiliumDbgCommand(mock, []string{"config"}, "PolicyEnforcement=default", nil)
		ctx = cmd.WithShellExecutor(ctx, mock)

		req := newRequestWithArgs(map[string]any{"node_name": "test-node"})
		result, err := handleShowConfigurationOptions(ctx, req)
		require.NoError(t, err)
		assert.False(t, result.IsError)
		assert.Contains(t, getResultText(result), "PolicyEnforcement")
	})

	t.Run("all", func(t *testing.T) {
		ctx := context.Background()
		mock := cmd.NewMockShellExecutor()
		mockCiliumDbgCommand(mock, []string{"config", "--all"}, "all config options", nil)
		ctx = cmd.WithShellExecutor(ctx, mock)

		req := newRequestWithArgs(map[string]any{"node_name": "test-node", "list_all": "true"})
		result, err := handleShowConfigurationOptions(ctx, req)
		require.NoError(t, err)
		assert.False(t, result.IsError)
		assert.Contains(t, getResultText(result), "all config options")
	})

	t.Run("read_only", func(t *testing.T) {
		ctx := context.Background()
		mock := cmd.NewMockShellExecutor()
		mockCiliumDbgCommand(mock, []string{"config", "-r"}, "read only config", nil)
		ctx = cmd.WithShellExecutor(ctx, mock)

		req := newRequestWithArgs(map[string]any{"node_name": "test-node", "list_read_only": "true"})
		result, err := handleShowConfigurationOptions(ctx, req)
		require.NoError(t, err)
		assert.False(t, result.IsError)
		assert.Contains(t, getResultText(result), "read only config")
	})
}

func TestHandleToggleConfigurationOption(t *testing.T) {
	ctx := context.Background()
	mock := cmd.NewMockShellExecutor()
	mockCiliumDbgCommand(mock, []string{"config", "PolicyEnforcement=enable"}, "option toggled", nil)
	ctx = cmd.WithShellExecutor(ctx, mock)

	req := newRequestWithArgs(map[string]any{"option": "PolicyEnforcement", "value": "true", "node_name": "test-node"})
	result, err := handleToggleConfigurationOption(ctx, req)
	require.NoError(t, err)
	assert.False(t, result.IsError)
	assert.Contains(t, getResultText(result), "option toggled")
}

func TestHandleListIdentities(t *testing.T) {
	ctx := context.Background()
	mock := cmd.NewMockShellExecutor()
	mockCiliumDbgCommand(mock, []string{"identity", "list"}, "ID  LABELS\n1   reserved:host", nil)
	ctx = cmd.WithShellExecutor(ctx, mock)

	req := newRequestWithArgs(map[string]any{"node_name": "test-node"})
	result, err := handleListIdentities(ctx, req)
	require.NoError(t, err)
	assert.False(t, result.IsError)
	assert.Contains(t, getResultText(result), "reserved:host")
}

func TestHandleGetDaemonStatus(t *testing.T) {
	ctx := context.Background()
	mock := cmd.NewMockShellExecutor()
	mockCiliumDbgCommand(mock, []string{"status"}, "KVStore: Ok\nKubernetes: Ok", nil)
	ctx = cmd.WithShellExecutor(ctx, mock)

	req := newRequestWithArgs(map[string]any{"node_name": "test-node"})
	result, err := handleGetDaemonStatus(ctx, req)
	require.NoError(t, err)
	assert.False(t, result.IsError)
	assert.Contains(t, getResultText(result), "KVStore: Ok")
}

func TestHandleDisplayEncryptionState(t *testing.T) {
	ctx := context.Background()
	mock := cmd.NewMockShellExecutor()
	mockCiliumDbgCommand(mock, []string{"encrypt", "status"}, "Encryption: Disabled", nil)
	ctx = cmd.WithShellExecutor(ctx, mock)

	req := newRequestWithArgs(map[string]any{"node_name": "test-node"})
	result, err := handleDisplayEncryptionState(ctx, req)
	require.NoError(t, err)
	assert.False(t, result.IsError)
	assert.Contains(t, getResultText(result), "Encryption: Disabled")
}

func TestHandleShowDNSNames(t *testing.T) {
	ctx := context.Background()
	mock := cmd.NewMockShellExecutor()
	mockCiliumDbgCommand(mock, []string{"fqdn", "names"}, "DNS names output", nil)
	ctx = cmd.WithShellExecutor(ctx, mock)

	req := newRequestWithArgs(map[string]any{"node_name": "test-node"})
	result, err := handleShowDNSNames(ctx, req)
	require.NoError(t, err)
	assert.False(t, result.IsError)
	assert.Contains(t, getResultText(result), "DNS names output")
}

func TestHandleFQDNCache(t *testing.T) {
	ctx := context.Background()
	mock := cmd.NewMockShellExecutor()
	mockCiliumDbgCommand(mock, []string{"fqdn", "cache", "list"}, "FQDN cache entries", nil)
	ctx = cmd.WithShellExecutor(ctx, mock)

	req := newRequestWithArgs(map[string]any{"node_name": "test-node"})
	result, err := handleFQDNCache(ctx, req)
	require.NoError(t, err)
	assert.False(t, result.IsError)
	assert.Contains(t, getResultText(result), "FQDN cache entries")
}

func TestHandleListClusterNodes(t *testing.T) {
	ctx := context.Background()
	mock := cmd.NewMockShellExecutor()
	mockCiliumDbgCommand(mock, []string{"node", "list"}, "Name   IPv4 Address\nnode1  10.0.0.1", nil)
	ctx = cmd.WithShellExecutor(ctx, mock)

	req := newRequestWithArgs(map[string]any{"node_name": "test-node"})
	result, err := handleListClusterNodes(ctx, req)
	require.NoError(t, err)
	assert.False(t, result.IsError)
	assert.Contains(t, getResultText(result), "node1")
}

func TestHandleListNodeIds(t *testing.T) {
	ctx := context.Background()
	mock := cmd.NewMockShellExecutor()
	mockCiliumDbgCommand(mock, []string{"nodeid", "list"}, "ID   IP\n1   10.0.0.1", nil)
	ctx = cmd.WithShellExecutor(ctx, mock)

	req := newRequestWithArgs(map[string]any{"node_name": "test-node"})
	result, err := handleListNodeIds(ctx, req)
	require.NoError(t, err)
	assert.False(t, result.IsError)
	assert.Contains(t, getResultText(result), "10.0.0.1")
}

func TestHandleListBPFMaps(t *testing.T) {
	ctx := context.Background()
	mock := cmd.NewMockShellExecutor()
	mockCiliumDbgCommand(mock, []string{"map", "list"}, "Name   Num entries\ncilium_lb4   22", nil)
	ctx = cmd.WithShellExecutor(ctx, mock)

	req := newRequestWithArgs(map[string]any{"node_name": "test-node"})
	result, err := handleListBPFMaps(ctx, req)
	require.NoError(t, err)
	assert.False(t, result.IsError)
	assert.Contains(t, getResultText(result), "cilium_lb4")
}

func TestHandleGetBPFMap(t *testing.T) {
	ctx := context.Background()
	mock := cmd.NewMockShellExecutor()
	mockCiliumDbgCommand(mock, []string{"map", "get", "cilium_lb4"}, "map contents", nil)
	ctx = cmd.WithShellExecutor(ctx, mock)

	req := newRequestWithArgs(map[string]any{"map_name": "cilium_lb4", "node_name": "test-node"})
	result, err := handleGetBPFMap(ctx, req)
	require.NoError(t, err)
	assert.False(t, result.IsError)
	assert.Contains(t, getResultText(result), "map contents")
}

func TestHandleListBPFMapEvents(t *testing.T) {
	ctx := context.Background()
	mock := cmd.NewMockShellExecutor()
	mockCiliumDbgCommand(mock, []string{"map", "events", "cilium_lb4"}, "map events", nil)
	ctx = cmd.WithShellExecutor(ctx, mock)

	req := newRequestWithArgs(map[string]any{"map_name": "cilium_lb4", "node_name": "test-node"})
	result, err := handleListBPFMapEvents(ctx, req)
	require.NoError(t, err)
	assert.False(t, result.IsError)
	assert.Contains(t, getResultText(result), "map events")
}

func TestHandleListMetrics(t *testing.T) {
	ctx := context.Background()
	mock := cmd.NewMockShellExecutor()
	mockCiliumDbgCommand(mock, []string{"metrics", "list"}, "Metric   Value\ncilium_endpoint_count   4", nil)
	ctx = cmd.WithShellExecutor(ctx, mock)

	req := newRequestWithArgs(map[string]any{"node_name": "test-node"})
	result, err := handleListMetrics(ctx, req)
	require.NoError(t, err)
	assert.False(t, result.IsError)
	assert.Contains(t, getResultText(result), "cilium_endpoint_count")
}

func TestHandleListServices(t *testing.T) {
	ctx := context.Background()
	mock := cmd.NewMockShellExecutor()
	mockCiliumDbgCommand(mock, []string{"service", "list"}, "ID   Frontend\n1   10.96.0.1:443", nil)
	ctx = cmd.WithShellExecutor(ctx, mock)

	req := newRequestWithArgs(map[string]any{"node_name": "test-node"})
	result, err := handleListServices(ctx, req)
	require.NoError(t, err)
	assert.False(t, result.IsError)
	assert.Contains(t, getResultText(result), "10.96.0.1")
}

func TestHandleListIPAddresses(t *testing.T) {
	ctx := context.Background()
	mock := cmd.NewMockShellExecutor()
	mockCiliumDbgCommand(mock, []string{"ip", "list"}, "IP   Identity\n10.0.0.1   1", nil)
	ctx = cmd.WithShellExecutor(ctx, mock)

	req := newRequestWithArgs(map[string]any{"node_name": "test-node"})
	result, err := handleListIPAddresses(ctx, req)
	require.NoError(t, err)
	assert.False(t, result.IsError)
	assert.Contains(t, getResultText(result), "10.0.0.1")
}

func TestHandleDisplaySelectors(t *testing.T) {
	ctx := context.Background()
	mock := cmd.NewMockShellExecutor()
	mockCiliumDbgCommand(mock, []string{"policy", "selectors"}, "SELECTOR   IDENTITIES", nil)
	ctx = cmd.WithShellExecutor(ctx, mock)

	req := newRequestWithArgs(map[string]any{"node_name": "test-node"})
	result, err := handleDisplaySelectors(ctx, req)
	require.NoError(t, err)
	assert.False(t, result.IsError)
	assert.Contains(t, getResultText(result), "SELECTOR")
}

func TestHandleListLocalRedirectPolicies(t *testing.T) {
	ctx := context.Background()
	mock := cmd.NewMockShellExecutor()
	mockCiliumDbgCommand(mock, []string{"lrp", "list"}, "No local redirect policies", nil)
	ctx = cmd.WithShellExecutor(ctx, mock)

	req := newRequestWithArgs(map[string]any{"node_name": "test-node"})
	result, err := handleListLocalRedirectPolicies(ctx, req)
	require.NoError(t, err)
	assert.False(t, result.IsError)
	assert.Contains(t, getResultText(result), "No local redirect policies")
}

func TestHandleRequestDebuggingInformation(t *testing.T) {
	ctx := context.Background()
	mock := cmd.NewMockShellExecutor()
	mockCiliumDbgCommand(mock, []string{"debuginfo"}, "debug info output", nil)
	ctx = cmd.WithShellExecutor(ctx, mock)

	req := newRequestWithArgs(map[string]any{"node_name": "test-node"})
	result, err := handleRequestDebuggingInformation(ctx, req)
	require.NoError(t, err)
	assert.False(t, result.IsError)
	assert.Contains(t, getResultText(result), "debug info output")
}

func TestHandleListXDPCIDRFilters(t *testing.T) {
	ctx := context.Background()
	mock := cmd.NewMockShellExecutor()
	mockCiliumDbgCommand(mock, []string{"prefilter", "list"}, "CIDR filters", nil)
	ctx = cmd.WithShellExecutor(ctx, mock)

	req := newRequestWithArgs(map[string]any{"node_name": "test-node"})
	result, err := handleListXDPCIDRFilters(ctx, req)
	require.NoError(t, err)
	assert.False(t, result.IsError)
	assert.Contains(t, getResultText(result), "CIDR filters")
}

func getResultText(r *mcp.CallToolResult) string {
	if r == nil || len(r.Content) == 0 {
		return ""
	}
	if textContent, ok := r.Content[0].(mcp.TextContent); ok {
		return strings.TrimSpace(textContent.Text)
	}
	return ""
}
