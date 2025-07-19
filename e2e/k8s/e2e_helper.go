package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/kagent-dev/tools/internal/logger"
)

// MCPClient represents a client for communicating with the MCP server
type MCPClient struct {
	baseURL    string
	httpClient *http.Client
	log        *slog.Logger
}

// MCPRequest represents a request to the MCP server
type MCPRequest struct {
	Jsonrpc string      `json:"jsonrpc"`
	ID      string      `json:"id"`
	Method  string      `json:"method"`
	Params  interface{} `json:"params"`
}

// MCPResponse represents a response from the MCP server
type MCPResponse struct {
	Jsonrpc string      `json:"jsonrpc"`
	ID      string      `json:"id"`
	Result  interface{} `json:"result,omitempty"`
	Error   *MCPError   `json:"error,omitempty"`
}

// MCPError represents an error response from the MCP server
type MCPError struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

// CallToolParams represents parameters for calling a tool
type CallToolParams struct {
	Name      string                 `json:"name"`
	Arguments map[string]interface{} `json:"arguments,omitempty"`
}

// GetMCPClient creates a new MCP client configured for the e2e test environment
func GetMCPClient() *MCPClient {
	return &MCPClient{
		baseURL: "http://127.0.0.1:30884", // MCP server responds at root path, not /mcp
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		log: logger.Get(),
	}
}

// callTool makes a tool call to the MCP server
func (c *MCPClient) callTool(ctx context.Context, toolName string, arguments map[string]interface{}) (*MCPResponse, error) {
	params := CallToolParams{
		Name:      toolName,
		Arguments: arguments,
	}

	request := MCPRequest{
		Jsonrpc: "2.0",
		ID:      fmt.Sprintf("test-%d", time.Now().UnixNano()),
		Method:  "tools/call",
		Params:  params,
	}

	return c.makeRequest(ctx, request)
}

// makeRequest sends an HTTP request to the MCP server
func (c *MCPClient) makeRequest(ctx context.Context, request MCPRequest) (*MCPResponse, error) {
	c.log.Info("Making MCP request", "method", request.Method, "tool", request.Params)

	jsonData, err := json.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", c.baseURL, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create HTTP request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to make HTTP request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("HTTP request failed with status %d: %s", resp.StatusCode, string(body))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	var mcpResponse MCPResponse
	if err := json.Unmarshal(body, &mcpResponse); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	if mcpResponse.Error != nil {
		return nil, fmt.Errorf("MCP error %d: %s", mcpResponse.Error.Code, mcpResponse.Error.Message)
	}

	c.log.Info("MCP request completed successfully", "method", request.Method)
	return &mcpResponse, nil
}

// k8sListResources calls the k8s_get_resources tool
func (c *MCPClient) k8sListResources(resourceType string) (*MCPResponse, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	arguments := map[string]interface{}{
		"resource_type": resourceType,
		"output":        "json",
	}

	return c.callTool(ctx, "k8s_get_resources", arguments)
}

// helmListReleases calls the helm_list_releases tool
func (c *MCPClient) helmListReleases() (*MCPResponse, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	arguments := map[string]interface{}{
		"all_namespaces": "true",
		"output":         "json",
	}

	return c.callTool(ctx, "helm_list_releases", arguments)
}

// istioInstall calls the istio_install_istio tool
func (c *MCPClient) istioInstall(profile string) (*MCPResponse, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second) // Istio install can take time
	defer cancel()

	arguments := map[string]interface{}{
		"profile": profile,
	}

	return c.callTool(ctx, "istio_install_istio", arguments)
}

// argoRolloutsList calls the argo_rollouts_get tool to list rollouts
func (c *MCPClient) argoRolloutsList(namespace string) (*MCPResponse, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	arguments := map[string]interface{}{
		"namespace": namespace,
		"output":    "json",
	}

	return c.callTool(ctx, "argo_rollouts_get", arguments)
}

// prometheusQuery calls the prometheus_query_tool
func (c *MCPClient) prometheusQuery(query string) (*MCPResponse, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	arguments := map[string]interface{}{
		"query":          query,
		"prometheus_url": "http://localhost:9090",
	}

	return c.callTool(ctx, "prometheus_query_tool", arguments)
}

// utilsDateTime calls the datetime_get_current_time tool
func (c *MCPClient) utilsDateTime() (*MCPResponse, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	return c.callTool(ctx, "datetime_get_current_time", nil)
}

// ciliumStatus calls the cilium_status_and_version tool
func (c *MCPClient) ciliumStatus() (*MCPResponse, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	return c.callTool(ctx, "cilium_status_and_version", nil)
}

// listTools calls the tools/list method to get available tools
func (c *MCPClient) listTools() (*MCPResponse, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	request := MCPRequest{
		Jsonrpc: "2.0",
		ID:      fmt.Sprintf("test-%d", time.Now().UnixNano()),
		Method:  "tools/list",
		Params:  struct{}{},
	}

	return c.makeRequest(ctx, request)
}

// validateConnection tests the connection to the MCP server by listing tools
func (c *MCPClient) validateConnection() error {
	c.log.Info("Validating MCP server connection", "url", c.baseURL)

	_, err := c.listTools()
	if err != nil {
		return fmt.Errorf("failed to connect to MCP server: %w", err)
	}

	c.log.Info("MCP server connection validated successfully")
	return nil
}

// Constants for default test values
const (
	DefaultTestNamespace = "kagent-tools-e2e"
	DefaultTimeout       = 30 * time.Second // Increased for more realistic timeouts
)

// TestMCPConnection creates a simple test to verify MCP client connectivity
func TestMCPConnection() error {
	client := GetMCPClient()

	// Test basic connection
	if err := client.validateConnection(); err != nil {
		return fmt.Errorf("MCP connection test failed: %w", err)
	}

	// Test a simple tool call (datetime)
	response, err := client.utilsDateTime()
	if err != nil {
		return fmt.Errorf("MCP tool call test failed: %w", err)
	}

	client.log.Info("MCP connection test successful", "response", response)
	return nil
}

// ExampleMCPUsage shows how to use the MCP client for different operations
func ExampleMCPUsage() error {
	client := GetMCPClient()

	// Example: List namespaces
	client.log.Info("Example: Listing namespaces")
	response, err := client.k8sListResources("namespace")
	if err != nil {
		client.log.Error("Failed to list namespaces", "error", err)
		return err
	}
	client.log.Info("Namespaces listed successfully", "response", response)

	// Example: List helm releases
	client.log.Info("Example: Listing helm releases")
	response, err = client.helmListReleases()
	if err != nil {
		client.log.Error("Failed to list helm releases", "error", err)
		return err
	}
	client.log.Info("Helm releases listed successfully", "response", response)

	// Example: Get current time
	client.log.Info("Example: Getting current time")
	response, err = client.utilsDateTime()
	if err != nil {
		client.log.Error("Failed to get current time", "error", err)
		return err
	}
	client.log.Info("Current time retrieved successfully", "response", response)

	return nil
}
