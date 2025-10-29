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

func TestRegisterTools(t *testing.T) {
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "v0.0.1"}, nil)
	require.NoError(t, RegisterTools(server))
}

func TestCiliumHandlers_Smoke(t *testing.T) {
	ctx := context.Background()

	// Helpers
	createReq := func(args map[string]interface{}) *mcp.CallToolRequest {
		argsJSON, _ := json.Marshal(args)
		return &mcp.CallToolRequest{Params: &mcp.CallToolParamsRaw{Arguments: argsJSON}}
	}
	// Mocks the cilium-dbg flow which requires two kubectl calls: get pod and then exec
	mockDbg := func(mock *cmd.MockShellExecutor, nodeName, podName, dbgCmd, output string) {
		mock.AddCommandString("kubectl", []string{
			"get", "pods", "-n", "kube-system",
			"--selector=k8s-app=cilium",
			fmt.Sprintf("--field-selector=spec.nodeName=%s", nodeName),
			"-o", "jsonpath={.items[0].metadata.name}",
		}, podName, nil)
		mock.AddCommandString("kubectl", []string{"exec", "-it", podName, "--", "cilium-dbg", dbgCmd}, output, nil)
	}

	// 1) Simple cilium CLI based handlers
	{
		mock := cmd.NewMockShellExecutor()
		mock.AddCommandString("cilium", []string{"clustermesh", "status"}, "cluster-mesh OK", nil)
		mock.AddCommandString("cilium", []string{"features", "status"}, "features OK", nil)
		ctx1 := cmd.WithShellExecutor(ctx, mock)

		res1, err := handleShowClusterMeshStatus(ctx1, createReq(map[string]interface{}{}))
		require.NoError(t, err)
		assert.False(t, res1.IsError)
		assert.Contains(t, getResultText(res1), "cluster-mesh OK")

		res2, err := handleShowFeaturesStatus(ctx1, createReq(map[string]interface{}{}))
		require.NoError(t, err)
		assert.False(t, res2.IsError)
		assert.Contains(t, getResultText(res2), "features OK")
	}

	// 2) Toggle cluster mesh (enable)
	{
		mock := cmd.NewMockShellExecutor()
		mock.AddCommandString("cilium", []string{"clustermesh", "enable"}, "enabled", nil)
		ctx1 := cmd.WithShellExecutor(ctx, mock)
		res, err := handleToggleClusterMesh(ctx1, createReq(map[string]interface{}{"enable": "true"}))
		require.NoError(t, err)
		assert.False(t, res.IsError)
		assert.Contains(t, getResultText(res), "enabled")
	}

	// 3) Debug flows with cilium-dbg: endpoints list
	{
		mock := cmd.NewMockShellExecutor()
		mockDbg(mock, "", "cilium-pod-0", "endpoint list", "endpoints listed")
		ctx1 := cmd.WithShellExecutor(ctx, mock)
		res, err := handleGetEndpointsList(ctx1, createReq(map[string]interface{}{}))
		require.NoError(t, err)
		assert.False(t, res.IsError)
		assert.Contains(t, getResultText(res), "endpoints listed")
	}

	// 4) Endpoint details via labels
	{
		mock := cmd.NewMockShellExecutor()
		mockDbg(mock, "", "cilium-pod-0", "endpoint get -l app=web -o json", "details json")
		ctx1 := cmd.WithShellExecutor(ctx, mock)
		res, err := handleGetEndpointDetails(ctx1, createReq(map[string]interface{}{"labels": "app=web", "output_format": "json"}))
		require.NoError(t, err)
		assert.False(t, res.IsError)
		assert.Contains(t, getResultText(res), "details json")
	}

	// 5) Daemon status with flags
	{
		mock := cmd.NewMockShellExecutor()
		// constructed command should include these flags in any order concatenated to status
		mockDbg(mock, "", "cilium-pod-0", "status --all-addresses --health --brief", "daemon ok")
		ctx1 := cmd.WithShellExecutor(ctx, mock)
		args := map[string]interface{}{
			"show_all_addresses": "true",
			"show_health":        "true",
			"brief":              "true",
		}
		res, err := handleGetDaemonStatus(ctx1, createReq(args))
		require.NoError(t, err)
		assert.False(t, res.IsError)
		assert.Contains(t, getResultText(res), "daemon ok")
	}

	// 6) FQDN cache list and metrics with pattern
	{
		mock := cmd.NewMockShellExecutor()
		mockDbg(mock, "", "cilium-pod-0", "fqdn cache list", "fqdn ok")
		mockDbg(mock, "", "cilium-pod-0", "metrics list --pattern cilium_*", "metrics ok")
		ctx1 := cmd.WithShellExecutor(ctx, mock)
		res1, err := handleFQDNCache(ctx1, createReq(map[string]interface{}{"command": "list"}))
		require.NoError(t, err)
		assert.False(t, res1.IsError)
		res2, err := handleListMetrics(ctx1, createReq(map[string]interface{}{"match_pattern": "cilium_*"}))
		require.NoError(t, err)
		assert.False(t, res2.IsError)
	}

	// 7) Simple debug commands: list maps, list nodes, ip list
	{
		mock := cmd.NewMockShellExecutor()
		mockDbg(mock, "", "cilium-pod-0", "bpf map list", "maps")
		mockDbg(mock, "", "cilium-pod-0", "nodes list", "nodes")
		mockDbg(mock, "", "cilium-pod-0", "ip list", "ips")
		ctx1 := cmd.WithShellExecutor(ctx, mock)
		_, err := handleListBPFMaps(ctx1, createReq(map[string]interface{}{}))
		require.NoError(t, err)
		_, err = handleListClusterNodes(ctx1, createReq(map[string]interface{}{}))
		require.NoError(t, err)
		_, err = handleListIPAddresses(ctx1, createReq(map[string]interface{}{}))
		require.NoError(t, err)
	}

	// 8) KV store get/set/delete
	{
		mock := cmd.NewMockShellExecutor()
		mockDbg(mock, "", "cilium-pod-0", "kvstore get key1", "v1")
		mockDbg(mock, "", "cilium-pod-0", "kvstore set key2=val2", "ok")
		mockDbg(mock, "", "cilium-pod-0", "kvstore delete key3", "deleted")
		ctx1 := cmd.WithShellExecutor(ctx, mock)
		_, err := handleGetKVStoreKey(ctx1, createReq(map[string]interface{}{"key": "key1"}))
		require.NoError(t, err)
		_, err = handleSetKVStoreKey(ctx1, createReq(map[string]interface{}{"key": "key2", "value": "val2"}))
		require.NoError(t, err)
		_, err = handleDeleteKeyFromKVStore(ctx1, createReq(map[string]interface{}{"key": "key3"}))
		require.NoError(t, err)
	}
}

func TestCiliumHandlers_Extended(t *testing.T) {
	ctx := context.Background()
	createReq := func(args map[string]interface{}) *mcp.CallToolRequest {
		argsJSON, _ := json.Marshal(args)
		return &mcp.CallToolRequest{Params: &mcp.CallToolParamsRaw{Arguments: argsJSON}}
	}
	mockDbg := func(mock *cmd.MockShellExecutor, nodeName, podName, dbgCmd, output string) {
		mock.AddCommandString("kubectl", []string{"get", "pods", "-n", "kube-system", "--selector=k8s-app=cilium", fmt.Sprintf("--field-selector=spec.nodeName=%s", nodeName), "-o", "jsonpath={.items[0].metadata.name}"}, podName, nil)
		mock.AddCommandString("kubectl", []string{"exec", "-it", podName, "--", "cilium-dbg", dbgCmd}, output, nil)
	}

	// Show configuration options (all)
	{
		mock := cmd.NewMockShellExecutor()
		mockDbg(mock, "", "cilium-pod-0", "endpoint config --all", "opts")
		ctx1 := cmd.WithShellExecutor(ctx, mock)
		_, err := handleShowConfigurationOptions(ctx1, createReq(map[string]interface{}{"list_all": "true"}))
		require.NoError(t, err)
	}

	// Toggle configuration option
	{
		mock := cmd.NewMockShellExecutor()
		mockDbg(mock, "", "cilium-pod-0", "endpoint config AllowICMP=enable", "ok")
		ctx1 := cmd.WithShellExecutor(ctx, mock)
		_, err := handleToggleConfigurationOption(ctx1, createReq(map[string]interface{}{"option": "AllowICMP", "value": "true"}))
		require.NoError(t, err)
	}

	// Services list, get, update, delete
	{
		mock := cmd.NewMockShellExecutor()
		mockDbg(mock, "", "cilium-pod-0", "service list --clustermesh-affinity", "list")
		mockDbg(mock, "", "cilium-pod-0", "service get 42", "get")
		mockDbg(mock, "", "cilium-pod-0", "service update --id 1 --frontend 1.1.1.1:80 --backends 2.2.2.2:80 --protocol tcp", "upd")
		mockDbg(mock, "", "cilium-pod-0", "service delete --all", "delall")
		mockDbg(mock, "", "cilium-pod-0", "service delete 9", "delone")
		ctx1 := cmd.WithShellExecutor(ctx, mock)
		_, err := handleListServices(ctx1, createReq(map[string]interface{}{"show_cluster_mesh_affinity": "true"}))
		require.NoError(t, err)
		_, err = handleGetServiceInformation(ctx1, createReq(map[string]interface{}{"service_id": "42"}))
		require.NoError(t, err)
		_, err = handleUpdateService(ctx1, createReq(map[string]interface{}{"id": "1", "frontend": "1.1.1.1:80", "backends": "2.2.2.2:80", "protocol": "tcp"}))
		require.NoError(t, err)
		_, err = handleDeleteService(ctx1, createReq(map[string]interface{}{"all": "true"}))
		require.NoError(t, err)
		_, err = handleDeleteService(ctx1, createReq(map[string]interface{}{"service_id": "9"}))
		require.NoError(t, err)
	}

	// Endpoint logs and health, labels, config, disconnect
	{
		mock := cmd.NewMockShellExecutor()
		mockDbg(mock, "", "cilium-pod-0", "endpoint logs 123", "logs")
		mockDbg(mock, "", "cilium-pod-0", "endpoint health 123", "health")
		mockDbg(mock, "", "cilium-pod-0", "endpoint labels 123 --add k=v", "labels")
		mockDbg(mock, "", "cilium-pod-0", "endpoint config 123 DropNotification=false", "cfg")
		mockDbg(mock, "", "cilium-pod-0", "endpoint disconnect 123", "disc")
		ctx1 := cmd.WithShellExecutor(ctx, mock)
		_, err := handleGetEndpointLogs(ctx1, createReq(map[string]interface{}{"endpoint_id": "123"}))
		require.NoError(t, err)
		_, err = handleGetEndpointHealth(ctx1, createReq(map[string]interface{}{"endpoint_id": "123"}))
		require.NoError(t, err)
		_, err = handleManageEndpointLabels(ctx1, createReq(map[string]interface{}{"endpoint_id": "123", "labels": "k=v", "action": "add"}))
		require.NoError(t, err)
		_, err = handleManageEndpointConfig(ctx1, createReq(map[string]interface{}{"endpoint_id": "123", "config": "DropNotification=false"}))
		require.NoError(t, err)
		_, err = handleDisconnectEndpoint(ctx1, createReq(map[string]interface{}{"endpoint_id": "123"}))
		require.NoError(t, err)
	}

	// Identities
	{
		mock := cmd.NewMockShellExecutor()
		mockDbg(mock, "", "cilium-pod-0", "identity list", "ids")
		mockDbg(mock, "", "cilium-pod-0", "identity get 7", "id7")
		ctx1 := cmd.WithShellExecutor(ctx, mock)
		_, err := handleListIdentities(ctx1, createReq(map[string]interface{}{}))
		require.NoError(t, err)
		_, err = handleGetIdentityDetails(ctx1, createReq(map[string]interface{}{"identity_id": "7"}))
		require.NoError(t, err)
	}

	// Misc debug/info
	{
		mock := cmd.NewMockShellExecutor()
		mockDbg(mock, "", "cilium-pod-0", "debuginfo", "dbg")
		mockDbg(mock, "", "cilium-pod-0", "encrypt status", "enc")
		mockDbg(mock, "", "cilium-pod-0", "encrypt flush -f", "flushed")
		mockDbg(mock, "", "cilium-pod-0", "envoy admin clusters", "clusters")
		mockDbg(mock, "", "cilium-pod-0", "dns names", "dns")
		mockDbg(mock, "", "cilium-pod-0", "ip get --labels app=web", "ipcache")
		mockDbg(mock, "", "cilium-pod-0", "loadinfo", "load")
		mockDbg(mock, "", "cilium-pod-0", "lrp list", "lrp")
		mockDbg(mock, "", "cilium-pod-0", "bpf map events tc/globals/cilium_calls", "events")
		mockDbg(mock, "", "cilium-pod-0", "bpf map get tc/globals/cilium_calls", "getmap")
		mockDbg(mock, "", "cilium-pod-0", "nodeid list", "nodeids")
		mockDbg(mock, "", "cilium-pod-0", "policy get k8s:app=web", "polget")
		mockDbg(mock, "", "cilium-pod-0", "policy delete --all", "poldel")
		mockDbg(mock, "", "cilium-pod-0", "policy selectors", "selectors")
		mockDbg(mock, "", "cilium-pod-0", "prefilter update 10.0.0.0/24 --revision 2", "preupd")
		mockDbg(mock, "", "cilium-pod-0", "prefilter delete 10.0.0.0/24 --revision 2", "predel")
		mockDbg(mock, "", "cilium-pod-0", "policy validate --enable-k8s --enable-k8s-api-discovery", "valid")
		ctx1 := cmd.WithShellExecutor(ctx, mock)
		_, err := handleRequestDebuggingInformation(ctx1, createReq(map[string]interface{}{}))
		require.NoError(t, err)
		_, err = handleDisplayEncryptionState(ctx1, createReq(map[string]interface{}{}))
		require.NoError(t, err)
		_, err = handleFlushIPsecState(ctx1, createReq(map[string]interface{}{}))
		require.NoError(t, err)
		_, err = handleListEnvoyConfig(ctx1, createReq(map[string]interface{}{"resource_name": "clusters"}))
		require.NoError(t, err)
		_, err = handleShowDNSNames(ctx1, createReq(map[string]interface{}{}))
		require.NoError(t, err)
		_, err = handleShowIPCacheInformation(ctx1, createReq(map[string]interface{}{"labels": "app=web"}))
		require.NoError(t, err)
		_, err = handleShowLoadInformation(ctx1, createReq(map[string]interface{}{}))
		require.NoError(t, err)
		_, err = handleListLocalRedirectPolicies(ctx1, createReq(map[string]interface{}{}))
		require.NoError(t, err)
		_, err = handleListBPFMapEvents(ctx1, createReq(map[string]interface{}{"map_name": "tc/globals/cilium_calls"}))
		require.NoError(t, err)
		_, err = handleGetBPFMap(ctx1, createReq(map[string]interface{}{"map_name": "tc/globals/cilium_calls"}))
		require.NoError(t, err)
		_, err = handleListNodeIds(ctx1, createReq(map[string]interface{}{}))
		require.NoError(t, err)
		_, err = handleDisplayPolicyNodeInformation(ctx1, createReq(map[string]interface{}{"labels": "k8s:app=web"}))
		require.NoError(t, err)
		_, err = handleDeletePolicyRules(ctx1, createReq(map[string]interface{}{"all": "true"}))
		require.NoError(t, err)
		_, err = handleDisplaySelectors(ctx1, createReq(map[string]interface{}{}))
		require.NoError(t, err)
		_, err = handleUpdateXDPCIDRFilters(ctx1, createReq(map[string]interface{}{"cidr_prefixes": "10.0.0.0/24", "revision": "2"}))
		require.NoError(t, err)
		_, err = handleDeleteXDPCIDRFilters(ctx1, createReq(map[string]interface{}{"cidr_prefixes": "10.0.0.0/24", "revision": "2"}))
		require.NoError(t, err)
		_, err = handleValidateCiliumNetworkPolicies(ctx1, createReq(map[string]interface{}{"enable_k8s": "true", "enable_k8s_api_discovery": "true"}))
		require.NoError(t, err)
	}

	// PCAP recorders
	{
		mock := cmd.NewMockShellExecutor()
		mockDbg(mock, "", "cilium-pod-0", "recorder list", "list")
		mockDbg(mock, "", "cilium-pod-0", "recorder get r1", "get")
		mockDbg(mock, "", "cilium-pod-0", "recorder delete r1", "del")
		mockDbg(mock, "", "cilium-pod-0", "recorder update r1 --filters port:80 --caplen 64 --id recA", "upd")
		ctx1 := cmd.WithShellExecutor(ctx, mock)
		_, err := handleListPCAPRecorders(ctx1, createReq(map[string]interface{}{}))
		require.NoError(t, err)
		_, err = handleGetPCAPRecorder(ctx1, createReq(map[string]interface{}{"recorder_id": "r1"}))
		require.NoError(t, err)
		_, err = handleDeletePCAPRecorder(ctx1, createReq(map[string]interface{}{"recorder_id": "r1"}))
		require.NoError(t, err)
		_, err = handleUpdatePCAPRecorder(ctx1, createReq(map[string]interface{}{"recorder_id": "r1", "filters": "port:80", "caplen": "64", "id": "recA"}))
		require.NoError(t, err)
	}
}
