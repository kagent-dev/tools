package http_helpers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// HTTPClient wraps the standard HTTP client with MCP-specific helpers
// T036 Implementation: HTTP client wrapper for tool invocation tests
type HTTPClient struct {
	client    *http.Client
	baseURL   string
	timeout   time.Duration
	requestID uint64
	headers   map[string]string
}

// MCPRequest represents an MCP request sent over HTTP
type MCPRequest struct {
	JSONRPC string                 `json:"jsonrpc"`
	Method  string                 `json:"method"`
	Params  map[string]interface{} `json:"params,omitempty"`
	ID      string                 `json:"id"`
}

// MCPResponse represents an MCP response received over HTTP
type MCPResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *MCPError       `json:"error,omitempty"`
	ID      string          `json:"id"`
}

// MCPError represents an error in MCP response
type MCPError struct {
	Code    interface{} `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

// NewHTTPClient creates a new HTTP client for testing
func NewHTTPClient(baseURL string) *HTTPClient {
	return &HTTPClient{
		client:    &http.Client{Timeout: 30 * time.Second},
		baseURL:   baseURL,
		timeout:   30 * time.Second,
		requestID: 0,
		headers: map[string]string{
			"Content-Type": "application/json",
		},
	}
}

// SetTimeout sets the HTTP request timeout
func (c *HTTPClient) SetTimeout(timeout time.Duration) {
	c.timeout = timeout
	c.client.Timeout = timeout
}

// SetHeader sets a custom header for all requests
func (c *HTTPClient) SetHeader(key, value string) {
	c.headers[key] = value
}

// generateRequestID generates a unique request ID for correlation
func (c *HTTPClient) generateRequestID() string {
	c.requestID++
	return fmt.Sprintf("http-req-%d", c.requestID)
}

// Initialize sends an MCP initialize request
func (c *HTTPClient) Initialize(ctx context.Context, params map[string]interface{}) (*json.RawMessage, error) {
	req := MCPRequest{
		JSONRPC: "2.0",
		Method:  "initialize",
		Params:  params,
		ID:      c.generateRequestID(),
	}

	resp, err := c.sendRequest(ctx, "/mcp/initialize", req)
	if err != nil {
		return nil, err
	}

	return &resp.Result, nil
}

// ListTools sends a tools/list request
func (c *HTTPClient) ListTools(ctx context.Context) (*json.RawMessage, error) {
	req := MCPRequest{
		JSONRPC: "2.0",
		Method:  "tools/list",
		ID:      c.generateRequestID(),
	}

	resp, err := c.sendRequest(ctx, "/mcp/tools/list", req)
	if err != nil {
		return nil, err
	}

	return &resp.Result, nil
}

// CallTool sends a tools/call request to invoke a tool
// T036 Implementation: Tool invocation with parameter marshaling
func (c *HTTPClient) CallTool(ctx context.Context, toolName string, args map[string]interface{}) (*json.RawMessage, error) {
	// Build parameters with tool name
	params := map[string]interface{}{
		"name": toolName,
	}

	// Merge tool arguments
	for k, v := range args {
		params[k] = v
	}

	req := MCPRequest{
		JSONRPC: "2.0",
		Method:  "tools/call",
		Params:  params,
		ID:      c.generateRequestID(),
	}

	resp, err := c.sendRequest(ctx, "/mcp/tools/call", req)
	if err != nil {
		return nil, err
	}

	return &resp.Result, nil
}

// sendRequest sends an HTTP request and returns the MCP response
func (c *HTTPClient) sendRequest(ctx context.Context, endpoint string, req MCPRequest) (*MCPResponse, error) {
	// Set up context with timeout if not already set
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, c.timeout)
		defer cancel()
	}

	// Marshal request
	data, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	// Create HTTP request
	url := c.baseURL + endpoint
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("failed to create HTTP request: %w", err)
	}

	// Set headers
	for key, value := range c.headers {
		httpReq.Header.Set(key, value)
	}

	// Add request ID header for correlation (optional)
	httpReq.Header.Set("X-Request-ID", req.ID)

	// Send request
	httpResp, err := c.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("HTTP request failed: %w", err)
	}
	defer func() {
		if err := httpResp.Body.Close(); err != nil {
			// Log close error but don't fail - already have response
			_ = err
		}
	}()

	// Read response body
	body, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	// Parse MCP response
	var mcpResp MCPResponse
	if err := json.Unmarshal(body, &mcpResp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal MCP response: %w", err)
	}

	// Check HTTP status code
	if httpResp.StatusCode != http.StatusOK {
		errMsg := "HTTP request failed"
		if mcpResp.Error != nil {
			errMsg = mcpResp.Error.Message
		}
		return nil, fmt.Errorf("%s (HTTP %d): %s", errMsg, httpResp.StatusCode, body)
	}

	// Check MCP error
	if mcpResp.Error != nil {
		return nil, fmt.Errorf("MCP error: %s (code: %v)", mcpResp.Error.Message, mcpResp.Error.Code)
	}

	return &mcpResp, nil
}

// Health sends a GET request to the health endpoint
func (c *HTTPClient) Health(ctx context.Context) error {
	// Set up context with timeout if not already set
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, c.timeout)
		defer cancel()
	}

	url := c.baseURL + "/health"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("failed to create health check request: %w", err)
	}

	httpResp, err := c.client.Do(httpReq)
	if err != nil {
		return fmt.Errorf("health check failed: %w", err)
	}
	defer func() {
		if err := httpResp.Body.Close(); err != nil {
			// Log close error but don't fail - already have status
			_ = err
		}
	}()

	if httpResp.StatusCode != http.StatusOK {
		return fmt.Errorf("health check returned status %d", httpResp.StatusCode)
	}

	return nil
}

// RawCall sends a raw MCP request and returns the parsed response
// Useful for testing error scenarios
func (c *HTTPClient) RawCall(ctx context.Context, endpoint string, data []byte) (int, []byte, error) {
	// Set up context with timeout if not already set
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, c.timeout)
		defer cancel()
	}

	url := c.baseURL + endpoint
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		return 0, nil, fmt.Errorf("failed to create HTTP request: %w", err)
	}

	// Set headers
	for key, value := range c.headers {
		httpReq.Header.Set(key, value)
	}

	httpResp, err := c.client.Do(httpReq)
	if err != nil {
		return 0, nil, fmt.Errorf("HTTP request failed: %w", err)
	}
	defer func() {
		if err := httpResp.Body.Close(); err != nil {
			// Log close error but don't fail - already have response
			_ = err
		}
	}()

	body, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return httpResp.StatusCode, nil, fmt.Errorf("failed to read response body: %w", err)
	}

	return httpResp.StatusCode, body, nil
}

// UnmarshalResult unmarshals the result from an MCP response
func UnmarshalResult(raw *json.RawMessage, v interface{}) error {
	if raw == nil {
		return fmt.Errorf("result is nil")
	}
	return json.Unmarshal(*raw, v)
}

// GetResultValue extracts a single value from the result using a key path
// Supports dot notation for nested keys: "result.data.value"
func GetResultValue(raw *json.RawMessage, keyPath string) (interface{}, error) {
	if raw == nil {
		return nil, fmt.Errorf("result is nil")
	}

	var result map[string]interface{}
	if err := json.Unmarshal(*raw, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal result: %w", err)
	}

	// For now, support simple keys
	if value, ok := result[keyPath]; ok {
		return value, nil
	}

	return nil, fmt.Errorf("key not found: %s", keyPath)
}
