package helm

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/kagent-dev/tools/internal/commands"
	"github.com/kagent-dev/tools/internal/errors"
	"github.com/kagent-dev/tools/internal/logger"
	"github.com/kagent-dev/tools/internal/security"
	"github.com/kagent-dev/tools/pkg/utils"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Helm list releases
func handleHelmListReleases(ctx context.Context, request *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var args map[string]interface{}
	if err := json.Unmarshal(request.Params.Arguments, &args); err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "failed to parse arguments"}},
			IsError: true,
		}, nil
	}

	namespace := ""
	if ns, ok := args["namespace"].(string); ok {
		namespace = ns
	}

	allNamespaces := false
	if allNs, ok := args["all_namespaces"].(string); ok {
		allNamespaces = allNs == "true"
	}

	all := false
	if allArg, ok := args["all"].(string); ok {
		all = allArg == "true"
	}

	uninstalled := false
	if uninst, ok := args["uninstalled"].(string); ok {
		uninstalled = uninst == "true"
	}

	uninstalling := false
	if uninsting, ok := args["uninstalling"].(string); ok {
		uninstalling = uninsting == "true"
	}

	failed := false
	if failedArg, ok := args["failed"].(string); ok {
		failed = failedArg == "true"
	}

	deployed := false
	if deployedArg, ok := args["deployed"].(string); ok {
		deployed = deployedArg == "true"
	}

	pending := false
	if pendingArg, ok := args["pending"].(string); ok {
		pending = pendingArg == "true"
	}

	filter := ""
	if filterArg, ok := args["filter"].(string); ok {
		filter = filterArg
	}

	output := ""
	if outputArg, ok := args["output"].(string); ok {
		output = outputArg
	}

	cmdArgs := []string{"list"}

	if namespace != "" {
		cmdArgs = append(cmdArgs, "-n", namespace)
	}

	if allNamespaces {
		cmdArgs = append(cmdArgs, "-A")
	}

	if all {
		cmdArgs = append(cmdArgs, "-a")
	}

	if uninstalled {
		cmdArgs = append(cmdArgs, "--uninstalled")
	}

	if uninstalling {
		cmdArgs = append(cmdArgs, "--uninstalling")
	}

	if failed {
		cmdArgs = append(cmdArgs, "--failed")
	}

	if deployed {
		cmdArgs = append(cmdArgs, "--deployed")
	}

	if pending {
		cmdArgs = append(cmdArgs, "--pending")
	}

	if filter != "" {
		cmdArgs = append(cmdArgs, "-f", filter)
	}

	if output != "" {
		cmdArgs = append(cmdArgs, "-o", output)
	}

	result, err := runHelmCommand(ctx, cmdArgs)
	if err != nil {
		// Check if it's a structured error
		if toolErr, ok := err.(*errors.ToolError); ok {
			// Add namespace context if provided
			if namespace != "" {
				toolErr = toolErr.WithContext("namespace", namespace)
			}
			return &mcp.CallToolResult{
				Content: []mcp.Content{&mcp.TextContent{Text: toolErr.Error()}},
				IsError: true,
			}, nil
		}
		// Fallback for non-structured errors
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("Helm list command failed: %v", err)}},
			IsError: true,
		}, nil
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: result}},
	}, nil
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
func handleHelmGetRelease(ctx context.Context, request *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var args map[string]interface{}
	if err := json.Unmarshal(request.Params.Arguments, &args); err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "failed to parse arguments"}},
			IsError: true,
		}, nil
	}

	name, ok := args["name"].(string)
	if !ok || name == "" {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "name parameter is required"}},
			IsError: true,
		}, nil
	}

	namespace, ok := args["namespace"].(string)
	if !ok || namespace == "" {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "namespace parameter is required"}},
			IsError: true,
		}, nil
	}

	resource := "all"
	if res, ok := args["resource"].(string); ok && res != "" {
		resource = res
	}

	cmdArgs := []string{"get", resource, name, "-n", namespace}

	result, err := runHelmCommand(ctx, cmdArgs)
	if err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("Helm get command failed: %v", err)}},
			IsError: true,
		}, nil
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: result}},
	}, nil
}

// Helm upgrade release
func handleHelmUpgradeRelease(ctx context.Context, request *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var args map[string]interface{}
	if err := json.Unmarshal(request.Params.Arguments, &args); err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "failed to parse arguments"}},
			IsError: true,
		}, nil
	}

	name, nameOk := args["name"].(string)
	chart, chartOk := args["chart"].(string)
	if !nameOk || name == "" || !chartOk || chart == "" {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "name and chart parameters are required"}},
			IsError: true,
		}, nil
	}

	// Validate release name
	if err := security.ValidateHelmReleaseName(name); err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("Invalid release name: %v", err)}},
			IsError: true,
		}, nil
	}

	namespace := ""
	if ns, ok := args["namespace"].(string); ok {
		namespace = ns
	}

	// Validate namespace if provided
	if namespace != "" {
		if err := security.ValidateNamespace(namespace); err != nil {
			return &mcp.CallToolResult{
				Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("Invalid namespace: %v", err)}},
				IsError: true,
			}, nil
		}
	}

	version := ""
	if ver, ok := args["version"].(string); ok {
		version = ver
	}

	values := ""
	if val, ok := args["values"].(string); ok {
		values = val
	}

	// Validate values file path if provided
	if values != "" {
		if err := security.ValidateFilePath(values); err != nil {
			return &mcp.CallToolResult{
				Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("Invalid values file path: %v", err)}},
				IsError: true,
			}, nil
		}
	}

	setValues := ""
	if set, ok := args["set"].(string); ok {
		setValues = set
	}

	install := false
	if inst, ok := args["install"].(string); ok {
		install = inst == "true"
	}

	dryRun := false
	if dry, ok := args["dry_run"].(string); ok {
		dryRun = dry == "true"
	}

	wait := false
	if waitArg, ok := args["wait"].(string); ok {
		wait = waitArg == "true"
	}

	cmdArgs := []string{"upgrade", name, chart}

	if namespace != "" {
		cmdArgs = append(cmdArgs, "-n", namespace)
	}

	if version != "" {
		cmdArgs = append(cmdArgs, "--version", version)
	}

	if values != "" {
		cmdArgs = append(cmdArgs, "-f", values)
	}

	if setValues != "" {
		// Split multiple set values by comma
		setValuesList := strings.Split(setValues, ",")
		for _, setValue := range setValuesList {
			cmdArgs = append(cmdArgs, "--set", strings.TrimSpace(setValue))
		}
	}

	if install {
		cmdArgs = append(cmdArgs, "--install")
	}

	if dryRun {
		cmdArgs = append(cmdArgs, "--dry-run")
	}

	if wait {
		cmdArgs = append(cmdArgs, "--wait")
	}

	result, err := runHelmCommand(ctx, cmdArgs)
	if err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("Helm upgrade command failed: %v", err)}},
			IsError: true,
		}, nil
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: result}},
	}, nil
}

// Helm uninstall release
func handleHelmUninstall(ctx context.Context, request *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var args map[string]interface{}
	if err := json.Unmarshal(request.Params.Arguments, &args); err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "failed to parse arguments"}},
			IsError: true,
		}, nil
	}

	name, nameOk := args["name"].(string)
	namespace, nsOk := args["namespace"].(string)
	if !nameOk || name == "" || !nsOk || namespace == "" {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "name and namespace parameters are required"}},
			IsError: true,
		}, nil
	}

	dryRun := false
	if dry, ok := args["dry_run"].(string); ok {
		dryRun = dry == "true"
	}

	wait := false
	if waitArg, ok := args["wait"].(string); ok {
		wait = waitArg == "true"
	}

	cmdArgs := []string{"uninstall", name, "-n", namespace}

	if dryRun {
		cmdArgs = append(cmdArgs, "--dry-run")
	}

	if wait {
		cmdArgs = append(cmdArgs, "--wait")
	}

	result, err := runHelmCommand(ctx, cmdArgs)
	if err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("Helm uninstall command failed: %v", err)}},
			IsError: true,
		}, nil
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: result}},
	}, nil
}

// Helm repo add
func handleHelmRepoAdd(ctx context.Context, request *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var args map[string]interface{}
	if err := json.Unmarshal(request.Params.Arguments, &args); err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "failed to parse arguments"}},
			IsError: true,
		}, nil
	}

	name, nameOk := args["name"].(string)
	url, urlOk := args["url"].(string)
	if !nameOk || name == "" || !urlOk || url == "" {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "name and url parameters are required"}},
			IsError: true,
		}, nil
	}

	// Validate repository name
	if err := security.ValidateHelmReleaseName(name); err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("Invalid repository name: %v", err)}},
			IsError: true,
		}, nil
	}

	// Validate repository URL
	if err := security.ValidateURL(url); err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("Invalid repository URL: %v", err)}},
			IsError: true,
		}, nil
	}

	cmdArgs := []string{"repo", "add", name, url}

	result, err := runHelmCommand(ctx, cmdArgs)
	if err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("Helm repo add command failed: %v", err)}},
			IsError: true,
		}, nil
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: result}},
	}, nil
}

// Helm repo update
func handleHelmRepoUpdate(ctx context.Context, request *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	cmdArgs := []string{"repo", "update"}

	result, err := runHelmCommand(ctx, cmdArgs)
	if err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("Helm repo update command failed: %v", err)}},
			IsError: true,
		}, nil
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: result}},
	}, nil
}

// Helm template
func handleHelmTemplate(ctx context.Context, request *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var args map[string]interface{}
	if err := json.Unmarshal(request.Params.Arguments, &args); err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "failed to parse arguments"}},
			IsError: true,
		}, nil
	}

	name, nameOk := args["name"].(string)
	chart, chartOk := args["chart"].(string)
	if !nameOk || name == "" || !chartOk || chart == "" {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "name and chart parameters are required"}},
			IsError: true,
		}, nil
	}

	// Validate release name
	if err := security.ValidateHelmReleaseName(name); err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("Invalid release name: %v", err)}},
			IsError: true,
		}, nil
	}

	namespace := ""
	if ns, ok := args["namespace"].(string); ok {
		namespace = ns
	}

	// Validate namespace if provided
	if namespace != "" {
		if err := security.ValidateNamespace(namespace); err != nil {
			return &mcp.CallToolResult{
				Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("Invalid namespace: %v", err)}},
				IsError: true,
			}, nil
		}
	}

	version := ""
	if ver, ok := args["version"].(string); ok {
		version = ver
	}

	values := ""
	if val, ok := args["values"].(string); ok {
		values = val
	}

	// Validate values file path if provided
	if values != "" {
		if err := security.ValidateFilePath(values); err != nil {
			return &mcp.CallToolResult{
				Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("Invalid values file path: %v", err)}},
				IsError: true,
			}, nil
		}
	}

	setValues := ""
	if set, ok := args["set"].(string); ok {
		setValues = set
	}

	cmdArgs := []string{"template", name, chart}

	if namespace != "" {
		cmdArgs = append(cmdArgs, "-n", namespace)
	}

	if version != "" {
		cmdArgs = append(cmdArgs, "--version", version)
	}

	if values != "" {
		cmdArgs = append(cmdArgs, "-f", values)
	}

	if setValues != "" {
		// Split multiple set values by comma
		setValuesList := strings.Split(setValues, ",")
		for _, setValue := range setValuesList {
			cmdArgs = append(cmdArgs, "--set", strings.TrimSpace(setValue))
		}
	}

	result, err := runHelmCommand(ctx, cmdArgs)
	if err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("Helm template command failed: %v", err)}},
			IsError: true,
		}, nil
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: result}},
	}, nil
}

// Register Helm tools
func RegisterTools(s *mcp.Server) error {
	logger.Get().Info("RegisterTools initialized")
	// Register helm_list_releases tool
	s.AddTool(&mcp.Tool{
		Name:        "helm_list_releases",
		Description: "List Helm releases in a namespace",
		InputSchema: &jsonschema.Schema{
			Type: "object",
			Properties: map[string]*jsonschema.Schema{
				"namespace": {
					Type:        "string",
					Description: "The namespace to list releases from",
				},
				"all_namespaces": {
					Type:        "string",
					Description: "List releases from all namespaces",
				},
				"all": {
					Type:        "string",
					Description: "Show all releases without any filter applied",
				},
				"uninstalled": {
					Type:        "string",
					Description: "List uninstalled releases",
				},
				"uninstalling": {
					Type:        "string",
					Description: "List uninstalling releases",
				},
				"failed": {
					Type:        "string",
					Description: "List failed releases",
				},
				"deployed": {
					Type:        "string",
					Description: "List deployed releases",
				},
				"pending": {
					Type:        "string",
					Description: "List pending releases",
				},
				"filter": {
					Type:        "string",
					Description: "A regular expression to filter releases by",
				},
				"output": {
					Type:        "string",
					Description: "The output format (e.g., 'json', 'yaml', 'table')",
				},
			},
		},
	}, handleHelmListReleases)

	// Register helm_get_release tool
	s.AddTool(&mcp.Tool{
		Name:        "helm_get_release",
		Description: "Get extended information about a Helm release",
		InputSchema: &jsonschema.Schema{
			Type: "object",
			Properties: map[string]*jsonschema.Schema{
				"name": {
					Type:        "string",
					Description: "The name of the release",
				},
				"namespace": {
					Type:        "string",
					Description: "The namespace of the release",
				},
				"resource": {
					Type:        "string",
					Description: "The resource to get (all, hooks, manifest, notes, values)",
				},
			},
			Required: []string{"name", "namespace"},
		},
	}, handleHelmGetRelease)

	// Register helm_upgrade tool
	s.AddTool(&mcp.Tool{
		Name:        "helm_upgrade",
		Description: "Upgrade or install a Helm release",
		InputSchema: &jsonschema.Schema{
			Type: "object",
			Properties: map[string]*jsonschema.Schema{
				"name": {
					Type:        "string",
					Description: "The name of the release",
				},
				"chart": {
					Type:        "string",
					Description: "The chart to install or upgrade to",
				},
				"namespace": {
					Type:        "string",
					Description: "The namespace of the release",
				},
				"version": {
					Type:        "string",
					Description: "The version of the chart to upgrade to",
				},
				"values": {
					Type:        "string",
					Description: "Path to a values file",
				},
				"set": {
					Type:        "string",
					Description: "Set values on the command line (e.g., 'key1=val1,key2=val2')",
				},
				"install": {
					Type:        "string",
					Description: "Run an install if the release is not present",
				},
				"dry_run": {
					Type:        "string",
					Description: "Simulate an upgrade",
				},
				"wait": {
					Type:        "string",
					Description: "Wait for the upgrade to complete",
				},
			},
			Required: []string{"name", "chart"},
		},
	}, handleHelmUpgradeRelease)

	// Register helm_uninstall tool
	s.AddTool(&mcp.Tool{
		Name:        "helm_uninstall",
		Description: "Uninstall a Helm release",
		InputSchema: &jsonschema.Schema{
			Type: "object",
			Properties: map[string]*jsonschema.Schema{
				"name": {
					Type:        "string",
					Description: "The name of the release to uninstall",
				},
				"namespace": {
					Type:        "string",
					Description: "The namespace of the release",
				},
				"dry_run": {
					Type:        "string",
					Description: "Simulate an uninstall",
				},
				"wait": {
					Type:        "string",
					Description: "Wait for the uninstall to complete",
				},
			},
			Required: []string{"name", "namespace"},
		},
	}, handleHelmUninstall)

	// Register helm_repo_add tool
	s.AddTool(&mcp.Tool{
		Name:        "helm_repo_add",
		Description: "Add a Helm repository",
		InputSchema: &jsonschema.Schema{
			Type: "object",
			Properties: map[string]*jsonschema.Schema{
				"name": {
					Type:        "string",
					Description: "The name of the repository",
				},
				"url": {
					Type:        "string",
					Description: "The URL of the repository",
				},
			},
			Required: []string{"name", "url"},
		},
	}, handleHelmRepoAdd)

	// Register helm_repo_update tool
	s.AddTool(&mcp.Tool{
		Name:        "helm_repo_update",
		Description: "Update information of available charts locally from chart repositories",
		InputSchema: &jsonschema.Schema{
			Type: "object",
		},
	}, handleHelmRepoUpdate)

	// Register helm_template tool
	s.AddTool(&mcp.Tool{
		Name:        "helm_template",
		Description: "Render Helm chart templates locally",
		InputSchema: &jsonschema.Schema{
			Type: "object",
			Properties: map[string]*jsonschema.Schema{
				"name": {
					Type:        "string",
					Description: "The name of the release",
				},
				"chart": {
					Type:        "string",
					Description: "The chart to template",
				},
				"namespace": {
					Type:        "string",
					Description: "The namespace of the release",
				},
				"version": {
					Type:        "string",
					Description: "The version of the chart to template",
				},
				"set": {
					Type:        "string",
					Description: "Set values on the command line (e.g., 'key1=val1,key2=val2')",
				},
			},
			Required: []string{"name", "chart"},
		},
	}, handleHelmTemplate)

	return nil
}
