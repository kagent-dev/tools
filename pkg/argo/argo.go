// Package argo provides Argo Rollouts and ArgoCD operations.
//
// This package implements MCP tools for Argo, providing operations such as:
//   - Argo Rollouts analysis and promotion
//   - ArgoCD application management
//   - Rollout status tracking and management
//   - Gateway plugin operations
//
// All tools require Argo Rollouts and/or ArgoCD to be properly installed.
// Tools support analysis runs, automatic promotions, and rollback operations.
//
// Example usage:
//
//	server := mcp.NewServer(...)
//	err := RegisterTools(server)
package argo

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/kagent-dev/tools/internal/commands"
	"github.com/kagent-dev/tools/internal/logger"
	"github.com/kagent-dev/tools/pkg/utils"
)

// getArgoCDClient gets or creates an ArgoCD client instance
var getArgoCDClient = func() (*ArgoCDClient, error) {
	return GetArgoCDClientFromEnv()
}

// isReadOnlyMode checks if the server is in read-only mode
func isReadOnlyMode() bool {
	return strings.ToLower(strings.TrimSpace(os.Getenv("MCP_READ_ONLY"))) == "true"
}

// returnJSONResult returns a JSON result as text content
func returnJSONResult(data interface{}) (*mcp.CallToolResult, error) {
	jsonData, err := json.Marshal(data)
	if err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("failed to marshal result: %v", err)}},
			IsError: true,
		}, nil
	}
	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: string(jsonData)}},
	}, nil
}

// returnErrorResult returns an error result
func returnErrorResult(message string) (*mcp.CallToolResult, error) {
	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: message}},
		IsError: true,
	}, nil
}

// Argo Rollouts tools

func handleVerifyArgoRolloutsControllerInstall(ctx context.Context, request *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var args map[string]interface{}
	if err := json.Unmarshal(request.Params.Arguments, &args); err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "failed to parse arguments"}},
			IsError: true,
		}, nil
	}

	ns := "argo-rollouts"
	if namespace, ok := args["namespace"].(string); ok && namespace != "" {
		ns = namespace
	}

	label := "app.kubernetes.io/component=rollouts-controller"
	if labelArg, ok := args["label"].(string); ok && labelArg != "" {
		label = labelArg
	}

	cmd := []string{"get", "pods", "-n", ns, "-l", label, "-o", "jsonpath={.items[*].status.phase}"}
	output, err := runArgoRolloutCommand(ctx, cmd)
	if err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "Error: " + err.Error()}},
			IsError: true,
		}, nil
	}

	output = strings.TrimSpace(output)
	if output == "" {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "Error: No pods found"}},
		}, nil
	}

	if strings.HasPrefix(output, "Error") {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: output}},
		}, nil
	}

	podStatuses := strings.Fields(output)
	if len(podStatuses) == 0 {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "Error: No pod statuses returned"}},
		}, nil
	}

	allRunning := true
	for _, status := range podStatuses {
		if status != "Running" {
			allRunning = false
			break
		}
	}

	if allRunning {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "All pods are running"}},
		}, nil
	} else {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "Error: Not all pods are running (" + strings.Join(podStatuses, " ") + ")"}},
		}, nil
	}
}

func handleVerifyKubectlPluginInstall(ctx context.Context, request *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := []string{"argo", "rollouts", "version"}
	output, err := runArgoRolloutCommand(ctx, args)
	if err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "Kubectl Argo Rollouts plugin is not installed: " + err.Error()}},
		}, nil
	}

	if strings.HasPrefix(output, "Error") {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "Kubectl Argo Rollouts plugin is not installed: " + output}},
		}, nil
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: output}},
	}, nil
}

func runArgoRolloutCommand(ctx context.Context, args []string) (string, error) {
	kubeconfigPath := utils.GetKubeconfig()
	return commands.NewCommandBuilder("kubectl").
		WithArgs(args...).
		WithKubeconfig(kubeconfigPath).
		Execute(ctx)
}

func handlePromoteRollout(ctx context.Context, request *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var args map[string]interface{}
	if err := json.Unmarshal(request.Params.Arguments, &args); err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "failed to parse arguments"}},
			IsError: true,
		}, nil
	}

	rolloutName, ok := args["rollout_name"].(string)
	if !ok || rolloutName == "" {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "rollout_name parameter is required"}},
			IsError: true,
		}, nil
	}

	ns := ""
	if namespace, ok := args["namespace"].(string); ok {
		ns = namespace
	}

	fullStr := "false"
	if fullArg, ok := args["full"].(string); ok {
		fullStr = fullArg
	}
	full := fullStr == "true"

	cmd := []string{"argo", "rollouts", "promote"}
	if ns != "" {
		cmd = append(cmd, "-n", ns)
	}
	cmd = append(cmd, rolloutName)
	if full {
		cmd = append(cmd, "--full")
	}

	output, err := runArgoRolloutCommand(ctx, cmd)
	if err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "Error promoting rollout: " + err.Error()}},
			IsError: true,
		}, nil
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: output}},
	}, nil
}

func handlePauseRollout(ctx context.Context, request *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var args map[string]interface{}
	if err := json.Unmarshal(request.Params.Arguments, &args); err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "failed to parse arguments"}},
			IsError: true,
		}, nil
	}

	rolloutName, ok := args["rollout_name"].(string)
	if !ok || rolloutName == "" {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "rollout_name parameter is required"}},
			IsError: true,
		}, nil
	}

	ns := ""
	if namespace, ok := args["namespace"].(string); ok {
		ns = namespace
	}

	cmd := []string{"argo", "rollouts", "pause"}
	if ns != "" {
		cmd = append(cmd, "-n", ns)
	}
	cmd = append(cmd, rolloutName)

	output, err := runArgoRolloutCommand(ctx, cmd)
	if err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "Error pausing rollout: " + err.Error()}},
			IsError: true,
		}, nil
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: output}},
	}, nil
}

func handleSetRolloutImage(ctx context.Context, request *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var args map[string]interface{}
	if err := json.Unmarshal(request.Params.Arguments, &args); err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "failed to parse arguments"}},
			IsError: true,
		}, nil
	}

	rolloutName, ok := args["rollout_name"].(string)
	if !ok || rolloutName == "" {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "rollout_name parameter is required"}},
			IsError: true,
		}, nil
	}

	containerImage, ok := args["container_image"].(string)
	if !ok || containerImage == "" {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "container_image parameter is required"}},
			IsError: true,
		}, nil
	}

	ns := ""
	if namespace, ok := args["namespace"].(string); ok {
		ns = namespace
	}

	cmd := []string{"argo", "rollouts", "set", "image", rolloutName, containerImage}
	if ns != "" {
		cmd = append(cmd, "-n", ns)
	}

	output, err := runArgoRolloutCommand(ctx, cmd)
	if err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "Error setting rollout image: " + err.Error()}},
			IsError: true,
		}, nil
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: output}},
	}, nil
}

// Gateway Plugin Status struct
type GatewayPluginStatus struct {
	Installed    bool    `json:"installed"`
	Version      string  `json:"version,omitempty"`
	Architecture string  `json:"architecture,omitempty"`
	DownloadTime float64 `json:"download_time,omitempty"`
	ErrorMessage string  `json:"error_message,omitempty"`
}

func (gps GatewayPluginStatus) String() string {
	data, _ := json.MarshalIndent(gps, "", "  ")
	return string(data)
}

func getSystemArchitecture() (string, error) {
	system := strings.ToLower(runtime.GOOS)
	machine := strings.ToLower(runtime.GOARCH)

	// Map Go architecture to plugin architecture
	archMap := map[string]string{
		"amd64": "amd64",
		"arm64": "arm64",
		"arm":   "arm",
	}

	arch, ok := archMap[machine]
	if !ok {
		arch = machine
	}

	switch system {
	case "windows":
		return fmt.Sprintf("windows-%s.exe", arch), nil
	case "darwin":
		return fmt.Sprintf("darwin-%s", arch), nil
	case "linux":
		return fmt.Sprintf("linux-%s", arch), nil
	default:
		return "", fmt.Errorf("unsupported system: %s", system)
	}
}

func getLatestVersion(ctx context.Context) string {
	client := &http.Client{Timeout: 10 * time.Second}
	req, err := http.NewRequestWithContext(ctx, "GET", "https://api.github.com/repos/argoproj-labs/rollouts-plugin-trafficrouter-gatewayapi/releases/latest", nil)
	if err != nil {
		return "0.5.0" // Default version
	}
	resp, err := client.Do(req)
	if err != nil {
		return "0.5.0" // Default version
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "0.5.0"
	}

	versionRegex := regexp.MustCompile(`"tag_name":\s*"v([^"]+)"`)
	matches := versionRegex.FindStringSubmatch(string(body))
	if len(matches) > 1 {
		return matches[1]
	}

	return "0.5.0"
}

func configureGatewayPlugin(ctx context.Context, version, namespace string) GatewayPluginStatus {
	arch, err := getSystemArchitecture()
	if err != nil {
		return GatewayPluginStatus{
			Installed:    false,
			ErrorMessage: fmt.Sprintf("Error determining system architecture: %s", err.Error()),
		}
	}

	if version == "" {
		version = getLatestVersion(ctx)
	}

	configMap := fmt.Sprintf(`apiVersion: v1
kind: ConfigMap
metadata:
  name: argo-rollouts-config
  namespace: %s
data:
  trafficRouterPlugins: |-
    - name: "argoproj-labs/gatewayAPI"
      location: "https://github.com/argoproj-labs/rollouts-plugin-trafficrouter-gatewayapi/releases/download/v%s/gatewayapi-plugin-%s"
`, namespace, version, arch)

	// Create temporary file
	tmpFile, err := os.CreateTemp("", "argo-gateway-config-*.yaml")
	if err != nil {
		return GatewayPluginStatus{
			Installed:    false,
			ErrorMessage: fmt.Sprintf("Failed to create temp file: %s", err.Error()),
		}
	}
	defer func() { _ = os.Remove(tmpFile.Name()) }()

	if _, err := tmpFile.WriteString(configMap); err != nil {
		return GatewayPluginStatus{
			Installed:    false,
			ErrorMessage: fmt.Sprintf("Failed to write config map: %s", err.Error()),
		}
	}
	_ = tmpFile.Close()

	// Apply the ConfigMap
	cmdArgs := []string{"apply", "-f", tmpFile.Name()}
	output, err := runArgoRolloutCommand(ctx, cmdArgs)
	if err != nil {
		return GatewayPluginStatus{
			Installed:    false,
			ErrorMessage: fmt.Sprintf("Error applying Gateway API plugin config: %s. Output: %s", err.Error(), output),
		}
	}

	return GatewayPluginStatus{
		Installed:    true,
		Version:      version,
		Architecture: arch,
	}
}

func handleVerifyGatewayPlugin(ctx context.Context, request *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var args map[string]interface{}
	if err := json.Unmarshal(request.Params.Arguments, &args); err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "failed to parse arguments"}},
			IsError: true,
		}, nil
	}

	version := ""
	if versionArg, ok := args["version"].(string); ok {
		version = versionArg
	}

	namespace := "argo-rollouts"
	if namespaceArg, ok := args["namespace"].(string); ok && namespaceArg != "" {
		namespace = namespaceArg
	}

	shouldInstallStr := "true"
	if shouldInstallArg, ok := args["should_install"].(string); ok {
		shouldInstallStr = shouldInstallArg
	}
	shouldInstall := shouldInstallStr == "true"

	// Check if ConfigMap exists and is configured
	cmd := []string{"get", "configmap", "argo-rollouts-config", "-n", namespace, "-o", "yaml"}
	output, err := runArgoRolloutCommand(ctx, cmd)
	if err == nil && strings.Contains(output, "argoproj-labs/gatewayAPI") {
		status := GatewayPluginStatus{
			Installed:    true,
			ErrorMessage: "Gateway API plugin is already configured",
		}
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: status.String()}},
		}, nil
	}

	if !shouldInstall {
		status := GatewayPluginStatus{
			Installed:    false,
			ErrorMessage: "Gateway API plugin is not configured and installation is disabled",
		}
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: status.String()}},
		}, nil
	}

	// Configure plugin
	status := configureGatewayPlugin(ctx, version, namespace)
	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: status.String()}},
	}, nil
}

func handleCheckPluginLogs(ctx context.Context, request *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var args map[string]interface{}
	if err := json.Unmarshal(request.Params.Arguments, &args); err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "failed to parse arguments"}},
			IsError: true,
		}, nil
	}

	namespace := "argo-rollouts"
	if namespaceArg, ok := args["namespace"].(string); ok && namespaceArg != "" {
		namespace = namespaceArg
	}

	// timeout parameter is parsed but not used currently
	_ = ""
	if timeoutArg, ok := args["timeout"].(string); ok {
		_ = timeoutArg
	}

	cmd := []string{"logs", "-n", namespace, "-l", "app.kubernetes.io/name=argo-rollouts", "--tail", "100"}
	output, err := runArgoRolloutCommand(ctx, cmd)
	if err != nil {
		status := GatewayPluginStatus{
			Installed:    false,
			ErrorMessage: err.Error(),
		}
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: status.String()}},
		}, nil
	}

	// Parse download information
	downloadPattern := regexp.MustCompile(`Downloading plugin argoproj-labs/gatewayAPI from: .*/v([\d.]+)/gatewayapi-plugin-([\w-]+)"`)
	timePattern := regexp.MustCompile(`Download complete, it took ([\d.]+)s`)

	versionMatches := downloadPattern.FindStringSubmatch(output)
	timeMatches := timePattern.FindStringSubmatch(output)

	if len(versionMatches) > 2 && len(timeMatches) > 1 {
		downloadTime, _ := strconv.ParseFloat(timeMatches[1], 64)
		status := GatewayPluginStatus{
			Installed:    true,
			Version:      versionMatches[1],
			Architecture: versionMatches[2],
			DownloadTime: downloadTime,
		}
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: status.String()}},
		}, nil
	}

	status := GatewayPluginStatus{
		Installed:    false,
		ErrorMessage: "Plugin installation not found in logs",
	}
	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: status.String()}},
	}, nil
}

func handleListRollouts(ctx context.Context, request *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var args map[string]interface{}
	if err := json.Unmarshal(request.Params.Arguments, &args); err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "failed to parse arguments"}},
			IsError: true,
		}, nil
	}

	ns := "argo-rollouts"
	if namespace, ok := args["namespace"].(string); ok && namespace != "" {
		ns = namespace
	}

	tt := "rollouts"
	if typeArg, ok := args["type"].(string); ok && typeArg != "" {
		tt = typeArg
	}

	cmd := []string{"argo", "rollouts", "list", tt}
	if ns != "" {
		cmd = append(cmd, "-n", ns)
	}

	output, err := runArgoRolloutCommand(ctx, cmd)
	if err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "Error listing rollouts: " + err.Error()}},
			IsError: true,
		}, nil
	}

	if strings.HasPrefix(output, "Error") {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: output}},
		}, nil
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: output}},
	}, nil
}

// ArgoCD tools

// handleArgoCDListApplications lists ArgoCD applications
func handleArgoCDListApplications(ctx context.Context, request *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var args map[string]interface{}
	if err := json.Unmarshal(request.Params.Arguments, &args); err != nil {
		return returnErrorResult("failed to parse arguments")
	}

	client, err := getArgoCDClient()
	if err != nil {
		return returnErrorResult(fmt.Sprintf("failed to create ArgoCD client: %v", err))
	}

	opts := &ListApplicationsOptions{}
	if search, ok := args["search"].(string); ok && search != "" {
		opts.Search = search
	}
	if limit, ok := args["limit"].(float64); ok {
		limitInt := int(limit)
		opts.Limit = &limitInt
	}
	if offset, ok := args["offset"].(float64); ok {
		offsetInt := int(offset)
		opts.Offset = &offsetInt
	}

	result, err := client.ListApplications(ctx, opts)
	if err != nil {
		return returnErrorResult(fmt.Sprintf("failed to list applications: %v", err))
	}

	return returnJSONResult(result)
}

// handleArgoCDGetApplication gets an ArgoCD application by name
func handleArgoCDGetApplication(ctx context.Context, request *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var args map[string]interface{}
	if err := json.Unmarshal(request.Params.Arguments, &args); err != nil {
		return returnErrorResult("failed to parse arguments")
	}

	appName, ok := args["applicationName"].(string)
	if !ok || appName == "" {
		return returnErrorResult("applicationName parameter is required")
	}

	client, err := getArgoCDClient()
	if err != nil {
		return returnErrorResult(fmt.Sprintf("failed to create ArgoCD client: %v", err))
	}

	var namespace *string
	if ns, ok := args["applicationNamespace"].(string); ok && ns != "" {
		namespace = &ns
	}

	result, err := client.GetApplication(ctx, appName, namespace)
	if err != nil {
		return returnErrorResult(fmt.Sprintf("failed to get application: %v", err))
	}

	return returnJSONResult(result)
}

// handleArgoCDGetApplicationResourceTree gets the resource tree for an application
func handleArgoCDGetApplicationResourceTree(ctx context.Context, request *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var args map[string]interface{}
	if err := json.Unmarshal(request.Params.Arguments, &args); err != nil {
		return returnErrorResult("failed to parse arguments")
	}

	appName, ok := args["applicationName"].(string)
	if !ok || appName == "" {
		return returnErrorResult("applicationName parameter is required")
	}

	client, err := getArgoCDClient()
	if err != nil {
		return returnErrorResult(fmt.Sprintf("failed to create ArgoCD client: %v", err))
	}

	result, err := client.GetApplicationResourceTree(ctx, appName)
	if err != nil {
		return returnErrorResult(fmt.Sprintf("failed to get application resource tree: %v", err))
	}

	return returnJSONResult(result)
}

// handleArgoCDGetApplicationManagedResources gets managed resources for an application
func handleArgoCDGetApplicationManagedResources(ctx context.Context, request *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var args map[string]interface{}
	if err := json.Unmarshal(request.Params.Arguments, &args); err != nil {
		return returnErrorResult("failed to parse arguments")
	}

	appName, ok := args["applicationName"].(string)
	if !ok || appName == "" {
		return returnErrorResult("applicationName parameter is required")
	}

	client, err := getArgoCDClient()
	if err != nil {
		return returnErrorResult(fmt.Sprintf("failed to create ArgoCD client: %v", err))
	}

	filters := &ManagedResourcesFilters{}
	if kind, ok := args["kind"].(string); ok && kind != "" {
		filters.Kind = &kind
	}
	if ns, ok := args["namespace"].(string); ok && ns != "" {
		filters.Namespace = &ns
	}
	if name, ok := args["name"].(string); ok && name != "" {
		filters.Name = &name
	}
	if version, ok := args["version"].(string); ok && version != "" {
		filters.Version = &version
	}
	if group, ok := args["group"].(string); ok && group != "" {
		filters.Group = &group
	}
	if appNs, ok := args["appNamespace"].(string); ok && appNs != "" {
		filters.AppNamespace = &appNs
	}
	if project, ok := args["project"].(string); ok && project != "" {
		filters.Project = &project
	}

	var filtersToUse *ManagedResourcesFilters
	if filters.Kind != nil || filters.Namespace != nil || filters.Name != nil || filters.Version != nil || filters.Group != nil || filters.AppNamespace != nil || filters.Project != nil {
		filtersToUse = filters
	}

	result, err := client.GetApplicationManagedResources(ctx, appName, filtersToUse)
	if err != nil {
		return returnErrorResult(fmt.Sprintf("failed to get application managed resources: %v", err))
	}

	return returnJSONResult(result)
}

// handleArgoCDGetApplicationWorkloadLogs gets logs for application workload
func handleArgoCDGetApplicationWorkloadLogs(ctx context.Context, request *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var args map[string]interface{}
	if err := json.Unmarshal(request.Params.Arguments, &args); err != nil {
		return returnErrorResult("failed to parse arguments")
	}

	appName, ok := args["applicationName"].(string)
	if !ok || appName == "" {
		return returnErrorResult("applicationName parameter is required")
	}

	appNamespace, ok := args["applicationNamespace"].(string)
	if !ok || appNamespace == "" {
		return returnErrorResult("applicationNamespace parameter is required")
	}

	container, ok := args["container"].(string)
	if !ok || container == "" {
		return returnErrorResult("container parameter is required")
	}

	resourceRefRaw, ok := args["resourceRef"]
	if !ok {
		return returnErrorResult("resourceRef parameter is required")
	}

	resourceRefJSON, err := json.Marshal(resourceRefRaw)
	if err != nil {
		return returnErrorResult(fmt.Sprintf("failed to marshal resourceRef: %v", err))
	}

	var resourceRef ResourceRef
	if err := json.Unmarshal(resourceRefJSON, &resourceRef); err != nil {
		return returnErrorResult(fmt.Sprintf("failed to unmarshal resourceRef: %v", err))
	}

	client, err := getArgoCDClient()
	if err != nil {
		return returnErrorResult(fmt.Sprintf("failed to create ArgoCD client: %v", err))
	}

	result, err := client.GetWorkloadLogs(ctx, appName, appNamespace, resourceRef, container)
	if err != nil {
		return returnErrorResult(fmt.Sprintf("failed to get workload logs: %v", err))
	}

	return returnJSONResult(result)
}

// handleArgoCDGetApplicationEvents gets events for an application
func handleArgoCDGetApplicationEvents(ctx context.Context, request *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var args map[string]interface{}
	if err := json.Unmarshal(request.Params.Arguments, &args); err != nil {
		return returnErrorResult("failed to parse arguments")
	}

	appName, ok := args["applicationName"].(string)
	if !ok || appName == "" {
		return returnErrorResult("applicationName parameter is required")
	}

	client, err := getArgoCDClient()
	if err != nil {
		return returnErrorResult(fmt.Sprintf("failed to create ArgoCD client: %v", err))
	}

	result, err := client.GetApplicationEvents(ctx, appName)
	if err != nil {
		return returnErrorResult(fmt.Sprintf("failed to get application events: %v", err))
	}

	return returnJSONResult(result)
}

// handleArgoCDGetResourceEvents gets events for a resource
func handleArgoCDGetResourceEvents(ctx context.Context, request *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var args map[string]interface{}
	if err := json.Unmarshal(request.Params.Arguments, &args); err != nil {
		return returnErrorResult("failed to parse arguments")
	}

	appName, ok := args["applicationName"].(string)
	if !ok || appName == "" {
		return returnErrorResult("applicationName parameter is required")
	}

	appNamespace, ok := args["applicationNamespace"].(string)
	if !ok || appNamespace == "" {
		return returnErrorResult("applicationNamespace parameter is required")
	}

	resourceUID, ok := args["resourceUID"].(string)
	if !ok || resourceUID == "" {
		return returnErrorResult("resourceUID parameter is required")
	}

	resourceNamespace, ok := args["resourceNamespace"].(string)
	if !ok || resourceNamespace == "" {
		return returnErrorResult("resourceNamespace parameter is required")
	}

	resourceName, ok := args["resourceName"].(string)
	if !ok || resourceName == "" {
		return returnErrorResult("resourceName parameter is required")
	}

	client, err := getArgoCDClient()
	if err != nil {
		return returnErrorResult(fmt.Sprintf("failed to create ArgoCD client: %v", err))
	}

	result, err := client.GetResourceEvents(ctx, appName, appNamespace, resourceUID, resourceNamespace, resourceName)
	if err != nil {
		return returnErrorResult(fmt.Sprintf("failed to get resource events: %v", err))
	}

	return returnJSONResult(result)
}

// handleArgoCDGetResources gets resource manifests
func handleArgoCDGetResources(ctx context.Context, request *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var args map[string]interface{}
	if err := json.Unmarshal(request.Params.Arguments, &args); err != nil {
		return returnErrorResult("failed to parse arguments")
	}

	appName, ok := args["applicationName"].(string)
	if !ok || appName == "" {
		return returnErrorResult("applicationName parameter is required")
	}

	appNamespace, ok := args["applicationNamespace"].(string)
	if !ok || appNamespace == "" {
		return returnErrorResult("applicationNamespace parameter is required")
	}

	client, err := getArgoCDClient()
	if err != nil {
		return returnErrorResult(fmt.Sprintf("failed to create ArgoCD client: %v", err))
	}

	var resourceRefs []ResourceRef
	if resourceRefsRaw, ok := args["resourceRefs"]; ok && resourceRefsRaw != nil {
		resourceRefsJSON, err := json.Marshal(resourceRefsRaw)
		if err != nil {
			return returnErrorResult(fmt.Sprintf("failed to marshal resourceRefs: %v", err))
		}

		if err := json.Unmarshal(resourceRefsJSON, &resourceRefs); err != nil {
			return returnErrorResult(fmt.Sprintf("failed to unmarshal resourceRefs: %v", err))
		}
	}

	// If no resourceRefs provided, get all resources from resource tree
	if len(resourceRefs) == 0 {
		tree, err := client.GetApplicationResourceTree(ctx, appName)
		if err != nil {
			return returnErrorResult(fmt.Sprintf("failed to get resource tree: %v", err))
		}

		// Parse tree to extract resource references
		treeJSON, err := json.Marshal(tree)
		if err != nil {
			return returnErrorResult(fmt.Sprintf("failed to marshal resource tree: %v", err))
		}

		var treeData map[string]interface{}
		if err := json.Unmarshal(treeJSON, &treeData); err != nil {
			return returnErrorResult(fmt.Sprintf("failed to unmarshal resource tree: %v", err))
		}

		if nodes, ok := treeData["nodes"].([]interface{}); ok {
			for _, nodeRaw := range nodes {
				if node, ok := nodeRaw.(map[string]interface{}); ok {
					ref := ResourceRef{}
					if uid, ok := node["uid"].(string); ok {
						ref.UID = uid
					}
					if version, ok := node["version"].(string); ok {
						ref.Version = version
					}
					if group, ok := node["group"].(string); ok {
						ref.Group = group
					}
					if kind, ok := node["kind"].(string); ok {
						ref.Kind = kind
					}
					if name, ok := node["name"].(string); ok {
						ref.Name = name
					}
					if ns, ok := node["namespace"].(string); ok {
						ref.Namespace = ns
					}
					if ref.UID != "" && ref.Name != "" && ref.Kind != "" {
						resourceRefs = append(resourceRefs, ref)
					}
				}
			}
		}
	}

	// Get all resources
	results := make([]interface{}, 0, len(resourceRefs))
	for _, ref := range resourceRefs {
		result, err := client.GetResource(ctx, appName, appNamespace, ref)
		if err != nil {
			return returnErrorResult(fmt.Sprintf("failed to get resource: %v", err))
		}
		results = append(results, result)
	}

	return returnJSONResult(results)
}

// handleArgoCDGetResourceActions gets available actions for a resource
func handleArgoCDGetResourceActions(ctx context.Context, request *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var args map[string]interface{}
	if err := json.Unmarshal(request.Params.Arguments, &args); err != nil {
		return returnErrorResult("failed to parse arguments")
	}

	appName, ok := args["applicationName"].(string)
	if !ok || appName == "" {
		return returnErrorResult("applicationName parameter is required")
	}

	appNamespace, ok := args["applicationNamespace"].(string)
	if !ok || appNamespace == "" {
		return returnErrorResult("applicationNamespace parameter is required")
	}

	resourceRefRaw, ok := args["resourceRef"]
	if !ok {
		return returnErrorResult("resourceRef parameter is required")
	}

	resourceRefJSON, err := json.Marshal(resourceRefRaw)
	if err != nil {
		return returnErrorResult(fmt.Sprintf("failed to marshal resourceRef: %v", err))
	}

	var resourceRef ResourceRef
	if err := json.Unmarshal(resourceRefJSON, &resourceRef); err != nil {
		return returnErrorResult(fmt.Sprintf("failed to unmarshal resourceRef: %v", err))
	}

	client, err := getArgoCDClient()
	if err != nil {
		return returnErrorResult(fmt.Sprintf("failed to create ArgoCD client: %v", err))
	}

	result, err := client.GetResourceActions(ctx, appName, appNamespace, resourceRef)
	if err != nil {
		return returnErrorResult(fmt.Sprintf("failed to get resource actions: %v", err))
	}

	return returnJSONResult(result)
}

// handleArgoCDCreateApplication creates a new ArgoCD application
func handleArgoCDCreateApplication(ctx context.Context, request *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var args map[string]interface{}
	if err := json.Unmarshal(request.Params.Arguments, &args); err != nil {
		return returnErrorResult("failed to parse arguments")
	}

	applicationRaw, ok := args["application"]
	if !ok {
		return returnErrorResult("application parameter is required")
	}

	client, err := getArgoCDClient()
	if err != nil {
		return returnErrorResult(fmt.Sprintf("failed to create ArgoCD client: %v", err))
	}

	result, err := client.CreateApplication(ctx, applicationRaw)
	if err != nil {
		return returnErrorResult(fmt.Sprintf("failed to create application: %v", err))
	}

	return returnJSONResult(result)
}

// handleArgoCDUpdateApplication updates an ArgoCD application
func handleArgoCDUpdateApplication(ctx context.Context, request *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var args map[string]interface{}
	if err := json.Unmarshal(request.Params.Arguments, &args); err != nil {
		return returnErrorResult("failed to parse arguments")
	}

	appName, ok := args["applicationName"].(string)
	if !ok || appName == "" {
		return returnErrorResult("applicationName parameter is required")
	}

	applicationRaw, ok := args["application"]
	if !ok {
		return returnErrorResult("application parameter is required")
	}

	client, err := getArgoCDClient()
	if err != nil {
		return returnErrorResult(fmt.Sprintf("failed to create ArgoCD client: %v", err))
	}

	result, err := client.UpdateApplication(ctx, appName, applicationRaw)
	if err != nil {
		return returnErrorResult(fmt.Sprintf("failed to update application: %v", err))
	}

	return returnJSONResult(result)
}

// handleArgoCDDeleteApplication deletes an ArgoCD application
func handleArgoCDDeleteApplication(ctx context.Context, request *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var args map[string]interface{}
	if err := json.Unmarshal(request.Params.Arguments, &args); err != nil {
		return returnErrorResult("failed to parse arguments")
	}

	appName, ok := args["applicationName"].(string)
	if !ok || appName == "" {
		return returnErrorResult("applicationName parameter is required")
	}

	client, err := getArgoCDClient()
	if err != nil {
		return returnErrorResult(fmt.Sprintf("failed to create ArgoCD client: %v", err))
	}

	options := &DeleteApplicationOptions{}
	if appNs, ok := args["applicationNamespace"].(string); ok && appNs != "" {
		options.AppNamespace = &appNs
	}
	if cascade, ok := args["cascade"].(bool); ok {
		options.Cascade = &cascade
	}
	if propagationPolicy, ok := args["propagationPolicy"].(string); ok && propagationPolicy != "" {
		options.PropagationPolicy = &propagationPolicy
	}

	var optionsToUse *DeleteApplicationOptions
	if options.AppNamespace != nil || options.Cascade != nil || options.PropagationPolicy != nil {
		optionsToUse = options
	}

	result, err := client.DeleteApplication(ctx, appName, optionsToUse)
	if err != nil {
		return returnErrorResult(fmt.Sprintf("failed to delete application: %v", err))
	}

	return returnJSONResult(result)
}

// handleArgoCDSyncApplication syncs an ArgoCD application
func handleArgoCDSyncApplication(ctx context.Context, request *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var args map[string]interface{}
	if err := json.Unmarshal(request.Params.Arguments, &args); err != nil {
		return returnErrorResult("failed to parse arguments")
	}

	appName, ok := args["applicationName"].(string)
	if !ok || appName == "" {
		return returnErrorResult("applicationName parameter is required")
	}

	client, err := getArgoCDClient()
	if err != nil {
		return returnErrorResult(fmt.Sprintf("failed to create ArgoCD client: %v", err))
	}

	options := &SyncApplicationOptions{}
	if appNs, ok := args["applicationNamespace"].(string); ok && appNs != "" {
		options.AppNamespace = &appNs
	}
	if dryRun, ok := args["dryRun"].(bool); ok {
		options.DryRun = &dryRun
	}
	if prune, ok := args["prune"].(bool); ok {
		options.Prune = &prune
	}
	if revision, ok := args["revision"].(string); ok && revision != "" {
		options.Revision = &revision
	}
	if syncOptionsRaw, ok := args["syncOptions"].([]interface{}); ok {
		syncOptions := make([]string, 0, len(syncOptionsRaw))
		for _, opt := range syncOptionsRaw {
			if optStr, ok := opt.(string); ok {
				syncOptions = append(syncOptions, optStr)
			}
		}
		if len(syncOptions) > 0 {
			options.SyncOptions = syncOptions
		}
	}

	var optionsToUse *SyncApplicationOptions
	if options.AppNamespace != nil || options.DryRun != nil || options.Prune != nil || options.Revision != nil || options.SyncOptions != nil {
		optionsToUse = options
	}

	result, err := client.SyncApplication(ctx, appName, optionsToUse)
	if err != nil {
		return returnErrorResult(fmt.Sprintf("failed to sync application: %v", err))
	}

	return returnJSONResult(result)
}

// handleArgoCDRunResourceAction runs an action on a resource
func handleArgoCDRunResourceAction(ctx context.Context, request *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var args map[string]interface{}
	if err := json.Unmarshal(request.Params.Arguments, &args); err != nil {
		return returnErrorResult("failed to parse arguments")
	}

	appName, ok := args["applicationName"].(string)
	if !ok || appName == "" {
		return returnErrorResult("applicationName parameter is required")
	}

	appNamespace, ok := args["applicationNamespace"].(string)
	if !ok || appNamespace == "" {
		return returnErrorResult("applicationNamespace parameter is required")
	}

	action, ok := args["action"].(string)
	if !ok || action == "" {
		return returnErrorResult("action parameter is required")
	}

	resourceRefRaw, ok := args["resourceRef"]
	if !ok {
		return returnErrorResult("resourceRef parameter is required")
	}

	resourceRefJSON, err := json.Marshal(resourceRefRaw)
	if err != nil {
		return returnErrorResult(fmt.Sprintf("failed to marshal resourceRef: %v", err))
	}

	var resourceRef ResourceRef
	if err := json.Unmarshal(resourceRefJSON, &resourceRef); err != nil {
		return returnErrorResult(fmt.Sprintf("failed to unmarshal resourceRef: %v", err))
	}

	client, err := getArgoCDClient()
	if err != nil {
		return returnErrorResult(fmt.Sprintf("failed to create ArgoCD client: %v", err))
	}

	result, err := client.RunResourceAction(ctx, appName, appNamespace, resourceRef, action)
	if err != nil {
		return returnErrorResult(fmt.Sprintf("failed to run resource action: %v", err))
	}

	return returnJSONResult(result)
}

// ToolRegistry is an interface for tool registration (to avoid import cycles)
type ToolRegistry interface {
	Register(tool *mcp.Tool, handler mcp.ToolHandler)
}

// RegisterTools registers Argo tools with the MCP server
func RegisterTools(s *mcp.Server) error {
	return RegisterToolsWithRegistry(s, nil)
}

// RegisterToolsWithRegistry registers Argo tools with the MCP server and optionally with a tool registry
func RegisterToolsWithRegistry(s *mcp.Server, registry ToolRegistry) error {
	logger.Get().Info("Registering Argo tools", "modules", []string{"Argo Rollouts", "ArgoCD"})

	// Helper function to register tool with both server and registry
	registerTool := func(tool *mcp.Tool, handler mcp.ToolHandler) {
		s.AddTool(tool, handler)
		if registry != nil {
			registry.Register(tool, handler)
		}
	}
	// Register argo_verify_argo_rollouts_controller_install tool
	registerTool(&mcp.Tool{
		Name:        "argo_verify_argo_rollouts_controller_install",
		Description: "Verify that the Argo Rollouts controller is installed and running",
		InputSchema: &jsonschema.Schema{
			Type: "object",
			Properties: map[string]*jsonschema.Schema{
				"namespace": {
					Type:        "string",
					Description: "The namespace where Argo Rollouts is installed",
				},
				"label": {
					Type:        "string",
					Description: "The label of the Argo Rollouts controller pods",
				},
			},
		},
	}, handleVerifyArgoRolloutsControllerInstall)

	// Register argo_verify_kubectl_plugin_install tool
	registerTool(&mcp.Tool{
		Name:        "argo_verify_kubectl_plugin_install",
		Description: "Verify that the kubectl Argo Rollouts plugin is installed",
		InputSchema: &jsonschema.Schema{
			Type: "object",
		},
	}, handleVerifyKubectlPluginInstall)

	// Register argo_rollouts_list tool
	registerTool(&mcp.Tool{
		Name:        "argo_rollouts_list",
		Description: "List rollouts or experiments",
		InputSchema: &jsonschema.Schema{
			Type: "object",
			Properties: map[string]*jsonschema.Schema{
				"namespace": {
					Type:        "string",
					Description: "The namespace of the rollout",
				},
				"type": {
					Type:        "string",
					Description: "What to list: rollouts or experiments",
				},
			},
		},
	}, handleListRollouts)

	// Register argo_promote_rollout tool
	registerTool(&mcp.Tool{
		Name:        "argo_promote_rollout",
		Description: "Promote a paused rollout to the next step",
		InputSchema: &jsonschema.Schema{
			Type: "object",
			Properties: map[string]*jsonschema.Schema{
				"rollout_name": {
					Type:        "string",
					Description: "The name of the rollout to promote",
				},
				"namespace": {
					Type:        "string",
					Description: "The namespace of the rollout",
				},
				"full": {
					Type:        "string",
					Description: "Promote the rollout to the final step",
				},
			},
			Required: []string{"rollout_name"},
		},
	}, handlePromoteRollout)

	// Register argo_pause_rollout tool
	registerTool(&mcp.Tool{
		Name:        "argo_pause_rollout",
		Description: "Pause a rollout",
		InputSchema: &jsonschema.Schema{
			Type: "object",
			Properties: map[string]*jsonschema.Schema{
				"rollout_name": {
					Type:        "string",
					Description: "The name of the rollout to pause",
				},
				"namespace": {
					Type:        "string",
					Description: "The namespace of the rollout",
				},
			},
			Required: []string{"rollout_name"},
		},
	}, handlePauseRollout)

	// Register argo_set_rollout_image tool
	registerTool(&mcp.Tool{
		Name:        "argo_set_rollout_image",
		Description: "Set the image of a rollout",
		InputSchema: &jsonschema.Schema{
			Type: "object",
			Properties: map[string]*jsonschema.Schema{
				"rollout_name": {
					Type:        "string",
					Description: "The name of the rollout to set the image for",
				},
				"container_image": {
					Type:        "string",
					Description: "The container image to set for the rollout",
				},
				"namespace": {
					Type:        "string",
					Description: "The namespace of the rollout",
				},
			},
			Required: []string{"rollout_name", "container_image"},
		},
	}, handleSetRolloutImage)

	// Register argo_verify_gateway_plugin tool
	registerTool(&mcp.Tool{
		Name:        "argo_verify_gateway_plugin",
		Description: "Verify the installation status of the Argo Rollouts Gateway API plugin",
		InputSchema: &jsonschema.Schema{
			Type: "object",
			Properties: map[string]*jsonschema.Schema{
				"version": {
					Type:        "string",
					Description: "The version of the plugin to check",
				},
				"namespace": {
					Type:        "string",
					Description: "The namespace for the plugin resources",
				},
				"should_install": {
					Type:        "string",
					Description: "Whether to install the plugin if not found",
				},
			},
		},
	}, handleVerifyGatewayPlugin)

	// Register argo_check_plugin_logs tool
	registerTool(&mcp.Tool{
		Name:        "argo_check_plugin_logs",
		Description: "Check the logs of the Argo Rollouts Gateway API plugin",
		InputSchema: &jsonschema.Schema{
			Type: "object",
			Properties: map[string]*jsonschema.Schema{
				"namespace": {
					Type:        "string",
					Description: "The namespace of the plugin resources",
				},
				"timeout": {
					Type:        "string",
					Description: "Timeout for log collection in seconds",
				},
			},
		},
	}, handleCheckPluginLogs)

	// Register ArgoCD tools (read-only)
	registerTool(&mcp.Tool{
		Name:        "argocd_list_applications",
		Description: "List ArgoCD applications with optional search, limit, and offset parameters",
		InputSchema: &jsonschema.Schema{
			Type: "object",
			Properties: map[string]*jsonschema.Schema{
				"search": {
					Type:        "string",
					Description: "Search applications by name. This is a partial match on the application name and does not support glob patterns (e.g. \"*\"). Optional.",
				},
				"limit": {
					Type:        "number",
					Description: "Maximum number of applications to return. Use this to reduce token usage when there are many applications. Optional.",
				},
				"offset": {
					Type:        "number",
					Description: "Number of applications to skip before returning results. Use with limit for pagination. Optional.",
				},
			},
		},
	}, handleArgoCDListApplications)

	registerTool(&mcp.Tool{
		Name:        "argocd_get_application",
		Description: "Get ArgoCD application by application name. Optionally specify the application namespace to get applications from non-default namespaces.",
		InputSchema: &jsonschema.Schema{
			Type: "object",
			Properties: map[string]*jsonschema.Schema{
				"applicationName": {
					Type:        "string",
					Description: "The name of the application",
				},
				"applicationNamespace": {
					Type:        "string",
					Description: "The namespace where the application is located. Optional if application is in the default namespace.",
				},
			},
			Required: []string{"applicationName"},
		},
	}, handleArgoCDGetApplication)

	registerTool(&mcp.Tool{
		Name:        "argocd_get_application_resource_tree",
		Description: "Get resource tree for ArgoCD application by application name",
		InputSchema: &jsonschema.Schema{
			Type: "object",
			Properties: map[string]*jsonschema.Schema{
				"applicationName": {
					Type:        "string",
					Description: "The name of the application",
				},
			},
			Required: []string{"applicationName"},
		},
	}, handleArgoCDGetApplicationResourceTree)

	registerTool(&mcp.Tool{
		Name:        "argocd_get_application_managed_resources",
		Description: "Get managed resources for ArgoCD application by application name with optional filtering. Use filters to avoid token limits with large applications. Examples: kind=\"ConfigMap\" for config maps only, namespace=\"production\" for specific namespace, or combine multiple filters.",
		InputSchema: &jsonschema.Schema{
			Type: "object",
			Properties: map[string]*jsonschema.Schema{
				"applicationName": {
					Type:        "string",
					Description: "The name of the application",
				},
				"kind": {
					Type:        "string",
					Description: "Filter by Kubernetes resource kind (e.g., \"ConfigMap\", \"Secret\", \"Deployment\")",
				},
				"namespace": {
					Type:        "string",
					Description: "Filter by Kubernetes namespace",
				},
				"name": {
					Type:        "string",
					Description: "Filter by resource name",
				},
				"version": {
					Type:        "string",
					Description: "Filter by resource API version",
				},
				"group": {
					Type:        "string",
					Description: "Filter by API group",
				},
				"appNamespace": {
					Type:        "string",
					Description: "Filter by Argo CD application namespace",
				},
				"project": {
					Type:        "string",
					Description: "Filter by Argo CD project",
				},
			},
			Required: []string{"applicationName"},
		},
	}, handleArgoCDGetApplicationManagedResources)

	registerTool(&mcp.Tool{
		Name:        "argocd_get_application_workload_logs",
		Description: "Get logs for ArgoCD application workload (Deployment, StatefulSet, Pod, etc.) by application name and resource ref and optionally container name",
		InputSchema: &jsonschema.Schema{
			Type: "object",
			Properties: map[string]*jsonschema.Schema{
				"applicationName": {
					Type:        "string",
					Description: "The name of the application",
				},
				"applicationNamespace": {
					Type:        "string",
					Description: "The namespace where the application is located",
				},
				"resourceRef": {
					Type:        "object",
					Description: "Resource reference containing uid, version, group, kind, name, and namespace",
				},
				"container": {
					Type:        "string",
					Description: "The container name",
				},
			},
			Required: []string{"applicationName", "applicationNamespace", "resourceRef", "container"},
		},
	}, handleArgoCDGetApplicationWorkloadLogs)

	registerTool(&mcp.Tool{
		Name:        "argocd_get_application_events",
		Description: "Get events for ArgoCD application by application name",
		InputSchema: &jsonschema.Schema{
			Type: "object",
			Properties: map[string]*jsonschema.Schema{
				"applicationName": {
					Type:        "string",
					Description: "The name of the application",
				},
			},
			Required: []string{"applicationName"},
		},
	}, handleArgoCDGetApplicationEvents)

	registerTool(&mcp.Tool{
		Name:        "argocd_get_resource_events",
		Description: "Get events for a resource that is managed by an ArgoCD application",
		InputSchema: &jsonschema.Schema{
			Type: "object",
			Properties: map[string]*jsonschema.Schema{
				"applicationName": {
					Type:        "string",
					Description: "The name of the application",
				},
				"applicationNamespace": {
					Type:        "string",
					Description: "The namespace where the application is located",
				},
				"resourceUID": {
					Type:        "string",
					Description: "The UID of the resource",
				},
				"resourceNamespace": {
					Type:        "string",
					Description: "The namespace of the resource",
				},
				"resourceName": {
					Type:        "string",
					Description: "The name of the resource",
				},
			},
			Required: []string{"applicationName", "applicationNamespace", "resourceUID", "resourceNamespace", "resourceName"},
		},
	}, handleArgoCDGetResourceEvents)

	registerTool(&mcp.Tool{
		Name:        "argocd_get_resources",
		Description: "Get manifests for resources specified by resourceRefs. If resourceRefs is empty or not provided, fetches all resources managed by the application.",
		InputSchema: &jsonschema.Schema{
			Type: "object",
			Properties: map[string]*jsonschema.Schema{
				"applicationName": {
					Type:        "string",
					Description: "The name of the application",
				},
				"applicationNamespace": {
					Type:        "string",
					Description: "The namespace where the application is located",
				},
				"resourceRefs": {
					Type:        "array",
					Description: "Array of resource references. If empty, fetches all resources from the application.",
				},
			},
			Required: []string{"applicationName", "applicationNamespace"},
		},
	}, handleArgoCDGetResources)

	registerTool(&mcp.Tool{
		Name:        "argocd_get_resource_actions",
		Description: "Get actions for a resource that is managed by an ArgoCD application",
		InputSchema: &jsonschema.Schema{
			Type: "object",
			Properties: map[string]*jsonschema.Schema{
				"applicationName": {
					Type:        "string",
					Description: "The name of the application",
				},
				"applicationNamespace": {
					Type:        "string",
					Description: "The namespace where the application is located",
				},
				"resourceRef": {
					Type:        "object",
					Description: "Resource reference containing uid, version, group, kind, name, and namespace",
				},
			},
			Required: []string{"applicationName", "applicationNamespace", "resourceRef"},
		},
	}, handleArgoCDGetResourceActions)

	// Register write tools only if not in read-only mode
	if !isReadOnlyMode() {
		registerTool(&mcp.Tool{
			Name:        "argocd_create_application",
			Description: "Create a new ArgoCD application in the specified namespace. The application.metadata.namespace field determines where the Application resource will be created (e.g., \"argocd\", \"argocd-apps\", or any custom namespace).",
			InputSchema: &jsonschema.Schema{
				Type: "object",
				Properties: map[string]*jsonschema.Schema{
					"application": {
						Type:        "object",
						Description: "The ArgoCD Application resource definition",
					},
				},
				Required: []string{"application"},
			},
		}, handleArgoCDCreateApplication)

		registerTool(&mcp.Tool{
			Name:        "argocd_update_application",
			Description: "Update an ArgoCD application",
			InputSchema: &jsonschema.Schema{
				Type: "object",
				Properties: map[string]*jsonschema.Schema{
					"applicationName": {
						Type:        "string",
						Description: "The name of the application to update",
					},
					"application": {
						Type:        "object",
						Description: "The updated ArgoCD Application resource definition",
					},
				},
				Required: []string{"applicationName", "application"},
			},
		}, handleArgoCDUpdateApplication)

		registerTool(&mcp.Tool{
			Name:        "argocd_delete_application",
			Description: "Delete an ArgoCD application. Specify applicationNamespace if the application is in a non-default namespace to avoid permission errors.",
			InputSchema: &jsonschema.Schema{
				Type: "object",
				Properties: map[string]*jsonschema.Schema{
					"applicationName": {
						Type:        "string",
						Description: "The name of the application to delete",
					},
					"applicationNamespace": {
						Type:        "string",
						Description: "The namespace where the application is located. Required if application is not in the default namespace.",
					},
					"cascade": {
						Type:        "boolean",
						Description: "Whether to cascade the deletion to child resources",
					},
					"propagationPolicy": {
						Type:        "string",
						Description: "Deletion propagation policy (e.g., \"Foreground\", \"Background\", \"Orphan\")",
					},
				},
				Required: []string{"applicationName"},
			},
		}, handleArgoCDDeleteApplication)

		registerTool(&mcp.Tool{
			Name:        "argocd_sync_application",
			Description: "Sync an ArgoCD application. Specify applicationNamespace if the application is in a non-default namespace to avoid permission errors.",
			InputSchema: &jsonschema.Schema{
				Type: "object",
				Properties: map[string]*jsonschema.Schema{
					"applicationName": {
						Type:        "string",
						Description: "The name of the application to sync",
					},
					"applicationNamespace": {
						Type:        "string",
						Description: "The namespace where the application is located. Required if application is not in the default namespace.",
					},
					"dryRun": {
						Type:        "boolean",
						Description: "Perform a dry run sync without applying changes",
					},
					"prune": {
						Type:        "boolean",
						Description: "Remove resources that are no longer defined in the source",
					},
					"revision": {
						Type:        "string",
						Description: "Sync to a specific revision instead of the latest",
					},
					"syncOptions": {
						Type:        "array",
						Description: "Additional sync options (e.g., [\"CreateNamespace=true\", \"PrunePropagationPolicy=foreground\"])",
						Items: &jsonschema.Schema{
							Type: "string",
						},
					},
				},
				Required: []string{"applicationName"},
			},
		}, handleArgoCDSyncApplication)

		registerTool(&mcp.Tool{
			Name:        "argocd_run_resource_action",
			Description: "Run an action on a resource managed by an ArgoCD application",
			InputSchema: &jsonschema.Schema{
				Type: "object",
				Properties: map[string]*jsonschema.Schema{
					"applicationName": {
						Type:        "string",
						Description: "The name of the application",
					},
					"applicationNamespace": {
						Type:        "string",
						Description: "The namespace where the application is located",
					},
					"resourceRef": {
						Type:        "object",
						Description: "Resource reference containing uid, version, group, kind, name, and namespace",
					},
					"action": {
						Type:        "string",
						Description: "The action to run on the resource",
					},
				},
				Required: []string{"applicationName", "applicationNamespace", "resourceRef", "action"},
			},
		}, handleArgoCDRunResourceAction)
	}

	return nil
}
