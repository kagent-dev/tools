package e2e

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/kagent-dev/tools/internal/commands"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

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

// closeBody closes the response body while ignoring the returned error.
func closeBody(b io.ReadCloser) {
	if b != nil {
		_ = b.Close()
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

// MCPClient represents a client for communicating with the MCP server
// Uses official github.com/modelcontextprotocol/go-sdk v1.0.0
type MCPClient struct {
	client    *mcp.Client
	session   *mcp.ClientSession
	serverURL string
	timeout   time.Duration
	logger    *slog.Logger
}

// MCPClientOptions configures the MCPClient
type MCPClientOptions struct {
	ServerURL string
	Timeout   time.Duration
	Logger    *slog.Logger
}

// InstallKAgentTools installs KAgent Tools using helm in the specified namespace
func InstallKAgentTools(namespace string, releaseName string) {
	// Use longer timeout for helm installation as it can take time to pull images
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
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
	output, err := commands.NewCommandBuilder("helm").
		WithArgs("install", releaseName, "../../helm/kagent-tools").
		WithArgs("--namespace", namespace).
		WithArgs("-f").
		WithArgs("../../scripts/kind/test-values-e2e.yaml").
		WithArgs("--create-namespace").
		WithArgs("--debug").
		WithArgs("--wait").
		WithArgs("--timeout=1m").
		WithCache(false). // Don't cache helm installation
		Execute(ctx)

	Expect(err).ToNot(HaveOccurred(), "Failed to install KAgent Tools: %v %v", err, output)
	log.Info("KAgent Tools installation completed", "namespace", namespace, "output", output)

	// Verify the installation by checking if pods are running
	By("Verifying KAgent Tools pods are running")
	log.Info("Verifying KAgent Tools pods", "namespace", namespace)

	Eventually(func() bool {
		ctx, cancel := context.WithTimeout(context.Background(), DefaultTimeout)
		defer cancel()

		output, err := commands.NewCommandBuilder("kubectl").
			WithArgs("get", "pods", "-n", namespace, "-l", "app.kubernetes.io/name=kagent-tools", "-o", "jsonpath={.items[*].status.phase}").
			Execute(ctx)

		if err != nil {
			log.Error("Failed to get pod status", "error", err)
			return false
		}

		log.Info("Pod status check", "namespace", namespace, "output", output)
		// Check if all pods are in Running state
		return output == "Running" || (len(output) > 0 && !contains(output, "Pending") && !contains(output, "Failed"))
	}, 60*time.Second, 5*time.Second).Should(BeTrue(), "KAgent Tools pods should be running")

	log.Info("KAgent Tools pods are running", "namespace", namespace)
	//validate service nodePort == 30885
	By("Validating KAgent Tools service is accessible")
	nodePort, err := commands.NewCommandBuilder("kubectl").
		WithArgs("get", "svc", "-n", namespace, "-o", "jsonpath={.items[0].spec.ports[0].nodePort}").
		Execute(ctx)
	Expect(err).ToNot(HaveOccurred(), "Failed to get service nodePort: %v", err)
	Expect(nodePort).To(Equal("30885"))
}

// NewMCPClient creates a new MCP client for E2E testing
// Implements: T018 - Create MCPClient Struct and Constructor
func NewMCPClient(opts MCPClientOptions) (*MCPClient, error) {
	// Validate ServerURL
	if opts.ServerURL == "" {
		return nil, fmt.Errorf("invalid server URL: empty")
	}
	if !strings.HasPrefix(opts.ServerURL, "http://") && !strings.HasPrefix(opts.ServerURL, "https://") {
		return nil, fmt.Errorf("invalid server URL: %s (must start with http:// or https://)", opts.ServerURL)
	}

	// Validate Timeout
	if opts.Timeout <= 0 {
		return nil, fmt.Errorf("timeout must be > 0")
	}

	// Use provided logger or create default
	logger := opts.Logger
	if logger == nil {
		logger = slog.Default()
	}

	// Create MCP SDK client
	client := mcp.NewClient(&mcp.Implementation{
		Name:    "kagent-tools-e2e-client",
		Version: "1.0.0",
	}, nil)

	return &MCPClient{
		client:    client,
		serverURL: opts.ServerURL,
		timeout:   opts.Timeout,
		logger:    logger,
	}, nil
}

// Connect establishes connection to MCP server
// Implements: T019 - Implement MCPClient Connect Method
func (c *MCPClient) Connect(ctx context.Context) error {
	if c.session != nil {
		return fmt.Errorf("client already connected")
	}

	// Create HTTP transport for SSE endpoint
	transport := createHTTPTransport(c.serverURL)

	// Connect to server
	session, err := c.client.Connect(ctx, transport, nil)
	if err != nil {
		return fmt.Errorf("failed to connect to MCP server: %w", err)
	}

	c.session = session
	c.logger.Info("MCP client connected", "serverURL", c.serverURL)
	return nil
}

// Close closes the MCP session
// Implements: T020 - Implement MCPClient Close Method
func (c *MCPClient) Close() error {
	if c.session == nil {
		return nil // Already closed or never connected
	}

	err := c.session.Close()
	c.session = nil

	if err != nil {
		return fmt.Errorf("failed to close MCP session: %w", err)
	}

	c.logger.Info("MCP client closed")
	return nil
}

// ListTools retrieves available tools from server
// Implements: T021 - Implement MCPClient ListTools Method
func (c *MCPClient) ListTools(ctx context.Context) ([]*mcp.Tool, error) {
	if c.session == nil {
		return nil, fmt.Errorf("client not connected")
	}

	var tools []*mcp.Tool
	for tool, err := range c.session.Tools(ctx, nil) {
		if err != nil {
			return nil, fmt.Errorf("failed to list tools: %w", err)
		}
		tools = append(tools, tool)
	}

	c.logger.Info("Listed MCP tools", "count", len(tools))
	return tools, nil
}

// CallTool executes a tool with parameters
// Implements: T022 - Implement MCPClient CallTool Method
func (c *MCPClient) CallTool(ctx context.Context, name string, args map[string]any) (*mcp.CallToolResult, error) {
	if c.session == nil {
		return nil, fmt.Errorf("client not connected")
	}

	// Set timeout if not already in context
	if _, hasDeadline := ctx.Deadline(); !hasDeadline {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, c.timeout)
		defer cancel()
	}

	result, err := c.session.CallTool(ctx, &mcp.CallToolParams{
		Name:      name,
		Arguments: args,
	})

	if err != nil {
		return nil, fmt.Errorf("tool call failed: %w", err)
	}

	c.logger.Info("Tool called", "name", name, "isError", result.IsError)
	return result, nil
}

// k8sListResources calls the k8s_get_resources tool
// Implements: T023 - Implement k8sListResources Method
func (c *MCPClient) k8sListResources(resourceType string) (interface{}, error) {
	ctx, cancel := context.WithTimeout(context.Background(), c.timeout)
	defer cancel()

	return c.CallTool(ctx, "k8s_get_resources", map[string]any{
		"resource_type": resourceType,
		"namespace":     "default",
	})
}

// helmListReleases calls the helm_list_releases tool
// Implements: T024 - Implement helmListReleases Method
func (c *MCPClient) helmListReleases() (interface{}, error) {
	ctx, cancel := context.WithTimeout(context.Background(), c.timeout)
	defer cancel()

	return c.CallTool(ctx, "helm_list_releases", map[string]any{})
}

// istioVersion calls the istio_version tool
// Implements: T025 - Implement istioVersion Method
func (c *MCPClient) istioVersion() (interface{}, error) {
	ctx, cancel := context.WithTimeout(context.Background(), c.timeout)
	defer cancel()

	return c.CallTool(ctx, "istio_version", map[string]any{})
}

// argoRolloutsList calls the argo_rollouts_list tool to list rollouts
// Implements: T026 - Implement argoRolloutsList Method
func (c *MCPClient) argoRolloutsList(namespace string) (interface{}, error) {
	ctx, cancel := context.WithTimeout(context.Background(), c.timeout)
	defer cancel()

	return c.CallTool(ctx, "argo_rollouts_list", map[string]any{
		"namespace": namespace,
	})
}

// ciliumStatus calls the cilium_status_and_version tool
// Implements: T027 - Implement ciliumStatus Method
func (c *MCPClient) ciliumStatus() (interface{}, error) {
	ctx, cancel := context.WithTimeout(context.Background(), c.timeout)
	defer cancel()

	return c.CallTool(ctx, "cilium_status_and_version", map[string]any{})
}

// GetMCPClient creates a new MCP client configured for the e2e test environment
func GetMCPClient() (*MCPClient, error) {
	return NewMCPClient(MCPClientOptions{
		ServerURL: "http://localhost:30885/mcp",
		Timeout:   60 * time.Second,
		Logger:    slog.Default(),
	})
}

// createHTTPTransport creates an HTTP transport for MCP communication
// This helper is used by MCPClient and integration tests
func createHTTPTransport(serverURL string) mcp.Transport {
	// Parse the URL
	parsedURL, err := url.Parse(serverURL)
	if err != nil {
		panic(fmt.Sprintf("invalid server URL: %v", err))
	}

	// Create HTTP client
	httpClient := &http.Client{}

	// Create SSE client transport using the SDK
	// The SDK provides SSEClientTransport for HTTP/SSE communication
	transport := &mcp.SSEClientTransport{
		Endpoint:   parsedURL.String(),
		HTTPClient: httpClient,
	}

	return transport
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

	// First, check if the namespace already exists
	_, err := commands.NewCommandBuilder("kubectl").
		WithArgs("get", "namespace", namespace).
		WithCache(false).
		Execute(ctx)

	if err == nil {
		log.Info("Namespace already exists, skipping creation", "namespace", namespace)
		return
	}

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

// DeleteNamespace deletes a Kubernetes namespace
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
}

// contains checks if a string contains a substring
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && (s[:len(substr)] == substr || s[len(s)-len(substr):] == substr || containsHelper(s, substr)))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
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
