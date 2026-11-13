package k8s

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/kagent-dev/tools/internal/cmd"
	mcp "github.com/kagent-dev/tools/internal/mcp"
	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"k8s.io/client-go/tools/clientcmd"
)

// baseKubeconfig is the configured kubeconfig the per-call kubeconfig is
// derived from in these tests.
const baseKubeconfig = `apiVersion: v1
kind: Config
clusters:
- name: test-cluster
  cluster:
    server: https://127.0.0.1:6443
    insecure-skip-tls-verify: true
contexts:
- name: test-context
  context:
    cluster: test-cluster
    user: server-identity
current-context: test-context
users:
- name: server-identity
  user:
    token: server-identity-token-must-not-be-used
`

// passthroughKubeconfig records what a kubectl call saw in its --kubeconfig
// while it ran: the file is removed again before the call returns.
type passthroughKubeconfig struct {
	mu        sync.Mutex
	path      string
	fileMode  os.FileMode
	dirMode   os.FileMode
	tokenMode os.FileMode
	token     string
	inline    string // a token written into the kubeconfig itself (must stay empty)
	server    string
}

// kubeconfigCapturingExecutor reads the per-call kubeconfig during Exec, then
// forwards the call to the mock without the --kubeconfig pair, so the mock's
// expectations are the arguments a caller asked for.
type kubeconfigCapturingExecutor struct {
	t    *testing.T
	next cmd.ShellExecutor
	seen *passthroughKubeconfig
}

func (e *kubeconfigCapturingExecutor) Exec(ctx context.Context, command string, args ...string) ([]byte, error) {
	var rest []string
	for i := 0; i < len(args); i++ {
		if args[i] == "--kubeconfig" && i+1 < len(args) {
			e.capture(args[i+1])
			i++
			continue
		}
		rest = append(rest, args[i])
	}
	return e.next.Exec(ctx, command, rest...)
}

func (e *kubeconfigCapturingExecutor) capture(path string) {
	e.seen.mu.Lock()
	defer e.seen.mu.Unlock()
	e.seen.path = path
	fi, err := os.Stat(path)
	require.NoError(e.t, err, "per-call kubeconfig must exist while the command runs")
	e.seen.fileMode = fi.Mode().Perm()
	di, err := os.Stat(filepath.Dir(path))
	require.NoError(e.t, err)
	e.seen.dirMode = di.Mode().Perm()

	cfg, err := clientcmd.LoadFromFile(path)
	require.NoError(e.t, err)
	ctx := cfg.Contexts[cfg.CurrentContext]
	require.NotNil(e.t, ctx)
	e.seen.server = cfg.Clusters[ctx.Cluster].Server
	user := cfg.AuthInfos[ctx.AuthInfo]
	require.NotNil(e.t, user)
	e.seen.inline = user.Token
	ti, err := os.Stat(user.TokenFile)
	require.NoError(e.t, err)
	e.seen.tokenMode = ti.Mode().Perm()
	b, err := os.ReadFile(user.TokenFile)
	require.NoError(e.t, err)
	e.seen.token = string(b)
}

// withPassthroughKubeconfig configures a base kubeconfig and installs an
// executor that records the per-call kubeconfig before delegating to mock.
func withPassthroughKubeconfig(t *testing.T, ctx context.Context, mock cmd.ShellExecutor) (context.Context, *passthroughKubeconfig) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config")
	require.NoError(t, os.WriteFile(path, []byte(baseKubeconfig), 0o600))
	t.Setenv("KUBECONFIG", path)
	seen := &passthroughKubeconfig{}
	return cmd.WithShellExecutor(ctx, &kubeconfigCapturingExecutor{t: t, next: mock, seen: seen}), seen
}

// assertTokenOnlyInKubeconfig checks that token reached kubectl only through
// the per-call kubeconfig, never on argv, and that the files are gone.
func assertTokenOnlyInKubeconfig(t *testing.T, args []string, seen *passthroughKubeconfig, token string) {
	t.Helper()
	for _, a := range args {
		assert.NotContains(t, a, token, "token must not be on argv")
		assert.NotEqual(t, "--token", a)
	}
	seen.mu.Lock()
	defer seen.mu.Unlock()
	require.NotEmpty(t, seen.path, "kubectl must be given a per-call --kubeconfig")
	assert.Equal(t, token, seen.token, "token file must hold the caller's token")
	assert.Empty(t, seen.inline, "kubeconfig must reference the token, not contain it")
	assert.Equal(t, "https://127.0.0.1:6443", seen.server, "cluster must come from the base kubeconfig")
	assert.Equal(t, os.FileMode(0o600), seen.fileMode)
	assert.Equal(t, os.FileMode(0o600), seen.tokenMode)
	assert.Equal(t, os.FileMode(0o700), seen.dirMode)
	_, err := os.Stat(filepath.Dir(seen.path))
	assert.True(t, os.IsNotExist(err), "per-call kubeconfig directory must be removed after the call")
}

// TestBearerTokenNotInFailedResult covers the reported leak: a failing kubectl
// call's tool result must not contain the caller's token.
func TestBearerTokenNotInFailedResult(t *testing.T) {
	const token = "eyJhbGciOiJSUzI1NiJ9.leak-check-token.sig"
	mock := cmd.NewMockShellExecutor()
	mock.AddPartialMatcherString("kubectl", []string{"get", "secrets"}, `Error from server (Forbidden): secrets is forbidden`, assert.AnError)
	ctx, seen := withPassthroughKubeconfig(t, context.Background(), mock)

	exp := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exp))
	prev := otel.GetTracerProvider()
	otel.SetTracerProvider(tp)
	t.Cleanup(func() {
		otel.SetTracerProvider(prev)
		_ = tp.Shutdown(context.Background())
	})

	// Run the handler behind the same middleware the server installs, so the
	// tool span (status from the result text) is covered too.
	k8sTool := newTestK8sToolWithPassthrough(true)
	req := requestWithBearerToken(token)
	req.Params = &sdkmcp.CallToolParamsRaw{Name: "k8s_get_resources"}
	h := mcp.ToolMiddleware()(func(ctx context.Context, _ string, r sdkmcp.Request) (sdkmcp.Result, error) {
		res, _, err := k8sTool.handleKubectlGetEnhanced(ctx, r.(*sdkmcp.CallToolRequest), getResourcesInput{ResourceType: "secrets"})
		return res, err
	})
	res, err := h(ctx, "tools/call", req)
	require.NoError(t, err)
	result, ok := res.(*sdkmcp.CallToolResult)
	require.True(t, ok)
	require.NotNil(t, result)
	assert.True(t, result.IsError)
	text := getResultText(result)
	assert.NotContains(t, text, token)
	assert.Contains(t, text, "get secrets")
	for _, c := range result.Content {
		if tc, ok := c.(*sdkmcp.TextContent); ok {
			assert.False(t, strings.Contains(tc.Text, token))
		}
	}
	callLog := mock.GetCallLog()
	require.Len(t, callLog, 1)
	assertTokenOnlyInKubeconfig(t, callLog[0].Args, seen, token)

	spans := exp.GetSpans()
	names := map[string]bool{}
	for _, s := range spans {
		names[s.Name] = true
		assert.NotContains(t, s.Status.Description, token, "span %s status", s.Name)
		for _, a := range s.Attributes {
			assert.NotContains(t, a.Value.Emit(), token, "span %s attribute %s", s.Name, a.Key)
		}
		for _, ev := range s.Events {
			for _, a := range ev.Attributes {
				assert.NotContains(t, a.Value.Emit(), token, "span %s event %s", s.Name, ev.Name)
			}
		}
	}
	assert.True(t, names["mcp.tool.k8s_get_resources"], "tool span recorded, got %v", names)
	assert.True(t, names["commands.execute"], "command span recorded, got %v", names)
}
