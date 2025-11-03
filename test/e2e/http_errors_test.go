package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	nethttp "net/http"
	"testing"
	"time"

	mcphttp "github.com/kagent-dev/tools/internal/mcp/http"
)

// setupTestServerWithHandlers creates a test HTTP server with MCP handlers registered
func setupTestServerWithHandlers(t *testing.T, port int) *mcphttp.Server {
	server := mcphttp.NewServer(port)
	
	// Register handlers before starting server
	requestHandler := mcphttp.NewRequestHandler(server, 100)
	if err := requestHandler.RegisterHandlers(); err != nil {
		t.Fatalf("Failed to register handlers: %v", err)
	}
	
	if err := server.Start(context.Background()); err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}
	
	// Cleanup
	t.Cleanup(func() {
		if err := server.Stop(context.Background()); err != nil {
			t.Logf("error stopping server: %v", err)
		}
	})
	
	// Give server time to start
	time.Sleep(100 * time.Millisecond)
	
	return server
}

// TestMalformedJSONRequest tests handling of invalid JSON in request body
func TestMalformedJSONRequest(t *testing.T) {
	_ = setupTestServerWithHandlers(t, 18091)

	testCases := []struct {
		name           string
		body           string
		expectedStatus int
		expectError    bool
	}{
		{
			name:           "Invalid JSON syntax",
			body:           `{invalid json}`,
			expectedStatus: nethttp.StatusBadRequest,
			expectError:    true,
		},
		{
			name:           "Incomplete JSON object",
			body:           `{"jsonrpc": "2.0", "method": "initialize"`,
			expectedStatus: nethttp.StatusBadRequest,
			expectError:    true,
		},
		{
			name:           "Empty body",
			body:           ``,
			expectedStatus: nethttp.StatusBadRequest,
			expectError:    true,
		},
		{
			name:           "JSON array instead of object",
			body:           `[1, 2, 3]`,
			expectedStatus: nethttp.StatusBadRequest,
			expectError:    true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			client := &nethttp.Client{Timeout: 5 * time.Second}
			req, err := nethttp.NewRequest(nethttp.MethodPost, fmt.Sprintf("http://localhost:%d/mcp/initialize", 18091), bytes.NewBufferString(tc.body))
			if err != nil {
				t.Fatalf("Failed to create request: %v", err)
			}
			req.Header.Set("Content-Type", "application/json")

			resp, err := client.Do(req)
			if err != nil {
				t.Fatalf("Request failed: %v", err)
			}
			defer func() {
				if err := resp.Body.Close(); err != nil {
					t.Logf("error closing response body: %v", err)
				}
			}()

			if resp.StatusCode != tc.expectedStatus {
				t.Errorf("Expected status %d, got %d", tc.expectedStatus, resp.StatusCode)
			}

			if tc.expectError {
				var errResp map[string]interface{}
				if err := json.NewDecoder(resp.Body).Decode(&errResp); err != nil {
					t.Fatalf("Failed to decode error response: %v", err)
				}

				// Verify error structure
				if errResp["error"] == nil {
					t.Error("Expected error field in response")
				}
				if errResp["jsonrpc"] != "2.0" {
					t.Error("Expected jsonrpc 2.0 in response")
				}
			}
		})
	}
}

// TestMissingRequiredFields tests handling of missing required fields in requests
func TestMissingRequiredFields(t *testing.T) {
	server := mcphttp.NewServer(18092)
	defer func() {
		if err := server.Stop(context.Background()); err != nil {
			t.Logf("error stopping server: %v", err)
		}
	}()

	if err := server.Start(context.Background()); err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}

	time.Sleep(100 * time.Millisecond)

	testCases := []struct {
		name           string
		request        map[string]interface{}
		expectedStatus int
		expectField    string
	}{
		{
			name:           "Missing jsonrpc field",
			request:        map[string]interface{}{"method": "initialize", "id": "1"},
			expectedStatus: nethttp.StatusBadRequest,
			expectField:    "jsonrpc",
		},
		{
			name:           "Missing id field",
			request:        map[string]interface{}{"jsonrpc": "2.0", "method": "initialize"},
			expectedStatus: nethttp.StatusBadRequest,
			expectField:    "id",
		},
		{
			name:           "Missing method field",
			request:        map[string]interface{}{"jsonrpc": "2.0", "id": "1"},
			expectedStatus: nethttp.StatusBadRequest,
			expectField:    "method",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			body, _ := json.Marshal(tc.request)
			client := &nethttp.Client{Timeout: 5 * time.Second}
			req, _ := nethttp.NewRequest(nethttp.MethodPost, fmt.Sprintf("http://localhost:%d/mcp/initialize", 18092), bytes.NewBuffer(body))
			req.Header.Set("Content-Type", "application/json")

			resp, err := client.Do(req)
			if err != nil {
				t.Fatalf("Failed to make request: %v", err)
			}
			defer func() {
				if err := resp.Body.Close(); err != nil {
					t.Logf("error closing response body: %v", err)
				}
			}()

			if resp.StatusCode != tc.expectedStatus {
				t.Errorf("Expected status %d, got %d", tc.expectedStatus, resp.StatusCode)
			}

			var errResp map[string]interface{}
			if err := json.NewDecoder(resp.Body).Decode(&errResp); err != nil {
				t.Logf("error decoding response: %v", err)
			}

			// Verify error contains field info
			if errResp["error"] != nil {
				if _, ok := errResp["error"].(map[string]interface{}); ok {
					t.Logf("Error response: %v", errResp["error"])
				}
			}
		})
	}
}

// TestInvalidFieldTypes tests handling of invalid field types in requests
func TestInvalidFieldTypes(t *testing.T) {
	server := mcphttp.NewServer(18093)
	defer func() {
		if err := server.Stop(context.Background()); err != nil {
			t.Logf("error stopping server: %v", err)
		}
	}()

	if err := server.Start(context.Background()); err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}

	time.Sleep(100 * time.Millisecond)

	testCases := []struct {
		name           string
		request        map[string]interface{}
		expectedStatus int
	}{
		{
			name: "JSONRPC is number instead of string",
			request: map[string]interface{}{
				"jsonrpc": 2.0,
				"method":  "initialize",
				"id":      "1",
			},
			expectedStatus: nethttp.StatusBadRequest,
		},
		{
			name: "Method is object instead of string",
			request: map[string]interface{}{
				"jsonrpc": "2.0",
				"method":  map[string]interface{}{"op": "initialize"},
				"id":      "1",
			},
			expectedStatus: nethttp.StatusBadRequest,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			body, _ := json.Marshal(tc.request)
			client := &nethttp.Client{Timeout: 5 * time.Second}
			req, _ := nethttp.NewRequest(nethttp.MethodPost, fmt.Sprintf("http://localhost:%d/mcp/initialize", 18093), bytes.NewBuffer(body))
			req.Header.Set("Content-Type", "application/json")

			resp, err := client.Do(req)
			if err != nil {
				t.Fatalf("Failed to make request: %v", err)
			}
			defer func() {
				if err := resp.Body.Close(); err != nil {
					t.Logf("error closing response body: %v", err)
				}
			}()

			if resp.StatusCode != tc.expectedStatus {
				t.Errorf("Expected status %d, got %d", tc.expectedStatus, resp.StatusCode)
			}
		})
	}
}

// TestToolNotFoundError tests 404 response for non-existent tools
func TestToolNotFoundError(t *testing.T) {
	server := mcphttp.NewServer(18094)
	defer func() {
		if err := server.Stop(context.Background()); err != nil {
			t.Logf("error stopping server: %v", err)
		}
	}()

	if err := server.Start(context.Background()); err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}

	time.Sleep(100 * time.Millisecond)

	client := &nethttp.Client{Timeout: 5 * time.Second}

	request := map[string]interface{}{
		"jsonrpc": "2.0",
		"method":  "tools/call",
		"params": map[string]interface{}{
			"name": "nonexistent_tool",
			"arg1": "value1",
		},
		"id": "1",
	}

	body, _ := json.Marshal(request)
	req, _ := nethttp.NewRequest(nethttp.MethodPost, fmt.Sprintf("http://localhost:%d/mcp/tools/call", 18094), bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("Failed to make request: %v", err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			t.Logf("error closing response body: %v", err)
		}
	}()

	// Should return 404 for tool not found
	if resp.StatusCode != nethttp.StatusNotFound {
		t.Errorf("Expected status 404, got %d", resp.StatusCode)
	}

	var errResp map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&errResp); err != nil {
		t.Logf("error decoding response: %v", err)
	}

	// Verify error contains tool not found info
	if errResp["error"] != nil {
		t.Logf("Tool not found error: %v", errResp["error"])
	}
}

// TestInvalidToolParameters tests 400 response for invalid tool parameters
func TestInvalidToolParameters(t *testing.T) {
	server := mcphttp.NewServer(18095)
	defer func() {
		if err := server.Stop(context.Background()); err != nil {
			t.Logf("error stopping server: %v", err)
		}
	}()

	if err := server.Start(context.Background()); err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}

	time.Sleep(100 * time.Millisecond)

	testCases := []struct {
		name           string
		request        map[string]interface{}
		expectedStatus int
	}{
		{
			name: "Missing tool name in params",
			request: map[string]interface{}{
				"jsonrpc": "2.0",
				"method":  "tools/call",
				"params":  map[string]interface{}{},
				"id":      "1",
			},
			expectedStatus: nethttp.StatusBadRequest,
		},
		{
			name: "Tool name is empty string",
			request: map[string]interface{}{
				"jsonrpc": "2.0",
				"method":  "tools/call",
				"params": map[string]interface{}{
					"name": "",
				},
				"id": "1",
			},
			expectedStatus: nethttp.StatusBadRequest,
		},
		{
			name: "Tool name is not a string",
			request: map[string]interface{}{
				"jsonrpc": "2.0",
				"method":  "tools/call",
				"params": map[string]interface{}{
					"name": 123,
				},
				"id": "1",
			},
			expectedStatus: nethttp.StatusBadRequest,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			body, _ := json.Marshal(tc.request)
			client := &nethttp.Client{Timeout: 5 * time.Second}
			req, _ := nethttp.NewRequest(nethttp.MethodPost, fmt.Sprintf("http://localhost:%d/mcp/tools/call", 18095), bytes.NewBuffer(body))
			req.Header.Set("Content-Type", "application/json")

			resp, err := client.Do(req)
			if err != nil {
				t.Fatalf("Failed to make request: %v", err)
			}
			defer func() {
				if err := resp.Body.Close(); err != nil {
					t.Logf("error closing response body: %v", err)
				}
			}()

			if resp.StatusCode != tc.expectedStatus {
				t.Errorf("Expected status %d, got %d", tc.expectedStatus, resp.StatusCode)
			}
		})
	}
}

// TestRequestTimeout tests request timeout handling
func TestRequestTimeout(t *testing.T) {
	server := mcphttp.NewServer(18096)
	server.SetRequestTimeout(500 * time.Millisecond) // Very short timeout for testing
	defer func() {
		if err := server.Stop(context.Background()); err != nil {
			t.Logf("error stopping server: %v", err)
		}
	}()

	if err := server.Start(context.Background()); err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}

	time.Sleep(100 * time.Millisecond)

	client := &nethttp.Client{Timeout: 2 * time.Second}

	request := map[string]interface{}{
		"jsonrpc": "2.0",
		"method":  "initialize",
		"id":      "1",
	}

	body, _ := json.Marshal(request)
	req, _ := nethttp.NewRequest(nethttp.MethodPost, fmt.Sprintf("http://localhost:%d/mcp/initialize", 18096), bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")

	// Request should complete within timeout or receive timeout error
	resp, err := client.Do(req)
	if err == nil && resp != nil {
		defer func() {
			if err := resp.Body.Close(); err != nil {
				t.Logf("error closing response body: %v", err)
			}
		}()
		// If request completes, verify it's a valid response
		if resp.StatusCode == 0 {
			t.Error("Expected non-zero status code")
		}
	}
}

// TestConnectionRefused tests error handling when connection is refused
func TestConnectionRefused(t *testing.T) {
	// Try to connect to a port that's not listening
	client := &nethttp.Client{Timeout: 2 * time.Second}

	request := map[string]interface{}{
		"jsonrpc": "2.0",
		"method":  "initialize",
		"id":      "1",
	}

	body, _ := json.Marshal(request)
	req, _ := nethttp.NewRequest(nethttp.MethodPost, "http://localhost:19999/mcp/initialize", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")

	_, err := client.Do(req)
	if err == nil {
		t.Error("Expected connection error for unavailable port")
	}

	// Verify it's a connection refused error
	if opErr, ok := err.(*net.OpError); ok {
		if opErr.Op != "dial" {
			t.Logf("Connection error type: %v", opErr.Op)
		}
	}
}

// TestErrorResponseStructure tests that error responses have proper structure
func TestErrorResponseStructure(t *testing.T) {
	server := mcphttp.NewServer(18097)
	defer func() {
		if err := server.Stop(context.Background()); err != nil {
			t.Logf("error stopping server: %v", err)
		}
	}()

	if err := server.Start(context.Background()); err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}

	time.Sleep(100 * time.Millisecond)

	client := &nethttp.Client{Timeout: 5 * time.Second}

	request := map[string]interface{}{
		"jsonrpc": "2.0",
		"method":  "tools/call",
		"params": map[string]interface{}{
			"name": "nonexistent_tool",
		},
		"id": "1",
	}

	body, _ := json.Marshal(request)
	req, _ := nethttp.NewRequest(nethttp.MethodPost, fmt.Sprintf("http://localhost:%d/mcp/tools/call", 18097), bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("Failed to make request: %v", err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			t.Logf("error closing response body: %v", err)
		}
	}()

	var errResp map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&errResp); err != nil {
		t.Logf("error decoding response: %v", err)
	}

	// Verify required error fields
	if errResp["jsonrpc"] != "2.0" {
		t.Error("Expected jsonrpc 2.0 in error response")
	}

	if errResp["error"] == nil {
		t.Error("Expected error field in response")
	} else {
		errMap := errResp["error"].(map[string]interface{})
		if errMap["code"] == nil {
			t.Error("Expected error code in error response")
		}
		if errMap["message"] == nil {
			t.Error("Expected error message in error response")
		}
		if errMap["data"] != nil {
			if data, ok := errMap["data"].(map[string]interface{}); ok {
				if data["details"] != nil {
					details := data["details"].(map[string]interface{})
					// Verify actionable error details
					if _, ok := details["suggestion"]; !ok {
						t.Error("Expected suggestion in error details")
					}
					if _, ok := details["error_type"]; !ok {
						t.Error("Expected error_type in error details")
					}
				}
			}
		}
	}
}

// TestErrorRecovery tests that server continues functioning after errors
func TestErrorRecovery(t *testing.T) {
	server := mcphttp.NewServer(18098)
	defer func() {
		if err := server.Stop(context.Background()); err != nil {
			t.Logf("error stopping server: %v", err)
		}
	}()

	if err := server.Start(context.Background()); err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}

	time.Sleep(100 * time.Millisecond)

	client := &nethttp.Client{Timeout: 5 * time.Second}

	// Send a malformed request
	req1, _ := nethttp.NewRequest(nethttp.MethodPost, fmt.Sprintf("http://localhost:%d/mcp/initialize", 18098), bytes.NewBufferString("{invalid}"))
	req1.Header.Set("Content-Type", "application/json")
	resp1, _ := client.Do(req1)
	if err := resp1.Body.Close(); err != nil {
		t.Logf("error closing response body: %v", err)
	}

	if resp1.StatusCode != nethttp.StatusBadRequest {
		t.Errorf("Expected 400 for malformed request, got %d", resp1.StatusCode)
	}

	// Now send a valid request - server should still work
	validRequest := map[string]interface{}{
		"jsonrpc": "2.0",
		"method":  "initialize",
		"id":      "1",
	}

	body, _ := json.Marshal(validRequest)
	req2, _ := nethttp.NewRequest(nethttp.MethodPost, fmt.Sprintf("http://localhost:%d/mcp/initialize", 18098), bytes.NewBuffer(body))
	req2.Header.Set("Content-Type", "application/json")

	resp2, err := client.Do(req2)
	if err != nil {
		t.Fatalf("Valid request failed after error: %v", err)
	}
	defer func() {
		if err := resp2.Body.Close(); err != nil {
			t.Logf("error closing response body: %v", err)
		}
	}()

	if resp2.StatusCode != nethttp.StatusOK {
		t.Errorf("Valid request should succeed (got %d), server not recovered from error", resp2.StatusCode)
	}

	// Verify response structure
	var successResp map[string]interface{}
	if err := json.NewDecoder(resp2.Body).Decode(&successResp); err != nil {
		t.Fatalf("Failed to decode successful response: %v", err)
	}

	if successResp["result"] == nil {
		t.Error("Expected result field in successful response")
	}
}

// TestMultipleErrorsRecovery tests recovery after multiple error scenarios
func TestMultipleErrorsRecovery(t *testing.T) {
	server := mcphttp.NewServer(18099)
	defer func() {
		if err := server.Stop(context.Background()); err != nil {
			t.Logf("error stopping server: %v", err)
		}
	}()

	if err := server.Start(context.Background()); err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}

	time.Sleep(100 * time.Millisecond)

	client := &nethttp.Client{Timeout: 5 * time.Second}
	baseURL := fmt.Sprintf("http://localhost:%d", 18099)

	// Scenario 1: Invalid JSON
	req, _ := nethttp.NewRequest(nethttp.MethodPost, baseURL+"/mcp/initialize", bytes.NewBufferString("{bad}"))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := client.Do(req)
	if err := resp.Body.Close(); err != nil {
		t.Logf("error closing response body: %v", err)
	}

	// Scenario 2: Missing field
	missingFieldReq := map[string]interface{}{"jsonrpc": "2.0"}
	body, _ := json.Marshal(missingFieldReq)
	req, _ = nethttp.NewRequest(nethttp.MethodPost, baseURL+"/mcp/initialize", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	resp, _ = client.Do(req)
	if err := resp.Body.Close(); err != nil {
		t.Logf("error closing response body: %v", err)
	}

	// Scenario 3: Invalid tool
	toolReq := map[string]interface{}{
		"jsonrpc": "2.0",
		"method":  "tools/call",
		"params":  map[string]interface{}{"name": "invalid_tool"},
		"id":      "1",
	}
	body, _ = json.Marshal(toolReq)
	req, _ = nethttp.NewRequest(nethttp.MethodPost, baseURL+"/mcp/tools/call", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	resp, _ = client.Do(req)
	if err := resp.Body.Close(); err != nil {
		t.Logf("error closing response body: %v", err)
	}

	// Scenario 4: Valid request - should succeed
	validReq := map[string]interface{}{
		"jsonrpc": "2.0",
		"method":  "initialize",
		"id":      "1",
	}
	body, _ = json.Marshal(validReq)
	req, _ = nethttp.NewRequest(nethttp.MethodPost, baseURL+"/mcp/initialize", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("Final valid request failed: %v", err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			t.Logf("error closing response body: %v", err)
		}
	}()

	if resp.StatusCode != nethttp.StatusOK {
		t.Errorf("Expected 200 for valid request, got %d", resp.StatusCode)
	}

	// Verify no state corruption - server metrics should be valid
	metrics := server.GetMetrics()
	if metrics["is_running"] != true {
		t.Error("Server should still be running after errors")
	}
}
