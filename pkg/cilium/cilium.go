package cilium

import (
	"context"
	"fmt"
	"strings"

	"github.com/kagent-dev/tools/internal/commands"
	mcp "github.com/kagent-dev/tools/internal/mcp"
	"github.com/kagent-dev/tools/pkg/utils"
)

type noInput struct{}

type nodeNameInput struct {
	NodeName string `json:"node_name" jsonschema:"The name of the node to run the command on"`
}

type upgradeCiliumInput struct {
	ClusterName  string `json:"cluster_name" jsonschema:"The name of the cluster to upgrade Cilium on"`
	DatapathMode string `json:"datapath_mode" jsonschema:"The datapath mode to use for Cilium (tunnel, native, aws-eni, gke, azure, aks-byocni)"`
}

type installCiliumInput struct {
	ClusterName  string `json:"cluster_name" jsonschema:"The name of the cluster to install Cilium on"`
	ClusterID    string `json:"cluster_id" jsonschema:"The ID of the cluster to install Cilium on"`
	DatapathMode string `json:"datapath_mode" jsonschema:"The datapath mode to use for Cilium (tunnel, native, aws-eni, gke, azure, aks-byocni)"`
}

type connectToRemoteClusterInput struct {
	ClusterName string `json:"cluster_name" jsonschema:"The name of the destination cluster"`
	Context     string `json:"context" jsonschema:"The kubectl context for the destination cluster"`
}

type disconnectRemoteClusterInput struct {
	ClusterName string `json:"cluster_name" jsonschema:"The name of the destination cluster"`
}

type enableToggleInput struct {
	Enable *bool `json:"enable" jsonschema:"Set to true to enable, false to disable"`
}

type getDaemonStatusInput struct {
	ShowAllAddresses   bool   `json:"show_all_addresses" jsonschema:"Whether to show all addresses"`
	ShowAllClusters    bool   `json:"show_all_clusters" jsonschema:"Whether to show all clusters"`
	ShowAllControllers bool   `json:"show_all_controllers" jsonschema:"Whether to show all controllers"`
	ShowHealth         bool   `json:"show_health" jsonschema:"Whether to show health"`
	ShowAllNodes       bool   `json:"show_all_nodes" jsonschema:"Whether to show all nodes"`
	ShowAllRedirects   bool   `json:"show_all_redirects" jsonschema:"Whether to show all redirects"`
	Brief              bool   `json:"brief" jsonschema:"Whether to show a brief status"`
	NodeName           string `json:"node_name" jsonschema:"The name of the node to get the daemon status for"`
}

type getEndpointDetailsInput struct {
	EndpointID   string `json:"endpoint_id" jsonschema:"The ID of the endpoint to get details for"`
	Labels       string `json:"labels" jsonschema:"The labels of the endpoint to get details for"`
	OutputFormat string `json:"output_format" jsonschema:"The output format of the endpoint details (json, yaml, jsonpath)"`
	NodeName     string `json:"node_name" jsonschema:"The name of the node to get the endpoint details for"`
}

type getEndpointLogsInput struct {
	EndpointID string `json:"endpoint_id" jsonschema:"The ID of the endpoint to get logs for"`
	NodeName   string `json:"node_name" jsonschema:"The name of the node to get the endpoint logs for"`
}

type getEndpointHealthInput struct {
	EndpointID string `json:"endpoint_id" jsonschema:"The ID of the endpoint to get health for"`
	NodeName   string `json:"node_name" jsonschema:"The name of the node to get the endpoint health for"`
}

type manageEndpointLabelsInput struct {
	EndpointID string `json:"endpoint_id" jsonschema:"The ID of the endpoint to manage labels for"`
	Labels     string `json:"labels" jsonschema:"Space-separated labels to manage (e.g., 'key1=value1 key2=value2')"`
	Action     string `json:"action" jsonschema:"The action to perform on the labels (add or delete)"`
	NodeName   string `json:"node_name" jsonschema:"The name of the node to manage the endpoint labels on"`
}

type manageEndpointConfigurationInput struct {
	EndpointID string `json:"endpoint_id" jsonschema:"The ID of the endpoint to manage configuration for"`
	Config     string `json:"config" jsonschema:"The configuration to manage for the endpoint provided as a space-separated list of key-value pairs (e.g. 'DropNotification=false TraceNotification=false')"`
	NodeName   string `json:"node_name" jsonschema:"The name of the node to manage the endpoint configuration on"`
}

type disconnectEndpointInput struct {
	EndpointID string `json:"endpoint_id" jsonschema:"The ID of the endpoint to disconnect"`
	NodeName   string `json:"node_name" jsonschema:"The name of the node to disconnect the endpoint from"`
}

type showConfigurationOptionsInput struct {
	ListAll      bool   `json:"list_all" jsonschema:"Whether to list all configuration options"`
	ListReadOnly bool   `json:"list_read_only" jsonschema:"Whether to list read-only configuration options"`
	ListOptions  bool   `json:"list_options" jsonschema:"Whether to list options"`
	NodeName     string `json:"node_name" jsonschema:"The name of the node to show the configuration options for"`
}

type toggleConfigurationOptionInput struct {
	Option   string `json:"option" jsonschema:"The option to toggle"`
	Value    *bool  `json:"value" jsonschema:"The value to set the option to (true/false)"`
	NodeName string `json:"node_name" jsonschema:"The name of the node to toggle the configuration option for"`
}

type getIdentityDetailsInput struct {
	IdentityID string `json:"identity_id" jsonschema:"The ID of the identity to get details for"`
	NodeName   string `json:"node_name" jsonschema:"The name of the node to get the identity details for"`
}

type listEnvoyConfigInput struct {
	ResourceName string `json:"resource_name" jsonschema:"The name of the resource to get the Envoy configuration for"`
	NodeName     string `json:"node_name" jsonschema:"The name of the node to get the Envoy configuration for"`
}

type fqdnCacheInput struct {
	Command  string `json:"command" jsonschema:"The command to perform on the FQDN cache (list, clean, or a specific command)"`
	NodeName string `json:"node_name" jsonschema:"The name of the node to manage the FQDN cache for"`
}

type showIPCacheInformationInput struct {
	CIDR     string `json:"cidr" jsonschema:"The CIDR of the IP to get cache information for"`
	Labels   string `json:"labels" jsonschema:"The labels of the IP to get cache information for"`
	NodeName string `json:"node_name" jsonschema:"The name of the node to get the IP cache information for"`
}

type kvStoreKeyInput struct {
	Key      string `json:"key" jsonschema:"The key in the kvstore"`
	NodeName string `json:"node_name" jsonschema:"The name of the node to run the kvstore command on"`
}

type setKVStoreKeyInput struct {
	Key      string `json:"key" jsonschema:"The key to set in the kvstore"`
	Value    string `json:"value" jsonschema:"The value to set in the kvstore"`
	NodeName string `json:"node_name" jsonschema:"The name of the node to set the key in"`
}

type bpfMapInput struct {
	MapName  string `json:"map_name" jsonschema:"The name of the BPF map"`
	NodeName string `json:"node_name" jsonschema:"The name of the node to run the BPF map command on"`
}

type listMetricsInput struct {
	MatchPattern string `json:"match_pattern" jsonschema:"The match pattern to filter metrics by"`
	NodeName     string `json:"node_name" jsonschema:"The name of the node to get the metrics for"`
}

type displayPolicyNodeInformationInput struct {
	Labels   string `json:"labels" jsonschema:"The labels to get policy node information for"`
	NodeName string `json:"node_name" jsonschema:"The name of the node to get policy node information for"`
}

type deletePolicyRulesInput struct {
	Labels   string `json:"labels" jsonschema:"The labels to delete policy rules for"`
	All      bool   `json:"all" jsonschema:"Whether to delete all policy rules"`
	NodeName string `json:"node_name" jsonschema:"The name of the node to delete policy rules for"`
}

type xdpCIDRFiltersInput struct {
	CIDRPrefixes string `json:"cidr_prefixes" jsonschema:"The CIDR prefixes for the XDP filters"`
	Revision     string `json:"revision" jsonschema:"The revision of the XDP filters"`
	NodeName     string `json:"node_name" jsonschema:"The name of the node to run the XDP filter command on"`
}

type validateCiliumNetworkPoliciesInput struct {
	EnableK8s             bool   `json:"enable_k8s" jsonschema:"Whether to enable k8s API discovery"`
	EnableK8sAPIDiscovery bool   `json:"enable_k8s_api_discovery" jsonschema:"Whether to enable k8s API discovery"`
	NodeName              string `json:"node_name" jsonschema:"The name of the node to validate the Cilium network policies for"`
}

type pcapRecorderIDInput struct {
	RecorderID string `json:"recorder_id" jsonschema:"The ID of the PCAP recorder"`
	NodeName   string `json:"node_name" jsonschema:"The name of the node to run the PCAP recorder command on"`
}

type updatePCAPRecorderInput struct {
	RecorderID string `json:"recorder_id" jsonschema:"The ID of the PCAP recorder to update"`
	Filters    string `json:"filters" jsonschema:"The filters to update the PCAP recorder with"`
	Caplen     string `json:"caplen" jsonschema:"The caplen to update the PCAP recorder with"`
	ID         string `json:"id" jsonschema:"The id to update the PCAP recorder with"`
	NodeName   string `json:"node_name" jsonschema:"The name of the node to update the PCAP recorder on"`
}

type listServicesInput struct {
	ShowClusterMeshAffinity bool   `json:"show_cluster_mesh_affinity" jsonschema:"Whether to show cluster mesh affinity"`
	NodeName                string `json:"node_name" jsonschema:"The name of the node to get the services for"`
}

type getServiceInformationInput struct {
	ServiceID string `json:"service_id" jsonschema:"The ID of the service to get information about"`
	NodeName  string `json:"node_name" jsonschema:"The name of the node to get the service information for"`
}

type deleteServiceInput struct {
	ServiceID string `json:"service_id" jsonschema:"The ID of the service to delete"`
	All       bool   `json:"all" jsonschema:"Whether to delete all services"`
	NodeName  string `json:"node_name" jsonschema:"The name of the node to delete the service from"`
}

type updateServiceInput struct {
	BackendWeights      string `json:"backend_weights" jsonschema:"The backend weights to update the service with"`
	Backends            string `json:"backends" jsonschema:"The backends to update the service with"`
	Frontend            string `json:"frontend" jsonschema:"The frontend to update the service with"`
	ID                  string `json:"id" jsonschema:"The ID of the service to update"`
	K8sClusterInternal  bool   `json:"k8s_cluster_internal" jsonschema:"Whether to update the k8s cluster internal flag"`
	K8sExtTrafficPolicy string `json:"k8s_ext_traffic_policy" jsonschema:"The k8s ext traffic policy to update the service with"`
	K8sExternal         bool   `json:"k8s_external" jsonschema:"Whether to update the k8s external flag"`
	K8sHostPort         bool   `json:"k8s_host_port" jsonschema:"Whether to update the k8s host port flag"`
	K8sIntTrafficPolicy string `json:"k8s_int_traffic_policy" jsonschema:"The k8s int traffic policy to update the service with"`
	K8sLoadBalancer     bool   `json:"k8s_load_balancer" jsonschema:"Whether to update the k8s load balancer flag"`
	K8sNodePort         bool   `json:"k8s_node_port" jsonschema:"Whether to update the k8s node port flag"`
	LocalRedirect       bool   `json:"local_redirect" jsonschema:"Whether to update the local redirect flag"`
	Protocol            string `json:"protocol" jsonschema:"The protocol to update the service with"`
	States              string `json:"states" jsonschema:"The states to update the service with"`
	NodeName            string `json:"node_name" jsonschema:"The name of the node to update the service on"`
}

func runCiliumCliWithContext(ctx context.Context, args ...string) (string, error) {
	kubeconfigPath := utils.GetKubeconfig()
	return commands.NewCommandBuilder("cilium").
		WithArgs(args...).
		WithKubeconfig(kubeconfigPath).
		Execute(ctx)
}

func handleCiliumStatusAndVersion(ctx context.Context, request *mcp.CallToolRequest, in noInput) (*mcp.CallToolResult, any, error) {
	status, err := runCiliumCliWithContext(ctx, "status")
	if err != nil {
		return mcp.NewToolResultError("Error getting Cilium status: " + err.Error()), nil, nil
	}

	version, err := runCiliumCliWithContext(ctx, "version")
	if err != nil {
		return mcp.NewToolResultError("Error getting Cilium version: " + err.Error()), nil, nil
	}

	result := status + "\n" + version
	return mcp.NewToolResultText(result), nil, nil
}

func handleUpgradeCilium(ctx context.Context, request *mcp.CallToolRequest, in upgradeCiliumInput) (*mcp.CallToolResult, any, error) {
	clusterName := in.ClusterName
	datapathMode := in.DatapathMode

	args := []string{"upgrade"}
	if clusterName != "" {
		args = append(args, "--cluster-name", clusterName)
	}
	if datapathMode != "" {
		args = append(args, "--datapath-mode", datapathMode)
	}

	output, err := runCiliumCliWithContext(ctx, args...)
	if err != nil {
		return mcp.NewToolResultError("Error upgrading Cilium: " + err.Error()), nil, nil
	}

	return mcp.NewToolResultText(output), nil, nil
}

func handleInstallCilium(ctx context.Context, request *mcp.CallToolRequest, in installCiliumInput) (*mcp.CallToolResult, any, error) {
	clusterName := in.ClusterName
	clusterID := in.ClusterID
	datapathMode := in.DatapathMode

	args := []string{"install"}
	if clusterName != "" {
		args = append(args, "--set", "cluster.name="+clusterName)
	}
	if clusterID != "" {
		args = append(args, "--set", "cluster.id="+clusterID)
	}
	if datapathMode != "" {
		args = append(args, "--datapath-mode", datapathMode)
	}

	output, err := runCiliumCliWithContext(ctx, args...)
	if err != nil {
		return mcp.NewToolResultError("Error installing Cilium: " + err.Error()), nil, nil
	}

	return mcp.NewToolResultText(output), nil, nil
}

func handleUninstallCilium(ctx context.Context, request *mcp.CallToolRequest, in noInput) (*mcp.CallToolResult, any, error) {
	output, err := runCiliumCliWithContext(ctx, "uninstall")
	if err != nil {
		return mcp.NewToolResultError("Error uninstalling Cilium: " + err.Error()), nil, nil
	}

	return mcp.NewToolResultText(output), nil, nil
}

func handleConnectToRemoteCluster(ctx context.Context, request *mcp.CallToolRequest, in connectToRemoteClusterInput) (*mcp.CallToolResult, any, error) {
	clusterName := in.ClusterName
	destContext := in.Context

	if clusterName == "" {
		return mcp.NewToolResultError("cluster_name parameter is required"), nil, nil
	}

	args := []string{"clustermesh", "connect", "--destination-cluster", clusterName}
	if destContext != "" {
		args = append(args, "--destination-context", destContext)
	}

	output, err := runCiliumCliWithContext(ctx, args...)
	if err != nil {
		return mcp.NewToolResultError("Error connecting to remote cluster: " + err.Error()), nil, nil
	}

	return mcp.NewToolResultText(output), nil, nil
}

func handleDisconnectRemoteCluster(ctx context.Context, request *mcp.CallToolRequest, in disconnectRemoteClusterInput) (*mcp.CallToolResult, any, error) {
	clusterName := in.ClusterName

	if clusterName == "" {
		return mcp.NewToolResultError("cluster_name parameter is required"), nil, nil
	}

	args := []string{"clustermesh", "disconnect", "--destination-cluster", clusterName}

	output, err := runCiliumCliWithContext(ctx, args...)
	if err != nil {
		return mcp.NewToolResultError("Error disconnecting from remote cluster: " + err.Error()), nil, nil
	}

	return mcp.NewToolResultText(output), nil, nil
}

func handleListBGPPeers(ctx context.Context, request *mcp.CallToolRequest, in noInput) (*mcp.CallToolResult, any, error) {
	output, err := runCiliumCliWithContext(ctx, "bgp", "peers")
	if err != nil {
		return mcp.NewToolResultError("Error listing BGP peers: " + err.Error()), nil, nil
	}

	return mcp.NewToolResultText(output), nil, nil
}

func handleListBGPRoutes(ctx context.Context, request *mcp.CallToolRequest, in noInput) (*mcp.CallToolResult, any, error) {
	output, err := runCiliumCliWithContext(ctx, "bgp", "routes")
	if err != nil {
		return mcp.NewToolResultError("Error listing BGP routes: " + err.Error()), nil, nil
	}

	return mcp.NewToolResultText(output), nil, nil
}

func handleShowClusterMeshStatus(ctx context.Context, request *mcp.CallToolRequest, in noInput) (*mcp.CallToolResult, any, error) {
	output, err := runCiliumCliWithContext(ctx, "clustermesh", "status")
	if err != nil {
		return mcp.NewToolResultError("Error getting cluster mesh status: " + err.Error()), nil, nil
	}

	return mcp.NewToolResultText(output), nil, nil
}

func handleShowFeaturesStatus(ctx context.Context, request *mcp.CallToolRequest, in noInput) (*mcp.CallToolResult, any, error) {
	output, err := runCiliumCliWithContext(ctx, "features", "status")
	if err != nil {
		return mcp.NewToolResultError("Error getting features status: " + err.Error()), nil, nil
	}

	return mcp.NewToolResultText(output), nil, nil
}

func handleToggleHubble(ctx context.Context, request *mcp.CallToolRequest, in enableToggleInput) (*mcp.CallToolResult, any, error) {
	enable := true
	if in.Enable != nil {
		enable = *in.Enable
	}
	var action string
	if enable {
		action = "enable"
	} else {
		action = "disable"
	}

	output, err := runCiliumCliWithContext(ctx, "hubble", action)
	if err != nil {
		return mcp.NewToolResultError("Error toggling Hubble: " + err.Error()), nil, nil
	}

	return mcp.NewToolResultText(output), nil, nil
}

func handleToggleClusterMesh(ctx context.Context, request *mcp.CallToolRequest, in enableToggleInput) (*mcp.CallToolResult, any, error) {
	enable := true
	if in.Enable != nil {
		enable = *in.Enable
	}
	var action string
	if enable {
		action = "enable"
	} else {
		action = "disable"
	}

	output, err := runCiliumCliWithContext(ctx, "clustermesh", action)
	if err != nil {
		return mcp.NewToolResultError("Error toggling cluster mesh: " + err.Error()), nil, nil
	}

	return mcp.NewToolResultText(output), nil, nil
}

func RegisterTools(s *mcp.Server, readOnly bool) {
	// Read-only tools - always registered
	mcp.AddTool(s, "cilium", &mcp.Tool{Name: "cilium_status_and_version", Description: "Get the status and version of Cilium installation"}, handleCiliumStatusAndVersion)
	mcp.AddTool(s, "cilium", &mcp.Tool{Name: "cilium_list_bgp_peers", Description: "List BGP peers"}, handleListBGPPeers)
	mcp.AddTool(s, "cilium", &mcp.Tool{Name: "cilium_list_bgp_routes", Description: "List BGP routes"}, handleListBGPRoutes)
	mcp.AddTool(s, "cilium", &mcp.Tool{Name: "cilium_show_cluster_mesh_status", Description: "Show cluster mesh status"}, handleShowClusterMeshStatus)
	mcp.AddTool(s, "cilium", &mcp.Tool{Name: "cilium_show_features_status", Description: "Show Cilium features status"}, handleShowFeaturesStatus)

	if !readOnly {
		mcp.AddTool(s, "cilium", &mcp.Tool{Name: "cilium_upgrade_cilium", Description: "Upgrade Cilium on the cluster"}, handleUpgradeCilium)
		mcp.AddTool(s, "cilium", &mcp.Tool{Name: "cilium_install_cilium", Description: "Install Cilium on the cluster"}, handleInstallCilium)
		mcp.AddTool(s, "cilium", &mcp.Tool{Name: "cilium_uninstall_cilium", Description: "Uninstall Cilium from the cluster"}, handleUninstallCilium)
		mcp.AddTool(s, "cilium", &mcp.Tool{Name: "cilium_connect_to_remote_cluster", Description: "Connect to a remote cluster for cluster mesh"}, handleConnectToRemoteCluster)
		mcp.AddTool(s, "cilium", &mcp.Tool{Name: "cilium_disconnect_remote_cluster", Description: "Disconnect from a remote cluster"}, handleDisconnectRemoteCluster)
		mcp.AddTool(s, "cilium", &mcp.Tool{Name: "cilium_toggle_hubble", Description: "Enable or disable Hubble"}, handleToggleHubble)
		mcp.AddTool(s, "cilium", &mcp.Tool{Name: "cilium_toggle_cluster_mesh", Description: "Enable or disable cluster mesh"}, handleToggleClusterMesh)
	}

	mcp.AddTool(s, "cilium", &mcp.Tool{Name: "cilium_get_daemon_status", Description: "Get the status of the Cilium daemon for the cluster"}, handleGetDaemonStatus)
	mcp.AddTool(s, "cilium", &mcp.Tool{Name: "cilium_get_endpoints_list", Description: "Get the list of all endpoints in the cluster"}, handleGetEndpointsList)
	mcp.AddTool(s, "cilium", &mcp.Tool{Name: "cilium_get_endpoint_details", Description: "List the details of an endpoint in the cluster"}, handleGetEndpointDetails)
	mcp.AddTool(s, "cilium", &mcp.Tool{Name: "cilium_show_configuration_options", Description: "Show Cilium configuration options"}, handleShowConfigurationOptions)

	if !readOnly {
		mcp.AddTool(s, "cilium", &mcp.Tool{Name: "cilium_toggle_configuration_option", Description: "Toggle a Cilium configuration option"}, handleToggleConfigurationOption)
	}

	mcp.AddTool(s, "cilium", &mcp.Tool{Name: "cilium_list_services", Description: "List services for the cluster"}, handleListServices)
	mcp.AddTool(s, "cilium", &mcp.Tool{Name: "cilium_get_service_information", Description: "Get information about a service in the cluster"}, handleGetServiceInformation)

	if !readOnly {
		mcp.AddTool(s, "cilium", &mcp.Tool{Name: "cilium_update_service", Description: "Update a service in the cluster"}, handleUpdateService)
		mcp.AddTool(s, "cilium", &mcp.Tool{Name: "cilium_delete_service", Description: "Delete a service from the cluster"}, handleDeleteService)
	}

	mcp.AddTool(s, "cilium", &mcp.Tool{Name: "cilium_get_endpoint_details", Description: "List the details of an endpoint in the cluster"}, handleGetEndpointDetails)
	mcp.AddTool(s, "cilium", &mcp.Tool{Name: "cilium_get_endpoint_logs", Description: "Get the logs of an endpoint in the cluster"}, handleGetEndpointLogs)
	mcp.AddTool(s, "cilium", &mcp.Tool{Name: "cilium_get_endpoint_health", Description: "Get the health of an endpoint in the cluster"}, handleGetEndpointHealth)

	if !readOnly {
		mcp.AddTool(s, "cilium", &mcp.Tool{Name: "cilium_manage_endpoint_labels", Description: "Manage the labels (add or delete) of an endpoint in the cluster"}, handleManageEndpointLabels)
		mcp.AddTool(s, "cilium", &mcp.Tool{Name: "cilium_manage_endpoint_config", Description: "Manage the configuration of an endpoint in the cluster"}, handleManageEndpointConfiguration)
		mcp.AddTool(s, "cilium", &mcp.Tool{Name: "cilium_disconnect_endpoint", Description: "Disconnect an endpoint from the network"}, handleDisconnectEndpoint)
	}

	mcp.AddTool(s, "cilium", &mcp.Tool{Name: "cilium_list_identities", Description: "List all identities in the cluster"}, handleListIdentities)
	mcp.AddTool(s, "cilium", &mcp.Tool{Name: "cilium_get_identity_details", Description: "Get the details of an identity in the cluster"}, handleGetIdentityDetails)
	mcp.AddTool(s, "cilium", &mcp.Tool{Name: "cilium_request_debugging_information", Description: "Request debugging information for the cluster"}, handleRequestDebuggingInformation)
	mcp.AddTool(s, "cilium", &mcp.Tool{Name: "cilium_display_encryption_state", Description: "Display the encryption state for the cluster"}, handleDisplayEncryptionState)

	if !readOnly {
		mcp.AddTool(s, "cilium", &mcp.Tool{Name: "cilium_flush_ipsec_state", Description: "Flush the IPsec state for the cluster"}, handleFlushIPsecState)
	}

	mcp.AddTool(s, "cilium", &mcp.Tool{Name: "cilium_list_envoy_config", Description: "List the Envoy configuration for a resource in the cluster"}, handleListEnvoyConfig)
	mcp.AddTool(s, "cilium", &mcp.Tool{Name: "cilium_fqdn_cache", Description: "Manage the FQDN cache for the cluster"}, handleFQDNCache)
	mcp.AddTool(s, "cilium", &mcp.Tool{Name: "cilium_show_dns_names", Description: "Show the DNS names for the cluster"}, handleShowDNSNames)
	mcp.AddTool(s, "cilium", &mcp.Tool{Name: "cilium_list_ip_addresses", Description: "List the IP addresses for the cluster"}, handleListIPAddresses)
	mcp.AddTool(s, "cilium", &mcp.Tool{Name: "cilium_show_ip_cache_information", Description: "Show the IP cache information for the cluster"}, handleShowIPCacheInformation)

	if !readOnly {
		mcp.AddTool(s, "cilium", &mcp.Tool{Name: "cilium_delete_key_from_kv_store", Description: "Delete a key from the kvstore for the cluster"}, handleDeleteKeyFromKVStore)
	}

	mcp.AddTool(s, "cilium", &mcp.Tool{Name: "cilium_get_kv_store_key", Description: "Get a key from the kvstore for the cluster"}, handleGetKVStoreKey)

	if !readOnly {
		mcp.AddTool(s, "cilium", &mcp.Tool{Name: "cilium_set_kv_store_key", Description: "Set a key in the kvstore for the cluster"}, handleSetKVStoreKey)
	}

	mcp.AddTool(s, "cilium", &mcp.Tool{Name: "cilium_show_load_information", Description: "Show load information for the cluster"}, handleShowLoadInformation)
	mcp.AddTool(s, "cilium", &mcp.Tool{Name: "cilium_list_local_redirect_policies", Description: "List local redirect policies for the cluster"}, handleListLocalRedirectPolicies)
	mcp.AddTool(s, "cilium", &mcp.Tool{Name: "cilium_list_bpf_map_events", Description: "List BPF map events for the cluster"}, handleListBPFMapEvents)
	mcp.AddTool(s, "cilium", &mcp.Tool{Name: "cilium_get_bpf_map", Description: "Get BPF map for the cluster"}, handleGetBPFMap)
	mcp.AddTool(s, "cilium", &mcp.Tool{Name: "cilium_list_bpf_maps", Description: "List BPF maps for the cluster"}, handleListBPFMaps)
	mcp.AddTool(s, "cilium", &mcp.Tool{Name: "cilium_list_metrics", Description: "List metrics for the cluster"}, handleListMetrics)
	mcp.AddTool(s, "cilium", &mcp.Tool{Name: "cilium_list_cluster_nodes", Description: "List cluster nodes for the cluster"}, handleListClusterNodes)
	mcp.AddTool(s, "cilium", &mcp.Tool{Name: "cilium_list_node_ids", Description: "List node IDs for the cluster"}, handleListNodeIds)
	mcp.AddTool(s, "cilium", &mcp.Tool{Name: "cilium_display_policy_node_information", Description: "Display policy node information for the cluster"}, handleDisplayPolicyNodeInformation)

	if !readOnly {
		mcp.AddTool(s, "cilium", &mcp.Tool{Name: "cilium_delete_policy_rules", Description: "Delete policy rules for the cluster"}, handleDeletePolicyRules)
	}

	mcp.AddTool(s, "cilium", &mcp.Tool{Name: "cilium_display_selectors", Description: "Display selectors for the cluster"}, handleDisplaySelectors)
	mcp.AddTool(s, "cilium", &mcp.Tool{Name: "cilium_list_xdp_cidr_filters", Description: "List XDP CIDR filters for the cluster"}, handleListXDPCIDRFilters)

	if !readOnly {
		mcp.AddTool(s, "cilium", &mcp.Tool{Name: "cilium_update_xdp_cidr_filters", Description: "Update XDP CIDR filters for the cluster"}, handleUpdateXDPCIDRFilters)
		mcp.AddTool(s, "cilium", &mcp.Tool{Name: "cilium_delete_xdp_cidr_filters", Description: "Delete XDP CIDR filters for the cluster"}, handleDeleteXDPCIDRFilters)
	}

	mcp.AddTool(s, "cilium", &mcp.Tool{Name: "cilium_validate_cilium_network_policies", Description: "Validate Cilium network policies for the cluster"}, handleValidateCiliumNetworkPolicies)
	mcp.AddTool(s, "cilium", &mcp.Tool{Name: "cilium_list_pcap_recorders", Description: "List PCAP recorders for the cluster"}, handleListPCAPRecorders)
	mcp.AddTool(s, "cilium", &mcp.Tool{Name: "cilium_get_pcap_recorder", Description: "Get a PCAP recorder for the cluster"}, handleGetPCAPRecorder)

	if !readOnly {
		mcp.AddTool(s, "cilium", &mcp.Tool{Name: "cilium_delete_pcap_recorder", Description: "Delete a PCAP recorder for the cluster"}, handleDeletePCAPRecorder)
		mcp.AddTool(s, "cilium", &mcp.Tool{Name: "cilium_update_pcap_recorder", Description: "Update a PCAP recorder for the cluster"}, handleUpdatePCAPRecorder)
	}
}

// -- Debug Tools --

func getCiliumPodNameWithContext(ctx context.Context, nodeName string) (string, error) {
	args := []string{"get", "pods", "-n", "kube-system", "--selector=k8s-app=cilium", fmt.Sprintf("--field-selector=spec.nodeName=%s", nodeName), "-o", "jsonpath={.items[0].metadata.name}"}
	kubeconfigPath := utils.GetKubeconfig()
	return commands.NewCommandBuilder("kubectl").
		WithArgs(args...).
		WithKubeconfig(kubeconfigPath).
		Execute(ctx)
}

func runCiliumDbgCommand(ctx context.Context, command, nodeName string) (string, error) {
	return runCiliumDbgCommandWithContext(ctx, command, nodeName)
}

func runCiliumDbgCommandWithContext(ctx context.Context, command, nodeName string) (string, error) {
	podName, err := getCiliumPodNameWithContext(ctx, nodeName)
	if err != nil {
		return "", err
	}
	args := []string{"exec", "-n", "kube-system", podName, "--", "cilium-dbg"}
	args = append(args, strings.Fields(command)...)
	kubeconfigPath := utils.GetKubeconfig()
	return commands.NewCommandBuilder("kubectl").
		WithArgs(args...).
		WithKubeconfig(kubeconfigPath).
		Execute(ctx)
}

func handleGetEndpointDetails(ctx context.Context, request *mcp.CallToolRequest, in getEndpointDetailsInput) (*mcp.CallToolResult, any, error) {
	if in.OutputFormat == "" {
		in.OutputFormat = "json"
	}
	endpointID := in.EndpointID
	labels := in.Labels
	outputFormat := in.OutputFormat
	nodeName := in.NodeName

	var cmd string
	if labels != "" {
		cmd = fmt.Sprintf("endpoint get -l %s -o %s", labels, outputFormat)
	} else if endpointID != "" {
		cmd = fmt.Sprintf("endpoint get %s -o %s", endpointID, outputFormat)
	} else {
		return mcp.NewToolResultError("either endpoint_id or labels must be provided"), nil, nil
	}

	output, err := runCiliumDbgCommand(ctx, cmd, nodeName)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to get endpoint details: %v", err)), nil, nil
	}
	return mcp.NewToolResultText(output), nil, nil
}

func handleGetEndpointLogs(ctx context.Context, request *mcp.CallToolRequest, in getEndpointLogsInput) (*mcp.CallToolResult, any, error) {
	endpointID := in.EndpointID
	nodeName := in.NodeName

	if endpointID == "" {
		return mcp.NewToolResultError("endpoint_id parameter is required"), nil, nil
	}

	cmd := fmt.Sprintf("endpoint logs %s", endpointID)
	output, err := runCiliumDbgCommand(ctx, cmd, nodeName)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to get endpoint logs: %v", err)), nil, nil
	}
	return mcp.NewToolResultText(output), nil, nil
}

func handleGetEndpointHealth(ctx context.Context, request *mcp.CallToolRequest, in getEndpointHealthInput) (*mcp.CallToolResult, any, error) {
	endpointID := in.EndpointID
	nodeName := in.NodeName

	if endpointID == "" {
		return mcp.NewToolResultError("endpoint_id parameter is required"), nil, nil
	}

	cmd := fmt.Sprintf("endpoint health %s", endpointID)
	output, err := runCiliumDbgCommand(ctx, cmd, nodeName)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to get endpoint health: %v", err)), nil, nil
	}
	return mcp.NewToolResultText(output), nil, nil
}

func handleManageEndpointLabels(ctx context.Context, request *mcp.CallToolRequest, in manageEndpointLabelsInput) (*mcp.CallToolResult, any, error) {
	if in.Action == "" {
		in.Action = "add"
	}
	endpointID := in.EndpointID
	labels := in.Labels
	action := in.Action
	nodeName := in.NodeName

	if endpointID == "" || labels == "" {
		return mcp.NewToolResultError("endpoint_id and labels parameters are required"), nil, nil
	}

	cmd := fmt.Sprintf("endpoint labels %s --%s %s", endpointID, action, labels)
	output, err := runCiliumDbgCommand(ctx, cmd, nodeName)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to manage endpoint labels: %v", err)), nil, nil
	}
	return mcp.NewToolResultText(output), nil, nil
}

func handleManageEndpointConfiguration(ctx context.Context, request *mcp.CallToolRequest, in manageEndpointConfigurationInput) (*mcp.CallToolResult, any, error) {
	endpointID := in.EndpointID
	config := in.Config
	nodeName := in.NodeName

	if endpointID == "" {
		return mcp.NewToolResultError("endpoint_id parameter is required"), nil, nil
	}
	if config == "" {
		return mcp.NewToolResultError("config parameter is required"), nil, nil
	}

	command := fmt.Sprintf("endpoint config %s %s", endpointID, config)
	output, err := runCiliumDbgCommand(ctx, command, nodeName)
	if err != nil {
		return mcp.NewToolResultError("Error managing endpoint configuration: " + err.Error()), nil, nil
	}

	return mcp.NewToolResultText(output), nil, nil
}

func handleDisconnectEndpoint(ctx context.Context, request *mcp.CallToolRequest, in disconnectEndpointInput) (*mcp.CallToolResult, any, error) {
	endpointID := in.EndpointID
	nodeName := in.NodeName

	if endpointID == "" {
		return mcp.NewToolResultError("endpoint_id parameter is required"), nil, nil
	}

	cmd := fmt.Sprintf("endpoint disconnect %s", endpointID)
	output, err := runCiliumDbgCommand(ctx, cmd, nodeName)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to disconnect endpoint: %v", err)), nil, nil
	}
	return mcp.NewToolResultText(output), nil, nil
}

func handleGetEndpointsList(ctx context.Context, request *mcp.CallToolRequest, in nodeNameInput) (*mcp.CallToolResult, any, error) {
	nodeName := in.NodeName

	output, err := runCiliumDbgCommand(ctx, "endpoint list", nodeName)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to get endpoints list: %v", err)), nil, nil
	}
	return mcp.NewToolResultText(output), nil, nil
}

func handleListIdentities(ctx context.Context, request *mcp.CallToolRequest, in nodeNameInput) (*mcp.CallToolResult, any, error) {
	nodeName := in.NodeName

	output, err := runCiliumDbgCommand(ctx, "identity list", nodeName)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to list identities: %v", err)), nil, nil
	}
	return mcp.NewToolResultText(output), nil, nil
}

func handleGetIdentityDetails(ctx context.Context, request *mcp.CallToolRequest, in getIdentityDetailsInput) (*mcp.CallToolResult, any, error) {
	identityID := in.IdentityID
	nodeName := in.NodeName

	if identityID == "" {
		return mcp.NewToolResultError("identity_id parameter is required"), nil, nil
	}

	cmd := fmt.Sprintf("identity get %s", identityID)
	output, err := runCiliumDbgCommand(ctx, cmd, nodeName)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to get identity details: %v", err)), nil, nil
	}
	return mcp.NewToolResultText(output), nil, nil
}

func handleShowConfigurationOptions(ctx context.Context, request *mcp.CallToolRequest, in showConfigurationOptionsInput) (*mcp.CallToolResult, any, error) {
	listAll := in.ListAll
	listReadOnly := in.ListReadOnly
	listOptions := in.ListOptions
	nodeName := in.NodeName

	var cmd string
	if listAll {
		cmd = "config --all"
	} else if listReadOnly {
		cmd = "config -r"
	} else if listOptions {
		cmd = "config --list-options"
	} else {
		cmd = "config"
	}

	output, err := runCiliumDbgCommand(ctx, cmd, nodeName)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to show configuration options: %v", err)), nil, nil
	}
	return mcp.NewToolResultText(output), nil, nil
}

func handleToggleConfigurationOption(ctx context.Context, request *mcp.CallToolRequest, in toggleConfigurationOptionInput) (*mcp.CallToolResult, any, error) {
	option := in.Option
	value := true
	if in.Value != nil {
		value = *in.Value
	}
	nodeName := in.NodeName

	if option == "" {
		return mcp.NewToolResultError("option parameter is required"), nil, nil
	}

	valueStr := "enable"
	if !value {
		valueStr = "disable"
	}

	cmd := fmt.Sprintf("config %s=%s", option, valueStr)
	output, err := runCiliumDbgCommand(ctx, cmd, nodeName)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to toggle configuration option: %v", err)), nil, nil
	}
	return mcp.NewToolResultText(output), nil, nil
}

func handleRequestDebuggingInformation(ctx context.Context, request *mcp.CallToolRequest, in nodeNameInput) (*mcp.CallToolResult, any, error) {
	nodeName := in.NodeName

	output, err := runCiliumDbgCommand(ctx, "debuginfo", nodeName)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to request debugging information: %v", err)), nil, nil
	}
	return mcp.NewToolResultText(output), nil, nil
}

func handleDisplayEncryptionState(ctx context.Context, request *mcp.CallToolRequest, in nodeNameInput) (*mcp.CallToolResult, any, error) {
	nodeName := in.NodeName

	output, err := runCiliumDbgCommand(ctx, "encrypt status", nodeName)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to display encryption state: %v", err)), nil, nil
	}
	return mcp.NewToolResultText(output), nil, nil
}

func handleFlushIPsecState(ctx context.Context, request *mcp.CallToolRequest, in nodeNameInput) (*mcp.CallToolResult, any, error) {
	nodeName := in.NodeName

	output, err := runCiliumDbgCommand(ctx, "encrypt flush -f", nodeName)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to flush IPsec state: %v", err)), nil, nil
	}
	return mcp.NewToolResultText(output), nil, nil
}

func handleListEnvoyConfig(ctx context.Context, request *mcp.CallToolRequest, in listEnvoyConfigInput) (*mcp.CallToolResult, any, error) {
	resourceName := in.ResourceName
	nodeName := in.NodeName

	if resourceName == "" {
		return mcp.NewToolResultError("resource_name parameter is required"), nil, nil
	}

	cmd := fmt.Sprintf("envoy admin %s", resourceName)
	output, err := runCiliumDbgCommand(ctx, cmd, nodeName)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to list Envoy config: %v", err)), nil, nil
	}
	return mcp.NewToolResultText(output), nil, nil
}

func handleFQDNCache(ctx context.Context, request *mcp.CallToolRequest, in fqdnCacheInput) (*mcp.CallToolResult, any, error) {
	if in.Command == "" {
		in.Command = "list"
	}
	command := in.Command
	nodeName := in.NodeName

	var cmd string
	if command == "clean" {
		cmd = "fqdn cache clean"
	} else {
		cmd = "fqdn cache list"
	}

	output, err := runCiliumDbgCommand(ctx, cmd, nodeName)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to manage FQDN cache: %v", err)), nil, nil
	}
	return mcp.NewToolResultText(output), nil, nil
}

func handleShowDNSNames(ctx context.Context, request *mcp.CallToolRequest, in nodeNameInput) (*mcp.CallToolResult, any, error) {
	nodeName := in.NodeName

	output, err := runCiliumDbgCommand(ctx, "fqdn names", nodeName)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to show DNS names: %v", err)), nil, nil
	}
	return mcp.NewToolResultText(output), nil, nil
}

func handleListIPAddresses(ctx context.Context, request *mcp.CallToolRequest, in nodeNameInput) (*mcp.CallToolResult, any, error) {
	nodeName := in.NodeName

	output, err := runCiliumDbgCommand(ctx, "ip list", nodeName)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to list IP addresses: %v", err)), nil, nil
	}
	return mcp.NewToolResultText(output), nil, nil
}

func handleShowIPCacheInformation(ctx context.Context, request *mcp.CallToolRequest, in showIPCacheInformationInput) (*mcp.CallToolResult, any, error) {
	cidr := in.CIDR
	labels := in.Labels
	nodeName := in.NodeName

	var cmd string
	if labels != "" {
		cmd = fmt.Sprintf("ip get --labels %s", labels)
	} else if cidr != "" {
		cmd = fmt.Sprintf("ip get %s", cidr)
	} else {
		return mcp.NewToolResultError("either cidr or labels must be provided"), nil, nil
	}

	output, err := runCiliumDbgCommand(ctx, cmd, nodeName)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to show IP cache information: %v", err)), nil, nil
	}
	return mcp.NewToolResultText(output), nil, nil
}

func handleDeleteKeyFromKVStore(ctx context.Context, request *mcp.CallToolRequest, in kvStoreKeyInput) (*mcp.CallToolResult, any, error) {
	key := in.Key
	nodeName := in.NodeName

	if key == "" {
		return mcp.NewToolResultError("key parameter is required"), nil, nil
	}

	cmd := fmt.Sprintf("kvstore delete %s", key)
	output, err := runCiliumDbgCommand(ctx, cmd, nodeName)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to delete key from kvstore: %v", err)), nil, nil
	}
	return mcp.NewToolResultText(output), nil, nil
}

func handleGetKVStoreKey(ctx context.Context, request *mcp.CallToolRequest, in kvStoreKeyInput) (*mcp.CallToolResult, any, error) {
	key := in.Key
	nodeName := in.NodeName

	if key == "" {
		return mcp.NewToolResultError("key parameter is required"), nil, nil
	}

	cmd := fmt.Sprintf("kvstore get %s", key)
	output, err := runCiliumDbgCommand(ctx, cmd, nodeName)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to get key from kvstore: %v", err)), nil, nil
	}
	return mcp.NewToolResultText(output), nil, nil
}

func handleSetKVStoreKey(ctx context.Context, request *mcp.CallToolRequest, in setKVStoreKeyInput) (*mcp.CallToolResult, any, error) {
	key := in.Key
	value := in.Value
	nodeName := in.NodeName

	if key == "" || value == "" {
		return mcp.NewToolResultError("key and value parameters are required"), nil, nil
	}

	cmd := fmt.Sprintf("kvstore set %s=%s", key, value)
	output, err := runCiliumDbgCommand(ctx, cmd, nodeName)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to set key in kvstore: %v", err)), nil, nil
	}
	return mcp.NewToolResultText(output), nil, nil
}

func handleShowLoadInformation(ctx context.Context, request *mcp.CallToolRequest, in nodeNameInput) (*mcp.CallToolResult, any, error) {
	nodeName := in.NodeName

	output, err := runCiliumDbgCommand(ctx, "loadinfo", nodeName)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to show load information: %v", err)), nil, nil
	}
	return mcp.NewToolResultText(output), nil, nil
}

func handleListLocalRedirectPolicies(ctx context.Context, request *mcp.CallToolRequest, in nodeNameInput) (*mcp.CallToolResult, any, error) {
	nodeName := in.NodeName

	output, err := runCiliumDbgCommand(ctx, "lrp list", nodeName)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to list local redirect policies: %v", err)), nil, nil
	}
	return mcp.NewToolResultText(output), nil, nil
}

func handleListBPFMapEvents(ctx context.Context, request *mcp.CallToolRequest, in bpfMapInput) (*mcp.CallToolResult, any, error) {
	mapName := in.MapName
	nodeName := in.NodeName

	if mapName == "" {
		return mcp.NewToolResultError("map_name parameter is required"), nil, nil
	}

	cmd := fmt.Sprintf("map events %s", mapName)
	output, err := runCiliumDbgCommand(ctx, cmd, nodeName)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to list BPF map events: %v", err)), nil, nil
	}
	return mcp.NewToolResultText(output), nil, nil
}

func handleGetBPFMap(ctx context.Context, request *mcp.CallToolRequest, in bpfMapInput) (*mcp.CallToolResult, any, error) {
	mapName := in.MapName
	nodeName := in.NodeName

	if mapName == "" {
		return mcp.NewToolResultError("map_name parameter is required"), nil, nil
	}

	cmd := fmt.Sprintf("map get %s", mapName)
	output, err := runCiliumDbgCommand(ctx, cmd, nodeName)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to get BPF map: %v", err)), nil, nil
	}
	return mcp.NewToolResultText(output), nil, nil
}

func handleListBPFMaps(ctx context.Context, request *mcp.CallToolRequest, in nodeNameInput) (*mcp.CallToolResult, any, error) {
	nodeName := in.NodeName

	output, err := runCiliumDbgCommand(ctx, "map list", nodeName)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to list BPF maps: %v", err)), nil, nil
	}
	return mcp.NewToolResultText(output), nil, nil
}

func handleListMetrics(ctx context.Context, request *mcp.CallToolRequest, in listMetricsInput) (*mcp.CallToolResult, any, error) {
	matchPattern := in.MatchPattern
	nodeName := in.NodeName

	var cmd string
	if matchPattern != "" {
		cmd = fmt.Sprintf("metrics list --pattern %s", matchPattern)
	} else {
		cmd = "metrics list"
	}

	output, err := runCiliumDbgCommand(ctx, cmd, nodeName)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to list metrics: %v", err)), nil, nil
	}
	return mcp.NewToolResultText(output), nil, nil
}

func handleListClusterNodes(ctx context.Context, request *mcp.CallToolRequest, in nodeNameInput) (*mcp.CallToolResult, any, error) {
	nodeName := in.NodeName

	output, err := runCiliumDbgCommand(ctx, "node list", nodeName)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to list cluster nodes: %v", err)), nil, nil
	}
	return mcp.NewToolResultText(output), nil, nil
}

func handleListNodeIds(ctx context.Context, request *mcp.CallToolRequest, in nodeNameInput) (*mcp.CallToolResult, any, error) {
	nodeName := in.NodeName

	output, err := runCiliumDbgCommand(ctx, "nodeid list", nodeName)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to list node IDs: %v", err)), nil, nil
	}
	return mcp.NewToolResultText(output), nil, nil
}

func handleDisplayPolicyNodeInformation(ctx context.Context, request *mcp.CallToolRequest, in displayPolicyNodeInformationInput) (*mcp.CallToolResult, any, error) {
	labels := in.Labels
	nodeName := in.NodeName

	var cmd string
	if labels != "" {
		cmd = fmt.Sprintf("policy get %s", labels)
	} else {
		cmd = "policy get"
	}

	output, err := runCiliumDbgCommand(ctx, cmd, nodeName)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to display policy node information: %v", err)), nil, nil
	}
	return mcp.NewToolResultText(output), nil, nil
}

func handleDeletePolicyRules(ctx context.Context, request *mcp.CallToolRequest, in deletePolicyRulesInput) (*mcp.CallToolResult, any, error) {
	labels := in.Labels
	all := in.All
	nodeName := in.NodeName

	var cmd string
	if all {
		cmd = "policy delete --all"
	} else if labels != "" {
		cmd = fmt.Sprintf("policy delete %s", labels)
	} else {
		return mcp.NewToolResultError("either labels or all=true must be provided"), nil, nil
	}

	output, err := runCiliumDbgCommand(ctx, cmd, nodeName)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to delete policy rules: %v", err)), nil, nil
	}
	return mcp.NewToolResultText(output), nil, nil
}

func handleDisplaySelectors(ctx context.Context, request *mcp.CallToolRequest, in nodeNameInput) (*mcp.CallToolResult, any, error) {
	nodeName := in.NodeName

	output, err := runCiliumDbgCommand(ctx, "policy selectors", nodeName)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to display selectors: %v", err)), nil, nil
	}
	return mcp.NewToolResultText(output), nil, nil
}

func handleListXDPCIDRFilters(ctx context.Context, request *mcp.CallToolRequest, in nodeNameInput) (*mcp.CallToolResult, any, error) {
	nodeName := in.NodeName

	output, err := runCiliumDbgCommand(ctx, "prefilter list", nodeName)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to list XDP CIDR filters: %v", err)), nil, nil
	}
	return mcp.NewToolResultText(output), nil, nil
}

func handleUpdateXDPCIDRFilters(ctx context.Context, request *mcp.CallToolRequest, in xdpCIDRFiltersInput) (*mcp.CallToolResult, any, error) {
	cidrPrefixes := in.CIDRPrefixes
	revision := in.Revision
	nodeName := in.NodeName

	if cidrPrefixes == "" {
		return mcp.NewToolResultError("cidr_prefixes parameter is required"), nil, nil
	}

	var cmd string
	if revision != "" {
		cmd = fmt.Sprintf("prefilter update --cidr %s --revision %s", cidrPrefixes, revision)
	} else {
		cmd = fmt.Sprintf("prefilter update --cidr %s", cidrPrefixes)
	}

	output, err := runCiliumDbgCommand(ctx, cmd, nodeName)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to update XDP CIDR filters: %v", err)), nil, nil
	}
	return mcp.NewToolResultText(output), nil, nil
}

func handleDeleteXDPCIDRFilters(ctx context.Context, request *mcp.CallToolRequest, in xdpCIDRFiltersInput) (*mcp.CallToolResult, any, error) {
	cidrPrefixes := in.CIDRPrefixes
	revision := in.Revision
	nodeName := in.NodeName

	if cidrPrefixes == "" {
		return mcp.NewToolResultError("cidr_prefixes parameter is required"), nil, nil
	}

	var cmd string
	if revision != "" {
		cmd = fmt.Sprintf("prefilter delete --cidr %s --revision %s", cidrPrefixes, revision)
	} else {
		cmd = fmt.Sprintf("prefilter delete --cidr %s", cidrPrefixes)
	}

	output, err := runCiliumDbgCommand(ctx, cmd, nodeName)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to delete XDP CIDR filters: %v", err)), nil, nil
	}
	return mcp.NewToolResultText(output), nil, nil
}

func handleValidateCiliumNetworkPolicies(ctx context.Context, request *mcp.CallToolRequest, in validateCiliumNetworkPoliciesInput) (*mcp.CallToolResult, any, error) {
	enableK8s := in.EnableK8s
	enableK8sAPIDiscovery := in.EnableK8sAPIDiscovery
	nodeName := in.NodeName

	cmd := "preflight validate-cnp"
	if enableK8s {
		cmd += " --enable-k8s"
	}
	if enableK8sAPIDiscovery {
		cmd += " --enable-k8s-api-discovery"
	}

	output, err := runCiliumDbgCommand(ctx, cmd, nodeName)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to validate Cilium network policies: %v", err)), nil, nil
	}
	return mcp.NewToolResultText(output), nil, nil
}

func handleListPCAPRecorders(ctx context.Context, request *mcp.CallToolRequest, in nodeNameInput) (*mcp.CallToolResult, any, error) {
	nodeName := in.NodeName

	output, err := runCiliumDbgCommand(ctx, "recorder list", nodeName)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to list PCAP recorders: %v", err)), nil, nil
	}
	return mcp.NewToolResultText(output), nil, nil
}

func handleGetPCAPRecorder(ctx context.Context, request *mcp.CallToolRequest, in pcapRecorderIDInput) (*mcp.CallToolResult, any, error) {
	recorderID := in.RecorderID
	nodeName := in.NodeName

	if recorderID == "" {
		return mcp.NewToolResultError("recorder_id parameter is required"), nil, nil
	}

	cmd := fmt.Sprintf("recorder get %s", recorderID)
	output, err := runCiliumDbgCommand(ctx, cmd, nodeName)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to get PCAP recorder: %v", err)), nil, nil
	}
	return mcp.NewToolResultText(output), nil, nil
}

func handleDeletePCAPRecorder(ctx context.Context, request *mcp.CallToolRequest, in pcapRecorderIDInput) (*mcp.CallToolResult, any, error) {
	recorderID := in.RecorderID
	nodeName := in.NodeName

	if recorderID == "" {
		return mcp.NewToolResultError("recorder_id parameter is required"), nil, nil
	}

	cmd := fmt.Sprintf("recorder delete %s", recorderID)
	output, err := runCiliumDbgCommand(ctx, cmd, nodeName)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to delete PCAP recorder: %v", err)), nil, nil
	}
	return mcp.NewToolResultText(output), nil, nil
}

func handleUpdatePCAPRecorder(ctx context.Context, request *mcp.CallToolRequest, in updatePCAPRecorderInput) (*mcp.CallToolResult, any, error) {
	if in.Caplen == "" {
		in.Caplen = "0"
	}
	if in.ID == "" {
		in.ID = "0"
	}
	recorderID := in.RecorderID
	filters := in.Filters
	caplen := in.Caplen
	id := in.ID
	nodeName := in.NodeName

	if recorderID == "" || filters == "" {
		return mcp.NewToolResultError("recorder_id and filters parameters are required"), nil, nil
	}

	cmd := fmt.Sprintf("recorder update %s --filters %s --caplen %s --id %s", recorderID, filters, caplen, id)
	output, err := runCiliumDbgCommand(ctx, cmd, nodeName)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to update PCAP recorder: %v", err)), nil, nil
	}
	return mcp.NewToolResultText(output), nil, nil
}

func handleListServices(ctx context.Context, request *mcp.CallToolRequest, in listServicesInput) (*mcp.CallToolResult, any, error) {
	showClusterMeshAffinity := in.ShowClusterMeshAffinity
	nodeName := in.NodeName

	var cmd string
	if showClusterMeshAffinity {
		cmd = "service list --clustermesh-affinity"
	} else {
		cmd = "service list"
	}

	output, err := runCiliumDbgCommand(ctx, cmd, nodeName)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to list services: %v", err)), nil, nil
	}
	return mcp.NewToolResultText(output), nil, nil
}

func handleGetServiceInformation(ctx context.Context, request *mcp.CallToolRequest, in getServiceInformationInput) (*mcp.CallToolResult, any, error) {
	serviceID := in.ServiceID
	nodeName := in.NodeName

	if serviceID == "" {
		return mcp.NewToolResultError("service_id parameter is required"), nil, nil
	}

	cmd := fmt.Sprintf("service get %s", serviceID)
	output, err := runCiliumDbgCommand(ctx, cmd, nodeName)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to get service information: %v", err)), nil, nil
	}
	return mcp.NewToolResultText(output), nil, nil
}

func handleDeleteService(ctx context.Context, request *mcp.CallToolRequest, in deleteServiceInput) (*mcp.CallToolResult, any, error) {
	serviceID := in.ServiceID
	all := in.All
	nodeName := in.NodeName

	var cmd string
	if all {
		cmd = "service delete --all"
	} else if serviceID != "" {
		cmd = fmt.Sprintf("service delete %s", serviceID)
	} else {
		return mcp.NewToolResultError("either service_id or all=true must be provided"), nil, nil
	}

	output, err := runCiliumDbgCommand(ctx, cmd, nodeName)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to delete service: %v", err)), nil, nil
	}
	return mcp.NewToolResultText(output), nil, nil
}

func handleUpdateService(ctx context.Context, request *mcp.CallToolRequest, in updateServiceInput) (*mcp.CallToolResult, any, error) {
	if in.K8sExtTrafficPolicy == "" {
		in.K8sExtTrafficPolicy = "Cluster"
	}
	if in.K8sIntTrafficPolicy == "" {
		in.K8sIntTrafficPolicy = "Cluster"
	}
	if in.Protocol == "" {
		in.Protocol = "TCP"
	}
	if in.States == "" {
		in.States = "active"
	}
	backendWeights := in.BackendWeights
	backends := in.Backends
	frontend := in.Frontend
	id := in.ID
	k8sClusterInternal := in.K8sClusterInternal
	k8sExtTrafficPolicy := in.K8sExtTrafficPolicy
	k8sExternal := in.K8sExternal
	k8sHostPort := in.K8sHostPort
	k8sIntTrafficPolicy := in.K8sIntTrafficPolicy
	k8sLoadBalancer := in.K8sLoadBalancer
	k8sNodePort := in.K8sNodePort
	localRedirect := in.LocalRedirect
	protocol := in.Protocol
	states := in.States
	nodeName := in.NodeName

	if backends == "" || frontend == "" || id == "" {
		return mcp.NewToolResultError("backends, frontend, and id parameters are required"), nil, nil
	}

	cmd := fmt.Sprintf("service update %s --backends %s --frontend %s --protocol %s --states %s",
		id, backends, frontend, protocol, states)

	if backendWeights != "" {
		cmd += fmt.Sprintf(" --backend-weights %s", backendWeights)
	}
	if k8sClusterInternal {
		cmd += " --k8s-cluster-internal"
	}
	if k8sExtTrafficPolicy != "Cluster" {
		cmd += fmt.Sprintf(" --k8s-ext-traffic-policy %s", k8sExtTrafficPolicy)
	}
	if k8sExternal {
		cmd += " --k8s-external"
	}
	if k8sHostPort {
		cmd += " --k8s-host-port"
	}
	if k8sIntTrafficPolicy != "Cluster" {
		cmd += fmt.Sprintf(" --k8s-int-traffic-policy %s", k8sIntTrafficPolicy)
	}
	if k8sLoadBalancer {
		cmd += " --k8s-load-balancer"
	}
	if k8sNodePort {
		cmd += " --k8s-node-port"
	}
	if localRedirect {
		cmd += " --local-redirect"
	}

	output, err := runCiliumDbgCommand(ctx, cmd, nodeName)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to update service: %v", err)), nil, nil
	}
	return mcp.NewToolResultText(output), nil, nil
}

func handleGetDaemonStatus(ctx context.Context, request *mcp.CallToolRequest, in getDaemonStatusInput) (*mcp.CallToolResult, any, error) {
	showAllAddresses := in.ShowAllAddresses
	showAllClusters := in.ShowAllClusters
	showAllControllers := in.ShowAllControllers
	showHealth := in.ShowHealth
	showAllNodes := in.ShowAllNodes
	showAllRedirects := in.ShowAllRedirects
	brief := in.Brief
	nodeName := in.NodeName

	cmd := "status"
	if showAllAddresses {
		cmd += " --all-addresses"
	}
	if showAllClusters {
		cmd += " --all-clusters"
	}
	if showAllControllers {
		cmd += " --all-controllers"
	}
	if showHealth {
		cmd += " --health"
	}
	if showAllNodes {
		cmd += " --all-nodes"
	}
	if showAllRedirects {
		cmd += " --all-redirects"
	}
	if brief {
		cmd += " --brief"
	}

	output, err := runCiliumDbgCommand(ctx, cmd, nodeName)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to get daemon status: %v", err)), nil, nil
	}
	return mcp.NewToolResultText(output), nil, nil
}
