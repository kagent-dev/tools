package cilium

import (
	"testing"

	"github.com/kagent-dev/tools/internal/commands/commandstest"
)

func ciliumExec(args ...string) commandstest.Exec {
	return commandstest.Exec{Command: "cilium", Args: args, Output: "ok"}
}

// ciliumDbgExecs are the two kubectl calls behind a cilium-dbg tool: the
// node's cilium pod lookup and the exec into it. Both must authenticate as the
// caller.
func ciliumDbgExecs(nodeName string, dbgArgs ...string) []commandstest.Exec {
	const pod = "cilium-test"
	return []commandstest.Exec{
		{
			Command: "kubectl",
			Args:    []string{"get", "pods", "-n", "kube-system", "--selector=k8s-app=cilium", "--field-selector=spec.nodeName=" + nodeName, "-o", "jsonpath={.items[0].metadata.name}"},
			Output:  pod,
		},
		{
			Command: "kubectl",
			Args:    append([]string{"exec", "-n", "kube-system", pod, "--", "cilium-dbg"}, dbgArgs...),
			Output:  "ok",
		},
	}
}

// TestCiliumTokenPassthrough covers both ways cilium tools reach the cluster,
// the cilium CLI and cilium-dbg through kubectl exec, for read and write
// tools. Every cilium tool talks to the cluster, so none runs without a token
// under TOKEN_PASSTHROUGH.
func TestCiliumTokenPassthrough(t *testing.T) {
	enable := true
	cluster := []commandstest.ToolCall{
		commandstest.Call("cilium_status_and_version", handleCiliumStatusAndVersion, noInput{}, ciliumExec("status"), ciliumExec("version")),
		commandstest.Call("cilium_list_bgp_peers", handleListBGPPeers, noInput{}, ciliumExec("bgp", "peers")),
		commandstest.Call("cilium_install_cilium", handleInstallCilium, installCiliumInput{ClusterName: "test"}, ciliumExec("install", "--set", "cluster.name=test")),
		commandstest.Call("cilium_uninstall_cilium", handleUninstallCilium, noInput{}, ciliumExec("uninstall")),
		commandstest.Call("cilium_toggle_hubble", handleToggleHubble, enableToggleInput{Enable: &enable}, ciliumExec("hubble", "enable")),
		commandstest.Call("cilium_get_endpoints_list", handleGetEndpointsList, nodeNameInput{NodeName: "node-1"}, ciliumDbgExecs("node-1", "endpoint", "list")...),
		commandstest.Call("cilium_delete_service", handleDeleteService, deleteServiceInput{All: true, NodeName: "node-1"}, ciliumDbgExecs("node-1", "service", "delete", "--all")...),
	}
	commandstest.RunTokenPassthroughTests(t, cluster, nil)
}
