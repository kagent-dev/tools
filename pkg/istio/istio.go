package istio

import (
	"context"
	"fmt"
	"strings"

	"github.com/kagent-dev/tools/internal/commands"
	mcp "github.com/kagent-dev/tools/internal/mcp"
	"github.com/kagent-dev/tools/pkg/utils"
)

type istioProxyStatusInput struct {
	PodName   string `json:"pod_name" jsonschema:"Name of the pod to get proxy status for"`
	Namespace string `json:"namespace" jsonschema:"Namespace of the pod"`
}

// Istio proxy status
func handleIstioProxyStatus(ctx context.Context, request *mcp.CallToolRequest, in istioProxyStatusInput) (*mcp.CallToolResult, any, error) {
	args := []string{"proxy-status"}

	if in.Namespace != "" {
		args = append(args, "-n", in.Namespace)
	}

	if in.PodName != "" {
		args = append(args, in.PodName)
	}

	result, err := runIstioCtl(ctx, args)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("istioctl proxy-status failed: %v", err)), nil, nil
	}

	return mcp.NewToolResultText(result), nil, nil
}

func runIstioCtl(ctx context.Context, args []string) (string, error) {
	kubeconfigPath := utils.GetKubeconfig()
	return commands.NewCommandBuilder("istioctl").
		WithArgs(args...).
		WithKubeconfig(kubeconfigPath).
		Execute(ctx)
}

type istioProxyConfigInput struct {
	PodName    string `json:"pod_name" jsonschema:"Name of the pod to get proxy configuration for"`
	Namespace  string `json:"namespace" jsonschema:"Namespace of the pod"`
	ConfigType string `json:"config_type" jsonschema:"Type of configuration (all, bootstrap, cluster, ecds, listener, log, route, secret)"`
}

// Istio proxy config
func handleIstioProxyConfig(ctx context.Context, request *mcp.CallToolRequest, in istioProxyConfigInput) (*mcp.CallToolResult, any, error) {
	if in.ConfigType == "" {
		in.ConfigType = "all"
	}

	if in.PodName == "" {
		return mcp.NewToolResultError("pod_name parameter is required"), nil, nil
	}

	args := []string{"proxy-config", in.ConfigType}

	if in.Namespace != "" {
		args = append(args, fmt.Sprintf("%s.%s", in.PodName, in.Namespace))
	} else {
		args = append(args, in.PodName)
	}

	result, err := runIstioCtl(ctx, args)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("istioctl proxy-config failed: %v", err)), nil, nil
	}

	return mcp.NewToolResultText(result), nil, nil
}

type istioInstallInput struct {
	Profile string `json:"profile" jsonschema:"Istio configuration profile (ambient, default, demo, minimal, empty)"`
}

// Istio install
func handleIstioInstall(ctx context.Context, request *mcp.CallToolRequest, in istioInstallInput) (*mcp.CallToolResult, any, error) {
	if in.Profile == "" {
		in.Profile = "default"
	}

	args := []string{"install", "--set", fmt.Sprintf("profile=%s", in.Profile), "-y"}

	result, err := runIstioCtl(ctx, args)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("istioctl install failed: %v", err)), nil, nil
	}

	return mcp.NewToolResultText(result), nil, nil
}

type istioGenerateManifestInput struct {
	Profile string `json:"profile" jsonschema:"Istio configuration profile (ambient, default, demo, minimal, empty)"`
}

// Istio generate manifest
func handleIstioGenerateManifest(ctx context.Context, request *mcp.CallToolRequest, in istioGenerateManifestInput) (*mcp.CallToolResult, any, error) {
	if in.Profile == "" {
		in.Profile = "default"
	}

	args := []string{"manifest", "generate", "--set", fmt.Sprintf("profile=%s", in.Profile)}

	result, err := runIstioCtl(ctx, args)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("istioctl manifest generate failed: %v", err)), nil, nil
	}

	return mcp.NewToolResultText(result), nil, nil
}

type istioAnalyzeClusterConfigurationInput struct {
	Namespace     string `json:"namespace" jsonschema:"Namespace to analyze"`
	AllNamespaces bool   `json:"all_namespaces" jsonschema:"Analyze all namespaces"`
}

// Istio analyze
func handleIstioAnalyzeClusterConfiguration(ctx context.Context, request *mcp.CallToolRequest, in istioAnalyzeClusterConfigurationInput) (*mcp.CallToolResult, any, error) {
	args := []string{"analyze"}

	if in.AllNamespaces {
		args = append(args, "-A")
	} else if in.Namespace != "" {
		args = append(args, "-n", in.Namespace)
	}

	result, err := runIstioCtl(ctx, args)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("istioctl analyze failed: %v", err)), nil, nil
	}

	return mcp.NewToolResultText(result), nil, nil
}

type istioVersionInput struct {
	Short bool `json:"short" jsonschema:"Return short version output"`
}

// Istio version
func handleIstioVersion(ctx context.Context, request *mcp.CallToolRequest, in istioVersionInput) (*mcp.CallToolResult, any, error) {
	args := []string{"version"}

	if in.Short {
		args = append(args, "--short")
	}

	result, err := runIstioCtl(ctx, args)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("istioctl version failed: %v", err)), nil, nil
	}

	return mcp.NewToolResultText(result), nil, nil
}

type istioRemoteClustersInput struct{}

// Istio remote clusters
func handleIstioRemoteClusters(ctx context.Context, request *mcp.CallToolRequest, in istioRemoteClustersInput) (*mcp.CallToolResult, any, error) {
	args := []string{"remote-clusters"}

	result, err := runIstioCtl(ctx, args)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("istioctl remote-clusters failed: %v", err)), nil, nil
	}

	return mcp.NewToolResultText(result), nil, nil
}

type waypointListInput struct {
	Namespace     string `json:"namespace" jsonschema:"Namespace to list waypoints in"`
	AllNamespaces bool   `json:"all_namespaces" jsonschema:"List waypoints in all namespaces"`
}

// Waypoint list
func handleWaypointList(ctx context.Context, request *mcp.CallToolRequest, in waypointListInput) (*mcp.CallToolResult, any, error) {
	args := []string{"waypoint", "list"}

	if in.AllNamespaces {
		args = append(args, "-A")
	} else if in.Namespace != "" {
		args = append(args, "-n", in.Namespace)
	}

	result, err := runIstioCtl(ctx, args)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("istioctl waypoint list failed: %v", err)), nil, nil
	}

	return mcp.NewToolResultText(result), nil, nil
}

type waypointGenerateInput struct {
	Namespace   string `json:"namespace" jsonschema:"Namespace for the waypoint resource"`
	Name        string `json:"name" jsonschema:"Name of the waypoint resource"`
	TrafficType string `json:"traffic_type" jsonschema:"Traffic type for the waypoint (all, service, workload)"`
}

// Waypoint generate
func handleWaypointGenerate(ctx context.Context, request *mcp.CallToolRequest, in waypointGenerateInput) (*mcp.CallToolResult, any, error) {
	if in.Name == "" {
		in.Name = "waypoint"
	}
	if in.TrafficType == "" {
		in.TrafficType = "all"
	}

	if in.Namespace == "" {
		return mcp.NewToolResultError("namespace parameter is required"), nil, nil
	}

	args := []string{"waypoint", "generate"}

	if in.Name != "" {
		args = append(args, in.Name)
	}

	args = append(args, "-n", in.Namespace)

	if in.TrafficType != "" {
		args = append(args, "--for", in.TrafficType)
	}

	result, err := runIstioCtl(ctx, args)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("istioctl waypoint generate failed: %v", err)), nil, nil
	}

	return mcp.NewToolResultText(result), nil, nil
}

type waypointApplyInput struct {
	Namespace       string `json:"namespace" jsonschema:"Namespace to apply the waypoint in"`
	EnrollNamespace bool   `json:"enroll_namespace" jsonschema:"Enroll the namespace in the ambient mesh"`
}

// Waypoint apply
func handleWaypointApply(ctx context.Context, request *mcp.CallToolRequest, in waypointApplyInput) (*mcp.CallToolResult, any, error) {
	if in.Namespace == "" {
		return mcp.NewToolResultError("namespace parameter is required"), nil, nil
	}

	args := []string{"waypoint", "apply", "-n", in.Namespace}

	if in.EnrollNamespace {
		args = append(args, "--enroll-namespace")
	}

	result, err := runIstioCtl(ctx, args)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("istioctl waypoint apply failed: %v", err)), nil, nil
	}

	return mcp.NewToolResultText(result), nil, nil
}

type waypointDeleteInput struct {
	Namespace string `json:"namespace" jsonschema:"Namespace containing the waypoints to delete"`
	Names     string `json:"names" jsonschema:"Comma-separated list of waypoint names to delete"`
	All       bool   `json:"all" jsonschema:"Delete all waypoints in the namespace"`
}

// Waypoint delete
func handleWaypointDelete(ctx context.Context, request *mcp.CallToolRequest, in waypointDeleteInput) (*mcp.CallToolResult, any, error) {
	if in.Namespace == "" {
		return mcp.NewToolResultError("namespace parameter is required"), nil, nil
	}

	args := []string{"waypoint", "delete"}

	if in.All {
		args = append(args, "--all")
	} else if in.Names != "" {
		namesList := strings.Split(in.Names, ",")
		for _, name := range namesList {
			args = append(args, strings.TrimSpace(name))
		}
	}

	args = append(args, "-n", in.Namespace)

	result, err := runIstioCtl(ctx, args)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("istioctl waypoint delete failed: %v", err)), nil, nil
	}

	return mcp.NewToolResultText(result), nil, nil
}

type waypointStatusInput struct {
	Namespace string `json:"namespace" jsonschema:"Namespace of the waypoint"`
	Name      string `json:"name" jsonschema:"Name of the waypoint resource"`
}

// Waypoint status
func handleWaypointStatus(ctx context.Context, request *mcp.CallToolRequest, in waypointStatusInput) (*mcp.CallToolResult, any, error) {
	if in.Namespace == "" {
		return mcp.NewToolResultError("namespace parameter is required"), nil, nil
	}

	args := []string{"waypoint", "status"}

	if in.Name != "" {
		args = append(args, in.Name)
	}

	args = append(args, "-n", in.Namespace)

	result, err := runIstioCtl(ctx, args)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("istioctl waypoint status failed: %v", err)), nil, nil
	}

	return mcp.NewToolResultText(result), nil, nil
}

type ztunnelConfigInput struct {
	Namespace  string `json:"namespace" jsonschema:"Namespace to get ztunnel configuration for"`
	ConfigType string `json:"config_type" jsonschema:"Type of ztunnel configuration (all, workloads, services)"`
}

// Ztunnel config
func handleZtunnelConfig(ctx context.Context, request *mcp.CallToolRequest, in ztunnelConfigInput) (*mcp.CallToolResult, any, error) {
	if in.ConfigType == "" {
		in.ConfigType = "all"
	}

	args := []string{"ztunnel", "config", in.ConfigType}

	if in.Namespace != "" {
		args = append(args, "-n", in.Namespace)
	}

	result, err := runIstioCtl(ctx, args)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("istioctl ztunnel config failed: %v", err)), nil, nil
	}

	return mcp.NewToolResultText(result), nil, nil
}

// Register Istio tools
func RegisterTools(s *mcp.Server, readOnly bool) {
	// Read-only tools - always registered

	mcp.AddTool(s, "istio", &mcp.Tool{
		Name:        "istio_proxy_status",
		Description: "Get Envoy proxy status for pods, retrieves last sent and acknowledged xDS sync from Istiod to each Envoy in the mesh",
	}, handleIstioProxyStatus)

	mcp.AddTool(s, "istio", &mcp.Tool{
		Name:        "istio_proxy_config",
		Description: "Get specific proxy configuration for a single pod",
	}, handleIstioProxyConfig)

	mcp.AddTool(s, "istio", &mcp.Tool{
		Name:        "istio_generate_manifest",
		Description: "Generate Istio manifest for a given profile",
	}, handleIstioGenerateManifest)

	mcp.AddTool(s, "istio", &mcp.Tool{
		Name:        "istio_analyze_cluster_configuration",
		Description: "Analyze Istio cluster configuration for issues",
	}, handleIstioAnalyzeClusterConfiguration)

	mcp.AddTool(s, "istio", &mcp.Tool{
		Name:        "istio_version",
		Description: "Get Istio version information",
	}, handleIstioVersion)

	mcp.AddTool(s, "istio", &mcp.Tool{
		Name:        "istio_remote_clusters",
		Description: "List remote clusters registered with Istio",
	}, handleIstioRemoteClusters)

	mcp.AddTool(s, "istio", &mcp.Tool{
		Name:        "istio_list_waypoints",
		Description: "List all waypoints in the mesh",
	}, handleWaypointList)

	mcp.AddTool(s, "istio", &mcp.Tool{
		Name:        "istio_generate_waypoint",
		Description: "Generate a waypoint resource YAML",
	}, handleWaypointGenerate)

	mcp.AddTool(s, "istio", &mcp.Tool{
		Name:        "istio_waypoint_status",
		Description: "Get the status of a waypoint resource",
	}, handleWaypointStatus)

	mcp.AddTool(s, "istio", &mcp.Tool{
		Name:        "istio_ztunnel_config",
		Description: "Get the ztunnel configuration for a namespace",
	}, handleZtunnelConfig)

	// Write tools - only registered when write operations are enabled
	if !readOnly {
		mcp.AddTool(s, "istio", &mcp.Tool{
			Name:        "istio_install_istio",
			Description: "Install Istio with a specified configuration profile",
		}, handleIstioInstall)

		mcp.AddTool(s, "istio", &mcp.Tool{
			Name:        "istio_apply_waypoint",
			Description: "Apply a waypoint resource to the cluster",
		}, handleWaypointApply)

		mcp.AddTool(s, "istio", &mcp.Tool{
			Name:        "istio_delete_waypoint",
			Description: "Delete a waypoint resource from the cluster",
		}, handleWaypointDelete)
	}
}
