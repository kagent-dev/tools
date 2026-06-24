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

	"github.com/kagent-dev/tools/internal/commands"
	mcp "github.com/kagent-dev/tools/internal/mcp"
	"github.com/kagent-dev/tools/pkg/utils"
)

type verifyArgoRolloutsControllerInstallInput struct {
	Namespace string `json:"namespace" jsonschema:"The namespace where Argo Rollouts is installed"`
	Label     string `json:"label" jsonschema:"The label of the Argo Rollouts controller pods"`
}

func handleVerifyArgoRolloutsControllerInstall(ctx context.Context, request *mcp.CallToolRequest, in verifyArgoRolloutsControllerInstallInput) (*mcp.CallToolResult, any, error) {
	ns := in.Namespace
	if ns == "" {
		ns = "argo-rollouts"
	}
	label := in.Label
	if label == "" {
		label = "app.kubernetes.io/component=rollouts-controller"
	}

	cmd := []string{"get", "pods", "-n", ns, "-l", label, "-o", "jsonpath={.items[*].status.phase}"}
	output, err := runArgoRolloutCommand(ctx, cmd)
	if err != nil {
		return mcp.NewToolResultError("Error: " + err.Error()), nil, nil
	}

	output = strings.TrimSpace(output)
	if output == "" {
		return mcp.NewToolResultText("Error: No pods found"), nil, nil
	}

	if strings.HasPrefix(output, "Error") {
		return mcp.NewToolResultText(output), nil, nil
	}

	podStatuses := strings.Fields(output)
	if len(podStatuses) == 0 {
		return mcp.NewToolResultText("Error: No pod statuses returned"), nil, nil
	}

	allRunning := true
	for _, status := range podStatuses {
		if status != "Running" {
			allRunning = false
			break
		}
	}

	if allRunning {
		return mcp.NewToolResultText("All pods are running"), nil, nil
	}
	return mcp.NewToolResultText("Error: Not all pods are running (" + strings.Join(podStatuses, " ") + ")"), nil, nil
}

type verifyKubectlPluginInstallInput struct{}

func handleVerifyKubectlPluginInstall(ctx context.Context, request *mcp.CallToolRequest, in verifyKubectlPluginInstallInput) (*mcp.CallToolResult, any, error) {
	args := []string{"argo", "rollouts", "version"}
	output, err := runArgoRolloutCommand(ctx, args)
	if err != nil {
		return mcp.NewToolResultText("Kubectl Argo Rollouts plugin is not installed: " + err.Error()), nil, nil
	}

	if strings.HasPrefix(output, "Error") {
		return mcp.NewToolResultText("Kubectl Argo Rollouts plugin is not installed: " + output), nil, nil
	}

	return mcp.NewToolResultText(output), nil, nil
}

func runArgoRolloutCommand(ctx context.Context, args []string) (string, error) {
	kubeconfigPath := utils.GetKubeconfig()
	return commands.NewCommandBuilder("kubectl").
		WithArgs(args...).
		WithKubeconfig(kubeconfigPath).
		Execute(ctx)
}

type promoteRolloutInput struct {
	RolloutName string `json:"rollout_name" jsonschema:"The name of the rollout to promote"`
	Namespace   string `json:"namespace" jsonschema:"The namespace of the rollout"`
	Full        bool   `json:"full" jsonschema:"Promote the rollout to the final step"`
}

func handlePromoteRollout(ctx context.Context, request *mcp.CallToolRequest, in promoteRolloutInput) (*mcp.CallToolResult, any, error) {
	if in.RolloutName == "" {
		return mcp.NewToolResultError("rollout_name parameter is required"), nil, nil
	}

	cmd := []string{"argo", "rollouts", "promote"}
	if in.Namespace != "" {
		cmd = append(cmd, "-n", in.Namespace)
	}
	cmd = append(cmd, in.RolloutName)
	if in.Full {
		cmd = append(cmd, "--full")
	}

	output, err := runArgoRolloutCommand(ctx, cmd)
	if err != nil {
		return mcp.NewToolResultError("Error promoting rollout: " + err.Error()), nil, nil
	}

	return mcp.NewToolResultText(output), nil, nil
}

type pauseRolloutInput struct {
	RolloutName string `json:"rollout_name" jsonschema:"The name of the rollout to pause"`
	Namespace   string `json:"namespace" jsonschema:"The namespace of the rollout"`
}

func handlePauseRollout(ctx context.Context, request *mcp.CallToolRequest, in pauseRolloutInput) (*mcp.CallToolResult, any, error) {
	if in.RolloutName == "" {
		return mcp.NewToolResultError("rollout_name parameter is required"), nil, nil
	}

	cmd := []string{"argo", "rollouts", "pause"}
	if in.Namespace != "" {
		cmd = append(cmd, "-n", in.Namespace)
	}
	cmd = append(cmd, in.RolloutName)

	output, err := runArgoRolloutCommand(ctx, cmd)
	if err != nil {
		return mcp.NewToolResultError("Error pausing rollout: " + err.Error()), nil, nil
	}

	return mcp.NewToolResultText(output), nil, nil
}

type setRolloutImageInput struct {
	RolloutName    string `json:"rollout_name" jsonschema:"The name of the rollout to set the image for"`
	ContainerImage string `json:"container_image" jsonschema:"The container image to set for the rollout"`
	Namespace      string `json:"namespace" jsonschema:"The namespace of the rollout"`
}

func handleSetRolloutImage(ctx context.Context, request *mcp.CallToolRequest, in setRolloutImageInput) (*mcp.CallToolResult, any, error) {
	if in.RolloutName == "" {
		return mcp.NewToolResultError("rollout_name parameter is required"), nil, nil
	}
	if in.ContainerImage == "" {
		return mcp.NewToolResultError("container_image parameter is required"), nil, nil
	}

	cmd := []string{"argo", "rollouts", "set", "image", in.RolloutName, in.ContainerImage}
	if in.Namespace != "" {
		cmd = append(cmd, "-n", in.Namespace)
	}

	output, err := runArgoRolloutCommand(ctx, cmd)
	if err != nil {
		return mcp.NewToolResultError("Error setting rollout image: " + err.Error()), nil, nil
	}

	return mcp.NewToolResultText(output), nil, nil
}

// GatewayPluginStatus struct
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
	defer resp.Body.Close()

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
	defer os.Remove(tmpFile.Name())

	if _, err := tmpFile.WriteString(configMap); err != nil {
		return GatewayPluginStatus{
			Installed:    false,
			ErrorMessage: fmt.Sprintf("Failed to write config map: %s", err.Error()),
		}
	}
	tmpFile.Close()

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

type verifyGatewayPluginInput struct {
	Version       string `json:"version" jsonschema:"The version of the plugin to check"`
	Namespace     string `json:"namespace" jsonschema:"The namespace for the plugin resources"`
	ShouldInstall *bool  `json:"should_install" jsonschema:"Whether to install the plugin if not found"`
}

func handleVerifyGatewayPlugin(ctx context.Context, request *mcp.CallToolRequest, in verifyGatewayPluginInput) (*mcp.CallToolResult, any, error) {
	version := in.Version
	namespace := in.Namespace
	if namespace == "" {
		namespace = "argo-rollouts"
	}
	shouldInstall := true
	if in.ShouldInstall != nil {
		shouldInstall = *in.ShouldInstall
	}

	// Check if ConfigMap exists and is configured
	cmd := []string{"get", "configmap", "argo-rollouts-config", "-n", namespace, "-o", "yaml"}
	output, err := runArgoRolloutCommand(ctx, cmd)
	if err == nil && strings.Contains(output, "argoproj-labs/gatewayAPI") {
		status := GatewayPluginStatus{
			Installed:    true,
			ErrorMessage: "Gateway API plugin is already configured",
		}
		return mcp.NewToolResultText(status.String()), nil, nil
	}

	if !shouldInstall {
		status := GatewayPluginStatus{
			Installed:    false,
			ErrorMessage: "Gateway API plugin is not configured and installation is disabled",
		}
		return mcp.NewToolResultText(status.String()), nil, nil
	}

	// Configure plugin
	status := configureGatewayPlugin(ctx, version, namespace)
	return mcp.NewToolResultText(status.String()), nil, nil
}

type checkPluginLogsInput struct {
	Namespace string `json:"namespace" jsonschema:"The namespace of the plugin resources"`
	Timeout   int    `json:"timeout" jsonschema:"Timeout for log collection in seconds"`
}

func handleCheckPluginLogs(ctx context.Context, request *mcp.CallToolRequest, in checkPluginLogsInput) (*mcp.CallToolResult, any, error) {
	namespace := in.Namespace
	if namespace == "" {
		namespace = "argo-rollouts"
	}
	// timeout parameter is parsed but not used currently
	if in.Timeout == 0 {
		in.Timeout = 60
	}
	_ = in.Timeout

	cmd := []string{"logs", "-n", namespace, "-l", "app.kubernetes.io/name=argo-rollouts", "--tail", "100"}
	output, err := runArgoRolloutCommand(ctx, cmd)
	if err != nil {
		status := GatewayPluginStatus{
			Installed:    false,
			ErrorMessage: err.Error(),
		}
		return mcp.NewToolResultText(status.String()), nil, nil
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
		return mcp.NewToolResultText(status.String()), nil, nil
	}

	status := GatewayPluginStatus{
		Installed:    false,
		ErrorMessage: "Plugin installation not found in logs",
	}
	return mcp.NewToolResultText(status.String()), nil, nil
}

type listRolloutsInput struct {
	Namespace string `json:"namespace" jsonschema:"The namespace of the rollout"`
	Type      string `json:"type" jsonschema:"What to list: rollouts or experiments"`
}

func handleListRollouts(ctx context.Context, request *mcp.CallToolRequest, in listRolloutsInput) (*mcp.CallToolResult, any, error) {
	ns := in.Namespace
	if ns == "" {
		ns = "argo-rollouts"
	}
	tt := in.Type
	if tt == "" {
		tt = "rollouts"
	}

	cmd := []string{"argo", "rollouts", "list", tt}
	if ns != "" {
		cmd = append(cmd, "-n", ns)
	}

	output, err := runArgoRolloutCommand(ctx, cmd)
	if err != nil {
		return mcp.NewToolResultError("Error listing rollouts: " + err.Error()), nil, nil
	}

	if strings.HasPrefix(output, "Error") {
		return mcp.NewToolResultText(output), nil, nil
	}

	return mcp.NewToolResultText(output), nil, nil
}

func RegisterTools(s *mcp.Server, readOnly bool) {
	// Read-only tools - always registered
	mcp.AddTool(s, "argo", &mcp.Tool{
		Name:        "argo_verify_argo_rollouts_controller_install",
		Description: "Verify that the Argo Rollouts controller is installed and running",
	}, handleVerifyArgoRolloutsControllerInstall)

	mcp.AddTool(s, "argo", &mcp.Tool{
		Name:        "argo_verify_kubectl_plugin_install",
		Description: "Verify that the kubectl Argo Rollouts plugin is installed",
	}, handleVerifyKubectlPluginInstall)

	mcp.AddTool(s, "argo", &mcp.Tool{
		Name:        "argo_rollouts_list",
		Description: "List rollouts or experiments",
	}, handleListRollouts)

	mcp.AddTool(s, "argo", &mcp.Tool{
		Name:        "argo_check_plugin_logs",
		Description: "Check the logs of the Argo Rollouts Gateway API plugin",
	}, handleCheckPluginLogs)

	// Write tools - only registered when not in read-only mode
	if !readOnly {
		mcp.AddTool(s, "argo", &mcp.Tool{
			Name:        "argo_promote_rollout",
			Description: "Promote a paused rollout to the next step",
		}, handlePromoteRollout)

		mcp.AddTool(s, "argo", &mcp.Tool{
			Name:        "argo_pause_rollout",
			Description: "Pause a rollout",
		}, handlePauseRollout)

		mcp.AddTool(s, "argo", &mcp.Tool{
			Name:        "argo_set_rollout_image",
			Description: "Set the image of a rollout",
		}, handleSetRolloutImage)

		mcp.AddTool(s, "argo", &mcp.Tool{
			Name:        "argo_verify_gateway_plugin",
			Description: "Verify the installation status of the Argo Rollouts Gateway API plugin",
		}, handleVerifyGatewayPlugin)
	}
}
