package k8s

import (
	"context"
	_ "embed"
	"fmt"
	"maps"
	"math/rand"
	"net/http"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/tmc/langchaingo/llms"

	"github.com/kagent-dev/tools/internal/cache"
	"github.com/kagent-dev/tools/internal/commands"
	"github.com/kagent-dev/tools/internal/logger"
	mcp "github.com/kagent-dev/tools/internal/mcp"
	"github.com/kagent-dev/tools/internal/security"
)

// K8sTool struct to hold the LLM model
type K8sTool struct {
	kubeconfig       string
	llmModel         llms.Model
	tokenPassthrough bool // when true, require Bearer token and pass it to kubectl; when false, do not use token
}

func NewK8sTool(llmModel llms.Model) *K8sTool {
	return &K8sTool{llmModel: llmModel, tokenPassthrough: os.Getenv("TOKEN_PASSTHROUGH") == "true"}
}

func NewK8sToolWithConfig(kubeconfig string, llmModel llms.Model) *K8sTool {
	return &K8sTool{kubeconfig: kubeconfig, llmModel: llmModel, tokenPassthrough: os.Getenv("TOKEN_PASSTHROUGH") == "true"}
}

// runKubectlCommandWithCacheInvalidation runs a kubectl command and invalidates cache if it's a modification operation
func (k *K8sTool) runKubectlCommandWithCacheInvalidation(ctx context.Context, headers http.Header, args ...string) (*mcp.CallToolResult, error) {
	result, err := k.runKubectlCommand(ctx, headers, args...)

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

// getResourcesInput is the typed input for k8s_get_resources.
type getResourcesInput struct {
	ResourceType  string `json:"resource_type" jsonschema:"Type of resource (pod, service, deployment, etc.)"`
	ResourceName  string `json:"resource_name" jsonschema:"Name of specific resource (optional)"`
	Namespace     string `json:"namespace" jsonschema:"Namespace to query (optional)"`
	AllNamespaces bool   `json:"all_namespaces" jsonschema:"Query all namespaces"`
	Output        string `json:"output" jsonschema:"Output format (json, yaml, wide)"`
}

// Enhanced kubectl get
func (k *K8sTool) handleKubectlGetEnhanced(ctx context.Context, request *mcp.CallToolRequest, in getResourcesInput) (*mcp.CallToolResult, any, error) {
	if in.ResourceType == "" {
		return mcp.NewToolResultError("resource_type parameter is required"), nil, nil
	}
	if in.Output == "" {
		in.Output = "wide"
	}

	args := []string{"get", in.ResourceType}

	if in.ResourceName != "" {
		args = append(args, in.ResourceName)
	}

	if in.AllNamespaces {
		args = append(args, "--all-namespaces")
	} else if in.Namespace != "" {
		args = append(args, "-n", in.Namespace)
	}

	args = append(args, "-o", in.Output)

	res, err := k.runKubectlCommand(ctx, mcp.Header(request), args...)
	return res, nil, err
}

// logsInput is the typed input for k8s_get_pod_logs.
type logsInput struct {
	PodName   string `json:"pod_name" jsonschema:"Name of the pod"`
	Namespace string `json:"namespace" jsonschema:"Namespace of the pod (default: default)"`
	Container string `json:"container" jsonschema:"Container name (for multi-container pods)"`
	TailLines int    `json:"tail_lines" jsonschema:"Number of lines to show from the end (default: 50)"`
}

// Get pod logs
func (k *K8sTool) handleKubectlLogsEnhanced(ctx context.Context, request *mcp.CallToolRequest, in logsInput) (*mcp.CallToolResult, any, error) {
	if in.PodName == "" {
		return mcp.NewToolResultError("pod_name parameter is required"), nil, nil
	}
	if in.Namespace == "" {
		in.Namespace = "default"
	}
	if in.TailLines == 0 {
		in.TailLines = 50
	}

	args := []string{"logs", in.PodName, "-n", in.Namespace}

	if in.Container != "" {
		args = append(args, "-c", in.Container)
	}

	if in.TailLines > 0 {
		args = append(args, "--tail", fmt.Sprintf("%d", in.TailLines))
	}

	res, err := k.runKubectlCommand(ctx, mcp.Header(request), args...)
	return res, nil, err
}

// scaleInput is the typed input for k8s_scale.
type scaleInput struct {
	Name      string `json:"name" jsonschema:"Name of the deployment"`
	Namespace string `json:"namespace" jsonschema:"Namespace of the deployment (default: default)"`
	Replicas  int    `json:"replicas" jsonschema:"Number of replicas"`
}

// Scale deployment
func (k *K8sTool) handleScaleDeployment(ctx context.Context, request *mcp.CallToolRequest, in scaleInput) (*mcp.CallToolResult, any, error) {
	if in.Name == "" {
		return mcp.NewToolResultError("name parameter is required"), nil, nil
	}
	if in.Namespace == "" {
		in.Namespace = "default"
	}
	if in.Replicas == 0 {
		in.Replicas = 1
	}

	args := []string{"scale", "deployment", in.Name, "--replicas", fmt.Sprintf("%d", in.Replicas), "-n", in.Namespace}

	res, err := k.runKubectlCommandWithCacheInvalidation(ctx, mcp.Header(request), args...)
	return res, nil, err
}

// patchResourceInput is the typed input for k8s_patch_resource.
type patchResourceInput struct {
	ResourceType string `json:"resource_type" jsonschema:"Type of resource (deployment, service, etc.)"`
	ResourceName string `json:"resource_name" jsonschema:"Name of the resource"`
	Patch        string `json:"patch" jsonschema:"JSON patch to apply"`
	PatchType    string `json:"patch_type" jsonschema:"Patch strategy: \"strategic\" (default; built-in Kubernetes types only), \"merge\" (RFC 7386 JSON merge patch; required for CustomResources/CRDs), or \"json\" (RFC 6902 JSON patch)."`
	Namespace    string `json:"namespace" jsonschema:"Namespace of the resource (default: default)"`
}

// Patch resource
func (k *K8sTool) handlePatchResource(ctx context.Context, request *mcp.CallToolRequest, in patchResourceInput) (*mcp.CallToolResult, any, error) {
	if in.Namespace == "" {
		in.Namespace = "default"
	}
	if in.PatchType == "" {
		in.PatchType = "strategic"
	}

	if in.ResourceType == "" || in.ResourceName == "" || in.Patch == "" {
		return mcp.NewToolResultError("resource_type, resource_name, and patch parameters are required"), nil, nil
	}

	// Validate patch type. "strategic" is only implemented for built-in Kubernetes
	// types; CustomResources (CRDs) reject it and require "merge" or "json".
	switch in.PatchType {
	case "strategic", "merge", "json":
	default:
		return mcp.NewToolResultError(fmt.Sprintf("Invalid patch_type %q: must be one of strategic, merge, json", in.PatchType)), nil, nil
	}

	if err := security.ValidateK8sResourceName(in.ResourceName); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Invalid resource name: %v", err)), nil, nil
	}

	if err := security.ValidateNamespace(in.Namespace); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Invalid namespace: %v", err)), nil, nil
	}

	if err := security.ValidateYAMLContent(in.Patch); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Invalid patch content: %v", err)), nil, nil
	}

	args := []string{"patch", in.ResourceType, in.ResourceName, "--type=" + in.PatchType, "-p", in.Patch, "-n", in.Namespace}

	res, err := k.runKubectlCommandWithCacheInvalidation(ctx, mcp.Header(request), args...)
	return res, nil, err
}

// patchStatusInput is the typed input for k8s_patch_status.
type patchStatusInput struct {
	ResourceType string `json:"resource_type" jsonschema:"Type of resource (deployment, service, etc.)"`
	ResourceName string `json:"resource_name" jsonschema:"Name of the resource"`
	Patch        string `json:"patch" jsonschema:"JSON/YAML status patch"`
	Namespace    string `json:"namespace" jsonschema:"Namespace of the resource (default: default)"`
}

// Patch resource status
func (k *K8sTool) handlePatchStatus(ctx context.Context, request *mcp.CallToolRequest, in patchStatusInput) (*mcp.CallToolResult, any, error) {
	if in.Namespace == "" {
		in.Namespace = "default"
	}

	if in.ResourceType == "" || in.ResourceName == "" || in.Patch == "" {
		return mcp.NewToolResultError("resource_type, resource_name, and patch parameters are required"), nil, nil
	}

	if err := security.ValidateK8sResourceName(in.ResourceName); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Invalid resource name: %v", err)), nil, nil
	}

	if err := security.ValidateNamespace(in.Namespace); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Invalid namespace: %v", err)), nil, nil
	}

	if err := security.ValidateYAMLContent(in.Patch); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Invalid patch content: %v", err)), nil, nil
	}

	args := []string{
		"patch",
		in.ResourceType,
		in.ResourceName,
		"--subresource=status",
		"--type=merge",
		"-p",
		in.Patch,
		"-n",
		in.Namespace,
	}

	res, err := k.runKubectlCommandWithCacheInvalidation(ctx, mcp.Header(request), args...)
	return res, nil, err
}

// applyManifestInput is the typed input for k8s_apply_manifest.
type applyManifestInput struct {
	Manifest string `json:"manifest" jsonschema:"YAML manifest content"`
}

// Apply manifest from content
func (k *K8sTool) handleApplyManifest(ctx context.Context, request *mcp.CallToolRequest, in applyManifestInput) (*mcp.CallToolResult, any, error) {
	if in.Manifest == "" {
		return mcp.NewToolResultError("manifest parameter is required"), nil, nil
	}

	if err := security.ValidateYAMLContent(in.Manifest); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Invalid manifest content: %v", err)), nil, nil
	}

	tmpFile, err := os.CreateTemp("", "k8s-manifest-*.yaml")
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to create temp file: %v", err)), nil, nil
	}

	defer func() {
		if removeErr := os.Remove(tmpFile.Name()); removeErr != nil {
			logger.Get().Error("Failed to remove temporary file", "error", removeErr, "file", tmpFile.Name())
		}
	}()

	if err := os.Chmod(tmpFile.Name(), 0600); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to set file permissions: %v", err)), nil, nil
	}

	if _, err := tmpFile.WriteString(in.Manifest); err != nil {
		tmpFile.Close()
		return mcp.NewToolResultError(fmt.Sprintf("Failed to write to temp file: %v", err)), nil, nil
	}

	if err := tmpFile.Close(); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to close temp file: %v", err)), nil, nil
	}

	res, err := k.runKubectlCommandWithCacheInvalidation(ctx, mcp.Header(request), "apply", "-f", tmpFile.Name())
	return res, nil, err
}

// deleteResourceInput is the typed input for k8s_delete_resource.
type deleteResourceInput struct {
	ResourceType string `json:"resource_type" jsonschema:"Type of resource (pod, service, deployment, etc.)"`
	ResourceName string `json:"resource_name" jsonschema:"Name of the resource"`
	Namespace    string `json:"namespace" jsonschema:"Namespace of the resource (default: default)"`
}

// Delete resource
func (k *K8sTool) handleDeleteResource(ctx context.Context, request *mcp.CallToolRequest, in deleteResourceInput) (*mcp.CallToolResult, any, error) {
	if in.Namespace == "" {
		in.Namespace = "default"
	}

	if in.ResourceType == "" || in.ResourceName == "" {
		return mcp.NewToolResultError("resource_type and resource_name parameters are required"), nil, nil
	}

	args := []string{"delete", in.ResourceType, in.ResourceName, "-n", in.Namespace}

	res, err := k.runKubectlCommandWithCacheInvalidation(ctx, mcp.Header(request), args...)
	return res, nil, err
}

// waitInput is the typed input for k8s_wait.
type waitInput struct {
	ResourceType string `json:"resource_type" jsonschema:"Type of resource (pod, deployment, job, etc.)"`
	Condition    string `json:"condition" jsonschema:"Condition to wait for, passed to --for. Examples: 'condition=Ready', 'condition=Available', 'delete', 'create', \"jsonpath={.status.phase}=Running\""`
	ResourceName string `json:"resource_name" jsonschema:"Name of a specific resource. Omit to target by selector or all"`
	Selector     string `json:"selector" jsonschema:"Label selector to target resources, e.g. 'app=nginx'"`
	All          bool   `json:"all" jsonschema:"Wait on all resources of the type in the namespace"`
	Namespace    string `json:"namespace" jsonschema:"Namespace of the resource (default: default)"`
	Timeout      string `json:"timeout" jsonschema:"Max wait duration, e.g. '30s', '5m'. 0 waits forever (default: 30s)"`
}

// Wait for a condition on one or more resources (kubectl wait)
func (k *K8sTool) handleKubectlWait(ctx context.Context, request *mcp.CallToolRequest, in waitInput) (*mcp.CallToolResult, any, error) {
	if in.Namespace == "" {
		in.Namespace = "default"
	}
	if in.Timeout == "" {
		in.Timeout = "30s"
	}

	if in.ResourceType == "" || in.Condition == "" {
		return mcp.NewToolResultError("resource_type and condition parameters are required"), nil, nil
	}
	if in.ResourceName == "" && in.Selector == "" && !in.All {
		return mcp.NewToolResultError("one of resource_name, selector, or all=true must be provided"), nil, nil
	}

	if err := security.ValidateNamespace(in.Namespace); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Invalid namespace: %v", err)), nil, nil
	}

	target := in.ResourceType
	if in.ResourceName != "" {
		if err := security.ValidateK8sResourceName(in.ResourceName); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Invalid resource name: %v", err)), nil, nil
		}
		target = fmt.Sprintf("%s/%s", in.ResourceType, in.ResourceName)
	}

	args := []string{"wait", target, "--for=" + in.Condition, "--timeout", in.Timeout, "-n", in.Namespace}
	if in.Selector != "" {
		args = append(args, "-l", in.Selector)
	}
	if in.All {
		args = append(args, "--all")
	}

	res, err := k.runKubectlCommand(ctx, mcp.Header(request), args...)
	return res, nil, err
}

// serviceConnectivityInput is the typed input for k8s_check_service_connectivity.
type serviceConnectivityInput struct {
	ServiceName string `json:"service_name" jsonschema:"Service name to test (e.g., my-service.my-namespace.svc.cluster.local:80)"`
	Namespace   string `json:"namespace" jsonschema:"Namespace to run the check from (default: default)"`
}

// Check service connectivity
func (k *K8sTool) handleCheckServiceConnectivity(ctx context.Context, request *mcp.CallToolRequest, in serviceConnectivityInput) (*mcp.CallToolResult, any, error) {
	if in.Namespace == "" {
		in.Namespace = "default"
	}
	if in.ServiceName == "" {
		return mcp.NewToolResultError("service_name parameter is required"), nil, nil
	}

	headers := mcp.Header(request)

	// Create a temporary curl pod for connectivity check
	podName := fmt.Sprintf("curl-test-%d", rand.Intn(10000))
	defer func() {
		_, _ = k.runKubectlCommand(ctx, headers, "delete", "pod", podName, "-n", in.Namespace, "--ignore-not-found")
	}()

	// Create the curl pod
	_, err := k.runKubectlCommand(ctx, headers, "run", podName, "--image=curlimages/curl", "-n", in.Namespace, "--restart=Never", "--", "sleep", "3600")
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to create curl pod: %v", err)), nil, nil
	}

	// Wait for pod to be ready
	_, err = k.runKubectlCommandWithTimeout(ctx, headers, 60*time.Second, "wait", "--for=condition=ready", "pod/"+podName, "-n", in.Namespace)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to wait for curl pod: %v", err)), nil, nil
	}

	// Execute kubectl command
	res, err := k.runKubectlCommand(ctx, headers, "exec", podName, "-n", in.Namespace, "--", "curl", "-s", in.ServiceName)
	return res, nil, err
}

// eventsInput is the typed input for k8s_get_events.
type eventsInput struct {
	Namespace string `json:"namespace" jsonschema:"Namespace to get events from (default: default)"`
}

// Get cluster events
func (k *K8sTool) handleGetEvents(ctx context.Context, request *mcp.CallToolRequest, in eventsInput) (*mcp.CallToolResult, any, error) {
	args := []string{"get", "events", "-o", "json"}
	if in.Namespace != "" {
		args = append(args, "-n", in.Namespace)
	} else {
		args = append(args, "--all-namespaces")
	}

	res, err := k.runKubectlCommand(ctx, mcp.Header(request), args...)
	return res, nil, err
}

// execCommandInput is the typed input for k8s_execute_command.
type execCommandInput struct {
	PodName   string `json:"pod_name" jsonschema:"Name of the pod to execute in"`
	Namespace string `json:"namespace" jsonschema:"Namespace of the pod (default: default)"`
	Container string `json:"container" jsonschema:"Container name (for multi-container pods)"`
	Command   string `json:"command" jsonschema:"Command to execute"`
}

// Execute command in pod
func (k *K8sTool) handleExecCommand(ctx context.Context, request *mcp.CallToolRequest, in execCommandInput) (*mcp.CallToolResult, any, error) {
	if in.Namespace == "" {
		in.Namespace = "default"
	}
	if in.PodName == "" || in.Command == "" {
		return mcp.NewToolResultError("pod_name and command parameters are required"), nil, nil
	}

	if err := security.ValidateK8sResourceName(in.PodName); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Invalid pod name: %v", err)), nil, nil
	}

	if err := security.ValidateNamespace(in.Namespace); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Invalid namespace: %v", err)), nil, nil
	}

	if err := security.ValidateCommandInput(in.Command); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Invalid command: %v", err)), nil, nil
	}

	args := []string{"exec", in.PodName, "-n", in.Namespace, "--", in.Command}

	res, err := k.runKubectlCommand(ctx, mcp.Header(request), args...)
	return res, nil, err
}

// noInput is the typed input for tools that take no arguments.
type noInput struct{}

// Get available API resources
func (k *K8sTool) handleGetAvailableAPIResources(ctx context.Context, request *mcp.CallToolRequest, _ noInput) (*mcp.CallToolResult, any, error) {
	res, err := k.runKubectlCommand(ctx, mcp.Header(request), "api-resources")
	return res, nil, err
}

// describeInput is the typed input for k8s_describe_resource.
type describeInput struct {
	ResourceType string `json:"resource_type" jsonschema:"Type of resource (deployment, service, pod, node, etc.)"`
	ResourceName string `json:"resource_name" jsonschema:"Name of the resource"`
	Namespace    string `json:"namespace" jsonschema:"Namespace of the resource (optional)"`
}

// Kubectl describe tool
func (k *K8sTool) handleKubectlDescribeTool(ctx context.Context, request *mcp.CallToolRequest, in describeInput) (*mcp.CallToolResult, any, error) {
	if in.ResourceType == "" || in.ResourceName == "" {
		return mcp.NewToolResultError("resource_type and resource_name parameters are required"), nil, nil
	}

	args := []string{"describe", in.ResourceType, in.ResourceName}
	if in.Namespace != "" {
		args = append(args, "-n", in.Namespace)
	}

	res, err := k.runKubectlCommand(ctx, mcp.Header(request), args...)
	return res, nil, err
}

// rolloutInput is the typed input for k8s_rollout.
type rolloutInput struct {
	Action       string `json:"action" jsonschema:"The rollout action to perform"`
	ResourceType string `json:"resource_type" jsonschema:"The type of resource to rollout (e.g., deployment)"`
	ResourceName string `json:"resource_name" jsonschema:"The name of the resource to rollout"`
	Namespace    string `json:"namespace" jsonschema:"The namespace of the resource"`
}

// Rollout operations
func (k *K8sTool) handleRollout(ctx context.Context, request *mcp.CallToolRequest, in rolloutInput) (*mcp.CallToolResult, any, error) {
	if in.Action == "" || in.ResourceType == "" || in.ResourceName == "" {
		return mcp.NewToolResultError("action, resource_type, and resource_name parameters are required"), nil, nil
	}

	args := []string{"rollout", in.Action, fmt.Sprintf("%s/%s", in.ResourceType, in.ResourceName)}
	if in.Namespace != "" {
		args = append(args, "-n", in.Namespace)
	}

	res, err := k.runKubectlCommand(ctx, mcp.Header(request), args...)
	return res, nil, err
}

// Get cluster configuration
func (k *K8sTool) handleGetClusterConfiguration(ctx context.Context, request *mcp.CallToolRequest, _ noInput) (*mcp.CallToolResult, any, error) {
	res, err := k.runKubectlCommand(ctx, mcp.Header(request), "config", "view", "-o", "json")
	return res, nil, err
}

// removeAnnotationInput is the typed input for k8s_remove_annotation.
type removeAnnotationInput struct {
	ResourceType  string `json:"resource_type" jsonschema:"The type of resource"`
	ResourceName  string `json:"resource_name" jsonschema:"The name of the resource"`
	AnnotationKey string `json:"annotation_key" jsonschema:"The key of the annotation to remove"`
	Namespace     string `json:"namespace" jsonschema:"The namespace of the resource"`
}

// Remove annotation
func (k *K8sTool) handleRemoveAnnotation(ctx context.Context, request *mcp.CallToolRequest, in removeAnnotationInput) (*mcp.CallToolResult, any, error) {
	if in.ResourceType == "" || in.ResourceName == "" || in.AnnotationKey == "" {
		return mcp.NewToolResultError("resource_type, resource_name, and annotation_key parameters are required"), nil, nil
	}

	args := []string{"annotate", in.ResourceType, in.ResourceName, in.AnnotationKey + "-"}
	if in.Namespace != "" {
		args = append(args, "-n", in.Namespace)
	}

	res, err := k.runKubectlCommand(ctx, mcp.Header(request), args...)
	return res, nil, err
}

// removeLabelInput is the typed input for k8s_remove_label.
type removeLabelInput struct {
	ResourceType string `json:"resource_type" jsonschema:"The type of resource"`
	ResourceName string `json:"resource_name" jsonschema:"The name of the resource"`
	LabelKey     string `json:"label_key" jsonschema:"The key of the label to remove"`
	Namespace    string `json:"namespace" jsonschema:"The namespace of the resource"`
}

// Remove label
func (k *K8sTool) handleRemoveLabel(ctx context.Context, request *mcp.CallToolRequest, in removeLabelInput) (*mcp.CallToolResult, any, error) {
	if in.ResourceType == "" || in.ResourceName == "" || in.LabelKey == "" {
		return mcp.NewToolResultError("resource_type, resource_name, and label_key parameters are required"), nil, nil
	}

	args := []string{"label", in.ResourceType, in.ResourceName, in.LabelKey + "-"}
	if in.Namespace != "" {
		args = append(args, "-n", in.Namespace)
	}

	res, err := k.runKubectlCommand(ctx, mcp.Header(request), args...)
	return res, nil, err
}

// annotateInput is the typed input for k8s_annotate_resource.
type annotateInput struct {
	ResourceType string `json:"resource_type" jsonschema:"The type of resource"`
	ResourceName string `json:"resource_name" jsonschema:"The name of the resource"`
	Annotations  string `json:"annotations" jsonschema:"Space-separated key=value pairs for annotations"`
	Namespace    string `json:"namespace" jsonschema:"The namespace of the resource"`
}

// Annotate resource
func (k *K8sTool) handleAnnotateResource(ctx context.Context, request *mcp.CallToolRequest, in annotateInput) (*mcp.CallToolResult, any, error) {
	if in.ResourceType == "" || in.ResourceName == "" || in.Annotations == "" {
		return mcp.NewToolResultError("resource_type, resource_name, and annotations parameters are required"), nil, nil
	}

	args := []string{"annotate", in.ResourceType, in.ResourceName}
	args = append(args, strings.Fields(in.Annotations)...)

	if in.Namespace != "" {
		args = append(args, "-n", in.Namespace)
	}

	res, err := k.runKubectlCommand(ctx, mcp.Header(request), args...)
	return res, nil, err
}

// labelInput is the typed input for k8s_label_resource.
type labelInput struct {
	ResourceType string `json:"resource_type" jsonschema:"The type of resource"`
	ResourceName string `json:"resource_name" jsonschema:"The name of the resource"`
	Labels       string `json:"labels" jsonschema:"Space-separated key=value pairs for labels"`
	Namespace    string `json:"namespace" jsonschema:"The namespace of the resource"`
}

// Label resource
func (k *K8sTool) handleLabelResource(ctx context.Context, request *mcp.CallToolRequest, in labelInput) (*mcp.CallToolResult, any, error) {
	if in.ResourceType == "" || in.ResourceName == "" || in.Labels == "" {
		return mcp.NewToolResultError("resource_type, resource_name, and labels parameters are required"), nil, nil
	}

	args := []string{"label", in.ResourceType, in.ResourceName}
	args = append(args, strings.Fields(in.Labels)...)

	if in.Namespace != "" {
		args = append(args, "-n", in.Namespace)
	}

	res, err := k.runKubectlCommand(ctx, mcp.Header(request), args...)
	return res, nil, err
}

// createFromURLInput is the typed input for k8s_create_resource_from_url.
type createFromURLInput struct {
	URL       string `json:"url" jsonschema:"The URL of the manifest"`
	Namespace string `json:"namespace" jsonschema:"The namespace to create the resource in"`
}

// Create resource from URL
func (k *K8sTool) handleCreateResourceFromURL(ctx context.Context, request *mcp.CallToolRequest, in createFromURLInput) (*mcp.CallToolResult, any, error) {
	if in.URL == "" {
		return mcp.NewToolResultError("url parameter is required"), nil, nil
	}

	args := []string{"create", "-f", in.URL}
	if in.Namespace != "" {
		args = append(args, "-n", in.Namespace)
	}

	res, err := k.runKubectlCommand(ctx, mcp.Header(request), args...)
	return res, nil, err
}

// getResourceYAMLInput is the typed input for k8s_get_resource_yaml.
type getResourceYAMLInput struct {
	ResourceType string `json:"resource_type" jsonschema:"Type of resource"`
	ResourceName string `json:"resource_name" jsonschema:"Name of the resource"`
	Namespace    string `json:"namespace" jsonschema:"Namespace of the resource (optional)"`
}

// Get resource YAML
func (k *K8sTool) handleGetResourceYAML(ctx context.Context, request *mcp.CallToolRequest, in getResourceYAMLInput) (*mcp.CallToolResult, any, error) {
	if in.ResourceType == "" || in.ResourceName == "" {
		return mcp.NewToolResultError("resource_type and resource_name are required"), nil, nil
	}

	args := []string{"get", in.ResourceType, in.ResourceName, "-o", "yaml"}
	if in.Namespace != "" {
		args = append(args, "-n", in.Namespace)
	}

	res, err := k.runKubectlCommand(ctx, mcp.Header(request), args...)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Get YAML command failed: %v", err)), nil, nil
	}
	return res, nil, nil
}

// createResourceInput is the typed input for k8s_create_resource.
type createResourceInput struct {
	YAMLContent string `json:"yaml_content" jsonschema:"YAML content of the resource"`
}

// Create resource from YAML content
func (k *K8sTool) handleCreateResource(ctx context.Context, request *mcp.CallToolRequest, in createResourceInput) (*mcp.CallToolResult, any, error) {
	if in.YAMLContent == "" {
		return mcp.NewToolResultError("yaml_content is required"), nil, nil
	}

	tmpFile, err := os.CreateTemp("", "k8s-resource-*.yaml")
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to create temp file: %v", err)), nil, nil
	}
	defer os.Remove(tmpFile.Name())

	if _, err := tmpFile.WriteString(in.YAMLContent); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to write to temp file: %v", err)), nil, nil
	}
	tmpFile.Close()

	res, err := k.runKubectlCommand(ctx, mcp.Header(request), "create", "-f", tmpFile.Name())
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Create command failed: %v", err)), nil, nil
	}
	return res, nil, nil
}

// Resource generation embeddings
var (
	//go:embed resources/istio/peer_auth.md
	istioAuthPolicy string

	//go:embed resources/istio/virtual_service.md
	istioVirtualService string

	//go:embed resources/gw_api/reference_grant.md
	gatewayApiReferenceGrant string

	//go:embed resources/gw_api/gateway.md
	gatewayApiGateway string

	//go:embed resources/gw_api/http_route.md
	gatewayApiHttpRoute string

	//go:embed resources/gw_api/gateway_class.md
	gatewayApiGatewayClass string

	//go:embed resources/gw_api/grpc_route.md
	gatewayApiGrpcRoute string

	//go:embed resources/argo/rollout.md
	argoRollout string

	//go:embed resources/argo/analysis_template.md
	argoAnalaysisTempalte string

	resourceMap = map[string]string{
		"istio_auth_policy":           istioAuthPolicy,
		"istio_virtual_service":       istioVirtualService,
		"gateway_api_reference_grant": gatewayApiReferenceGrant,
		"gateway_api_gateway":         gatewayApiGateway,
		"gateway_api_http_route":      gatewayApiHttpRoute,
		"gateway_api_gateway_class":   gatewayApiGatewayClass,
		"gateway_api_grpc_route":      gatewayApiGrpcRoute,
		"argo_rollout":                argoRollout,
		"argo_analysis_template":      argoAnalaysisTempalte,
	}

	resourceTypes = maps.Keys(resourceMap)
)

// generateResourceInput is the typed input for k8s_generate_resource.
type generateResourceInput struct {
	ResourceDescription string `json:"resource_description" jsonschema:"Detailed description of the resource to generate"`
	ResourceType        string `json:"resource_type" jsonschema:"Type of resource to generate"`
}

// Generate resource using LLM
func (k *K8sTool) handleGenerateResource(ctx context.Context, request *mcp.CallToolRequest, in generateResourceInput) (*mcp.CallToolResult, any, error) {
	if in.ResourceType == "" || in.ResourceDescription == "" {
		return mcp.NewToolResultError("resource_type and resource_description parameters are required"), nil, nil
	}

	systemPrompt, ok := resourceMap[in.ResourceType]
	if !ok {
		return mcp.NewToolResultError(fmt.Sprintf("resource type %s not found", in.ResourceType)), nil, nil
	}

	if k.llmModel == nil {
		return mcp.NewToolResultError("No LLM client present, can't generate resource"), nil, nil
	}
	llm := k.llmModel

	contents := []llms.MessageContent{
		{
			Role: llms.ChatMessageTypeSystem,
			Parts: []llms.ContentPart{
				llms.TextContent{Text: systemPrompt},
			},
		},
		{
			Role: llms.ChatMessageTypeHuman,
			Parts: []llms.ContentPart{
				llms.TextContent{Text: in.ResourceDescription},
			},
		},
	}

	resp, err := llm.GenerateContent(ctx, contents, llms.WithModel("gpt-4o-mini"))
	if err != nil {
		return mcp.NewToolResultError("failed to generate content: " + err.Error()), nil, nil
	}

	choices := resp.Choices
	if len(choices) < 1 {
		return mcp.NewToolResultError("empty response from model"), nil, nil
	}
	responseText := choices[0].Content

	return mcp.NewToolResultText(responseText), nil, nil
}

// extractBearerToken extracts the Bearer token from the Authorization header
func extractBearerToken(headers http.Header) string {
	if auth := headers.Get("Authorization"); auth != "" {
		if strings.HasPrefix(auth, "Bearer ") {
			return strings.TrimPrefix(auth, "Bearer ")
		}
	}
	return ""
}

// tokenForKubectl returns the token to pass to kubectl and an error if passthrough is true but token is missing.
func (k *K8sTool) tokenForKubectl(headers http.Header) (string, error) {
	token := extractBearerToken(headers)
	if k.tokenPassthrough && token == "" {
		return "", fmt.Errorf("Bearer token required when TOKEN_PASSTHROUGH is true")
	}
	if k.tokenPassthrough {
		return token, nil
	}
	return "", nil // do not use token when passthrough is false
}

// runKubectlCommand is a helper function to execute kubectl commands
func (k *K8sTool) runKubectlCommand(ctx context.Context, headers http.Header, args ...string) (*mcp.CallToolResult, error) {
	token, err := k.tokenForKubectl(headers)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	builder := commands.NewCommandBuilder("kubectl").
		WithArgs(args...).
		WithKubeconfig(k.kubeconfig)
	if token != "" {
		builder = builder.WithToken(token)
	}
	output, err := builder.Execute(ctx)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	return mcp.NewToolResultText(output), nil
}

// runKubectlCommandWithTimeout is a helper function to execute kubectl commands with a timeout
func (k *K8sTool) runKubectlCommandWithTimeout(ctx context.Context, headers http.Header, timeout time.Duration, args ...string) (*mcp.CallToolResult, error) {
	token, err := k.tokenForKubectl(headers)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	builder := commands.NewCommandBuilder("kubectl").
		WithArgs(args...).
		WithKubeconfig(k.kubeconfig).
		WithTimeout(timeout)
	if token != "" {
		builder = builder.WithToken(token)
	}
	output, err := builder.Execute(ctx)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	return mcp.NewToolResultText(output), nil
}

// RegisterTools registers all k8s tools with the MCP server
func RegisterTools(s *mcp.Server, llm llms.Model, kubeconfig string, readOnly bool) {
	k8sTool := NewK8sToolWithConfig(kubeconfig, llm)

	// Read-only tools - always registered
	mcp.AddTool(s, "k8s", &mcp.Tool{
		Name:        "k8s_get_resources",
		Description: "Get Kubernetes resources using kubectl",
	}, k8sTool.handleKubectlGetEnhanced)

	mcp.AddTool(s, "k8s", &mcp.Tool{
		Name:        "k8s_get_pod_logs",
		Description: "Get logs from a Kubernetes pod",
	}, k8sTool.handleKubectlLogsEnhanced)

	mcp.AddTool(s, "k8s", &mcp.Tool{
		Name:        "k8s_get_events",
		Description: "Get events from a Kubernetes namespace",
	}, k8sTool.handleGetEvents)

	mcp.AddTool(s, "k8s", &mcp.Tool{
		Name:        "k8s_get_available_api_resources",
		Description: "Get available Kubernetes API resources",
	}, k8sTool.handleGetAvailableAPIResources)

	mcp.AddTool(s, "k8s", &mcp.Tool{
		Name:        "k8s_get_cluster_configuration",
		Description: "Get cluster configuration details",
	}, k8sTool.handleGetClusterConfiguration)

	mcp.AddTool(s, "k8s", &mcp.Tool{
		Name:        "k8s_get_resource_yaml",
		Description: "Get the YAML representation of a Kubernetes resource",
	}, k8sTool.handleGetResourceYAML)

	mcp.AddTool(s, "k8s", &mcp.Tool{
		Name:        "k8s_describe_resource",
		Description: "Describe a Kubernetes resource in detail",
	}, k8sTool.handleKubectlDescribeTool)

	mcp.AddTool(s, "k8s", &mcp.Tool{
		Name:        "k8s_wait",
		Description: "Wait for a condition on Kubernetes resources (kubectl wait). Blocks until the condition is met or the timeout elapses.",
	}, k8sTool.handleKubectlWait)

	mcp.AddTool(s, "k8s", &mcp.Tool{
		Name:        "k8s_generate_resource",
		Description: fmt.Sprintf("Generate a Kubernetes resource YAML from a description. Supported resource_type values: %s", strings.Join(slices.Collect(resourceTypes), ", ")),
	}, k8sTool.handleGenerateResource)

	// Write tools - only registered when write operations are enabled
	if !readOnly {
		mcp.AddTool(s, "k8s", &mcp.Tool{
			Name:        "k8s_scale",
			Description: "Scale a Kubernetes deployment",
		}, k8sTool.handleScaleDeployment)

		mcp.AddTool(s, "k8s", &mcp.Tool{
			Name:        "k8s_patch_resource",
			Description: "Patch a Kubernetes resource. Defaults to a strategic merge patch, which is only supported for built-in types; set patch_type to \"merge\" (or \"json\") to patch a CustomResource/CRD.",
		}, k8sTool.handlePatchResource)

		mcp.AddTool(s, "k8s", &mcp.Tool{
			Name:        "k8s_patch_status",
			Description: "Patch the status of a Kubernetes resource",
		}, k8sTool.handlePatchStatus)

		mcp.AddTool(s, "k8s", &mcp.Tool{
			Name:        "k8s_apply_manifest",
			Description: "Apply a YAML manifest to the Kubernetes cluster",
		}, k8sTool.handleApplyManifest)

		mcp.AddTool(s, "k8s", &mcp.Tool{
			Name:        "k8s_delete_resource",
			Description: "Delete a Kubernetes resource",
		}, k8sTool.handleDeleteResource)

		mcp.AddTool(s, "k8s", &mcp.Tool{
			Name:        "k8s_check_service_connectivity",
			Description: "Check connectivity to a service using a temporary curl pod",
		}, k8sTool.handleCheckServiceConnectivity)

		mcp.AddTool(s, "k8s", &mcp.Tool{
			Name:        "k8s_execute_command",
			Description: "Execute a command in a Kubernetes pod",
		}, k8sTool.handleExecCommand)

		mcp.AddTool(s, "k8s", &mcp.Tool{
			Name:        "k8s_rollout",
			Description: "Perform rollout operations on Kubernetes resources (history, pause, restart, resume, status, undo)",
		}, k8sTool.handleRollout)

		mcp.AddTool(s, "k8s", &mcp.Tool{
			Name:        "k8s_label_resource",
			Description: "Add or update labels on a Kubernetes resource",
		}, k8sTool.handleLabelResource)

		mcp.AddTool(s, "k8s", &mcp.Tool{
			Name:        "k8s_annotate_resource",
			Description: "Add or update annotations on a Kubernetes resource",
		}, k8sTool.handleAnnotateResource)

		mcp.AddTool(s, "k8s", &mcp.Tool{
			Name:        "k8s_remove_annotation",
			Description: "Remove an annotation from a Kubernetes resource",
		}, k8sTool.handleRemoveAnnotation)

		mcp.AddTool(s, "k8s", &mcp.Tool{
			Name:        "k8s_remove_label",
			Description: "Remove a label from a Kubernetes resource",
		}, k8sTool.handleRemoveLabel)

		mcp.AddTool(s, "k8s", &mcp.Tool{
			Name:        "k8s_create_resource",
			Description: "Create a Kubernetes resource from YAML content",
		}, k8sTool.handleCreateResource)

		mcp.AddTool(s, "k8s", &mcp.Tool{
			Name:        "k8s_create_resource_from_url",
			Description: "Create a Kubernetes resource from a URL pointing to a YAML manifest",
		}, k8sTool.handleCreateResourceFromURL)
	}
}
