package e2e

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/kagent-dev/tools/internal/commands"
	toolsmcp "github.com/kagent-dev/tools/internal/mcp"
	mcp "github.com/modelcontextprotocol/go-sdk/mcp"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// toolResultText concatenates the text content of a tool result. The go-sdk
// represents content as []mcp.Content (a slice of pointers), so formatting it
// with %v yields opaque addresses; this returns the human-readable message so
// failed tool calls surface the real error.
func toolResultText(result *mcp.CallToolResult) string {
	if result == nil {
		return ""
	}
	var b strings.Builder
	for _, c := range result.Content {
		if tc, ok := c.(*mcp.TextContent); ok {
			b.WriteString(tc.Text)
		}
	}
	return b.String()
}

// getBinaryName returns the platform-specific binary name
func getBinaryName() string {
	osName := runtime.GOOS
	archName := runtime.GOARCH
	return fmt.Sprintf("kagent-tools-%s-%s", osName, archName)
}

// TestServerConfig holds configuration for server tests
type TestServerConfig struct {
	Port       int
	Tools      []string
	Kubeconfig string
	Stdio      bool
	Timeout    time.Duration
}

// TestServer represents a test server instance
type TestServer struct {
	cmd    *exec.Cmd
	port   int
	stdio  bool
	cancel context.CancelFunc
	done   chan struct{}
	output strings.Builder
	mu     sync.RWMutex
}

// NewTestServer creates a new test server instance
func NewTestServer(config TestServerConfig) *TestServer {
	return &TestServer{
		port:  config.Port,
		stdio: config.Stdio,
		done:  make(chan struct{}),
	}
}

// Start starts the test server
func (ts *TestServer) Start(ctx context.Context, config TestServerConfig) error {
	ts.mu.Lock()
	defer ts.mu.Unlock()

	// Build command arguments
	args := []string{}
	if config.Stdio {
		args = append(args, "--stdio")
	} else {
		args = append(args, "--port", fmt.Sprintf("%d", config.Port))
	}

	if len(config.Tools) > 0 {
		args = append(args, "--tools", strings.Join(config.Tools, ","))
	}

	if config.Kubeconfig != "" {
		args = append(args, "--kubeconfig", config.Kubeconfig)
	}

	// Create context with cancellation
	ctx, cancel := context.WithCancel(ctx)
	ts.cancel = cancel

	// Start server process
	binaryName := getBinaryName()
	ts.cmd = exec.CommandContext(ctx, fmt.Sprintf("../../bin/%s", binaryName), args...)
	ts.cmd.Env = append(os.Environ(), "LOG_LEVEL=debug")

	// Set up output capture
	stdout, err := ts.cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("failed to create stdout pipe: %w", err)
	}

	stderr, err := ts.cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("failed to create stderr pipe: %w", err)
	}

	// Start the command
	if err := ts.cmd.Start(); err != nil {
		return fmt.Errorf("failed to start server: %w", err)
	}

	// Start goroutines to capture output
	go ts.captureOutput(stdout, "STDOUT")
	go ts.captureOutput(stderr, "STDERR")

	// Wait for server to start
	if !config.Stdio {
		return ts.waitForHTTPServer(ctx, config.Timeout)
	}

	return nil
}

// Stop stops the test server
func (ts *TestServer) Stop() error {
	ts.mu.Lock()
	defer ts.mu.Unlock()

	if ts.cancel != nil {
		ts.cancel()
	}

	if ts.cmd != nil && ts.cmd.Process != nil {
		// Send interrupt signal for graceful shutdown
		if err := ts.cmd.Process.Signal(os.Interrupt); err != nil {
			// If interrupt fails, kill the process
			_ = ts.cmd.Process.Kill()
		}

		// Wait for process to exit with timeout
		done := make(chan error, 1)
		go func() {
			done <- ts.cmd.Wait()
		}()

		select {
		case <-done:
			// Process exited
		case <-time.After(8 * time.Second): // Increased timeout
			// Timeout, force kill
			_ = ts.cmd.Process.Kill()
			// Wait a bit more for force kill to complete
			select {
			case <-done:
			case <-time.After(2 * time.Second):
				// Force kill timeout, continue anyway
			}
		}
	}

	// Signal done and wait for goroutines to exit
	if ts.done != nil {
		close(ts.done)
	}

	// Give goroutines time to exit
	time.Sleep(100 * time.Millisecond)

	return nil
}

// MCPClient represents a client for communicating with the MCP server using the official go-sdk client
type MCPClient struct {
	session *mcp.ClientSession
	log     *slog.Logger
}

// InstallKAgentTools installs KAgent Tools using helm in the specified namespace
func InstallKAgentTools(namespace string, releaseName string) {
	// The context must outlive helm's own --timeout below, otherwise the context
	// cancels first and helm is killed with "signal: killed" rather than being
	// allowed to report its real status.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	log := slog.Default()
	By("Installing KAgent Tools in namespace " + namespace)
	log.Info("Installing KAgent Tools", "namespace", namespace)

	// First, try to uninstall any existing release to clean up
	log.Info("Cleaning up any existing release", "release", releaseName, "namespace", namespace)
	_, _ = commands.NewCommandBuilder("helm").
		WithArgs("uninstall", releaseName).
		WithArgs("--namespace", namespace).
		WithArgs("--ignore-not-found").
		WithCache(false).
		Execute(ctx)

	// install crd scripts/kind/crd-argo.yaml
	By("Installing CRDs for KAgent Tools")
	_, err := commands.NewCommandBuilder("kubectl").
		WithArgs("apply", "-f", "../../scripts/kind/crd-argo.yaml").
		WithArgs("--namespace", namespace).
		WithCache(false). // Don't cache CRD installation
		Execute(ctx)
	Expect(err).ToNot(HaveOccurred(), "Failed to install CRDs: %v", err)

	// Install KAgent Tools using helm with unique release name
	// Use absolute path from project root
	//
	// --timeout must comfortably exceed the readiness probe's initialDelaySeconds
	// (15s) plus image pull and scheduling. The previous 1m expired with
	// "resource Deployment ... not ready: Available: 0/1" whenever the node was
	// busy, failing BeforeAll before any spec ran.
	output, err := commands.NewCommandBuilder("helm").
		WithArgs("install", releaseName, "../../helm/kagent-tools").
		WithArgs("--namespace", namespace).
		WithArgs("-f").
		WithArgs("../../scripts/kind/test-values-e2e.yaml").
		WithArgs("--create-namespace").
		WithArgs("--debug").
		WithArgs("--wait").
		WithArgs("--timeout=3m").
		WithCache(false). // Don't cache helm installation
		Execute(ctx)

	Expect(err).ToNot(HaveOccurred(), "Failed to install KAgent Tools: %v %v", err, output)
	log.Info("KAgent Tools installation completed", "namespace", namespace, "output", output)

	// Verify the installation by checking that pods are Running AND Ready.
	// Waiting on status.phase alone is racy: the container reports Running
	// immediately, but the server only starts serving /health and /mcp once the
	// readiness probe passes (initialDelaySeconds=15), so an MCP client that
	// connects in between gets "connection reset by peer". Gate on the
	// Ready condition instead.
	By("Verifying KAgent Tools pods are ready")
	log.Info("Verifying KAgent Tools pods", "namespace", namespace)

	Eventually(func() bool {
		ctx, cancel := context.WithTimeout(context.Background(), DefaultTimeout)
		defer cancel()

		output, err := commands.NewCommandBuilder("kubectl").
			WithArgs("get", "pods", "-n", namespace, "-l", "app.kubernetes.io/instance="+releaseName,
				"-o", "jsonpath={.items[*].status.conditions[?(@.type=='Ready')].status}").
			WithCache(false).
			Execute(ctx)

		if err != nil {
			log.Error("Failed to get pod readiness", "error", err)
			return false
		}

		log.Info("Pod readiness check", "namespace", namespace, "output", output)

		// Every pod must report Ready=True; an empty list means no pods yet.
		statuses := strings.Fields(strings.TrimSpace(output))
		if len(statuses) == 0 {
			return false
		}
		for _, status := range statuses {
			if status != "True" {
				return false
			}
		}
		return true
	}, 2*time.Minute, 5*time.Second).Should(BeTrue(), "KAgent Tools pods should become ready")

	log.Info("KAgent Tools pods are ready", "namespace", namespace)
	//validate service nodePort == 30885
	By("Validating KAgent Tools service is accessible")
	nodePort, err := commands.NewCommandBuilder("kubectl").
		WithArgs("get", "svc", "-n", namespace, "-o", "jsonpath={.items[0].spec.ports[0].nodePort}").
		Execute(ctx)
	Expect(err).ToNot(HaveOccurred(), "Failed to get service nodePort: %v", err)
	Expect(nodePort).To(Equal("30885"))

	// A Ready pod does not guarantee the NodePort is routable yet: kube-proxy
	// still has to program the new endpoint into the node's rules, and until it
	// does the NodePort answers with a connection reset. Probe it cheaply before
	// the suite starts, and keep the retry in GetMCPClient as well, so neither
	// side races kube-proxy.
	By("Waiting for the MCP endpoint to answer over the NodePort")
	Eventually(func() bool {
		probeCtx, probeCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer probeCancel()

		req, err := http.NewRequestWithContext(probeCtx, http.MethodPost,
			"http://127.0.0.1:30885/mcp", strings.NewReader("{}"))
		if err != nil {
			return false
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json, text/event-stream")

		resp, err := (&http.Client{Timeout: 5 * time.Second}).Do(req)
		if err != nil {
			log.Info("MCP endpoint not routable yet", "error", err)
			return false
		}
		defer func() { _ = resp.Body.Close() }()
		// Any HTTP response (even a 400 for the malformed body) proves the
		// NodePort is programmed and reaching the server.
		return resp.StatusCode > 0
	}, 2*time.Minute, 3*time.Second).Should(BeTrue(),
		"MCP endpoint did not become reachable on NodePort 30885")
}

// GetMCPClient creates a new MCP client configured for the e2e test environment
// using the official go-sdk client. The initialize handshake is retried because
// the NodePort can briefly reset connections while kube-proxy programs the
// Service endpoint after a rollout.
func GetMCPClient() (*MCPClient, error) {
	deadline := time.Now().Add(90 * time.Second)
	var lastErr error

	for time.Now().Before(deadline) {
		client, err := connectMCPClient()
		if err == nil {
			return client, nil
		}
		lastErr = err
		time.Sleep(2 * time.Second)
	}
	return nil, fmt.Errorf("failed to connect MCP client after retries: %w", lastErr)
}

// connectMCPClient performs a single MCP connect + initialize handshake.
func connectMCPClient() (*MCPClient, error) {
	// HTTP timeout long enough for operations like Istio installation.
	httpTransport := &mcp.StreamableClientTransport{
		Endpoint:   "http://127.0.0.1:30885/mcp",
		HTTPClient: &http.Client{Timeout: 180 * time.Second},
	}

	mcpClient := mcp.NewClient(&mcp.Implementation{Name: "e2e-test-client", Version: "1.0.0"}, nil)

	// Connect performs the initialization handshake.
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	session, err := mcpClient.Connect(ctx, httpTransport, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to connect MCP client: %w", err)
	}

	mcpHelper := &MCPClient{
		session: session,
		log:     slog.Default(),
	}

	// Validate connection by listing tools
	tools, err := mcpHelper.listTools()
	if len(tools) == 0 {
		return nil, fmt.Errorf("no tools found in MCP server: %w", err)
	}
	slog.Default().Info("MCP Client created", "baseURL", "http://127.0.0.1:30885/mcp", "tools", len(tools))
	return mcpHelper, err
}

// callTool invokes any MCP tool by name with typed arguments and returns the
// raw result. It does not treat a tool-level error (IsError) as a Go error, so
// callers can assert on either outcome; use it with ExpectMCPToolSuccess when a
// spec requires a successful call.
func (c *MCPClient) callTool(name string, args any) (*mcp.CallToolResult, error) {
	return c.callToolWithTimeout(name, args, 60*time.Second)
}

// callToolWithTimeout is callTool with an explicit timeout, for slow tools such
// as istio_install_istio or cilium_install_cilium.
func (c *MCPClient) callToolWithTimeout(name string, args any, timeout time.Duration) (*mcp.CallToolResult, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	return c.session.CallTool(ctx, &mcp.CallToolParams{
		Name:      name,
		Arguments: args,
	})
}

// listTools calls the tools/list method to get available tools
func (c *MCPClient) listTools() ([]*mcp.Tool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	result, err := c.session.ListTools(ctx, &mcp.ListToolsParams{})
	if err != nil {
		return nil, err
	}

	return result.Tools, nil
}

// k8sListResources calls the k8s_get_resources tool
func (c *MCPClient) k8sListResources(resourceType string) (*mcp.CallToolResult, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	type K8sArgs struct {
		ResourceType string `json:"resource_type"`
		Output       string `json:"output"`
	}

	arguments := K8sArgs{
		ResourceType: resourceType,
		Output:       "json",
	}

	result, err := c.session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "k8s_get_resources",
		Arguments: arguments,
	})
	if err != nil {
		return nil, err
	}
	if result.IsError {
		return nil, fmt.Errorf("tool call failed: %s", toolResultText(result))
	}
	return result, nil
}

// helmListReleases calls the helm_list_releases tool
func (c *MCPClient) helmListReleases() (*mcp.CallToolResult, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// all_namespaces is declared as a boolean in the tool's input schema, so it
	// must be sent as a JSON boolean. Sending the string "true" fails input
	// validation and the tool never executes.
	type HelmArgs struct {
		AllNamespaces bool   `json:"all_namespaces"`
		Output        string `json:"output"`
	}

	arguments := HelmArgs{
		AllNamespaces: true,
		Output:        "json",
	}

	result, err := c.session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "helm_list_releases",
		Arguments: arguments,
	})
	if err != nil {
		return nil, err
	}
	if result.IsError {
		return nil, fmt.Errorf("tool call failed: %s", toolResultText(result))
	}
	return result, nil
}

// istioInstall calls the istio_install_istio tool
func (c *MCPClient) istioInstall(profile string) (*mcp.CallToolResult, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second) // Istio install can take time
	defer cancel()

	type IstioArgs struct {
		Profile string `json:"profile"`
	}

	arguments := IstioArgs{
		Profile: profile,
	}

	result, err := c.session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "istio_install_istio",
		Arguments: arguments,
	})
	if err != nil {
		return nil, err
	}
	if result.IsError {
		return nil, fmt.Errorf("tool call failed: %s", toolResultText(result))
	}
	return result, nil
}

// argoRolloutsList calls the argo_rollouts_get tool to list rollouts
func (c *MCPClient) argoRolloutsList(namespace string) (*mcp.CallToolResult, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	type ArgoArgs struct {
		Namespace string `json:"namespace"`
		Output    string `json:"output"`
	}

	arguments := ArgoArgs{
		Namespace: namespace,
		Output:    "json",
	}

	result, err := c.session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "argo_rollouts_list",
		Arguments: arguments,
	})
	if err != nil {
		return nil, err
	}
	if result.IsError {
		return nil, fmt.Errorf("tool call failed: %s", toolResultText(result))
	}
	return result, nil
}

// ciliumStatus calls the cilium_status_and_version tool
func (c *MCPClient) ciliumStatus() (*mcp.CallToolResult, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	result, err := c.session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "cilium_status_and_version",
		Arguments: nil,
	})
	if err != nil {
		return nil, err
	}
	if result.IsError {
		return nil, fmt.Errorf("tool call failed: %s", toolResultText(result))
	}
	return result, nil
}

// TextOutputArgs mirrors internal/mcp.TextOutput so e2e assertions decode the
// same DTO the server produces for raw CLI text tools.
type TextOutputArgs = toolsmcp.TextOutput

// decodeTextOutput decodes a tool result's typed structuredContent into the
// shared TextOutput DTO. Every migrated handler returns a concrete Out type, so
// the SDK must populate StructuredContent; a nil value means a handler regressed
// to Out=any and the typed-output contract is broken.
func decodeTextOutput(result *mcp.CallToolResult) (TextOutputArgs, error) {
	var out TextOutputArgs
	if result == nil {
		return out, fmt.Errorf("nil tool result")
	}
	if result.StructuredContent == nil {
		return out, fmt.Errorf("result has no structuredContent: handler did not return a typed Out value")
	}
	raw, err := json.Marshal(result.StructuredContent)
	if err != nil {
		return out, fmt.Errorf("marshaling structuredContent: %w", err)
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return out, fmt.Errorf("decoding structuredContent into TextOutput: %w", err)
	}
	return out, nil
}

// clusterHasCilium reports whether Cilium is installed as a DaemonSet. The Kind
// cluster uses kindnet by default, so Cilium-backed tools are unavailable unless
// a test installed it; specs that need it must skip rather than fail.
func clusterHasCilium() bool {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	output, err := commands.NewCommandBuilder("kubectl").
		WithArgs("get", "daemonset", "cilium", "-n", "kube-system",
			"--ignore-not-found", "-o", "jsonpath={.metadata.name}").
		WithCache(false).
		Execute(ctx)
	return err == nil && strings.TrimSpace(output) == "cilium"
}

// clusterHasIstio reports whether Istio's control plane is installed.
func clusterHasIstio() bool {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	output, err := commands.NewCommandBuilder("kubectl").
		WithArgs("get", "deployment", "istiod", "-n", "istio-system",
			"--ignore-not-found", "-o", "jsonpath={.metadata.name}").
		WithCache(false).
		Execute(ctx)
	return err == nil && strings.TrimSpace(output) == "istiod"
}

// Constants for default test values
const (
	DefaultReleaseName   = "kagent-tools-e2e"
	DefaultTestNamespace = "kagent-tools-e2e"
	DefaultTimeout       = 60 * time.Second // Increased for more realistic timeouts
)

// CreateNamespace creates a new Kubernetes namespace
func CreateNamespace(namespace string) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	log := slog.Default()
	By("Creating namespace " + namespace)
	log.Info("Creating namespace", "namespace", namespace)

	// A namespace left over from a previous run may still be terminating
	// (DeleteNamespace issues the delete without waiting). Creating resources in
	// a terminating namespace fails with "unable to create new content ...
	// because it is being terminated", so wait for the old one to disappear
	// before deciding whether creation is needed.
	//
	// Note: --ignore-not-found makes kubectl exit 0 even when the namespace is
	// absent, so the wait must key on empty output rather than on an error.
	Eventually(func() bool {
		checkCtx, checkCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer checkCancel()

		output, err := commands.NewCommandBuilder("kubectl").
			WithArgs("get", "namespace", namespace, "--ignore-not-found", "-o", "jsonpath={.metadata.name}").
			WithCache(false).
			Execute(checkCtx)
		return err == nil && strings.TrimSpace(output) == ""
	}, 2*time.Minute, 2*time.Second).Should(BeTrue(),
		"namespace %s did not finish terminating before test setup", namespace)

	// Create the namespace using kubectl
	output, err := commands.NewCommandBuilder("kubectl").
		WithArgs("create", "namespace", namespace).
		WithCache(false). // Don't cache namespace creation
		Execute(ctx)

	// If it's an AlreadyExists error, that's fine - treat it as success
	if err != nil && strings.Contains(err.Error(), "AlreadyExists") {
		log.Info("Namespace already exists, continuing", "namespace", namespace)
		return
	}

	Expect(err).ToNot(HaveOccurred(), "Failed to create namespace: %v", err)
	log.Info("Namespace creation completed", "namespace", namespace, "output", output)
}

// DeleteNamespace deletes a Kubernetes namespace and waits for it to be fully
// removed, so a subsequent run can recreate it immediately.
func DeleteNamespace(namespace string) {
	// Use longer timeout for namespace deletion as it can take more time
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	log := slog.Default()
	By("Deleting namespace " + namespace)
	log.Info("Deleting namespace", "namespace", namespace)

	// Delete the namespace using kubectl
	output, err := commands.NewCommandBuilder("kubectl").
		WithArgs("delete", "namespace", namespace, "--ignore-not-found=true", "--wait=false").
		WithCache(false). // Don't cache namespace deletion
		Execute(ctx)

	Expect(err).ToNot(HaveOccurred(), "Failed to delete namespace: %v", err)
	log.Info("Namespace deletion completed", "namespace", namespace, "output", output)

	// Wait until the namespace is actually gone. Without this the next test run
	// can attempt to create resources in a still-terminating namespace and fail
	// with "unable to create new content ... because it is being terminated".
	// As above, --ignore-not-found returns exit 0 for a missing namespace, so
	// the wait keys on empty output.
	Eventually(func() bool {
		checkCtx, checkCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer checkCancel()

		output, err := commands.NewCommandBuilder("kubectl").
			WithArgs("get", "namespace", namespace, "--ignore-not-found", "-o", "jsonpath={.metadata.name}").
			WithCache(false).
			Execute(checkCtx)
		return err == nil && strings.TrimSpace(output) == ""
	}, 2*time.Minute, 2*time.Second).Should(BeTrue(),
		"namespace %s was not removed", namespace)
}

// waitForHTTPServer waits for the HTTP server to become available
func (ts *TestServer) waitForHTTPServer(ctx context.Context, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	url := fmt.Sprintf("http://localhost:%d/health", ts.port)
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("timeout waiting for server to start")
		case <-ticker.C:
			resp, err := http.Get(url)
			if err == nil {
				_ = resp.Body.Close()
				if resp.StatusCode == http.StatusOK {
					return nil
				}
			}
		}
	}
}

// waitForShutdown waits for the HTTP server to become unavailable
func (ts *TestServer) waitForShutdown(ctx context.Context, port int) error {
	url := fmt.Sprintf("http://localhost:%d/health", port)
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("timeout waiting for server to shutdown")
		case <-ticker.C:
			_, err := http.Get(url)
			if err != nil {
				// Server is not accessible, shutdown complete
				return nil
			}
		}
	}
}

// GetOutput returns the captured output
func (ts *TestServer) GetOutput() string {
	ts.mu.RLock()
	defer ts.mu.RUnlock()
	return ts.output.String()
}

// captureOutput captures output from the server
func (ts *TestServer) captureOutput(reader io.Reader, prefix string) {
	scanner := bufio.NewScanner(reader)
	for scanner.Scan() {
		select {
		case <-ts.done:
			// Shutdown signal received, exit goroutine
			return
		default:
			line := scanner.Text()
			ts.mu.Lock()
			ts.output.WriteString(fmt.Sprintf("[%s] %s\n", prefix, line))
			ts.mu.Unlock()
		}
	}
}
