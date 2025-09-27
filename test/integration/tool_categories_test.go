package integration

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ToolCategoryTest represents a test case for a specific tool category
type ToolCategoryTest struct {
	Name        string
	Tools       []string
	Port        int
	ExpectedLog []string
}

// TestToolCategoriesRegistration tests that all tool categories register and initialize correctly
func TestToolCategoriesRegistration(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	testCases := []ToolCategoryTest{
		{
			Name:  "utils_tools",
			Tools: []string{"utils"},
			Port:  8120,
			ExpectedLog: []string{
				"RegisterTools initialized",
				"utils",
				"Running KAgent Tools Server",
			},
		},
		{
			Name:  "k8s_tools",
			Tools: []string{"k8s"},
			Port:  8121,
			ExpectedLog: []string{
				"RegisterTools initialized",
				"k8s",
				"Running KAgent Tools Server",
			},
		},
		{
			Name:  "helm_tools",
			Tools: []string{"helm"},
			Port:  8122,
			ExpectedLog: []string{
				"RegisterTools initialized",
				"helm",
				"Running KAgent Tools Server",
			},
		},
		{
			Name:  "argo_tools",
			Tools: []string{"argo"},
			Port:  8123,
			ExpectedLog: []string{
				"RegisterTools initialized",
				"argo",
				"Running KAgent Tools Server",
			},
		},
		{
			Name:  "cilium_tools",
			Tools: []string{"cilium"},
			Port:  8124,
			ExpectedLog: []string{
				"RegisterTools initialized",
				"cilium",
				"Running KAgent Tools Server",
			},
		},
		{
			Name:  "istio_tools",
			Tools: []string{"istio"},
			Port:  8125,
			ExpectedLog: []string{
				"RegisterTools initialized",
				"istio",
				"Running KAgent Tools Server",
			},
		},
		{
			Name:  "prometheus_tools",
			Tools: []string{"prometheus"},
			Port:  8126,
			ExpectedLog: []string{
				"RegisterTools initialized",
				"prometheus",
				"Running KAgent Tools Server",
			},
		},
		{
			Name:  "multiple_tools",
			Tools: []string{"utils", "k8s", "helm"},
			Port:  8127,
			ExpectedLog: []string{
				"RegisterTools initialized",
				"utils",
				"k8s",
				"helm",
				"Running KAgent Tools Server",
			},
		},
		{
			Name:  "all_tools",
			Tools: []string{}, // Empty means all tools
			Port:  8128,
			ExpectedLog: []string{
				"RegisterTools initialized",
				"Running KAgent Tools Server",
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.Name, func(t *testing.T) {
			config := HTTPTestServerConfig{
				Port:    tc.Port,
				Tools:   tc.Tools,
				Timeout: 30 * time.Second,
			}

			server := NewHTTPTestServer(config)
			err := server.Start(ctx, config)
			require.NoError(t, err, "Server should start successfully for %s", tc.Name)
			defer func() { _ = server.Stop() }()

			// Wait for server to be ready
			time.Sleep(5 * time.Second)

			// Test health endpoint
			resp, err := http.Get(fmt.Sprintf("http://localhost:%d/health", config.Port))
			require.NoError(t, err, "Health endpoint should be accessible for %s", tc.Name)
			assert.Equal(t, http.StatusOK, resp.StatusCode)
			_ = resp.Body.Close()

			// Verify server output contains expected log entries
			output := server.GetOutput()
			for _, expectedLog := range tc.ExpectedLog {
				assert.Contains(t, output, expectedLog, "Output should contain '%s' for %s", expectedLog, tc.Name)
			}

			// Test metrics endpoint
			resp, err = http.Get(fmt.Sprintf("http://localhost:%d/metrics", config.Port))
			require.NoError(t, err, "Metrics endpoint should be accessible for %s", tc.Name)
			assert.Equal(t, http.StatusOK, resp.StatusCode)

			body, err := io.ReadAll(resp.Body)
			require.NoError(t, err)
			_ = resp.Body.Close()

			metricsContent := string(body)
			assert.Contains(t, metricsContent, "go_")
			assert.Contains(t, metricsContent, "process_")
		})
	}
}

// TestToolCategoryCompatibility tests that tool categories maintain compatibility with the new SDK
func TestToolCategoryCompatibility(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// Test each tool category individually to ensure they don't interfere with each other
	toolCategories := []string{"utils", "k8s", "helm", "argo", "cilium", "istio", "prometheus"}

	for i, tool := range toolCategories {
		t.Run(fmt.Sprintf("compatibility_%s", tool), func(t *testing.T) {
			config := HTTPTestServerConfig{
				Port:    8130 + i,
				Tools:   []string{tool},
				Timeout: 30 * time.Second,
			}

			server := NewHTTPTestServer(config)
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

			// Verify tool registration in output
			output := server.GetOutput()
			assert.Contains(t, output, "RegisterTools initialized", "Should initialize RegisterTools for %s", tool)
			assert.Contains(t, output, tool, "Should register %s tool", tool)
			assert.Contains(t, output, "Running KAgent Tools Server", "Should start server for %s", tool)

			// Ensure no error messages in output
			assert.NotContains(t, output, "Failed to register tool provider", "Should not have registration errors for %s", tool)
			assert.NotContains(t, output, "panic", "Should not have panics for %s", tool)
		})
	}
}

// TestToolCategoryErrorHandling tests error handling for each tool category
func TestToolCategoryErrorHandling(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// Test with invalid tool names mixed with valid ones
	testCases := []struct {
		name          string
		tools         []string
		port          int
		expectError   string
		expectSuccess []string
	}{
		{
			name:          "invalid_with_utils",
			tools:         []string{"invalid-tool", "utils"},
			port:          8140,
			expectError:   "Unknown tool specified",
			expectSuccess: []string{"utils"},
		},
		{
			name:          "invalid_with_k8s",
			tools:         []string{"k8s", "nonexistent-tool"},
			port:          8141,
			expectError:   "Unknown tool specified",
			expectSuccess: []string{"k8s"},
		},
		{
			name:          "multiple_invalid",
			tools:         []string{"bad-tool", "another-bad-tool", "helm"},
			port:          8142,
			expectError:   "Unknown tool specified",
			expectSuccess: []string{"helm"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			config := HTTPTestServerConfig{
				Port:    tc.port,
				Tools:   tc.tools,
				Timeout: 30 * time.Second,
			}

			server := NewHTTPTestServer(config)
			err := server.Start(ctx, config)
			require.NoError(t, err, "Server should start even with invalid tools for %s", tc.name)
			defer func() { _ = server.Stop() }()

			// Wait for server to be ready
			time.Sleep(3 * time.Second)

			// Server should still be accessible despite invalid tools
			resp, err := http.Get(fmt.Sprintf("http://localhost:%d/health", config.Port))
			require.NoError(t, err, "Health endpoint should be accessible for %s", tc.name)
			assert.Equal(t, http.StatusOK, resp.StatusCode)
			_ = resp.Body.Close()

			// Check server output
			output := server.GetOutput()

			// Should contain error about invalid tools
			assert.Contains(t, output, tc.expectError, "Should contain error message for %s", tc.name)

			// Should still register valid tools
			assert.Contains(t, output, "RegisterTools initialized", "Should initialize RegisterTools for %s", tc.name)
			for _, validTool := range tc.expectSuccess {
				assert.Contains(t, output, validTool, "Should register valid tool %s for %s", validTool, tc.name)
			}

			// Should still start server
			assert.Contains(t, output, "Running KAgent Tools Server", "Should start server for %s", tc.name)
		})
	}
}

// TestToolCategoryPerformance tests performance characteristics of tool registration
func TestToolCategoryPerformance(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// Test startup time with different numbers of tools
	testCases := []struct {
		name    string
		tools   []string
		port    int
		maxTime time.Duration
	}{
		{
			name:    "single_tool",
			tools:   []string{"utils"},
			port:    8150,
			maxTime: 10 * time.Second,
		},
		{
			name:    "three_tools",
			tools:   []string{"utils", "k8s", "helm"},
			port:    8151,
			maxTime: 15 * time.Second,
		},
		{
			name:    "all_tools",
			tools:   []string{}, // All tools
			port:    8152,
			maxTime: 20 * time.Second,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			config := HTTPTestServerConfig{
				Port:    tc.port,
				Tools:   tc.tools,
				Timeout: tc.maxTime,
			}

			server := NewHTTPTestServer(config)

			// Measure startup time
			start := time.Now()
			err := server.Start(ctx, config)
			startupTime := time.Since(start)

			require.NoError(t, err, "Server should start successfully for %s", tc.name)
			defer func() { _ = server.Stop() }()

			// Verify startup time is reasonable
			assert.Less(t, startupTime, tc.maxTime, "Startup time should be reasonable for %s", tc.name)

			// Wait a bit more for full initialization
			time.Sleep(2 * time.Second)

			// Test that server is responsive
			resp, err := http.Get(fmt.Sprintf("http://localhost:%d/health", config.Port))
			require.NoError(t, err, "Health endpoint should be accessible for %s", tc.name)
			assert.Equal(t, http.StatusOK, resp.StatusCode)
			_ = resp.Body.Close()

			// Verify all expected tools are registered
			output := server.GetOutput()
			assert.Contains(t, output, "RegisterTools initialized", "Should initialize RegisterTools for %s", tc.name)
			assert.Contains(t, output, "Running KAgent Tools Server", "Should start server for %s", tc.name)
		})
	}
}

// TestToolCategoryMemoryUsage tests that tool registration doesn't cause memory leaks
func TestToolCategoryMemoryUsage(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	config := HTTPTestServerConfig{
		Port:    8160,
		Tools:   []string{}, // All tools
		Timeout: 30 * time.Second,
	}

	server := NewHTTPTestServer(config)
	err := server.Start(ctx, config)
	require.NoError(t, err, "Server should start successfully")
	defer func() { _ = server.Stop() }()

	// Wait for server to be ready
	time.Sleep(5 * time.Second)

	// Make multiple requests to check for memory stability
	for i := 0; i < 10; i++ {
		resp, err := http.Get(fmt.Sprintf("http://localhost:%d/health", config.Port))
		require.NoError(t, err, "Health endpoint should be accessible")
		assert.Equal(t, http.StatusOK, resp.StatusCode)
		_ = resp.Body.Close()

		// Also test metrics endpoint
		resp, err = http.Get(fmt.Sprintf("http://localhost:%d/metrics", config.Port))
		require.NoError(t, err, "Metrics endpoint should be accessible")
		assert.Equal(t, http.StatusOK, resp.StatusCode)

		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		_ = resp.Body.Close()

		// Verify metrics contain memory information
		metricsContent := string(body)
		assert.Contains(t, metricsContent, "go_memstats_alloc_bytes")
		assert.Contains(t, metricsContent, "go_goroutines")

		// Brief pause between requests
		time.Sleep(100 * time.Millisecond)
	}

	// Verify server is still responsive after multiple requests
	output := server.GetOutput()
	assert.Contains(t, output, "RegisterTools initialized")
	assert.Contains(t, output, "Running KAgent Tools Server")

	// Should not contain any error messages about memory or goroutine issues
	assert.NotContains(t, output, "out of memory")
	assert.NotContains(t, output, "goroutine leak")
	assert.NotContains(t, output, "panic")
}

// TestToolCategorySDKIntegration tests that all tools work correctly with the new SDK
func TestToolCategorySDKIntegration(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// Test that the new SDK patterns are being used correctly
	config := HTTPTestServerConfig{
		Port:    8170,
		Tools:   []string{"utils", "k8s", "helm"},
		Timeout: 30 * time.Second,
	}

	server := NewHTTPTestServer(config)
	err := server.Start(ctx, config)
	require.NoError(t, err, "Server should start successfully")
	defer func() { _ = server.Stop() }()

	// Wait for server to be ready
	time.Sleep(5 * time.Second)

	// Test basic endpoints
	resp, err := http.Get(fmt.Sprintf("http://localhost:%d/health", config.Port))
	require.NoError(t, err, "Health endpoint should be accessible")
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	_ = resp.Body.Close()

	// Verify server output shows new SDK usage
	output := server.GetOutput()
	assert.Contains(t, output, "RegisterTools initialized")
	assert.Contains(t, output, "Running KAgent Tools Server")

	// Should not contain old SDK patterns or error messages
	assert.NotContains(t, output, "mark3labs/mcp-go", "Should not reference old SDK")
	assert.NotContains(t, output, "Failed to register tool provider", "Should not have registration failures")

	// Should contain evidence of new SDK usage
	assert.Contains(t, output, "utils")
	assert.Contains(t, output, "k8s")
	assert.Contains(t, output, "helm")

	// Test MCP endpoint (should return not implemented until HTTP transport is complete)
	resp, err = http.Get(fmt.Sprintf("http://localhost:%d/mcp", config.Port))
	require.NoError(t, err, "MCP endpoint should be accessible")
	assert.Equal(t, http.StatusNotImplemented, resp.StatusCode)

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	_ = resp.Body.Close()
	assert.Contains(t, string(body), "MCP HTTP transport not yet implemented with new SDK")
}

// TestToolCategoryRobustness tests robustness of tool registration under various conditions
func TestToolCategoryRobustness(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	// Test various edge cases
	testCases := []struct {
		name    string
		tools   []string
		port    int
		timeout time.Duration
	}{
		{
			name:    "empty_tools_list",
			tools:   []string{},
			port:    8180,
			timeout: 30 * time.Second,
		},
		{
			name:    "duplicate_tools",
			tools:   []string{"utils", "utils", "k8s"},
			port:    8181,
			timeout: 30 * time.Second,
		},
		{
			name:    "case_sensitive_tools",
			tools:   []string{"Utils", "K8S", "utils"},
			port:    8182,
			timeout: 30 * time.Second,
		},
		{
			name:    "whitespace_tools",
			tools:   []string{" utils ", "k8s", " helm "},
			port:    8183,
			timeout: 30 * time.Second,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			config := HTTPTestServerConfig{
				Port:    tc.port,
				Tools:   tc.tools,
				Timeout: tc.timeout,
			}

			server := NewHTTPTestServer(config)
			err := server.Start(ctx, config)
			require.NoError(t, err, "Server should start successfully for %s", tc.name)
			defer func() { _ = server.Stop() }()

			// Wait for server to be ready
			time.Sleep(3 * time.Second)

			// Server should be accessible regardless of edge cases
			resp, err := http.Get(fmt.Sprintf("http://localhost:%d/health", config.Port))
			require.NoError(t, err, "Health endpoint should be accessible for %s", tc.name)
			assert.Equal(t, http.StatusOK, resp.StatusCode)
			_ = resp.Body.Close()

			// Verify server started successfully
			output := server.GetOutput()
			assert.Contains(t, output, "Running KAgent Tools Server", "Should start server for %s", tc.name)

			// Should handle edge cases gracefully without panics
			assert.NotContains(t, output, "panic", "Should not panic for %s", tc.name)
		})
	}
}
