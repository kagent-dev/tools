package argo

import (
	"testing"

	"github.com/kagent-dev/tools/internal/commands/commandstest"
)

func kubectlExec(output string, args ...string) commandstest.Exec {
	return commandstest.Exec{Command: "kubectl", Args: args, Output: output}
}

// TestArgoTokenPassthrough covers every argo tool: those that reach the cluster
// authenticate as the caller under TOKEN_PASSTHROUGH; the plugin version check
// only runs the local kubectl plugin and needs no token.
func TestArgoTokenPassthrough(t *testing.T) {
	noInstall := false
	cluster := []commandstest.ToolCall{
		commandstest.Call("argo_verify_argo_rollouts_controller_install", handleVerifyArgoRolloutsControllerInstall, verifyArgoRolloutsControllerInstallInput{},
			kubectlExec("Running", "get", "pods", "-n", "argo-rollouts", "-l", "app.kubernetes.io/component=rollouts-controller", "-o", "jsonpath={.items[*].status.phase}")),
		commandstest.Call("argo_rollouts_list", handleListRollouts, listRolloutsInput{},
			kubectlExec("ok", "argo", "rollouts", "list", "rollouts", "-n", "argo-rollouts")),
		commandstest.Call("argo_check_plugin_logs", handleCheckPluginLogs, checkPluginLogsInput{},
			kubectlExec("ok", "logs", "-n", "argo-rollouts", "-l", "app.kubernetes.io/name=argo-rollouts", "--tail", "100")),
		commandstest.Call("argo_promote_rollout", handlePromoteRollout, promoteRolloutInput{RolloutName: "app"},
			kubectlExec("ok", "argo", "rollouts", "promote", "app")),
		commandstest.Call("argo_pause_rollout", handlePauseRollout, pauseRolloutInput{RolloutName: "app"},
			kubectlExec("ok", "argo", "rollouts", "pause", "app")),
		commandstest.Call("argo_set_rollout_image", handleSetRolloutImage, setRolloutImageInput{RolloutName: "app", ContainerImage: "app=nginx:1.27"},
			kubectlExec("ok", "argo", "rollouts", "set", "image", "app", "app=nginx:1.27")),
		commandstest.Call("argo_verify_gateway_plugin", handleVerifyGatewayPlugin, verifyGatewayPluginInput{ShouldInstall: &noInstall},
			kubectlExec("trafficRouterPlugins: argoproj-labs/gatewayAPI", "get", "configmap", "argo-rollouts-config", "-n", "argo-rollouts", "-o", "yaml")),
	}
	local := []commandstest.ToolCall{
		commandstest.Call("argo_verify_kubectl_plugin_install", handleVerifyKubectlPluginInstall, verifyKubectlPluginInstallInput{},
			kubectlExec("kubectl-argo-rollouts: v1.8.0", "argo", "rollouts", "version")),
	}
	commandstest.RunTokenPassthroughTests(t, cluster, local)
}
