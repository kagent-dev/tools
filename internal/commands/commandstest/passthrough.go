// Package commandstest provides test helpers for code that runs CLIs through
// commands.CommandBuilder with a passthrough token: they check that the token
// reaches the CLI only through the per-call kubeconfig that Execute writes,
// never on argv.
package commandstest

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/kagent-dev/tools/internal/cmd"
	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/client-go/tools/clientcmd"
)

// BaseKubeconfig is the configured kubeconfig the per-call kubeconfig is
// derived from in these tests.
const BaseKubeconfig = `apiVersion: v1
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

// BaseServer is the cluster server of BaseKubeconfig.
const BaseServer = "https://127.0.0.1:6443"

// RequestWithBearerToken returns a tool request carrying
// "Authorization: Bearer <token>", as the HTTP transport delivers it.
func RequestWithBearerToken(token string) *sdkmcp.CallToolRequest {
	h := http.Header{}
	h.Set("Authorization", "Bearer "+token)
	return &sdkmcp.CallToolRequest{Extra: &sdkmcp.RequestExtra{Header: h}}
}

// Kubeconfig records what the last command saw in its --kubeconfig while it
// ran (the file is removed again before the call returns), and the paths and
// tokens of every command that was given one.
type Kubeconfig struct {
	mu        sync.Mutex
	paths     []string
	tokens    []string
	path      string
	fileMode  os.FileMode
	dirMode   os.FileMode
	tokenMode os.FileMode
	token     string
	inline    string // a token written into the kubeconfig itself (must stay empty)
	server    string
}

// Path returns the --kubeconfig path the last command was given, or "".
func (k *Kubeconfig) Path() string {
	k.mu.Lock()
	defer k.mu.Unlock()
	return k.path
}

// Tokens returns, for every command given a --kubeconfig, the token its
// kubeconfig's token file held ("" for a kubeconfig without one).
func (k *Kubeconfig) Tokens() []string {
	k.mu.Lock()
	defer k.mu.Unlock()
	return append([]string(nil), k.tokens...)
}

// kubeconfigCapturingExecutor reads the per-call kubeconfig during Exec, then
// forwards the call to the next executor without the --kubeconfig pair, so a
// mock's expectations are the arguments a caller asked for.
type kubeconfigCapturingExecutor struct {
	t    *testing.T
	next cmd.ShellExecutor
	seen *Kubeconfig
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
	seen := e.seen
	seen.paths = append(seen.paths, path)
	seen.path, seen.token, seen.inline, seen.server = path, "", "", ""
	seen.fileMode, seen.dirMode, seen.tokenMode = 0, 0, 0
	defer func() { seen.tokens = append(seen.tokens, seen.token) }()
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
	if user.TokenFile == "" {
		// The configured kubeconfig, not a per-call one: nothing more to read.
		return
	}
	ti, err := os.Stat(user.TokenFile)
	require.NoError(e.t, err)
	e.seen.tokenMode = ti.Mode().Perm()
	b, err := os.ReadFile(user.TokenFile)
	require.NoError(e.t, err)
	e.seen.token = string(b)
}

// WithPassthroughKubeconfig configures BaseKubeconfig through $KUBECONFIG and
// returns a context whose shell executor records the --kubeconfig a command was
// given before delegating to next (typically a cmd.MockShellExecutor).
func WithPassthroughKubeconfig(t *testing.T, ctx context.Context, next cmd.ShellExecutor) (context.Context, *Kubeconfig) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config")
	require.NoError(t, os.WriteFile(path, []byte(BaseKubeconfig), 0o600))
	t.Setenv("KUBECONFIG", path)
	seen := &Kubeconfig{}
	return cmd.WithShellExecutor(ctx, &kubeconfigCapturingExecutor{t: t, next: next, seen: seen}), seen
}

// AssertTokenOnlyInKubeconfig checks that token reached the command only
// through the per-call kubeconfig, never on argv, and that the files are gone.
func AssertTokenOnlyInKubeconfig(t *testing.T, args []string, seen *Kubeconfig, token string) {
	t.Helper()
	for _, a := range args {
		assert.NotContains(t, a, token, "token must not be on argv")
		assert.NotEqual(t, "--token", a)
	}
	seen.mu.Lock()
	defer seen.mu.Unlock()
	require.NotEmpty(t, seen.path, "the command must be given a per-call --kubeconfig")
	assert.Equal(t, token, seen.token, "token file must hold the caller's token")
	assert.Empty(t, seen.inline, "kubeconfig must reference the token, not contain it")
	assert.Equal(t, BaseServer, seen.server, "cluster must come from the base kubeconfig")
	assert.Equal(t, os.FileMode(0o600), seen.fileMode)
	assert.Equal(t, os.FileMode(0o600), seen.tokenMode)
	assert.Equal(t, os.FileMode(0o700), seen.dirMode)
	for _, path := range seen.paths {
		_, err := os.Stat(filepath.Dir(path))
		assert.True(t, os.IsNotExist(err), "per-call kubeconfig directory must be removed after the call")
	}
}
