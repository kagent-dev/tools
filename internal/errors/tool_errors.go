package errors

import (
	"fmt"
	"strings"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
)

// ToolError represents a structured error with context and recovery suggestions
type ToolError struct {
	Operation    string                 `json:"operation"`
	Cause        error                  `json:"cause"`
	Suggestions  []string               `json:"suggestions"`
	IsRetryable  bool                   `json:"is_retryable"`
	Timestamp    time.Time              `json:"timestamp"`
	ErrorCode    string                 `json:"error_code"`
	Component    string                 `json:"component"`
	ResourceType string                 `json:"resource_type,omitempty"`
	ResourceName string                 `json:"resource_name,omitempty"`
	Context      map[string]interface{} `json:"context,omitempty"`
}

// Error implements the error interface
func (e *ToolError) Error() string {
	return fmt.Sprintf("[%s] %s failed: %v", e.Component, e.Operation, e.Cause)
}

// ToMCPResult converts the error to an MCP result with rich context
func (e *ToolError) ToMCPResult() *mcp.CallToolResult {
	var message strings.Builder

	// Format the error message with context
	message.WriteString(fmt.Sprintf("❌ **%s Error**\n\n", e.Component))
	message.WriteString(fmt.Sprintf("**Operation**: %s\n", e.Operation))
	message.WriteString(fmt.Sprintf("**Error**: %s\n", e.Cause.Error()))

	if e.ResourceType != "" {
		message.WriteString(fmt.Sprintf("**Resource Type**: %s\n", e.ResourceType))
	}

	if e.ResourceName != "" {
		message.WriteString(fmt.Sprintf("**Resource Name**: %s\n", e.ResourceName))
	}

	message.WriteString(fmt.Sprintf("**Error Code**: %s\n", e.ErrorCode))
	message.WriteString(fmt.Sprintf("**Timestamp**: %s\n", e.Timestamp.Format(time.RFC3339)))

	if e.IsRetryable {
		message.WriteString("**Retryable**: Yes\n")
	} else {
		message.WriteString("**Retryable**: No\n")
	}

	if len(e.Suggestions) > 0 {
		message.WriteString("\n**💡 Suggestions**:\n")
		for i, suggestion := range e.Suggestions {
			message.WriteString(fmt.Sprintf("%d. %s\n", i+1, suggestion))
		}
	}

	if len(e.Context) > 0 {
		message.WriteString("\n**📋 Context**:\n")
		for key, value := range e.Context {
			message.WriteString(fmt.Sprintf("- %s: %v\n", key, value))
		}
	}

	return mcp.NewToolResultError(message.String())
}

// NewToolError creates a new structured tool error
func NewToolError(component, operation string, cause error) *ToolError {
	return &ToolError{
		Operation:   operation,
		Cause:       cause,
		Suggestions: []string{},
		IsRetryable: false,
		Timestamp:   time.Now(),
		ErrorCode:   "UNKNOWN",
		Component:   component,
		Context:     make(map[string]interface{}),
	}
}

// WithSuggestions adds recovery suggestions to the error
func (e *ToolError) WithSuggestions(suggestions ...string) *ToolError {
	e.Suggestions = append(e.Suggestions, suggestions...)
	return e
}

// WithRetryable sets whether the error is retryable
func (e *ToolError) WithRetryable(retryable bool) *ToolError {
	e.IsRetryable = retryable
	return e
}

// WithErrorCode sets the error code
func (e *ToolError) WithErrorCode(code string) *ToolError {
	e.ErrorCode = code
	return e
}

// WithResource adds resource information to the error
func (e *ToolError) WithResource(resourceType, resourceName string) *ToolError {
	e.ResourceType = resourceType
	e.ResourceName = resourceName
	return e
}

// WithContext adds contextual information to the error
func (e *ToolError) WithContext(key string, value interface{}) *ToolError {
	e.Context[key] = value
	return e
}

// Common error creators for different components

// NewKubernetesError creates a Kubernetes-specific error
func NewKubernetesError(operation string, cause error) *ToolError {
	err := NewToolError("Kubernetes", operation, cause)

	// Add Kubernetes-specific suggestions based on common errors
	if strings.Contains(cause.Error(), "connection refused") {
		err = err.WithSuggestions(
			"Check if the Kubernetes cluster is running",
			"Verify your kubeconfig is correct",
			"Ensure network connectivity to the cluster",
		).WithRetryable(true).WithErrorCode("K8S_CONNECTION_ERROR")
	} else if strings.Contains(cause.Error(), "forbidden") {
		err = err.WithSuggestions(
			"Check your RBAC permissions",
			"Verify your service account has the required permissions",
			"Contact your cluster administrator",
		).WithRetryable(false).WithErrorCode("K8S_PERMISSION_ERROR")
	} else if strings.Contains(cause.Error(), "not found") {
		err = err.WithSuggestions(
			"Check if the resource exists",
			"Verify the resource name and namespace",
			"List available resources to confirm",
		).WithRetryable(false).WithErrorCode("K8S_RESOURCE_NOT_FOUND")
	} else if strings.Contains(cause.Error(), "already exists") {
		err = err.WithSuggestions(
			"Use a different name for the resource",
			"Delete the existing resource first",
			"Use 'kubectl apply' instead of 'kubectl create'",
		).WithRetryable(false).WithErrorCode("K8S_RESOURCE_EXISTS")
	} else {
		err = err.WithSuggestions(
			"Check the kubectl command syntax",
			"Verify your kubeconfig is valid",
			"Check cluster connectivity",
		).WithRetryable(true).WithErrorCode("K8S_GENERIC_ERROR")
	}

	return err
}

// NewHelmError creates a Helm-specific error
func NewHelmError(operation string, cause error) *ToolError {
	err := NewToolError("Helm", operation, cause)

	if strings.Contains(cause.Error(), "not found") {
		err = err.WithSuggestions(
			"Check if the Helm release exists",
			"Verify the release name and namespace",
			"Use 'helm list' to see available releases",
		).WithRetryable(false).WithErrorCode("HELM_RELEASE_NOT_FOUND")
	} else if strings.Contains(cause.Error(), "already exists") {
		err = err.WithSuggestions(
			"Use a different release name",
			"Upgrade the existing release instead",
			"Uninstall the existing release first",
		).WithRetryable(false).WithErrorCode("HELM_RELEASE_EXISTS")
	} else if strings.Contains(cause.Error(), "repository") {
		err = err.WithSuggestions(
			"Add the required Helm repository",
			"Update your Helm repositories",
			"Check repository URL and credentials",
		).WithRetryable(true).WithErrorCode("HELM_REPOSITORY_ERROR")
	} else {
		err = err.WithSuggestions(
			"Check the Helm command syntax",
			"Verify your kubeconfig is valid",
			"Ensure Helm is properly installed",
		).WithRetryable(true).WithErrorCode("HELM_GENERIC_ERROR")
	}

	return err
}

// NewIstioError creates an Istio-specific error
func NewIstioError(operation string, cause error) *ToolError {
	err := NewToolError("Istio", operation, cause)

	if strings.Contains(cause.Error(), "not found") {
		err = err.WithSuggestions(
			"Check if Istio is installed in the cluster",
			"Verify the pod/service name and namespace",
			"Ensure Istio sidecar is injected",
		).WithRetryable(false).WithErrorCode("ISTIO_RESOURCE_NOT_FOUND")
	} else if strings.Contains(cause.Error(), "connection refused") {
		err = err.WithSuggestions(
			"Check if Istio control plane is running",
			"Verify Istio proxy is healthy",
			"Check network policies",
		).WithRetryable(true).WithErrorCode("ISTIO_CONNECTION_ERROR")
	} else {
		err = err.WithSuggestions(
			"Check istioctl command syntax",
			"Verify Istio installation",
			"Check Istio proxy status",
		).WithRetryable(true).WithErrorCode("ISTIO_GENERIC_ERROR")
	}

	return err
}

// NewPrometheusError creates a Prometheus-specific error
func NewPrometheusError(operation string, cause error) *ToolError {
	err := NewToolError("Prometheus", operation, cause)

	if strings.Contains(cause.Error(), "connection refused") {
		err = err.WithSuggestions(
			"Check if Prometheus server is running",
			"Verify the Prometheus URL",
			"Check network connectivity",
		).WithRetryable(true).WithErrorCode("PROMETHEUS_CONNECTION_ERROR")
	} else if strings.Contains(cause.Error(), "parse error") {
		err = err.WithSuggestions(
			"Check your PromQL query syntax",
			"Verify metric names and labels",
			"Test the query in Prometheus UI",
		).WithRetryable(false).WithErrorCode("PROMETHEUS_QUERY_ERROR")
	} else {
		err = err.WithSuggestions(
			"Check Prometheus server status",
			"Verify the query format",
			"Check authentication if required",
		).WithRetryable(true).WithErrorCode("PROMETHEUS_GENERIC_ERROR")
	}

	return err
}

// NewArgoError creates an Argo-specific error
func NewArgoError(operation string, cause error) *ToolError {
	err := NewToolError("Argo Rollouts", operation, cause)

	if strings.Contains(cause.Error(), "not found") {
		err = err.WithSuggestions(
			"Check if Argo Rollouts is installed",
			"Verify the rollout name and namespace",
			"Use 'kubectl get rollouts' to list available rollouts",
		).WithRetryable(false).WithErrorCode("ARGO_ROLLOUT_NOT_FOUND")
	} else if strings.Contains(cause.Error(), "plugin") {
		err = err.WithSuggestions(
			"Install the kubectl argo rollouts plugin",
			"Check plugin version compatibility",
			"Verify plugin installation path",
		).WithRetryable(true).WithErrorCode("ARGO_PLUGIN_ERROR")
	} else {
		err = err.WithSuggestions(
			"Check Argo Rollouts installation",
			"Verify the command syntax",
			"Check RBAC permissions",
		).WithRetryable(true).WithErrorCode("ARGO_GENERIC_ERROR")
	}

	return err
}

// NewCiliumError creates a Cilium-specific error
func NewCiliumError(operation string, cause error) *ToolError {
	err := NewToolError("Cilium", operation, cause)

	if strings.Contains(cause.Error(), "not found") {
		err = err.WithSuggestions(
			"Check if Cilium is installed",
			"Verify the cilium CLI is installed",
			"Check Cilium agent status",
		).WithRetryable(false).WithErrorCode("CILIUM_NOT_FOUND")
	} else if strings.Contains(cause.Error(), "connection") {
		err = err.WithSuggestions(
			"Check Cilium agent connectivity",
			"Verify cluster mesh configuration",
			"Check Cilium operator status",
		).WithRetryable(true).WithErrorCode("CILIUM_CONNECTION_ERROR")
	} else {
		err = err.WithSuggestions(
			"Check Cilium installation",
			"Verify cilium CLI version",
			"Check Cilium system pods",
		).WithRetryable(true).WithErrorCode("CILIUM_GENERIC_ERROR")
	}

	return err
}

// NewValidationError creates a validation error
func NewValidationError(field, message string) *ToolError {
	err := NewToolError("Validation", fmt.Sprintf("validate %s", field), fmt.Errorf("%s", message))

	err = err.WithSuggestions(
		"Check the input format",
		"Refer to the documentation for valid values",
		"Verify the parameter requirements",
	).WithRetryable(false).WithErrorCode("VALIDATION_ERROR")

	return err
}

// NewSecurityError creates a security-related error
func NewSecurityError(operation string, cause error) *ToolError {
	err := NewToolError("Security", operation, cause)

	err = err.WithSuggestions(
		"Review the input for potentially dangerous content",
		"Use only trusted input sources",
		"Contact security team if needed",
	).WithRetryable(false).WithErrorCode("SECURITY_ERROR")

	return err
}

// NewTimeoutError creates a timeout error
func NewTimeoutError(operation string, timeout time.Duration) *ToolError {
	cause := fmt.Errorf("operation timed out after %v", timeout)
	err := NewToolError("Timeout", operation, cause)

	err = err.WithSuggestions(
		"Try the operation again",
		"Check network connectivity",
		"Increase timeout if possible",
	).WithRetryable(true).WithErrorCode("TIMEOUT_ERROR")

	return err
}

// NewCommandError creates a command execution error
func NewCommandError(command string, cause error) *ToolError {
	err := NewToolError("Command", fmt.Sprintf("execute %s", command), cause)

	err = err.WithSuggestions(
		"Check if the command exists in PATH",
		"Verify command syntax and arguments",
		"Check system permissions",
	).WithRetryable(true).WithErrorCode("COMMAND_ERROR")

	return err
}
