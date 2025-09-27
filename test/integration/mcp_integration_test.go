package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestServer represents a test server instance for integration testing
type TestServer struct {
	cmd    *exec.Cmd
	port   int
	stdio  bool
	cancel context.CancelFunc
	done   chan struct{}
	output strings.Builder
	mu     sync.RWMutex
}

// TestServerConfig holds configuration for integration test servers
type TestServerConfig struct {
	Port       int
	Tools      []string
	Kubeconfig string
	Stdio      bool
	Timeout    time.Duration
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
	binaryPath := "../../bin/kagent-tools-" + getBinaryName()
	ts.cmd = exec.CommandContext(ctx, binaryPath, args...)
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
		case <-time.After(8 * time.Second):
			// Timeout, force kill
			_ = ts.cmd.Process.Kill()
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

// GetOutput returns the captured output
func (ts *TestServer) GetOutput() string {
	ts.mu.RLock()
	defer ts.mu.RUnlock()
	return ts.output.String()
}

// captureOutput captures output from the server
func (ts *TestServer) captureOutput(reader io.Reader, prefix string) {
	buf := make([]byte, 1024)
	for {
		select {
		case <-ts.done:
			return
		default:
			n, err := reader.Read(buf)
			if n > 0 {
				ts.mu.Lock()
				ts.output.WriteString(fmt.Sprintf("[%s] %s", prefix, string(buf[:n])))
				ts.mu.Unlock()
			}
			if err != nil {
				return
			}
		}
	}
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

// MCPTestClient represents a test client for MCP communication
type MCPTestClient struct {
	baseURL string
	client  *http.Client
}

// NewMCPTestClient creates a new MCP test client
func NewMCPTestClient(baseURL string) *MCPTestClient {
	return &MCPTestClient{
		baseURL: baseURL,
		client:  &http.Client{Timeout: 30 * time.Second},
	}
}

// CallTool calls an MCP tool via HTTP
func (c *MCPTestClient) CallTool(ctx context.Context, toolName string, arguments map[string]interface{}) (*mcp.CallToolResult, error) {
	// Create JSON-RPC request manually since the SDK types are for internal use
	request := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "tools/call",
		"params": map[string]interface{}{
			"name":      toolName,
			"arguments": arguments,
		},
	}

	reqBody, err := json.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/mcp", strings.NewReader(string(reqBody)))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to make request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("HTTP error %d: %s", resp.StatusCode, string(body))
	}

	// Parse JSON-RPC response
	var jsonRPCResponse map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&jsonRPCResponse); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	// Check for JSON-RPC error
	if errorObj, exists := jsonRPCResponse["error"]; exists {
		return nil, fmt.Errorf("JSON-RPC error: %v", errorObj)
	}

	// Extract result and convert to CallToolResult
	resultData, exists := jsonRPCResponse["result"]
	if !exists {
		return nil, fmt.Errorf("no result in response")
	}

	// Marshal and unmarshal to convert to CallToolResult
	resultBytes, err := json.Marshal(resultData)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal result: %w", err)
	}

	var result mcp.CallToolResult
	if err := json.Unmarshal(resultBytes, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal result: %w", err)
	}

	return &result, nil
}

// ListTools lists available MCP tools
func (c *MCPTestClient) ListTools(ctx context.Context) ([]*mcp.Tool, error) {
	// Create JSON-RPC request manually
	request := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "tools/list",
		"params":  map[string]interface{}{},
	}

	reqBody, err := json.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/mcp", strings.NewReader(string(reqBody)))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to make request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("HTTP error %d: %s", resp.StatusCode, string(body))
	}

	// Parse JSON-RPC response
	var jsonRPCResponse map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&jsonRPCResponse); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	// Check for JSON-RPC error
	if errorObj, exists := jsonRPCResponse["error"]; exists {
		return nil, fmt.Errorf("JSON-RPC error: %v", errorObj)
	}

	// Extract result
	resultData, exists := jsonRPCResponse["result"]
	if !exists {
		return nil, fmt.Errorf("no result in response")
	}

	// Marshal and unmarshal to convert to ListToolsResult
	resultBytes, err := json.Marshal(resultData)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal result: %w", err)
	}

	var result mcp.ListToolsResult
	if err := json.Unmarshal(resultBytes, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal result: %w", err)
	}

	return result.Tools, nil
}

// TestMCPIntegrationHTTP tests MCP functionality over HTTP transport
func TestMCPIntegrationHTTP(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	config := TestServerConfig{
		Port:    8090,
		Tools:   []string{"utils", "k8s"},
		Stdio:   false,
		Timeout: 30 * time.Second,
	}

	server := NewTestServer(config)
	err := server.Start(ctx, config)
	require.NoError(t, err, "Server should start successfully")
	defer func() { _ = server.Stop() }()

	// Wait for server to be ready
	time.Sleep(3 * time.Second)

	// Test health endpoint
	resp, err := http.Get(fmt.Sprintf("http://localhost:%d/health", config.Port))
	require.NoError(t, err, "Health endpoint should be accessible")
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	_ = resp.Body.Close()

	// Test metrics endpoint
	resp, err = http.Get(fmt.Sprintf("http://localhost:%d/metrics", config.Port))
	require.NoError(t, err, "Metrics endpoint should be accessible")
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	_ = resp.Body.Close()

	metricsContent := string(body)
	assert.Contains(t, metricsContent, "go_")
	assert.Contains(t, metricsContent, "process_")

	// TODO: Test MCP endpoints once HTTP transport is implemented
	// For now, verify the placeholder response
	resp, err = http.Get(fmt.Sprintf("http://localhost:%d/mcp", config.Port))
	require.NoError(t, err, "MCP endpoint should be accessible")
	assert.Equal(t, http.StatusNotImplemented, resp.StatusCode)
	_ = resp.Body.Close()

	// Verify server output contains expected tool registrations
	output := server.GetOutput()
	assert.Contains(t, output, "RegisterTools initialized")
	assert.Contains(t, output, "Running KAgent Tools Server")
}

// TestMCPIntegrationStdio tests MCP functionality over stdio transport
func TestMCPIntegrationStdio(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	config := TestServerConfig{
		Tools:   []string{"utils"},
		Stdio:   true,
		Timeout: 10 * time.Second,
	}

	server := NewTestServer(config)
	err := server.Start(ctx, config)
	require.NoError(t, err, "Server should start successfully")
	defer func() { _ = server.Stop() }()

	// Wait for server to be ready
	time.Sleep(3 * time.Second)

	// Verify server output contains expected stdio mode message
	output := server.GetOutput()
	assert.Contains(t, output, "Running KAgent Tools Server STDIO")
	assert.Contains(t, output, "RegisterTools initialized")

	// TODO: Test actual stdio communication once transport is implemented
	// For now, verify the error message about unimplemented stdio transport
	assert.Contains(t, output, "Stdio transport not yet implemented with new SDK")
}

// TestToolRegistration tests that all tool categories register correctly
func TestToolRegistration(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	testCases := []struct {
		name  string
		tools []string
		port  int
	}{
		{
			name:  "utils_only",
			tools: []string{"utils"},
			port:  8091,
		},
		{
			name:  "k8s_only",
			tools: []string{"k8s"},
			port:  8092,
		},
		{
			name:  "multiple_tools",
			tools: []string{"utils", "k8s", "helm"},
			port:  8093,
		},
		{
			name:  "all_tools",
			tools: []string{},
			port:  8094,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			config := TestServerConfig{
				Port:    tc.port,
				Tools:   tc.tools,
				Stdio:   false,
				Timeout: 30 * time.Second,
			}

			server := NewTestServer(config)
			err := server.Start(ctx, config)
			require.NoError(t, err, "Server should start successfully")
			defer func() { _ = server.Stop() }()

			// Wait for server to be ready
			time.Sleep(3 * time.Second)

			// Test health endpoint
			resp, err := http.Get(fmt.Sprintf("http://localhost:%d/health", config.Port))
			require.NoError(t, err, "Health endpoint should be accessible")
			assert.Equal(t, http.StatusOK, resp.StatusCode)
			_ = resp.Body.Close()

			// Verify server output contains expected tool registrations
			output := server.GetOutput()
			assert.Contains(t, output, "RegisterTools initialized")
			assert.Contains(t, output, "Running KAgent Tools Server")

			// If specific tools were requested, verify they appear in output
			if len(tc.tools) > 0 {
				for _, tool := range tc.tools {
					assert.Contains(t, output, tool)
				}
			}
		})
	}
}

// TestServerGracefulShutdown tests that the server shuts down gracefully
func TestServerGracefulShutdown(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	config := TestServerConfig{
		Port:    8095,
		Tools:   []string{"utils"},
		Stdio:   false,
		Timeout: 10 * time.Second,
	}

	server := NewTestServer(config)
	err := server.Start(ctx, config)
	require.NoError(t, err, "Server should start successfully")

	// Wait for server to be ready
	time.Sleep(2 * time.Second)

	// Test health endpoint to ensure server is running
	resp, err := http.Get(fmt.Sprintf("http://localhost:%d/health", config.Port))
	require.NoError(t, err, "Health endpoint should be accessible")
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	_ = resp.Body.Close()

	// Stop server and measure shutdown time
	start := time.Now()
	err = server.Stop()
	duration := time.Since(start)

	require.NoError(t, err, "Server should stop gracefully")
	assert.Less(t, duration, 10*time.Second, "Shutdown should complete within reasonable time")

	// Verify server is no longer accessible
	time.Sleep(1 * time.Second)
	_, err = http.Get(fmt.Sprintf("http://localhost:%d/health", config.Port))
	assert.Error(t, err, "Server should no longer be accessible after shutdown")
}

// TestConcurrentRequests tests that the server handles concurrent requests correctly
func TestConcurrentRequests(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	config := TestServerConfig{
		Port:    8096,
		Tools:   []string{"utils"},
		Stdio:   false,
		Timeout: 30 * time.Second,
	}

	server := NewTestServer(config)
	err := server.Start(ctx, config)
	require.NoError(t, err, "Server should start successfully")
	defer func() { _ = server.Stop() }()

	// Wait for server to be ready
	time.Sleep(3 * time.Second)

	// Create multiple concurrent requests
	var wg sync.WaitGroup
	numRequests := 10
	results := make([]error, numRequests)

	for i := 0; i < numRequests; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			resp, err := http.Get(fmt.Sprintf("http://localhost:%d/health", config.Port))
			if err != nil {
				results[id] = err
				return
			}
			defer func() { _ = resp.Body.Close() }()
			if resp.StatusCode != http.StatusOK {
				results[id] = fmt.Errorf("unexpected status code: %d", resp.StatusCode)
			}
		}(i)
	}

	wg.Wait()

	// Verify all requests succeeded
	for i, err := range results {
		assert.NoError(t, err, "Concurrent request %d should succeed", i)
	}
}

// TestErrorHandling tests error handling scenarios
func TestErrorHandling(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	config := TestServerConfig{
		Port:    8097,
		Tools:   []string{"invalid-tool", "utils"},
		Stdio:   false,
		Timeout: 20 * time.Second,
	}

	server := NewTestServer(config)
	err := server.Start(ctx, config)
	require.NoError(t, err, "Server should start even with invalid tools")
	defer func() { _ = server.Stop() }()

	// Wait for server to be ready
	time.Sleep(3 * time.Second)

	// Verify server is still accessible despite invalid tool
	resp, err := http.Get(fmt.Sprintf("http://localhost:%d/health", config.Port))
	require.NoError(t, err, "Health endpoint should be accessible")
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	_ = resp.Body.Close()

	// Check server output for error about invalid tool
	output := server.GetOutput()
	assert.Contains(t, output, "Unknown tool specified")
	assert.Contains(t, output, "invalid-tool")

	// Valid tools should still be registered
	assert.Contains(t, output, "RegisterTools initialized")
	assert.Contains(t, output, "utils")
}

// TestEnvironmentVariables tests that environment variables are handled correctly
func TestEnvironmentVariables(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Set environment variables
	originalEnv := os.Environ()
	defer func() {
		os.Clearenv()
		for _, env := range originalEnv {
			parts := strings.SplitN(env, "=", 2)
			if len(parts) == 2 {
				_ = os.Setenv(parts[0], parts[1])
			}
		}
	}()

	_ = os.Setenv("LOG_LEVEL", "info")
	_ = os.Setenv("OTEL_SERVICE_NAME", "test-kagent-tools")

	config := TestServerConfig{
		Port:    8098,
		Tools:   []string{"utils"},
		Stdio:   false,
		Timeout: 20 * time.Second,
	}

	server := NewTestServer(config)
	err := server.Start(ctx, config)
	require.NoError(t, err, "Server should start successfully")
	defer func() { _ = server.Stop() }()

	// Wait for server to be ready
	time.Sleep(3 * time.Second)

	// Verify server is running
	resp, err := http.Get(fmt.Sprintf("http://localhost:%d/health", config.Port))
	require.NoError(t, err, "Health endpoint should be accessible")
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	_ = resp.Body.Close()

	// Check server output
	output := server.GetOutput()
	assert.Contains(t, output, "Starting kagent-tools-server")
}

// TestUtilsToolFunctionality tests specific utils tool functionality
func TestUtilsToolFunctionality(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	config := TestServerConfig{
		Port:    8099,
		Tools:   []string{"utils"},
		Stdio:   false,
		Timeout: 20 * time.Second,
	}

	server := NewTestServer(config)
	err := server.Start(ctx, config)
	require.NoError(t, err, "Server should start successfully")
	defer func() { _ = server.Stop() }()

	// Wait for server to be ready
	time.Sleep(3 * time.Second)

	// Verify server output contains utils tool registration
	output := server.GetOutput()
	assert.Contains(t, output, "RegisterTools initialized")
	assert.Contains(t, output, "utils")

	// TODO: Once HTTP transport is implemented, test actual tool calls:
	// client := NewMCPTestClient(fmt.Sprintf("http://localhost:%d", config.Port))
	//
	// Test datetime tool
	// result, err := client.CallTool(ctx, "datetime_get_current_time", map[string]interface{}{})
	// require.NoError(t, err)
	// assert.False(t, result.IsError)
	// assert.NotEmpty(t, result.Content)
	//
	// Test shell tool
	// result, err = client.CallTool(ctx, "shell", map[string]interface{}{
	//     "command": "echo hello",
	// })
	// require.NoError(t, err)
	// assert.False(t, result.IsError)
	// assert.Contains(t, result.Content[0].(*mcp.TextContent).Text, "hello")
}

// TestK8sToolFunctionality tests specific k8s tool functionality
func TestK8sToolFunctionality(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	config := TestServerConfig{
		Port:    8100,
		Tools:   []string{"k8s"},
		Stdio:   false,
		Timeout: 20 * time.Second,
	}

	server := NewTestServer(config)
	err := server.Start(ctx, config)
	require.NoError(t, err, "Server should start successfully")
	defer func() { _ = server.Stop() }()

	// Wait for server to be ready
	time.Sleep(3 * time.Second)

	// Verify server output contains k8s tool registration
	output := server.GetOutput()
	assert.Contains(t, output, "RegisterTools initialized")
	assert.Contains(t, output, "k8s")

	// TODO: Once HTTP transport is implemented, test actual k8s tool calls:
	// client := NewMCPTestClient(fmt.Sprintf("http://localhost:%d", config.Port))
	//
	// Test k8s_get_resources tool (this will fail without a real cluster, but we can test the call)
	// result, err := client.CallTool(ctx, "k8s_get_resources", map[string]interface{}{
	//     "resource_type": "pods",
	//     "output": "json",
	// })
	// The result will likely be an error due to no cluster, but the tool should be callable
}

// TestAllToolCategories tests that all tool categories can be registered
func TestAllToolCategories(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	allTools := []string{"utils", "k8s", "helm", "argo", "cilium", "istio", "prometheus"}

	config := TestServerConfig{
		Port:    8101,
		Tools:   allTools,
		Stdio:   false,
		Timeout: 30 * time.Second,
	}

	server := NewTestServer(config)
	err := server.Start(ctx, config)
	require.NoError(t, err, "Server should start successfully")
	defer func() { _ = server.Stop() }()

	// Wait for server to be ready
	time.Sleep(5 * time.Second)

	// Test health endpoint
	resp, err := http.Get(fmt.Sprintf("http://localhost:%d/health", config.Port))
	require.NoError(t, err, "Health endpoint should be accessible")
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	_ = resp.Body.Close()

	// Verify server output contains all tool registrations
	output := server.GetOutput()
	assert.Contains(t, output, "RegisterTools initialized")
	assert.Contains(t, output, "Running KAgent Tools Server")

	// Verify each tool category appears in the output
	for _, tool := range allTools {
		assert.Contains(t, output, tool, "Tool %s should be registered", tool)
	}
}

// Helper function to ensure binary exists before running tests
func init() {
	binaryPath := "../../bin/kagent-tools-" + getBinaryName()
	if _, err := os.Stat(binaryPath); os.IsNotExist(err) {
		// Try to build the binary
		cmd := exec.Command("make", "build")
		cmd.Dir = "../.."
		if err := cmd.Run(); err != nil {
			fmt.Printf("Warning: Failed to build server binary: %v\n", err)
		}
	}
}
