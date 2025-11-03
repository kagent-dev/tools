package istio

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/kagent-dev/tools/internal/commands"
	"github.com/kagent-dev/tools/internal/logger"
	"github.com/kagent-dev/tools/pkg/utils"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Istio proxy status
func handleIstioProxyStatus(ctx context.Context, request *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var args map[string]interface{}
	if err := json.Unmarshal(request.Params.Arguments, &args); err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "failed to parse arguments"}},
			IsError: true,
		}, nil
	}

	podName := ""
	namespace := ""

	if val, ok := args["pod_name"].(string); ok {
		podName = val
	}
	if val, ok := args["namespace"].(string); ok {
		namespace = val
	}

	cmdArgs := []string{"proxy-status"}

	if namespace != "" {
		cmdArgs = append(cmdArgs, "-n", namespace)
	}

	if podName != "" {
		cmdArgs = append(cmdArgs, podName)
	}

	result, err := runIstioCtl(ctx, cmdArgs)
	if err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("istioctl proxy-status failed: %v", err)}},
			IsError: true,
		}, nil
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: result}},
	}, nil
}

func runIstioCtl(ctx context.Context, args []string) (string, error) {
	kubeconfigPath := utils.GetKubeconfig()
	return commands.NewCommandBuilder("istioctl").
		WithArgs(args...).
		WithKubeconfig(kubeconfigPath).
		Execute(ctx)
}

// Istio proxy config
func handleIstioProxyConfig(ctx context.Context, request *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var args map[string]interface{}
	if err := json.Unmarshal(request.Params.Arguments, &args); err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "failed to parse arguments"}},
			IsError: true,
		}, nil
	}

	podName := ""
	namespace := ""
	configType := "all"

	if val, ok := args["pod_name"].(string); ok {
		podName = val
	}
	if val, ok := args["namespace"].(string); ok {
		namespace = val
	}
	if val, ok := args["config_type"].(string); ok {
		configType = val
	}

	if podName == "" {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "pod_name parameter is required"}},
			IsError: true,
		}, nil
	}

	cmdArgs := []string{"proxy-config", configType}

	if namespace != "" {
		cmdArgs = append(cmdArgs, fmt.Sprintf("%s.%s", podName, namespace))
	} else {
		cmdArgs = append(cmdArgs, podName)
	}

	result, err := runIstioCtl(ctx, cmdArgs)
	if err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("istioctl proxy-config failed: %v", err)}},
			IsError: true,
		}, nil
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: result}},
	}, nil
}

// Istio install
func handleIstioInstall(ctx context.Context, request *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var args map[string]interface{}
	if err := json.Unmarshal(request.Params.Arguments, &args); err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "failed to parse arguments"}},
			IsError: true,
		}, nil
	}

	profile := "default"
	if val, ok := args["profile"].(string); ok {
		profile = val
	}

	cmdArgs := []string{"install", "--set", fmt.Sprintf("profile=%s", profile), "-y"}

	result, err := runIstioCtl(ctx, cmdArgs)
	if err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("istioctl install failed: %v", err)}},
			IsError: true,
		}, nil
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: result}},
	}, nil
}

// Istio generate manifest
func handleIstioGenerateManifest(ctx context.Context, request *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var args map[string]interface{}
	if err := json.Unmarshal(request.Params.Arguments, &args); err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "failed to parse arguments"}},
			IsError: true,
		}, nil
	}

	profile := "default"
	if val, ok := args["profile"].(string); ok {
		profile = val
	}

	cmdArgs := []string{"manifest", "generate", "--set", fmt.Sprintf("profile=%s", profile)}

	result, err := runIstioCtl(ctx, cmdArgs)
	if err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("istioctl manifest generate failed: %v", err)}},
			IsError: true,
		}, nil
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: result}},
	}, nil
}

// Istio analyze
func handleIstioAnalyzeClusterConfiguration(ctx context.Context, request *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var args map[string]interface{}
	if err := json.Unmarshal(request.Params.Arguments, &args); err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "failed to parse arguments"}},
			IsError: true,
		}, nil
	}

	namespace := ""
	allNamespaces := false

	if val, ok := args["namespace"].(string); ok {
		namespace = val
	}
	if val, ok := args["all_namespaces"].(string); ok {
		allNamespaces = val == "true"
	}

	cmdArgs := []string{"analyze"}

	if allNamespaces {
		cmdArgs = append(cmdArgs, "-A")
	} else if namespace != "" {
		cmdArgs = append(cmdArgs, "-n", namespace)
	}

	result, err := runIstioCtl(ctx, cmdArgs)
	if err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("istioctl analyze failed: %v", err)}},
			IsError: true,
		}, nil
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: result}},
	}, nil
}

// Istio version
func handleIstioVersion(ctx context.Context, request *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var args map[string]interface{}
	if err := json.Unmarshal(request.Params.Arguments, &args); err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "failed to parse arguments"}},
			IsError: true,
		}, nil
	}

	short := false
	if val, ok := args["short"].(string); ok {
		short = val == "true"
	}

	cmdArgs := []string{"version"}

	if short {
		cmdArgs = append(cmdArgs, "--short")
	}

	result, err := runIstioCtl(ctx, cmdArgs)
	if err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("istioctl version failed: %v", err)}},
			IsError: true,
		}, nil
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: result}},
	}, nil
}

// Istio remote clusters
func handleIstioRemoteClusters(ctx context.Context, request *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	cmdArgs := []string{"remote-clusters"}

	result, err := runIstioCtl(ctx, cmdArgs)
	if err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("istioctl remote-clusters failed: %v", err)}},
			IsError: true,
		}, nil
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: result}},
	}, nil
}

// Waypoint list
func handleWaypointList(ctx context.Context, request *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var args map[string]interface{}
	if err := json.Unmarshal(request.Params.Arguments, &args); err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "failed to parse arguments"}},
			IsError: true,
		}, nil
	}

	namespace := ""
	allNamespaces := false

	if val, ok := args["namespace"].(string); ok {
		namespace = val
	}
	if val, ok := args["all_namespaces"].(string); ok {
		allNamespaces = val == "true"
	}

	cmdArgs := []string{"waypoint", "list"}

	if allNamespaces {
		cmdArgs = append(cmdArgs, "-A")
	} else if namespace != "" {
		cmdArgs = append(cmdArgs, "-n", namespace)
	}

	result, err := runIstioCtl(ctx, cmdArgs)
	if err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("istioctl waypoint list failed: %v", err)}},
			IsError: true,
		}, nil
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: result}},
	}, nil
}

// Waypoint generate
func handleWaypointGenerate(ctx context.Context, request *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var args map[string]interface{}
	if err := json.Unmarshal(request.Params.Arguments, &args); err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "failed to parse arguments"}},
			IsError: true,
		}, nil
	}

	namespace := ""
	name := "waypoint"
	trafficType := "all"

	if val, ok := args["namespace"].(string); ok {
		namespace = val
	}
	if val, ok := args["name"].(string); ok {
		name = val
	}
	if val, ok := args["traffic_type"].(string); ok {
		trafficType = val
	}

	if namespace == "" {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "namespace parameter is required"}},
			IsError: true,
		}, nil
	}

	cmdArgs := []string{"waypoint", "generate"}

	if name != "" {
		cmdArgs = append(cmdArgs, name)
	}

	cmdArgs = append(cmdArgs, "-n", namespace)

	if trafficType != "" {
		cmdArgs = append(cmdArgs, "--for", trafficType)
	}

	result, err := runIstioCtl(ctx, cmdArgs)
	if err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("istioctl waypoint generate failed: %v", err)}},
			IsError: true,
		}, nil
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: result}},
	}, nil
}

// Waypoint apply
func handleWaypointApply(ctx context.Context, request *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var args map[string]interface{}
	if err := json.Unmarshal(request.Params.Arguments, &args); err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "failed to parse arguments"}},
			IsError: true,
		}, nil
	}

	namespace := ""
	enrollNamespace := false

	if val, ok := args["namespace"].(string); ok {
		namespace = val
	}
	if val, ok := args["enroll_namespace"].(string); ok {
		enrollNamespace = val == "true"
	}

	if namespace == "" {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "namespace parameter is required"}},
			IsError: true,
		}, nil
	}

	cmdArgs := []string{"waypoint", "apply", "-n", namespace}

	if enrollNamespace {
		cmdArgs = append(cmdArgs, "--enroll-namespace")
	}

	result, err := runIstioCtl(ctx, cmdArgs)
	if err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("istioctl waypoint apply failed: %v", err)}},
			IsError: true,
		}, nil
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: result}},
	}, nil
}

// Waypoint delete
func handleWaypointDelete(ctx context.Context, request *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var args map[string]interface{}
	if err := json.Unmarshal(request.Params.Arguments, &args); err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "failed to parse arguments"}},
			IsError: true,
		}, nil
	}

	namespace := ""
	names := ""
	all := false

	if val, ok := args["namespace"].(string); ok {
		namespace = val
	}
	if val, ok := args["names"].(string); ok {
		names = val
	}
	if val, ok := args["all"].(string); ok {
		all = val == "true"
	}

	if namespace == "" {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "namespace parameter is required"}},
			IsError: true,
		}, nil
	}

	cmdArgs := []string{"waypoint", "delete"}

	if all {
		cmdArgs = append(cmdArgs, "--all")
	} else if names != "" {
		namesList := strings.Split(names, ",")
		for _, name := range namesList {
			cmdArgs = append(cmdArgs, strings.TrimSpace(name))
		}
	}

	cmdArgs = append(cmdArgs, "-n", namespace)

	result, err := runIstioCtl(ctx, cmdArgs)
	if err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("istioctl waypoint delete failed: %v", err)}},
			IsError: true,
		}, nil
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: result}},
	}, nil
}

// Waypoint status
func handleWaypointStatus(ctx context.Context, request *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var args map[string]interface{}
	if err := json.Unmarshal(request.Params.Arguments, &args); err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "failed to parse arguments"}},
			IsError: true,
		}, nil
	}

	namespace := ""
	name := ""

	if val, ok := args["namespace"].(string); ok {
		namespace = val
	}
	if val, ok := args["name"].(string); ok {
		name = val
	}

	if namespace == "" {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "namespace parameter is required"}},
			IsError: true,
		}, nil
	}

	cmdArgs := []string{"waypoint", "status"}

	if name != "" {
		cmdArgs = append(cmdArgs, name)
	}

	cmdArgs = append(cmdArgs, "-n", namespace)

	result, err := runIstioCtl(ctx, cmdArgs)
	if err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("istioctl waypoint status failed: %v", err)}},
			IsError: true,
		}, nil
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: result}},
	}, nil
}

// Ztunnel config
func handleZtunnelConfig(ctx context.Context, request *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var args map[string]interface{}
	if err := json.Unmarshal(request.Params.Arguments, &args); err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "failed to parse arguments"}},
			IsError: true,
		}, nil
	}

	namespace := ""
	configType := "all"

	if val, ok := args["namespace"].(string); ok {
		namespace = val
	}
	if val, ok := args["config_type"].(string); ok {
		configType = val
	}

	cmdArgs := []string{"ztunnel", "config", configType}

	if namespace != "" {
		cmdArgs = append(cmdArgs, "-n", namespace)
	}

	result, err := runIstioCtl(ctx, cmdArgs)
	if err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("istioctl ztunnel config failed: %v", err)}},
			IsError: true,
		}, nil
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: result}},
	}, nil
}

// ToolRegistry is an interface for tool registration (to avoid import cycles)
type ToolRegistry interface {
	Register(tool *mcp.Tool, handler mcp.ToolHandler)
}

// RegisterTools registers Istio tools with the MCP server
func RegisterTools(s *mcp.Server) error {
	return RegisterToolsWithRegistry(s, nil)
}

// RegisterToolsWithRegistry registers Istio tools with the MCP server and optionally with a tool registry
func RegisterToolsWithRegistry(s *mcp.Server, registry ToolRegistry) error {
	logger.Get().Info("RegisterTools initialized")
	
	// Helper function to register tool with both server and registry
	registerTool := func(tool *mcp.Tool, handler mcp.ToolHandler) {
		s.AddTool(tool, handler)
		if registry != nil {
			registry.Register(tool, handler)
		}
	}

	// Istio proxy status
	registerTool(&mcp.Tool{
		Name:        "istio_proxy_status",
		Description: "Get Envoy proxy status for pods, retrieves last sent and acknowledged xDS sync from Istiod to each Envoy in the mesh",
		InputSchema: &jsonschema.Schema{
			Type: "object",
			Properties: map[string]*jsonschema.Schema{
				"pod_name": {
					Type:        "string",
					Description: "Name of the pod to get proxy status for",
				},
				"namespace": {
					Type:        "string",
					Description: "Namespace of the pod",
				},
			},
		},
	}, handleIstioProxyStatus)

	// Istio proxy config
	registerTool(&mcp.Tool{
		Name:        "istio_proxy_config",
		Description: "Get specific proxy configuration for a single pod",
		InputSchema: &jsonschema.Schema{
			Type: "object",
			Properties: map[string]*jsonschema.Schema{
				"pod_name": {
					Type:        "string",
					Description: "Name of the pod to get proxy configuration for",
				},
				"namespace": {
					Type:        "string",
					Description: "Namespace of the pod",
				},
				"config_type": {
					Type:        "string",
					Description: "Type of configuration (all, bootstrap, cluster, ecds, listener, log, route, secret)",
				},
			},
			Required: []string{"pod_name"},
		},
	}, handleIstioProxyConfig)

	// Istio install
	registerTool(&mcp.Tool{
		Name:        "istio_install_istio",
		Description: "Install Istio with a specified configuration profile",
		InputSchema: &jsonschema.Schema{
			Type: "object",
			Properties: map[string]*jsonschema.Schema{
				"profile": {
					Type:        "string",
					Description: "Istio configuration profile (ambient, default, demo, minimal, empty)",
				},
			},
		},
	}, handleIstioInstall)

	// Istio generate manifest
	registerTool(&mcp.Tool{
		Name:        "istio_generate_manifest",
		Description: "Generate Istio manifest for a given profile",
		InputSchema: &jsonschema.Schema{
			Type: "object",
			Properties: map[string]*jsonschema.Schema{
				"profile": {
					Type:        "string",
					Description: "Istio configuration profile (ambient, default, demo, minimal, empty)",
				},
			},
		},
	}, handleIstioGenerateManifest)

	// Istio analyze
	registerTool(&mcp.Tool{
		Name:        "istio_analyze_cluster_configuration",
		Description: "Analyze Istio cluster configuration for issues",
		InputSchema: &jsonschema.Schema{
			Type: "object",
			Properties: map[string]*jsonschema.Schema{
				"namespace": {
					Type:        "string",
					Description: "Namespace to analyze",
				},
				"all_namespaces": {
					Type:        "string",
					Description: "Analyze all namespaces (true/false)",
				},
			},
		},
	}, handleIstioAnalyzeClusterConfiguration)

	// Istio version
	registerTool(&mcp.Tool{
		Name:        "istio_version",
		Description: "Get Istio version information",
		InputSchema: &jsonschema.Schema{
			Type: "object",
			Properties: map[string]*jsonschema.Schema{
				"short": {
					Type:        "string",
					Description: "Show short version (true/false)",
				},
			},
		},
	}, handleIstioVersion)

	// Istio remote clusters
	registerTool(&mcp.Tool{
		Name:        "istio_remote_clusters",
		Description: "List remote clusters registered with Istio",
		InputSchema: &jsonschema.Schema{
			Type:       "object",
			Properties: map[string]*jsonschema.Schema{},
		},
	}, handleIstioRemoteClusters)

	// Waypoint list
	registerTool(&mcp.Tool{
		Name:        "istio_list_waypoints",
		Description: "List all waypoints in the mesh",
		InputSchema: &jsonschema.Schema{
			Type: "object",
			Properties: map[string]*jsonschema.Schema{
				"namespace": {
					Type:        "string",
					Description: "Namespace to list waypoints from",
				},
				"all_namespaces": {
					Type:        "string",
					Description: "List waypoints from all namespaces (true/false)",
				},
			},
		},
	}, handleWaypointList)

	// Waypoint generate
	registerTool(&mcp.Tool{
		Name:        "istio_generate_waypoint",
		Description: "Generate a waypoint resource YAML",
		InputSchema: &jsonschema.Schema{
			Type: "object",
			Properties: map[string]*jsonschema.Schema{
				"namespace": {
					Type:        "string",
					Description: "Namespace for the waypoint",
				},
				"name": {
					Type:        "string",
					Description: "Name of the waypoint",
				},
				"traffic_type": {
					Type:        "string",
					Description: "Traffic type for the waypoint (all, service, workload)",
				},
			},
			Required: []string{"namespace"},
		},
	}, handleWaypointGenerate)

	// Waypoint apply
	registerTool(&mcp.Tool{
		Name:        "istio_apply_waypoint",
		Description: "Apply a waypoint resource to the cluster",
		InputSchema: &jsonschema.Schema{
			Type: "object",
			Properties: map[string]*jsonschema.Schema{
				"namespace": {
					Type:        "string",
					Description: "Namespace for the waypoint",
				},
				"enroll_namespace": {
					Type:        "string",
					Description: "Enroll the namespace to use the waypoint (true/false)",
				},
			},
			Required: []string{"namespace"},
		},
	}, handleWaypointApply)

	// Waypoint delete
	registerTool(&mcp.Tool{
		Name:        "istio_delete_waypoint",
		Description: "Delete a waypoint resource from the cluster",
		InputSchema: &jsonschema.Schema{
			Type: "object",
			Properties: map[string]*jsonschema.Schema{
				"namespace": {
					Type:        "string",
					Description: "Namespace of the waypoint",
				},
				"names": {
					Type:        "string",
					Description: "Comma-separated list of waypoint names to delete",
				},
				"all": {
					Type:        "string",
					Description: "Delete all waypoints in the namespace (true/false)",
				},
			},
			Required: []string{"namespace"},
		},
	}, handleWaypointDelete)

	// Waypoint status
	registerTool(&mcp.Tool{
		Name:        "istio_waypoint_status",
		Description: "Get the status of a waypoint resource",
		InputSchema: &jsonschema.Schema{
			Type: "object",
			Properties: map[string]*jsonschema.Schema{
				"namespace": {
					Type:        "string",
					Description: "Namespace of the waypoint",
				},
				"name": {
					Type:        "string",
					Description: "Name of the waypoint",
				},
			},
			Required: []string{"namespace"},
		},
	}, handleWaypointStatus)

	// Ztunnel config
	registerTool(&mcp.Tool{
		Name:        "istio_ztunnel_config",
		Description: "Get the ztunnel configuration for a namespace",
		InputSchema: &jsonschema.Schema{
			Type: "object",
			Properties: map[string]*jsonschema.Schema{
				"namespace": {
					Type:        "string",
					Description: "Namespace to get ztunnel config for",
				},
				"config_type": {
					Type:        "string",
					Description: "Type of configuration (all, workload, service, policy)",
				},
			},
		},
	}, handleZtunnelConfig)

	return nil
}
