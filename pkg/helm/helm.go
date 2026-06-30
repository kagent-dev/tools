package helm

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/kagent-dev/tools/internal/commands"
	"github.com/kagent-dev/tools/internal/errors"
	mcp "github.com/kagent-dev/tools/internal/mcp"
	"github.com/kagent-dev/tools/internal/security"
	"github.com/kagent-dev/tools/pkg/utils"
)

// toolErrorResult formats a ToolError as an MCP error result.
func toolErrorResult(toolErr *errors.ToolError) *mcp.CallToolResult {
	return toolErr.ToMCPResult()
}

type helmListReleasesInput struct {
	Namespace     string `json:"namespace" jsonschema:"The namespace to list releases from"`
	AllNamespaces bool   `json:"all_namespaces" jsonschema:"List releases from all namespaces"`
	All           bool   `json:"all" jsonschema:"Show all releases without any filter applied"`
	Uninstalled   bool   `json:"uninstalled" jsonschema:"List uninstalled releases"`
	Uninstalling  bool   `json:"uninstalling" jsonschema:"List uninstalling releases"`
	Failed        bool   `json:"failed" jsonschema:"List failed releases"`
	Deployed      bool   `json:"deployed" jsonschema:"List deployed releases"`
	Pending       bool   `json:"pending" jsonschema:"List pending releases"`
	Filter        string `json:"filter" jsonschema:"A regular expression to filter releases by"`
	Output        string `json:"output" jsonschema:"The output format (e.g., 'json', 'yaml', 'table')"`
}

// Helm list releases
func handleHelmListReleases(ctx context.Context, request *mcp.CallToolRequest, in helmListReleasesInput) (*mcp.CallToolResult, any, error) {
	args := []string{"list"}

	if in.Namespace != "" {
		args = append(args, "-n", in.Namespace)
	}

	if in.AllNamespaces {
		args = append(args, "-A")
	}

	if in.All {
		args = append(args, "-a")
	}

	if in.Uninstalled {
		args = append(args, "--uninstalled")
	}

	if in.Uninstalling {
		args = append(args, "--uninstalling")
	}

	if in.Failed {
		args = append(args, "--failed")
	}

	if in.Deployed {
		args = append(args, "--deployed")
	}

	if in.Pending {
		args = append(args, "--pending")
	}

	if in.Filter != "" {
		args = append(args, "-f", in.Filter)
	}

	if in.Output != "" {
		args = append(args, "-o", in.Output)
	}

	result, err := runHelmCommand(ctx, args)
	if err != nil {
		// Check if it's a structured error
		if toolErr, ok := err.(*errors.ToolError); ok {
			// Add namespace context if provided
			if in.Namespace != "" {
				toolErr = toolErr.WithContext("namespace", in.Namespace)
			}
			return toolErrorResult(toolErr), nil, nil
		}
		// Fallback for non-structured errors
		return mcp.NewToolResultError(fmt.Sprintf("Helm list command failed: %v", err)), nil, nil
	}

	return mcp.NewToolResultText(result), nil, nil
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

type helmGetReleaseInput struct {
	Name      string `json:"name" jsonschema:"The name of the release"`
	Namespace string `json:"namespace" jsonschema:"The namespace of the release"`
	Resource  string `json:"resource" jsonschema:"The resource to get (all, hooks, manifest, notes, values)"`
}

// Helm get release
func handleHelmGetRelease(ctx context.Context, request *mcp.CallToolRequest, in helmGetReleaseInput) (*mcp.CallToolResult, any, error) {
	if in.Resource == "" {
		in.Resource = "all"
	}

	if in.Name == "" {
		return mcp.NewToolResultError("name parameter is required"), nil, nil
	}

	if in.Namespace == "" {
		return mcp.NewToolResultError("namespace parameter is required"), nil, nil
	}

	args := []string{"get", in.Resource, in.Name, "-n", in.Namespace}

	result, err := runHelmCommand(ctx, args)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Helm get command failed: %v", err)), nil, nil
	}

	return mcp.NewToolResultText(result), nil, nil
}

type helmUpgradeReleaseInput struct {
	Name      string `json:"name" jsonschema:"The name of the release"`
	Chart     string `json:"chart" jsonschema:"The chart to install or upgrade to"`
	Namespace string `json:"namespace" jsonschema:"The namespace of the release"`
	Version   string `json:"version" jsonschema:"The version of the chart to upgrade to"`
	Values    string `json:"values" jsonschema:"Path to a values file"`
	Set       string `json:"set" jsonschema:"Set values on the command line (e.g., 'key1=val1,key2=val2')"`
	Install   bool   `json:"install" jsonschema:"Run an install if the release is not present"`
	DryRun    bool   `json:"dry_run" jsonschema:"Simulate an upgrade"`
	Wait      bool   `json:"wait" jsonschema:"Wait for the upgrade to complete"`
}

// Helm upgrade release
func handleHelmUpgradeRelease(ctx context.Context, request *mcp.CallToolRequest, in helmUpgradeReleaseInput) (*mcp.CallToolResult, any, error) {
	if in.Name == "" || in.Chart == "" {
		return mcp.NewToolResultError("name and chart parameters are required"), nil, nil
	}

	// Validate release name
	if err := security.ValidateHelmReleaseName(in.Name); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Invalid release name: %v", err)), nil, nil
	}

	// Validate namespace if provided
	if in.Namespace != "" {
		if err := security.ValidateNamespace(in.Namespace); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Invalid namespace: %v", err)), nil, nil
		}
	}

	// Validate values file path if provided
	if in.Values != "" {
		if err := security.ValidateFilePath(in.Values); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Invalid values file path: %v", err)), nil, nil
		}
	}

	args := []string{"upgrade", in.Name, in.Chart}

	if in.Namespace != "" {
		args = append(args, "-n", in.Namespace)
	}

	if in.Version != "" {
		args = append(args, "--version", in.Version)
	}

	if in.Values != "" {
		args = append(args, "-f", in.Values)
	}

	if in.Set != "" {
		// Split multiple set values by comma
		setValuesList := strings.Split(in.Set, ",")
		for _, setValue := range setValuesList {
			args = append(args, "--set", strings.TrimSpace(setValue))
		}
	}

	if in.Install {
		args = append(args, "--install")
	}

	if in.DryRun {
		args = append(args, "--dry-run")
	}

	if in.Wait {
		args = append(args, "--wait")
	}

	result, err := runHelmCommand(ctx, args)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Helm upgrade command failed: %v", err)), nil, nil
	}

	return mcp.NewToolResultText(result), nil, nil
}

type helmUninstallInput struct {
	Name      string `json:"name" jsonschema:"The name of the release to uninstall"`
	Namespace string `json:"namespace" jsonschema:"The namespace of the release"`
	DryRun    bool   `json:"dry_run" jsonschema:"Simulate an uninstall"`
	Wait      bool   `json:"wait" jsonschema:"Wait for the uninstall to complete"`
}

// Helm uninstall release
func handleHelmUninstall(ctx context.Context, request *mcp.CallToolRequest, in helmUninstallInput) (*mcp.CallToolResult, any, error) {
	if in.Name == "" || in.Namespace == "" {
		return mcp.NewToolResultError("name and namespace parameters are required"), nil, nil
	}

	args := []string{"uninstall", in.Name, "-n", in.Namespace}

	if in.DryRun {
		args = append(args, "--dry-run")
	}

	if in.Wait {
		args = append(args, "--wait")
	}

	result, err := runHelmCommand(ctx, args)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Helm uninstall command failed: %v", err)), nil, nil
	}

	return mcp.NewToolResultText(result), nil, nil
}

type helmRepoAddInput struct {
	Name string `json:"name" jsonschema:"The name of the repository"`
	URL  string `json:"url" jsonschema:"The URL of the repository"`
}

// Helm repo add
func handleHelmRepoAdd(ctx context.Context, request *mcp.CallToolRequest, in helmRepoAddInput) (*mcp.CallToolResult, any, error) {
	if in.Name == "" || in.URL == "" {
		return mcp.NewToolResultError("name and url parameters are required"), nil, nil
	}

	// Validate repository name
	if err := security.ValidateHelmReleaseName(in.Name); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Invalid repository name: %v", err)), nil, nil
	}

	// Validate repository URL
	if err := security.ValidateURL(in.URL); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Invalid repository URL: %v", err)), nil, nil
	}

	args := []string{"repo", "add", in.Name, in.URL}

	result, err := runHelmCommand(ctx, args)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Helm repo add command failed: %v", err)), nil, nil
	}

	return mcp.NewToolResultText(result), nil, nil
}

type helmRepoUpdateInput struct{}

// Helm repo update
func handleHelmRepoUpdate(ctx context.Context, request *mcp.CallToolRequest, in helmRepoUpdateInput) (*mcp.CallToolResult, any, error) {
	args := []string{"repo", "update"}

	result, err := runHelmCommand(ctx, args)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Helm repo update command failed: %v", err)), nil, nil
	}

	return mcp.NewToolResultText(result), nil, nil
}

// Register Helm tools
func RegisterTools(s *mcp.Server, readOnly bool) {
	// Read-only tools - always registered
	mcp.AddTool(s, "helm", &mcp.Tool{
		Name:        "helm_list_releases",
		Description: "List Helm releases in a namespace",
	}, handleHelmListReleases)

	mcp.AddTool(s, "helm", &mcp.Tool{
		Name:        "helm_get_release",
		Description: "Get extended information about a Helm release",
	}, handleHelmGetRelease)

	mcp.AddTool(s, "helm", &mcp.Tool{
		Name:        "helm_repo_update",
		Description: "Update information of available charts locally from chart repositories",
	}, handleHelmRepoUpdate)

	// Write tools - only registered when not in read-only mode
	if !readOnly {
		mcp.AddTool(s, "helm", &mcp.Tool{
			Name:        "helm_upgrade",
			Description: "Upgrade or install a Helm release",
		}, handleHelmUpgradeRelease)

		mcp.AddTool(s, "helm", &mcp.Tool{
			Name:        "helm_uninstall",
			Description: "Uninstall a Helm release",
		}, handleHelmUninstall)

		mcp.AddTool(s, "helm", &mcp.Tool{
			Name:        "helm_repo_add",
			Description: "Add a Helm repository",
		}, handleHelmRepoAdd)
	}
}
