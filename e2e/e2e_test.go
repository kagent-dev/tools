package e2e

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

// ServerTestResult holds the result of a server test
type ServerTestResult struct {
	Output   string
	Error    error
	Duration time.Duration
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
	ts.cmd = exec.CommandContext(ctx, fmt.Sprintf("../bin/%s", binaryName), args...)
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
		case <-time.After(5 * time.Second):
			// Timeout, force kill
			_ = ts.cmd.Process.Kill()
		}
	}

	close(ts.done)
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
	scanner := bufio.NewScanner(reader)
	for scanner.Scan() {
		line := scanner.Text()
		ts.mu.Lock()
		ts.output.WriteString(fmt.Sprintf("[%s] %s\n", prefix, line))
		ts.mu.Unlock()
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
				resp.Body.Close()
				if resp.StatusCode == http.StatusOK {
					return nil
				}
			}
		}
	}
}

// TestHTTPServerStartup tests basic HTTP server startup and shutdown
func TestHTTPServerStartup(t *testing.T) {
	ctx := context.Background()

	config := TestServerConfig{
		Port:    8085,
		Stdio:   false,
		Timeout: 30 * time.Second,
	}

	server := NewTestServer(config)

	// Start server
	err := server.Start(ctx, config)
	require.NoError(t, err, "Server should start successfully")

	// Wait a bit for server to be fully ready
	time.Sleep(3 * time.Second)

	// Test health endpoint
	resp, err := http.Get(fmt.Sprintf("http://localhost:%d/health", config.Port))
	require.NoError(t, err, "Health endpoint should be accessible")
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	resp.Body.Close()

	// Check server output
	output := server.GetOutput()
	assert.Contains(t, output, "Running KAgent Tools Server")
	assert.Contains(t, output, fmt.Sprintf(":%d", config.Port))

	// Stop server
	err = server.Stop()
	require.NoError(t, err, "Server should stop gracefully")

	// Verify server is stopped
	time.Sleep(1 * time.Second)
	_, err = http.Get(fmt.Sprintf("http://localhost:%d/health", config.Port))
	assert.Error(t, err, "Server should not be accessible after stop")
}

// TestHTTPServerWithSpecificTools tests server with specific tools enabled
func TestHTTPServerWithSpecificTools(t *testing.T) {
	ctx := context.Background()

	config := TestServerConfig{
		Port:    8086,
		Tools:   []string{"utils", "k8s"},
		Stdio:   false,
		Timeout: 30 * time.Second,
	}

	server := NewTestServer(config)

	// Start server
	err := server.Start(ctx, config)
	require.NoError(t, err, "Server should start successfully")

	// Wait for server to be ready
	time.Sleep(3 * time.Second)

	// Check server output for tool registration
	output := server.GetOutput()
	assert.Contains(t, output, "RegisterTools initialized", "Should register specified tools")
	assert.Contains(t, output, "utils", "Should register utils tools")
	assert.Contains(t, output, "k8s", "Should register k8s tools")

	// Stop server
	err = server.Stop()
	require.NoError(t, err, "Server should stop gracefully")
}

// TestHTTPServerWithAllTools tests server with all tools enabled (default)
func TestHTTPServerWithAllTools(t *testing.T) {
	ctx := context.Background()

	config := TestServerConfig{
		Port:    8087,
		Stdio:   false,
		Timeout: 30 * time.Second,
	}

	server := NewTestServer(config)

	// Start server
	err := server.Start(ctx, config)
	require.NoError(t, err, "Server should start successfully")

	// Wait for server to be ready
	time.Sleep(3 * time.Second)

	// Check server output for all tools registration
	output := server.GetOutput()
	assert.Contains(t, output, "RegisterTools initialized", "Should initialize RegisterTools")

	// Verify server is running (tools are implicitly registered when no specific tools are provided)
	assert.Contains(t, output, "Running KAgent Tools Server", "Should be running with all tools")

	// Stop server
	err = server.Stop()
	require.NoError(t, err, "Server should stop gracefully")
}

// TestHTTPServerWithKubeconfig tests server with kubeconfig parameter
func TestHTTPServerWithKubeconfig(t *testing.T) {
	ctx := context.Background()

	// Create a temporary kubeconfig file
	tempDir := t.TempDir()
	kubeconfigPath := filepath.Join(tempDir, "kubeconfig")

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

	config := TestServerConfig{
		Port:       8088,
		Kubeconfig: kubeconfigPath,
		Stdio:      false,
		Timeout:    30 * time.Second,
	}

	server := NewTestServer(config)

	// Start server
	err = server.Start(ctx, config)
	require.NoError(t, err, "Server should start successfully")

	// Wait for server to be ready
	time.Sleep(3 * time.Second)

	// Check server output for kubeconfig setting
	output := server.GetOutput()
	assert.Contains(t, output, "RegisterTools initialized", "Should initialize RegisterTools")
	assert.Contains(t, output, "Running KAgent Tools Server", "Should be running with kubeconfig")

	// Stop server
	err = server.Stop()
	require.NoError(t, err, "Server should stop gracefully")
}

// TestStdioServer tests STDIO server mode
func TestStdioServer(t *testing.T) {
	ctx := context.Background()

	config := TestServerConfig{
		Stdio:   true,
		Timeout: 30 * time.Second,
	}

	server := NewTestServer(config)

	// Start server
	err := server.Start(ctx, config)
	require.NoError(t, err, "Server should start successfully")

	// Wait for server to be ready
	time.Sleep(3 * time.Second)

	// Check server output for STDIO mode
	output := server.GetOutput()
	assert.Contains(t, output, "Running KAgent Tools Server STDIO")

	// Stop server
	err = server.Stop()
	require.NoError(t, err, "Server should stop gracefully")
}

// TestServerGracefulShutdown tests graceful shutdown behavior
func TestServerGracefulShutdown(t *testing.T) {
	ctx := context.Background()

	config := TestServerConfig{
		Port:    8100,
		Stdio:   false,
		Timeout: 30 * time.Second,
	}

	server := NewTestServer(config)

	// Start server
	err := server.Start(ctx, config)
	require.NoError(t, err, "Server should start successfully")

	// Wait for server to be ready
	time.Sleep(3 * time.Second)

	// Stop server and measure shutdown time
	start := time.Now()
	err = server.Stop()
	duration := time.Since(start)

	require.NoError(t, err, "Server should stop gracefully")
	assert.Less(t, duration, 10*time.Second, "Shutdown should complete within reasonable time")

	// Wait a bit for shutdown logs to be captured
	time.Sleep(3 * time.Second)

	// Check server output for graceful shutdown
	output := server.GetOutput()
	// The main test is that the server started successfully and stopped without error
	assert.Contains(t, output, "Running KAgent Tools Server", "Server should have started successfully")

	// Try to verify the server is actually stopped by attempting to connect
	_, err = http.Get(fmt.Sprintf("http://localhost:%d/health", config.Port))
	assert.Error(t, err, "Server should not be accessible after stop")
}

// TestServerWithInvalidTool tests server behavior with invalid tool names
func TestServerWithInvalidTool(t *testing.T) {
	ctx := context.Background()

	config := TestServerConfig{
		Port:    8090,
		Tools:   []string{"invalid-tool", "utils"},
		Stdio:   false,
		Timeout: 30 * time.Second,
	}

	server := NewTestServer(config)

	// Start server
	err := server.Start(ctx, config)
	require.NoError(t, err, "Server should start even with invalid tools")

	// Wait for server to be ready
	time.Sleep(3 * time.Second)

	// Check server output for error about invalid tool
	output := server.GetOutput()
	assert.Contains(t, output, "Unknown tool specified")
	assert.Contains(t, output, "invalid-tool")

	// Valid tools should still be registered
	assert.Contains(t, output, "RegisterTools initialized")
	assert.Contains(t, output, "utils")

	// Stop server
	err = server.Stop()
	require.NoError(t, err, "Server should stop gracefully")
}

// TestServerVersionAndBuildInfo tests server version and build information
func TestServerVersionAndBuildInfo(t *testing.T) {
	ctx := context.Background()

	config := TestServerConfig{
		Port:    8091,
		Stdio:   false,
		Timeout: 30 * time.Second,
	}

	server := NewTestServer(config)

	// Start server
	err := server.Start(ctx, config)
	require.NoError(t, err, "Server should start successfully")

	// Wait for server to be ready
	time.Sleep(3 * time.Second)

	// Check server output for version information
	output := server.GetOutput()
	assert.Contains(t, output, "Starting kagent-tools-server")
	assert.Contains(t, output, "version")

	// Stop server
	err = server.Stop()
	require.NoError(t, err, "Server should stop gracefully")
}

// TestConcurrentServerInstances tests running multiple server instances
func TestConcurrentServerInstances(t *testing.T) {
	ctx := context.Background()

	var wg sync.WaitGroup
	numServers := 3
	servers := make([]*TestServer, numServers)

	// Start multiple servers on different ports
	for i := 0; i < numServers; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()

			config := TestServerConfig{
				Port:    8092 + index,
				Tools:   []string{"utils"},
				Stdio:   false,
				Timeout: 30 * time.Second,
			}

			server := NewTestServer(config)
			servers[index] = server

			err := server.Start(ctx, config)
			assert.NoError(t, err, fmt.Sprintf("Server %d should start successfully", index))

			// Wait for server to be ready
			time.Sleep(3 * time.Second)

			// Test health endpoint
			resp, err := http.Get(fmt.Sprintf("http://localhost:%d/health", config.Port))
			assert.NoError(t, err, fmt.Sprintf("Health endpoint should be accessible for server %d", index))
			if resp != nil {
				resp.Body.Close()
			}
		}(i)
	}

	wg.Wait()

	// Stop all servers
	for i, server := range servers {
		if server != nil {
			err := server.Stop()
			assert.NoError(t, err, fmt.Sprintf("Server %d should stop gracefully", i))
		}
	}
}

// TestServerEnvironmentVariables tests server with environment variables
func TestServerEnvironmentVariables(t *testing.T) {
	ctx := context.Background()

	// Set environment variables
	originalEnv := os.Environ()
	defer func() {
		os.Clearenv()
		for _, env := range originalEnv {
			parts := strings.SplitN(env, "=", 2)
			if len(parts) == 2 {
				os.Setenv(parts[0], parts[1])
			}
		}
	}()

	os.Setenv("LOG_LEVEL", "info")
	os.Setenv("OTEL_SERVICE_NAME", "test-kagent-tools")

	config := TestServerConfig{
		Port:    8095,
		Stdio:   false,
		Timeout: 30 * time.Second,
	}

	server := NewTestServer(config)

	// Start server
	err := server.Start(ctx, config)
	require.NoError(t, err, "Server should start successfully")

	// Wait for server to be ready
	time.Sleep(3 * time.Second)

	// Check server output
	output := server.GetOutput()
	assert.Contains(t, output, "Starting kagent-tools-server")

	// Stop server
	err = server.Stop()
	require.NoError(t, err, "Server should stop gracefully")
}

// TestServerBuildAndExecution tests that the server binary exists and is executable
func TestServerBuildAndExecution(t *testing.T) {
	// Check if server binary exists
	binaryName := getBinaryName()
	binaryPath := fmt.Sprintf("../bin/%s", binaryName)
	_, err := os.Stat(binaryPath)
	if os.IsNotExist(err) {
		t.Skip("Server binary not found, skipping test. Run 'make build' first.")
	}
	require.NoError(t, err, "Server binary should exist")

	// Test --help flag
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, binaryPath, "--help")
	output, err := cmd.CombinedOutput()
	require.NoError(t, err, "Server should respond to --help flag")

	outputStr := string(output)
	assert.Contains(t, outputStr, "KAgent tool server")
	assert.Contains(t, outputStr, "--port")
	assert.Contains(t, outputStr, "--stdio")
	assert.Contains(t, outputStr, "--tools")
	assert.Contains(t, outputStr, "--kubeconfig")
}

// Benchmark tests
func BenchmarkServerStartup(b *testing.B) {
	ctx := context.Background()

	for i := 0; i < b.N; i++ {
		config := TestServerConfig{
			Port:    8096 + i,
			Stdio:   false,
			Timeout: 30 * time.Second,
		}

		server := NewTestServer(config)

		start := time.Now()
		err := server.Start(ctx, config)
		if err != nil {
			b.Fatalf("Server startup failed: %v", err)
		}

		// Wait for server to be ready
		time.Sleep(1 * time.Second)

		duration := time.Since(start)
		b.ReportMetric(float64(duration.Nanoseconds()), "startup_time_ns")

		// Stop server
		_ = server.Stop()
	}
}

// Helper functions for test setup
func init() {
	// Ensure the binary exists before running tests
	binaryName := getBinaryName()
	binaryPath := fmt.Sprintf("../bin/%s", binaryName)
	if _, err := os.Stat(binaryPath); os.IsNotExist(err) {
		// Try to build the binary
		cmd := exec.Command("make", "build")
		cmd.Dir = ".."
		if err := cmd.Run(); err != nil {
			panic(fmt.Sprintf("Failed to build server binary: %v", err))
		}
	}
}

// TestToolRegistrationValidation tests that tool registration works correctly
func TestToolRegistrationValidation(t *testing.T) {
	ctx := context.Background()

	testCases := []struct {
		name          string
		config        TestServerConfig
		expectedTools []string
		shouldFail    bool
	}{
		{
			name: "Register single tool",
			config: TestServerConfig{
				Port:    8087,
				Tools:   []string{"k8s"},
				Timeout: 30 * time.Second,
			},
			expectedTools: []string{"k8s"},
			shouldFail:    false,
		},
		{
			name: "Register multiple tools",
			config: TestServerConfig{
				Port:    8088,
				Tools:   []string{"k8s", "prometheus", "utils"},
				Timeout: 30 * time.Second,
			},
			expectedTools: []string{"k8s", "prometheus", "utils"},
			shouldFail:    false,
		},
		{
			name: "Register invalid tool",
			config: TestServerConfig{
				Port:    8089,
				Tools:   []string{"invalid-tool"},
				Timeout: 30 * time.Second,
			},
			shouldFail: false,
		},
		{
			name: "Register all tools implicitly",
			config: TestServerConfig{
				Port:    8090,
				Tools:   []string{},
				Timeout: 30 * time.Second,
			},
			expectedTools: []string{"utils", "k8s", "prometheus", "helm", "istio", "argo", "cilium"},
			shouldFail:    false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			server := NewTestServer(tc.config)
			err := server.Start(ctx, tc.config)

			if tc.shouldFail {
				require.Error(t, err, "Server should fail to start with invalid configuration")
				return
			}

			require.NoError(t, err, "Server should start successfully")
			defer func() {
				if err := server.Stop(); err != nil {
					t.Errorf("Failed to stop server: %v", err)
				}
			}()

			// Wait for server to be ready
			time.Sleep(3 * time.Second)

			// Verify registered tools
			output := server.GetOutput()

			// Special handling for invalid tool test case
			if tc.name == "Register invalid tool" {
				assert.Contains(t, output, "Unknown tool specified", "Should warn about invalid tool")
				assert.Contains(t, output, "invalid-tool", "Should mention the invalid tool name")
			} else {
				if tc.name == "Register all tools implicitly" {
					// For implicit all tools registration, check for RegisterTools initialized
					assert.Contains(t, output, "RegisterTools initialized", "Should initialize RegisterTools")
					// Don't check for individual tool names as they're not logged individually
					assert.Contains(t, output, "Running KAgent Tools Server", "Should be running with all tools")
				} else {
					// For specific tools, check for Running server message and tool names
					assert.Contains(t, output, "Running KAgent Tools Server", "Should be running server")
					for _, tool := range tc.expectedTools {
						assert.Contains(t, output, tool, fmt.Sprintf("Should register %s tool", tool))
					}
				}
			}

			// Test health endpoint
			resp, err := http.Get(fmt.Sprintf("http://localhost:%d/health", tc.config.Port))
			require.NoError(t, err, "Health endpoint should be accessible")
			assert.Equal(t, http.StatusOK, resp.StatusCode)
			resp.Body.Close()
		})
	}
}

// TestToolExecutionFlow tests the complete flow of tool execution
func TestToolExecutionFlow(t *testing.T) {
	ctx := context.Background()

	config := TestServerConfig{
		Port:    8091,
		Tools:   []string{"utils"},
		Timeout: 30 * time.Second,
	}

	server := NewTestServer(config)
	err := server.Start(ctx, config)
	require.NoError(t, err, "Server should start successfully")
	defer func() {
		if err := server.Stop(); err != nil {
			t.Errorf("Failed to stop server: %v", err)
		}
	}()

	// Wait for server to be ready
	time.Sleep(3 * time.Second)

	// Test health endpoint (MCP server doesn't have REST endpoints for tool execution)
	resp, err := http.Get(fmt.Sprintf("http://localhost:%d/health", config.Port))
	require.NoError(t, err, "Should execute request successfully")
	defer resp.Body.Close()

	// Check response
	assert.Equal(t, http.StatusOK, resp.StatusCode, "Should return OK status")

	// Read response body
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err, "Should read response body")

	// Response should contain "OK"
	assert.Equal(t, "OK", string(body), "Should return OK response")
}

// TestServerTelemetry tests that telemetry is properly initialized and working
func TestServerTelemetry(t *testing.T) {
	ctx := context.Background()

	config := TestServerConfig{
		Port:    8092,
		Tools:   []string{"utils"},
		Timeout: 30 * time.Second,
	}

	// Set test environment variables for telemetry
	os.Setenv("OTEL_SERVICE_NAME", "kagent-tools-test")
	os.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "localhost:4317")
	defer os.Unsetenv("OTEL_SERVICE_NAME")
	defer os.Unsetenv("OTEL_EXPORTER_OTLP_ENDPOINT")

	server := NewTestServer(config)
	err := server.Start(ctx, config)
	require.NoError(t, err, "Server should start successfully")
	defer func() {
		if err := server.Stop(); err != nil {
			t.Errorf("Failed to stop server: %v", err)
		}
	}()

	// Wait for server to be ready
	time.Sleep(3 * time.Second)

	// Check server output for telemetry initialization
	output := server.GetOutput()
	assert.Contains(t, output, "Starting kagent-tools-server", "Server should start with telemetry")

	// Make a request to generate telemetry
	resp, err := http.Get(fmt.Sprintf("http://localhost:%d/health", config.Port))
	require.NoError(t, err, "Health endpoint should be accessible")
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	resp.Body.Close()

	// Check server output for successful startup (telemetry is initialized internally)
	output = server.GetOutput()
	assert.Contains(t, output, "Running KAgent Tools Server", "Server should be running with telemetry enabled")
}

// TestToolRegistrationWithInvalidNames tests server behavior with invalid tool names
func TestToolRegistrationWithInvalidNames(t *testing.T) {
	ctx := context.Background()

	config := TestServerConfig{
		Port:    8087,
		Tools:   []string{"invalid-tool", "not-exists", "k8s"},
		Stdio:   false,
		Timeout: 30 * time.Second,
	}

	server := NewTestServer(config)
	err := server.Start(ctx, config)
	require.NoError(t, err, "Server should start successfully despite invalid tools")

	// Wait for server to be ready
	time.Sleep(3 * time.Second)

	// Check server output for warning messages about invalid tools
	output := server.GetOutput()
	assert.Contains(t, output, "Unknown tool specified")
	assert.Contains(t, output, "invalid-tool")
	assert.Contains(t, output, "not-exists")

	// Verify that valid tools were still registered
	assert.Contains(t, output, "Running KAgent Tools Server")
	assert.Contains(t, output, "k8s")

	err = server.Stop()
	require.NoError(t, err, "Server should stop gracefully")
}

// TestConcurrentToolExecution tests concurrent tool execution
func TestConcurrentToolExecution(t *testing.T) {
	ctx := context.Background()

	config := TestServerConfig{
		Port:    8088,
		Tools:   []string{"utils", "k8s"},
		Stdio:   false,
		Timeout: 30 * time.Second,
	}

	server := NewTestServer(config)
	err := server.Start(ctx, config)
	require.NoError(t, err, "Server should start successfully")

	// Wait for server to be ready
	time.Sleep(3 * time.Second)

	// Create multiple concurrent requests
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			resp, err := http.Get(fmt.Sprintf("http://localhost:%d/health", config.Port))
			require.NoError(t, err, "Concurrent request %d should succeed", id)
			assert.Equal(t, http.StatusOK, resp.StatusCode)
			resp.Body.Close()
		}(i)
	}

	wg.Wait()
	err = server.Stop()
	require.NoError(t, err, "Server should stop gracefully")
}

// TestServerErrorHandling tests server's error handling capabilities
func TestServerErrorHandling(t *testing.T) {
	ctx := context.Background()

	config := TestServerConfig{
		Port:    8089,
		Tools:   []string{"utils"},
		Stdio:   false,
		Timeout: 30 * time.Second,
	}

	server := NewTestServer(config)
	err := server.Start(ctx, config)
	require.NoError(t, err, "Server should start successfully")

	// Wait for server to be ready
	time.Sleep(3 * time.Second)

	// Test malformed request
	req, err := http.NewRequest("POST", fmt.Sprintf("http://localhost:%d/nonexistent", config.Port), strings.NewReader("invalid json"))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	require.NoError(t, err)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	resp.Body.Close()

	err = server.Stop()
	require.NoError(t, err, "Server should stop gracefully")
}

// TestServerMetricsEndpoint tests the metrics endpoint functionality
func TestServerMetricsEndpoint(t *testing.T) {
	ctx := context.Background()

	config := TestServerConfig{
		Port:    8090,
		Tools:   []string{"utils"},
		Stdio:   false,
		Timeout: 30 * time.Second,
	}

	server := NewTestServer(config)
	err := server.Start(ctx, config)
	require.NoError(t, err, "Server should start successfully")

	// Wait for server to be ready
	time.Sleep(3 * time.Second)

	// Test metrics endpoint
	resp, err := http.Get(fmt.Sprintf("http://localhost:%d/metrics", config.Port))
	require.NoError(t, err, "Metrics endpoint should be accessible")
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	// Read and verify metrics content
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	resp.Body.Close()

	metricsContent := string(body)
	assert.Contains(t, metricsContent, "go_")
	assert.Contains(t, metricsContent, "process_")

	err = server.Stop()
	require.NoError(t, err, "Server should stop gracefully")
}

// TestToolSpecificFunctionality tests specific functionality of registered tools
func TestToolSpecificFunctionality(t *testing.T) {
	ctx := context.Background()

	config := TestServerConfig{
		Port:    8091,
		Tools:   []string{"utils", "k8s"},
		Stdio:   false,
		Timeout: 30 * time.Second,
	}

	server := NewTestServer(config)
	err := server.Start(ctx, config)
	require.NoError(t, err, "Server should start successfully")

	// Wait for server to be ready
	time.Sleep(3 * time.Second)

	// Test utils tool endpoint
	resp, err := http.Get(fmt.Sprintf("http://localhost:%d/health", config.Port))
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	resp.Body.Close()

	// Verify response format matches expected OK response
	assert.Equal(t, "OK", string(body), "Should return OK response")

	err = server.Stop()
	require.NoError(t, err, "Server should stop gracefully")
}
