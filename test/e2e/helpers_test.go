package e2e

import (
	"bufio"
	"context"
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
	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/client/transport"
	"github.com/mark3labs/mcp-go/mcp"
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

// MCPClient represents a client for communicating with the MCP server using the official mcp-go client
type MCPClient struct {
	client *client.Client
	log    *slog.Logger
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

// GetMCPClient creates a new MCP client configured for the e2e test environment using the official mcp-go client
func GetMCPClient() (*MCPClient, error) {
	// Create HTTP transport for the MCP server with timeout long enough for operations like Istio installation
	httpTransport, err := transport.NewStreamableHTTP("http://127.0.0.1:30885/mcp", transport.WithHTTPTimeout(180*time.Second))
	if err != nil {
		return nil, fmt.Errorf("failed to create HTTP transport: %w", err)
	}

	// Create the official MCP client
	mcpClient := client.NewClient(httpTransport)

	// Start the client
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	if err := mcpClient.Start(ctx); err != nil {
		return nil, fmt.Errorf("failed to start MCP client: %w", err)
	}

	// Initialize the client
	initRequest := mcp.InitializeRequest{}
	initRequest.Params.ProtocolVersion = mcp.LATEST_PROTOCOL_VERSION
	initRequest.Params.ClientInfo = mcp.Implementation{
		Name:    "e2e-test-client",
		Version: "1.0.0",
	}
	initRequest.Params.Capabilities = mcp.ClientCapabilities{}

	_, err = mcpClient.Initialize(ctx, initRequest)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize MCP client: %w", err)
	}

	mcpHelper := &MCPClient{
		client: mcpClient,
		log:    slog.Default(),
	}

	// Validate connection by listing tools
	tools, err := mcpHelper.listTools()
	if len(tools) == 0 {
		return nil, fmt.Errorf("no tools found in MCP server: %w", err)
	}
	slog.Default().Info("MCP Client created", "baseURL", "http://127.0.0.1:30885/mcp", "tools", len(tools))
	return mcpHelper, err
}

// listTools calls the tools/list method to get available tools
func (c *MCPClient) listTools() ([]interface{}, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	request := mcp.ListToolsRequest{}
	result, err := c.client.ListTools(ctx, request)
	if err != nil {
		return nil, err
	}

	// Convert tools to interface{} slice for compatibility
	tools := make([]interface{}, len(result.Tools))
	for i, tool := range result.Tools {
		tools[i] = tool
	}

	return tools, nil
}

// k8sListResources calls the k8s_get_resources tool
func (c *MCPClient) k8sListResources(resourceType string) (interface{}, error) {
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

	request := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name:      "k8s_get_resources",
			Arguments: arguments,
		},
	}

	result, err := c.client.CallTool(ctx, request)
	if err != nil {
		return nil, err
	}
	if result.IsError {
		return nil, fmt.Errorf("tool call failed: %s", result.Content)
	}
	return result, nil
}

// helmListReleases calls the helm_list_releases tool
func (c *MCPClient) helmListReleases() (interface{}, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	type HelmArgs struct {
		AllNamespaces string `json:"all_namespaces"`
		Output        string `json:"output"`
	}

	arguments := HelmArgs{
		AllNamespaces: "true",
		Output:        "json",
	}

	request := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name:      "helm_list_releases",
			Arguments: arguments,
		},
	}

	result, err := c.client.CallTool(ctx, request)
	if err != nil {
		return nil, err
	}
	if result.IsError {
		return nil, fmt.Errorf("tool call failed: %s", result.Content)
	}
	return result, nil
}

// istioInstall calls the istio_install_istio tool
func (c *MCPClient) istioInstall(profile string) (interface{}, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second) // Istio install can take time
	defer cancel()

	type IstioArgs struct {
		Profile string `json:"profile"`
	}

	arguments := IstioArgs{
		Profile: profile,
	}

	request := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name:      "istio_install_istio",
			Arguments: arguments,
		},
	}

	result, err := c.client.CallTool(ctx, request)
	if err != nil {
		return nil, err
	}
	if result.IsError {
		return nil, fmt.Errorf("tool call failed: %s", result.Content)
	}
	return result, nil
}

// argoRolloutsList calls the argo_rollouts_get tool to list rollouts
func (c *MCPClient) argoRolloutsList(namespace string) (interface{}, error) {
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

	request := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name:      "argo_rollouts_list",
			Arguments: arguments,
		},
	}

	result, err := c.client.CallTool(ctx, request)
	if err != nil {
		return nil, err
	}
	if result.IsError {
		return nil, fmt.Errorf("tool call failed: %s", result.Content)
	}
	return result, nil
}

// ciliumStatus calls the cilium_status_and_version tool
func (c *MCPClient) ciliumStatus() (interface{}, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	request := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name:      "cilium_status_and_version",
			Arguments: nil,
		},
	}

	result, err := c.client.CallTool(ctx, request)
	if err != nil {
		return nil, err
	}
	return result, nil
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
