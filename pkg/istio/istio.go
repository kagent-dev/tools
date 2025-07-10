package istio

import (
	"context"
	"fmt"
	"strings"

	"github.com/kagent-dev/tools/internal/commands"
	"github.com/kagent-dev/tools/internal/telemetry"
	"github.com/kagent-dev/tools/pkg/utils"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// Istio proxy status
func handleIstioProxyStatus(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	podName := mcp.ParseString(request, "pod_name", "")
	namespace := mcp.ParseString(request, "namespace", "")

	args := []string{"proxy-status"}

	if namespace != "" {
		args = append(args, "-n", namespace)
	}

	if podName != "" {
		args = append(args, podName)
	}

	result, err := runIstioCtl(ctx, args)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("istioctl proxy-status failed: %v", err)), nil
	}

	return mcp.NewToolResultText(result), nil
}

func runIstioCtl(ctx context.Context, args []string) (string, error) {
	kubeconfigPath := utils.GetKubeconfig()
	return commands.NewCommandBuilder("istioctl").
		WithArgs(args...).
		WithKubeconfig(kubeconfigPath).
		Execute(ctx)
}

// Istio proxy config
func handleIstioProxyConfig(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	podName := mcp.ParseString(request, "pod_name", "")
	namespace := mcp.ParseString(request, "namespace", "")
	configType := mcp.ParseString(request, "config_type", "all")

	if podName == "" {
		return mcp.NewToolResultError("pod_name parameter is required"), nil
	}

	args := []string{"proxy-config", configType}

	if namespace != "" {
		args = append(args, fmt.Sprintf("%s.%s", podName, namespace))
	} else {
		args = append(args, podName)
	}

	result, err := runIstioCtl(ctx, args)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("istioctl proxy-config failed: %v", err)), nil
	}

	return mcp.NewToolResultText(result), nil
}

// Istio install
func handleIstioInstall(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	profile := mcp.ParseString(request, "profile", "default")

	args := []string{"install", "--set", fmt.Sprintf("profile=%s", profile), "-y"}

	result, err := runIstioCtl(ctx, args)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("istioctl install failed: %v", err)), nil
	}

	return mcp.NewToolResultText(result), nil
}

// Istio generate manifest
func handleIstioGenerateManifest(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	profile := mcp.ParseString(request, "profile", "default")

	args := []string{"manifest", "generate", "--set", fmt.Sprintf("profile=%s", profile)}

	result, err := runIstioCtl(ctx, args)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("istioctl manifest generate failed: %v", err)), nil
	}

	return mcp.NewToolResultText(result), nil
}

// Istio analyze
func handleIstioAnalyzeClusterConfiguration(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	namespace := mcp.ParseString(request, "namespace", "")
	allNamespaces := mcp.ParseString(request, "all_namespaces", "") == "true"

	args := []string{"analyze"}

	if allNamespaces {
		args = append(args, "-A")
	} else if namespace != "" {
		args = append(args, "-n", namespace)
	}

	result, err := runIstioCtl(ctx, args)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("istioctl analyze failed: %v", err)), nil
	}

	return mcp.NewToolResultText(result), nil
}

// Istio version
func handleIstioVersion(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	short := mcp.ParseString(request, "short", "") == "true"

	args := []string{"version"}

	if short {
		args = append(args, "--short")
	}

	result, err := runIstioCtl(ctx, args)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("istioctl version failed: %v", err)), nil
	}

	return mcp.NewToolResultText(result), nil
}

// Istio remote clusters
func handleIstioRemoteClusters(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := []string{"remote-clusters"}

	result, err := runIstioCtl(ctx, args)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("istioctl remote-clusters failed: %v", err)), nil
	}

	return mcp.NewToolResultText(result), nil
}

// Waypoint list
func handleWaypointList(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	namespace := mcp.ParseString(request, "namespace", "")
	allNamespaces := mcp.ParseString(request, "all_namespaces", "") == "true"

	args := []string{"waypoint", "list"}

	if allNamespaces {
		args = append(args, "-A")
	} else if namespace != "" {
		args = append(args, "-n", namespace)
	}

	result, err := runIstioCtl(ctx, args)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("istioctl waypoint list failed: %v", err)), nil
	}

	return mcp.NewToolResultText(result), nil
}

// Waypoint generate
func handleWaypointGenerate(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	namespace := mcp.ParseString(request, "namespace", "")
	name := mcp.ParseString(request, "name", "waypoint")
	trafficType := mcp.ParseString(request, "traffic_type", "all")

	if namespace == "" {
		return mcp.NewToolResultError("namespace parameter is required"), nil
	}

	args := []string{"waypoint", "generate"}

	if name != "" {
		args = append(args, name)
	}

	args = append(args, "-n", namespace)

	if trafficType != "" {
		args = append(args, "--for", trafficType)
	}

	result, err := runIstioCtl(ctx, args)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("istioctl waypoint generate failed: %v", err)), nil
	}

	return mcp.NewToolResultText(result), nil
}

// Waypoint apply
func handleWaypointApply(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	namespace := mcp.ParseString(request, "namespace", "")
	enrollNamespace := mcp.ParseString(request, "enroll_namespace", "") == "true"

	if namespace == "" {
		return mcp.NewToolResultError("namespace parameter is required"), nil
	}

	args := []string{"waypoint", "apply", "-n", namespace}

	if enrollNamespace {
		args = append(args, "--enroll-namespace")
	}

	result, err := runIstioCtl(ctx, args)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("istioctl waypoint apply failed: %v", err)), nil
	}

	return mcp.NewToolResultText(result), nil
}

// Waypoint delete
func handleWaypointDelete(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	namespace := mcp.ParseString(request, "namespace", "")
	names := mcp.ParseString(request, "names", "")
	all := mcp.ParseString(request, "all", "") == "true"

	if namespace == "" {
		return mcp.NewToolResultError("namespace parameter is required"), nil
	}

	args := []string{"waypoint", "delete"}

	if all {
		args = append(args, "--all")
	} else if names != "" {
		namesList := strings.Split(names, ",")
		for _, name := range namesList {
			args = append(args, strings.TrimSpace(name))
		}
	}

	args = append(args, "-n", namespace)

	result, err := runIstioCtl(ctx, args)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("istioctl waypoint delete failed: %v", err)), nil
	}

	return mcp.NewToolResultText(result), nil
}

// Waypoint status
func handleWaypointStatus(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	namespace := mcp.ParseString(request, "namespace", "")
	name := mcp.ParseString(request, "name", "")

	if namespace == "" {
		return mcp.NewToolResultError("namespace parameter is required"), nil
	}

	args := []string{"waypoint", "status"}

	if name != "" {
		args = append(args, name)
	}

	args = append(args, "-n", namespace)

	result, err := runIstioCtl(ctx, args)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("istioctl waypoint status failed: %v", err)), nil
	}

	return mcp.NewToolResultText(result), nil
}

// Ztunnel config
func handleZtunnelConfig(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	namespace := mcp.ParseString(request, "namespace", "")
	configType := mcp.ParseString(request, "config_type", "all")

	args := []string{"ztunnel", "config", configType}

	if namespace != "" {
		args = append(args, "-n", namespace)
	}

	result, err := runIstioCtl(ctx, args)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("istioctl ztunnel config failed: %v", err)), nil
	}

	return mcp.NewToolResultText(result), nil
}

// Register Istio tools
func RegisterTools(s *server.MCPServer) {

	// Istio proxy status
	s.AddTool(mcp.NewTool("istio_proxy_status",
		mcp.WithDescription("Get Envoy proxy status for pods, retrieves last sent and acknowledged xDS sync from Istiod to each Envoy in the mesh"),
		mcp.WithString("pod_name", mcp.Description("Name of the pod to get proxy status for")),
		mcp.WithString("namespace", mcp.Description("Namespace of the pod")),
	), telemetry.AdaptToolHandler(telemetry.WithTracing("istio_proxy_status", handleIstioProxyStatus)))

	// Istio proxy config
	s.AddTool(mcp.NewTool("istio_proxy_config",
		mcp.WithDescription("Get specific proxy configuration for a single pod"),
		mcp.WithString("pod_name", mcp.Description("Name of the pod to get proxy configuration for"), mcp.Required()),
		mcp.WithString("namespace", mcp.Description("Namespace of the pod")),
		mcp.WithString("config_type", mcp.Description("Type of configuration (all, bootstrap, cluster, ecds, listener, log, route, secret)")),
	), telemetry.AdaptToolHandler(telemetry.WithTracing("istio_proxy_config", handleIstioProxyConfig)))

	// Istio install
	s.AddTool(mcp.NewTool("istio_install_istio",
		mcp.WithDescription("Install Istio with a specified configuration profile"),
		mcp.WithString("profile", mcp.Description("Istio configuration profile (ambient, default, demo, minimal, empty)")),
	), telemetry.AdaptToolHandler(telemetry.WithTracing("istio_install_istio", handleIstioInstall)))

	// Istio generate manifest
	s.AddTool(mcp.NewTool("istio_generate_manifest",
		mcp.WithDescription("Generate Istio manifest for a given profile"),
		mcp.WithString("profile", mcp.Description("Istio configuration profile (ambient, default, demo, minimal, empty)")),
	), telemetry.AdaptToolHandler(telemetry.WithTracing("istio_generate_manifest", handleIstioGenerateManifest)))

	// Istio analyze
	s.AddTool(mcp.NewTool("istio_analyze_cluster_configuration",
		mcp.WithDescription("Analyze Istio cluster configuration for issues"),
	), telemetry.AdaptToolHandler(telemetry.WithTracing("istio_analyze_cluster_configuration", handleIstioAnalyzeClusterConfiguration)))

	// Istio version
	s.AddTool(mcp.NewTool("istio_version",
		mcp.WithDescription("Get Istio version information"),
	), telemetry.AdaptToolHandler(telemetry.WithTracing("istio_version", handleIstioVersion)))

	// Istio remote clusters
	s.AddTool(mcp.NewTool("istio_remote_clusters",
		mcp.WithDescription("List remote clusters registered with Istio"),
	), telemetry.AdaptToolHandler(telemetry.WithTracing("istio_remote_clusters", handleIstioRemoteClusters)))

	// Waypoint list
	s.AddTool(mcp.NewTool("istio_list_waypoints",
		mcp.WithDescription("List all waypoints in the mesh"),
	), telemetry.AdaptToolHandler(telemetry.WithTracing("istio_list_waypoints", handleWaypointList)))

	// Waypoint generate
	s.AddTool(mcp.NewTool("istio_generate_waypoint",
		mcp.WithDescription("Generate a waypoint resource YAML"),
	), telemetry.AdaptToolHandler(telemetry.WithTracing("istio_generate_waypoint", handleWaypointGenerate)))

	// Waypoint apply
	s.AddTool(mcp.NewTool("istio_apply_waypoint",
		mcp.WithDescription("Apply a waypoint resource to the cluster"),
	), telemetry.AdaptToolHandler(telemetry.WithTracing("istio_apply_waypoint", handleWaypointApply)))

	// Waypoint delete
	s.AddTool(mcp.NewTool("istio_delete_waypoint",
		mcp.WithDescription("Delete a waypoint resource from the cluster"),
	), telemetry.AdaptToolHandler(telemetry.WithTracing("istio_delete_waypoint", handleWaypointDelete)))

	// Waypoint status
	s.AddTool(mcp.NewTool("istio_waypoint_status",
		mcp.WithDescription("Get the status of a waypoint resource"),
	), telemetry.AdaptToolHandler(telemetry.WithTracing("istio_waypoint_status", handleWaypointStatus)))

	// Ztunnel config
	s.AddTool(mcp.NewTool("istio_ztunnel_config",
		mcp.WithDescription("Get the ztunnel configuration for a namespace"),
	), telemetry.AdaptToolHandler(telemetry.WithTracing("istio_ztunnel_config", handleZtunnelConfig)))
}
