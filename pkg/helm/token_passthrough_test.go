package helm

import (
	"testing"

	"github.com/kagent-dev/tools/internal/commands/commandstest"
)

func helmExec(args ...string) commandstest.Exec {
	return commandstest.Exec{Command: "helm", Args: args, Output: "ok"}
}

// TestHelmTokenPassthrough covers every helm tool: those that reach the cluster
// authenticate as the caller under TOKEN_PASSTHROUGH; repo add and repo update
// only touch the local helm configuration and need no token.
func TestHelmTokenPassthrough(t *testing.T) {
	cluster := []commandstest.ToolCall{
		commandstest.Call("helm_list_releases", handleHelmListReleases, helmListReleasesInput{AllNamespaces: true}, helmExec("list", "-A")),
		commandstest.Call("helm_get_release", handleHelmGetRelease, helmGetReleaseInput{Name: "app", Namespace: "default"}, helmExec("get", "all", "app", "-n", "default")),
		commandstest.Call("helm_upgrade", handleHelmUpgradeRelease, helmUpgradeReleaseInput{Name: "app", Chart: "repo/chart", Namespace: "default"}, helmExec("upgrade", "app", "repo/chart", "-n", "default", "--timeout", "30s")),
		commandstest.Call("helm_uninstall", handleHelmUninstall, helmUninstallInput{Name: "app", Namespace: "default"}, helmExec("uninstall", "app", "-n", "default")),
	}
	local := []commandstest.ToolCall{
		commandstest.Call("helm_repo_add", handleHelmRepoAdd, helmRepoAddInput{Name: "example", URL: "https://charts.example.com"}, helmExec("repo", "add", "example", "https://charts.example.com")),
		commandstest.Call("helm_repo_update", handleHelmRepoUpdate, helmRepoUpdateInput{}, helmExec("repo", "update")),
	}
	commandstest.RunTokenPassthroughTests(t, cluster, local)
}
