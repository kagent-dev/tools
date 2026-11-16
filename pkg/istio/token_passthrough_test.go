package istio

import (
	"testing"

	"github.com/kagent-dev/tools/internal/commands/commandstest"
)

func istioctlExec(args ...string) commandstest.Exec {
	return commandstest.Exec{Command: "istioctl", Args: args, Output: "ok"}
}

// TestIstioTokenPassthrough covers every istio tool: those that reach the
// cluster authenticate as the caller under TOKEN_PASSTHROUGH; manifest
// generate and waypoint generate only render YAML and need no token.
func TestIstioTokenPassthrough(t *testing.T) {
	cluster := []commandstest.ToolCall{
		commandstest.Call("istio_proxy_status", handleIstioProxyStatus, istioProxyStatusInput{}, istioctlExec("proxy-status")),
		commandstest.Call("istio_proxy_config", handleIstioProxyConfig, istioProxyConfigInput{PodName: "pod", Namespace: "default"}, istioctlExec("proxy-config", "all", "pod.default")),
		commandstest.Call("istio_analyze_cluster_configuration", handleIstioAnalyzeClusterConfiguration, istioAnalyzeClusterConfigurationInput{}, istioctlExec("analyze")),
		commandstest.Call("istio_version", handleIstioVersion, istioVersionInput{}, istioctlExec("version")),
		commandstest.Call("istio_remote_clusters", handleIstioRemoteClusters, istioRemoteClustersInput{}, istioctlExec("remote-clusters")),
		commandstest.Call("istio_list_waypoints", handleWaypointList, waypointListInput{}, istioctlExec("waypoint", "list")),
		commandstest.Call("istio_waypoint_status", handleWaypointStatus, waypointStatusInput{Namespace: "default"}, istioctlExec("waypoint", "status", "-n", "default")),
		commandstest.Call("istio_ztunnel_config", handleZtunnelConfig, ztunnelConfigInput{}, istioctlExec("ztunnel", "config", "all")),
		commandstest.Call("istio_install_istio", handleIstioInstall, istioInstallInput{}, istioctlExec("install", "--set", "profile=default", "-y")),
		commandstest.Call("istio_apply_waypoint", handleWaypointApply, waypointApplyInput{Namespace: "default"}, istioctlExec("waypoint", "apply", "-n", "default")),
		commandstest.Call("istio_delete_waypoint", handleWaypointDelete, waypointDeleteInput{Namespace: "default", All: true}, istioctlExec("waypoint", "delete", "--all", "-n", "default")),
	}
	local := []commandstest.ToolCall{
		commandstest.Call("istio_generate_manifest", handleIstioGenerateManifest, istioGenerateManifestInput{}, istioctlExec("manifest", "generate", "--set", "profile=default")),
		commandstest.Call("istio_generate_waypoint", handleWaypointGenerate, waypointGenerateInput{Namespace: "default"}, istioctlExec("waypoint", "generate", "waypoint", "-n", "default", "--for", "all")),
	}
	commandstest.RunTokenPassthroughTests(t, cluster, local)
}
