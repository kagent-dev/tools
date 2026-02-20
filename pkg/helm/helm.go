package helm

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/kagent-dev/tools/internal/commands"
	"github.com/kagent-dev/tools/internal/errors"
	"github.com/kagent-dev/tools/internal/mcpcompat"
	"github.com/kagent-dev/tools/internal/mcpcompat/server"
	"github.com/kagent-dev/tools/internal/security"
	"github.com/kagent-dev/tools/internal/telemetry"
	"github.com/kagent-dev/tools/pkg/utils"
)

type listReleasesRequest struct {
	Namespace     string `json:"namespace"`
	AllNamespaces string `json:"all_namespaces"`
	All           string `json:"all"`
	Uninstalled   string `json:"uninstalled"`
	Uninstalling  string `json:"uninstalling"`
	Failed        string `json:"failed"`
	Deployed      string `json:"deployed"`
	Pending       string `json:"pending"`
	Filter        string `json:"filter"`
	Output        string `json:"output"`
}

type getReleaseRequest struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Resource  string `json:"resource"`
}

type upgradeReleaseRequest struct {
	Name      string `json:"name"`
	Chart     string `json:"chart"`
	Namespace string `json:"namespace"`
	Version   string `json:"version"`
	Values    string `json:"values"`
	Set       string `json:"set"`
	Install   string `json:"install"`
	DryRun    string `json:"dry_run"`
	Wait      string `json:"wait"`
}

type uninstallRequest struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	DryRun    string `json:"dry_run"`
	Wait      string `json:"wait"`
}

type repoAddRequest struct {
	Name string `json:"name"`
	URL  string `json:"url"`
}

func decodeArgs(request mcp.CallToolRequest, out interface{}) error {
	if request.Params == nil || request.Params.Arguments == nil {
		return nil
	}
	return json.Unmarshal(request.Params.Arguments, out)
}

func isTrue(v string) bool {
	return v == "true"
}

// Helm list releases
func handleHelmListReleases(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var req listReleasesRequest
	if err := decodeArgs(request, &req); err != nil {
		return mcp.NewToolResultError("invalid arguments: " + err.Error()), nil
	}

	namespace := req.Namespace
	allNamespaces := isTrue(req.AllNamespaces)
	all := isTrue(req.All)
	uninstalled := isTrue(req.Uninstalled)
	uninstalling := isTrue(req.Uninstalling)
	failed := isTrue(req.Failed)
	deployed := isTrue(req.Deployed)
	pending := isTrue(req.Pending)
	filter := req.Filter
	output := req.Output

	args := []string{"list"}

	if namespace != "" {
		args = append(args, "-n", namespace)
	}

	if allNamespaces {
		args = append(args, "-A")
	}

	if all {
		args = append(args, "-a")
	}

	if uninstalled {
		args = append(args, "--uninstalled")
	}

	if uninstalling {
		args = append(args, "--uninstalling")
	}

	if failed {
		args = append(args, "--failed")
	}

	if deployed {
		args = append(args, "--deployed")
	}

	if pending {
		args = append(args, "--pending")
	}

	if filter != "" {
		args = append(args, "-f", filter)
	}

	if output != "" {
		args = append(args, "-o", output)
	}

	result, err := runHelmCommand(ctx, args)
	if err != nil {
		// Check if it's a structured error
		if toolErr, ok := err.(*errors.ToolError); ok {
			// Add namespace context if provided
			if namespace != "" {
				toolErr = toolErr.WithContext("namespace", namespace)
			}
			return toolErr.ToMCPResult(), nil
		}
		// Fallback for non-structured errors
		return mcp.NewToolResultError(fmt.Sprintf("Helm list command failed: %v", err)), nil
	}

	return mcp.NewToolResultText(result), nil
}

func runHelmCommand(ctx context.Context, args []string) (string, error) {
	kubeconfigPath := utils.GetKubeconfig()

	// Add timeout for helm upgrade commands
	cmdBuilder := commands.NewCommandBuilder("helm").
		WithArgs(args...).
		WithKubeconfig(kubeconfigPath)

	// Only add timeout for upgrade commands
	if len(args) > 0 && args[0] == "upgrade" {
		cmdBuilder = cmdBuilder.WithTimeout(30 * time.Second)
	}

	result, err := cmdBuilder.Execute(ctx)

	if err != nil {
		if toolErr, ok := err.(*errors.ToolError); ok {
			if len(args) > 0 {
				toolErr = toolErr.WithContext("helm_operation", args[0])
			}
			toolErr = toolErr.WithContext("helm_args", args)
			return "", toolErr
		}
		return "", err
	}

	return result, nil
}

// Helm get release
func handleHelmGetRelease(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var req getReleaseRequest
	if err := decodeArgs(request, &req); err != nil {
		return mcp.NewToolResultError("invalid arguments: " + err.Error()), nil
	}

	name := req.Name
	namespace := req.Namespace
	resource := req.Resource
	if resource == "" {
		resource = "all"
	}

	if name == "" {
		return mcp.NewToolResultError("name parameter is required"), nil
	}

	if namespace == "" {
		return mcp.NewToolResultError("namespace parameter is required"), nil
	}

	args := []string{"get", resource, name, "-n", namespace}

	result, err := runHelmCommand(ctx, args)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Helm get command failed: %v", err)), nil
	}

	return mcp.NewToolResultText(result), nil
}

// Helm upgrade release
func handleHelmUpgradeRelease(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var req upgradeReleaseRequest
	if err := decodeArgs(request, &req); err != nil {
		return mcp.NewToolResultError("invalid arguments: " + err.Error()), nil
	}

	name := req.Name
	chart := req.Chart
	namespace := req.Namespace
	version := req.Version
	values := req.Values
	setValues := req.Set
	install := isTrue(req.Install)
	dryRun := isTrue(req.DryRun)
	wait := isTrue(req.Wait)

	if name == "" || chart == "" {
		return mcp.NewToolResultError("name and chart parameters are required"), nil
	}

	// Validate release name
	if err := security.ValidateHelmReleaseName(name); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Invalid release name: %v", err)), nil
	}

	// Validate namespace if provided
	if namespace != "" {
		if err := security.ValidateNamespace(namespace); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Invalid namespace: %v", err)), nil
		}
	}

	// Validate values file path if provided
	if values != "" {
		if err := security.ValidateFilePath(values); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Invalid values file path: %v", err)), nil
		}
	}

	args := []string{"upgrade", name, chart}

	if namespace != "" {
		args = append(args, "-n", namespace)
	}

	if version != "" {
		args = append(args, "--version", version)
	}

	if values != "" {
		args = append(args, "-f", values)
	}

	if setValues != "" {
		// Split multiple set values by comma
		setValuesList := strings.Split(setValues, ",")
		for _, setValue := range setValuesList {
			args = append(args, "--set", strings.TrimSpace(setValue))
		}
	}

	if install {
		args = append(args, "--install")
	}

	if dryRun {
		args = append(args, "--dry-run")
	}

	if wait {
		args = append(args, "--wait")
	}

	result, err := runHelmCommand(ctx, args)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Helm upgrade command failed: %v", err)), nil
	}

	return mcp.NewToolResultText(result), nil
}

// Helm uninstall release
func handleHelmUninstall(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var req uninstallRequest
	if err := decodeArgs(request, &req); err != nil {
		return mcp.NewToolResultError("invalid arguments: " + err.Error()), nil
	}

	name := req.Name
	namespace := req.Namespace
	dryRun := isTrue(req.DryRun)
	wait := isTrue(req.Wait)

	if name == "" || namespace == "" {
		return mcp.NewToolResultError("name and namespace parameters are required"), nil
	}

	args := []string{"uninstall", name, "-n", namespace}

	if dryRun {
		args = append(args, "--dry-run")
	}

	if wait {
		args = append(args, "--wait")
	}

	result, err := runHelmCommand(ctx, args)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Helm uninstall command failed: %v", err)), nil
	}

	return mcp.NewToolResultText(result), nil
}

// Helm repo add
func handleHelmRepoAdd(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var req repoAddRequest
	if err := decodeArgs(request, &req); err != nil {
		return mcp.NewToolResultError("invalid arguments: " + err.Error()), nil
	}

	name := req.Name
	url := req.URL

	if name == "" || url == "" {
		return mcp.NewToolResultError("name and url parameters are required"), nil
	}

	// Validate repository name
	if err := security.ValidateHelmReleaseName(name); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Invalid repository name: %v", err)), nil
	}

	// Validate repository URL
	if err := security.ValidateURL(url); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Invalid repository URL: %v", err)), nil
	}

	args := []string{"repo", "add", name, url}

	result, err := runHelmCommand(ctx, args)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Helm repo add command failed: %v", err)), nil
	}

	return mcp.NewToolResultText(result), nil
}

// Helm repo update
func handleHelmRepoUpdate(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := []string{"repo", "update"}

	result, err := runHelmCommand(ctx, args)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Helm repo update command failed: %v", err)), nil
	}

	return mcp.NewToolResultText(result), nil
}

// Register Helm tools
func RegisterTools(s *server.MCPServer, readOnly bool) {
	// Read-only tools - always registered
	s.AddTool(mcp.NewTool("helm_list_releases",
		mcp.WithDescription("List Helm releases in a namespace"),
		mcp.WithString("namespace", mcp.Description("The namespace to list releases from")),
		mcp.WithString("all_namespaces", mcp.Description("List releases from all namespaces")),
		mcp.WithString("all", mcp.Description("Show all releases without any filter applied")),
		mcp.WithString("uninstalled", mcp.Description("List uninstalled releases")),
		mcp.WithString("uninstalling", mcp.Description("List uninstalling releases")),
		mcp.WithString("failed", mcp.Description("List failed releases")),
		mcp.WithString("deployed", mcp.Description("List deployed releases")),
		mcp.WithString("pending", mcp.Description("List pending releases")),
		mcp.WithString("filter", mcp.Description("A regular expression to filter releases by")),
		mcp.WithString("output", mcp.Description("The output format (e.g., 'json', 'yaml', 'table')")),
	), telemetry.AdaptToolHandler(telemetry.WithTracing("helm_list_releases", handleHelmListReleases)))

	s.AddTool(mcp.NewTool("helm_get_release",
		mcp.WithDescription("Get extended information about a Helm release"),
		mcp.WithString("name", mcp.Description("The name of the release"), mcp.Required()),
		mcp.WithString("namespace", mcp.Description("The namespace of the release"), mcp.Required()),
		mcp.WithString("resource", mcp.Description("The resource to get (all, hooks, manifest, notes, values)")),
	), telemetry.AdaptToolHandler(telemetry.WithTracing("helm_get_release", handleHelmGetRelease)))

	s.AddTool(mcp.NewTool("helm_repo_update",
		mcp.WithDescription("Update information of available charts locally from chart repositories"),
	), telemetry.AdaptToolHandler(telemetry.WithTracing("helm_repo_update", handleHelmRepoUpdate)))

	// Write tools - only registered when not in read-only mode
	if !readOnly {
		s.AddTool(mcp.NewTool("helm_upgrade",
			mcp.WithDescription("Upgrade or install a Helm release"),
			mcp.WithString("name", mcp.Description("The name of the release"), mcp.Required()),
			mcp.WithString("chart", mcp.Description("The chart to install or upgrade to"), mcp.Required()),
			mcp.WithString("namespace", mcp.Description("The namespace of the release")),
			mcp.WithString("version", mcp.Description("The version of the chart to upgrade to")),
			mcp.WithString("values", mcp.Description("Path to a values file")),
			mcp.WithString("set", mcp.Description("Set values on the command line (e.g., 'key1=val1,key2=val2')")),
			mcp.WithString("install", mcp.Description("Run an install if the release is not present")),
			mcp.WithString("dry_run", mcp.Description("Simulate an upgrade")),
			mcp.WithString("wait", mcp.Description("Wait for the upgrade to complete")),
		), telemetry.AdaptToolHandler(telemetry.WithTracing("helm_upgrade", handleHelmUpgradeRelease)))

		s.AddTool(mcp.NewTool("helm_uninstall",
			mcp.WithDescription("Uninstall a Helm release"),
			mcp.WithString("name", mcp.Description("The name of the release to uninstall"), mcp.Required()),
			mcp.WithString("namespace", mcp.Description("The namespace of the release"), mcp.Required()),
			mcp.WithString("dry_run", mcp.Description("Simulate an uninstall")),
			mcp.WithString("wait", mcp.Description("Wait for the uninstall to complete")),
		), telemetry.AdaptToolHandler(telemetry.WithTracing("helm_uninstall", handleHelmUninstall)))

		s.AddTool(mcp.NewTool("helm_repo_add",
			mcp.WithDescription("Add a Helm repository"),
			mcp.WithString("name", mcp.Description("The name of the repository"), mcp.Required()),
			mcp.WithString("url", mcp.Description("The URL of the repository"), mcp.Required()),
		), telemetry.AdaptToolHandler(telemetry.WithTracing("helm_repo_add", handleHelmRepoAdd)))
	}
}
