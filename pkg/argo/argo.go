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

func RegisterTools(s *mcp.Server) error {
	logger.Get().Info("RegisterTools initialized")
	// Register argo_verify_argo_rollouts_controller_install tool
	s.AddTool(&mcp.Tool{
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
	s.AddTool(&mcp.Tool{
		Name:        "argo_verify_kubectl_plugin_install",
		Description: "Verify that the kubectl Argo Rollouts plugin is installed",
		InputSchema: &jsonschema.Schema{
			Type: "object",
		},
	}, handleVerifyKubectlPluginInstall)

	// Register argo_rollouts_list tool
	s.AddTool(&mcp.Tool{
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
	s.AddTool(&mcp.Tool{
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
	s.AddTool(&mcp.Tool{
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
	s.AddTool(&mcp.Tool{
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
	s.AddTool(&mcp.Tool{
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
	s.AddTool(&mcp.Tool{
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

	return nil
}
