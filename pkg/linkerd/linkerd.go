package linkerd

import (
	"context"
	goerrors "errors"
	"fmt"
	"os"
	"strings"

	"github.com/kagent-dev/tools/internal/commands"
	toolerrors "github.com/kagent-dev/tools/internal/errors"
	"github.com/kagent-dev/tools/internal/security"
	"github.com/kagent-dev/tools/internal/telemetry"
	"github.com/kagent-dev/tools/pkg/utils"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

const (
	linkerdInjectionAnnotationKey      = "linkerd.io/inject"
	linkerdInjectionAnnotationJSONPath = "/spec/template/metadata/annotations/linkerd.io~1inject"
)

func runLinkerdCommand(ctx context.Context, args []string) (string, error) {
	return runLinkerdCommandWithOptions(ctx, args, false)
}

func runLinkerdManifestCommand(ctx context.Context, args []string) (string, error) {
	return runLinkerdCommandWithOptions(ctx, args, true)
}

func runLinkerdCommandWithOptions(ctx context.Context, args []string, stdoutOnly bool) (string, error) {
	kubeconfigPath := utils.GetKubeconfig()
	builder := commands.NewCommandBuilder("linkerd").
		WithArgs(args...).
		WithKubeconfig(kubeconfigPath)

	if stdoutOnly {
		builder = builder.WithStdoutOnly(true)
	}

	return builder.Execute(ctx)
}

func applyManifest(ctx context.Context, manifest string) (string, error) {
	return runKubectlManifestCommand(ctx, "apply", manifest)
}

func deleteManifest(ctx context.Context, manifest string) (string, error) {
	return runKubectlManifestCommand(ctx, "delete", manifest)
}

func runKubectlManifestCommand(ctx context.Context, action, manifest string) (string, error) {
	manifestPath, err := writeManifestToTempFile(manifest)
	if err != nil {
		return "", fmt.Errorf("failed to prepare manifest for kubectl %s: %w", action, err)
	}
	defer os.Remove(manifestPath)

	return runKubectlCommand(ctx, []string{action, "-f", manifestPath})
}

func runKubectlCommand(ctx context.Context, args []string) (string, error) {
	kubeconfigPath := utils.GetKubeconfig()
	return commands.NewCommandBuilder("kubectl").
		WithArgs(args...).
		WithKubeconfig(kubeconfigPath).
		Execute(ctx)
}

func writeManifestToTempFile(manifest string) (string, error) {
	tempFile, err := os.CreateTemp("", "linkerd-manifest-*.yaml")
	if err != nil {
		return "", fmt.Errorf("failed to create temporary manifest file: %w", err)
	}

	if _, err := tempFile.WriteString(manifest); err != nil {
		tempFile.Close()
		os.Remove(tempFile.Name())
		return "", fmt.Errorf("failed to write manifest to temporary file: %w", err)
	}

	if err := tempFile.Close(); err != nil {
		os.Remove(tempFile.Name())
		return "", fmt.Errorf("failed to close temporary manifest file: %w", err)
	}

	return tempFile.Name(), nil
}

func formatLinkerdCommandResult(operation string, output string, err error) (*mcp.CallToolResult, error) {
	if err == nil {
		return mcp.NewToolResultText(output), nil
	}

	trimmedOutput := strings.TrimSpace(output)
	failureMessage := fmt.Sprintf("%s failed: %v", operation, err)
	if trimmedOutput != "" {
		failureMessage = fmt.Sprintf("%s\n\n%s", failureMessage, trimmedOutput)
	}

	var toolErr *toolerrors.ToolError
	if goerrors.As(err, &toolErr) {
		toolErr = toolErr.WithContext("kagent_operation", operation)
		if trimmedOutput != "" {
			toolErr = toolErr.WithContext("command_output", trimmedOutput)
		}
		return toolErr.ToMCPResult(), nil
	}

	return mcp.NewToolResultError(failureMessage), nil
}

// Linkerd check
func handleLinkerdCheck(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	namespace := mcp.ParseString(request, "namespace", "")
	preCheck := mcp.ParseString(request, "pre_check", "") == "true"
	proxyCheck := mcp.ParseString(request, "proxy_check", "") == "true"
	waitDuration := mcp.ParseString(request, "wait", "")
	output := mcp.ParseString(request, "output", "")

	args := []string{"check"}

	if preCheck {
		args = append(args, "--pre")
	}

	if proxyCheck {
		args = append(args, "--proxy")
	}

	if namespace != "" {
		args = append(args, "-n", namespace)
	}

	if waitDuration != "" {
		args = append(args, "--wait", waitDuration)
	}

	if output != "" {
		args = append(args, "--output", output)
	}

	result, err := runLinkerdCommand(ctx, args)
	return formatLinkerdCommandResult("linkerd check", result, err)

}

// Linkerd install
func handleLinkerdInstall(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	ha := mcp.ParseString(request, "ha", "") == "true"
	crdsOnly := mcp.ParseString(request, "crds_only", "") == "true"
	skipChecks := mcp.ParseString(request, "skip_checks", "") == "true"
	identityTrustAnchors := mcp.ParseString(request, "identity_trust_anchors_pem", "")
	setOverrides := mcp.ParseString(request, "set_overrides", "")

	args := []string{"install"}

	if ha {
		args = append(args, "--ha")
	}

	if skipChecks {
		args = append(args, "--skip-checks")
	}

	if identityTrustAnchors != "" {
		args = append(args, "--identity-trust-anchors-pem", identityTrustAnchors)
	}

	args = appendSetOverrides(args, setOverrides)

	crdArgs := append([]string{}, args...)
	crdArgs = append(crdArgs, "--crds")

	if crdsOnly {
		manifest, err := runLinkerdManifestCommand(ctx, crdArgs)
		if err != nil {
			return formatLinkerdCommandResult("linkerd install --crds", manifest, err)
		}

		applyResult, applyErr := applyManifest(ctx, manifest)
		return formatLinkerdCommandResult("kubectl apply linkerd install CRDs manifest", applyResult, applyErr)
	}

	var combinedOutput strings.Builder

	crdManifest, err := runLinkerdManifestCommand(ctx, crdArgs)
	if err != nil {
		return formatLinkerdCommandResult("linkerd install --crds", crdManifest, err)
	}

	crdApplyResult, crdApplyErr := applyManifest(ctx, crdManifest)
	if crdApplyErr != nil {
		return formatLinkerdCommandResult("kubectl apply linkerd install CRDs manifest", crdApplyResult, crdApplyErr)
	}

	if trimmed := strings.TrimSpace(crdApplyResult); trimmed != "" {
		combinedOutput.WriteString("Linkerd CRDs applied:\n")
		combinedOutput.WriteString(trimmed)
		combinedOutput.WriteString("\n\n")
	}

	manifest, err := runLinkerdManifestCommand(ctx, args)
	if err != nil {
		return formatLinkerdCommandResult("linkerd install", manifest, err)
	}

	applyResult, applyErr := applyManifest(ctx, manifest)

	finalOutput := applyResult
	if combinedOutput.Len() > 0 {
		trimmedApply := strings.TrimSpace(applyResult)
		if trimmedApply != "" {
			combinedOutput.WriteString("Linkerd control plane applied:\n")
			combinedOutput.WriteString(trimmedApply)
		} else if applyErr != nil {
			combinedOutput.WriteString("Linkerd control plane apply failed (no output captured)")
		} else {
			combinedOutput.WriteString("Linkerd control plane applied.")
		}
		finalOutput = combinedOutput.String()
	}

	return formatLinkerdCommandResult("kubectl apply linkerd install manifest", finalOutput, applyErr)

}

// Linkerd workload injection patch
func handleLinkerdWorkloadInjection(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	workloadName := mcp.ParseString(request, "workload_name", "")
	if workloadName == "" {
		return mcp.NewToolResultError("workload_name parameter is required"), nil
	}

	if err := security.ValidateK8sResourceName(workloadName); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("invalid workload name: %v", err)), nil
	}

	namespace := mcp.ParseString(request, "namespace", "default")
	if err := security.ValidateNamespace(namespace); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("invalid namespace: %v", err)), nil
	}

	workloadType := strings.ToLower(mcp.ParseString(request, "workload_type", "deployment"))
	switch workloadType {
	case "deployment", "statefulset", "daemonset":
	default:
		return mcp.NewToolResultError("workload_type must be deployment, statefulset, or daemonset"), nil
	}

	removeAnnotation := mcp.ParseString(request, "remove_annotation", "") == "true"
	injectState := strings.ToLower(mcp.ParseString(request, "inject_state", "disabled"))
	if !removeAnnotation {
		switch injectState {
		case "enabled", "disabled", "ingress":
		default:
			return mcp.NewToolResultError("inject_state must be enabled, disabled, or ingress"), nil
		}
	}

	args := []string{"patch", workloadType, workloadName, "-n", namespace}
	var patch string
	operation := fmt.Sprintf("kubectl patch %s %s for linkerd injection", workloadType, workloadName)
	if removeAnnotation {
		patch = fmt.Sprintf(`[{"op":"remove","path":"%s"}]`, linkerdInjectionAnnotationJSONPath)
		args = append(args, "--type=json", "-p", patch)
		operation = fmt.Sprintf("kubectl remove linkerd injection annotation from %s %s", workloadType, workloadName)
	} else {
		patch = fmt.Sprintf(`{"spec":{"template":{"metadata":{"annotations":{"%s":"%s"}}}}}`, linkerdInjectionAnnotationKey, injectState)
		args = append(args, "-p", patch)
		operation = fmt.Sprintf("kubectl set linkerd injection=%s on %s %s", injectState, workloadType, workloadName)
	}

	result, err := runKubectlCommand(ctx, args)
	return formatLinkerdCommandResult(operation, result, err)
}

// Linkerd install CNI
func handleLinkerdInstallCNI(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	skipChecks := mcp.ParseString(request, "skip_checks", "") == "true"
	setOverrides := mcp.ParseString(request, "set_overrides", "")

	args := []string{"install-cni"}

	if skipChecks {
		args = append(args, "--skip-checks")
	}

	args = appendSetOverrides(args, setOverrides)

	manifest, err := runLinkerdManifestCommand(ctx, args)
	if err != nil {
		return formatLinkerdCommandResult("linkerd install-cni", manifest, err)
	}

	applyResult, applyErr := applyManifest(ctx, manifest)
	return formatLinkerdCommandResult("kubectl apply linkerd install-cni manifest", applyResult, applyErr)

}

// Linkerd upgrade
func handleLinkerdUpgrade(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	ha := mcp.ParseString(request, "ha", "") == "true"
	crdsOnly := mcp.ParseString(request, "crds_only", "") == "true"
	skipChecks := mcp.ParseString(request, "skip_checks", "") == "true"
	setOverrides := mcp.ParseString(request, "set_overrides", "")

	args := []string{"upgrade"}

	if crdsOnly {
		args = append(args, "--crds")
	}

	if ha {
		args = append(args, "--ha")
	}

	if skipChecks {
		args = append(args, "--skip-checks")
	}

	args = appendSetOverrides(args, setOverrides)

	manifest, err := runLinkerdManifestCommand(ctx, args)
	if err != nil {
		return formatLinkerdCommandResult("linkerd upgrade", manifest, err)
	}

	applyResult, applyErr := applyManifest(ctx, manifest)
	return formatLinkerdCommandResult("kubectl apply linkerd upgrade manifest", applyResult, applyErr)

}

// Linkerd uninstall
func handleLinkerdUninstall(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	force := mcp.ParseString(request, "force", "") == "true"

	args := []string{"uninstall"}

	if force {
		args = append(args, "--force")
	}

	manifest, err := runLinkerdManifestCommand(ctx, args)
	if err != nil {
		return formatLinkerdCommandResult("linkerd uninstall", manifest, err)
	}

	deleteResult, deleteErr := deleteManifest(ctx, manifest)
	return formatLinkerdCommandResult("kubectl delete linkerd uninstall manifest", deleteResult, deleteErr)

}

// Linkerd version
func handleLinkerdVersion(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	clientOnly := mcp.ParseString(request, "client_only", "") == "true"
	short := mcp.ParseString(request, "short", "") == "true"

	args := []string{"version"}

	if clientOnly {
		args = append(args, "--client")
	}

	if short {
		args = append(args, "--short")
	}

	result, err := runLinkerdCommand(ctx, args)
	return formatLinkerdCommandResult("linkerd version", result, err)

}

// Linkerd authz
func handleLinkerdAuthz(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	resource := mcp.ParseString(request, "resource", "")
	if resource == "" {
		return mcp.NewToolResultError("resource parameter is required"), nil
	}

	namespace := mcp.ParseString(request, "namespace", "")

	args := []string{"authz"}

	if namespace != "" {
		args = append(args, "-n", namespace)
	}

	args = append(args, resource)

	result, err := runLinkerdCommand(ctx, args)
	return formatLinkerdCommandResult("linkerd authz", result, err)

}

// Linkerd stat
func handleLinkerdStat(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	resource := mcp.ParseString(request, "resource", "")
	if resource == "" {
		return mcp.NewToolResultError("resource parameter is required"), nil
	}

	namespace := mcp.ParseString(request, "namespace", "")
	allNamespaces := mcp.ParseString(request, "all_namespaces", "") == "true"
	from := mcp.ParseString(request, "from", "")
	to := mcp.ParseString(request, "to", "")
	timeWindow := mcp.ParseString(request, "time_window", "")
	output := mcp.ParseString(request, "output", "")

	args := []string{"stat", resource}

	if allNamespaces {
		args = append(args, "-A")
	} else if namespace != "" {
		args = append(args, "-n", namespace)
	}

	if from != "" {
		args = append(args, "--from", from)
	}

	if to != "" {
		args = append(args, "--to", to)
	}

	if timeWindow != "" {
		args = append(args, "--time-window", timeWindow)
	}

	if output != "" {
		args = append(args, "-o", output)
	}

	result, err := runLinkerdCommand(ctx, args)
	return formatLinkerdCommandResult("linkerd stat", result, err)

}

// Linkerd top
func handleLinkerdTop(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	resource := mcp.ParseString(request, "resource", "")
	if resource == "" {
		return mcp.NewToolResultError("resource parameter is required"), nil
	}

	namespace := mcp.ParseString(request, "namespace", "")
	from := mcp.ParseString(request, "from", "")
	to := mcp.ParseString(request, "to", "")
	maxRows := mcp.ParseString(request, "max_results", "")
	timeWindow := mcp.ParseString(request, "time_window", "")

	args := []string{"top", resource}

	if namespace != "" {
		args = append(args, "-n", namespace)
	}

	if from != "" {
		args = append(args, "--from", from)
	}

	if to != "" {
		args = append(args, "--to", to)
	}

	if maxRows != "" {
		args = append(args, "--max", maxRows)
	}

	if timeWindow != "" {
		args = append(args, "--time-window", timeWindow)
	}

	result, err := runLinkerdCommand(ctx, args)
	return formatLinkerdCommandResult("linkerd top", result, err)

}

// Linkerd edges
func handleLinkerdEdges(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	resource := mcp.ParseString(request, "resource", "")
	if resource == "" {
		return mcp.NewToolResultError("resource parameter is required"), nil
	}

	namespace := mcp.ParseString(request, "namespace", "")
	allNamespaces := mcp.ParseString(request, "all_namespaces", "") == "true"
	output := mcp.ParseString(request, "output", "")

	args := []string{"edges", resource}

	if allNamespaces {
		args = append(args, "-A")
	} else if namespace != "" {
		args = append(args, "-n", namespace)
	}

	if output != "" {
		args = append(args, "-o", output)
	}

	result, err := runLinkerdCommand(ctx, args)
	return formatLinkerdCommandResult("linkerd edges", result, err)

}

// Linkerd routes
func handleLinkerdRoutes(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	resource := mcp.ParseString(request, "resource", "")
	if resource == "" {
		return mcp.NewToolResultError("resource parameter is required"), nil
	}

	namespace := mcp.ParseString(request, "namespace", "")
	output := mcp.ParseString(request, "output", "")
	from := mcp.ParseString(request, "from", "")
	to := mcp.ParseString(request, "to", "")

	args := []string{"routes", resource}

	if namespace != "" {
		args = append(args, "-n", namespace)
	}

	if from != "" {
		args = append(args, "--from", from)
	}

	if to != "" {
		args = append(args, "--to", to)
	}

	if output != "" {
		args = append(args, "-o", output)
	}

	result, err := runLinkerdCommand(ctx, args)
	return formatLinkerdCommandResult("linkerd routes", result, err)

}

// Linkerd diagnostics proxy metrics
func handleLinkerdDiagnosticsProxyMetrics(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	namespace := mcp.ParseString(request, "namespace", "")
	allNamespaces := mcp.ParseString(request, "all_namespaces", "") == "true"
	selector := mcp.ParseString(request, "selector", "")
	resource := mcp.ParseString(request, "resource", "")

	args := []string{"diagnostics", "proxy-metrics"}

	if allNamespaces {
		args = append(args, "-A")
	} else if namespace != "" {
		args = append(args, "-n", namespace)
	}

	if selector != "" {
		args = append(args, "--selector", selector)
	}

	if resource != "" {
		args = append(args, resource)
	}

	result, err := runLinkerdCommand(ctx, args)
	return formatLinkerdCommandResult("linkerd diagnostics proxy-metrics", result, err)

}

// Linkerd diagnostics controller metrics
func handleLinkerdDiagnosticsControllerMetrics(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	namespace := mcp.ParseString(request, "namespace", "")
	component := mcp.ParseString(request, "component", "")

	args := []string{"diagnostics", "controller-metrics"}

	if namespace != "" {
		args = append(args, "-n", namespace)
	}

	if component != "" {
		args = append(args, "--component", component)
	}

	result, err := runLinkerdCommand(ctx, args)
	return formatLinkerdCommandResult("linkerd diagnostics controller-metrics", result, err)

}

// Linkerd diagnostics endpoints
func handleLinkerdDiagnosticsEndpoints(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	authority := mcp.ParseString(request, "authority", "")
	if authority == "" {
		return mcp.NewToolResultError("authority parameter is required"), nil
	}

	namespace := mcp.ParseString(request, "namespace", "")

	args := []string{"diagnostics", "endpoints"}

	if namespace != "" {
		args = append(args, "-n", namespace)
	}

	args = append(args, authority)

	result, err := runLinkerdCommand(ctx, args)
	return formatLinkerdCommandResult("linkerd diagnostics endpoints", result, err)

}

// Linkerd diagnostics policy
func handleLinkerdDiagnosticsPolicy(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	authority := mcp.ParseString(request, "authority", "")
	if authority == "" {
		return mcp.NewToolResultError("authority parameter is required"), nil
	}

	namespace := mcp.ParseString(request, "namespace", "")

	args := []string{"diagnostics", "policy"}

	if namespace != "" {
		args = append(args, "-n", namespace)
	}

	args = append(args, authority)

	result, err := runLinkerdCommand(ctx, args)
	return formatLinkerdCommandResult("linkerd diagnostics policy", result, err)

}

// Linkerd diagnostics profile
func handleLinkerdDiagnosticsProfile(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	authority := mcp.ParseString(request, "authority", "")
	if authority == "" {
		return mcp.NewToolResultError("authority parameter is required"), nil
	}

	namespace := mcp.ParseString(request, "namespace", "")

	args := []string{"diagnostics", "profile"}

	if namespace != "" {
		args = append(args, "-n", namespace)
	}

	args = append(args, authority)

	result, err := runLinkerdCommand(ctx, args)
	return formatLinkerdCommandResult("linkerd diagnostics profile", result, err)

}

// Linkerd viz install
func handleLinkerdVizInstall(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	ha := mcp.ParseString(request, "ha", "") == "true"
	skipChecks := mcp.ParseString(request, "skip_checks", "") == "true"
	setOverrides := mcp.ParseString(request, "set_overrides", "")

	args := []string{"viz", "install"}

	if ha {
		args = append(args, "--ha")
	}

	if skipChecks {
		args = append(args, "--skip-checks")
	}

	args = appendSetOverrides(args, setOverrides)

	manifest, err := runLinkerdManifestCommand(ctx, args)
	if err != nil {
		return formatLinkerdCommandResult("linkerd viz install", manifest, err)
	}

	applyResult, applyErr := applyManifest(ctx, manifest)
	return formatLinkerdCommandResult("kubectl apply linkerd viz install manifest", applyResult, applyErr)

}

// Linkerd viz uninstall
func handleLinkerdVizUninstall(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	force := mcp.ParseString(request, "force", "") == "true"

	args := []string{"viz", "uninstall"}

	if force {
		args = append(args, "--force")
	}

	manifest, err := runLinkerdManifestCommand(ctx, args)
	if err != nil {
		return formatLinkerdCommandResult("linkerd viz uninstall", manifest, err)
	}

	deleteResult, deleteErr := deleteManifest(ctx, manifest)
	return formatLinkerdCommandResult("kubectl delete linkerd viz uninstall manifest", deleteResult, deleteErr)

}

// Linkerd viz top
func handleLinkerdVizTop(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	resource := mcp.ParseString(request, "resource", "")
	if resource == "" {
		return mcp.NewToolResultError("resource parameter is required"), nil
	}

	namespace := mcp.ParseString(request, "namespace", "")
	from := mcp.ParseString(request, "from", "")
	to := mcp.ParseString(request, "to", "")
	maxRows := mcp.ParseString(request, "max_results", "")
	timeWindow := mcp.ParseString(request, "time_window", "")

	args := []string{"viz", "top", resource}

	if namespace != "" {
		args = append(args, "-n", namespace)
	}

	if from != "" {
		args = append(args, "--from", from)
	}

	if to != "" {
		args = append(args, "--to", to)
	}

	if maxRows != "" {
		args = append(args, "--max", maxRows)
	}

	if timeWindow != "" {
		args = append(args, "--time-window", timeWindow)
	}

	result, err := runLinkerdCommand(ctx, args)
	return formatLinkerdCommandResult("linkerd viz top", result, err)

}

// Linkerd viz stat
func handleLinkerdVizStat(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	resource := mcp.ParseString(request, "resource", "")
	if resource == "" {
		return mcp.NewToolResultError("resource parameter is required"), nil
	}

	namespace := mcp.ParseString(request, "namespace", "")
	allNamespaces := mcp.ParseString(request, "all_namespaces", "") == "true"
	from := mcp.ParseString(request, "from", "")
	to := mcp.ParseString(request, "to", "")
	timeWindow := mcp.ParseString(request, "time_window", "")
	output := mcp.ParseString(request, "output", "")

	args := []string{"viz", "stat", resource}

	if allNamespaces {
		args = append(args, "-A")
	} else if namespace != "" {
		args = append(args, "-n", namespace)
	}

	if from != "" {
		args = append(args, "--from", from)
	}

	if to != "" {
		args = append(args, "--to", to)
	}

	if timeWindow != "" {
		args = append(args, "--time-window", timeWindow)
	}

	if output != "" {
		args = append(args, "-o", output)
	}

	result, err := runLinkerdCommand(ctx, args)
	return formatLinkerdCommandResult("linkerd viz stat", result, err)

}

// Linkerd FIPS audit
func handleLinkerdFipsAudit(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	namespace := mcp.ParseString(request, "namespace", "")

	args := []string{"fips", "audit"}

	if namespace != "" {
		args = append(args, "-n", namespace)
	}

	result, err := runLinkerdCommand(ctx, args)
	return formatLinkerdCommandResult("linkerd fips audit", result, err)

}

// Linkerd policy generate
func handleLinkerdPolicyGenerate(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	namespace := mcp.ParseString(request, "namespace", "")
	output := mcp.ParseString(request, "output", "")
	timeout := mcp.ParseString(request, "timeout", "")

	args := []string{"policy", "generate"}

	if namespace != "" {
		args = append(args, "-n", namespace)
	}

	if output != "" {
		args = append(args, "-o", output)
	}

	if timeout != "" {
		args = append(args, "--timeout", timeout)
	}

	result, err := runLinkerdCommand(ctx, args)
	return formatLinkerdCommandResult("linkerd policy generate", result, err)

}

func appendSetOverrides(args []string, overrides string) []string {
	if overrides == "" {
		return args
	}

	pairs := strings.Split(overrides, ",")
	for _, pair := range pairs {
		trimmed := strings.TrimSpace(pair)
		if trimmed == "" {
			continue
		}
		args = append(args, "--set", trimmed)
	}

	return args
}

// Register Linkerd tools
func RegisterTools(s *server.MCPServer) {
	s.AddTool(mcp.NewTool("linkerd_check",
		mcp.WithDescription("Run pre-flight or data plane checks for the Linkerd control plane"),
		mcp.WithString("namespace", mcp.Description("Namespace that contains Linkerd components")),
		mcp.WithString("pre_check", mcp.Description("Set to true to run pre-installation checks")),
		mcp.WithString("proxy_check", mcp.Description("Set to true to run proxy diagnostics")),
		mcp.WithString("wait", mcp.Description("Duration to wait for checks to complete, e.g. 30s")),
		mcp.WithString("output", mcp.Description("Output format, e.g. table, short, json")),
	), telemetry.AdaptToolHandler(telemetry.WithTracing("linkerd_check", handleLinkerdCheck)))

	s.AddTool(mcp.NewTool("linkerd_install",
		mcp.WithDescription("Install the Linkerd control plane components"),
		mcp.WithString("ha", mcp.Description("Set to true to deploy high availability components")),
		mcp.WithString("crds_only", mcp.Description("Set to true to output only CRDs")),
		mcp.WithString("skip_checks", mcp.Description("Skip Kubernetes and environment checks")),
		mcp.WithString("identity_trust_anchors_pem", mcp.Description("PEM encoded trust anchors to use")),
		mcp.WithString("set_overrides", mcp.Description("Comma-separated Helm style key=value overrides")),
	), telemetry.AdaptToolHandler(telemetry.WithTracing("linkerd_install", handleLinkerdInstall)))

	s.AddTool(mcp.NewTool("linkerd_patch_workload_injection",
		mcp.WithDescription("Enable, disable, or remove Linkerd proxy injection annotations on a workload's pod template"),
		mcp.WithString("workload_name", mcp.Description("Name of the workload (e.g. simple-app)"), mcp.Required()),
		mcp.WithString("namespace", mcp.Description("Namespace containing the workload (default: default)")),
		mcp.WithString("workload_type", mcp.Description("Workload type: deployment, statefulset, or daemonset (default: deployment)")),
		mcp.WithString("inject_state", mcp.Description("Annotation value to set (enabled, disabled, ingress); ignored if remove_annotation is true")),
		mcp.WithString("remove_annotation", mcp.Description("Set to true to remove the annotation instead of setting it")),
	), telemetry.AdaptToolHandler(telemetry.WithTracing("linkerd_patch_workload_injection", handleLinkerdWorkloadInjection)))

	s.AddTool(mcp.NewTool("linkerd_install_cni",
		mcp.WithDescription("Install the Linkerd CNI components"),
		mcp.WithString("skip_checks", mcp.Description("Skip Kubernetes and environment checks")),
		mcp.WithString("set_overrides", mcp.Description("Comma-separated Helm style key=value overrides")),
	), telemetry.AdaptToolHandler(telemetry.WithTracing("linkerd_install_cni", handleLinkerdInstallCNI)))

	s.AddTool(mcp.NewTool("linkerd_upgrade",
		mcp.WithDescription("Upgrade the Linkerd control plane components"),
		mcp.WithString("ha", mcp.Description("Set to true to use high availability values")),
		mcp.WithString("crds_only", mcp.Description("Set to true to upgrade only CRDs")),
		mcp.WithString("skip_checks", mcp.Description("Skip Kubernetes and environment checks")),
		mcp.WithString("set_overrides", mcp.Description("Comma-separated Helm style key=value overrides")),
	), telemetry.AdaptToolHandler(telemetry.WithTracing("linkerd_upgrade", handleLinkerdUpgrade)))

	s.AddTool(mcp.NewTool("linkerd_uninstall",
		mcp.WithDescription("Uninstall the Linkerd control plane components"),
		mcp.WithString("force", mcp.Description("Set to true to skip confirmation prompts")),
	), telemetry.AdaptToolHandler(telemetry.WithTracing("linkerd_uninstall", handleLinkerdUninstall)))

	s.AddTool(mcp.NewTool("linkerd_version",
		mcp.WithDescription("Get Linkerd client and server versions"),
		mcp.WithString("client_only", mcp.Description("Set to true to print only client version")),
		mcp.WithString("short", mcp.Description("Set to true for short version output")),
	), telemetry.AdaptToolHandler(telemetry.WithTracing("linkerd_version", handleLinkerdVersion)))

	s.AddTool(mcp.NewTool("linkerd_authz",
		mcp.WithDescription("List Linkerd authorizations for a resource"),
		mcp.WithString("resource", mcp.Description("Resource to inspect, e.g. deploy/web"), mcp.Required()),
		mcp.WithString("namespace", mcp.Description("Namespace containing the resource")),
	), telemetry.AdaptToolHandler(telemetry.WithTracing("linkerd_authz", handleLinkerdAuthz)))

	s.AddTool(mcp.NewTool("linkerd_stat",
		mcp.WithDescription("Get resource metrics using linkerd stat"),
		mcp.WithString("resource", mcp.Description("Kubernetes resource to inspect (e.g. deploy/web)"), mcp.Required()),
		mcp.WithString("namespace", mcp.Description("Namespace for the resource")),
		mcp.WithString("all_namespaces", mcp.Description("Set to true to inspect every namespace")),
		mcp.WithString("from", mcp.Description("Restrict metrics to traffic from the specified resource")),
		mcp.WithString("to", mcp.Description("Restrict metrics to traffic to the specified resource")),
		mcp.WithString("time_window", mcp.Description("Time window for metrics, e.g. 1m")),
		mcp.WithString("output", mcp.Description("Output format (table, json)")),
	), telemetry.AdaptToolHandler(telemetry.WithTracing("linkerd_stat", handleLinkerdStat)))

	s.AddTool(mcp.NewTool("linkerd_top",
		mcp.WithDescription("Inspect live traffic using linkerd top"),
		mcp.WithString("resource", mcp.Description("Resource to observe (e.g. deploy/web)"), mcp.Required()),
		mcp.WithString("namespace", mcp.Description("Namespace for the resource")),
		mcp.WithString("from", mcp.Description("Limit traffic to requests originating from this resource")),
		mcp.WithString("to", mcp.Description("Limit traffic to requests destined to this resource")),
		mcp.WithString("max_results", mcp.Description("Maximum number of rows to display")),
		mcp.WithString("time_window", mcp.Description("Time window for sampling, e.g. 30s")),
	), telemetry.AdaptToolHandler(telemetry.WithTracing("linkerd_top", handleLinkerdTop)))

	s.AddTool(mcp.NewTool("linkerd_edges",
		mcp.WithDescription("Describe allowed and denied edges between resources"),
		mcp.WithString("resource", mcp.Description("Resource to inspect (e.g. deploy/web)"), mcp.Required()),
		mcp.WithString("namespace", mcp.Description("Namespace for the resource")),
		mcp.WithString("all_namespaces", mcp.Description("Set to true to inspect every namespace")),
		mcp.WithString("output", mcp.Description("Output format (table, wide, json)")),
	), telemetry.AdaptToolHandler(telemetry.WithTracing("linkerd_edges", handleLinkerdEdges)))

	s.AddTool(mcp.NewTool("linkerd_routes",
		mcp.WithDescription("Describe HTTP routes for resources"),
		mcp.WithString("resource", mcp.Description("Resource to inspect (e.g. deploy/web)"), mcp.Required()),
		mcp.WithString("namespace", mcp.Description("Namespace for the resource")),
		mcp.WithString("from", mcp.Description("Filter by traffic originating from this resource")),
		mcp.WithString("to", mcp.Description("Filter by traffic destined to this resource")),
		mcp.WithString("output", mcp.Description("Output format (table, json)")),
	), telemetry.AdaptToolHandler(telemetry.WithTracing("linkerd_routes", handleLinkerdRoutes)))

	s.AddTool(mcp.NewTool("linkerd_diagnostics_proxy_metrics",
		mcp.WithDescription("Collect raw proxy metrics for Linkerd workloads"),
		mcp.WithString("namespace", mcp.Description("Namespace to inspect")),
		mcp.WithString("all_namespaces", mcp.Description("Set to true to inspect every namespace")),
		mcp.WithString("selector", mcp.Description("Label selector to target specific pods")),
		mcp.WithString("resource", mcp.Description("Specific resource to query, e.g. deploy/web")),
	), telemetry.AdaptToolHandler(telemetry.WithTracing("linkerd_diagnostics_proxy_metrics", handleLinkerdDiagnosticsProxyMetrics)))

	s.AddTool(mcp.NewTool("linkerd_diagnostics_controller_metrics",
		mcp.WithDescription("Fetch metrics directly from Linkerd control-plane components"),
		mcp.WithString("namespace", mcp.Description("Namespace containing the control-plane pods")),
		mcp.WithString("component", mcp.Description("Specific control-plane component name")),
	), telemetry.AdaptToolHandler(telemetry.WithTracing("linkerd_diagnostics_controller_metrics", handleLinkerdDiagnosticsControllerMetrics)))

	s.AddTool(mcp.NewTool("linkerd_diagnostics_endpoints",
		mcp.WithDescription("Inspect Linkerd's service discovery endpoints for an authority"),
		mcp.WithString("authority", mcp.Description("Authority host:port to inspect"), mcp.Required()),
		mcp.WithString("namespace", mcp.Description("Namespace context for the query")),
	), telemetry.AdaptToolHandler(telemetry.WithTracing("linkerd_diagnostics_endpoints", handleLinkerdDiagnosticsEndpoints)))

	s.AddTool(mcp.NewTool("linkerd_diagnostics_policy",
		mcp.WithDescription("Inspect Linkerd's policy state for an authority"),
		mcp.WithString("authority", mcp.Description("Authority host:port to inspect"), mcp.Required()),
		mcp.WithString("namespace", mcp.Description("Namespace context for the query")),
	), telemetry.AdaptToolHandler(telemetry.WithTracing("linkerd_diagnostics_policy", handleLinkerdDiagnosticsPolicy)))

	s.AddTool(mcp.NewTool("linkerd_diagnostics_profile",
		mcp.WithDescription("Inspect Linkerd's service discovery profile for an authority"),
		mcp.WithString("authority", mcp.Description("Authority host:port to inspect"), mcp.Required()),
		mcp.WithString("namespace", mcp.Description("Namespace context for the query")),
	), telemetry.AdaptToolHandler(telemetry.WithTracing("linkerd_diagnostics_profile", handleLinkerdDiagnosticsProfile)))

	s.AddTool(mcp.NewTool("linkerd_viz_install",
		mcp.WithDescription("Install the Linkerd viz extension components"),
		mcp.WithString("ha", mcp.Description("Set to true to deploy high availability viz components")),
		mcp.WithString("skip_checks", mcp.Description("Skip Kubernetes and environment checks")),
		mcp.WithString("set_overrides", mcp.Description("Comma-separated Helm style key=value overrides")),
	), telemetry.AdaptToolHandler(telemetry.WithTracing("linkerd_viz_install", handleLinkerdVizInstall)))

	s.AddTool(mcp.NewTool("linkerd_viz_uninstall",
		mcp.WithDescription("Remove the Linkerd viz extension from the cluster"),
		mcp.WithString("force", mcp.Description("Set to true to skip confirmation prompts")),
	), telemetry.AdaptToolHandler(telemetry.WithTracing("linkerd_viz_uninstall", handleLinkerdVizUninstall)))

	s.AddTool(mcp.NewTool("linkerd_viz_top",
		mcp.WithDescription("Inspect live traffic for viz-injected workloads"),
		mcp.WithString("resource", mcp.Description("Resource to observe"), mcp.Required()),
		mcp.WithString("namespace", mcp.Description("Namespace for the resource")),
		mcp.WithString("from", mcp.Description("Limit traffic to requests originating from this resource")),
		mcp.WithString("to", mcp.Description("Limit traffic to requests destined to this resource")),
		mcp.WithString("max_results", mcp.Description("Maximum number of rows to display")),
		mcp.WithString("time_window", mcp.Description("Time window for sampling, e.g. 30s")),
	), telemetry.AdaptToolHandler(telemetry.WithTracing("linkerd_viz_top", handleLinkerdVizTop)))

	s.AddTool(mcp.NewTool("linkerd_viz_stat",
		mcp.WithDescription("Get viz metrics using linkerd viz stat"),
		mcp.WithString("resource", mcp.Description("Resource to inspect (e.g. deploy/web)"), mcp.Required()),
		mcp.WithString("namespace", mcp.Description("Namespace for the resource")),
		mcp.WithString("all_namespaces", mcp.Description("Set to true to inspect every namespace")),
		mcp.WithString("from", mcp.Description("Restrict metrics to traffic from the specified resource")),
		mcp.WithString("to", mcp.Description("Restrict metrics to traffic to the specified resource")),
		mcp.WithString("time_window", mcp.Description("Time window for metrics, e.g. 1m")),
		mcp.WithString("output", mcp.Description("Output format (table, json)")),
	), telemetry.AdaptToolHandler(telemetry.WithTracing("linkerd_viz_stat", handleLinkerdVizStat)))

	s.AddTool(mcp.NewTool("linkerd_fips_audit",
		mcp.WithDescription("Audit Linkerd proxies for FIPS compliance"),
		mcp.WithString("namespace", mcp.Description("Namespace scope for the audit")),
	), telemetry.AdaptToolHandler(telemetry.WithTracing("linkerd_fips_audit", handleLinkerdFipsAudit)))

	s.AddTool(mcp.NewTool("linkerd_policy_generate",
		mcp.WithDescription("Generate Linkerd policy manifests for existing workloads"),
		mcp.WithString("namespace", mcp.Description("Namespace containing workload manifests")),
		mcp.WithString("output", mcp.Description("Output format, e.g. yaml or json")),
		mcp.WithString("timeout", mcp.Description("Command timeout, e.g. 30s")),
	), telemetry.AdaptToolHandler(telemetry.WithTracing("linkerd_policy_generate", handleLinkerdPolicyGenerate)))
}
