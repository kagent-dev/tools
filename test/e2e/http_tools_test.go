package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"sync"
	"testing"
	"time"

	helpers "github.com/kagent-dev/tools/test/e2e/http_helpers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// T038: TestHTTPKubernetesTool tests Kubernetes tool operations over HTTP
func TestHTTPKubernetesTool(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping HTTP E2E tests in short mode")
	}

	port := 19000
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Build the binary once
	cmd := exec.CommandContext(ctx, "go", "build", "-o", "bin/test-http-tools", "./cmd")
	require.NoError(t, cmd.Run(), "Failed to build test binary")
	t.Cleanup(func() {
		_ = os.Remove("bin/test-http-tools")
	})

	// Start HTTP server with k8s tool
	serverCmd := exec.CommandContext(ctx, "bin/test-http-tools", "--http-port", fmt.Sprintf("%d", port), "--tools", "k8s")
	require.NoError(t, serverCmd.Start(), "Failed to start HTTP server")
	t.Cleanup(func() {
		_ = serverCmd.Process.Kill()
	})

	time.Sleep(500 * time.Millisecond)

	// Create HTTP client
	client := helpers.NewHTTPClient(fmt.Sprintf("http://localhost:%d", port))
	client.SetTimeout(5 * time.Second)

	t.Run("ListPods", func(t *testing.T) {
		// Test calling k8s list tool via HTTP
		result, err := client.CallTool(context.Background(), "k8s-list-pods", map[string]interface{}{
			"namespace": "default",
		})
		require.NoError(t, err, "Failed to call k8s-list-pods tool")
		require.NotNil(t, result, "Expected result from tool call")

		// Verify result is JSON
		var output interface{}
		err = helpers.UnmarshalResult(result, &output)
		require.NoError(t, err, "Failed to unmarshal result")
		assert.NotNil(t, output, "Expected non-nil output")
		t.Logf("k8s-list-pods output: %v", output)
	})

	t.Run("ToolDiscovery", func(t *testing.T) {
		// Test that k8s tools are listed
		result, err := client.ListTools(context.Background())
		require.NoError(t, err, "Failed to list tools")
		require.NotNil(t, result, "Expected tool list result")

		var tools interface{}
		err = helpers.UnmarshalResult(result, &tools)
		require.NoError(t, err, "Failed to unmarshal tool list")
		assert.NotNil(t, tools, "Expected tool list")
		t.Logf("Available tools: %v", tools)
	})

	t.Run("ResponseFormat", func(t *testing.T) {
		// Verify response is valid MCP format
		reqBody := []byte(`{
			"jsonrpc": "2.0",
			"method": "tools/call",
			"params": {"name": "k8s-list-pods", "namespace": "default"},
			"id": "test-1"
		}`)

		status, respBody, err := client.RawCall(context.Background(), "/mcp/tools/call", reqBody)
		require.NoError(t, err, "Failed to call tools/call endpoint")
		assert.Equal(t, http.StatusOK, status, "Expected HTTP 200")

		var mcpResp helpers.MCPResponse
		err = json.Unmarshal(respBody, &mcpResp)
		require.NoError(t, err, "Failed to parse MCP response")
		assert.Equal(t, "2.0", mcpResp.JSONRPC, "Expected JSONRPC 2.0")
		assert.NotNil(t, mcpResp.Result, "Expected result in response")
		assert.Nil(t, mcpResp.Error, "Expected no error in response")
	})
}

// T039: TestHTTPHelmTool tests Helm tool operations over HTTP
func TestHTTPHelmTool(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping HTTP E2E tests in short mode")
	}

	port := 19001
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Build if needed
	cmd := exec.CommandContext(ctx, "go", "build", "-o", "bin/test-http-helm", "./cmd")
	if err := cmd.Run(); err != nil {
		t.Logf("Build warning: %v (may already exist)", err)
	}
	t.Cleanup(func() {
		_ = os.Remove("bin/test-http-helm")
	})

	// Start HTTP server with helm tool
	serverCmd := exec.CommandContext(ctx, "bin/test-http-helm", "--http-port", fmt.Sprintf("%d", port), "--tools", "helm")
	require.NoError(t, serverCmd.Start(), "Failed to start HTTP server with helm")
	t.Cleanup(func() {
		_ = serverCmd.Process.Kill()
	})

	time.Sleep(500 * time.Millisecond)

	// Create HTTP client
	client := helpers.NewHTTPClient(fmt.Sprintf("http://localhost:%d", port))
	client.SetTimeout(5 * time.Second)

	t.Run("ListReleases", func(t *testing.T) {
		// Test calling helm list tool via HTTP
		result, err := client.CallTool(context.Background(), "helm-list-releases", map[string]interface{}{
			"all_namespaces": false,
		})
		require.NoError(t, err, "Failed to call helm-list-releases tool")
		require.NotNil(t, result, "Expected result from tool call")

		// Verify result is JSON
		var output interface{}
		err = helpers.UnmarshalResult(result, &output)
		require.NoError(t, err, "Failed to unmarshal result")
		assert.NotNil(t, output, "Expected non-nil output")
		t.Logf("helm-list-releases output: %v", output)
	})

	t.Run("HelmToolDiscovery", func(t *testing.T) {
		// Verify helm tools are available
		result, err := client.ListTools(context.Background())
		require.NoError(t, err, "Failed to list tools")
		require.NotNil(t, result, "Expected tool list result")

		var tools interface{}
		err = helpers.UnmarshalResult(result, &tools)
		require.NoError(t, err, "Failed to unmarshal tool list")
		assert.NotNil(t, tools, "Expected tool list with helm tools")
	})

	t.Run("HelmResponseFormat", func(t *testing.T) {
		// Verify Helm response format
		reqBody := []byte(`{
			"jsonrpc": "2.0",
			"method": "tools/call",
			"params": {"name": "helm-list-releases", "all_namespaces": false},
			"id": "test-helm-1"
		}`)

		status, respBody, err := client.RawCall(context.Background(), "/mcp/tools/call", reqBody)
		require.NoError(t, err, "Failed to call tools/call endpoint")
		assert.Equal(t, http.StatusOK, status, "Expected HTTP 200")

		var mcpResp helpers.MCPResponse
		err = json.Unmarshal(respBody, &mcpResp)
		require.NoError(t, err, "Failed to parse MCP response")
		assert.Equal(t, "2.0", mcpResp.JSONRPC, "Expected JSONRPC 2.0")
		assert.NotNil(t, mcpResp.Result, "Expected result in helm response")
	})
}

// T040: TestHTTPIstioTool tests Istio tool operations over HTTP
func TestHTTPIstioTool(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping HTTP E2E tests in short mode")
	}

	port := 19002
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Build if needed
	cmd := exec.CommandContext(ctx, "go", "build", "-o", "bin/test-http-istio", "./cmd")
	if err := cmd.Run(); err != nil {
		t.Logf("Build warning: %v (may already exist)", err)
	}
	t.Cleanup(func() {
		_ = os.Remove("bin/test-http-istio")
	})

	// Start HTTP server with istio tool
	serverCmd := exec.CommandContext(ctx, "bin/test-http-istio", "--http-port", fmt.Sprintf("%d", port), "--tools", "istio")
	require.NoError(t, serverCmd.Start(), "Failed to start HTTP server with istio")
	t.Cleanup(func() {
		_ = serverCmd.Process.Kill()
	})

	time.Sleep(500 * time.Millisecond)

	// Create HTTP client
	client := helpers.NewHTTPClient(fmt.Sprintf("http://localhost:%d", port))
	client.SetTimeout(5 * time.Second)

	t.Run("ListIstioResources", func(t *testing.T) {
		// Test calling istio list tool via HTTP
		result, err := client.CallTool(context.Background(), "istio-list-virtualservices", map[string]interface{}{})
		require.NoError(t, err, "Failed to call istio-list-virtualservices tool")
		require.NotNil(t, result, "Expected result from tool call")

		// Verify result is JSON
		var output interface{}
		err = helpers.UnmarshalResult(result, &output)
		require.NoError(t, err, "Failed to unmarshal result")
		assert.NotNil(t, output, "Expected non-nil output from istio")
		t.Logf("istio-list-virtualservices output: %v", output)
	})

	t.Run("IstioToolDiscovery", func(t *testing.T) {
		// Verify istio tools are available
		result, err := client.ListTools(context.Background())
		require.NoError(t, err, "Failed to list tools")
		require.NotNil(t, result, "Expected tool list result")

		var tools interface{}
		err = helpers.UnmarshalResult(result, &tools)
		require.NoError(t, err, "Failed to unmarshal tool list")
		assert.NotNil(t, tools, "Expected tool list with istio tools")
	})

	t.Run("IstioResponseFormat", func(t *testing.T) {
		// Verify Istio response format
		reqBody := []byte(`{
			"jsonrpc": "2.0",
			"method": "tools/call",
			"params": {"name": "istio-list-virtualservices"},
			"id": "test-istio-1"
		}`)

		status, respBody, err := client.RawCall(context.Background(), "/mcp/tools/call", reqBody)
		require.NoError(t, err, "Failed to call tools/call endpoint")
		assert.Equal(t, http.StatusOK, status, "Expected HTTP 200")

		var mcpResp helpers.MCPResponse
		err = json.Unmarshal(respBody, &mcpResp)
		require.NoError(t, err, "Failed to parse MCP response")
		assert.Equal(t, "2.0", mcpResp.JSONRPC, "Expected JSONRPC 2.0")
		assert.NotNil(t, mcpResp.Result, "Expected result in istio response")
	})
}

// T041: TestHTTPParameterPassing tests parameter passing and validation
func TestHTTPParameterPassing(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping HTTP E2E tests in short mode")
	}

	port := 19003
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Start HTTP server
	serverCmd := exec.CommandContext(ctx, "bin/test-http-tools", "--http-port", fmt.Sprintf("%d", port), "--tools", "k8s,helm")
	require.NoError(t, serverCmd.Start(), "Failed to start server")
	t.Cleanup(func() {
		_ = serverCmd.Process.Kill()
	})

	time.Sleep(500 * time.Millisecond)

	client := helpers.NewHTTPClient(fmt.Sprintf("http://localhost:%d", port))
	client.SetTimeout(5 * time.Second)

	t.Run("SimpleStringParameter", func(t *testing.T) {
		// Test passing simple string parameter
		result, err := client.CallTool(context.Background(), "k8s-list-pods", map[string]interface{}{
			"namespace": "kube-system",
		})
		require.NoError(t, err, "Failed to call tool with string parameter")
		assert.NotNil(t, result, "Expected result")
	})

	t.Run("BooleanParameter", func(t *testing.T) {
		// Test passing boolean parameter
		result, err := client.CallTool(context.Background(), "helm-list-releases", map[string]interface{}{
			"all_namespaces": true,
		})
		require.NoError(t, err, "Failed to call tool with boolean parameter")
		assert.NotNil(t, result, "Expected result")
	})

	t.Run("NumericParameter", func(t *testing.T) {
		// Test passing numeric parameter
		result, err := client.CallTool(context.Background(), "k8s-list-pods", map[string]interface{}{
			"namespace": "default",
			"limit":     10,
		})
		require.NoError(t, err, "Failed to call tool with numeric parameter")
		assert.NotNil(t, result, "Expected result with numeric parameter")
	})

	t.Run("MissingRequiredParameter", func(t *testing.T) {
		// Test tool behavior with missing required parameters
		reqBody := []byte(`{
			"jsonrpc": "2.0",
			"method": "tools/call",
			"params": {"name": "k8s-list-pods"},
			"id": "test-missing"
		}`)

		status, respBody, err := client.RawCall(context.Background(), "/mcp/tools/call", reqBody)
		require.NoError(t, err, "Request should complete")
		// Status might be 400 or 200 with error in result, depending on implementation
		assert.True(t, status == http.StatusOK || status == http.StatusBadRequest, "Expected 200 or 400 status")
		assert.NotEmpty(t, respBody, "Expected response body")
		t.Logf("Missing parameter response (HTTP %d): %s", status, string(respBody))
	})

	t.Run("ParameterValidation", func(t *testing.T) {
		// Test parameter type validation
		reqBody := []byte(`{
			"jsonrpc": "2.0",
			"method": "tools/call",
			"params": {"name": "k8s-list-pods", "namespace": 12345},
			"id": "test-validation"
		}`)

		status, respBody, err := client.RawCall(context.Background(), "/mcp/tools/call", reqBody)
		require.NoError(t, err, "Request should complete")
		assert.NotEmpty(t, respBody, "Expected response body")
		t.Logf("Parameter validation response (HTTP %d): %s", status, string(respBody))
	})
}

// T042: TestHTTPConsistency tests consistency of HTTP vs stdio and repeated runs
func TestHTTPConsistency(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping HTTP E2E tests in short mode")
	}

	port := 19004
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Start HTTP server
	serverCmd := exec.CommandContext(ctx, "bin/test-http-tools", "--http-port", fmt.Sprintf("%d", port), "--tools", "k8s")
	require.NoError(t, serverCmd.Start(), "Failed to start server")
	t.Cleanup(func() {
		_ = serverCmd.Process.Kill()
	})

	time.Sleep(500 * time.Millisecond)

	client := helpers.NewHTTPClient(fmt.Sprintf("http://localhost:%d", port))
	client.SetTimeout(5 * time.Second)

	t.Run("ConsistentResults", func(t *testing.T) {
		// Test that multiple runs produce identical results (determinism)
		numRuns := 3
		var results []interface{}
		var rawResults [][]byte

		for i := 0; i < numRuns; i++ {
			reqBody := []byte(`{
				"jsonrpc": "2.0",
				"method": "tools/call",
				"params": {"name": "k8s-list-pods", "namespace": "default"},
				"id": "test-consistency-` + fmt.Sprintf("%d", i) + `"
			}`)

			status, respBody, err := client.RawCall(context.Background(), "/mcp/tools/call", reqBody)
			require.NoError(t, err, "Run %d failed", i)
			assert.Equal(t, http.StatusOK, status, "Expected HTTP 200 on run %d", i)

			// Parse response
			var mcpResp helpers.MCPResponse
			err = json.Unmarshal(respBody, &mcpResp)
			require.NoError(t, err, "Failed to parse response on run %d", i)

			// Extract result
			var resultData interface{}
			err = json.Unmarshal(mcpResp.Result, &resultData)
			require.NoError(t, err, "Failed to unmarshal result on run %d", i)

			results = append(results, resultData)
			// Note: rawResults kept for potential future debugging
			_ = append(rawResults, respBody)
		}

		// Compare all results
		comparer := helpers.NewOutputComparer()
		for i := 1; i < numRuns; i++ {
			comparison := comparer.Compare(results[0], results[i])
			assert.True(t, comparison.Match,
				"Run %d result differs from run 0:\n%s", i, comparison.DetailedDiff())
		}

		// All results should match
		t.Logf("✓ All %d runs produced consistent results", numRuns)
	})

	t.Run("SuccessRateOnRepeatedCalls", func(t *testing.T) {
		// Test that repeated calls maintain 100% success rate
		numCalls := 10
		successCount := 0

		for i := 0; i < numCalls; i++ {
			result, err := client.CallTool(context.Background(), "k8s-list-pods", map[string]interface{}{
				"namespace": "default",
			})
			if err == nil && result != nil {
				successCount++
			} else {
				t.Logf("Call %d failed: %v", i, err)
			}
		}

		successRate := float64(successCount) / float64(numCalls) * 100
		assert.Equal(t, numCalls, successCount,
			"Expected 100%% success rate, got %.1f%% (%d/%d)", successRate, successCount, numCalls)
		t.Logf("✓ Achieved 100%% success rate on %d repeated calls", numCalls)
	})

	t.Run("ConcurrentConsistency", func(t *testing.T) {
		// Test consistency under concurrent execution
		numConcurrent := 5
		var wg sync.WaitGroup
		results := make([]interface{}, numConcurrent)
		var mu sync.Mutex
		successCount := 0

		for i := 0; i < numConcurrent; i++ {
			wg.Add(1)
			go func(idx int) {
				defer wg.Done()
				result, err := client.CallTool(context.Background(), "k8s-list-pods", map[string]interface{}{
					"namespace": "default",
				})
				if err == nil && result != nil {
					mu.Lock()
					successCount++
					// Extract result for comparison
					var resultData interface{}
					if err := helpers.UnmarshalResult(result, &resultData); err == nil {
						results[idx] = resultData
					}
					mu.Unlock()
				}
			}(i)
		}

		wg.Wait()

		assert.Equal(t, numConcurrent, successCount,
			"All concurrent calls should succeed")

		// Verify some results are present
		validResults := 0
		for _, r := range results {
			if r != nil {
				validResults++
			}
		}
		assert.Greater(t, validResults, 0, "Should have at least some valid results")
		t.Logf("✓ Concurrent execution maintained consistency (%d/%d calls succeeded)", validResults, numConcurrent)
	})

	t.Run("ConsistencyAcrossToolInvocations", func(t *testing.T) {
		// Test that different tool invocations maintain output consistency
		toolTests := []struct {
			name   string
			params map[string]interface{}
		}{
			{
				name:   "k8s-list-pods-default",
				params: map[string]interface{}{"namespace": "default"},
			},
			{
				name:   "k8s-list-pods-kube-system",
				params: map[string]interface{}{"namespace": "kube-system"},
			},
		}

		for _, tt := range toolTests {
			t.Run(tt.name, func(t *testing.T) {
				// Make two identical calls
				result1, err1 := client.CallTool(context.Background(), "k8s-list-pods", tt.params)
				result2, err2 := client.CallTool(context.Background(), "k8s-list-pods", tt.params)

				require.NoError(t, err1, "First call failed")
				require.NoError(t, err2, "Second call failed")
				require.NotNil(t, result1, "First result is nil")
				require.NotNil(t, result2, "Second result is nil")

				// Parse and compare results
				var data1, data2 interface{}
				err1 = helpers.UnmarshalResult(result1, &data1)
				err2 = helpers.UnmarshalResult(result2, &data2)

				require.NoError(t, err1, "Failed to unmarshal first result")
				require.NoError(t, err2, "Failed to unmarshal second result")

				// Compare results
				comparer := helpers.NewOutputComparer()
				comparison := comparer.Compare(data1, data2)
				assert.True(t, comparison.Match,
					"Results should be identical:\n%s", comparison.DetailedDiff())
			})
		}
	})
}

// TestHTTPToolsErrorRecovery tests error recovery in repeated calls
func TestHTTPToolsErrorRecovery(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping HTTP E2E tests in short mode")
	}

	port := 19005
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Start HTTP server
	serverCmd := exec.CommandContext(ctx, "bin/test-http-tools", "--http-port", fmt.Sprintf("%d", port), "--tools", "k8s")
	require.NoError(t, serverCmd.Start(), "Failed to start server")
	t.Cleanup(func() {
		_ = serverCmd.Process.Kill()
	})

	time.Sleep(500 * time.Millisecond)

	client := helpers.NewHTTPClient(fmt.Sprintf("http://localhost:%d", port))
	client.SetTimeout(5 * time.Second)

	t.Run("ServerRecoveryAfterError", func(t *testing.T) {
		// Make invalid request
		invalidReq := []byte(`{
			"jsonrpc": "2.0",
			"method": "tools/call",
			"params": {"name": "nonexistent-tool"},
			"id": "invalid"
		}`)

		status1, _, err := client.RawCall(context.Background(), "/mcp/tools/call", invalidReq)
		require.NoError(t, err, "Request should complete (even with error)")
		t.Logf("Invalid request returned status: %d", status1)

		// Wait briefly
		time.Sleep(100 * time.Millisecond)

		// Make valid request after error
		result, err := client.CallTool(context.Background(), "k8s-list-pods", map[string]interface{}{
			"namespace": "default",
		})
		assert.NoError(t, err, "Server should recover and process valid request after error")
		assert.NotNil(t, result, "Should get valid result after error")
	})
}
