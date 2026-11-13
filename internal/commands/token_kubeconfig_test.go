package commands

import (
	"context"
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kagent-dev/tools/internal/cache"
	"github.com/kagent-dev/tools/internal/cmd"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

const testToken = "eyJhbGciOiJSUzI1NiJ9.passthrough-test-token.signature"

var testCAData = []byte("-----BEGIN CERTIFICATE-----\nnot-a-real-ca\n-----END CERTIFICATE-----\n")

// writeBaseKubeconfig writes a kubeconfig whose user carries every credential
// type and impersonation, so tests can check none of it is carried over.
func writeBaseKubeconfig(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	caFile := filepath.Join(dir, "ca.crt")
	require.NoError(t, os.WriteFile(caFile, testCAData, 0o600))
	cfg := `apiVersion: v1
kind: Config
clusters:
- name: prod
  cluster:
    server: https://prod.example.test:6443
    certificate-authority-data: ` + base64.StdEncoding.EncodeToString(testCAData) + `
    tls-server-name: api.prod.example.test
    proxy-url: http://proxy.example.test:3128
    disable-compression: true
- name: staging
  cluster:
    server: https://staging.example.test:6443
    certificate-authority: ca.crt
contexts:
- name: prod-ctx
  context:
    cluster: prod
    user: admin
    namespace: apps
- name: staging-ctx
  context:
    cluster: staging
    user: admin
current-context: prod-ctx
users:
- name: admin
  user:
    client-certificate-data: ` + base64.StdEncoding.EncodeToString([]byte("client-cert")) + `
    client-key-data: ` + base64.StdEncoding.EncodeToString([]byte("client-key")) + `
    token: server-identity-token
    username: admin
    password: admin-password
    as: system:admin
    as-uid: "1234"
    as-groups: ["system:masters"]
    as-user-extra:
      scopes: ["all"]
    exec:
      apiVersion: client.authentication.k8s.io/v1
      command: get-credentials
      interactiveMode: Never
`
	path := filepath.Join(dir, "config")
	require.NoError(t, os.WriteFile(path, []byte(cfg), 0o600))
	return path
}

// observedKubeconfig is what a command saw in its --kubeconfig while it ran.
type observedKubeconfig struct {
	path      string
	dirMode   os.FileMode
	fileMode  os.FileMode
	tokenMode os.FileMode
	token     string
	config    *clientcmdapi.Config
}

// recordingExecutor captures argv and the per-call kubeconfig during Exec.
type recordingExecutor struct {
	t      *testing.T
	mu     sync.Mutex
	calls  [][]string
	seen   []observedKubeconfig
	output string
	err    error
	// block, when set, makes Exec wait for the context to be done.
	block bool
}

func (e *recordingExecutor) Exec(ctx context.Context, _ string, args ...string) ([]byte, error) {
	e.mu.Lock()
	e.calls = append(e.calls, append([]string(nil), args...))
	for i := 0; i+1 < len(args); i++ {
		if args[i] == "--kubeconfig" {
			e.seen = append(e.seen, observe(e.t, args[i+1]))
		}
	}
	e.mu.Unlock()
	if e.block {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	return []byte(e.output), e.err
}

func observe(t *testing.T, path string) observedKubeconfig {
	o := observedKubeconfig{path: path}
	fi, err := os.Stat(path)
	require.NoError(t, err)
	o.fileMode = fi.Mode().Perm()
	di, err := os.Stat(filepath.Dir(path))
	require.NoError(t, err)
	o.dirMode = di.Mode().Perm()
	o.config, err = clientcmd.LoadFromFile(path)
	require.NoError(t, err)
	if user := o.config.AuthInfos[tokenUserName]; user != nil && user.TokenFile != "" {
		ti, err := os.Stat(user.TokenFile)
		require.NoError(t, err)
		o.tokenMode = ti.Mode().Perm()
		b, err := os.ReadFile(user.TokenFile)
		require.NoError(t, err)
		o.token = string(b)
	}
	return o
}

func (e *recordingExecutor) lastSeen(t *testing.T) observedKubeconfig {
	e.mu.Lock()
	defer e.mu.Unlock()
	require.NotEmpty(t, e.seen, "command must be given a --kubeconfig")
	return e.seen[len(e.seen)-1]
}

func assertNoTokenOnArgv(t *testing.T, e *recordingExecutor) {
	t.Helper()
	e.mu.Lock()
	defer e.mu.Unlock()
	require.NotEmpty(t, e.calls)
	for _, args := range e.calls {
		for _, a := range args {
			assert.NotContains(t, a, testToken)
			assert.NotEqual(t, "--token", a)
		}
	}
}

func assertRemoved(t *testing.T, o observedKubeconfig) {
	t.Helper()
	_, err := os.Stat(filepath.Dir(o.path))
	assert.True(t, os.IsNotExist(err), "per-call kubeconfig directory must be removed, stat err: %v", err)
}

func TestExecuteWithTokenKeepsTokenOffArgv(t *testing.T) {
	base := writeBaseKubeconfig(t)
	exec := &recordingExecutor{t: t, output: "ok"}
	ctx := cmd.WithShellExecutor(context.Background(), exec)

	out, err := KubectlBuilder().WithArgs("get", "pods").WithNamespace("default").
		WithKubeconfig(base).WithToken(testToken).Execute(ctx)
	require.NoError(t, err)
	assert.Equal(t, "ok", out)

	assertNoTokenOnArgv(t, exec)
	args := exec.calls[0]
	assert.Equal(t, []string{"get", "pods", "--namespace", "default", "--kubeconfig"}, args[:5])
	assert.NotEqual(t, base, args[5], "the configured kubeconfig is replaced for the call")
	assert.Len(t, args, 6)

	o := exec.lastSeen(t)
	assert.Equal(t, testToken, o.token)
	assert.Equal(t, os.FileMode(0o600), o.fileMode)
	assert.Equal(t, os.FileMode(0o600), o.tokenMode)
	assert.Equal(t, os.FileMode(0o700), o.dirMode)
	assertRemoved(t, o)
}

func TestBuildNeverEmitsToken(t *testing.T) {
	_, args, err := KubectlBuilder().WithArgs("get", "pods").WithToken(testToken).Build()
	require.NoError(t, err)
	assert.Equal(t, []string{"get", "pods"}, args)
}

func TestTokenKubeconfigUserCarriesOnlyTheToken(t *testing.T) {
	base := writeBaseKubeconfig(t)
	exec := &recordingExecutor{t: t}
	ctx := cmd.WithShellExecutor(context.Background(), exec)
	_, err := KubectlBuilder().WithArgs("get", "pods").WithKubeconfig(base).WithToken(testToken).Execute(ctx)
	require.NoError(t, err)

	cfg := exec.lastSeen(t).config
	require.Len(t, cfg.AuthInfos, 1, "exactly one user")
	user := cfg.AuthInfos[tokenUserName]
	require.NotNil(t, user)

	// Exactly one credential: the token, by reference.
	assert.NotEmpty(t, user.TokenFile)
	assert.Empty(t, user.Token, "token is referenced through tokenFile, not inlined")
	assert.Empty(t, user.ClientCertificate)
	assert.Empty(t, user.ClientCertificateData)
	assert.Empty(t, user.ClientKey)
	assert.Empty(t, user.ClientKeyData)
	assert.Empty(t, user.Username)
	assert.Empty(t, user.Password)
	assert.Nil(t, user.Exec)
	assert.Nil(t, user.AuthProvider)
	// No impersonation.
	assert.Empty(t, user.Impersonate)
	assert.Empty(t, user.ImpersonateUID)
	assert.Empty(t, user.ImpersonateGroups)
	assert.Empty(t, user.ImpersonateUserExtra)
	// Nothing else either: the whole entry equals a token-file-only user.
	assert.Equal(t, clientcmdapi.AuthInfo{TokenFile: user.TokenFile, Extensions: user.Extensions, LocationOfOrigin: user.LocationOfOrigin}, *user)
	assert.Empty(t, user.Extensions)

	// The file on disk holds no credential from the base kubeconfig.
	raw, err := os.ReadFile(base)
	require.NoError(t, err)
	require.Contains(t, string(raw), "server-identity-token")
	derived, err := clientcmd.Write(*cfg)
	require.NoError(t, err)
	for _, s := range []string{"server-identity-token", "admin-password", "get-credentials", "system:admin", "system:masters", base64.StdEncoding.EncodeToString([]byte("client-key"))} {
		assert.NotContains(t, string(derived), s)
	}
}

func TestTokenKubeconfigKeepsClusterAndContext(t *testing.T) {
	base := writeBaseKubeconfig(t)

	t.Run("current context", func(t *testing.T) {
		exec := &recordingExecutor{t: t}
		ctx := cmd.WithShellExecutor(context.Background(), exec)
		_, err := KubectlBuilder().WithArgs("get", "pods").WithKubeconfig(base).WithToken(testToken).Execute(ctx)
		require.NoError(t, err)

		cfg := exec.lastSeen(t).config
		assert.Equal(t, "prod-ctx", cfg.CurrentContext)
		require.Len(t, cfg.Contexts, 1)
		c := cfg.Contexts["prod-ctx"]
		require.NotNil(t, c)
		assert.Equal(t, "prod", c.Cluster)
		assert.Equal(t, tokenUserName, c.AuthInfo)
		assert.Equal(t, "apps", c.Namespace, "context namespace is kept")
		require.Len(t, cfg.Clusters, 1)
		cl := cfg.Clusters["prod"]
		require.NotNil(t, cl)
		assert.Equal(t, "https://prod.example.test:6443", cl.Server)
		assert.Equal(t, testCAData, cl.CertificateAuthorityData)
		assert.Equal(t, "api.prod.example.test", cl.TLSServerName)
		assert.Equal(t, "http://proxy.example.test:3128", cl.ProxyURL)
		assert.True(t, cl.DisableCompression)
	})

	t.Run("explicit context and relative CA path", func(t *testing.T) {
		exec := &recordingExecutor{t: t}
		ctx := cmd.WithShellExecutor(context.Background(), exec)
		_, err := KubectlBuilder().WithArgs("get", "pods").WithKubeconfig(base).WithContext("staging-ctx").
			WithToken(testToken).Execute(ctx)
		require.NoError(t, err)

		args := exec.calls[0]
		assert.Contains(t, strings.Join(args, " "), "--context staging-ctx")
		cfg := exec.lastSeen(t).config
		assert.Equal(t, "staging-ctx", cfg.CurrentContext)
		cl := cfg.Clusters["staging"]
		require.NotNil(t, cl)
		assert.Equal(t, "https://staging.example.test:6443", cl.Server)
		assert.Equal(t, filepath.Join(filepath.Dir(base), "ca.crt"), cl.CertificateAuthority,
			"relative CA path is resolved against the base kubeconfig")
	})

	t.Run("unknown context fails without running the command", func(t *testing.T) {
		exec := &recordingExecutor{t: t}
		ctx := cmd.WithShellExecutor(context.Background(), exec)
		_, err := KubectlBuilder().WithArgs("get", "pods").WithKubeconfig(base).WithContext("missing").
			WithToken(testToken).Execute(ctx)
		require.Error(t, err)
		assert.NotContains(t, err.Error(), testToken)
		assert.Empty(t, exec.calls)
	})
}

func TestTokenKubeconfigInCluster(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("KUBECONFIG", filepath.Join(dir, "does-not-exist"))
	t.Setenv("HOME", dir)
	t.Setenv("KUBERNETES_SERVICE_HOST", "10.96.0.1")
	t.Setenv("KUBERNETES_SERVICE_PORT", "443")
	ca := filepath.Join(dir, "ca.crt")
	ns := filepath.Join(dir, "namespace")
	require.NoError(t, os.WriteFile(ca, testCAData, 0o600))
	require.NoError(t, os.WriteFile(ns, []byte("kagent\n"), 0o600))
	prevCA, prevNS := inClusterCAFile, inClusterNamespaceFile
	inClusterCAFile, inClusterNamespaceFile = ca, ns
	t.Cleanup(func() { inClusterCAFile, inClusterNamespaceFile = prevCA, prevNS })

	exec := &recordingExecutor{t: t}
	ctx := cmd.WithShellExecutor(context.Background(), exec)
	_, err := KubectlBuilder().WithArgs("get", "pods").WithToken(testToken).Execute(ctx)
	require.NoError(t, err)

	o := exec.lastSeen(t)
	cfg := o.config
	assert.Equal(t, inClusterName, cfg.CurrentContext)
	assert.Equal(t, "https://10.96.0.1:443", cfg.Clusters[inClusterName].Server)
	assert.Equal(t, ca, cfg.Clusters[inClusterName].CertificateAuthority)
	assert.Equal(t, "kagent", cfg.Contexts[inClusterName].Namespace)
	assert.Equal(t, testToken, o.token)
	assertNoTokenOnArgv(t, exec)
	assertRemoved(t, o)

	t.Run("no kubeconfig and not in a cluster", func(t *testing.T) {
		t.Setenv("KUBERNETES_SERVICE_HOST", "")
		exec := &recordingExecutor{t: t}
		ctx := cmd.WithShellExecutor(context.Background(), exec)
		_, err := KubectlBuilder().WithArgs("get", "pods").WithToken(testToken).Execute(ctx)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not running in a cluster")
		assert.NotContains(t, err.Error(), testToken)
		assert.Empty(t, exec.calls)
	})
}

func TestTokenKubeconfigRemovedOnErrorAndCancel(t *testing.T) {
	base := writeBaseKubeconfig(t)

	t.Run("command fails", func(t *testing.T) {
		exec := &recordingExecutor{t: t, output: "Error from server (Forbidden)", err: assert.AnError}
		ctx := cmd.WithShellExecutor(context.Background(), exec)
		_, err := KubectlBuilder().WithArgs("get", "secrets").WithKubeconfig(base).WithToken(testToken).Execute(ctx)
		require.Error(t, err)
		assertRemoved(t, exec.lastSeen(t))
	})

	t.Run("context cancelled while the command runs", func(t *testing.T) {
		exec := &recordingExecutor{t: t, block: true}
		ctx, cancel := context.WithCancel(cmd.WithShellExecutor(context.Background(), exec))
		done := make(chan error, 1)
		go func() {
			_, err := KubectlBuilder().WithArgs("get", "pods", "--watch").WithKubeconfig(base).WithToken(testToken).Execute(ctx)
			done <- err
		}()
		require.Eventually(t, func() bool {
			exec.mu.Lock()
			defer exec.mu.Unlock()
			return len(exec.seen) == 1
		}, 5*time.Second, 10*time.Millisecond)
		o := exec.lastSeen(t)
		_, err := os.Stat(o.path)
		require.NoError(t, err, "kubeconfig exists while the command runs")
		cancel()
		select {
		case err := <-done:
			require.Error(t, err)
		case <-time.After(5 * time.Second):
			t.Fatal("Execute did not return after cancel")
		}
		assertRemoved(t, o)
	})

	t.Run("context deadline exceeded", func(t *testing.T) {
		exec := &recordingExecutor{t: t, block: true}
		ctx, cancel := context.WithTimeout(cmd.WithShellExecutor(context.Background(), exec), 50*time.Millisecond)
		defer cancel()
		_, err := KubectlBuilder().WithArgs("wait", "--for=condition=ready", "pod/x").WithKubeconfig(base).
			WithToken(testToken).Execute(ctx)
		require.Error(t, err)
		assertRemoved(t, exec.lastSeen(t))
	})
}

// TestFailedCommandDoesNotLeakToken covers the reported leak: the error a
// failing call returns (shown to the MCP client) and the spans it records must
// not contain the token.
func TestFailedCommandDoesNotLeakToken(t *testing.T) {
	exp := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exp))
	prev := otel.GetTracerProvider()
	otel.SetTracerProvider(tp)
	t.Cleanup(func() {
		otel.SetTracerProvider(prev)
		_ = tp.Shutdown(context.Background())
	})

	base := writeBaseKubeconfig(t)
	for _, command := range []string{"kubectl", "helm", "istioctl", "cilium"} {
		t.Run(command, func(t *testing.T) {
			exp.Reset()
			exec := &recordingExecutor{t: t, err: assert.AnError}
			ctx := cmd.WithShellExecutor(context.Background(), exec)
			_, err := NewCommandBuilder(command).WithArgs("get", "secrets").WithKubeconfig(base).
				WithToken(testToken).Execute(ctx)
			require.Error(t, err)
			assert.NotContains(t, err.Error(), testToken)
			assert.Contains(t, err.Error(), "get secrets --kubeconfig "+base+" failed",
				"the error names the configured kubeconfig, not the removed per-call one")
			assertNoTokenOnArgv(t, exec)

			spans := exp.GetSpans()
			require.NotEmpty(t, spans)
			for _, s := range spans {
				assert.NotContains(t, s.Status.Description, testToken)
				for _, a := range s.Attributes {
					assert.NotContains(t, a.Value.Emit(), testToken, "span %s attribute %s", s.Name, a.Key)
				}
				for _, ev := range s.Events {
					for _, a := range ev.Attributes {
						assert.NotContains(t, a.Value.Emit(), testToken, "span %s event %s", s.Name, ev.Name)
					}
				}
			}
		})
	}
}

// TestFailedCommandRedactsCredentialArgs covers the defense in depth: a
// credential a caller puts in the arguments is redacted from the error.
func TestFailedCommandRedactsCredentialArgs(t *testing.T) {
	exec := &recordingExecutor{t: t, err: assert.AnError}
	ctx := cmd.WithShellExecutor(context.Background(), exec)
	_, err := HelmBuilder().WithArgs("install", "x", "chart", "--kube-token", testToken, "--set", "db.password="+testToken).Execute(ctx)
	require.Error(t, err)
	assert.NotContains(t, err.Error(), testToken)
	assert.Contains(t, err.Error(), "--kube-token <REDACTED>")
}

func TestBuilderFlagsGoBeforeDoubleDash(t *testing.T) {
	base := writeBaseKubeconfig(t)
	exec := &recordingExecutor{t: t}
	ctx := cmd.WithShellExecutor(context.Background(), exec)
	_, err := KubectlBuilder().WithArgs("exec", "mypod", "--", "ls", "-la").WithNamespace("default").
		WithKubeconfig(base).WithToken(testToken).Execute(ctx)
	require.NoError(t, err)

	args := exec.calls[0]
	dd := -1
	for i, a := range args {
		if a == "--" {
			dd = i
			break
		}
	}
	require.NotEqual(t, -1, dd)
	assert.Equal(t, []string{"--", "ls", "-la"}, args[dd:], "the remote command is unchanged")
	assert.Contains(t, args[:dd], "--kubeconfig")
	assert.Contains(t, args[:dd], "--namespace")
}

func TestCachedExecutionIsPerToken(t *testing.T) {
	cache.InitCaches()
	cache.InvalidateKubernetesCache()
	t.Cleanup(cache.InvalidateKubernetesCache)

	base := writeBaseKubeconfig(t)
	exec := &recordingExecutor{t: t, output: "pods"}
	ctx := cmd.WithShellExecutor(context.Background(), exec)
	run := func(token string) {
		_, err := KubectlBuilder().WithArgs("get", "pods").WithKubeconfig(base).WithToken(token).
			WithCache(true).Execute(ctx)
		require.NoError(t, err)
	}
	run(testToken)
	run(testToken)
	run("another-callers-token")
	exec.mu.Lock()
	calls := len(exec.calls)
	exec.mu.Unlock()
	assert.Equal(t, 2, calls, "same token hits the cache, a different token does not")
	assertNoTokenOnArgv(t, exec)

	// A custom cache key is per token too.
	_, err := KubectlBuilder().WithArgs("get", "pods").WithKubeconfig(base).WithToken("third-token").
		WithCache(true).WithCacheKey("pods").Execute(ctx)
	require.NoError(t, err)
	_, err = KubectlBuilder().WithArgs("get", "pods").WithKubeconfig(base).WithToken("fourth-token").
		WithCache(true).WithCacheKey("pods").Execute(ctx)
	require.NoError(t, err)
	exec.mu.Lock()
	calls = len(exec.calls)
	exec.mu.Unlock()
	assert.Equal(t, 4, calls, "a custom cache key does not share results across tokens")

	key := tokenFingerprint(testToken)
	assert.NotContains(t, key, testToken)
	assert.Len(t, key, 64)
	assert.NotEqual(t, key, tokenFingerprint("another-callers-token"))
}
