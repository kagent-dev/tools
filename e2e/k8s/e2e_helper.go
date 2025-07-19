package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/kagent-dev/tools/internal/commands"
	"github.com/kagent-dev/tools/internal/logger"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
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
func GetMCPClient() (*MCPClient, error) {
	client := &MCPClient{
		baseURL: "http://127.0.0.1:30885/mcp", // MCP server responds at /mcp endpoint
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		log: logger.Get(),
	}

	r, err := client.listTools() // Validate connection by listing tools
	logger.Get().Info("MCP Client created", "baseURL", client.baseURL, "tools", r)

	return client, err
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

	return c.makeHTTPRequest(ctx, request)
}

// makeHTTPRequest performs an HTTP request to the MCP server
func (c *MCPClient) makeHTTPRequest(ctx context.Context, req MCPRequest) (*MCPResponse, error) {
	reqBody, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	c.log.Info("Making MCP request", "method", req.Method, "tool", req.Params)

	httpReq, err := http.NewRequestWithContext(ctx, "POST", c.baseURL, bytes.NewBuffer(reqBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create HTTP request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to make HTTP request: %w", err)
	}
	defer resp.Body.Close()

	// Handle different HTTP status codes
	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("MCP server not found (404): service may not be running or accessible")
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("HTTP request failed with status %d: %s", resp.StatusCode, string(body))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	var mcpResp MCPResponse
	if err := json.Unmarshal(body, &mcpResp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	if mcpResp.Error != nil {
		return nil, fmt.Errorf("MCP server returned error: %v", mcpResp.Error)
	}

	return &mcpResp, nil
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

	return c.makeHTTPRequest(ctx, request)
}

// InstallKAgentTools installs KAgent Tools using helm in the specified namespace
func InstallKAgentTools(namespace string, releaseName string) {
	// Use longer timeout for helm installation as it can take time to pull images
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	log := logger.Get()
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

	// Install KAgent Tools using helm with unique release name
	// Use absolute path from project root
	output, err := commands.NewCommandBuilder("helm").
		WithArgs("install", releaseName, "../../helm/kagent-tools").
		WithArgs("--namespace", namespace).
		WithArgs("-f").
		WithArgs("../../scripts/kind/test-values-e2e.yaml").
		WithArgs("--create-namespace").
		WithArgs("--wait").
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
			WithArgs("get", "pods", "-n", namespace, "-l", "app.kubernetes.io/name=kagent", "-o", "jsonpath={.items[*].status.phase}").
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

// Constants for default test values
const (
	DefaultReleaseName   = "kagent-tools-e2e"
	DefaultTestNamespace = "kagent-tools-e2e"
	DefaultTimeout       = 30 * time.Second // Increased for more realistic timeouts
)

// CreateNamespace creates a new Kubernetes namespace
func CreateNamespace(namespace string) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	log := logger.Get()
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

	log := logger.Get()
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
