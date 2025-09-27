package integration

import (
	"bytes"
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

// HTTPTestServer represents a server instance for HTTP transport testing
type HTTPTestServer struct {
	cmd    *exec.Cmd
	port   int
	cancel context.CancelFunc
	done   chan struct{}
	output strings.Builder
	mu     sync.RWMutex
}

// HTTPTestServerConfig holds configuration for HTTP test servers
type HTTPTestServerConfig struct {
	Port       int
	Tools      []string
	Kubeconfig string
	Timeout    time.Duration
}

// NewHTTPTestServer creates a new HTTP test server
func NewHTTPTestServer(config HTTPTestServerConfig) *HTTPTestServer {
	return &HTTPTestServer{
		port: config.Port,
		done: make(chan struct{}),
	}
}

// Start starts the HTTP test server
func (s *HTTPTestServer) Start(ctx context.Context, config HTTPTestServerConfig) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Build command arguments
	args := []string{"--port", fmt.Sprintf("%d", config.Port)}

	if len(config.Tools) > 0 {
		args = append(args, "--tools", strings.Join(config.Tools, ","))
	}

	if config.Kubeconfig != "" {
		args = append(args, "--kubeconfig", config.Kubeconfig)
	}

	// Create context with cancellation
	ctx, cancel := context.WithCancel(ctx)
	s.cancel = cancel

	// Start server process
	binaryPath := "../../bin/kagent-tools-" + getBinaryName()
	s.cmd = exec.CommandContext(ctx, binaryPath, args...)
	s.cmd.Env = append(os.Environ(), "LOG_LEVEL=debug")

	// Set up output capture
	stdout, err := s.cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("failed to create stdout pipe: %w", err)
	}

	stderr, err := s.cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("failed to create stderr pipe: %w", err)
	}

	// Start the command
	if err := s.cmd.Start(); err != nil {
		return fmt.Errorf("failed to start server: %w", err)
	}

	// Start goroutines to capture output
	go s.captureOutput(stdout, "STDOUT")
	go s.captureOutput(stderr, "STDERR")

	// Wait for server to start
	return s.waitForHTTPServer(ctx, config.Timeout)
}

// Stop stops the HTTP test server
func (s *HTTPTestServer) Stop() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.cancel != nil {
		s.cancel()
	}

	if s.cmd != nil && s.cmd.Process != nil {
		// Send interrupt signal for graceful shutdown
		if err := s.cmd.Process.Signal(os.Interrupt); err != nil {
			// If interrupt fails, kill the process
			_ = s.cmd.Process.Kill()
		}

		// Wait for process to exit with timeout
		done := make(chan error, 1)
		go func() {
			done <- s.cmd.Wait()
		}()

		select {
		case <-done:
			// Process exited
		case <-time.After(8 * time.Second):
			// Timeout, force kill
			_ = s.cmd.Process.Kill()
			select {
			case <-done:
			case <-time.After(2 * time.Second):
				// Force kill timeout, continue anyway
			}
		}
	}

	// Signal done and wait for goroutines to exit
	if s.done != nil {
		close(s.done)
	}

	// Give goroutines time to exit
	time.Sleep(100 * time.Millisecond)

	return nil
}

// GetOutput returns the captured output
func (s *HTTPTestServer) GetOutput() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.output.String()
}

// captureOutput captures output from the server
func (s *HTTPTestServer) captureOutput(reader io.Reader, prefix string) {
	buf := make([]byte, 1024)
	for {
		select {
		case <-s.done:
			return
		default:
			n, err := reader.Read(buf)
			if n > 0 {
				s.mu.Lock()
				s.output.WriteString(fmt.Sprintf("[%s] %s", prefix, string(buf[:n])))
				s.mu.Unlock()
			}
			if err != nil {
				return
			}
		}
	}
}

// waitForHTTPServer waits for the HTTP server to become available
func (s *HTTPTestServer) waitForHTTPServer(ctx context.Context, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	url := fmt.Sprintf("http://localhost:%d/health", s.port)
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

// HTTPMCPClient represents an HTTP client for MCP communication
type HTTPMCPClient struct {
	baseURL string
	client  *http.Client
}

// NewHTTPMCPClient creates a new HTTP MCP client
func NewHTTPMCPClient(baseURL string) *HTTPMCPClient {
	return &HTTPMCPClient{
		baseURL: baseURL,
		client:  &http.Client{Timeout: 30 * time.Second},
	}
}

// Initialize sends an initialize request to the MCP server
func (c *HTTPMCPClient) Initialize(ctx context.Context) (*mcp.InitializeResult, error) {
	params := map[string]interface{}{
		"protocolVersion": "2024-11-05",
		"clientInfo": map[string]interface{}{
			"name":    "test-client",
			"version": "1.0.0",
		},
		"capabilities": map[string]interface{}{},
	}

	var result mcp.InitializeResult
	err := c.sendJSONRPCRequest(ctx, "initialize", params, &result)
	return &result, err
}

// ListTools lists available MCP tools
func (c *HTTPMCPClient) ListTools(ctx context.Context) (*mcp.ListToolsResult, error) {
	params := map[string]interface{}{}
	var result mcp.ListToolsResult
	err := c.sendJSONRPCRequest(ctx, "tools/list", params, &result)
	return &result, err
}

// CallTool calls an MCP tool
func (c *HTTPMCPClient) CallTool(ctx context.Context, toolName string, arguments map[string]interface{}) (*mcp.CallToolResult, error) {
	params := map[string]interface{}{
		"name":      toolName,
		"arguments": arguments,
	}
	var result mcp.CallToolResult
	err := c.sendJSONRPCRequest(ctx, "tools/call", params, &result)
	return &result, err
}

// sendJSONRPCRequest sends a JSON-RPC request to the MCP server
func (c *HTTPMCPClient) sendJSONRPCRequest(ctx context.Context, method string, params interface{}, result interface{}) error {
	// Create JSON-RPC request
	jsonRPCRequest := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  method,
		"params":  params,
	}

	reqBody, err := json.Marshal(jsonRPCRequest)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	// Send HTTP request
	httpReq, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/mcp", bytes.NewReader(reqBody))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(httpReq)
	if err != nil {
		return fmt.Errorf("failed to make request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("HTTP error %d: %s", resp.StatusCode, string(body))
	}

	// Parse JSON-RPC response
	var jsonRPCResponse map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&jsonRPCResponse); err != nil {
		return fmt.Errorf("failed to decode response: %w", err)
	}

	// Check for JSON-RPC error
	if errorObj, exists := jsonRPCResponse["error"]; exists {
		return fmt.Errorf("JSON-RPC error: %v", errorObj)
	}

	// Extract result
	resultData, exists := jsonRPCResponse["result"]
	if !exists {
		return fmt.Errorf("no result in response")
	}

	// Marshal and unmarshal to convert to target type
	resultBytes, err := json.Marshal(resultData)
	if err != nil {
		return fmt.Errorf("failed to marshal result: %w", err)
	}

	if err := json.Unmarshal(resultBytes, result); err != nil {
		return fmt.Errorf("failed to unmarshal result: %w", err)
	}

	return nil
}

// TestHTTPTransportBasic tests basic HTTP transport functionality
func TestHTTPTransportBasic(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	config := HTTPTestServerConfig{
		Port:    8110,
		Tools:   []string{"utils"},
		Timeout: 30 * time.Second,
	}

	server := NewHTTPTestServer(config)
	err := server.Start(ctx, config)
	require.NoError(t, err, "Server should start successfully")
	defer server.Stop()

	// Wait for server to be ready
	time.Sleep(3 * time.Second)

	// Test health endpoint
	resp, err := http.Get(fmt.Sprintf("http://localhost:%d/health", config.Port))
	require.NoError(t, err, "Health endpoint should be accessible")
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	resp.Body.Close()
	assert.Equal(t, "OK", string(body))

	// Test metrics endpoint
	resp, err = http.Get(fmt.Sprintf("http://localhost:%d/metrics", config.Port))
	require.NoError(t, err, "Metrics endpoint should be accessible")
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	body, err = io.ReadAll(resp.Body)
	require.NoError(t, err)
	resp.Body.Close()

	metricsContent := string(body)
	assert.Contains(t, metricsContent, "go_")
	assert.Contains(t, metricsContent, "process_")
	assert.Contains(t, metricsContent, "go_memstats_alloc_bytes")
	assert.Contains(t, metricsContent, "go_goroutines")

	// Verify server output
	output := server.GetOutput()
	assert.Contains(t, output, "RegisterTools initialized")
	assert.Contains(t, output, "Running KAgent Tools Server")
}

// TestHTTPTransportMCPEndpoint tests the MCP endpoint (currently returns not implemented)
func TestHTTPTransportMCPEndpoint(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	config := HTTPTestServerConfig{
		Port:    8111,
		Tools:   []string{"utils"},
		Timeout: 20 * time.Second,
	}

	server := NewHTTPTestServer(config)
	err := server.Start(ctx, config)
	require.NoError(t, err, "Server should start successfully")
	defer server.Stop()

	// Wait for server to be ready
	time.Sleep(3 * time.Second)

	// Test MCP endpoint (should return not implemented for now)
	resp, err := http.Get(fmt.Sprintf("http://localhost:%d/mcp", config.Port))
	require.NoError(t, err, "MCP endpoint should be accessible")
	assert.Equal(t, http.StatusNotImplemented, resp.StatusCode)

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	resp.Body.Close()
	assert.Contains(t, string(body), "MCP HTTP transport not yet implemented")

	// TODO: Once HTTP transport is implemented, test actual MCP communication:
	//
	// client := NewHTTPMCPClient(fmt.Sprintf("http://localhost:%d", config.Port))
	//
	// // Test initialize
	// initResult, err := client.Initialize(ctx)
	// require.NoError(t, err, "Initialize should succeed")
	// assert.Equal(t, mcp.LATEST_PROTOCOL_VERSION, initResult.ProtocolVersion)
	// assert.Equal(t, "kagent-tools-server", initResult.ServerInfo.Name)
	//
	// // Test list tools
	// toolsResult, err := client.ListTools(ctx)
	// require.NoError(t, err, "List tools should succeed")
	// assert.Greater(t, len(toolsResult.Tools), 0, "Should have tools")
	//
	// // Find datetime tool
	// var datetimeTool *mcp.Tool
	// for _, tool := range toolsResult.Tools {
	//     if tool.Name == "datetime_get_current_time" {
	//         datetimeTool = &tool
	//         break
	//     }
	// }
	// require.NotNil(t, datetimeTool, "Should find datetime tool")
	//
	// // Test call tool
	// callResult, err := client.CallTool(ctx, "datetime_get_current_time", map[string]interface{}{})
	// require.NoError(t, err, "Tool call should succeed")
	// assert.False(t, callResult.IsError, "Tool call should not error")
	// assert.Greater(t, len(callResult.Content), 0, "Should have content")
}

// TestHTTPTransportConcurrentRequests tests concurrent HTTP requests
func TestHTTPTransportConcurrentRequests(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	config := HTTPTestServerConfig{
		Port:    8112,
		Tools:   []string{"utils"},
		Timeout: 30 * time.Second,
	}

	server := NewHTTPTestServer(config)
	err := server.Start(ctx, config)
	require.NoError(t, err, "Server should start successfully")
	defer server.Stop()

	// Wait for server to be ready
	time.Sleep(3 * time.Second)

	// Create multiple concurrent requests
	var wg sync.WaitGroup
	numRequests := 20
	results := make([]error, numRequests)

	for i := 0; i < numRequests; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			// Alternate between health and metrics endpoints
			var url string
			if id%2 == 0 {
				url = fmt.Sprintf("http://localhost:%d/health", config.Port)
			} else {
				url = fmt.Sprintf("http://localhost:%d/metrics", config.Port)
			}

			resp, err := http.Get(url)
			if err != nil {
				results[id] = err
				return
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				results[id] = fmt.Errorf("unexpected status code: %d", resp.StatusCode)
				return
			}

			// Read body to ensure complete response
			_, err = io.ReadAll(resp.Body)
			if err != nil {
				results[id] = err
			}
		}(i)
	}

	wg.Wait()

	// Verify all requests succeeded
	for i, err := range results {
		assert.NoError(t, err, "Concurrent request %d should succeed", i)
	}
}

// TestHTTPTransportLargeResponses tests handling of large responses
func TestHTTPTransportLargeResponses(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	config := HTTPTestServerConfig{
		Port:    8113,
		Tools:   []string{"utils"},
		Timeout: 20 * time.Second,
	}

	server := NewHTTPTestServer(config)
	err := server.Start(ctx, config)
	require.NoError(t, err, "Server should start successfully")
	defer server.Stop()

	// Wait for server to be ready
	time.Sleep(3 * time.Second)

	// Test metrics endpoint which should have a reasonably large response
	resp, err := http.Get(fmt.Sprintf("http://localhost:%d/metrics", config.Port))
	require.NoError(t, err, "Metrics endpoint should be accessible")
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	resp.Body.Close()

	metricsContent := string(body)
	assert.Greater(t, len(metricsContent), 100, "Metrics response should be reasonably large")
	assert.Contains(t, metricsContent, "go_memstats_alloc_bytes")
	assert.Contains(t, metricsContent, "go_memstats_total_alloc_bytes")
	assert.Contains(t, metricsContent, "go_memstats_sys_bytes")
	assert.Contains(t, metricsContent, "go_goroutines")
}

// TestHTTPTransportErrorHandling tests HTTP error handling
func TestHTTPTransportErrorHandling(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	config := HTTPTestServerConfig{
		Port:    8114,
		Tools:   []string{"utils"},
		Timeout: 20 * time.Second,
	}

	server := NewHTTPTestServer(config)
	err := server.Start(ctx, config)
	require.NoError(t, err, "Server should start successfully")
	defer server.Stop()

	// Wait for server to be ready
	time.Sleep(3 * time.Second)

	// Test non-existent endpoint
	resp, err := http.Get(fmt.Sprintf("http://localhost:%d/nonexistent", config.Port))
	require.NoError(t, err, "Request should complete")
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
	resp.Body.Close()

	// Test malformed POST request
	malformedJSON := "{invalid json"
	req, err := http.NewRequest("POST", fmt.Sprintf("http://localhost:%d/mcp", config.Port), strings.NewReader(malformedJSON))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err = client.Do(req)
	require.NoError(t, err)
	// Should return not implemented for now, but once implemented should handle malformed JSON gracefully
	assert.True(t, resp.StatusCode == http.StatusNotImplemented || resp.StatusCode == http.StatusBadRequest)
	resp.Body.Close()
}

// TestHTTPTransportMultipleTools tests HTTP transport with multiple tool categories
func TestHTTPTransportMultipleTools(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	allTools := []string{"utils", "k8s", "helm", "argo", "cilium", "istio", "prometheus"}

	config := HTTPTestServerConfig{
		Port:    8115,
		Tools:   allTools,
		Timeout: 30 * time.Second,
	}

	server := NewHTTPTestServer(config)
	err := server.Start(ctx, config)
	require.NoError(t, err, "Server should start successfully")
	defer server.Stop()

	// Wait for server to be ready
	time.Sleep(5 * time.Second)

	// Test health endpoint
	resp, err := http.Get(fmt.Sprintf("http://localhost:%d/health", config.Port))
	require.NoError(t, err, "Health endpoint should be accessible")
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	resp.Body.Close()

	// Verify server output contains all tool registrations
	output := server.GetOutput()
	assert.Contains(t, output, "RegisterTools initialized")
	assert.Contains(t, output, "Running KAgent Tools Server")

	// Verify each tool category appears in the output
	for _, tool := range allTools {
		assert.Contains(t, output, tool, "Tool %s should be registered", tool)
	}
}

// TestHTTPTransportGracefulShutdown tests graceful shutdown of HTTP server
func TestHTTPTransportGracefulShutdown(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	config := HTTPTestServerConfig{
		Port:    8116,
		Tools:   []string{"utils"},
		Timeout: 20 * time.Second,
	}

	server := NewHTTPTestServer(config)
	err := server.Start(ctx, config)
	require.NoError(t, err, "Server should start successfully")

	// Wait for server to be ready
	time.Sleep(2 * time.Second)

	// Test health endpoint to ensure server is running
	resp, err := http.Get(fmt.Sprintf("http://localhost:%d/health", config.Port))
	require.NoError(t, err, "Health endpoint should be accessible")
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	resp.Body.Close()

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

// TestHTTPTransportInvalidTools tests HTTP transport with invalid tool names
func TestHTTPTransportInvalidTools(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	config := HTTPTestServerConfig{
		Port:    8117,
		Tools:   []string{"invalid-tool", "utils"},
		Timeout: 20 * time.Second,
	}

	server := NewHTTPTestServer(config)
	err := server.Start(ctx, config)
	require.NoError(t, err, "Server should start even with invalid tools")
	defer server.Stop()

	// Wait for server to be ready
	time.Sleep(3 * time.Second)

	// Verify server is still accessible despite invalid tool
	resp, err := http.Get(fmt.Sprintf("http://localhost:%d/health", config.Port))
	require.NoError(t, err, "Health endpoint should be accessible")
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	resp.Body.Close()

	// Check server output for error about invalid tool
	output := server.GetOutput()
	assert.Contains(t, output, "Unknown tool specified")
	assert.Contains(t, output, "invalid-tool")

	// Valid tools should still be registered
	assert.Contains(t, output, "RegisterTools initialized")
	assert.Contains(t, output, "utils")
}

// TestHTTPTransportCustomKubeconfig tests HTTP transport with custom kubeconfig
func TestHTTPTransportCustomKubeconfig(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Create a temporary kubeconfig file
	tempDir := t.TempDir()
	kubeconfigPath := fmt.Sprintf("%s/kubeconfig", tempDir)

	kubeconfigContent := `apiVersion: v1
kind: Config
clusters:
- cluster:
    server: https://test-cluster
  name: test-cluster
contexts:
- context:
    cluster: test-cluster
    user: test-user
  name: test-context
current-context: test-context
users:
- name: test-user
  user:
    token: test-token
`

	err := os.WriteFile(kubeconfigPath, []byte(kubeconfigContent), 0644)
	require.NoError(t, err, "Should create temporary kubeconfig file")

	config := HTTPTestServerConfig{
		Port:       8118,
		Tools:      []string{"k8s"},
		Kubeconfig: kubeconfigPath,
		Timeout:    20 * time.Second,
	}

	server := NewHTTPTestServer(config)
	err = server.Start(ctx, config)
	require.NoError(t, err, "Server should start successfully")
	defer server.Stop()

	// Wait for server to be ready
	time.Sleep(3 * time.Second)

	// Test health endpoint
	resp, err := http.Get(fmt.Sprintf("http://localhost:%d/health", config.Port))
	require.NoError(t, err, "Health endpoint should be accessible")
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	resp.Body.Close()

	// Check server output for kubeconfig setting
	output := server.GetOutput()
	assert.Contains(t, output, "RegisterTools initialized")
	assert.Contains(t, output, "Running KAgent Tools Server")
}

// TestHTTPTransportContentTypes tests different content types
func TestHTTPTransportContentTypes(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	config := HTTPTestServerConfig{
		Port:    8119,
		Tools:   []string{"utils"},
		Timeout: 20 * time.Second,
	}

	server := NewHTTPTestServer(config)
	err := server.Start(ctx, config)
	require.NoError(t, err, "Server should start successfully")
	defer server.Stop()

	// Wait for server to be ready
	time.Sleep(3 * time.Second)

	// Test health endpoint content type
	resp, err := http.Get(fmt.Sprintf("http://localhost:%d/health", config.Port))
	require.NoError(t, err, "Health endpoint should be accessible")
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	resp.Body.Close()

	// Test metrics endpoint content type
	resp, err = http.Get(fmt.Sprintf("http://localhost:%d/metrics", config.Port))
	require.NoError(t, err, "Metrics endpoint should be accessible")
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "text/plain", resp.Header.Get("Content-Type"))
	resp.Body.Close()

	// Test MCP endpoint with JSON content type
	jsonData := `{"jsonrpc": "2.0", "id": 1, "method": "initialize", "params": {}}`
	req, err := http.NewRequest("POST", fmt.Sprintf("http://localhost:%d/mcp", config.Port), strings.NewReader(jsonData))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err = client.Do(req)
	require.NoError(t, err)
	// Should return not implemented for now
	assert.Equal(t, http.StatusNotImplemented, resp.StatusCode)
	resp.Body.Close()
}
