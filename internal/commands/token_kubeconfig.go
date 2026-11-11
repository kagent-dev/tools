package commands

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"

	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

const (
	// tokenUserName names the only user entry in a per-call kubeconfig.
	tokenUserName = "kagent-token-passthrough"
	// inClusterName names the cluster and context derived from the in-cluster
	// environment when no kubeconfig is available.
	inClusterName = "in-cluster"
)

// In-cluster service account files, the same ones kubectl and client-go read.
// Variables so tests can point them elsewhere.
var (
	inClusterCAFile        = "/var/run/secrets/kubernetes.io/serviceaccount/ca.crt"
	inClusterNamespaceFile = "/var/run/secrets/kubernetes.io/serviceaccount/namespace"
)

// tokenKubeconfig is a kubeconfig written for a single command execution. It
// carries the caller's bearer token by reference (tokenFile) so the token is
// never on the child process's command line, and never inside the kubeconfig
// itself (a CLI that quotes its kubeconfig, or `kubectl config view --raw`,
// shows only the token file's path).
type tokenKubeconfig struct {
	dir  string
	path string
}

// newTokenKubeconfig writes a minimal kubeconfig to a fresh private temporary
// directory (mode 0700, files 0600). The cluster entry is copied unchanged from
// the base configuration's selected context (server, CA, TLS settings,
// proxy-url, ...). The user entry carries only the token file: no client
// certificate or key, exec plugin, auth provider, basic auth or impersonation
// is copied from the base user.
//
// The base configuration is resolved the way kubectl resolves it: the explicit
// kubeconfig path when set, otherwise $KUBECONFIG or ~/.kube/config, and when
// none of those yields any configuration, the in-cluster environment
// (KUBERNETES_SERVICE_HOST/PORT and the service account CA and namespace).
// contextName selects a context of the base kubeconfig; empty means its
// current context.
//
// The caller must call cleanup, which removes the directory and both files.
// Error messages never contain the token.
func newTokenKubeconfig(baseKubeconfig, contextName, token string) (*tokenKubeconfig, error) {
	cfg, err := tokenKubeconfigConfig(baseKubeconfig, contextName)
	if err != nil {
		return nil, err
	}

	dir, err := os.MkdirTemp("", "kagent-kubeconfig-")
	if err != nil {
		return nil, fmt.Errorf("token passthrough: create temporary directory: %w", err)
	}
	kc := &tokenKubeconfig{dir: dir}
	ok := false
	defer func() {
		if !ok {
			kc.cleanup()
		}
	}()
	// MkdirTemp creates the directory 0700; make that explicit so a permissive
	// umask or platform default can never widen it.
	if err := os.Chmod(dir, 0o700); err != nil {
		return nil, fmt.Errorf("token passthrough: restrict temporary directory: %w", err)
	}

	tokenPath, err := writePrivateFile(dir, "token-*", []byte(token))
	if err != nil {
		return nil, err
	}
	cfg.AuthInfos[tokenUserName] = &clientcmdapi.AuthInfo{TokenFile: tokenPath}

	data, err := clientcmd.Write(*cfg)
	if err != nil {
		return nil, fmt.Errorf("token passthrough: encode kubeconfig: %w", err)
	}
	kc.path, err = writePrivateFile(dir, "kubeconfig-*", data)
	if err != nil {
		return nil, err
	}

	ok = true
	return kc, nil
}

// cleanup removes the kubeconfig and the token file. It is safe to call more
// than once and on a nil receiver.
func (kc *tokenKubeconfig) cleanup() {
	if kc == nil || kc.dir == "" {
		return
	}
	_ = os.RemoveAll(kc.dir)
}

// tokenKubeconfigConfig builds the per-call kubeconfig without the user's
// credential, which newTokenKubeconfig adds once the token file exists.
func tokenKubeconfigConfig(baseKubeconfig, contextName string) (*clientcmdapi.Config, error) {
	rules := clientcmd.NewDefaultClientConfigLoadingRules()
	rules.ExplicitPath = baseKubeconfig
	rules.MigrationRules = nil // read only: never move files in the home directory
	base, err := rules.Load()
	if err != nil {
		return nil, fmt.Errorf("token passthrough: load kubeconfig: %w", err)
	}

	if isEmptyConfig(base) {
		if contextName != "" {
			return nil, fmt.Errorf("token passthrough: context %q not found: no kubeconfig is configured", contextName)
		}
		return inClusterConfig()
	}

	if contextName == "" {
		contextName = base.CurrentContext
	}
	if contextName == "" {
		return nil, fmt.Errorf("token passthrough: kubeconfig has no current context")
	}
	ctx, ok := base.Contexts[contextName]
	if !ok || ctx == nil {
		return nil, fmt.Errorf("token passthrough: context %q not found in kubeconfig", contextName)
	}
	cluster, ok := base.Clusters[ctx.Cluster]
	if !ok || cluster == nil {
		return nil, fmt.Errorf("token passthrough: cluster %q of context %q not found in kubeconfig", ctx.Cluster, contextName)
	}

	cfg := clientcmdapi.NewConfig()
	// The whole cluster entry is kept: server, certificate-authority(-data),
	// insecure-skip-tls-verify, tls-server-name, proxy-url, disable-compression
	// and extensions. The loader has already made file paths absolute.
	cfg.Clusters[ctx.Cluster] = cluster.DeepCopy()
	cfg.Contexts[contextName] = &clientcmdapi.Context{
		Cluster:   ctx.Cluster,
		AuthInfo:  tokenUserName,
		Namespace: ctx.Namespace,
	}
	cfg.CurrentContext = contextName
	return cfg, nil
}

// inClusterConfig derives the cluster from the in-cluster environment, as
// kubectl does when no kubeconfig is available.
func inClusterConfig() (*clientcmdapi.Config, error) {
	host, port := os.Getenv("KUBERNETES_SERVICE_HOST"), os.Getenv("KUBERNETES_SERVICE_PORT")
	if host == "" || port == "" {
		return nil, fmt.Errorf("token passthrough: no kubeconfig is configured and not running in a cluster")
	}

	cluster := &clientcmdapi.Cluster{Server: "https://" + net.JoinHostPort(host, port)}
	if _, err := os.Stat(inClusterCAFile); err == nil {
		cluster.CertificateAuthority = inClusterCAFile
	}
	ctx := &clientcmdapi.Context{Cluster: inClusterName, AuthInfo: tokenUserName}
	if ns, err := os.ReadFile(inClusterNamespaceFile); err == nil {
		ctx.Namespace = strings.TrimSpace(string(ns))
	}

	cfg := clientcmdapi.NewConfig()
	cfg.Clusters[inClusterName] = cluster
	cfg.Contexts[inClusterName] = ctx
	cfg.CurrentContext = inClusterName
	return cfg, nil
}

func isEmptyConfig(cfg *clientcmdapi.Config) bool {
	return cfg == nil || (len(cfg.Clusters) == 0 && len(cfg.Contexts) == 0 && len(cfg.AuthInfos) == 0)
}

// writePrivateFile writes data to a new file in dir with mode 0600 and returns
// its path.
func writePrivateFile(dir, pattern string, data []byte) (string, error) {
	f, err := os.CreateTemp(dir, pattern)
	if err != nil {
		return "", fmt.Errorf("token passthrough: create %s: %w", pattern, err)
	}
	path := f.Name()
	if err := f.Chmod(0o600); err != nil {
		_ = f.Close()
		return "", fmt.Errorf("token passthrough: restrict %s: %w", filepath.Base(path), err)
	}
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		return "", fmt.Errorf("token passthrough: write %s: %w", filepath.Base(path), err)
	}
	if err := f.Close(); err != nil {
		return "", fmt.Errorf("token passthrough: close %s: %w", filepath.Base(path), err)
	}
	return path, nil
}

// fingerprintKey keys tokenFingerprint. It is random per process, so a
// fingerprint in a log or a trace cannot be used to confirm a guessed token.
var fingerprintKey = func() []byte {
	k := make([]byte, 32)
	if _, err := rand.Read(k); err != nil {
		panic("commands: read random fingerprint key: " + err.Error())
	}
	return k
}()

// tokenFingerprint identifies a token in cache keys without revealing it.
func tokenFingerprint(token string) string {
	m := hmac.New(sha256.New, fingerprintKey)
	m.Write([]byte(token))
	return hex.EncodeToString(m.Sum(nil))
}
