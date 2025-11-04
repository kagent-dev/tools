package cilium

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/kagent-dev/tools/internal/commands"
	"github.com/kagent-dev/tools/internal/logger"
	"github.com/kagent-dev/tools/pkg/utils"
)

func runCiliumCliWithContext(ctx context.Context, args ...string) (string, error) {
	kubeconfigPath := utils.GetKubeconfig()
	return commands.NewCommandBuilder("cilium").
		WithArgs(args...).
		WithKubeconfig(kubeconfigPath).
		Execute(ctx)
}

func handleCiliumStatusAndVersion(ctx context.Context, request *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	status, err := runCiliumCliWithContext(ctx, "status")
	if err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "Error getting Cilium status: " + err.Error()}},
			IsError: true,
		}, nil
	}

	version, err := runCiliumCliWithContext(ctx, "version")
	if err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "Error getting Cilium version: " + err.Error()}},
			IsError: true,
		}, nil
	}

	result := status + "\n" + version
	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: result}},
	}, nil
}

func handleUpgradeCilium(ctx context.Context, request *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var args map[string]interface{}
	if err := json.Unmarshal(request.Params.Arguments, &args); err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "failed to parse arguments"}},
			IsError: true,
		}, nil
	}

	clusterName := ""
	if clusterNameArg, ok := args["cluster_name"].(string); ok {
		clusterName = clusterNameArg
	}

	datapathMode := ""
	if datapathModeArg, ok := args["datapath_mode"].(string); ok {
		datapathMode = datapathModeArg
	}

	cmdArgs := []string{"upgrade"}
	if clusterName != "" {
		cmdArgs = append(cmdArgs, "--cluster-name", clusterName)
	}
	if datapathMode != "" {
		cmdArgs = append(cmdArgs, "--datapath-mode", datapathMode)
	}

	output, err := runCiliumCliWithContext(ctx, cmdArgs...)
	if err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "Error upgrading Cilium: " + err.Error()}},
			IsError: true,
		}, nil
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: output}},
	}, nil
}

func handleInstallCilium(ctx context.Context, request *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var args map[string]interface{}
	if err := json.Unmarshal(request.Params.Arguments, &args); err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "failed to parse arguments"}},
			IsError: true,
		}, nil
	}

	clusterName := ""
	if clusterNameArg, ok := args["cluster_name"].(string); ok {
		clusterName = clusterNameArg
	}

	clusterID := ""
	if clusterIDArg, ok := args["cluster_id"].(string); ok {
		clusterID = clusterIDArg
	}

	datapathMode := ""
	if datapathModeArg, ok := args["datapath_mode"].(string); ok {
		datapathMode = datapathModeArg
	}

	cmdArgs := []string{"install"}
	if clusterName != "" {
		cmdArgs = append(cmdArgs, "--set", "cluster.name="+clusterName)
	}
	if clusterID != "" {
		cmdArgs = append(cmdArgs, "--set", "cluster.id="+clusterID)
	}
	if datapathMode != "" {
		cmdArgs = append(cmdArgs, "--datapath-mode", datapathMode)
	}

	output, err := runCiliumCliWithContext(ctx, cmdArgs...)
	if err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "Error installing Cilium: " + err.Error()}},
			IsError: true,
		}, nil
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: output}},
	}, nil
}

func handleUninstallCilium(ctx context.Context, request *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	output, err := runCiliumCliWithContext(ctx, "uninstall")
	if err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "Error uninstalling Cilium: " + err.Error()}},
			IsError: true,
		}, nil
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: output}},
	}, nil
}

func handleConnectToRemoteCluster(ctx context.Context, request *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var args map[string]interface{}
	if err := json.Unmarshal(request.Params.Arguments, &args); err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "failed to parse arguments"}},
			IsError: true,
		}, nil
	}

	clusterName, ok := args["cluster_name"].(string)
	if !ok || clusterName == "" {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "cluster_name parameter is required"}},
			IsError: true,
		}, nil
	}

	context := ""
	if contextArg, ok := args["context"].(string); ok {
		context = contextArg
	}

	cmdArgs := []string{"clustermesh", "connect", "--destination-cluster", clusterName}
	if context != "" {
		cmdArgs = append(cmdArgs, "--destination-context", context)
	}

	output, err := runCiliumCliWithContext(ctx, cmdArgs...)
	if err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "Error connecting to remote cluster: " + err.Error()}},
			IsError: true,
		}, nil
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: output}},
	}, nil
}

func handleDisconnectRemoteCluster(ctx context.Context, request *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var args map[string]interface{}
	if err := json.Unmarshal(request.Params.Arguments, &args); err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "failed to parse arguments"}},
			IsError: true,
		}, nil
	}

	clusterName, ok := args["cluster_name"].(string)
	if !ok || clusterName == "" {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "cluster_name parameter is required"}},
			IsError: true,
		}, nil
	}

	cmdArgs := []string{"clustermesh", "disconnect", "--destination-cluster", clusterName}

	output, err := runCiliumCliWithContext(ctx, cmdArgs...)
	if err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "Error disconnecting from remote cluster: " + err.Error()}},
			IsError: true,
		}, nil
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: output}},
	}, nil
}

func handleListBGPPeers(ctx context.Context, request *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	output, err := runCiliumCliWithContext(ctx, "bgp", "peers")
	if err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "Error listing BGP peers: " + err.Error()}},
			IsError: true,
		}, nil
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: output}},
	}, nil
}

func handleListBGPRoutes(ctx context.Context, request *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	output, err := runCiliumCliWithContext(ctx, "bgp", "routes")
	if err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "Error listing BGP routes: " + err.Error()}},
			IsError: true,
		}, nil
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: output}},
	}, nil
}

func handleShowClusterMeshStatus(ctx context.Context, request *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	output, err := runCiliumCliWithContext(ctx, "clustermesh", "status")
	if err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "Error getting cluster mesh status: " + err.Error()}},
			IsError: true,
		}, nil
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: output}},
	}, nil
}

func handleShowFeaturesStatus(ctx context.Context, request *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	output, err := runCiliumCliWithContext(ctx, "features", "status")
	if err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "Error getting features status: " + err.Error()}},
			IsError: true,
		}, nil
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: output}},
	}, nil
}

func handleToggleHubble(ctx context.Context, request *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var args map[string]interface{}
	if err := json.Unmarshal(request.Params.Arguments, &args); err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "failed to parse arguments"}},
			IsError: true,
		}, nil
	}

	enableStr := "true"
	if enableArg, ok := args["enable"].(string); ok {
		enableStr = enableArg
	}
	enable := enableStr == "true"

	var action string
	if enable {
		action = "enable"
	} else {
		action = "disable"
	}

	output, err := runCiliumCliWithContext(ctx, "hubble", action)
	if err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "Error toggling Hubble: " + err.Error()}},
			IsError: true,
		}, nil
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: output}},
	}, nil
}

func handleToggleClusterMesh(ctx context.Context, request *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var args map[string]interface{}
	if err := json.Unmarshal(request.Params.Arguments, &args); err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "failed to parse arguments"}},
			IsError: true,
		}, nil
	}

	enableStr := "true"
	if enableArg, ok := args["enable"].(string); ok {
		enableStr = enableArg
	}
	enable := enableStr == "true"

	var action string
	if enable {
		action = "enable"
	} else {
		action = "disable"
	}

	output, err := runCiliumCliWithContext(ctx, "clustermesh", action)
	if err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "Error toggling cluster mesh: " + err.Error()}},
			IsError: true,
		}, nil
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: output}},
	}, nil
}

// Debug tools helper functions
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
	args := []string{"exec", "-it", podName, "--", "cilium-dbg", command}
	kubeconfigPath := utils.GetKubeconfig()
	return commands.NewCommandBuilder("kubectl").
		WithArgs(args...).
		WithKubeconfig(kubeconfigPath).
		Execute(ctx)
}

// Daemon status handlers
func handleGetDaemonStatus(ctx context.Context, request *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var args map[string]interface{}
	if err := json.Unmarshal(request.Params.Arguments, &args); err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "failed to parse arguments"}},
			IsError: true,
		}, nil
	}

	nodeName := ""
	if nodeNameArg, ok := args["node_name"].(string); ok {
		nodeName = nodeNameArg
	}

	cmd := "status"

	// Add flags based on arguments
	if showAllAddresses, ok := args["show_all_addresses"].(string); ok && showAllAddresses == "true" {
		cmd += " --all-addresses"
	}
	if showAllClusters, ok := args["show_all_clusters"].(string); ok && showAllClusters == "true" {
		cmd += " --all-clusters"
	}
	if showAllControllers, ok := args["show_all_controllers"].(string); ok && showAllControllers == "true" {
		cmd += " --all-controllers"
	}
	if showHealth, ok := args["show_health"].(string); ok && showHealth == "true" {
		cmd += " --health"
	}
	if showAllNodes, ok := args["show_all_nodes"].(string); ok && showAllNodes == "true" {
		cmd += " --all-nodes"
	}
	if showAllRedirects, ok := args["show_all_redirects"].(string); ok && showAllRedirects == "true" {
		cmd += " --all-redirects"
	}
	if brief, ok := args["brief"].(string); ok && brief == "true" {
		cmd += " --brief"
	}

	output, err := runCiliumDbgCommand(ctx, cmd, nodeName)
	if err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "Error getting daemon status: " + err.Error()}},
			IsError: true,
		}, nil
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: output}},
	}, nil
}

func handleGetEndpointsList(ctx context.Context, request *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var args map[string]interface{}
	if err := json.Unmarshal(request.Params.Arguments, &args); err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "failed to parse arguments"}},
			IsError: true,
		}, nil
	}

	nodeName := ""
	if nodeNameArg, ok := args["node_name"].(string); ok {
		nodeName = nodeNameArg
	}

	output, err := runCiliumDbgCommand(ctx, "endpoint list", nodeName)
	if err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "Error getting endpoints list: " + err.Error()}},
			IsError: true,
		}, nil
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: output}},
	}, nil
}

func handleGetEndpointDetails(ctx context.Context, request *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var args map[string]interface{}
	if err := json.Unmarshal(request.Params.Arguments, &args); err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "failed to parse arguments"}},
			IsError: true,
		}, nil
	}

	endpointID := ""
	if endpointIDArg, ok := args["endpoint_id"].(string); ok {
		endpointID = endpointIDArg
	}

	labels := ""
	if labelsArg, ok := args["labels"].(string); ok {
		labels = labelsArg
	}

	outputFormat := "json"
	if outputFormatArg, ok := args["output_format"].(string); ok {
		outputFormat = outputFormatArg
	}

	nodeName := ""
	if nodeNameArg, ok := args["node_name"].(string); ok {
		nodeName = nodeNameArg
	}

	var cmd string
	if labels != "" {
		cmd = fmt.Sprintf("endpoint get -l %s -o %s", labels, outputFormat)
	} else if endpointID != "" {
		cmd = fmt.Sprintf("endpoint get %s -o %s", endpointID, outputFormat)
	} else {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "either endpoint_id or labels must be provided"}},
			IsError: true,
		}, nil
	}

	output, err := runCiliumDbgCommand(ctx, cmd, nodeName)
	if err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "Error getting endpoint details: " + err.Error()}},
			IsError: true,
		}, nil
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: output}},
	}, nil
}

func handleShowConfigurationOptions(ctx context.Context, request *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var args map[string]interface{}
	if err := json.Unmarshal(request.Params.Arguments, &args); err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "failed to parse arguments"}},
			IsError: true,
		}, nil
	}

	listAll := false
	if listAllArg, ok := args["list_all"].(string); ok {
		listAll = listAllArg == "true"
	}

	listReadOnly := false
	if listReadOnlyArg, ok := args["list_read_only"].(string); ok {
		listReadOnly = listReadOnlyArg == "true"
	}

	listOptions := false
	if listOptionsArg, ok := args["list_options"].(string); ok {
		listOptions = listOptionsArg == "true"
	}

	nodeName := ""
	if nodeNameArg, ok := args["node_name"].(string); ok {
		nodeName = nodeNameArg
	}

	var cmd string
	if listAll {
		cmd = "endpoint config --all"
	} else if listReadOnly {
		cmd = "endpoint config -r"
	} else if listOptions {
		cmd = "endpoint config --list-options"
	} else {
		cmd = "endpoint config"
	}

	output, err := runCiliumDbgCommand(ctx, cmd, nodeName)
	if err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "Error showing configuration options: " + err.Error()}},
			IsError: true,
		}, nil
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: output}},
	}, nil
}

func handleToggleConfigurationOption(ctx context.Context, request *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var args map[string]interface{}
	if err := json.Unmarshal(request.Params.Arguments, &args); err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "failed to parse arguments"}},
			IsError: true,
		}, nil
	}

	option, ok := args["option"].(string)
	if !ok || option == "" {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "option parameter is required"}},
			IsError: true,
		}, nil
	}

	valueStr, ok := args["value"].(string)
	if !ok || valueStr == "" {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "value parameter is required"}},
			IsError: true,
		}, nil
	}
	value := valueStr == "true"

	nodeName := ""
	if nodeNameArg, ok := args["node_name"].(string); ok {
		nodeName = nodeNameArg
	}

	valueAction := "enable"
	if !value {
		valueAction = "disable"
	}

	cmd := fmt.Sprintf("endpoint config %s=%s", option, valueAction)
	output, err := runCiliumDbgCommand(ctx, cmd, nodeName)
	if err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "Error toggling configuration option: " + err.Error()}},
			IsError: true,
		}, nil
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: output}},
	}, nil
}

func handleListServices(ctx context.Context, request *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var args map[string]interface{}
	if err := json.Unmarshal(request.Params.Arguments, &args); err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "failed to parse arguments"}},
			IsError: true,
		}, nil
	}

	nodeName := ""
	if nodeNameArg, ok := args["node_name"].(string); ok {
		nodeName = nodeNameArg
	}

	cmd := "service list"
	if showClusterMeshAffinity, ok := args["show_cluster_mesh_affinity"].(string); ok && showClusterMeshAffinity == "true" {
		cmd += " --clustermesh-affinity"
	}

	output, err := runCiliumDbgCommand(ctx, cmd, nodeName)
	if err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "Error listing services: " + err.Error()}},
			IsError: true,
		}, nil
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: output}},
	}, nil
}

func handleGetServiceInformation(ctx context.Context, request *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var args map[string]interface{}
	if err := json.Unmarshal(request.Params.Arguments, &args); err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "failed to parse arguments"}},
			IsError: true,
		}, nil
	}

	serviceID, ok := args["service_id"].(string)
	if !ok || serviceID == "" {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "service_id parameter is required"}},
			IsError: true,
		}, nil
	}

	nodeName := ""
	if nodeNameArg, ok := args["node_name"].(string); ok {
		nodeName = nodeNameArg
	}

	cmd := fmt.Sprintf("service get %s", serviceID)
	output, err := runCiliumDbgCommand(ctx, cmd, nodeName)
	if err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "Error getting service information: " + err.Error()}},
			IsError: true,
		}, nil
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: output}},
	}, nil
}
func handleUpdateService(ctx context.Context, request *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var args map[string]interface{}
	if err := json.Unmarshal(request.Params.Arguments, &args); err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "failed to parse arguments"}},
			IsError: true,
		}, nil
	}

	id, ok := args["id"].(string)
	if !ok || id == "" {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "id parameter is required"}},
			IsError: true,
		}, nil
	}

	frontend, ok := args["frontend"].(string)
	if !ok || frontend == "" {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "frontend parameter is required"}},
			IsError: true,
		}, nil
	}

	backends, ok := args["backends"].(string)
	if !ok || backends == "" {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "backends parameter is required"}},
			IsError: true,
		}, nil
	}

	nodeName := ""
	if nodeNameArg, ok := args["node_name"].(string); ok {
		nodeName = nodeNameArg
	}

	cmd := fmt.Sprintf("service update --id %s --frontend %s --backends %s", id, frontend, backends)

	// Add optional parameters
	if backendWeights, ok := args["backend_weights"].(string); ok && backendWeights != "" {
		cmd += fmt.Sprintf(" --backend-weights %s", backendWeights)
	}
	if k8sClusterInternal, ok := args["k8s_cluster_internal"].(string); ok && k8sClusterInternal != "" {
		cmd += fmt.Sprintf(" --k8s-cluster-internal=%s", k8sClusterInternal)
	}
	if k8sExtTrafficPolicy, ok := args["k8s_ext_traffic_policy"].(string); ok && k8sExtTrafficPolicy != "" {
		cmd += fmt.Sprintf(" --k8s-ext-traffic-policy %s", k8sExtTrafficPolicy)
	}
	if k8sExternal, ok := args["k8s_external"].(string); ok && k8sExternal != "" {
		cmd += fmt.Sprintf(" --k8s-external=%s", k8sExternal)
	}
	if k8sHostPort, ok := args["k8s_host_port"].(string); ok && k8sHostPort != "" {
		cmd += fmt.Sprintf(" --k8s-host-port=%s", k8sHostPort)
	}
	if k8sIntTrafficPolicy, ok := args["k8s_int_traffic_policy"].(string); ok && k8sIntTrafficPolicy != "" {
		cmd += fmt.Sprintf(" --k8s-int-traffic-policy %s", k8sIntTrafficPolicy)
	}
	if k8sLoadBalancer, ok := args["k8s_load_balancer"].(string); ok && k8sLoadBalancer != "" {
		cmd += fmt.Sprintf(" --k8s-load-balancer=%s", k8sLoadBalancer)
	}
	if k8sNodePort, ok := args["k8s_node_port"].(string); ok && k8sNodePort != "" {
		cmd += fmt.Sprintf(" --k8s-node-port=%s", k8sNodePort)
	}
	if localRedirect, ok := args["local_redirect"].(string); ok && localRedirect != "" {
		cmd += fmt.Sprintf(" --local-redirect=%s", localRedirect)
	}
	if protocol, ok := args["protocol"].(string); ok && protocol != "" {
		cmd += fmt.Sprintf(" --protocol %s", protocol)
	}
	if states, ok := args["states"].(string); ok && states != "" {
		cmd += fmt.Sprintf(" --states %s", states)
	}

	output, err := runCiliumDbgCommand(ctx, cmd, nodeName)
	if err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "Error updating service: " + err.Error()}},
			IsError: true,
		}, nil
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: output}},
	}, nil
}

func handleDeleteService(ctx context.Context, request *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var args map[string]interface{}
	if err := json.Unmarshal(request.Params.Arguments, &args); err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "failed to parse arguments"}},
			IsError: true,
		}, nil
	}

	nodeName := ""
	if nodeNameArg, ok := args["node_name"].(string); ok {
		nodeName = nodeNameArg
	}

	var cmd string
	if all, ok := args["all"].(string); ok && all == "true" {
		cmd = "service delete --all"
	} else if serviceID, ok := args["service_id"].(string); ok && serviceID != "" {
		cmd = fmt.Sprintf("service delete %s", serviceID)
	} else {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "either service_id or all=true must be provided"}},
			IsError: true,
		}, nil
	}

	output, err := runCiliumDbgCommand(ctx, cmd, nodeName)
	if err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "Error deleting service: " + err.Error()}},
			IsError: true,
		}, nil
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: output}},
	}, nil
}

// Additional debug handlers
func handleGetEndpointLogs(ctx context.Context, request *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var args map[string]interface{}
	if err := json.Unmarshal(request.Params.Arguments, &args); err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "failed to parse arguments"}},
			IsError: true,
		}, nil
	}

	endpointID, ok := args["endpoint_id"].(string)
	if !ok || endpointID == "" {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "endpoint_id parameter is required"}},
			IsError: true,
		}, nil
	}

	nodeName := ""
	if nodeNameArg, ok := args["node_name"].(string); ok {
		nodeName = nodeNameArg
	}

	cmd := fmt.Sprintf("endpoint logs %s", endpointID)
	output, err := runCiliumDbgCommand(ctx, cmd, nodeName)
	if err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "Error getting endpoint logs: " + err.Error()}},
			IsError: true,
		}, nil
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: output}},
	}, nil
}

func handleGetEndpointHealth(ctx context.Context, request *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var args map[string]interface{}
	if err := json.Unmarshal(request.Params.Arguments, &args); err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "failed to parse arguments"}},
			IsError: true,
		}, nil
	}

	endpointID, ok := args["endpoint_id"].(string)
	if !ok || endpointID == "" {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "endpoint_id parameter is required"}},
			IsError: true,
		}, nil
	}

	nodeName := ""
	if nodeNameArg, ok := args["node_name"].(string); ok {
		nodeName = nodeNameArg
	}

	cmd := fmt.Sprintf("endpoint health %s", endpointID)
	output, err := runCiliumDbgCommand(ctx, cmd, nodeName)
	if err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "Error getting endpoint health: " + err.Error()}},
			IsError: true,
		}, nil
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: output}},
	}, nil
}

func handleManageEndpointLabels(ctx context.Context, request *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var args map[string]interface{}
	if err := json.Unmarshal(request.Params.Arguments, &args); err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "failed to parse arguments"}},
			IsError: true,
		}, nil
	}

	endpointID, ok := args["endpoint_id"].(string)
	if !ok || endpointID == "" {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "endpoint_id parameter is required"}},
			IsError: true,
		}, nil
	}

	labels, ok := args["labels"].(string)
	if !ok || labels == "" {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "labels parameter is required"}},
			IsError: true,
		}, nil
	}

	action, ok := args["action"].(string)
	if !ok || action == "" {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "action parameter is required"}},
			IsError: true,
		}, nil
	}

	nodeName := ""
	if nodeNameArg, ok := args["node_name"].(string); ok {
		nodeName = nodeNameArg
	}

	cmd := fmt.Sprintf("endpoint labels %s --%s %s", endpointID, action, labels)
	output, err := runCiliumDbgCommand(ctx, cmd, nodeName)
	if err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "Error managing endpoint labels: " + err.Error()}},
			IsError: true,
		}, nil
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: output}},
	}, nil
}

func handleManageEndpointConfig(ctx context.Context, request *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var args map[string]interface{}
	if err := json.Unmarshal(request.Params.Arguments, &args); err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "failed to parse arguments"}},
			IsError: true,
		}, nil
	}

	endpointID, ok := args["endpoint_id"].(string)
	if !ok || endpointID == "" {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "endpoint_id parameter is required"}},
			IsError: true,
		}, nil
	}

	config, ok := args["config"].(string)
	if !ok || config == "" {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "config parameter is required"}},
			IsError: true,
		}, nil
	}

	nodeName := ""
	if nodeNameArg, ok := args["node_name"].(string); ok {
		nodeName = nodeNameArg
	}

	cmd := fmt.Sprintf("endpoint config %s %s", endpointID, config)
	output, err := runCiliumDbgCommand(ctx, cmd, nodeName)
	if err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "Error managing endpoint configuration: " + err.Error()}},
			IsError: true,
		}, nil
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: output}},
	}, nil
}

func handleDisconnectEndpoint(ctx context.Context, request *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var args map[string]interface{}
	if err := json.Unmarshal(request.Params.Arguments, &args); err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "failed to parse arguments"}},
			IsError: true,
		}, nil
	}

	endpointID, ok := args["endpoint_id"].(string)
	if !ok || endpointID == "" {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "endpoint_id parameter is required"}},
			IsError: true,
		}, nil
	}

	nodeName := ""
	if nodeNameArg, ok := args["node_name"].(string); ok {
		nodeName = nodeNameArg
	}

	cmd := fmt.Sprintf("endpoint disconnect %s", endpointID)
	output, err := runCiliumDbgCommand(ctx, cmd, nodeName)
	if err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "Error disconnecting endpoint: " + err.Error()}},
			IsError: true,
		}, nil
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: output}},
	}, nil
}
func handleListIdentities(ctx context.Context, request *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var args map[string]interface{}
	if err := json.Unmarshal(request.Params.Arguments, &args); err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "failed to parse arguments"}},
			IsError: true,
		}, nil
	}

	nodeName := ""
	if nodeNameArg, ok := args["node_name"].(string); ok {
		nodeName = nodeNameArg
	}

	output, err := runCiliumDbgCommand(ctx, "identity list", nodeName)
	if err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "Error listing identities: " + err.Error()}},
			IsError: true,
		}, nil
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: output}},
	}, nil
}

func handleGetIdentityDetails(ctx context.Context, request *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var args map[string]interface{}
	if err := json.Unmarshal(request.Params.Arguments, &args); err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "failed to parse arguments"}},
			IsError: true,
		}, nil
	}

	identityID, ok := args["identity_id"].(string)
	if !ok || identityID == "" {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "identity_id parameter is required"}},
			IsError: true,
		}, nil
	}

	nodeName := ""
	if nodeNameArg, ok := args["node_name"].(string); ok {
		nodeName = nodeNameArg
	}

	cmd := fmt.Sprintf("identity get %s", identityID)
	output, err := runCiliumDbgCommand(ctx, cmd, nodeName)
	if err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "Error getting identity details: " + err.Error()}},
			IsError: true,
		}, nil
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: output}},
	}, nil
}

func handleRequestDebuggingInformation(ctx context.Context, request *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var args map[string]interface{}
	if err := json.Unmarshal(request.Params.Arguments, &args); err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "failed to parse arguments"}},
			IsError: true,
		}, nil
	}

	nodeName := ""
	if nodeNameArg, ok := args["node_name"].(string); ok {
		nodeName = nodeNameArg
	}

	output, err := runCiliumDbgCommand(ctx, "debuginfo", nodeName)
	if err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "Error requesting debugging information: " + err.Error()}},
			IsError: true,
		}, nil
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: output}},
	}, nil
}

func handleDisplayEncryptionState(ctx context.Context, request *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var args map[string]interface{}
	if err := json.Unmarshal(request.Params.Arguments, &args); err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "failed to parse arguments"}},
			IsError: true,
		}, nil
	}

	nodeName := ""
	if nodeNameArg, ok := args["node_name"].(string); ok {
		nodeName = nodeNameArg
	}

	output, err := runCiliumDbgCommand(ctx, "encrypt status", nodeName)
	if err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "Error displaying encryption state: " + err.Error()}},
			IsError: true,
		}, nil
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: output}},
	}, nil
}

func handleFlushIPsecState(ctx context.Context, request *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var args map[string]interface{}
	if err := json.Unmarshal(request.Params.Arguments, &args); err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "failed to parse arguments"}},
			IsError: true,
		}, nil
	}

	nodeName := ""
	if nodeNameArg, ok := args["node_name"].(string); ok {
		nodeName = nodeNameArg
	}

	output, err := runCiliumDbgCommand(ctx, "encrypt flush -f", nodeName)
	if err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "Error flushing IPsec state: " + err.Error()}},
			IsError: true,
		}, nil
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: output}},
	}, nil
}

func handleListEnvoyConfig(ctx context.Context, request *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var args map[string]interface{}
	if err := json.Unmarshal(request.Params.Arguments, &args); err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "failed to parse arguments"}},
			IsError: true,
		}, nil
	}

	resourceName, ok := args["resource_name"].(string)
	if !ok || resourceName == "" {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "resource_name parameter is required"}},
			IsError: true,
		}, nil
	}

	nodeName := ""
	if nodeNameArg, ok := args["node_name"].(string); ok {
		nodeName = nodeNameArg
	}

	cmd := fmt.Sprintf("envoy admin %s", resourceName)
	output, err := runCiliumDbgCommand(ctx, cmd, nodeName)
	if err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "Error listing Envoy config: " + err.Error()}},
			IsError: true,
		}, nil
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: output}},
	}, nil
}

func handleFQDNCache(ctx context.Context, request *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var args map[string]interface{}
	if err := json.Unmarshal(request.Params.Arguments, &args); err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "failed to parse arguments"}},
			IsError: true,
		}, nil
	}

	command, ok := args["command"].(string)
	if !ok || command == "" {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "command parameter is required"}},
			IsError: true,
		}, nil
	}

	nodeName := ""
	if nodeNameArg, ok := args["node_name"].(string); ok {
		nodeName = nodeNameArg
	}

	var cmd string
	if command == "clean" {
		cmd = "fqdn cache clean"
	} else {
		cmd = "fqdn cache list"
	}

	output, err := runCiliumDbgCommand(ctx, cmd, nodeName)
	if err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "Error managing FQDN cache: " + err.Error()}},
			IsError: true,
		}, nil
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: output}},
	}, nil
}

func handleShowDNSNames(ctx context.Context, request *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var args map[string]interface{}
	if err := json.Unmarshal(request.Params.Arguments, &args); err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "failed to parse arguments"}},
			IsError: true,
		}, nil
	}

	nodeName := ""
	if nodeNameArg, ok := args["node_name"].(string); ok {
		nodeName = nodeNameArg
	}

	output, err := runCiliumDbgCommand(ctx, "dns names", nodeName)
	if err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "Error showing DNS names: " + err.Error()}},
			IsError: true,
		}, nil
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: output}},
	}, nil
}

func handleListIPAddresses(ctx context.Context, request *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var args map[string]interface{}
	if err := json.Unmarshal(request.Params.Arguments, &args); err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "failed to parse arguments"}},
			IsError: true,
		}, nil
	}

	nodeName := ""
	if nodeNameArg, ok := args["node_name"].(string); ok {
		nodeName = nodeNameArg
	}

	output, err := runCiliumDbgCommand(ctx, "ip list", nodeName)
	if err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "Error listing IP addresses: " + err.Error()}},
			IsError: true,
		}, nil
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: output}},
	}, nil
}

func handleShowIPCacheInformation(ctx context.Context, request *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var args map[string]interface{}
	if err := json.Unmarshal(request.Params.Arguments, &args); err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "failed to parse arguments"}},
			IsError: true,
		}, nil
	}

	cidr := ""
	if cidrArg, ok := args["cidr"].(string); ok {
		cidr = cidrArg
	}

	labels := ""
	if labelsArg, ok := args["labels"].(string); ok {
		labels = labelsArg
	}

	nodeName := ""
	if nodeNameArg, ok := args["node_name"].(string); ok {
		nodeName = nodeNameArg
	}

	var cmd string
	if labels != "" {
		cmd = fmt.Sprintf("ip get --labels %s", labels)
	} else if cidr != "" {
		cmd = fmt.Sprintf("ip get %s", cidr)
	} else {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "either cidr or labels must be provided"}},
			IsError: true,
		}, nil
	}

	output, err := runCiliumDbgCommand(ctx, cmd, nodeName)
	if err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "Error showing IP cache information: " + err.Error()}},
			IsError: true,
		}, nil
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: output}},
	}, nil
}
func handleDeleteKeyFromKVStore(ctx context.Context, request *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var args map[string]interface{}
	if err := json.Unmarshal(request.Params.Arguments, &args); err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "failed to parse arguments"}},
			IsError: true,
		}, nil
	}

	key, ok := args["key"].(string)
	if !ok || key == "" {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "key parameter is required"}},
			IsError: true,
		}, nil
	}

	nodeName := ""
	if nodeNameArg, ok := args["node_name"].(string); ok {
		nodeName = nodeNameArg
	}

	cmd := fmt.Sprintf("kvstore delete %s", key)
	output, err := runCiliumDbgCommand(ctx, cmd, nodeName)
	if err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "Error deleting key from kvstore: " + err.Error()}},
			IsError: true,
		}, nil
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: output}},
	}, nil
}

func handleGetKVStoreKey(ctx context.Context, request *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var args map[string]interface{}
	if err := json.Unmarshal(request.Params.Arguments, &args); err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "failed to parse arguments"}},
			IsError: true,
		}, nil
	}

	key, ok := args["key"].(string)
	if !ok || key == "" {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "key parameter is required"}},
			IsError: true,
		}, nil
	}

	nodeName := ""
	if nodeNameArg, ok := args["node_name"].(string); ok {
		nodeName = nodeNameArg
	}

	cmd := fmt.Sprintf("kvstore get %s", key)
	output, err := runCiliumDbgCommand(ctx, cmd, nodeName)
	if err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "Error getting key from kvstore: " + err.Error()}},
			IsError: true,
		}, nil
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: output}},
	}, nil
}

func handleSetKVStoreKey(ctx context.Context, request *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var args map[string]interface{}
	if err := json.Unmarshal(request.Params.Arguments, &args); err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "failed to parse arguments"}},
			IsError: true,
		}, nil
	}

	key, ok := args["key"].(string)
	if !ok || key == "" {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "key parameter is required"}},
			IsError: true,
		}, nil
	}

	value, ok := args["value"].(string)
	if !ok || value == "" {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "value parameter is required"}},
			IsError: true,
		}, nil
	}

	nodeName := ""
	if nodeNameArg, ok := args["node_name"].(string); ok {
		nodeName = nodeNameArg
	}

	cmd := fmt.Sprintf("kvstore set %s=%s", key, value)
	output, err := runCiliumDbgCommand(ctx, cmd, nodeName)
	if err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "Error setting key in kvstore: " + err.Error()}},
			IsError: true,
		}, nil
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: output}},
	}, nil
}

func handleShowLoadInformation(ctx context.Context, request *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var args map[string]interface{}
	if err := json.Unmarshal(request.Params.Arguments, &args); err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "failed to parse arguments"}},
			IsError: true,
		}, nil
	}

	nodeName := ""
	if nodeNameArg, ok := args["node_name"].(string); ok {
		nodeName = nodeNameArg
	}

	output, err := runCiliumDbgCommand(ctx, "loadinfo", nodeName)
	if err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "Error showing load information: " + err.Error()}},
			IsError: true,
		}, nil
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: output}},
	}, nil
}

func handleListLocalRedirectPolicies(ctx context.Context, request *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var args map[string]interface{}
	if err := json.Unmarshal(request.Params.Arguments, &args); err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "failed to parse arguments"}},
			IsError: true,
		}, nil
	}

	nodeName := ""
	if nodeNameArg, ok := args["node_name"].(string); ok {
		nodeName = nodeNameArg
	}

	output, err := runCiliumDbgCommand(ctx, "lrp list", nodeName)
	if err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "Error listing local redirect policies: " + err.Error()}},
			IsError: true,
		}, nil
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: output}},
	}, nil
}

func handleListBPFMapEvents(ctx context.Context, request *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var args map[string]interface{}
	if err := json.Unmarshal(request.Params.Arguments, &args); err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "failed to parse arguments"}},
			IsError: true,
		}, nil
	}

	mapName, ok := args["map_name"].(string)
	if !ok || mapName == "" {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "map_name parameter is required"}},
			IsError: true,
		}, nil
	}

	nodeName := ""
	if nodeNameArg, ok := args["node_name"].(string); ok {
		nodeName = nodeNameArg
	}

	cmd := fmt.Sprintf("bpf map events %s", mapName)
	output, err := runCiliumDbgCommand(ctx, cmd, nodeName)
	if err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "Error listing BPF map events: " + err.Error()}},
			IsError: true,
		}, nil
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: output}},
	}, nil
}

func handleGetBPFMap(ctx context.Context, request *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var args map[string]interface{}
	if err := json.Unmarshal(request.Params.Arguments, &args); err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "failed to parse arguments"}},
			IsError: true,
		}, nil
	}

	mapName, ok := args["map_name"].(string)
	if !ok || mapName == "" {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "map_name parameter is required"}},
			IsError: true,
		}, nil
	}

	nodeName := ""
	if nodeNameArg, ok := args["node_name"].(string); ok {
		nodeName = nodeNameArg
	}

	cmd := fmt.Sprintf("bpf map get %s", mapName)
	output, err := runCiliumDbgCommand(ctx, cmd, nodeName)
	if err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "Error getting BPF map: " + err.Error()}},
			IsError: true,
		}, nil
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: output}},
	}, nil
}

func handleListBPFMaps(ctx context.Context, request *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var args map[string]interface{}
	if err := json.Unmarshal(request.Params.Arguments, &args); err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "failed to parse arguments"}},
			IsError: true,
		}, nil
	}

	nodeName := ""
	if nodeNameArg, ok := args["node_name"].(string); ok {
		nodeName = nodeNameArg
	}

	output, err := runCiliumDbgCommand(ctx, "bpf map list", nodeName)
	if err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "Error listing BPF maps: " + err.Error()}},
			IsError: true,
		}, nil
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: output}},
	}, nil
}

func handleListMetrics(ctx context.Context, request *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var args map[string]interface{}
	if err := json.Unmarshal(request.Params.Arguments, &args); err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "failed to parse arguments"}},
			IsError: true,
		}, nil
	}

	matchPattern := ""
	if matchPatternArg, ok := args["match_pattern"].(string); ok {
		matchPattern = matchPatternArg
	}

	nodeName := ""
	if nodeNameArg, ok := args["node_name"].(string); ok {
		nodeName = nodeNameArg
	}

	var cmd string
	if matchPattern != "" {
		cmd = fmt.Sprintf("metrics list --pattern %s", matchPattern)
	} else {
		cmd = "metrics list"
	}

	output, err := runCiliumDbgCommand(ctx, cmd, nodeName)
	if err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "Error listing metrics: " + err.Error()}},
			IsError: true,
		}, nil
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: output}},
	}, nil
}

func handleListClusterNodes(ctx context.Context, request *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var args map[string]interface{}
	if err := json.Unmarshal(request.Params.Arguments, &args); err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "failed to parse arguments"}},
			IsError: true,
		}, nil
	}

	nodeName := ""
	if nodeNameArg, ok := args["node_name"].(string); ok {
		nodeName = nodeNameArg
	}

	output, err := runCiliumDbgCommand(ctx, "nodes list", nodeName)
	if err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "Error listing cluster nodes: " + err.Error()}},
			IsError: true,
		}, nil
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: output}},
	}, nil
}

func handleListNodeIds(ctx context.Context, request *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var args map[string]interface{}
	if err := json.Unmarshal(request.Params.Arguments, &args); err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "failed to parse arguments"}},
			IsError: true,
		}, nil
	}

	nodeName := ""
	if nodeNameArg, ok := args["node_name"].(string); ok {
		nodeName = nodeNameArg
	}

	output, err := runCiliumDbgCommand(ctx, "nodeid list", nodeName)
	if err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "Error listing node IDs: " + err.Error()}},
			IsError: true,
		}, nil
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: output}},
	}, nil
}
func handleDisplayPolicyNodeInformation(ctx context.Context, request *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var args map[string]interface{}
	if err := json.Unmarshal(request.Params.Arguments, &args); err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "failed to parse arguments"}},
			IsError: true,
		}, nil
	}

	labels := ""
	if labelsArg, ok := args["labels"].(string); ok {
		labels = labelsArg
	}

	nodeName := ""
	if nodeNameArg, ok := args["node_name"].(string); ok {
		nodeName = nodeNameArg
	}

	var cmd string
	if labels != "" {
		cmd = fmt.Sprintf("policy get %s", labels)
	} else {
		cmd = "policy get"
	}

	output, err := runCiliumDbgCommand(ctx, cmd, nodeName)
	if err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "Error displaying policy node information: " + err.Error()}},
			IsError: true,
		}, nil
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: output}},
	}, nil
}

func handleDeletePolicyRules(ctx context.Context, request *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var args map[string]interface{}
	if err := json.Unmarshal(request.Params.Arguments, &args); err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "failed to parse arguments"}},
			IsError: true,
		}, nil
	}

	labels := ""
	if labelsArg, ok := args["labels"].(string); ok {
		labels = labelsArg
	}

	all := false
	if allArg, ok := args["all"].(string); ok {
		all = allArg == "true"
	}

	nodeName := ""
	if nodeNameArg, ok := args["node_name"].(string); ok {
		nodeName = nodeNameArg
	}

	var cmd string
	if all {
		cmd = "policy delete --all"
	} else if labels != "" {
		cmd = fmt.Sprintf("policy delete %s", labels)
	} else {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "either labels or all=true must be provided"}},
			IsError: true,
		}, nil
	}

	output, err := runCiliumDbgCommand(ctx, cmd, nodeName)
	if err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "Error deleting policy rules: " + err.Error()}},
			IsError: true,
		}, nil
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: output}},
	}, nil
}

func handleDisplaySelectors(ctx context.Context, request *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var args map[string]interface{}
	if err := json.Unmarshal(request.Params.Arguments, &args); err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "failed to parse arguments"}},
			IsError: true,
		}, nil
	}

	nodeName := ""
	if nodeNameArg, ok := args["node_name"].(string); ok {
		nodeName = nodeNameArg
	}

	output, err := runCiliumDbgCommand(ctx, "policy selectors", nodeName)
	if err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "Error displaying selectors: " + err.Error()}},
			IsError: true,
		}, nil
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: output}},
	}, nil
}

func handleListXDPCIDRFilters(ctx context.Context, request *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var args map[string]interface{}
	if err := json.Unmarshal(request.Params.Arguments, &args); err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "failed to parse arguments"}},
			IsError: true,
		}, nil
	}

	nodeName := ""
	if nodeNameArg, ok := args["node_name"].(string); ok {
		nodeName = nodeNameArg
	}

	output, err := runCiliumDbgCommand(ctx, "prefilter list", nodeName)
	if err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "Error listing XDP CIDR filters: " + err.Error()}},
			IsError: true,
		}, nil
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: output}},
	}, nil
}

func handleUpdateXDPCIDRFilters(ctx context.Context, request *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var args map[string]interface{}
	if err := json.Unmarshal(request.Params.Arguments, &args); err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "failed to parse arguments"}},
			IsError: true,
		}, nil
	}

	cidrPrefixes, ok := args["cidr_prefixes"].(string)
	if !ok || cidrPrefixes == "" {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "cidr_prefixes parameter is required"}},
			IsError: true,
		}, nil
	}

	revision := ""
	if revisionArg, ok := args["revision"].(string); ok {
		revision = revisionArg
	}

	nodeName := ""
	if nodeNameArg, ok := args["node_name"].(string); ok {
		nodeName = nodeNameArg
	}

	cmd := fmt.Sprintf("prefilter update %s", cidrPrefixes)
	if revision != "" {
		cmd += fmt.Sprintf(" --revision %s", revision)
	}

	output, err := runCiliumDbgCommand(ctx, cmd, nodeName)
	if err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "Error updating XDP CIDR filters: " + err.Error()}},
			IsError: true,
		}, nil
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: output}},
	}, nil
}

func handleDeleteXDPCIDRFilters(ctx context.Context, request *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var args map[string]interface{}
	if err := json.Unmarshal(request.Params.Arguments, &args); err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "failed to parse arguments"}},
			IsError: true,
		}, nil
	}

	cidrPrefixes, ok := args["cidr_prefixes"].(string)
	if !ok || cidrPrefixes == "" {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "cidr_prefixes parameter is required"}},
			IsError: true,
		}, nil
	}

	revision := ""
	if revisionArg, ok := args["revision"].(string); ok {
		revision = revisionArg
	}

	nodeName := ""
	if nodeNameArg, ok := args["node_name"].(string); ok {
		nodeName = nodeNameArg
	}

	cmd := fmt.Sprintf("prefilter delete %s", cidrPrefixes)
	if revision != "" {
		cmd += fmt.Sprintf(" --revision %s", revision)
	}

	output, err := runCiliumDbgCommand(ctx, cmd, nodeName)
	if err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "Error deleting XDP CIDR filters: " + err.Error()}},
			IsError: true,
		}, nil
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: output}},
	}, nil
}

func handleValidateCiliumNetworkPolicies(ctx context.Context, request *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var args map[string]interface{}
	if err := json.Unmarshal(request.Params.Arguments, &args); err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "failed to parse arguments"}},
			IsError: true,
		}, nil
	}

	enableK8s := false
	if enableK8sArg, ok := args["enable_k8s"].(string); ok {
		enableK8s = enableK8sArg == "true"
	}

	enableK8sAPIDiscovery := false
	if enableK8sAPIDiscoveryArg, ok := args["enable_k8s_api_discovery"].(string); ok {
		enableK8sAPIDiscovery = enableK8sAPIDiscoveryArg == "true"
	}

	nodeName := ""
	if nodeNameArg, ok := args["node_name"].(string); ok {
		nodeName = nodeNameArg
	}

	cmd := "policy validate"
	if enableK8s {
		cmd += " --enable-k8s"
	}
	if enableK8sAPIDiscovery {
		cmd += " --enable-k8s-api-discovery"
	}

	output, err := runCiliumDbgCommand(ctx, cmd, nodeName)
	if err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "Error validating Cilium network policies: " + err.Error()}},
			IsError: true,
		}, nil
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: output}},
	}, nil
}

func handleListPCAPRecorders(ctx context.Context, request *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var args map[string]interface{}
	if err := json.Unmarshal(request.Params.Arguments, &args); err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "failed to parse arguments"}},
			IsError: true,
		}, nil
	}

	nodeName := ""
	if nodeNameArg, ok := args["node_name"].(string); ok {
		nodeName = nodeNameArg
	}

	output, err := runCiliumDbgCommand(ctx, "recorder list", nodeName)
	if err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "Error listing PCAP recorders: " + err.Error()}},
			IsError: true,
		}, nil
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: output}},
	}, nil
}

func handleGetPCAPRecorder(ctx context.Context, request *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var args map[string]interface{}
	if err := json.Unmarshal(request.Params.Arguments, &args); err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "failed to parse arguments"}},
			IsError: true,
		}, nil
	}

	recorderID, ok := args["recorder_id"].(string)
	if !ok || recorderID == "" {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "recorder_id parameter is required"}},
			IsError: true,
		}, nil
	}

	nodeName := ""
	if nodeNameArg, ok := args["node_name"].(string); ok {
		nodeName = nodeNameArg
	}

	cmd := fmt.Sprintf("recorder get %s", recorderID)
	output, err := runCiliumDbgCommand(ctx, cmd, nodeName)
	if err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "Error getting PCAP recorder: " + err.Error()}},
			IsError: true,
		}, nil
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: output}},
	}, nil
}

func handleDeletePCAPRecorder(ctx context.Context, request *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var args map[string]interface{}
	if err := json.Unmarshal(request.Params.Arguments, &args); err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "failed to parse arguments"}},
			IsError: true,
		}, nil
	}

	recorderID, ok := args["recorder_id"].(string)
	if !ok || recorderID == "" {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "recorder_id parameter is required"}},
			IsError: true,
		}, nil
	}

	nodeName := ""
	if nodeNameArg, ok := args["node_name"].(string); ok {
		nodeName = nodeNameArg
	}

	cmd := fmt.Sprintf("recorder delete %s", recorderID)
	output, err := runCiliumDbgCommand(ctx, cmd, nodeName)
	if err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "Error deleting PCAP recorder: " + err.Error()}},
			IsError: true,
		}, nil
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: output}},
	}, nil
}

func handleUpdatePCAPRecorder(ctx context.Context, request *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var args map[string]interface{}
	if err := json.Unmarshal(request.Params.Arguments, &args); err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "failed to parse arguments"}},
			IsError: true,
		}, nil
	}

	recorderID, ok := args["recorder_id"].(string)
	if !ok || recorderID == "" {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "recorder_id parameter is required"}},
			IsError: true,
		}, nil
	}

	filters, ok := args["filters"].(string)
	if !ok || filters == "" {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "filters parameter is required"}},
			IsError: true,
		}, nil
	}

	nodeName := ""
	if nodeNameArg, ok := args["node_name"].(string); ok {
		nodeName = nodeNameArg
	}

	cmd := fmt.Sprintf("recorder update %s --filters %s", recorderID, filters)

	// Add optional parameters
	if caplen, ok := args["caplen"].(string); ok && caplen != "" {
		cmd += fmt.Sprintf(" --caplen %s", caplen)
	}
	if id, ok := args["id"].(string); ok && id != "" {
		cmd += fmt.Sprintf(" --id %s", id)
	}

	output, err := runCiliumDbgCommand(ctx, cmd, nodeName)
	if err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "Error updating PCAP recorder: " + err.Error()}},
			IsError: true,
		}, nil
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: output}},
	}, nil
}

// ToolRegistry is an interface for tool registration (to avoid import cycles)
type ToolRegistry interface {
	Register(tool *mcp.Tool, handler mcp.ToolHandler)
}

// RegisterTools registers Cilium tools with the MCP server
func RegisterTools(s *mcp.Server) error {
	return RegisterToolsWithRegistry(s, nil)
}

// RegisterToolsWithRegistry registers Cilium tools with the MCP server and optionally with a tool registry
func RegisterToolsWithRegistry(s *mcp.Server, registry ToolRegistry) error {
	logger.Get().Info("RegisterTools initialized")

	// Helper function to register tool with both server and registry
	registerTool := func(tool *mcp.Tool, handler mcp.ToolHandler) {
		s.AddTool(tool, handler)
		if registry != nil {
			registry.Register(tool, handler)
		}
	}
	// Register all Cilium tools (main and debug)
	registerTool(&mcp.Tool{
		Name:        "cilium_status_and_version",
		Description: "Get the status and version of Cilium installation",
		InputSchema: &jsonschema.Schema{
			Type: "object",
		},
	}, handleCiliumStatusAndVersion)

	registerTool(&mcp.Tool{
		Name:        "cilium_upgrade_cilium",
		Description: "Upgrade Cilium on the cluster",
		InputSchema: &jsonschema.Schema{
			Type: "object",
			Properties: map[string]*jsonschema.Schema{
				"cluster_name": {
					Type:        "string",
					Description: "The name of the cluster to upgrade Cilium on",
				},
				"datapath_mode": {
					Type:        "string",
					Description: "The datapath mode to use for Cilium (tunnel, native, aws-eni, gke, azure, aks-byocni)",
				},
			},
		},
	}, handleUpgradeCilium)

	registerTool(&mcp.Tool{
		Name:        "cilium_install_cilium",
		Description: "Install Cilium on the cluster",
		InputSchema: &jsonschema.Schema{
			Type: "object",
			Properties: map[string]*jsonschema.Schema{
				"cluster_name": {
					Type:        "string",
					Description: "The name of the cluster to install Cilium on",
				},
				"cluster_id": {
					Type:        "string",
					Description: "The ID of the cluster to install Cilium on",
				},
				"datapath_mode": {
					Type:        "string",
					Description: "The datapath mode to use for Cilium (tunnel, native, aws-eni, gke, azure, aks-byocni)",
				},
			},
		},
	}, handleInstallCilium)

	registerTool(&mcp.Tool{
		Name:        "cilium_uninstall_cilium",
		Description: "Uninstall Cilium from the cluster",
		InputSchema: &jsonschema.Schema{
			Type: "object",
		},
	}, handleUninstallCilium)

	registerTool(&mcp.Tool{
		Name:        "cilium_connect_to_remote_cluster",
		Description: "Connect to a remote cluster for cluster mesh",
		InputSchema: &jsonschema.Schema{
			Type: "object",
			Properties: map[string]*jsonschema.Schema{
				"cluster_name": {
					Type:        "string",
					Description: "The name of the destination cluster",
				},
				"context": {
					Type:        "string",
					Description: "The kubectl context for the destination cluster",
				},
			},
			Required: []string{"cluster_name"},
		},
	}, handleConnectToRemoteCluster)

	registerTool(&mcp.Tool{
		Name:        "cilium_disconnect_remote_cluster",
		Description: "Disconnect from a remote cluster",
		InputSchema: &jsonschema.Schema{
			Type: "object",
			Properties: map[string]*jsonschema.Schema{
				"cluster_name": {
					Type:        "string",
					Description: "The name of the destination cluster",
				},
			},
			Required: []string{"cluster_name"},
		},
	}, handleDisconnectRemoteCluster)

	registerTool(&mcp.Tool{
		Name:        "cilium_list_bgp_peers",
		Description: "List BGP peers",
		InputSchema: &jsonschema.Schema{
			Type: "object",
		},
	}, handleListBGPPeers)

	registerTool(&mcp.Tool{
		Name:        "cilium_list_bgp_routes",
		Description: "List BGP routes",
		InputSchema: &jsonschema.Schema{
			Type: "object",
		},
	}, handleListBGPRoutes)

	registerTool(&mcp.Tool{
		Name:        "cilium_show_cluster_mesh_status",
		Description: "Show cluster mesh status",
		InputSchema: &jsonschema.Schema{
			Type: "object",
		},
	}, handleShowClusterMeshStatus)

	registerTool(&mcp.Tool{
		Name:        "cilium_show_features_status",
		Description: "Show Cilium features status",
		InputSchema: &jsonschema.Schema{
			Type: "object",
		},
	}, handleShowFeaturesStatus)

	registerTool(&mcp.Tool{
		Name:        "cilium_toggle_hubble",
		Description: "Enable or disable Hubble",
		InputSchema: &jsonschema.Schema{
			Type: "object",
			Properties: map[string]*jsonschema.Schema{
				"enable": {
					Type:        "string",
					Description: "Set to 'true' to enable, 'false' to disable",
				},
			},
		},
	}, handleToggleHubble)

	registerTool(&mcp.Tool{
		Name:        "cilium_toggle_cluster_mesh",
		Description: "Enable or disable cluster mesh",
		InputSchema: &jsonschema.Schema{
			Type: "object",
			Properties: map[string]*jsonschema.Schema{
				"enable": {
					Type:        "string",
					Description: "Set to 'true' to enable, 'false' to disable",
				},
			},
		},
	}, handleToggleClusterMesh)

	// Add tools that are also needed by cilium-manager agent
	registerTool(&mcp.Tool{
		Name:        "cilium_get_daemon_status",
		Description: "Get the status of the Cilium daemon for the cluster",
		InputSchema: &jsonschema.Schema{
			Type: "object",
			Properties: map[string]*jsonschema.Schema{
				"show_all_addresses": {
					Type:        "string",
					Description: "Whether to show all addresses",
				},
				"show_all_clusters": {
					Type:        "string",
					Description: "Whether to show all clusters",
				},
				"show_all_controllers": {
					Type:        "string",
					Description: "Whether to show all controllers",
				},
				"show_health": {
					Type:        "string",
					Description: "Whether to show health",
				},
				"show_all_nodes": {
					Type:        "string",
					Description: "Whether to show all nodes",
				},
				"show_all_redirects": {
					Type:        "string",
					Description: "Whether to show all redirects",
				},
				"brief": {
					Type:        "string",
					Description: "Whether to show a brief status",
				},
				"node_name": {
					Type:        "string",
					Description: "The name of the node to get the daemon status for",
				},
			},
		},
	}, handleGetDaemonStatus)

	registerTool(&mcp.Tool{
		Name:        "cilium_get_endpoints_list",
		Description: "Get the list of all endpoints in the cluster",
		InputSchema: &jsonschema.Schema{
			Type: "object",
			Properties: map[string]*jsonschema.Schema{
				"node_name": {
					Type:        "string",
					Description: "The name of the node to get the endpoints list for",
				},
			},
		},
	}, handleGetEndpointsList)

	registerTool(&mcp.Tool{
		Name:        "cilium_get_endpoint_details",
		Description: "List the details of an endpoint in the cluster",
		InputSchema: &jsonschema.Schema{
			Type: "object",
			Properties: map[string]*jsonschema.Schema{
				"endpoint_id": {
					Type:        "string",
					Description: "The ID of the endpoint to get details for",
				},
				"labels": {
					Type:        "string",
					Description: "The labels of the endpoint to get details for",
				},
				"output_format": {
					Type:        "string",
					Description: "The output format of the endpoint details (json, yaml, jsonpath)",
				},
				"node_name": {
					Type:        "string",
					Description: "The name of the node to get the endpoint details for",
				},
			},
		},
	}, handleGetEndpointDetails)

	registerTool(&mcp.Tool{
		Name:        "cilium_show_configuration_options",
		Description: "Show Cilium configuration options",
		InputSchema: &jsonschema.Schema{
			Type: "object",
			Properties: map[string]*jsonschema.Schema{
				"list_all": {
					Type:        "string",
					Description: "Whether to list all configuration options",
				},
				"list_read_only": {
					Type:        "string",
					Description: "Whether to list read-only configuration options",
				},
				"list_options": {
					Type:        "string",
					Description: "Whether to list options",
				},
				"node_name": {
					Type:        "string",
					Description: "The name of the node to show the configuration options for",
				},
			},
		},
	}, handleShowConfigurationOptions)

	registerTool(&mcp.Tool{
		Name:        "cilium_toggle_configuration_option",
		Description: "Toggle a Cilium configuration option",
		InputSchema: &jsonschema.Schema{
			Type: "object",
			Properties: map[string]*jsonschema.Schema{
				"option": {
					Type:        "string",
					Description: "The option to toggle",
				},
				"value": {
					Type:        "string",
					Description: "The value to set the option to (true/false)",
				},
				"node_name": {
					Type:        "string",
					Description: "The name of the node to toggle the configuration option for",
				},
			},
			Required: []string{"option", "value"},
		},
	}, handleToggleConfigurationOption)

	registerTool(&mcp.Tool{
		Name:        "cilium_list_services",
		Description: "List services for the cluster",
		InputSchema: &jsonschema.Schema{
			Type: "object",
			Properties: map[string]*jsonschema.Schema{
				"show_cluster_mesh_affinity": {
					Type:        "string",
					Description: "Whether to show cluster mesh affinity",
				},
				"node_name": {
					Type:        "string",
					Description: "The name of the node to get the services for",
				},
			},
		},
	}, handleListServices)

	registerTool(&mcp.Tool{
		Name:        "cilium_get_service_information",
		Description: "Get information about a service in the cluster",
		InputSchema: &jsonschema.Schema{
			Type: "object",
			Properties: map[string]*jsonschema.Schema{
				"service_id": {
					Type:        "string",
					Description: "The ID of the service to get information about",
				},
				"node_name": {
					Type:        "string",
					Description: "The name of the node to get the service information for",
				},
			},
			Required: []string{"service_id"},
		},
	}, handleGetServiceInformation)

	// Continue with more tool registrations
	registerTool(&mcp.Tool{
		Name:        "cilium_update_service",
		Description: "Update a service in the cluster",
		InputSchema: &jsonschema.Schema{
			Type: "object",
			Properties: map[string]*jsonschema.Schema{
				"backend_weights": {
					Type:        "string",
					Description: "The backend weights to update the service with",
				},
				"backends": {
					Type:        "string",
					Description: "The backends to update the service with",
				},
				"frontend": {
					Type:        "string",
					Description: "The frontend to update the service with",
				},
				"id": {
					Type:        "string",
					Description: "The ID of the service to update",
				},
				"k8s_cluster_internal": {
					Type:        "string",
					Description: "Whether to update the k8s cluster internal flag",
				},
				"k8s_ext_traffic_policy": {
					Type:        "string",
					Description: "The k8s ext traffic policy to update the service with",
				},
				"k8s_external": {
					Type:        "string",
					Description: "Whether to update the k8s external flag",
				},
				"k8s_host_port": {
					Type:        "string",
					Description: "Whether to update the k8s host port flag",
				},
				"k8s_int_traffic_policy": {
					Type:        "string",
					Description: "The k8s int traffic policy to update the service with",
				},
				"k8s_load_balancer": {
					Type:        "string",
					Description: "Whether to update the k8s load balancer flag",
				},
				"k8s_node_port": {
					Type:        "string",
					Description: "Whether to update the k8s node port flag",
				},
				"local_redirect": {
					Type:        "string",
					Description: "Whether to update the local redirect flag",
				},
				"protocol": {
					Type:        "string",
					Description: "The protocol to update the service with",
				},
				"states": {
					Type:        "string",
					Description: "The states to update the service with",
				},
				"node_name": {
					Type:        "string",
					Description: "The name of the node to update the service on",
				},
			},
			Required: []string{"id", "frontend", "backends"},
		},
	}, handleUpdateService)

	registerTool(&mcp.Tool{
		Name:        "cilium_delete_service",
		Description: "Delete a service from the cluster",
		InputSchema: &jsonschema.Schema{
			Type: "object",
			Properties: map[string]*jsonschema.Schema{
				"service_id": {
					Type:        "string",
					Description: "The ID of the service to delete",
				},
				"all": {
					Type:        "string",
					Description: "Whether to delete all services (true/false)",
				},
				"node_name": {
					Type:        "string",
					Description: "The name of the node to delete the service from",
				},
			},
		},
	}, handleDeleteService)

	// Debug tools
	registerTool(&mcp.Tool{
		Name:        "cilium_get_endpoint_logs",
		Description: "Get the logs of an endpoint in the cluster",
		InputSchema: &jsonschema.Schema{
			Type: "object",
			Properties: map[string]*jsonschema.Schema{
				"endpoint_id": {
					Type:        "string",
					Description: "The ID of the endpoint to get logs for",
				},
				"node_name": {
					Type:        "string",
					Description: "The name of the node to get the endpoint logs for",
				},
			},
			Required: []string{"endpoint_id"},
		},
	}, handleGetEndpointLogs)

	registerTool(&mcp.Tool{
		Name:        "cilium_get_endpoint_health",
		Description: "Get the health of an endpoint in the cluster",
		InputSchema: &jsonschema.Schema{
			Type: "object",
			Properties: map[string]*jsonschema.Schema{
				"endpoint_id": {
					Type:        "string",
					Description: "The ID of the endpoint to get health for",
				},
				"node_name": {
					Type:        "string",
					Description: "The name of the node to get the endpoint health for",
				},
			},
			Required: []string{"endpoint_id"},
		},
	}, handleGetEndpointHealth)

	registerTool(&mcp.Tool{
		Name:        "cilium_manage_endpoint_labels",
		Description: "Manage the labels (add or delete) of an endpoint in the cluster",
		InputSchema: &jsonschema.Schema{
			Type: "object",
			Properties: map[string]*jsonschema.Schema{
				"endpoint_id": {
					Type:        "string",
					Description: "The ID of the endpoint to manage labels for",
				},
				"labels": {
					Type:        "string",
					Description: "Space-separated labels to manage (e.g., 'key1=value1 key2=value2')",
				},
				"action": {
					Type:        "string",
					Description: "The action to perform on the labels (add or delete)",
				},
				"node_name": {
					Type:        "string",
					Description: "The name of the node to manage the endpoint labels on",
				},
			},
			Required: []string{"endpoint_id", "labels", "action"},
		},
	}, handleManageEndpointLabels)

	registerTool(&mcp.Tool{
		Name:        "cilium_manage_endpoint_config",
		Description: "Manage the configuration of an endpoint in the cluster",
		InputSchema: &jsonschema.Schema{
			Type: "object",
			Properties: map[string]*jsonschema.Schema{
				"endpoint_id": {
					Type:        "string",
					Description: "The ID of the endpoint to manage configuration for",
				},
				"config": {
					Type:        "string",
					Description: "The configuration to manage for the endpoint provided as a space-separated list of key-value pairs (e.g. 'DropNotification=false TraceNotification=false')",
				},
				"node_name": {
					Type:        "string",
					Description: "The name of the node to manage the endpoint configuration on",
				},
			},
			Required: []string{"endpoint_id", "config"},
		},
	}, handleManageEndpointConfig)

	registerTool(&mcp.Tool{
		Name:        "cilium_disconnect_endpoint",
		Description: "Disconnect an endpoint from the network",
		InputSchema: &jsonschema.Schema{
			Type: "object",
			Properties: map[string]*jsonschema.Schema{
				"endpoint_id": {
					Type:        "string",
					Description: "The ID of the endpoint to disconnect",
				},
				"node_name": {
					Type:        "string",
					Description: "The name of the node to disconnect the endpoint from",
				},
			},
			Required: []string{"endpoint_id"},
		},
	}, handleDisconnectEndpoint)

	registerTool(&mcp.Tool{
		Name:        "cilium_list_identities",
		Description: "List all identities in the cluster",
		InputSchema: &jsonschema.Schema{
			Type: "object",
			Properties: map[string]*jsonschema.Schema{
				"node_name": {
					Type:        "string",
					Description: "The name of the node to list the identities for",
				},
			},
		},
	}, handleListIdentities)

	registerTool(&mcp.Tool{
		Name:        "cilium_get_identity_details",
		Description: "Get the details of an identity in the cluster",
		InputSchema: &jsonschema.Schema{
			Type: "object",
			Properties: map[string]*jsonschema.Schema{
				"identity_id": {
					Type:        "string",
					Description: "The ID of the identity to get details for",
				},
				"node_name": {
					Type:        "string",
					Description: "The name of the node to get the identity details for",
				},
			},
			Required: []string{"identity_id"},
		},
	}, handleGetIdentityDetails)

	registerTool(&mcp.Tool{
		Name:        "cilium_request_debugging_information",
		Description: "Request debugging information for the cluster",
		InputSchema: &jsonschema.Schema{
			Type: "object",
			Properties: map[string]*jsonschema.Schema{
				"node_name": {
					Type:        "string",
					Description: "The name of the node to get the debugging information for",
				},
			},
		},
	}, handleRequestDebuggingInformation)

	registerTool(&mcp.Tool{
		Name:        "cilium_display_encryption_state",
		Description: "Display the encryption state for the cluster",
		InputSchema: &jsonschema.Schema{
			Type: "object",
			Properties: map[string]*jsonschema.Schema{
				"node_name": {
					Type:        "string",
					Description: "The name of the node to get the encryption state for",
				},
			},
		},
	}, handleDisplayEncryptionState)

	registerTool(&mcp.Tool{
		Name:        "cilium_flush_ipsec_state",
		Description: "Flush the IPsec state for the cluster",
		InputSchema: &jsonschema.Schema{
			Type: "object",
			Properties: map[string]*jsonschema.Schema{
				"node_name": {
					Type:        "string",
					Description: "The name of the node to flush the IPsec state for",
				},
			},
		},
	}, handleFlushIPsecState)

	registerTool(&mcp.Tool{
		Name:        "cilium_list_envoy_config",
		Description: "List the Envoy configuration for a resource in the cluster",
		InputSchema: &jsonschema.Schema{
			Type: "object",
			Properties: map[string]*jsonschema.Schema{
				"resource_name": {
					Type:        "string",
					Description: "The name of the resource to get the Envoy configuration for",
				},
				"node_name": {
					Type:        "string",
					Description: "The name of the node to get the Envoy configuration for",
				},
			},
			Required: []string{"resource_name"},
		},
	}, handleListEnvoyConfig)

	registerTool(&mcp.Tool{
		Name:        "cilium_fqdn_cache",
		Description: "Manage the FQDN cache for the cluster",
		InputSchema: &jsonschema.Schema{
			Type: "object",
			Properties: map[string]*jsonschema.Schema{
				"command": {
					Type:        "string",
					Description: "The command to perform on the FQDN cache (list, clean, or a specific command)",
				},
				"node_name": {
					Type:        "string",
					Description: "The name of the node to manage the FQDN cache for",
				},
			},
			Required: []string{"command"},
		},
	}, handleFQDNCache)

	registerTool(&mcp.Tool{
		Name:        "cilium_show_dns_names",
		Description: "Show the DNS names for the cluster",
		InputSchema: &jsonschema.Schema{
			Type: "object",
			Properties: map[string]*jsonschema.Schema{
				"node_name": {
					Type:        "string",
					Description: "The name of the node to get the DNS names for",
				},
			},
		},
	}, handleShowDNSNames)

	registerTool(&mcp.Tool{
		Name:        "cilium_list_ip_addresses",
		Description: "List the IP addresses for the cluster",
		InputSchema: &jsonschema.Schema{
			Type: "object",
			Properties: map[string]*jsonschema.Schema{
				"node_name": {
					Type:        "string",
					Description: "The name of the node to get the IP addresses for",
				},
			},
		},
	}, handleListIPAddresses)

	registerTool(&mcp.Tool{
		Name:        "cilium_show_ip_cache_information",
		Description: "Show the IP cache information for the cluster",
		InputSchema: &jsonschema.Schema{
			Type: "object",
			Properties: map[string]*jsonschema.Schema{
				"cidr": {
					Type:        "string",
					Description: "The CIDR of the IP to get cache information for",
				},
				"labels": {
					Type:        "string",
					Description: "The labels of the IP to get cache information for",
				},
				"node_name": {
					Type:        "string",
					Description: "The name of the node to get the IP cache information for",
				},
			},
		},
	}, handleShowIPCacheInformation)

	// Continue with kvstore, load, BPF, metrics, nodes, policy, and other tools
	registerTool(&mcp.Tool{
		Name:        "cilium_delete_key_from_kv_store",
		Description: "Delete a key from the kvstore for the cluster",
		InputSchema: &jsonschema.Schema{
			Type: "object",
			Properties: map[string]*jsonschema.Schema{
				"key": {
					Type:        "string",
					Description: "The key to delete from the kvstore",
				},
				"node_name": {
					Type:        "string",
					Description: "The name of the node to delete the key from",
				},
			},
			Required: []string{"key"},
		},
	}, handleDeleteKeyFromKVStore)

	registerTool(&mcp.Tool{
		Name:        "cilium_get_kv_store_key",
		Description: "Get a key from the kvstore for the cluster",
		InputSchema: &jsonschema.Schema{
			Type: "object",
			Properties: map[string]*jsonschema.Schema{
				"key": {
					Type:        "string",
					Description: "The key to get from the kvstore",
				},
				"node_name": {
					Type:        "string",
					Description: "The name of the node to get the key from",
				},
			},
			Required: []string{"key"},
		},
	}, handleGetKVStoreKey)

	registerTool(&mcp.Tool{
		Name:        "cilium_set_kv_store_key",
		Description: "Set a key in the kvstore for the cluster",
		InputSchema: &jsonschema.Schema{
			Type: "object",
			Properties: map[string]*jsonschema.Schema{
				"key": {
					Type:        "string",
					Description: "The key to set in the kvstore",
				},
				"value": {
					Type:        "string",
					Description: "The value to set in the kvstore",
				},
				"node_name": {
					Type:        "string",
					Description: "The name of the node to set the key in",
				},
			},
			Required: []string{"key", "value"},
		},
	}, handleSetKVStoreKey)

	registerTool(&mcp.Tool{
		Name:        "cilium_show_load_information",
		Description: "Show load information for the cluster",
		InputSchema: &jsonschema.Schema{
			Type: "object",
			Properties: map[string]*jsonschema.Schema{
				"node_name": {
					Type:        "string",
					Description: "The name of the node to get the load information for",
				},
			},
		},
	}, handleShowLoadInformation)

	registerTool(&mcp.Tool{
		Name:        "cilium_list_local_redirect_policies",
		Description: "List local redirect policies for the cluster",
		InputSchema: &jsonschema.Schema{
			Type: "object",
			Properties: map[string]*jsonschema.Schema{
				"node_name": {
					Type:        "string",
					Description: "The name of the node to get the local redirect policies for",
				},
			},
		},
	}, handleListLocalRedirectPolicies)

	registerTool(&mcp.Tool{
		Name:        "cilium_list_bpf_map_events",
		Description: "List BPF map events for the cluster",
		InputSchema: &jsonschema.Schema{
			Type: "object",
			Properties: map[string]*jsonschema.Schema{
				"map_name": {
					Type:        "string",
					Description: "The name of the BPF map to get events for",
				},
				"node_name": {
					Type:        "string",
					Description: "The name of the node to get the BPF map events for",
				},
			},
			Required: []string{"map_name"},
		},
	}, handleListBPFMapEvents)

	registerTool(&mcp.Tool{
		Name:        "cilium_get_bpf_map",
		Description: "Get BPF map for the cluster",
		InputSchema: &jsonschema.Schema{
			Type: "object",
			Properties: map[string]*jsonschema.Schema{
				"map_name": {
					Type:        "string",
					Description: "The name of the BPF map to get",
				},
				"node_name": {
					Type:        "string",
					Description: "The name of the node to get the BPF map for",
				},
			},
			Required: []string{"map_name"},
		},
	}, handleGetBPFMap)

	registerTool(&mcp.Tool{
		Name:        "cilium_list_bpf_maps",
		Description: "List BPF maps for the cluster",
		InputSchema: &jsonschema.Schema{
			Type: "object",
			Properties: map[string]*jsonschema.Schema{
				"node_name": {
					Type:        "string",
					Description: "The name of the node to get the BPF maps for",
				},
			},
		},
	}, handleListBPFMaps)

	registerTool(&mcp.Tool{
		Name:        "cilium_list_metrics",
		Description: "List metrics for the cluster",
		InputSchema: &jsonschema.Schema{
			Type: "object",
			Properties: map[string]*jsonschema.Schema{
				"match_pattern": {
					Type:        "string",
					Description: "The match pattern to filter metrics by",
				},
				"node_name": {
					Type:        "string",
					Description: "The name of the node to get the metrics for",
				},
			},
		},
	}, handleListMetrics)

	registerTool(&mcp.Tool{
		Name:        "cilium_list_cluster_nodes",
		Description: "List cluster nodes for the cluster",
		InputSchema: &jsonschema.Schema{
			Type: "object",
			Properties: map[string]*jsonschema.Schema{
				"node_name": {
					Type:        "string",
					Description: "The name of the node to get the cluster nodes for",
				},
			},
		},
	}, handleListClusterNodes)

	registerTool(&mcp.Tool{
		Name:        "cilium_list_node_ids",
		Description: "List node IDs for the cluster",
		InputSchema: &jsonschema.Schema{
			Type: "object",
			Properties: map[string]*jsonschema.Schema{
				"node_name": {
					Type:        "string",
					Description: "The name of the node to get the node IDs for",
				},
			},
		},
	}, handleListNodeIds)

	registerTool(&mcp.Tool{
		Name:        "cilium_display_policy_node_information",
		Description: "Display policy node information for the cluster",
		InputSchema: &jsonschema.Schema{
			Type: "object",
			Properties: map[string]*jsonschema.Schema{
				"labels": {
					Type:        "string",
					Description: "The labels to get policy node information for",
				},
				"node_name": {
					Type:        "string",
					Description: "The name of the node to get policy node information for",
				},
			},
		},
	}, handleDisplayPolicyNodeInformation)

	registerTool(&mcp.Tool{
		Name:        "cilium_delete_policy_rules",
		Description: "Delete policy rules for the cluster",
		InputSchema: &jsonschema.Schema{
			Type: "object",
			Properties: map[string]*jsonschema.Schema{
				"labels": {
					Type:        "string",
					Description: "The labels to delete policy rules for",
				},
				"all": {
					Type:        "string",
					Description: "Whether to delete all policy rules",
				},
				"node_name": {
					Type:        "string",
					Description: "The name of the node to delete policy rules for",
				},
			},
		},
	}, handleDeletePolicyRules)

	registerTool(&mcp.Tool{
		Name:        "cilium_display_selectors",
		Description: "Display selectors for the cluster",
		InputSchema: &jsonschema.Schema{
			Type: "object",
			Properties: map[string]*jsonschema.Schema{
				"node_name": {
					Type:        "string",
					Description: "The name of the node to get selectors for",
				},
			},
		},
	}, handleDisplaySelectors)

	registerTool(&mcp.Tool{
		Name:        "cilium_list_xdp_cidr_filters",
		Description: "List XDP CIDR filters for the cluster",
		InputSchema: &jsonschema.Schema{
			Type: "object",
			Properties: map[string]*jsonschema.Schema{
				"node_name": {
					Type:        "string",
					Description: "The name of the node to get the XDP CIDR filters for",
				},
			},
		},
	}, handleListXDPCIDRFilters)

	registerTool(&mcp.Tool{
		Name:        "cilium_update_xdp_cidr_filters",
		Description: "Update XDP CIDR filters for the cluster",
		InputSchema: &jsonschema.Schema{
			Type: "object",
			Properties: map[string]*jsonschema.Schema{
				"cidr_prefixes": {
					Type:        "string",
					Description: "The CIDR prefixes to update the XDP filters for",
				},
				"revision": {
					Type:        "string",
					Description: "The revision of the XDP filters to update",
				},
				"node_name": {
					Type:        "string",
					Description: "The name of the node to update the XDP filters for",
				},
			},
			Required: []string{"cidr_prefixes"},
		},
	}, handleUpdateXDPCIDRFilters)

	registerTool(&mcp.Tool{
		Name:        "cilium_delete_xdp_cidr_filters",
		Description: "Delete XDP CIDR filters for the cluster",
		InputSchema: &jsonschema.Schema{
			Type: "object",
			Properties: map[string]*jsonschema.Schema{
				"cidr_prefixes": {
					Type:        "string",
					Description: "The CIDR prefixes to delete the XDP filters for",
				},
				"revision": {
					Type:        "string",
					Description: "The revision of the XDP filters to delete",
				},
				"node_name": {
					Type:        "string",
					Description: "The name of the node to delete the XDP filters for",
				},
			},
			Required: []string{"cidr_prefixes"},
		},
	}, handleDeleteXDPCIDRFilters)

	registerTool(&mcp.Tool{
		Name:        "cilium_validate_cilium_network_policies",
		Description: "Validate Cilium network policies for the cluster",
		InputSchema: &jsonschema.Schema{
			Type: "object",
			Properties: map[string]*jsonschema.Schema{
				"enable_k8s": {
					Type:        "string",
					Description: "Whether to enable k8s API discovery",
				},
				"enable_k8s_api_discovery": {
					Type:        "string",
					Description: "Whether to enable k8s API discovery",
				},
				"node_name": {
					Type:        "string",
					Description: "The name of the node to validate the Cilium network policies for",
				},
			},
		},
	}, handleValidateCiliumNetworkPolicies)

	registerTool(&mcp.Tool{
		Name:        "cilium_list_pcap_recorders",
		Description: "List PCAP recorders for the cluster",
		InputSchema: &jsonschema.Schema{
			Type: "object",
			Properties: map[string]*jsonschema.Schema{
				"node_name": {
					Type:        "string",
					Description: "The name of the node to get the PCAP recorders for",
				},
			},
		},
	}, handleListPCAPRecorders)

	registerTool(&mcp.Tool{
		Name:        "cilium_get_pcap_recorder",
		Description: "Get a PCAP recorder for the cluster",
		InputSchema: &jsonschema.Schema{
			Type: "object",
			Properties: map[string]*jsonschema.Schema{
				"recorder_id": {
					Type:        "string",
					Description: "The ID of the PCAP recorder to get",
				},
				"node_name": {
					Type:        "string",
					Description: "The name of the node to get the PCAP recorder for",
				},
			},
			Required: []string{"recorder_id"},
		},
	}, handleGetPCAPRecorder)

	registerTool(&mcp.Tool{
		Name:        "cilium_delete_pcap_recorder",
		Description: "Delete a PCAP recorder for the cluster",
		InputSchema: &jsonschema.Schema{
			Type: "object",
			Properties: map[string]*jsonschema.Schema{
				"recorder_id": {
					Type:        "string",
					Description: "The ID of the PCAP recorder to delete",
				},
				"node_name": {
					Type:        "string",
					Description: "The name of the node to delete the PCAP recorder from",
				},
			},
			Required: []string{"recorder_id"},
		},
	}, handleDeletePCAPRecorder)

	registerTool(&mcp.Tool{
		Name:        "cilium_update_pcap_recorder",
		Description: "Update a PCAP recorder for the cluster",
		InputSchema: &jsonschema.Schema{
			Type: "object",
			Properties: map[string]*jsonschema.Schema{
				"recorder_id": {
					Type:        "string",
					Description: "The ID of the PCAP recorder to update",
				},
				"filters": {
					Type:        "string",
					Description: "The filters to update the PCAP recorder with",
				},
				"caplen": {
					Type:        "string",
					Description: "The caplen to update the PCAP recorder with",
				},
				"id": {
					Type:        "string",
					Description: "The id to update the PCAP recorder with",
				},
				"node_name": {
					Type:        "string",
					Description: "The name of the node to update the PCAP recorder on",
				},
			},
			Required: []string{"recorder_id", "filters"},
		},
	}, handleUpdatePCAPRecorder)

	return nil
}
