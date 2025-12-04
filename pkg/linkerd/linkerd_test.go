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

type manifestExecCall struct {
	args   []string
	stdout string
	stderr string
	err    error
}

type mockManifestExecutor struct {
	t     *testing.T
	calls []manifestExecCall
	idx   int
}

func newMockManifestExecutor(t *testing.T, calls []manifestExecCall) *mockManifestExecutor {
	return &mockManifestExecutor{t: t, calls: calls}
}

func (m *mockManifestExecutor) Run(ctx context.Context, command string, args []string) (string, string, error) {
	m.t.Helper()
	if m.idx >= len(m.calls) {
		m.t.Fatalf("unexpected manifest command: %s %v", command, args)
	}
	call := m.calls[m.idx]
	m.idx++
	require.Equal(m.t, "linkerd", command)
	require.Equal(m.t, call.args, args)
	return call.stdout, call.stderr, call.err
}

func (m *mockManifestExecutor) assertDone() {
	m.t.Helper()
	if m.idx != len(m.calls) {
		m.t.Fatalf("expected %d manifest commands, got %d", len(m.calls), m.idx)
	}
}

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
		mock.AddPartialMatcherString("kubectl", []string{"apply", "-f"}, "applied", nil)
		ctx = cmd.WithShellExecutor(ctx, mock)

		manifestMock := newMockManifestExecutor(t, []manifestExecCall{
			{args: []string{"install", "--crds"}, stdout: "crd-manifest"},
			{args: []string{"install"}, stdout: "manifest"},
		})
		prev := linkerdManifestExecutor
		linkerdManifestExecutor = manifestMock
		t.Cleanup(func() {
			manifestMock.assertDone()
			linkerdManifestExecutor = prev
		})

		result, err := handleLinkerdInstall(ctx, mcp.CallToolRequest{})
		require.NoError(t, err)
		assert.False(t, result.IsError)
	})

	t.Run("ha install with overrides", func(t *testing.T) {
		mock := cmd.NewMockShellExecutor()
		mock.AddPartialMatcherString("kubectl", []string{"apply", "-f"}, "applied", nil)
		ctx = cmd.WithShellExecutor(ctx, mock)

		manifestMock := newMockManifestExecutor(t, []manifestExecCall{
			{args: []string{"install", "--ha", "--skip-checks", "--identity-trust-anchors-pem", "anchors", "--set", "global.proxy.logLevel=debug", "--crds"}, stdout: "manifest"},
		})
		prev := linkerdManifestExecutor
		linkerdManifestExecutor = manifestMock
		t.Cleanup(func() {
			manifestMock.assertDone()
			linkerdManifestExecutor = prev
		})

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

	t.Run("install with advanced flags", func(t *testing.T) {
		expectedArgs := []string{"install",
			"--disable-h2-upgrade",
			"--enable-endpoint-slices=false",
			"--admin-port", "5555",
			"--controller-log-level", "debug",
			"--default-inbound-policy", "cluster-authenticated",
			"--identity-trust-anchors-file", "/tmp/anchors",
			"--proxy-cpu-limit", "500m",
			"--registry", "registry.example.com/linkerd",
			"-o", "json",
			"--set-string", "global.proxy.logLevel=debug",
			"-f", "overrides1.yaml",
			"-f", "overrides2.yaml",
			"--crds",
		}
		mock := cmd.NewMockShellExecutor()
		mock.AddPartialMatcherString("kubectl", []string{"apply", "-f"}, "applied", nil)
		ctx = cmd.WithShellExecutor(ctx, mock)

		manifestMock := newMockManifestExecutor(t, []manifestExecCall{
			{args: expectedArgs, stdout: "manifest"},
		})
		prev := linkerdManifestExecutor
		linkerdManifestExecutor = manifestMock
		t.Cleanup(func() {
			manifestMock.assertDone()
			linkerdManifestExecutor = prev
		})

		request := mcp.CallToolRequest{}
		request.Params.Arguments = map[string]interface{}{
			"crds_only":                   "true",
			"disable_h2_upgrade":          "true",
			"enable_endpoint_slices":      "false",
			"admin_port":                  "5555",
			"controller_log_level":        "debug",
			"default_inbound_policy":      "cluster-authenticated",
			"identity_trust_anchors_file": "/tmp/anchors",
			"proxy_cpu_limit":             "500m",
			"registry":                    "registry.example.com/linkerd",
			"output":                      "json",
			"set_string_overrides":        "global.proxy.logLevel=debug",
			"values":                      "overrides1.yaml,overrides2.yaml",
		}

		result, err := handleLinkerdInstall(ctx, request)
		require.NoError(t, err)
		assert.False(t, result.IsError)
	})

	t.Run("invalid boolean flag", func(t *testing.T) {
		request := mcp.CallToolRequest{}
		request.Params.Arguments = map[string]interface{}{
			"disable_h2_upgrade": "maybe",
		}
		result, err := handleLinkerdInstall(ctx, request)
		require.NoError(t, err)
		assert.True(t, result.IsError)
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
		mock.AddPartialMatcherString("kubectl", []string{"apply", "-f"}, "applied", nil)
		ctx = cmd.WithShellExecutor(ctx, mock)

		manifestMock := newMockManifestExecutor(t, []manifestExecCall{
			{args: []string{"install-cni"}, stdout: "manifest"},
		})
		prev := linkerdManifestExecutor
		linkerdManifestExecutor = manifestMock
		t.Cleanup(func() {
			manifestMock.assertDone()
			linkerdManifestExecutor = prev
		})

		result, err := handleLinkerdInstallCNI(ctx, mcp.CallToolRequest{})
		require.NoError(t, err)
		assert.False(t, result.IsError)
	})

	t.Run("install-cni with overrides", func(t *testing.T) {
		mock := cmd.NewMockShellExecutor()
		mock.AddPartialMatcherString("kubectl", []string{"apply", "-f"}, "applied", nil)
		ctx = cmd.WithShellExecutor(ctx, mock)

		manifestMock := newMockManifestExecutor(t, []manifestExecCall{
			{args: []string{"install-cni", "--skip-checks", "--set", "cniResourceReadyTimeout=10m"}, stdout: "manifest"},
		})
		prev := linkerdManifestExecutor
		linkerdManifestExecutor = manifestMock
		t.Cleanup(func() {
			manifestMock.assertDone()
			linkerdManifestExecutor = prev
		})

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
	mock.AddPartialMatcherString("kubectl", []string{"apply", "-f"}, "applied", nil)
	ctx = cmd.WithShellExecutor(ctx, mock)

	manifestMock := newMockManifestExecutor(t, []manifestExecCall{
		{args: []string{"upgrade", "--crds", "--ha", "--skip-checks", "--set", "global.proxy.logLevel=debug"}, stdout: "manifest"},
	})
	prev := linkerdManifestExecutor
	linkerdManifestExecutor = manifestMock
	t.Cleanup(func() {
		manifestMock.assertDone()
		linkerdManifestExecutor = prev
	})

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
	mock.AddPartialMatcherString("kubectl", []string{"delete", "-f"}, "deleted", nil)
	ctx = cmd.WithShellExecutor(ctx, mock)

	manifestMock := newMockManifestExecutor(t, []manifestExecCall{
		{args: []string{"uninstall", "--force"}, stdout: "removed"},
	})
	prev := linkerdManifestExecutor
	linkerdManifestExecutor = manifestMock
	t.Cleanup(func() {
		manifestMock.assertDone()
		linkerdManifestExecutor = prev
	})

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

	t.Run("client short", func(t *testing.T) {
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
	})

	t.Run("proxy namespace", func(t *testing.T) {
		mock := cmd.NewMockShellExecutor()
		mock.AddCommandString("linkerd", []string{"version", "--proxy", "-n", "linkerd"}, "version", nil)
		ctx = cmd.WithShellExecutor(ctx, mock)

		request := mcp.CallToolRequest{}
		request.Params.Arguments = map[string]interface{}{
			"proxy":     "true",
			"namespace": "linkerd",
		}

		result, err := handleLinkerdVersion(ctx, request)
		require.NoError(t, err)
		assert.False(t, result.IsError)
	})

	t.Run("explicit false client flag", func(t *testing.T) {
		mock := cmd.NewMockShellExecutor()
		mock.AddCommandString("linkerd", []string{"version", "--client=false"}, "version", nil)
		ctx = cmd.WithShellExecutor(ctx, mock)

		request := mcp.CallToolRequest{}
		request.Params.Arguments = map[string]interface{}{
			"client_only": "false",
		}

		result, err := handleLinkerdVersion(ctx, request)
		require.NoError(t, err)
		assert.False(t, result.IsError)
	})

	t.Run("invalid bool flag", func(t *testing.T) {
		request := mcp.CallToolRequest{}
		request.Params.Arguments = map[string]interface{}{
			"short": "maybe",
		}

		result, err := handleLinkerdVersion(ctx, request)
		require.NoError(t, err)
		assert.True(t, result.IsError)
	})
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

func TestHandleLinkerdDiagnosticsProxyMetrics(t *testing.T) {
	ctx := context.Background()

	t.Run("missing resource", func(t *testing.T) {
		result, err := handleLinkerdDiagnosticsProxyMetrics(ctx, mcp.CallToolRequest{})
		require.NoError(t, err)
		assert.True(t, result.IsError)
	})

	t.Run("with namespace and obfuscate", func(t *testing.T) {
		mockCtx := context.Background()
		mock := cmd.NewMockShellExecutor()
		mock.AddCommandString("linkerd", []string{"diagnostics", "proxy-metrics", "-n", "emojivoto", "--obfuscate", "deploy/web"}, "metrics", nil)
		mockCtx = cmd.WithShellExecutor(mockCtx, mock)

		request := mcp.CallToolRequest{}
		request.Params.Arguments = map[string]interface{}{
			"resource":  "deploy/web",
			"namespace": "emojivoto",
			"obfuscate": "true",
		}

		result, err := handleLinkerdDiagnosticsProxyMetrics(mockCtx, request)
		require.NoError(t, err)
		assert.False(t, result.IsError)
	})

	t.Run("without namespace", func(t *testing.T) {
		mockCtx := context.Background()
		mock := cmd.NewMockShellExecutor()
		mock.AddCommandString("linkerd", []string{"diagnostics", "proxy-metrics", "po/pod-foo"}, "metrics", nil)
		mockCtx = cmd.WithShellExecutor(mockCtx, mock)

		request := mcp.CallToolRequest{}
		request.Params.Arguments = map[string]interface{}{
			"resource": "po/pod-foo",
		}

		result, err := handleLinkerdDiagnosticsProxyMetrics(mockCtx, request)
		require.NoError(t, err)
		assert.False(t, result.IsError)
	})
}

func TestHandleLinkerdDiagnosticsControllerMetrics(t *testing.T) {
	ctx := context.Background()

	mock := cmd.NewMockShellExecutor()
	mock.AddCommandString("linkerd", []string{"diagnostics", "controller-metrics", "-n", "linkerd", "--component", "controller", "--wait", "45s"}, "metrics", nil)
	ctx = cmd.WithShellExecutor(ctx, mock)

	request := mcp.CallToolRequest{}
	request.Params.Arguments = map[string]interface{}{
		"namespace": "linkerd",
		"component": "controller",
		"wait":      "45s",
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

func TestHandleLinkerdFips(t *testing.T) {
	ctx := context.Background()

	mock := cmd.NewMockShellExecutor()
	mock.AddCommandString("linkerd", []string{"fips", "audit", "-n", "default"}, "audit", nil)
	ctx = cmd.WithShellExecutor(ctx, mock)

	request := mcp.CallToolRequest{}
	request.Params.Arguments = map[string]interface{}{
		"namespace": "default",
	}

	result, err := handleLinkerdFips(ctx, request)
	require.NoError(t, err)
	assert.False(t, result.IsError)
}

func TestHandleLinkerdPolicy(t *testing.T) {
	t.Run("defaults to generate", func(t *testing.T) {
		ctx := context.Background()

		mock := cmd.NewMockShellExecutor()
		mock.AddCommandString("linkerd", []string{"policy", "generate", "-n", "ns"}, "policy", nil)
		ctx = cmd.WithShellExecutor(ctx, mock)

		request := mcp.CallToolRequest{}
		request.Params.Arguments = map[string]interface{}{
			"namespace": "ns",
		}

		result, err := handleLinkerdPolicy(ctx, request)
		require.NoError(t, err)
		assert.False(t, result.IsError)
	})

	t.Run("unsupported command", func(t *testing.T) {
		ctx := context.Background()
		request := mcp.CallToolRequest{}
		request.Params.Arguments = map[string]interface{}{
			"command": "unknown",
		}

		result, err := handleLinkerdPolicy(ctx, request)
		require.NoError(t, err)
		assert.True(t, result.IsError)
	})
}
