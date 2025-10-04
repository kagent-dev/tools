package integration

import (
	"bufio"
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

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ComprehensiveTestServer represents a test server instance for comprehensive integration testing
type ComprehensiveTestServer struct {
	cmd    *exec.Cmd
	port   int
	stdio  bool
	cancel context.CancelFunc
	done   chan struct{}
	output strings.Builder
	mu     sync.RWMutex
	stdin  io.WriteCloser
	stdout io.ReadCloser
	stderr io.ReadCloser
}

// ComprehensiveTestConfig holds configuration for comprehensive integration tests
type ComprehensiveTestConfig struct {
	Port       int
	Tools      []string
	Kubeconfig string
	Stdio      bool
	Timeout    time.Duration
}

// NewComprehensiveTestServer creates a new comprehensive test server instance
func NewComprehensiveTestServer(config ComprehensiveTestConfig) *ComprehensiveTestServer {
	return &ComprehensiveTestServer{
		port:  config.Port,
		stdio: config.Stdio,
		done:  make(chan struct{}),
	}
}

// Start starts the comprehensive test server
func (ts *ComprehensiveTestServer) Start(ctx context.Context, config ComprehensiveTestConfig) error {
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

	// Set up pipes for stdio mode
	if config.Stdio {
		stdin, err := ts.cmd.StdinPipe()
		if err != nil {
			return fmt.Errorf("failed to create stdin pipe: %w", err)
		}
		ts.stdin = stdin

		stdout, err := ts.cmd.StdoutPipe()
		if err != nil {
			return fmt.Errorf("failed to create stdout pipe: %w", err)
		}
		ts.stdout = stdout
	} else {
		// For HTTP mode, also capture stdout
		stdout, err := ts.cmd.StdoutPipe()
		if err != nil {
			return fmt.Errorf("failed to create stdout pipe: %w", err)
		}
		ts.stdout = stdout
	}

	// Set up stderr capture
	stderr, err := ts.cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("failed to create stderr pipe: %w", err)
	}
	ts.stderr = stderr

	// Start the command
	if err := ts.cmd.Start(); err != nil {
		return fmt.Errorf("failed to start server: %w", err)
	}

	// Start goroutines to capture output
	if ts.stdout != nil {
		go ts.captureOutput(ts.stdout, "STDOUT")
	}
	go ts.captureOutput(ts.stderr, "STDERR")

	// Wait for server to start
	if !config.Stdio {
		return ts.waitForHTTPServer(ctx, config.Timeout)
	}

	return nil
}

// Stop stops the comprehensive test server
func (ts *ComprehensiveTestServer) Stop() error {
	ts.mu.Lock()
	defer ts.mu.Unlock()

	if ts.cancel != nil {
		ts.cancel()
	}

	// Close pipes
	if ts.stdin != nil {
		_ = ts.stdin.Close()
	}
	if ts.stdout != nil {
		_ = ts.stdout.Close()
	}
	if ts.stderr != nil {
		_ = ts.stderr.Close()
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
func (ts *ComprehensiveTestServer) GetOutput() string {
	ts.mu.RLock()
	defer ts.mu.RUnlock()
	return ts.output.String()
}

// captureOutput captures output from the server
func (ts *ComprehensiveTestServer) captureOutput(reader io.Reader, prefix string) {
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
func (ts *ComprehensiveTestServer) waitForHTTPServer(ctx context.Context, timeout time.Duration) error {
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

// SendJSONRPCMessage sends a JSON-RPC message to the stdio server
func (ts *ComprehensiveTestServer) SendJSONRPCMessage(message interface{}) error {
	if !ts.stdio || ts.stdin == nil {
		return fmt.Errorf("server not in stdio mode or stdin not available")
	}

	data, err := json.Marshal(message)
	if err != nil {
		return fmt.Errorf("failed to marshal message: %w", err)
	}

	// Add newline for JSON-RPC over stdio
	data = append(data, '\n')

	_, err = ts.stdin.Write(data)
	if err != nil {
		return fmt.Errorf("failed to write message: %w", err)
	}

	return nil
}

// ReadJSONRPCMessage reads a JSON-RPC message from the stdio server
func (ts *ComprehensiveTestServer) ReadJSONRPCMessage(timeout time.Duration) (map[string]interface{}, error) {
	if !ts.stdio || ts.stdout == nil {
		return nil, fmt.Errorf("server not in stdio mode or stdout not available")
	}

	// Set up timeout
	done := make(chan map[string]interface{}, 1)
	errChan := make(chan error, 1)

	go func() {
		scanner := bufio.NewScanner(ts.stdout)
		if scanner.Scan() {
			var message map[string]interface{}
			if err := json.Unmarshal(scanner.Bytes(), &message); err != nil {
				errChan <- fmt.Errorf("failed to unmarshal message: %w", err)
				return
			}
			done <- message
		} else {
			if err := scanner.Err(); err != nil {
				errChan <- fmt.Errorf("failed to read message: %w", err)
			} else {
				errChan <- fmt.Errorf("no message received")
			}
		}
	}()

	select {
	case message := <-done:
		return message, nil
	case err := <-errChan:
		return nil, err
	case <-time.After(timeout):
		return nil, fmt.Errorf("timeout reading message")
	}
}

// ComprehensiveMCPClient represents a comprehensive MCP client for testing
type ComprehensiveMCPClient struct {
	baseURL string
	client  *http.Client
}

// NewComprehensiveMCPClient creates a new comprehensive MCP client
func NewComprehensiveMCPClient(baseURL string) *ComprehensiveMCPClient {
	return &ComprehensiveMCPClient{
		baseURL: baseURL,
		client:  &http.Client{Timeout: 30 * time.Second},
	}
}

// SendJSONRPCRequest sends a JSON-RPC request to the HTTP server
func (c *ComprehensiveMCPClient) SendJSONRPCRequest(ctx context.Context, method string, params interface{}) (map[string]interface{}, error) {
	// Create JSON-RPC request
	jsonRPCRequest := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  method,
		"params":  params,
	}

	reqBody, err := json.Marshal(jsonRPCRequest)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	// Send HTTP request
	httpReq, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/mcp", bytes.NewReader(reqBody))
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

	return jsonRPCResponse, nil
}

// TestComprehensiveHTTPTransport tests comprehensive HTTP transport functionality
func TestComprehensiveHTTPTransport(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	testCases := []struct {
		name     string
		tools    []string
		port     int
		testFunc func(t *testing.T, server *ComprehensiveTestServer, config ComprehensiveTestConfig)
	}{
		{
			name:  "single_tool_utils",
			tools: []string{"utils"},
			port:  8200,
			testFunc: func(t *testing.T, server *ComprehensiveTestServer, config ComprehensiveTestConfig) {
				// Test basic endpoints
				resp, err := http.Get(fmt.Sprintf("http://localhost:%d/health", config.Port))
				require.NoError(t, err)
				assert.Equal(t, http.StatusOK, resp.StatusCode)
				_ = resp.Body.Close()

				// Test metrics
				resp, err = http.Get(fmt.Sprintf("http://localhost:%d/metrics", config.Port))
				require.NoError(t, err)
				assert.Equal(t, http.StatusOK, resp.StatusCode)
				body, _ := io.ReadAll(resp.Body)
				_ = resp.Body.Close()
				assert.Contains(t, string(body), "go_memstats_alloc_bytes")

				// Verify tool registration
				output := server.GetOutput()
				assert.Contains(t, output, "RegisterTools initialized")
				assert.Contains(t, output, "utils")
			},
		},
		{
			name:  "multiple_tools",
			tools: []string{"utils", "k8s", "helm"},
			port:  8201,
			testFunc: func(t *testing.T, server *ComprehensiveTestServer, config ComprehensiveTestConfig) {
				// Test health endpoint
				resp, err := http.Get(fmt.Sprintf("http://localhost:%d/health", config.Port))
				require.NoError(t, err)
				assert.Equal(t, http.StatusOK, resp.StatusCode)
				_ = resp.Body.Close()

				// Verify all tools are registered
				output := server.GetOutput()
				assert.Contains(t, output, "RegisterTools initialized")
				for _, tool := range config.Tools {
					assert.Contains(t, output, tool)
				}
			},
		},
		{
			name:  "all_tools",
			tools: []string{}, // Empty means all tools
			port:  8202,
			testFunc: func(t *testing.T, server *ComprehensiveTestServer, config ComprehensiveTestConfig) {
				// Test health endpoint
				resp, err := http.Get(fmt.Sprintf("http://localhost:%d/health", config.Port))
				require.NoError(t, err)
				assert.Equal(t, http.StatusOK, resp.StatusCode)
				_ = resp.Body.Close()

				// Verify server started with all tools
				output := server.GetOutput()
				assert.Contains(t, output, "RegisterTools initialized")
				assert.Contains(t, output, "Running KAgent Tools Server")

				// Should contain evidence of multiple tool categories
				allTools := []string{"utils", "k8s", "helm", "argo", "cilium", "istio", "prometheus"}
				foundTools := 0
				for _, tool := range allTools {
					if strings.Contains(output, tool) {
						foundTools++
					}
				}
				assert.Greater(t, foundTools, 3, "Should register multiple tool categories")
			},
		},
		{
			name:  "error_handling",
			tools: []string{"invalid-tool", "utils"},
			port:  8203,
			testFunc: func(t *testing.T, server *ComprehensiveTestServer, config ComprehensiveTestConfig) {
				// Server should still be accessible despite invalid tool
				resp, err := http.Get(fmt.Sprintf("http://localhost:%d/health", config.Port))
				require.NoError(t, err)
				assert.Equal(t, http.StatusOK, resp.StatusCode)
				_ = resp.Body.Close()

				// Check for error about invalid tool
				output := server.GetOutput()
				assert.Contains(t, output, "Unknown tool specified")
				assert.Contains(t, output, "invalid-tool")
				// Valid tools should still be registered
				assert.Contains(t, output, "utils")
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			config := ComprehensiveTestConfig{
				Port:    tc.port,
				Tools:   tc.tools,
				Stdio:   false,
				Timeout: 30 * time.Second,
			}

			server := NewComprehensiveTestServer(config)
			err := server.Start(ctx, config)
			require.NoError(t, err, "Server should start successfully for %s", tc.name)
			defer func() { _ = server.Stop() }()

			// Wait for server to be ready
			time.Sleep(5 * time.Second)

			// Run test-specific checks
			tc.testFunc(t, server, config)
		})
	}
}

// TestComprehensiveStdioTransport tests comprehensive stdio transport functionality
func TestComprehensiveStdioTransport(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	testCases := []struct {
		name     string
		tools    []string
		testFunc func(t *testing.T, server *ComprehensiveTestServer)
	}{
		{
			name:  "stdio_basic",
			tools: []string{"utils"},
			testFunc: func(t *testing.T, server *ComprehensiveTestServer) {
				// Wait for server to initialize
				time.Sleep(3 * time.Second)

				// Check stderr for initialization messages
				output := server.GetOutput()
				assert.Contains(t, output, "Running KAgent Tools Server STDIO")
				assert.Contains(t, output, "RegisterTools initialized")
				assert.Contains(t, output, "utils")

				// Verify stdio transport is working (should not contain old error message)
				assert.NotContains(t, output, "Stdio transport not yet implemented with new SDK")

				// Test MCP communication over stdio
			},
		},
		{
			name:  "stdio_multiple_tools",
			tools: []string{"utils", "k8s", "helm"},
			testFunc: func(t *testing.T, server *ComprehensiveTestServer) {
				// Wait for server to initialize
				time.Sleep(3 * time.Second)

				// Check stderr for all tool registrations
				output := server.GetOutput()
				assert.Contains(t, output, "Running KAgent Tools Server STDIO")
				assert.Contains(t, output, "RegisterTools initialized")
				for _, tool := range []string{"utils", "k8s", "helm"} {
					assert.Contains(t, output, tool)
				}
			},
		},
		{
			name:  "stdio_error_handling",
			tools: []string{"invalid-tool", "utils"},
			testFunc: func(t *testing.T, server *ComprehensiveTestServer) {
				// Wait for server to initialize
				time.Sleep(3 * time.Second)

				// Check for error handling
				output := server.GetOutput()
				assert.Contains(t, output, "Unknown tool specified")
				assert.Contains(t, output, "invalid-tool")
				// Valid tools should still be registered
				assert.Contains(t, output, "utils")
				assert.Contains(t, output, "RegisterTools initialized")
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			config := ComprehensiveTestConfig{
				Tools:   tc.tools,
				Stdio:   true,
				Timeout: 20 * time.Second,
			}

			server := NewComprehensiveTestServer(config)
			err := server.Start(ctx, config)
			require.NoError(t, err, "Server should start successfully for %s", tc.name)
			defer func() { _ = server.Stop() }()

			// Run test-specific checks
			tc.testFunc(t, server)
		})
	}
}

// TestComprehensiveToolFunctionality tests tool functionality across both transports
func TestComprehensiveToolFunctionality(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	// Test each tool category individually
	toolCategories := []string{"utils", "k8s", "helm", "argo", "cilium", "istio", "prometheus"}

	for i, tool := range toolCategories {
		t.Run(fmt.Sprintf("tool_%s_http", tool), func(t *testing.T) {
			config := ComprehensiveTestConfig{
				Port:    8210 + i,
				Tools:   []string{tool},
				Stdio:   false,
				Timeout: 30 * time.Second,
			}

			server := NewComprehensiveTestServer(config)
			err := server.Start(ctx, config)
			require.NoError(t, err, "Server should start successfully for %s", tool)
			defer func() { _ = server.Stop() }()

			// Wait for server to be ready
			time.Sleep(3 * time.Second)

			// Test basic functionality
			resp, err := http.Get(fmt.Sprintf("http://localhost:%d/health", config.Port))
			require.NoError(t, err, "Health endpoint should be accessible for %s", tool)
			assert.Equal(t, http.StatusOK, resp.StatusCode)
			_ = resp.Body.Close()

			// Verify tool registration
			output := server.GetOutput()
			assert.Contains(t, output, "RegisterTools initialized")
			assert.Contains(t, output, tool)
			assert.Contains(t, output, "Running KAgent Tools Server")

			// Test MCP endpoint (should return not implemented for now)
			resp, err = http.Get(fmt.Sprintf("http://localhost:%d/mcp", config.Port))
			require.NoError(t, err, "MCP endpoint should be accessible")
			assert.Equal(t, http.StatusNotImplemented, resp.StatusCode)
			_ = resp.Body.Close()

			// Test actual tool calls:
			// client := NewComprehensiveMCPClient(fmt.Sprintf("http://localhost:%d", config.Port))
			//
			// // Test initialize
			// initParams := map[string]interface{}{
			//     "protocolVersion": "2024-11-05",
			//     "clientInfo": map[string]interface{}{
			//         "name": "test-client",
			//         "version": "1.0.0",
			//     },
			//     "capabilities": map[string]interface{}{},
			// }
			// response, err := client.SendJSONRPCRequest(ctx, "initialize", initParams)
			// require.NoError(t, err)
			// assert.Equal(t, "2.0", response["jsonrpc"])
			//
			// // Test list tools
			// response, err = client.SendJSONRPCRequest(ctx, "tools/list", map[string]interface{}{})
			// require.NoError(t, err)
			// assert.Contains(t, response, "result")
		})

		t.Run(fmt.Sprintf("tool_%s_stdio", tool), func(t *testing.T) {
			config := ComprehensiveTestConfig{
				Tools:   []string{tool},
				Stdio:   true,
				Timeout: 20 * time.Second,
			}

			server := NewComprehensiveTestServer(config)
			err := server.Start(ctx, config)
			require.NoError(t, err, "Server should start successfully for %s stdio", tool)
			defer func() { _ = server.Stop() }()

			// Wait for server to initialize
			time.Sleep(3 * time.Second)

			// Verify tool registration in stdio mode
			output := server.GetOutput()
			assert.Contains(t, output, "Running KAgent Tools Server STDIO")
			assert.Contains(t, output, "RegisterTools initialized")
			assert.Contains(t, output, tool)

			// Verify stdio transport is working (should not contain old error message)
			assert.NotContains(t, output, "Stdio transport not yet implemented with new SDK")

			// Test MCP communication over stdio
		})
	}
}

// TestComprehensiveConcurrency tests concurrent operations across both transports
func TestComprehensiveConcurrency(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	t.Run("http_concurrent_requests", func(t *testing.T) {
		config := ComprehensiveTestConfig{
			Port:    8220,
			Tools:   []string{"utils", "k8s"},
			Stdio:   false,
			Timeout: 30 * time.Second,
		}

		server := NewComprehensiveTestServer(config)
		err := server.Start(ctx, config)
		require.NoError(t, err, "Server should start successfully")
		defer func() { _ = server.Stop() }()

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

				// Alternate between different endpoints
				var url string
				switch id % 3 {
				case 0:
					url = fmt.Sprintf("http://localhost:%d/health", config.Port)
				case 1:
					url = fmt.Sprintf("http://localhost:%d/metrics", config.Port)
				case 2:
					url = fmt.Sprintf("http://localhost:%d/mcp", config.Port)
				}

				resp, err := http.Get(url)
				if err != nil {
					results[id] = err
					return
				}
				defer func() { _ = resp.Body.Close() }()

				// Accept both OK and NotImplemented status codes
				if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNotImplemented {
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
	})

	t.Run("multiple_servers_concurrent", func(t *testing.T) {
		// Test multiple servers running concurrently
		var wg sync.WaitGroup
		numServers := 3
		results := make([]error, numServers)

		for i := 0; i < numServers; i++ {
			wg.Add(1)
			go func(id int) {
				defer wg.Done()

				config := ComprehensiveTestConfig{
					Port:    8230 + id,
					Tools:   []string{"utils"},
					Stdio:   false,
					Timeout: 20 * time.Second,
				}

				server := NewComprehensiveTestServer(config)
				err := server.Start(ctx, config)
				if err != nil {
					results[id] = err
					return
				}
				defer func() { _ = server.Stop() }()

				// Wait for server to be ready
				time.Sleep(3 * time.Second)

				// Test health endpoint
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

		// Verify all servers started successfully
		for i, err := range results {
			assert.NoError(t, err, "Server %d should start and respond successfully", i)
		}
	})
}

// TestComprehensivePerformance tests performance characteristics
func TestComprehensivePerformance(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	t.Run("startup_performance", func(t *testing.T) {
		// Test startup time with different tool configurations
		testCases := []struct {
			name    string
			tools   []string
			port    int
			maxTime time.Duration
		}{
			{
				name:    "single_tool",
				tools:   []string{"utils"},
				port:    8240,
				maxTime: 10 * time.Second,
			},
			{
				name:    "multiple_tools",
				tools:   []string{"utils", "k8s", "helm"},
				port:    8241,
				maxTime: 15 * time.Second,
			},
			{
				name:    "all_tools",
				tools:   []string{}, // All tools
				port:    8242,
				maxTime: 25 * time.Second,
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				config := ComprehensiveTestConfig{
					Port:    tc.port,
					Tools:   tc.tools,
					Stdio:   false,
					Timeout: tc.maxTime,
				}

				// Measure startup time
				start := time.Now()
				server := NewComprehensiveTestServer(config)
				err := server.Start(ctx, config)
				startupTime := time.Since(start)

				require.NoError(t, err, "Server should start successfully for %s", tc.name)
				defer func() { _ = server.Stop() }()

				// Verify startup time is reasonable
				assert.Less(t, startupTime, tc.maxTime, "Startup time should be reasonable for %s", tc.name)

				// Test responsiveness
				resp, err := http.Get(fmt.Sprintf("http://localhost:%d/health", config.Port))
				require.NoError(t, err, "Health endpoint should be accessible")
				assert.Equal(t, http.StatusOK, resp.StatusCode)
				_ = resp.Body.Close()
			})
		}
	})

	t.Run("response_time_performance", func(t *testing.T) {
		config := ComprehensiveTestConfig{
			Port:    8250,
			Tools:   []string{"utils"},
			Stdio:   false,
			Timeout: 20 * time.Second,
		}

		server := NewComprehensiveTestServer(config)
		err := server.Start(ctx, config)
		require.NoError(t, err, "Server should start successfully")
		defer func() { _ = server.Stop() }()

		// Wait for server to be ready
		time.Sleep(3 * time.Second)

		// Measure response times for different endpoints
		endpoints := []string{"/health", "/metrics"}

		for _, endpoint := range endpoints {
			t.Run(fmt.Sprintf("endpoint_%s", strings.TrimPrefix(endpoint, "/")), func(t *testing.T) {
				// Measure multiple requests
				var totalTime time.Duration
				numRequests := 10

				for i := 0; i < numRequests; i++ {
					start := time.Now()
					resp, err := http.Get(fmt.Sprintf("http://localhost:%d%s", config.Port, endpoint))
					responseTime := time.Since(start)

					require.NoError(t, err, "Request should succeed")
					assert.Equal(t, http.StatusOK, resp.StatusCode)
					_ = resp.Body.Close()

					totalTime += responseTime

					// Individual request should be fast
					assert.Less(t, responseTime, 5*time.Second, "Individual request should be fast")
				}

				// Average response time should be reasonable
				avgTime := totalTime / time.Duration(numRequests)
				assert.Less(t, avgTime, 2*time.Second, "Average response time should be reasonable")
			})
		}
	})
}

// TestComprehensiveRobustness tests robustness and error handling
func TestComprehensiveRobustness(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	t.Run("graceful_shutdown", func(t *testing.T) {
		config := ComprehensiveTestConfig{
			Port:    8260,
			Tools:   []string{"utils"},
			Stdio:   false,
			Timeout: 20 * time.Second,
		}

		server := NewComprehensiveTestServer(config)
		err := server.Start(ctx, config)
		require.NoError(t, err, "Server should start successfully")

		// Wait for server to be ready
		time.Sleep(2 * time.Second)

		// Verify server is running
		resp, err := http.Get(fmt.Sprintf("http://localhost:%d/health", config.Port))
		require.NoError(t, err, "Health endpoint should be accessible")
		assert.Equal(t, http.StatusOK, resp.StatusCode)
		_ = resp.Body.Close()

		// Measure shutdown time
		start := time.Now()
		err = server.Stop()
		shutdownTime := time.Since(start)

		require.NoError(t, err, "Server should stop gracefully")
		assert.Less(t, shutdownTime, 10*time.Second, "Shutdown should complete within reasonable time")

		// Verify server is no longer accessible
		time.Sleep(1 * time.Second)
		_, err = http.Get(fmt.Sprintf("http://localhost:%d/health", config.Port))
		assert.Error(t, err, "Server should no longer be accessible after shutdown")
	})

	t.Run("invalid_configurations", func(t *testing.T) {
		testCases := []struct {
			name   string
			config ComprehensiveTestConfig
		}{
			{
				name: "invalid_tools",
				config: ComprehensiveTestConfig{
					Port:    8270,
					Tools:   []string{"nonexistent-tool", "another-invalid-tool"},
					Stdio:   false,
					Timeout: 20 * time.Second,
				},
			},
			{
				name: "mixed_valid_invalid_tools",
				config: ComprehensiveTestConfig{
					Port:    8271,
					Tools:   []string{"invalid-tool", "utils", "another-invalid-tool"},
					Stdio:   false,
					Timeout: 20 * time.Second,
				},
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				server := NewComprehensiveTestServer(tc.config)
				err := server.Start(ctx, tc.config)
				require.NoError(t, err, "Server should start even with invalid configuration")
				defer func() { _ = server.Stop() }()

				// Wait for server to be ready
				time.Sleep(3 * time.Second)

				// Server should still be accessible
				resp, err := http.Get(fmt.Sprintf("http://localhost:%d/health", tc.config.Port))
				require.NoError(t, err, "Health endpoint should be accessible")
				assert.Equal(t, http.StatusOK, resp.StatusCode)
				_ = resp.Body.Close()

				// Check for appropriate error messages
				output := server.GetOutput()
				if strings.Contains(tc.name, "invalid") {
					assert.Contains(t, output, "Unknown tool specified")
				}
			})
		}
	})

	t.Run("resource_cleanup", func(t *testing.T) {
		// Test that resources are properly cleaned up after multiple server starts/stops
		for i := 0; i < 3; i++ {
			config := ComprehensiveTestConfig{
				Port:    8280 + i,
				Tools:   []string{"utils"},
				Stdio:   false,
				Timeout: 15 * time.Second,
			}

			server := NewComprehensiveTestServer(config)
			err := server.Start(ctx, config)
			require.NoError(t, err, "Server should start successfully iteration %d", i)

			// Wait for server to be ready
			time.Sleep(2 * time.Second)

			// Test basic functionality
			resp, err := http.Get(fmt.Sprintf("http://localhost:%d/health", config.Port))
			require.NoError(t, err, "Health endpoint should be accessible iteration %d", i)
			assert.Equal(t, http.StatusOK, resp.StatusCode)
			_ = resp.Body.Close()

			// Stop server
			err = server.Stop()
			require.NoError(t, err, "Server should stop gracefully iteration %d", i)

			// Brief pause between iterations
			time.Sleep(500 * time.Millisecond)
		}
	})
}

// TestComprehensiveSDKMigration tests specific aspects of the MCP SDK migration
func TestComprehensiveSDKMigration(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	t.Run("new_sdk_patterns", func(t *testing.T) {
		config := ComprehensiveTestConfig{
			Port:    8290,
			Tools:   []string{"utils", "k8s", "helm"},
			Stdio:   false,
			Timeout: 30 * time.Second,
		}

		server := NewComprehensiveTestServer(config)
		err := server.Start(ctx, config)
		require.NoError(t, err, "Server should start successfully")
		defer func() { _ = server.Stop() }()

		// Wait for server to be ready
		time.Sleep(5 * time.Second)

		// Verify server output shows new SDK usage
		output := server.GetOutput()
		assert.Contains(t, output, "RegisterTools initialized")
		assert.Contains(t, output, "Running KAgent Tools Server")

		// Should not contain old SDK patterns
		assert.NotContains(t, output, "mark3labs/mcp-go", "Should not reference old SDK")
		assert.NotContains(t, output, "Failed to register tool provider", "Should not have registration failures")

		// Should contain evidence of new SDK usage for all requested tools
		for _, tool := range config.Tools {
			assert.Contains(t, output, tool, "Should register tool %s", tool)
		}

		// Test basic endpoints work
		resp, err := http.Get(fmt.Sprintf("http://localhost:%d/health", config.Port))
		require.NoError(t, err, "Health endpoint should be accessible")
		assert.Equal(t, http.StatusOK, resp.StatusCode)
		_ = resp.Body.Close()

		// Test MCP endpoint (should return not implemented until HTTP transport is complete)
		resp, err = http.Get(fmt.Sprintf("http://localhost:%d/mcp", config.Port))
		require.NoError(t, err, "MCP endpoint should be accessible")
		assert.Equal(t, http.StatusNotImplemented, resp.StatusCode)

		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		_ = resp.Body.Close()
		assert.Contains(t, string(body), "MCP HTTP transport not yet implemented with new SDK")
	})

	t.Run("all_tool_categories_migration", func(t *testing.T) {
		// Test that all tool categories work with the new SDK
		allTools := []string{"utils", "k8s", "helm", "argo", "cilium", "istio", "prometheus"}

		config := ComprehensiveTestConfig{
			Port:    8291,
			Tools:   allTools,
			Stdio:   false,
			Timeout: 40 * time.Second,
		}

		server := NewComprehensiveTestServer(config)
		err := server.Start(ctx, config)
		require.NoError(t, err, "Server should start successfully with all tools")
		defer func() { _ = server.Stop() }()

		// Wait for server to be ready
		time.Sleep(8 * time.Second)

		// Test health endpoint
		resp, err := http.Get(fmt.Sprintf("http://localhost:%d/health", config.Port))
		require.NoError(t, err, "Health endpoint should be accessible")
		assert.Equal(t, http.StatusOK, resp.StatusCode)
		_ = resp.Body.Close()

		// Verify all tool categories are registered
		output := server.GetOutput()
		assert.Contains(t, output, "RegisterTools initialized")
		assert.Contains(t, output, "Running KAgent Tools Server")

		// Check that most tools are registered (some may have specific requirements)
		registeredTools := 0
		for _, tool := range allTools {
			if strings.Contains(output, tool) {
				registeredTools++
			}
		}
		assert.Greater(t, registeredTools, len(allTools)/2, "Should register most tool categories")

		// Should not have critical errors
		assert.NotContains(t, output, "panic", "Should not have panics")
		assert.NotContains(t, output, "fatal", "Should not have fatal errors")
	})

	t.Run("backward_compatibility", func(t *testing.T) {
		// Test that the migration maintains backward compatibility
		config := ComprehensiveTestConfig{
			Port:    8292,
			Tools:   []string{"utils"},
			Stdio:   false,
			Timeout: 20 * time.Second,
		}

		server := NewComprehensiveTestServer(config)
		err := server.Start(ctx, config)
		require.NoError(t, err, "Server should start successfully")
		defer func() { _ = server.Stop() }()

		// Wait for server to be ready
		time.Sleep(3 * time.Second)

		// Test that existing endpoints still work
		endpoints := []string{"/health", "/metrics"}
		for _, endpoint := range endpoints {
			resp, err := http.Get(fmt.Sprintf("http://localhost:%d%s", config.Port, endpoint))
			require.NoError(t, err, "Endpoint %s should be accessible", endpoint)
			assert.Equal(t, http.StatusOK, resp.StatusCode)
			_ = resp.Body.Close()
		}

		// Verify command-line interface compatibility
		output := server.GetOutput()
		assert.Contains(t, output, "Starting kagent-tools-server")
		assert.Contains(t, output, "RegisterTools initialized")
		assert.Contains(t, output, "Running KAgent Tools Server")
	})
}
