package cilium

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/kagent-dev/tools/internal/cmd"
	mcp "github.com/kagent-dev/tools/internal/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func boolPtr(b bool) *bool { return &b }

func TestRegisterCiliumTools(t *testing.T) {
	s := mcp.NewServer(&mcp.Implementation{Name: "test-server", Version: "v0.0.1"}, nil)
	RegisterTools(s, false) // false = enable all tools including write operations
	// We can't directly check the tools, but we can ensure the call doesn't panic
}

func TestHandleCiliumStatusAndVersion(t *testing.T) {
	ctx := context.Background()
	mock := cmd.NewMockShellExecutor()
	mock.AddCommandString("cilium", []string{"status"}, "Cilium status: OK", nil)
	mock.AddCommandString("cilium", []string{"version"}, "cilium version 1.14.0", nil)

	ctx = cmd.WithShellExecutor(ctx, mock)

	result, _, err := handleCiliumStatusAndVersion(ctx, &mcp.CallToolRequest{}, noInput{})
	require.NoError(t, err)
	assert.NotNil(t, result)
	assert.False(t, result.IsError)

	text := getResultText(result)
	assert.Contains(t, text, "Cilium status: OK")
	assert.Contains(t, text, "cilium version 1.14.0")
}

func TestHandleCiliumStatusAndVersionError(t *testing.T) {
	ctx := context.Background()
	mock := cmd.NewMockShellExecutor()
	mock.AddCommandString("cilium", []string{"status"}, "", errors.New("command failed"))
	mock.AddCommandString("cilium", []string{"version"}, "cilium version 1.14.0", nil)

	ctx = cmd.WithShellExecutor(ctx, mock)

	result, _, err := handleCiliumStatusAndVersion(ctx, &mcp.CallToolRequest{}, noInput{})
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

	result, _, err := handleInstallCilium(ctx, &mcp.CallToolRequest{}, installCiliumInput{})
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

	result, _, err := handleUninstallCilium(ctx, &mcp.CallToolRequest{}, noInput{})
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

	result, _, err := handleUpgradeCilium(ctx, &mcp.CallToolRequest{}, upgradeCiliumInput{})
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
		result, _, err := handleConnectToRemoteCluster(ctx, &mcp.CallToolRequest{}, connectToRemoteClusterInput{ClusterName: "my-cluster"})
		require.NoError(t, err)
		assert.NotNil(t, result)
		assert.False(t, result.IsError)
		assert.Contains(t, getResultText(result), "✓ Connected to cluster my-cluster!")
	})

	t.Run("missing cluster_name", func(t *testing.T) {
		result, _, err := handleConnectToRemoteCluster(ctx, &mcp.CallToolRequest{}, connectToRemoteClusterInput{})
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
		result, _, err := handleDisconnectRemoteCluster(ctx, &mcp.CallToolRequest{}, disconnectRemoteClusterInput{ClusterName: "my-cluster"})
		require.NoError(t, err)
		assert.NotNil(t, result)
		assert.False(t, result.IsError)
		assert.Contains(t, getResultText(result), "✓ Disconnected from cluster my-cluster!")
	})

	t.Run("missing cluster_name", func(t *testing.T) {
		result, _, err := handleDisconnectRemoteCluster(ctx, &mcp.CallToolRequest{}, disconnectRemoteClusterInput{})
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
	result, _, err := handleToggleHubble(ctx, &mcp.CallToolRequest{}, enableToggleInput{Enable: boolPtr(true)})
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
	result, _, err := handleToggleHubble(ctx, &mcp.CallToolRequest{}, enableToggleInput{Enable: boolPtr(false)})
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
	result, _, err := handleListBGPPeers(ctx, &mcp.CallToolRequest{}, noInput{})
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
	result, _, err := handleListBGPRoutes(ctx, &mcp.CallToolRequest{}, noInput{})
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

func TestHandleGetEndpointsList(t *testing.T) {
	ctx := context.Background()
	mock := cmd.NewMockShellExecutor()
	mockCiliumDbgCommand(mock, []string{"endpoint", "list"}, "ENDPOINT   POLICY\n34   Disabled", nil)
	ctx = cmd.WithShellExecutor(ctx, mock)

	result, _, err := handleGetEndpointsList(ctx, &mcp.CallToolRequest{}, nodeNameInput{NodeName: "test-node"})
	require.NoError(t, err)
	assert.False(t, result.IsError)
	assert.Contains(t, getResultText(result), "ENDPOINT")
}

func TestHandleGetEndpointDetails(t *testing.T) {
	ctx := context.Background()
	mock := cmd.NewMockShellExecutor()
	mockCiliumDbgCommand(mock, []string{"endpoint", "get", "34", "-o", "json"}, `{"id": 34}`, nil)
	ctx = cmd.WithShellExecutor(ctx, mock)

	result, _, err := handleGetEndpointDetails(ctx, &mcp.CallToolRequest{}, getEndpointDetailsInput{EndpointID: "34", NodeName: "test-node"})
	require.NoError(t, err)
	assert.False(t, result.IsError)
	assert.Contains(t, getResultText(result), `"id": 34`)
}

func TestHandleGetEndpointLogs(t *testing.T) {
	ctx := context.Background()
	mock := cmd.NewMockShellExecutor()
	mockCiliumDbgCommand(mock, []string{"endpoint", "logs", "34"}, "endpoint log output", nil)
	ctx = cmd.WithShellExecutor(ctx, mock)

	result, _, err := handleGetEndpointLogs(ctx, &mcp.CallToolRequest{}, getEndpointLogsInput{EndpointID: "34", NodeName: "test-node"})
	require.NoError(t, err)
	assert.False(t, result.IsError)
	assert.Contains(t, getResultText(result), "endpoint log output")
}

func TestHandleGetEndpointHealth(t *testing.T) {
	ctx := context.Background()
	mock := cmd.NewMockShellExecutor()
	mockCiliumDbgCommand(mock, []string{"endpoint", "health", "34"}, "endpoint health OK", nil)
	ctx = cmd.WithShellExecutor(ctx, mock)

	result, _, err := handleGetEndpointHealth(ctx, &mcp.CallToolRequest{}, getEndpointHealthInput{EndpointID: "34", NodeName: "test-node"})
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

		result, _, err := handleShowConfigurationOptions(ctx, &mcp.CallToolRequest{}, showConfigurationOptionsInput{NodeName: "test-node"})
		require.NoError(t, err)
		assert.False(t, result.IsError)
		assert.Contains(t, getResultText(result), "PolicyEnforcement")
	})

	t.Run("all", func(t *testing.T) {
		ctx := context.Background()
		mock := cmd.NewMockShellExecutor()
		mockCiliumDbgCommand(mock, []string{"config", "--all"}, "all config options", nil)
		ctx = cmd.WithShellExecutor(ctx, mock)

		result, _, err := handleShowConfigurationOptions(ctx, &mcp.CallToolRequest{}, showConfigurationOptionsInput{NodeName: "test-node", ListAll: true})
		require.NoError(t, err)
		assert.False(t, result.IsError)
		assert.Contains(t, getResultText(result), "all config options")
	})

	t.Run("read_only", func(t *testing.T) {
		ctx := context.Background()
		mock := cmd.NewMockShellExecutor()
		mockCiliumDbgCommand(mock, []string{"config", "-r"}, "read only config", nil)
		ctx = cmd.WithShellExecutor(ctx, mock)

		result, _, err := handleShowConfigurationOptions(ctx, &mcp.CallToolRequest{}, showConfigurationOptionsInput{NodeName: "test-node", ListReadOnly: true})
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

	result, _, err := handleToggleConfigurationOption(ctx, &mcp.CallToolRequest{}, toggleConfigurationOptionInput{Option: "PolicyEnforcement", Value: boolPtr(true), NodeName: "test-node"})
	require.NoError(t, err)
	assert.False(t, result.IsError)
	assert.Contains(t, getResultText(result), "option toggled")
}

func TestHandleListIdentities(t *testing.T) {
	ctx := context.Background()
	mock := cmd.NewMockShellExecutor()
	mockCiliumDbgCommand(mock, []string{"identity", "list"}, "ID  LABELS\n1   reserved:host", nil)
	ctx = cmd.WithShellExecutor(ctx, mock)

	result, _, err := handleListIdentities(ctx, &mcp.CallToolRequest{}, nodeNameInput{NodeName: "test-node"})
	require.NoError(t, err)
	assert.False(t, result.IsError)
	assert.Contains(t, getResultText(result), "reserved:host")
}

func TestHandleGetDaemonStatus(t *testing.T) {
	ctx := context.Background()
	mock := cmd.NewMockShellExecutor()
	mockCiliumDbgCommand(mock, []string{"status"}, "KVStore: Ok\nKubernetes: Ok", nil)
	ctx = cmd.WithShellExecutor(ctx, mock)

	result, _, err := handleGetDaemonStatus(ctx, &mcp.CallToolRequest{}, getDaemonStatusInput{NodeName: "test-node"})
	require.NoError(t, err)
	assert.False(t, result.IsError)
	assert.Contains(t, getResultText(result), "KVStore: Ok")
}

func TestHandleDisplayEncryptionState(t *testing.T) {
	ctx := context.Background()
	mock := cmd.NewMockShellExecutor()
	mockCiliumDbgCommand(mock, []string{"encrypt", "status"}, "Encryption: Disabled", nil)
	ctx = cmd.WithShellExecutor(ctx, mock)

	result, _, err := handleDisplayEncryptionState(ctx, &mcp.CallToolRequest{}, nodeNameInput{NodeName: "test-node"})
	require.NoError(t, err)
	assert.False(t, result.IsError)
	assert.Contains(t, getResultText(result), "Encryption: Disabled")
}

func TestHandleShowDNSNames(t *testing.T) {
	ctx := context.Background()
	mock := cmd.NewMockShellExecutor()
	mockCiliumDbgCommand(mock, []string{"fqdn", "names"}, "DNS names output", nil)
	ctx = cmd.WithShellExecutor(ctx, mock)

	result, _, err := handleShowDNSNames(ctx, &mcp.CallToolRequest{}, nodeNameInput{NodeName: "test-node"})
	require.NoError(t, err)
	assert.False(t, result.IsError)
	assert.Contains(t, getResultText(result), "DNS names output")
}

func TestHandleFQDNCache(t *testing.T) {
	ctx := context.Background()
	mock := cmd.NewMockShellExecutor()
	mockCiliumDbgCommand(mock, []string{"fqdn", "cache", "list"}, "FQDN cache entries", nil)
	ctx = cmd.WithShellExecutor(ctx, mock)

	result, _, err := handleFQDNCache(ctx, &mcp.CallToolRequest{}, fqdnCacheInput{NodeName: "test-node"})
	require.NoError(t, err)
	assert.False(t, result.IsError)
	assert.Contains(t, getResultText(result), "FQDN cache entries")
}

func TestHandleListClusterNodes(t *testing.T) {
	ctx := context.Background()
	mock := cmd.NewMockShellExecutor()
	mockCiliumDbgCommand(mock, []string{"node", "list"}, "Name   IPv4 Address\nnode1  10.0.0.1", nil)
	ctx = cmd.WithShellExecutor(ctx, mock)

	result, _, err := handleListClusterNodes(ctx, &mcp.CallToolRequest{}, nodeNameInput{NodeName: "test-node"})
	require.NoError(t, err)
	assert.False(t, result.IsError)
	assert.Contains(t, getResultText(result), "node1")
}

func TestHandleListNodeIds(t *testing.T) {
	ctx := context.Background()
	mock := cmd.NewMockShellExecutor()
	mockCiliumDbgCommand(mock, []string{"nodeid", "list"}, "ID   IP\n1   10.0.0.1", nil)
	ctx = cmd.WithShellExecutor(ctx, mock)

	result, _, err := handleListNodeIds(ctx, &mcp.CallToolRequest{}, nodeNameInput{NodeName: "test-node"})
	require.NoError(t, err)
	assert.False(t, result.IsError)
	assert.Contains(t, getResultText(result), "10.0.0.1")
}

func TestHandleListBPFMaps(t *testing.T) {
	ctx := context.Background()
	mock := cmd.NewMockShellExecutor()
	mockCiliumDbgCommand(mock, []string{"map", "list"}, "Name   Num entries\ncilium_lb4   22", nil)
	ctx = cmd.WithShellExecutor(ctx, mock)

	result, _, err := handleListBPFMaps(ctx, &mcp.CallToolRequest{}, nodeNameInput{NodeName: "test-node"})
	require.NoError(t, err)
	assert.False(t, result.IsError)
	assert.Contains(t, getResultText(result), "cilium_lb4")
}

func TestHandleGetBPFMap(t *testing.T) {
	ctx := context.Background()
	mock := cmd.NewMockShellExecutor()
	mockCiliumDbgCommand(mock, []string{"map", "get", "cilium_lb4"}, "map contents", nil)
	ctx = cmd.WithShellExecutor(ctx, mock)

	result, _, err := handleGetBPFMap(ctx, &mcp.CallToolRequest{}, bpfMapInput{MapName: "cilium_lb4", NodeName: "test-node"})
	require.NoError(t, err)
	assert.False(t, result.IsError)
	assert.Contains(t, getResultText(result), "map contents")
}

func TestHandleListBPFMapEvents(t *testing.T) {
	ctx := context.Background()
	mock := cmd.NewMockShellExecutor()
	mockCiliumDbgCommand(mock, []string{"map", "events", "cilium_lb4"}, "map events", nil)
	ctx = cmd.WithShellExecutor(ctx, mock)

	result, _, err := handleListBPFMapEvents(ctx, &mcp.CallToolRequest{}, bpfMapInput{MapName: "cilium_lb4", NodeName: "test-node"})
	require.NoError(t, err)
	assert.False(t, result.IsError)
	assert.Contains(t, getResultText(result), "map events")
}

func TestHandleListMetrics(t *testing.T) {
	ctx := context.Background()
	mock := cmd.NewMockShellExecutor()
	mockCiliumDbgCommand(mock, []string{"metrics", "list"}, "Metric   Value\ncilium_endpoint_count   4", nil)
	ctx = cmd.WithShellExecutor(ctx, mock)

	result, _, err := handleListMetrics(ctx, &mcp.CallToolRequest{}, listMetricsInput{NodeName: "test-node"})
	require.NoError(t, err)
	assert.False(t, result.IsError)
	assert.Contains(t, getResultText(result), "cilium_endpoint_count")
}

func TestHandleListServices(t *testing.T) {
	ctx := context.Background()
	mock := cmd.NewMockShellExecutor()
	mockCiliumDbgCommand(mock, []string{"service", "list"}, "ID   Frontend\n1   10.96.0.1:443", nil)
	ctx = cmd.WithShellExecutor(ctx, mock)

	result, _, err := handleListServices(ctx, &mcp.CallToolRequest{}, listServicesInput{NodeName: "test-node"})
	require.NoError(t, err)
	assert.False(t, result.IsError)
	assert.Contains(t, getResultText(result), "10.96.0.1")
}

func TestHandleListIPAddresses(t *testing.T) {
	ctx := context.Background()
	mock := cmd.NewMockShellExecutor()
	mockCiliumDbgCommand(mock, []string{"ip", "list"}, "IP   Identity\n10.0.0.1   1", nil)
	ctx = cmd.WithShellExecutor(ctx, mock)

	result, _, err := handleListIPAddresses(ctx, &mcp.CallToolRequest{}, nodeNameInput{NodeName: "test-node"})
	require.NoError(t, err)
	assert.False(t, result.IsError)
	assert.Contains(t, getResultText(result), "10.0.0.1")
}

func TestHandleDisplaySelectors(t *testing.T) {
	ctx := context.Background()
	mock := cmd.NewMockShellExecutor()
	mockCiliumDbgCommand(mock, []string{"policy", "selectors"}, "SELECTOR   IDENTITIES", nil)
	ctx = cmd.WithShellExecutor(ctx, mock)

	result, _, err := handleDisplaySelectors(ctx, &mcp.CallToolRequest{}, nodeNameInput{NodeName: "test-node"})
	require.NoError(t, err)
	assert.False(t, result.IsError)
	assert.Contains(t, getResultText(result), "SELECTOR")
}

func TestHandleListLocalRedirectPolicies(t *testing.T) {
	ctx := context.Background()
	mock := cmd.NewMockShellExecutor()
	mockCiliumDbgCommand(mock, []string{"lrp", "list"}, "No local redirect policies", nil)
	ctx = cmd.WithShellExecutor(ctx, mock)

	result, _, err := handleListLocalRedirectPolicies(ctx, &mcp.CallToolRequest{}, nodeNameInput{NodeName: "test-node"})
	require.NoError(t, err)
	assert.False(t, result.IsError)
	assert.Contains(t, getResultText(result), "No local redirect policies")
}

func TestHandleRequestDebuggingInformation(t *testing.T) {
	ctx := context.Background()
	mock := cmd.NewMockShellExecutor()
	mockCiliumDbgCommand(mock, []string{"debuginfo"}, "debug info output", nil)
	ctx = cmd.WithShellExecutor(ctx, mock)

	result, _, err := handleRequestDebuggingInformation(ctx, &mcp.CallToolRequest{}, nodeNameInput{NodeName: "test-node"})
	require.NoError(t, err)
	assert.False(t, result.IsError)
	assert.Contains(t, getResultText(result), "debug info output")
}

func TestHandleListXDPCIDRFilters(t *testing.T) {
	ctx := context.Background()
	mock := cmd.NewMockShellExecutor()
	mockCiliumDbgCommand(mock, []string{"prefilter", "list"}, "CIDR filters", nil)
	ctx = cmd.WithShellExecutor(ctx, mock)

	result, _, err := handleListXDPCIDRFilters(ctx, &mcp.CallToolRequest{}, nodeNameInput{NodeName: "test-node"})
	require.NoError(t, err)
	assert.False(t, result.IsError)
	assert.Contains(t, getResultText(result), "CIDR filters")
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

// TestCiliumDbgHandlers exercises the success path of every cilium-dbg based handler.
func TestCiliumDbgHandlers(t *testing.T) {
	cases := []struct {
		name    string
		dbgArgs []string
		expect  string
		run     func(context.Context) (*mcp.CallToolResult, error)
	}{
		{"manage_endpoint_labels", []string{"endpoint", "labels", "34", "--add", "key=val"}, "ok", func(ctx context.Context) (*mcp.CallToolResult, error) {
			r, _, err := handleManageEndpointLabels(ctx, &mcp.CallToolRequest{}, manageEndpointLabelsInput{EndpointID: "34", Labels: "key=val", NodeName: "test-node"})
			return r, err
		}},
		{"manage_endpoint_configuration", []string{"endpoint", "config", "34", "Debug=true"}, "ok", func(ctx context.Context) (*mcp.CallToolResult, error) {
			r, _, err := handleManageEndpointConfiguration(ctx, &mcp.CallToolRequest{}, manageEndpointConfigurationInput{EndpointID: "34", Config: "Debug=true", NodeName: "test-node"})
			return r, err
		}},
		{"disconnect_endpoint", []string{"endpoint", "disconnect", "34"}, "ok", func(ctx context.Context) (*mcp.CallToolResult, error) {
			r, _, err := handleDisconnectEndpoint(ctx, &mcp.CallToolRequest{}, disconnectEndpointInput{EndpointID: "34", NodeName: "test-node"})
			return r, err
		}},
		{"get_identity_details", []string{"identity", "get", "123"}, "ok", func(ctx context.Context) (*mcp.CallToolResult, error) {
			r, _, err := handleGetIdentityDetails(ctx, &mcp.CallToolRequest{}, getIdentityDetailsInput{IdentityID: "123", NodeName: "test-node"})
			return r, err
		}},
		{"flush_ipsec_state", []string{"encrypt", "flush", "-f"}, "ok", func(ctx context.Context) (*mcp.CallToolResult, error) {
			r, _, err := handleFlushIPsecState(ctx, &mcp.CallToolRequest{}, nodeNameInput{NodeName: "test-node"})
			return r, err
		}},
		{"list_envoy_config", []string{"envoy", "admin", "clusters"}, "ok", func(ctx context.Context) (*mcp.CallToolResult, error) {
			r, _, err := handleListEnvoyConfig(ctx, &mcp.CallToolRequest{}, listEnvoyConfigInput{ResourceName: "clusters", NodeName: "test-node"})
			return r, err
		}},
		{"show_ipcache_cidr", []string{"ip", "get", "10.0.0.0/24"}, "ok", func(ctx context.Context) (*mcp.CallToolResult, error) {
			r, _, err := handleShowIPCacheInformation(ctx, &mcp.CallToolRequest{}, showIPCacheInformationInput{CIDR: "10.0.0.0/24", NodeName: "test-node"})
			return r, err
		}},
		{"show_ipcache_labels", []string{"ip", "get", "--labels", "app=foo"}, "ok", func(ctx context.Context) (*mcp.CallToolResult, error) {
			r, _, err := handleShowIPCacheInformation(ctx, &mcp.CallToolRequest{}, showIPCacheInformationInput{Labels: "app=foo", NodeName: "test-node"})
			return r, err
		}},
		{"delete_kvstore_key", []string{"kvstore", "delete", "foo"}, "ok", func(ctx context.Context) (*mcp.CallToolResult, error) {
			r, _, err := handleDeleteKeyFromKVStore(ctx, &mcp.CallToolRequest{}, kvStoreKeyInput{Key: "foo", NodeName: "test-node"})
			return r, err
		}},
		{"get_kvstore_key", []string{"kvstore", "get", "foo"}, "ok", func(ctx context.Context) (*mcp.CallToolResult, error) {
			r, _, err := handleGetKVStoreKey(ctx, &mcp.CallToolRequest{}, kvStoreKeyInput{Key: "foo", NodeName: "test-node"})
			return r, err
		}},
		{"set_kvstore_key", []string{"kvstore", "set", "foo=bar"}, "ok", func(ctx context.Context) (*mcp.CallToolResult, error) {
			r, _, err := handleSetKVStoreKey(ctx, &mcp.CallToolRequest{}, setKVStoreKeyInput{Key: "foo", Value: "bar", NodeName: "test-node"})
			return r, err
		}},
		{"show_load_information", []string{"loadinfo"}, "ok", func(ctx context.Context) (*mcp.CallToolResult, error) {
			r, _, err := handleShowLoadInformation(ctx, &mcp.CallToolRequest{}, nodeNameInput{NodeName: "test-node"})
			return r, err
		}},
		{"display_policy_node_info", []string{"policy", "get"}, "ok", func(ctx context.Context) (*mcp.CallToolResult, error) {
			r, _, err := handleDisplayPolicyNodeInformation(ctx, &mcp.CallToolRequest{}, displayPolicyNodeInformationInput{NodeName: "test-node"})
			return r, err
		}},
		{"display_policy_node_info_labels", []string{"policy", "get", "k=v"}, "ok", func(ctx context.Context) (*mcp.CallToolResult, error) {
			r, _, err := handleDisplayPolicyNodeInformation(ctx, &mcp.CallToolRequest{}, displayPolicyNodeInformationInput{Labels: "k=v", NodeName: "test-node"})
			return r, err
		}},
		{"delete_policy_rules_all", []string{"policy", "delete", "--all"}, "ok", func(ctx context.Context) (*mcp.CallToolResult, error) {
			r, _, err := handleDeletePolicyRules(ctx, &mcp.CallToolRequest{}, deletePolicyRulesInput{All: true, NodeName: "test-node"})
			return r, err
		}},
		{"delete_policy_rules_labels", []string{"policy", "delete", "k=v"}, "ok", func(ctx context.Context) (*mcp.CallToolResult, error) {
			r, _, err := handleDeletePolicyRules(ctx, &mcp.CallToolRequest{}, deletePolicyRulesInput{Labels: "k=v", NodeName: "test-node"})
			return r, err
		}},
		{"update_xdp_cidr", []string{"prefilter", "update", "--cidr", "10.0.0.0/8"}, "ok", func(ctx context.Context) (*mcp.CallToolResult, error) {
			r, _, err := handleUpdateXDPCIDRFilters(ctx, &mcp.CallToolRequest{}, xdpCIDRFiltersInput{CIDRPrefixes: "10.0.0.0/8", NodeName: "test-node"})
			return r, err
		}},
		{"update_xdp_cidr_rev", []string{"prefilter", "update", "--cidr", "10.0.0.0/8", "--revision", "2"}, "ok", func(ctx context.Context) (*mcp.CallToolResult, error) {
			r, _, err := handleUpdateXDPCIDRFilters(ctx, &mcp.CallToolRequest{}, xdpCIDRFiltersInput{CIDRPrefixes: "10.0.0.0/8", Revision: "2", NodeName: "test-node"})
			return r, err
		}},
		{"delete_xdp_cidr", []string{"prefilter", "delete", "--cidr", "10.0.0.0/8"}, "ok", func(ctx context.Context) (*mcp.CallToolResult, error) {
			r, _, err := handleDeleteXDPCIDRFilters(ctx, &mcp.CallToolRequest{}, xdpCIDRFiltersInput{CIDRPrefixes: "10.0.0.0/8", NodeName: "test-node"})
			return r, err
		}},
		{"delete_xdp_cidr_rev", []string{"prefilter", "delete", "--cidr", "10.0.0.0/8", "--revision", "2"}, "ok", func(ctx context.Context) (*mcp.CallToolResult, error) {
			r, _, err := handleDeleteXDPCIDRFilters(ctx, &mcp.CallToolRequest{}, xdpCIDRFiltersInput{CIDRPrefixes: "10.0.0.0/8", Revision: "2", NodeName: "test-node"})
			return r, err
		}},
		{"validate_cnp", []string{"preflight", "validate-cnp", "--enable-k8s", "--enable-k8s-api-discovery"}, "ok", func(ctx context.Context) (*mcp.CallToolResult, error) {
			r, _, err := handleValidateCiliumNetworkPolicies(ctx, &mcp.CallToolRequest{}, validateCiliumNetworkPoliciesInput{EnableK8s: true, EnableK8sAPIDiscovery: true, NodeName: "test-node"})
			return r, err
		}},
		{"list_pcap_recorders", []string{"recorder", "list"}, "ok", func(ctx context.Context) (*mcp.CallToolResult, error) {
			r, _, err := handleListPCAPRecorders(ctx, &mcp.CallToolRequest{}, nodeNameInput{NodeName: "test-node"})
			return r, err
		}},
		{"get_pcap_recorder", []string{"recorder", "get", "1"}, "ok", func(ctx context.Context) (*mcp.CallToolResult, error) {
			r, _, err := handleGetPCAPRecorder(ctx, &mcp.CallToolRequest{}, pcapRecorderIDInput{RecorderID: "1", NodeName: "test-node"})
			return r, err
		}},
		{"delete_pcap_recorder", []string{"recorder", "delete", "1"}, "ok", func(ctx context.Context) (*mcp.CallToolResult, error) {
			r, _, err := handleDeletePCAPRecorder(ctx, &mcp.CallToolRequest{}, pcapRecorderIDInput{RecorderID: "1", NodeName: "test-node"})
			return r, err
		}},
		{"update_pcap_recorder", []string{"recorder", "update", "1", "--filters", "f", "--caplen", "0", "--id", "0"}, "ok", func(ctx context.Context) (*mcp.CallToolResult, error) {
			r, _, err := handleUpdatePCAPRecorder(ctx, &mcp.CallToolRequest{}, updatePCAPRecorderInput{RecorderID: "1", Filters: "f", NodeName: "test-node"})
			return r, err
		}},
		{"get_service_information", []string{"service", "get", "5"}, "ok", func(ctx context.Context) (*mcp.CallToolResult, error) {
			r, _, err := handleGetServiceInformation(ctx, &mcp.CallToolRequest{}, getServiceInformationInput{ServiceID: "5", NodeName: "test-node"})
			return r, err
		}},
		{"delete_service_all", []string{"service", "delete", "--all"}, "ok", func(ctx context.Context) (*mcp.CallToolResult, error) {
			r, _, err := handleDeleteService(ctx, &mcp.CallToolRequest{}, deleteServiceInput{All: true, NodeName: "test-node"})
			return r, err
		}},
		{"delete_service_id", []string{"service", "delete", "5"}, "ok", func(ctx context.Context) (*mcp.CallToolResult, error) {
			r, _, err := handleDeleteService(ctx, &mcp.CallToolRequest{}, deleteServiceInput{ServiceID: "5", NodeName: "test-node"})
			return r, err
		}},
		{"update_service", []string{"service", "update", "1", "--backends", "b", "--frontend", "f", "--protocol", "TCP", "--states", "active"}, "ok", func(ctx context.Context) (*mcp.CallToolResult, error) {
			r, _, err := handleUpdateService(ctx, &mcp.CallToolRequest{}, updateServiceInput{Backends: "b", Frontend: "f", ID: "1", NodeName: "test-node"})
			return r, err
		}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mock := cmd.NewMockShellExecutor()
			mockCiliumDbgCommand(mock, tc.dbgArgs, tc.expect, nil)
			ctx := cmd.WithShellExecutor(context.Background(), mock)

			result, err := tc.run(ctx)
			require.NoError(t, err)
			assert.False(t, result.IsError, "handler returned error result: %s", getResultText(result))
			assert.Contains(t, getResultText(result), tc.expect)
		})
	}
}

// TestCiliumDbgHandlersMissingParams covers required-parameter validation branches.
func TestCiliumDbgHandlersMissingParams(t *testing.T) {
	cases := []struct {
		name string
		run  func(context.Context) (*mcp.CallToolResult, error)
	}{
		{"manage_endpoint_labels", func(ctx context.Context) (*mcp.CallToolResult, error) {
			r, _, err := handleManageEndpointLabels(ctx, &mcp.CallToolRequest{}, manageEndpointLabelsInput{})
			return r, err
		}},
		{"manage_endpoint_configuration_no_id", func(ctx context.Context) (*mcp.CallToolResult, error) {
			r, _, err := handleManageEndpointConfiguration(ctx, &mcp.CallToolRequest{}, manageEndpointConfigurationInput{})
			return r, err
		}},
		{"manage_endpoint_configuration_no_config", func(ctx context.Context) (*mcp.CallToolResult, error) {
			r, _, err := handleManageEndpointConfiguration(ctx, &mcp.CallToolRequest{}, manageEndpointConfigurationInput{EndpointID: "34"})
			return r, err
		}},
		{"disconnect_endpoint", func(ctx context.Context) (*mcp.CallToolResult, error) {
			r, _, err := handleDisconnectEndpoint(ctx, &mcp.CallToolRequest{}, disconnectEndpointInput{})
			return r, err
		}},
		{"get_identity_details", func(ctx context.Context) (*mcp.CallToolResult, error) {
			r, _, err := handleGetIdentityDetails(ctx, &mcp.CallToolRequest{}, getIdentityDetailsInput{})
			return r, err
		}},
		{"list_envoy_config", func(ctx context.Context) (*mcp.CallToolResult, error) {
			r, _, err := handleListEnvoyConfig(ctx, &mcp.CallToolRequest{}, listEnvoyConfigInput{})
			return r, err
		}},
		{"show_ipcache_none", func(ctx context.Context) (*mcp.CallToolResult, error) {
			r, _, err := handleShowIPCacheInformation(ctx, &mcp.CallToolRequest{}, showIPCacheInformationInput{})
			return r, err
		}},
		{"delete_kvstore_key", func(ctx context.Context) (*mcp.CallToolResult, error) {
			r, _, err := handleDeleteKeyFromKVStore(ctx, &mcp.CallToolRequest{}, kvStoreKeyInput{})
			return r, err
		}},
		{"get_kvstore_key", func(ctx context.Context) (*mcp.CallToolResult, error) {
			r, _, err := handleGetKVStoreKey(ctx, &mcp.CallToolRequest{}, kvStoreKeyInput{})
			return r, err
		}},
		{"set_kvstore_key", func(ctx context.Context) (*mcp.CallToolResult, error) {
			r, _, err := handleSetKVStoreKey(ctx, &mcp.CallToolRequest{}, setKVStoreKeyInput{Key: "foo"})
			return r, err
		}},
		{"delete_policy_rules_none", func(ctx context.Context) (*mcp.CallToolResult, error) {
			r, _, err := handleDeletePolicyRules(ctx, &mcp.CallToolRequest{}, deletePolicyRulesInput{})
			return r, err
		}},
		{"update_xdp_cidr", func(ctx context.Context) (*mcp.CallToolResult, error) {
			r, _, err := handleUpdateXDPCIDRFilters(ctx, &mcp.CallToolRequest{}, xdpCIDRFiltersInput{})
			return r, err
		}},
		{"delete_xdp_cidr", func(ctx context.Context) (*mcp.CallToolResult, error) {
			r, _, err := handleDeleteXDPCIDRFilters(ctx, &mcp.CallToolRequest{}, xdpCIDRFiltersInput{})
			return r, err
		}},
		{"get_pcap_recorder", func(ctx context.Context) (*mcp.CallToolResult, error) {
			r, _, err := handleGetPCAPRecorder(ctx, &mcp.CallToolRequest{}, pcapRecorderIDInput{})
			return r, err
		}},
		{"delete_pcap_recorder", func(ctx context.Context) (*mcp.CallToolResult, error) {
			r, _, err := handleDeletePCAPRecorder(ctx, &mcp.CallToolRequest{}, pcapRecorderIDInput{})
			return r, err
		}},
		{"update_pcap_recorder", func(ctx context.Context) (*mcp.CallToolResult, error) {
			r, _, err := handleUpdatePCAPRecorder(ctx, &mcp.CallToolRequest{}, updatePCAPRecorderInput{RecorderID: "1"})
			return r, err
		}},
		{"get_service_information", func(ctx context.Context) (*mcp.CallToolResult, error) {
			r, _, err := handleGetServiceInformation(ctx, &mcp.CallToolRequest{}, getServiceInformationInput{})
			return r, err
		}},
		{"delete_service_none", func(ctx context.Context) (*mcp.CallToolResult, error) {
			r, _, err := handleDeleteService(ctx, &mcp.CallToolRequest{}, deleteServiceInput{})
			return r, err
		}},
		{"update_service", func(ctx context.Context) (*mcp.CallToolResult, error) {
			r, _, err := handleUpdateService(ctx, &mcp.CallToolRequest{}, updateServiceInput{Backends: "b"})
			return r, err
		}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mock := cmd.NewMockShellExecutor()
			ctx := cmd.WithShellExecutor(context.Background(), mock)
			result, err := tc.run(ctx)
			require.NoError(t, err)
			assert.True(t, result.IsError)
			assert.Empty(t, mock.GetCallLog())
		})
	}
}

// TestCiliumCliHandlers covers the cilium-CLI based handlers.
func TestCiliumCliHandlers(t *testing.T) {
	cases := []struct {
		name    string
		cliArgs []string
		run     func(context.Context) (*mcp.CallToolResult, error)
	}{
		{"show_cluster_mesh_status", []string{"clustermesh", "status"}, func(ctx context.Context) (*mcp.CallToolResult, error) {
			r, _, err := handleShowClusterMeshStatus(ctx, &mcp.CallToolRequest{}, noInput{})
			return r, err
		}},
		{"show_features_status", []string{"features", "status"}, func(ctx context.Context) (*mcp.CallToolResult, error) {
			r, _, err := handleShowFeaturesStatus(ctx, &mcp.CallToolRequest{}, noInput{})
			return r, err
		}},
		{"toggle_cluster_mesh_enable", []string{"clustermesh", "enable"}, func(ctx context.Context) (*mcp.CallToolResult, error) {
			r, _, err := handleToggleClusterMesh(ctx, &mcp.CallToolRequest{}, enableToggleInput{Enable: boolPtr(true)})
			return r, err
		}},
		{"toggle_cluster_mesh_disable", []string{"clustermesh", "disable"}, func(ctx context.Context) (*mcp.CallToolResult, error) {
			r, _, err := handleToggleClusterMesh(ctx, &mcp.CallToolRequest{}, enableToggleInput{Enable: boolPtr(false)})
			return r, err
		}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mock := cmd.NewMockShellExecutor()
			mock.AddCommandString("cilium", tc.cliArgs, "cli-ok", nil)
			ctx := cmd.WithShellExecutor(context.Background(), mock)
			result, err := tc.run(ctx)
			require.NoError(t, err)
			assert.False(t, result.IsError)
			assert.Contains(t, getResultText(result), "cli-ok")
		})
	}
}

func TestCiliumCliHandlersError(t *testing.T) {
	mock := cmd.NewMockShellExecutor()
	mock.AddCommandString("cilium", []string{"clustermesh", "status"}, "", assert.AnError)
	ctx := cmd.WithShellExecutor(context.Background(), mock)
	result, _, err := handleShowClusterMeshStatus(ctx, &mcp.CallToolRequest{}, noInput{})
	require.NoError(t, err)
	assert.True(t, result.IsError)
	assert.Contains(t, getResultText(result), "Error getting cluster mesh status")
}

// TestCiliumDbgHandlerError covers the error path shared by dbg handlers.
func TestCiliumDbgHandlerError(t *testing.T) {
	mock := cmd.NewMockShellExecutor()
	mockCiliumDbgCommand(mock, []string{"loadinfo"}, "", assert.AnError)
	ctx := cmd.WithShellExecutor(context.Background(), mock)
	result, _, err := handleShowLoadInformation(ctx, &mcp.CallToolRequest{}, nodeNameInput{NodeName: "test-node"})
	require.NoError(t, err)
	assert.True(t, result.IsError)
}
