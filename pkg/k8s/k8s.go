package k8s

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"strconv"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/tmc/langchaingo/llms"

	"github.com/kagent-dev/tools/internal/cache"
	"github.com/kagent-dev/tools/internal/commands"
	"github.com/kagent-dev/tools/internal/logger"
	"github.com/kagent-dev/tools/internal/security"
)

// K8sTool struct to hold the LLM model
type K8sTool struct {
	kubeconfig string
	llmModel   llms.Model
}

func NewK8sTool(llmModel llms.Model) *K8sTool {
	return &K8sTool{llmModel: llmModel}
}

func NewK8sToolWithConfig(kubeconfig string, llmModel llms.Model) *K8sTool {
	return &K8sTool{kubeconfig: kubeconfig, llmModel: llmModel}
}

// runKubectlCommandWithCacheInvalidation runs a kubectl command and invalidates cache if it's a modification operation
func (k *K8sTool) runKubectlCommandWithCacheInvalidation(ctx context.Context, args ...string) (*mcp.CallToolResult, error) {
	result, err := k.runKubectlCommand(ctx, args...)

	// If command succeeded and it's a modification command, invalidate cache
	if err == nil && len(args) > 0 {
		subcommand := args[0]
		switch subcommand {
		case "apply", "delete", "patch", "scale", "annotate", "label", "create", "run", "rollout":
			cache.InvalidateKubernetesCache()
		}
	}

	return result, err
}

// Enhanced kubectl get
func (k *K8sTool) handleKubectlGetEnhanced(ctx context.Context, request *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	resourceType := parseString(request, "resource_type", "")
	resourceName := parseString(request, "resource_name", "")
	namespace := parseString(request, "namespace", "")
	allNamespaces := parseString(request, "all_namespaces", "") == "true"
	output := parseString(request, "output", "wide")

	if resourceType == "" {
		return newToolResultError("resource_type parameter is required"), nil
	}

	args := []string{"get", resourceType}

	if resourceName != "" {
		args = append(args, resourceName)
	}

	if allNamespaces {
		args = append(args, "--all-namespaces")
	} else if namespace != "" {
		args = append(args, "-n", namespace)
	}

	if output != "" {
		args = append(args, "-o", output)
	} else {
		args = append(args, "-o", "json")
	}

	return k.runKubectlCommand(ctx, args...)
}

// Get pod logs
func (k *K8sTool) handleKubectlLogsEnhanced(ctx context.Context, request *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	podName := parseString(request, "pod_name", "")
	namespace := parseString(request, "namespace", "default")
	container := parseString(request, "container", "")
	tailLines := parseInt(request, "tail_lines", 50)

	if podName == "" {
		return newToolResultError("pod_name parameter is required"), nil
	}

	args := []string{"logs", podName, "-n", namespace}

	if container != "" {
		args = append(args, "-c", container)
	}

	if tailLines > 0 {
		args = append(args, "--tail", fmt.Sprintf("%d", tailLines))
	}

	return k.runKubectlCommand(ctx, args...)
}

// Apply manifest from content
func (k *K8sTool) handleApplyManifest(ctx context.Context, request *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	manifest := parseString(request, "manifest", "")

	if manifest == "" {
		return newToolResultError("manifest parameter is required"), nil
	}

	// Validate YAML content for security
	if err := security.ValidateYAMLContent(manifest); err != nil {
		return newToolResultError(fmt.Sprintf("Invalid manifest content: %v", err)), nil
	}

	// Create temporary file with secure permissions
	tmpFile, err := os.CreateTemp("", "k8s-manifest-*.yaml")
	if err != nil {
		return newToolResultError(fmt.Sprintf("Failed to create temp file: %v", err)), nil
	}

	// Ensure file is removed regardless of execution path
	defer func() {
		if removeErr := os.Remove(tmpFile.Name()); removeErr != nil {
			logger.Get().Error("Failed to remove temporary file", "error", removeErr, "file", tmpFile.Name())
		}
	}()

	// Set secure file permissions (readable/writable by owner only)
	if err := os.Chmod(tmpFile.Name(), 0600); err != nil {
		return newToolResultError(fmt.Sprintf("Failed to set file permissions: %v", err)), nil
	}

	// Write manifest content to temporary file
	if _, err := tmpFile.WriteString(manifest); err != nil {
		_ = tmpFile.Close()
		return newToolResultError(fmt.Sprintf("Failed to write to temp file: %v", err)), nil
	}

	// Close the file before passing to kubectl
	if err := tmpFile.Close(); err != nil {
		return newToolResultError(fmt.Sprintf("Failed to close temp file: %v", err)), nil
	}

	return k.runKubectlCommandWithCacheInvalidation(ctx, "apply", "-f", tmpFile.Name())
}

// runKubectlCommand is a helper function to execute kubectl commands
func (k *K8sTool) runKubectlCommand(ctx context.Context, args ...string) (*mcp.CallToolResult, error) {
	output, err := commands.NewCommandBuilder("kubectl").
		WithArgs(args...).
		WithKubeconfig(k.kubeconfig).
		Execute(ctx)

	if err != nil {
		return newToolResultError(err.Error()), nil
	}

	return newToolResultText(output), nil
}

// Helper functions for parsing request parameters (adapted for new SDK)
func parseString(request *mcp.CallToolRequest, key, defaultValue string) string {
	if request.Params.Arguments == nil {
		return defaultValue
	}

	var args map[string]any
	if err := json.Unmarshal(request.Params.Arguments, &args); err != nil {
		return defaultValue
	}

	if val, exists := args[key]; exists {
		if str, ok := val.(string); ok {
			return str
		}
	}
	return defaultValue
}

func parseInt(request *mcp.CallToolRequest, key string, defaultValue int) int {
	if request.Params.Arguments == nil {
		return defaultValue
	}

	var args map[string]any
	if err := json.Unmarshal(request.Params.Arguments, &args); err != nil {
		return defaultValue
	}

	if val, exists := args[key]; exists {
		switch v := val.(type) {
		case int:
			return v
		case float64:
			return int(v)
		case string:
			if i, err := strconv.Atoi(v); err == nil {
				return i
			}
		}
	}
	return defaultValue
}

// Helper functions for creating tool results (adapted for new SDK)
func newToolResultError(message string) *mcp.CallToolResult {
	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: message}},
		IsError: true,
	}
}

func newToolResultText(text string) *mcp.CallToolResult {
	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: text}},
	}
}

// ToolRegistry is an interface for tool registration (to avoid import cycles)
type ToolRegistry interface {
	Register(tool *mcp.Tool, handler mcp.ToolHandler)
}

// RegisterTools registers all k8s tools with the MCP server
func RegisterTools(server *mcp.Server, llm llms.Model, kubeconfig string) error {
	return RegisterToolsWithRegistry(server, nil, llm, kubeconfig)
}

// RegisterToolsWithRegistry registers all k8s tools with the MCP server and optionally with a tool registry
func RegisterToolsWithRegistry(server *mcp.Server, registry ToolRegistry, llm llms.Model, kubeconfig string) error {
	logger.Get().Info("Registering Kubernetes tools")
	k8sTool := NewK8sToolWithConfig(kubeconfig, llm)

	// Helper function to register tool with both server and registry
	registerTool := func(tool *mcp.Tool, handler mcp.ToolHandler) {
		server.AddTool(tool, handler)
		if registry != nil {
			registry.Register(tool, handler)
		}
	}

	// Register k8s_get_resources tool
	registerTool(&mcp.Tool{
		Name:        "k8s_get_resources",
		Description: "Get Kubernetes resources using kubectl",
		InputSchema: &jsonschema.Schema{
			Type: "object",
			Properties: map[string]*jsonschema.Schema{
				"resource_type": {
					Type:        "string",
					Description: "Type of resource (pod, service, deployment, etc.)",
				},
				"resource_name": {
					Type:        "string",
					Description: "Name of specific resource (optional)",
				},
				"namespace": {
					Type:        "string",
					Description: "Namespace to query (optional)",
				},
				"all_namespaces": {
					Type:        "string",
					Description: "Query all namespaces (true/false)",
				},
				"output": {
					Type:        "string",
					Description: "Output format (json, yaml, wide)",
				},
			},
			Required: []string{"resource_type"},
		},
	}, k8sTool.handleKubectlGetEnhanced)

	// Register k8s_get_pod_logs tool
	registerTool(&mcp.Tool{
		Name:        "k8s_get_pod_logs",
		Description: "Get logs from a Kubernetes pod",
		InputSchema: &jsonschema.Schema{
			Type: "object",
			Properties: map[string]*jsonschema.Schema{
				"pod_name": {
					Type:        "string",
					Description: "Name of the pod",
				},
				"namespace": {
					Type:        "string",
					Description: "Namespace of the pod (default: default)",
				},
				"container": {
					Type:        "string",
					Description: "Container name (for multi-container pods)",
				},
				"tail_lines": {
					Type:        "number",
					Description: "Number of lines to show from the end (default: 50)",
				},
			},
			Required: []string{"pod_name"},
		},
	}, k8sTool.handleKubectlLogsEnhanced)

	// Register k8s_apply_manifest tool
	registerTool(&mcp.Tool{
		Name:        "k8s_apply_manifest",
		Description: "Apply a YAML manifest to the Kubernetes cluster",
		InputSchema: &jsonschema.Schema{
			Type: "object",
			Properties: map[string]*jsonschema.Schema{
				"manifest": {
					Type:        "string",
					Description: "YAML manifest content",
				},
			},
			Required: []string{"manifest"},
		},
	}, k8sTool.handleApplyManifest)

	// Register k8s_scale tool
	registerTool(&mcp.Tool{
		Name:        "k8s_scale",
		Description: "Scale a Kubernetes deployment",
		InputSchema: &jsonschema.Schema{
			Type: "object",
			Properties: map[string]*jsonschema.Schema{
				"name": {
					Type:        "string",
					Description: "Name of the deployment",
				},
				"namespace": {
					Type:        "string",
					Description: "Namespace of the deployment (default: default)",
				},
				"replicas": {
					Type:        "number",
					Description: "Number of replicas",
				},
			},
			Required: []string{"name", "replicas"},
		},
	}, k8sTool.handleScaleDeployment)

	// Register k8s_delete_resource tool
	registerTool(&mcp.Tool{
		Name:        "k8s_delete_resource",
		Description: "Delete a Kubernetes resource",
		InputSchema: &jsonschema.Schema{
			Type: "object",
			Properties: map[string]*jsonschema.Schema{
				"resource_type": {
					Type:        "string",
					Description: "Type of resource (pod, service, deployment, etc.)",
				},
				"resource_name": {
					Type:        "string",
					Description: "Name of the resource",
				},
				"namespace": {
					Type:        "string",
					Description: "Namespace of the resource (default: default)",
				},
			},
			Required: []string{"resource_type", "resource_name"},
		},
	}, k8sTool.handleDeleteResource)

	// Register k8s_get_events tool
	registerTool(&mcp.Tool{
		Name:        "k8s_get_events",
		Description: "Get events from a Kubernetes namespace",
		InputSchema: &jsonschema.Schema{
			Type: "object",
			Properties: map[string]*jsonschema.Schema{
				"namespace": {
					Type:        "string",
					Description: "Namespace to get events from (default: default)",
				},
				"output": {
					Type:        "string",
					Description: "Output format (json, yaml, wide)",
				},
			},
		},
	}, k8sTool.handleGetEvents)

	// Register k8s_execute_command tool
	registerTool(&mcp.Tool{
		Name:        "k8s_execute_command",
		Description: "Execute a command in a Kubernetes pod",
		InputSchema: &jsonschema.Schema{
			Type: "object",
			Properties: map[string]*jsonschema.Schema{
				"pod_name": {
					Type:        "string",
					Description: "Name of the pod to execute in",
				},
				"namespace": {
					Type:        "string",
					Description: "Namespace of the pod (default: default)",
				},
				"container": {
					Type:        "string",
					Description: "Container name (for multi-container pods)",
				},
				"command": {
					Type:        "string",
					Description: "Command to execute",
				},
			},
			Required: []string{"pod_name", "command"},
		},
	}, k8sTool.handleExecCommand)

	// Register k8s_describe tool
	registerTool(&mcp.Tool{
		Name:        "k8s_describe",
		Description: "Describe a Kubernetes resource",
		InputSchema: &jsonschema.Schema{
			Type: "object",
			Properties: map[string]*jsonschema.Schema{
				"resource_type": {
					Type:        "string",
					Description: "Type of resource",
				},
				"resource_name": {
					Type:        "string",
					Description: "Name of the resource",
				},
				"namespace": {
					Type:        "string",
					Description: "Namespace of the resource (optional)",
				},
			},
			Required: []string{"resource_type", "resource_name"},
		},
	}, k8sTool.handleKubectlDescribeTool)

	// Register k8s_get_available_api_resources tool
	registerTool(&mcp.Tool{
		Name:        "k8s_get_available_api_resources",
		Description: "Get available Kubernetes API resources",
		InputSchema: &jsonschema.Schema{
			Type:       "object",
			Properties: map[string]*jsonschema.Schema{},
		},
	}, k8sTool.handleGetAvailableAPIResources)

	return nil
}

// Scale deployment
func (k *K8sTool) handleScaleDeployment(ctx context.Context, request *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	deploymentName := parseString(request, "name", "")
	namespace := parseString(request, "namespace", "default")
	replicas := parseInt(request, "replicas", 1)

	if deploymentName == "" {
		return newToolResultError("name parameter is required"), nil
	}

	args := []string{"scale", "deployment", deploymentName, "--replicas", fmt.Sprintf("%d", replicas), "-n", namespace}

	return k.runKubectlCommandWithCacheInvalidation(ctx, args...)
}

// Delete resource
func (k *K8sTool) handleDeleteResource(ctx context.Context, request *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	resourceType := parseString(request, "resource_type", "")
	resourceName := parseString(request, "resource_name", "")
	namespace := parseString(request, "namespace", "default")

	if resourceType == "" || resourceName == "" {
		return newToolResultError("resource_type and resource_name parameters are required"), nil
	}

	args := []string{"delete", resourceType, resourceName, "-n", namespace}

	return k.runKubectlCommandWithCacheInvalidation(ctx, args...)
}

// Get cluster events
func (k *K8sTool) handleGetEvents(ctx context.Context, request *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	namespace := parseString(request, "namespace", "")
	output := parseString(request, "output", "wide")

	args := []string{"get", "events"}
	if namespace != "" {
		args = append(args, "-n", namespace)
	} else {
		args = append(args, "--all-namespaces")
	}

	if output != "" {
		args = append(args, "-o", output)
	}

	return k.runKubectlCommand(ctx, args...)
}

// Execute command in pod
func (k *K8sTool) handleExecCommand(ctx context.Context, request *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	podName := parseString(request, "pod_name", "")
	namespace := parseString(request, "namespace", "default")
	command := parseString(request, "command", "")

	if podName == "" || command == "" {
		return newToolResultError("pod_name and command parameters are required"), nil
	}

	// Validate pod name for security
	if err := security.ValidateK8sResourceName(podName); err != nil {
		return newToolResultError(fmt.Sprintf("Invalid pod name: %v", err)), nil
	}

	// Validate namespace for security
	if err := security.ValidateNamespace(namespace); err != nil {
		return newToolResultError(fmt.Sprintf("Invalid namespace: %v", err)), nil
	}

	// Validate command input for security
	if err := security.ValidateCommandInput(command); err != nil {
		return newToolResultError(fmt.Sprintf("Invalid command: %v", err)), nil
	}

	args := []string{"exec", podName, "-n", namespace, "--", command}

	return k.runKubectlCommand(ctx, args...)
}

// Kubectl describe tool
func (k *K8sTool) handleKubectlDescribeTool(ctx context.Context, request *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	resourceType := parseString(request, "resource_type", "")
	resourceName := parseString(request, "resource_name", "")
	namespace := parseString(request, "namespace", "")

	if resourceType == "" || resourceName == "" {
		return newToolResultError("resource_type and resource_name parameters are required"), nil
	}

	args := []string{"describe", resourceType, resourceName}
	if namespace != "" {
		args = append(args, "-n", namespace)
	}

	return k.runKubectlCommand(ctx, args...)
}

// Get available API resources
func (k *K8sTool) handleGetAvailableAPIResources(ctx context.Context, request *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return k.runKubectlCommand(ctx, "api-resources")
}
