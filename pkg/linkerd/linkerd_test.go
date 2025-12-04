package linkerd

import (
	"context"
	"testing"

	"github.com/kagent-dev/tools/internal/cmd"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRegisterTools(t *testing.T) {
	s := server.NewMCPServer("test-server", "v0.0.1")
	RegisterTools(s)
}

func TestHandleLinkerdCheck(t *testing.T) {
	ctx := context.Background()

	t.Run("basic check", func(t *testing.T) {
		mock := cmd.NewMockShellExecutor()
		mock.AddCommandString("linkerd", []string{"check"}, "ok", nil)

		ctx = cmd.WithShellExecutor(ctx, mock)

		result, err := handleLinkerdCheck(ctx, mcp.CallToolRequest{})
		require.NoError(t, err)
		assert.NotNil(t, result)
		assert.False(t, result.IsError)
	})

	t.Run("pre proxy check", func(t *testing.T) {
		mock := cmd.NewMockShellExecutor()
		mock.AddCommandString("linkerd", []string{"check", "--pre", "--proxy", "-n", "linkerd", "--wait", "60s", "--output", "short"}, "ok", nil)

		ctx = cmd.WithShellExecutor(ctx, mock)

		request := mcp.CallToolRequest{}
		request.Params.Arguments = map[string]interface{}{
			"pre_check":   "true",
			"proxy_check": "true",
			"namespace":   "linkerd",
			"wait":        "60s",
			"output":      "short",
		}

		result, err := handleLinkerdCheck(ctx, request)
		require.NoError(t, err)
		assert.NotNil(t, result)
		assert.False(t, result.IsError)
	})
}

func TestHandleLinkerdInstall(t *testing.T) {
	ctx := context.Background()

	t.Run("default install", func(t *testing.T) {
		mock := cmd.NewMockShellExecutor()
		mock.AddCommandString("linkerd", []string{"install", "--crds"}, "crd-manifest", nil)
		mock.AddCommandString("linkerd", []string{"install"}, "manifest", nil)
		mock.AddPartialMatcherString("kubectl", []string{"apply", "-f"}, "applied", nil)
		ctx = cmd.WithShellExecutor(ctx, mock)

		result, err := handleLinkerdInstall(ctx, mcp.CallToolRequest{})
		require.NoError(t, err)
		assert.False(t, result.IsError)
	})

	t.Run("ha install with overrides", func(t *testing.T) {
		mock := cmd.NewMockShellExecutor()
		mock.AddCommandString("linkerd", []string{"install", "--ha", "--skip-checks", "--identity-trust-anchors-pem", "anchors", "--set", "global.proxy.logLevel=debug", "--crds"}, "manifest", nil)
		mock.AddPartialMatcherString("kubectl", []string{"apply", "-f"}, "applied", nil)
		ctx = cmd.WithShellExecutor(ctx, mock)

		request := mcp.CallToolRequest{}
		request.Params.Arguments = map[string]interface{}{
			"ha":                         "true",
			"crds_only":                  "true",
			"skip_checks":                "true",
			"identity_trust_anchors_pem": "anchors",
			"set_overrides":              "global.proxy.logLevel=debug",
		}

		result, err := handleLinkerdInstall(ctx, request)
		require.NoError(t, err)
		assert.False(t, result.IsError)
	})
}

func TestHandleLinkerdWorkloadInjection(t *testing.T) {
	t.Run("disable deployment", func(t *testing.T) {
		ctx := context.Background()
		mock := cmd.NewMockShellExecutor()
		patch := `{"spec":{"template":{"metadata":{"annotations":{"linkerd.io/inject":"disabled"}}}}}`
		mock.AddCommandString("kubectl", []string{"patch", "deployment", "simple-app-v5", "-n", "simple-app", "-p", patch}, "patched", nil)
		ctx = cmd.WithShellExecutor(ctx, mock)

		req := mcp.CallToolRequest{}
		req.Params.Arguments = map[string]interface{}{
			"workload_name": "simple-app-v5",
			"namespace":     "simple-app",
			"workload_type": "deployment",
			"inject_state":  "disabled",
		}

		result, err := handleLinkerdWorkloadInjection(ctx, req)
		require.NoError(t, err)
		assert.False(t, result.IsError)
	})

	t.Run("remove annotation", func(t *testing.T) {
		ctx := context.Background()
		mock := cmd.NewMockShellExecutor()
		patch := `[{"op":"remove","path":"/spec/template/metadata/annotations/linkerd.io~1inject"}]`
		mock.AddCommandString("kubectl", []string{"patch", "statefulset", "inventory", "-n", "default", "--type=json", "-p", patch}, "patched", nil)
		ctx = cmd.WithShellExecutor(ctx, mock)

		req := mcp.CallToolRequest{}
		req.Params.Arguments = map[string]interface{}{
			"workload_name":     "inventory",
			"workload_type":     "statefulset",
			"remove_annotation": "true",
		}

		result, err := handleLinkerdWorkloadInjection(ctx, req)
		require.NoError(t, err)
		assert.False(t, result.IsError)
	})

	t.Run("missing name", func(t *testing.T) {
		ctx := context.Background()
		result, err := handleLinkerdWorkloadInjection(ctx, mcp.CallToolRequest{})
		require.NoError(t, err)
		assert.True(t, result.IsError)
	})
}

func TestHandleLinkerdInstallCNI(t *testing.T) {
	ctx := context.Background()

	t.Run("default install-cni", func(t *testing.T) {
		mock := cmd.NewMockShellExecutor()
		mock.AddCommandString("linkerd", []string{"install-cni"}, "manifest", nil)
		mock.AddPartialMatcherString("kubectl", []string{"apply", "-f"}, "applied", nil)
		ctx = cmd.WithShellExecutor(ctx, mock)

		result, err := handleLinkerdInstallCNI(ctx, mcp.CallToolRequest{})
		require.NoError(t, err)
		assert.False(t, result.IsError)
	})

	t.Run("install-cni with overrides", func(t *testing.T) {
		mock := cmd.NewMockShellExecutor()
		mock.AddCommandString("linkerd", []string{"install-cni", "--skip-checks", "--set", "cniResourceReadyTimeout=10m"}, "manifest", nil)
		mock.AddPartialMatcherString("kubectl", []string{"apply", "-f"}, "applied", nil)
		ctx = cmd.WithShellExecutor(ctx, mock)

		request := mcp.CallToolRequest{}
		request.Params.Arguments = map[string]interface{}{
			"skip_checks":   "true",
			"set_overrides": "cniResourceReadyTimeout=10m",
		}

		result, err := handleLinkerdInstallCNI(ctx, request)
		require.NoError(t, err)
		assert.False(t, result.IsError)
	})
}

func TestHandleLinkerdUpgrade(t *testing.T) {
	ctx := context.Background()

	mock := cmd.NewMockShellExecutor()
	mock.AddCommandString("linkerd", []string{"upgrade", "--crds", "--ha", "--skip-checks", "--set", "global.proxy.logLevel=debug"}, "manifest", nil)
	mock.AddPartialMatcherString("kubectl", []string{"apply", "-f"}, "applied", nil)
	ctx = cmd.WithShellExecutor(ctx, mock)

	request := mcp.CallToolRequest{}
	request.Params.Arguments = map[string]interface{}{
		"ha":            "true",
		"crds_only":     "true",
		"skip_checks":   "true",
		"set_overrides": "global.proxy.logLevel=debug",
	}

	result, err := handleLinkerdUpgrade(ctx, request)
	require.NoError(t, err)
	assert.False(t, result.IsError)
}

func TestHandleLinkerdUninstall(t *testing.T) {
	ctx := context.Background()

	mock := cmd.NewMockShellExecutor()
	mock.AddCommandString("linkerd", []string{"uninstall", "--force"}, "removed", nil)
	mock.AddPartialMatcherString("kubectl", []string{"delete", "-f"}, "deleted", nil)
	ctx = cmd.WithShellExecutor(ctx, mock)

	request := mcp.CallToolRequest{}
	request.Params.Arguments = map[string]interface{}{
		"force": "true",
	}

	result, err := handleLinkerdUninstall(ctx, request)
	require.NoError(t, err)
	assert.False(t, result.IsError)
}

func TestHandleLinkerdVersion(t *testing.T) {
	ctx := context.Background()

	mock := cmd.NewMockShellExecutor()
	mock.AddCommandString("linkerd", []string{"version", "--client", "--short"}, "version", nil)
	ctx = cmd.WithShellExecutor(ctx, mock)

	request := mcp.CallToolRequest{}
	request.Params.Arguments = map[string]interface{}{
		"client_only": "true",
		"short":       "true",
	}

	result, err := handleLinkerdVersion(ctx, request)
	require.NoError(t, err)
	assert.False(t, result.IsError)
}

func TestHandleLinkerdAuthz(t *testing.T) {
	ctx := context.Background()

	t.Run("missing resource", func(t *testing.T) {
		result, err := handleLinkerdAuthz(ctx, mcp.CallToolRequest{})
		require.NoError(t, err)
		assert.True(t, result.IsError)
	})

	t.Run("authz resource", func(t *testing.T) {
		mock := cmd.NewMockShellExecutor()
		mock.AddCommandString("linkerd", []string{"authz", "-n", "default", "deploy/web"}, "authz", nil)
		ctx = cmd.WithShellExecutor(ctx, mock)

		request := mcp.CallToolRequest{}
		request.Params.Arguments = map[string]interface{}{
			"resource":  "deploy/web",
			"namespace": "default",
		}

		result, err := handleLinkerdAuthz(ctx, request)
		require.NoError(t, err)
		assert.False(t, result.IsError)
	})
}

func TestHandleLinkerdStat(t *testing.T) {
	ctx := context.Background()

	t.Run("missing resource", func(t *testing.T) {
		result, err := handleLinkerdStat(ctx, mcp.CallToolRequest{})
		require.NoError(t, err)
		assert.True(t, result.IsError)
	})

	t.Run("stat specific namespace", func(t *testing.T) {
		mock := cmd.NewMockShellExecutor()
		mock.AddCommandString("linkerd", []string{"stat", "deploy/web", "-n", "default", "--from", "deploy/api", "--time-window", "1m", "-o", "json"}, "stats", nil)
		ctx = cmd.WithShellExecutor(ctx, mock)

		request := mcp.CallToolRequest{}
		request.Params.Arguments = map[string]interface{}{
			"resource":    "deploy/web",
			"namespace":   "default",
			"from":        "deploy/api",
			"time_window": "1m",
			"output":      "json",
		}

		result, err := handleLinkerdStat(ctx, request)
		require.NoError(t, err)
		assert.False(t, result.IsError)
	})
}

func TestHandleLinkerdTop(t *testing.T) {
	ctx := context.Background()

	t.Run("missing resource", func(t *testing.T) {
		result, err := handleLinkerdTop(ctx, mcp.CallToolRequest{})
		require.NoError(t, err)
		assert.True(t, result.IsError)
	})

	t.Run("top specific namespace", func(t *testing.T) {
		mock := cmd.NewMockShellExecutor()
		mock.AddCommandString("linkerd", []string{"top", "deploy/web", "-n", "default", "--max", "10", "--time-window", "30s"}, "top", nil)
		ctx = cmd.WithShellExecutor(ctx, mock)

		request := mcp.CallToolRequest{}
		request.Params.Arguments = map[string]interface{}{
			"resource":    "deploy/web",
			"namespace":   "default",
			"max_results": "10",
			"time_window": "30s",
		}

		result, err := handleLinkerdTop(ctx, request)
		require.NoError(t, err)
		assert.False(t, result.IsError)
	})
}

func TestHandleLinkerdEdges(t *testing.T) {
	ctx := context.Background()

	t.Run("missing resource", func(t *testing.T) {
		result, err := handleLinkerdEdges(ctx, mcp.CallToolRequest{})
		require.NoError(t, err)
		assert.True(t, result.IsError)
	})

	t.Run("edges all namespaces", func(t *testing.T) {
		mock := cmd.NewMockShellExecutor()
		mock.AddCommandString("linkerd", []string{"edges", "deploy/web", "-A", "-o", "wide"}, "edges", nil)
		ctx = cmd.WithShellExecutor(ctx, mock)

		request := mcp.CallToolRequest{}
		request.Params.Arguments = map[string]interface{}{
			"resource":       "deploy/web",
			"all_namespaces": "true",
			"output":         "wide",
		}

		result, err := handleLinkerdEdges(ctx, request)
		require.NoError(t, err)
		assert.False(t, result.IsError)
	})
}

func TestHandleLinkerdRoutes(t *testing.T) {
	ctx := context.Background()

	t.Run("missing resource", func(t *testing.T) {
		result, err := handleLinkerdRoutes(ctx, mcp.CallToolRequest{})
		require.NoError(t, err)
		assert.True(t, result.IsError)
	})

	t.Run("routes with filters", func(t *testing.T) {
		mock := cmd.NewMockShellExecutor()
		mock.AddCommandString("linkerd", []string{"routes", "deploy/web", "-n", "default", "--from", "deploy/api", "--to", "svc/backend"}, "routes", nil)
		ctx = cmd.WithShellExecutor(ctx, mock)

		request := mcp.CallToolRequest{}
		request.Params.Arguments = map[string]interface{}{
			"resource":  "deploy/web",
			"namespace": "default",
			"from":      "deploy/api",
			"to":        "svc/backend",
		}

		result, err := handleLinkerdRoutes(ctx, request)
		require.NoError(t, err)
		assert.False(t, result.IsError)
	})
}

func TestHandleLinkerdDiagnosticsProxyMetrics(t *testing.T) {
	ctx := context.Background()

	t.Run("basic selector", func(t *testing.T) {
		mock := cmd.NewMockShellExecutor()
		mock.AddCommandString("linkerd", []string{"diagnostics", "proxy-metrics", "-A", "--selector", "app=web"}, "metrics", nil)
		ctx = cmd.WithShellExecutor(ctx, mock)

		request := mcp.CallToolRequest{}
		request.Params.Arguments = map[string]interface{}{
			"all_namespaces": "true",
			"selector":       "app=web",
		}

		result, err := handleLinkerdDiagnosticsProxyMetrics(ctx, request)
		require.NoError(t, err)
		assert.False(t, result.IsError)
	})

	t.Run("with namespace and resource", func(t *testing.T) {
		mock := cmd.NewMockShellExecutor()
		mock.AddCommandString("linkerd", []string{"diagnostics", "proxy-metrics", "-n", "emojivoto", "deploy/web"}, "metrics", nil)
		ctx = cmd.WithShellExecutor(ctx, mock)

		request := mcp.CallToolRequest{}
		request.Params.Arguments = map[string]interface{}{
			"namespace": "emojivoto",
			"resource":  "deploy/web",
		}

		result, err := handleLinkerdDiagnosticsProxyMetrics(ctx, request)
		require.NoError(t, err)
		assert.False(t, result.IsError)
	})
}

func TestHandleLinkerdDiagnosticsControllerMetrics(t *testing.T) {
	ctx := context.Background()

	mock := cmd.NewMockShellExecutor()
	mock.AddCommandString("linkerd", []string{"diagnostics", "controller-metrics", "-n", "linkerd", "--component", "controller"}, "metrics", nil)
	ctx = cmd.WithShellExecutor(ctx, mock)

	request := mcp.CallToolRequest{}
	request.Params.Arguments = map[string]interface{}{
		"namespace": "linkerd",
		"component": "controller",
	}

	result, err := handleLinkerdDiagnosticsControllerMetrics(ctx, request)
	require.NoError(t, err)
	assert.False(t, result.IsError)
}

func TestHandleLinkerdDiagnosticsEndpoints(t *testing.T) {
	ctx := context.Background()

	t.Run("missing authority", func(t *testing.T) {
		result, err := handleLinkerdDiagnosticsEndpoints(ctx, mcp.CallToolRequest{})
		require.NoError(t, err)
		assert.True(t, result.IsError)
	})

	t.Run("with authority", func(t *testing.T) {
		mock := cmd.NewMockShellExecutor()
		mock.AddCommandString("linkerd", []string{"diagnostics", "endpoints", "web.linkerd-viz.svc.cluster.local:8084"}, "endpoints", nil)
		ctx = cmd.WithShellExecutor(ctx, mock)

		request := mcp.CallToolRequest{}
		request.Params.Arguments = map[string]interface{}{
			"authority": "web.linkerd-viz.svc.cluster.local:8084",
		}

		result, err := handleLinkerdDiagnosticsEndpoints(ctx, request)
		require.NoError(t, err)
		assert.False(t, result.IsError)
	})
}

func TestHandleLinkerdDiagnosticsPolicy(t *testing.T) {
	ctx := context.Background()

	mock := cmd.NewMockShellExecutor()
	mock.AddCommandString("linkerd", []string{"diagnostics", "policy", "-n", "default", "web.linkerd-viz.svc.cluster.local:8084"}, "policy", nil)
	ctx = cmd.WithShellExecutor(ctx, mock)

	request := mcp.CallToolRequest{}
	request.Params.Arguments = map[string]interface{}{
		"authority": "web.linkerd-viz.svc.cluster.local:8084",
		"namespace": "default",
	}

	result, err := handleLinkerdDiagnosticsPolicy(ctx, request)
	require.NoError(t, err)
	assert.False(t, result.IsError)
}

func TestHandleLinkerdDiagnosticsProfile(t *testing.T) {
	ctx := context.Background()

	mock := cmd.NewMockShellExecutor()
	mock.AddCommandString("linkerd", []string{"diagnostics", "profile", "web.linkerd-viz.svc.cluster.local:8084"}, "profile", nil)
	ctx = cmd.WithShellExecutor(ctx, mock)

	request := mcp.CallToolRequest{}
	request.Params.Arguments = map[string]interface{}{
		"authority": "web.linkerd-viz.svc.cluster.local:8084",
	}

	result, err := handleLinkerdDiagnosticsProfile(ctx, request)
	require.NoError(t, err)
	assert.False(t, result.IsError)
}

func TestHandleLinkerdVizInstall(t *testing.T) {
	ctx := context.Background()

	mock := cmd.NewMockShellExecutor()
	mock.AddCommandString("linkerd", []string{"viz", "install", "--ha", "--skip-checks", "--set", "tap.resources.limits.cpu=200m"}, "manifest", nil)
	mock.AddPartialMatcherString("kubectl", []string{"apply", "-f"}, "applied", nil)
	ctx = cmd.WithShellExecutor(ctx, mock)

	request := mcp.CallToolRequest{}
	request.Params.Arguments = map[string]interface{}{
		"ha":            "true",
		"skip_checks":   "true",
		"set_overrides": "tap.resources.limits.cpu=200m",
	}

	result, err := handleLinkerdVizInstall(ctx, request)
	require.NoError(t, err)
	assert.False(t, result.IsError)
}

func TestHandleLinkerdVizUninstall(t *testing.T) {
	ctx := context.Background()

	mock := cmd.NewMockShellExecutor()
	mock.AddCommandString("linkerd", []string{"viz", "uninstall", "--force"}, "removed", nil)
	mock.AddPartialMatcherString("kubectl", []string{"delete", "-f"}, "deleted", nil)
	ctx = cmd.WithShellExecutor(ctx, mock)

	request := mcp.CallToolRequest{}
	request.Params.Arguments = map[string]interface{}{
		"force": "true",
	}

	result, err := handleLinkerdVizUninstall(ctx, request)
	require.NoError(t, err)
	assert.False(t, result.IsError)
}

func TestHandleLinkerdVizTop(t *testing.T) {
	ctx := context.Background()

	t.Run("missing resource", func(t *testing.T) {
		result, err := handleLinkerdVizTop(ctx, mcp.CallToolRequest{})
		require.NoError(t, err)
		assert.True(t, result.IsError)
	})

	t.Run("viz top resource", func(t *testing.T) {
		mock := cmd.NewMockShellExecutor()
		mock.AddCommandString("linkerd", []string{"viz", "top", "deploy/web", "-n", "default", "--max", "5"}, "top", nil)
		ctx = cmd.WithShellExecutor(ctx, mock)

		request := mcp.CallToolRequest{}
		request.Params.Arguments = map[string]interface{}{
			"resource":    "deploy/web",
			"namespace":   "default",
			"max_results": "5",
		}

		result, err := handleLinkerdVizTop(ctx, request)
		require.NoError(t, err)
		assert.False(t, result.IsError)
	})
}

func TestHandleLinkerdVizStat(t *testing.T) {
	ctx := context.Background()

	t.Run("missing resource", func(t *testing.T) {
		result, err := handleLinkerdVizStat(ctx, mcp.CallToolRequest{})
		require.NoError(t, err)
		assert.True(t, result.IsError)
	})

	t.Run("viz stat all namespaces", func(t *testing.T) {
		mock := cmd.NewMockShellExecutor()
		mock.AddCommandString("linkerd", []string{"viz", "stat", "deploy/web", "-A", "--time-window", "30s"}, "stats", nil)
		ctx = cmd.WithShellExecutor(ctx, mock)

		request := mcp.CallToolRequest{}
		request.Params.Arguments = map[string]interface{}{
			"resource":       "deploy/web",
			"all_namespaces": "true",
			"time_window":    "30s",
		}

		result, err := handleLinkerdVizStat(ctx, request)
		require.NoError(t, err)
		assert.False(t, result.IsError)
	})
}

func TestHandleLinkerdFipsAudit(t *testing.T) {
	ctx := context.Background()

	mock := cmd.NewMockShellExecutor()
	mock.AddCommandString("linkerd", []string{"fips", "audit", "-n", "default"}, "audit", nil)
	ctx = cmd.WithShellExecutor(ctx, mock)

	request := mcp.CallToolRequest{}
	request.Params.Arguments = map[string]interface{}{
		"namespace": "default",
	}

	result, err := handleLinkerdFipsAudit(ctx, request)
	require.NoError(t, err)
	assert.False(t, result.IsError)
}

func TestHandleLinkerdPolicyGenerate(t *testing.T) {
	ctx := context.Background()

	mock := cmd.NewMockShellExecutor()
	mock.AddCommandString("linkerd", []string{"policy", "generate", "-n", "default", "-o", "yaml", "--timeout", "30s"}, "policy", nil)
	ctx = cmd.WithShellExecutor(ctx, mock)

	request := mcp.CallToolRequest{}
	request.Params.Arguments = map[string]interface{}{
		"namespace": "default",
		"output":    "yaml",
		"timeout":   "30s",
	}

	result, err := handleLinkerdPolicyGenerate(ctx, request)
	require.NoError(t, err)
	assert.False(t, result.IsError)
}
