package linkerd

import (
	"bytes"
	"context"
	"encoding/json"
	goerrors "errors"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"
	"time"

	"github.com/kagent-dev/tools/internal/commands"
	toolerrors "github.com/kagent-dev/tools/internal/errors"
	"github.com/kagent-dev/tools/internal/logger"
	"github.com/kagent-dev/tools/internal/security"
	"github.com/kagent-dev/tools/internal/telemetry"
	"github.com/kagent-dev/tools/pkg/utils"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// =================================
// Constants and variables
// =================================

const (
	linkerdInjectionAnnotationKey = "linkerd.io/inject"
)

type linkerdWorkloadTypeConfig struct {
	annotationsPath []string
	namespaced      bool
}

var linkerdWorkloadTypes = map[string]linkerdWorkloadTypeConfig{
	"namespace":             {annotationsPath: []string{"metadata", "annotations"}, namespaced: false},
	"deployment":            {annotationsPath: []string{"spec", "template", "metadata", "annotations"}, namespaced: true},
	"statefulset":           {annotationsPath: []string{"spec", "template", "metadata", "annotations"}, namespaced: true},
	"daemonset":             {annotationsPath: []string{"spec", "template", "metadata", "annotations"}, namespaced: true},
	"replicaset":            {annotationsPath: []string{"spec", "template", "metadata", "annotations"}, namespaced: true},
	"replicationcontroller": {annotationsPath: []string{"spec", "template", "metadata", "annotations"}, namespaced: true},
	"job":                   {annotationsPath: []string{"spec", "template", "metadata", "annotations"}, namespaced: true},
	"cronjob":               {annotationsPath: []string{"spec", "jobTemplate", "spec", "template", "metadata", "annotations"}, namespaced: true},
	"pod":                   {annotationsPath: []string{"metadata", "annotations"}, namespaced: true},
}

type manifestCommandExecutor interface {
	Run(ctx context.Context, command string, args []string) (stdout string, stderr string, err error)
}

var linkerdManifestExecutor manifestCommandExecutor = &execManifestCommandExecutor{}

type execManifestCommandExecutor struct{}

func (e *execManifestCommandExecutor) Run(ctx context.Context, command string, args []string) (string, string, error) {
	log := logger.WithContext(ctx)
	start := time.Now()
	cmd := exec.CommandContext(ctx, command, args...)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	duration := time.Since(start)
	if err != nil {
		log.Error("linkerd manifest command failed",
			"command", command,
			"args", args,
			"error", err,
			"stderr", strings.TrimSpace(stderr.String()),
			"duration", duration.String(),
		)
	} else {
		log.Info("linkerd manifest command executed",
			"command", command,
			"args", args,
			"stderr_length", stderr.Len(),
			"duration", duration.String(),
		)
	}

	return stdout.String(), stderr.String(), err
}

// =================================
// Helpers functions
// =================================

func supportedLinkerdWorkloadTypes() []string {
	types := make([]string, 0, len(linkerdWorkloadTypes))
	for t := range linkerdWorkloadTypes {
		types = append(types, t)
	}
	sort.Strings(types)
	return types
}

func buildAnnotationMergePatch(path []string, key, value string) (string, error) {
	if len(path) == 0 {
		return "", fmt.Errorf("annotation path is empty")
	}
	patch := map[string]interface{}{}
	current := patch
	for i, segment := range path {
		if i == len(path)-1 {
			current[segment] = map[string]interface{}{key: value}
			continue
		}
		next := map[string]interface{}{}
		current[segment] = next
		current = next
	}
	data, err := json.Marshal(patch)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func buildAnnotationRemovePatch(path []string, key string) string {
	segments := append([]string{}, path...)
	segments = append(segments, key)
	for i, segment := range segments {
		segments[i] = escapeJSONPointerSegment(segment)
	}
	return fmt.Sprintf(`[{"op":"remove","path":"/%s"}]`, strings.Join(segments, "/"))
}

func escapeJSONPointerSegment(segment string) string {
	segment = strings.ReplaceAll(segment, "~", "~0")
	segment = strings.ReplaceAll(segment, "/", "~1")
	return segment
}

func runLinkerdCommand(ctx context.Context, args []string) (string, error) {
	return executeLinkerdCommand(ctx, args)
}

func runLinkerdManifestCommand(ctx context.Context, args []string) (string, error) {
	kubeconfigPath := utils.GetKubeconfig()
	builder := commands.NewCommandBuilder("linkerd").
		WithArgs(args...).
		WithKubeconfig(kubeconfigPath)

	command, builtArgs, err := builder.Build()
	if err != nil {
		return "", err
	}

	stdout, stderr, execErr := linkerdManifestExecutor.Run(ctx, command, builtArgs)
	if execErr != nil {
		trimmed := strings.TrimSpace(stderr)
		combinedErr := execErr
		if trimmed != "" {
			combinedErr = fmt.Errorf("%w: %s", execErr, trimmed)
		}
		return stdout, toolerrors.NewCommandError(command, combinedErr)
	}

	if trimmed := strings.TrimSpace(stderr); trimmed != "" {
		logger.WithContext(ctx).Warn("linkerd manifest command produced stderr output",
			"command", command,
			"args", builtArgs,
			"stderr", trimmed,
		)
	}

	return stdout, nil
}

func executeLinkerdCommand(ctx context.Context, args []string) (string, error) {
	kubeconfigPath := utils.GetKubeconfig()
	builder := commands.NewCommandBuilder("linkerd").
		WithArgs(args...).
		WithKubeconfig(kubeconfigPath)

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

func appendSetOverrides(args []string, overrides string) []string {
	return appendCSVArgs(args, "--set", overrides)
}

func appendCSVArgs(args []string, flag, csv string) []string {
	for _, value := range parseCommaSeparated(csv) {
		args = append(args, flag, value)
	}
	return args
}

func appendFlagArg(args []string, flag, value string) []string {
	if value == "" {
		return args
	}
	return append(args, flag, value)
}

func appendBoolFlag(args []string, flag, value string) ([]string, error) {
	trimmed := strings.ToLower(strings.TrimSpace(value))
	if trimmed == "" {
		return args, nil
	}
	switch trimmed {
	case "true":
		return append(args, flag), nil
	case "false":
		return append(args, fmt.Sprintf("%s=false", flag)), nil
	default:
		return args, fmt.Errorf("invalid boolean value %q for %s", value, flag)
	}
}

func parseCommaSeparated(csv string) []string {
	if csv == "" {
		return nil
	}
	parts := strings.Split(csv, ",")
	values := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed == "" {
			continue
		}
		values = append(values, trimmed)
	}
	return values
}

// =================================
// Linkerd
// =================================

// The check command will perform a series of checks to validate that the linkerd
// CLI and control plane are configured correctly. If the command encounters a
// failure it will print additional information about the failure and exit with a
// non-zero exit code.
// Usage:
//
//	linkerd check [flags]
//
// Examples:
//
//	# Check that the Linkerd control plane is up and running
//	linkerd check
//	# Check that the Linkerd control plane can be installed in the "test" namespace
//	linkerd check --pre --linkerd-namespace test
//	# Check that the Linkerd data plane proxies in the "app" namespace are up and running
//	linkerd check --proxy --namespace app
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

// Output Kubernetes configs to install Linkerd.
//
// This command provides all Kubernetes configs necessary to install the Linkerd
// control plane.
//
// Usage:
//
//	linkerd install [flags]
//
// Examples:
//
//	# Install CRDs first.
//	linkerd install --crds | kubectl apply -f -
//
//	# Install the core control plane.
//	linkerd install | kubectl apply -f -
//
// The installation can be configured by using the --set, --values, --set-string and --set-file flags.
// A full list of configurable values can be found at https://artifacthub.io/packages/helm/linkerd2/linkerd-control-plane#values
func handleLinkerdInstall(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	ha := mcp.ParseString(request, "ha", "") == "true"
	crdsOnly := mcp.ParseString(request, "crds_only", "") == "true"
	skipChecks := mcp.ParseString(request, "skip_checks", "") == "true"
	identityTrustAnchors := mcp.ParseString(request, "identity_trust_anchors_pem", "")
	identityTrustAnchorsFile := mcp.ParseString(request, "identity_trust_anchors_file", "")
	identityClockSkewAllowance := mcp.ParseString(request, "identity_clock_skew_allowance", "")
	identityIssuanceLifetime := mcp.ParseString(request, "identity_issuance_lifetime", "")
	identityIssuerCertificateFile := mcp.ParseString(request, "identity_issuer_certificate_file", "")
	identityIssuerKeyFile := mcp.ParseString(request, "identity_issuer_key_file", "")
	identityTrustDomain := mcp.ParseString(request, "identity_trust_domain", "")
	setOverrides := mcp.ParseString(request, "set_overrides", "")
	setStringOverrides := mcp.ParseString(request, "set_string_overrides", "")
	setFileOverrides := mcp.ParseString(request, "set_file_overrides", "")
	valuesFiles := mcp.ParseString(request, "values", "")
	adminPort := mcp.ParseString(request, "admin_port", "")
	clusterDomain := mcp.ParseString(request, "cluster_domain", "")
	controlPort := mcp.ParseString(request, "control_port", "")
	controllerGID := mcp.ParseString(request, "controller_gid", "")
	controllerLogLevel := mcp.ParseString(request, "controller_log_level", "")
	controllerReplicas := mcp.ParseString(request, "controller_replicas", "")
	controllerUID := mcp.ParseString(request, "controller_uid", "")
	defaultInboundPolicy := mcp.ParseString(request, "default_inbound_policy", "")
	imagePullPolicy := mcp.ParseString(request, "image_pull_policy", "")
	inboundPort := mcp.ParseString(request, "inbound_port", "")
	initImage := mcp.ParseString(request, "init_image", "")
	initImageVersion := mcp.ParseString(request, "init_image_version", "")
	outboundPort := mcp.ParseString(request, "outbound_port", "")
	outputFormat := mcp.ParseString(request, "output", "")
	proxyCPULimit := mcp.ParseString(request, "proxy_cpu_limit", "")
	proxyCPURequest := mcp.ParseString(request, "proxy_cpu_request", "")
	proxyGID := mcp.ParseString(request, "proxy_gid", "")
	proxyImage := mcp.ParseString(request, "proxy_image", "")
	proxyLogLevel := mcp.ParseString(request, "proxy_log_level", "")
	proxyMemoryLimit := mcp.ParseString(request, "proxy_memory_limit", "")
	proxyMemoryRequest := mcp.ParseString(request, "proxy_memory_request", "")
	proxyUID := mcp.ParseString(request, "proxy_uid", "")
	registry := mcp.ParseString(request, "registry", "")
	skipInboundPorts := mcp.ParseString(request, "skip_inbound_ports", "")
	skipOutboundPorts := mcp.ParseString(request, "skip_outbound_ports", "")
	disableH2Upgrade := mcp.ParseString(request, "disable_h2_upgrade", "")
	disableHeartbeat := mcp.ParseString(request, "disable_heartbeat", "")
	enableEndpointSlices := mcp.ParseString(request, "enable_endpoint_slices", "")
	enableExternalProfiles := mcp.ParseString(request, "enable_external_profiles", "")
	identityExternalCA := mcp.ParseString(request, "identity_external_ca", "")
	identityExternalIssuer := mcp.ParseString(request, "identity_external_issuer", "")
	ignoreCluster := mcp.ParseString(request, "ignore_cluster", "")
	linkerdCNIEnabled := mcp.ParseString(request, "linkerd_cni_enabled", "")

	args := []string{"install"}

	if ha {
		args = append(args, "--ha")
	}

	if skipChecks {
		args = append(args, "--skip-checks")
	}

	boolFlags := []struct {
		flag  string
		value string
	}{
		{"--disable-h2-upgrade", disableH2Upgrade},
		{"--disable-heartbeat", disableHeartbeat},
		{"--enable-endpoint-slices", enableEndpointSlices},
		{"--enable-external-profiles", enableExternalProfiles},
		{"--identity-external-ca", identityExternalCA},
		{"--identity-external-issuer", identityExternalIssuer},
		{"--ignore-cluster", ignoreCluster},
		{"--linkerd-cni-enabled", linkerdCNIEnabled},
	}

	for _, flag := range boolFlags {
		var err error
		args, err = appendBoolFlag(args, flag.flag, flag.value)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
	}

	args = appendFlagArg(args, "--admin-port", adminPort)
	args = appendFlagArg(args, "--cluster-domain", clusterDomain)
	args = appendFlagArg(args, "--control-port", controlPort)
	args = appendFlagArg(args, "--controller-gid", controllerGID)
	args = appendFlagArg(args, "--controller-log-level", controllerLogLevel)
	args = appendFlagArg(args, "--controller-replicas", controllerReplicas)
	args = appendFlagArg(args, "--controller-uid", controllerUID)
	args = appendFlagArg(args, "--default-inbound-policy", defaultInboundPolicy)
	args = appendFlagArg(args, "--identity-clock-skew-allowance", identityClockSkewAllowance)
	args = appendFlagArg(args, "--identity-issuance-lifetime", identityIssuanceLifetime)
	args = appendFlagArg(args, "--identity-issuer-certificate-file", identityIssuerCertificateFile)
	args = appendFlagArg(args, "--identity-issuer-key-file", identityIssuerKeyFile)
	args = appendFlagArg(args, "--identity-trust-anchors-file", identityTrustAnchorsFile)
	if identityTrustAnchors != "" {
		args = append(args, "--identity-trust-anchors-pem", identityTrustAnchors)
	}
	args = appendFlagArg(args, "--identity-trust-domain", identityTrustDomain)
	args = appendFlagArg(args, "--image-pull-policy", imagePullPolicy)
	args = appendFlagArg(args, "--inbound-port", inboundPort)
	args = appendFlagArg(args, "--init-image", initImage)
	args = appendFlagArg(args, "--init-image-version", initImageVersion)
	args = appendFlagArg(args, "--outbound-port", outboundPort)
	args = appendFlagArg(args, "--proxy-cpu-limit", proxyCPULimit)
	args = appendFlagArg(args, "--proxy-cpu-request", proxyCPURequest)
	args = appendFlagArg(args, "--proxy-gid", proxyGID)
	args = appendFlagArg(args, "--proxy-image", proxyImage)
	args = appendFlagArg(args, "--proxy-log-level", proxyLogLevel)
	args = appendFlagArg(args, "--proxy-memory-limit", proxyMemoryLimit)
	args = appendFlagArg(args, "--proxy-memory-request", proxyMemoryRequest)
	args = appendFlagArg(args, "--proxy-uid", proxyUID)
	args = appendFlagArg(args, "--registry", registry)
	args = appendFlagArg(args, "--skip-inbound-ports", skipInboundPorts)
	args = appendFlagArg(args, "--skip-outbound-ports", skipOutboundPorts)
	args = appendFlagArg(args, "-o", outputFormat)

	args = appendSetOverrides(args, setOverrides)
	args = appendCSVArgs(args, "--set-string", setStringOverrides)
	args = appendCSVArgs(args, "--set-file", setFileOverrides)
	args = appendCSVArgs(args, "-f", valuesFiles)

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

// Output Kubernetes configs to install Linkerd CNI.
//
// This command provides all Kubernetes configs necessary to install the Linkerd
// CNI.
//
// Usage:
//
//	linkerd install-cni [flags]
func handleLinkerdInstallCNI(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	skipChecks := mcp.ParseString(request, "skip_checks", "") == "true"
	setOverrides := mcp.ParseString(request, "set_overrides", "")
	setStringOverrides := mcp.ParseString(request, "set_string_overrides", "")
	setFileOverrides := mcp.ParseString(request, "set_file_overrides", "")
	valuesFiles := mcp.ParseString(request, "values", "")
	adminPort := mcp.ParseString(request, "admin_port", "")
	cniImage := mcp.ParseString(request, "cni_image", "")
	cniImageVersion := mcp.ParseString(request, "cni_image_version", "")
	cniLogLevel := mcp.ParseString(request, "cni_log_level", "")
	controlPort := mcp.ParseString(request, "control_port", "")
	destCNIBinDir := mcp.ParseString(request, "dest_cni_bin_dir", "")
	destCNINetDir := mcp.ParseString(request, "dest_cni_net_dir", "")
	inboundPort := mcp.ParseString(request, "inbound_port", "")
	linkerdVersion := mcp.ParseString(request, "linkerd_version", "")
	outboundPort := mcp.ParseString(request, "outbound_port", "")
	priorityClassName := mcp.ParseString(request, "priority_class_name", "")
	proxyGID := mcp.ParseString(request, "proxy_gid", "")
	proxyUID := mcp.ParseString(request, "proxy_uid", "")
	redirectPorts := mcp.ParseString(request, "redirect_ports", "")
	registry := mcp.ParseString(request, "registry", "")
	skipInboundPorts := mcp.ParseString(request, "skip_inbound_ports", "")
	skipOutboundPorts := mcp.ParseString(request, "skip_outbound_ports", "")
	useWaitFlag := mcp.ParseString(request, "use_wait_flag", "") == "true"

	args := []string{"install-cni"}

	if skipChecks {
		args = append(args, "--skip-checks")
	}

	if useWaitFlag {
		args = append(args, "--use-wait-flag")
	}

	args = appendFlagArg(args, "--admin-port", adminPort)
	args = appendFlagArg(args, "--cni-image", cniImage)
	args = appendFlagArg(args, "--cni-image-version", cniImageVersion)
	args = appendFlagArg(args, "--cni-log-level", cniLogLevel)
	args = appendFlagArg(args, "--control-port", controlPort)
	args = appendFlagArg(args, "--dest-cni-bin-dir", destCNIBinDir)
	args = appendFlagArg(args, "--dest-cni-net-dir", destCNINetDir)
	args = appendFlagArg(args, "--inbound-port", inboundPort)
	args = appendFlagArg(args, "--linkerd-version", linkerdVersion)
	args = appendFlagArg(args, "--outbound-port", outboundPort)
	args = appendFlagArg(args, "--priority-class-name", priorityClassName)
	args = appendFlagArg(args, "--proxy-gid", proxyGID)
	args = appendFlagArg(args, "--proxy-uid", proxyUID)
	args = appendFlagArg(args, "--redirect-ports", redirectPorts)
	args = appendFlagArg(args, "--registry", registry)
	args = appendFlagArg(args, "--skip-inbound-ports", skipInboundPorts)
	args = appendFlagArg(args, "--skip-outbound-ports", skipOutboundPorts)

	args = appendSetOverrides(args, setOverrides)
	args = appendCSVArgs(args, "--set-string", setStringOverrides)
	args = appendCSVArgs(args, "--set-file", setFileOverrides)
	args = appendCSVArgs(args, "-f", valuesFiles)

	manifest, err := runLinkerdManifestCommand(ctx, args)
	if err != nil {
		return formatLinkerdCommandResult("linkerd install-cni", manifest, err)
	}

	applyResult, applyErr := applyManifest(ctx, manifest)
	return formatLinkerdCommandResult("kubectl apply linkerd install-cni manifest", applyResult, applyErr)

}

// Output Kubernetes configs to upgrade an existing Linkerd control plane.
//
// Note that the default flag values for this command come from the Linkerd control
// plane. The default values displayed in the Flags section below only apply to the
// install command.
//
// The upgrade can be configured by using the --set, --values, --set-string and --set-file flags.
// A full list of configurable values can be found at https://www.github.com/linkerd/linkerd2/tree/main/charts/linkerd2/README.md
//
// Usage:
//
//	linkerd upgrade [flags]
//
// Examples:
//
//	# Upgrade CRDs first
//	linkerd upgrade --crds | kubectl apply -f -
//
//	# Then upgrade the control plane
//	linkerd upgrade | kubectl apply -f -
//
//	# And lastly, remove linkerd resources that no longer exist in the current version
//	linkerd prune | kubectl delete -f -
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
// Print the client and server version information
//
// Usage:
//
//	linkerd version [flags]
func handleLinkerdVersion(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	clientOnly := mcp.ParseString(request, "client_only", "") == "true"
	short := mcp.ParseString(request, "short", "") == "true"
	proxyVersions := mcp.ParseString(request, "proxy", "") == "true"
	namespace := strings.TrimSpace(mcp.ParseString(request, "namespace", ""))
	if namespace != "" {
		if err := security.ValidateNamespace(namespace); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("invalid namespace: %v", err)), nil
		}
	}

	args := []string{"version"}

	if clientOnly {
		args = append(args, "--client")
	}

	if proxyVersions {
		args = append(args, "--proxy")
	}

	if namespace != "" {
		args = append(args, "-n", namespace)
	}

	if short {
		args = append(args, "--short")
	}

	result, err := runLinkerdCommand(ctx, args)
	return formatLinkerdCommandResult("linkerd version", result, err)

}

// List authorizations for a resource.
//
// Usage:
//
//	linkerd authz [flags] resource
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

// Add the Linkerd proxy to a Kubernetes config.
//
// You can inject resources contained in a single file, inside a folder and its
// sub-folders, or coming from stdin.
//
// Usage:
//
//	linkerd inject [flags] CONFIG-FILE
//
// Examples:
//
//	# Inject all the deployments in the default namespace.
//	kubectl get deploy -o yaml | linkerd inject - | kubectl apply -f -
//
//	# Injecting a file from a remote URL
//	linkerd inject https://url.to/yml | kubectl apply -f -
//
//	# Inject all the resources inside a folder and its sub-folders.
//	linkerd inject <folder> | kubectl apply -f -
func handleLinkerdWorkloadInjection(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	workloadName := mcp.ParseString(request, "workload_name", "")
	if workloadName == "" {
		return mcp.NewToolResultError("workload_name parameter is required"), nil
	}

	if err := security.ValidateK8sResourceName(workloadName); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("invalid workload name: %v", err)), nil
	}

	namespaceInput := mcp.ParseString(request, "namespace", "default")

	workloadType := strings.ToLower(mcp.ParseString(request, "workload_type", "deployment"))
	config, ok := linkerdWorkloadTypes[workloadType]
	if !ok {
		return mcp.NewToolResultError(fmt.Sprintf("workload_type must be one of: %s", strings.Join(supportedLinkerdWorkloadTypes(), ", "))), nil
	}

	var namespace string
	if config.namespaced {
		if namespaceInput == "" {
			namespaceInput = "default"
		}
		if err := security.ValidateNamespace(namespaceInput); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("invalid namespace: %v", err)), nil
		}
		namespace = namespaceInput
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

	args := []string{"patch", workloadType, workloadName}
	if config.namespaced {
		args = append(args, "-n", namespace)
	}

	var patch string
	var operation string
	if removeAnnotation {
		patch = buildAnnotationRemovePatch(config.annotationsPath, linkerdInjectionAnnotationKey)
		args = append(args, "--type=json", "-p", patch)
		operation = fmt.Sprintf("kubectl remove linkerd injection annotation from %s %s", workloadType, workloadName)
	} else {
		var err error
		patch, err = buildAnnotationMergePatch(config.annotationsPath, linkerdInjectionAnnotationKey, injectState)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("failed to build patch: %v", err)), nil
		}
		args = append(args, "-p", patch)
		operation = fmt.Sprintf("kubectl set linkerd injection=%s on %s %s", injectState, workloadType, workloadName)
	}

	result, err := runKubectlCommand(ctx, args)
	return formatLinkerdCommandResult(operation, result, err)
}

// Commands used to manage Linkerd policy.
//
// This command provides subcommands to manage Linkerd policy.
//
// Usage:
//
//	linkerd policy [command]
//
// Examples:
//
//	# Generate policy for existing meshed workloads
//	linkerd policy generate
//
// Available Commands:
//
//	generate    Generate policy based on current traffic (beta)
func handleLinkerdPolicy(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	command := strings.TrimSpace(strings.ToLower(mcp.ParseString(request, "command", "")))
	if command == "" {
		command = "generate"
	}

	switch command {
	case "generate":
		result, err := runLinkerdCommand(ctx, nil)
		return formatLinkerdCommandResult("linkerd policy generate", result, err)
	default:
		return mcp.NewToolResultError(fmt.Sprintf("unsupported linkerd policy command: %s", command)), nil
	}
}

// Output service profile config for Kubernetes.
//
// Usage:
//
//	linkerd profile [flags] (--template | --open-api file | --proto file) (SERVICE)
//
// Examples:
//
//	# Output a basic template to apply after modification.
//	linkerd profile -n emojivoto --template web-svc
//
//	# Generate a profile from an OpenAPI specification.
//	linkerd profile -n emojivoto --open-api web-svc.swagger web-svc
//
//	# Generate a profile from a protobuf definition.
//	linkerd profile -n emojivoto --proto Voting.proto vote-svc
func handleLinkerdProfile(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	serviceName := strings.TrimSpace(mcp.ParseString(request, "service_name", ""))
	if serviceName == "" {
		return mcp.NewToolResultError("service_name parameter is required"), nil
	}

	if err := security.ValidateK8sResourceName(serviceName); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("invalid service name: %v", err)), nil
	}

	namespace := strings.TrimSpace(mcp.ParseString(request, "namespace", ""))
	if namespace != "" {
		if err := security.ValidateNamespace(namespace); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("invalid namespace: %v", err)), nil
		}
	}

	ignoreCluster := mcp.ParseString(request, "ignore_cluster", "") == "true"
	templateOutput := mcp.ParseString(request, "template", "") == "true"
	openAPIFile := strings.TrimSpace(mcp.ParseString(request, "open_api", ""))
	protoFile := strings.TrimSpace(mcp.ParseString(request, "proto", ""))
	outputFormat := strings.TrimSpace(mcp.ParseString(request, "output", ""))

	modeCount := 0
	if templateOutput {
		modeCount++
	}
	if openAPIFile != "" {
		modeCount++
	}
	if protoFile != "" {
		modeCount++
	}
	if modeCount == 0 {
		return mcp.NewToolResultError("one of template, open_api, or proto must be provided"), nil
	}
	if modeCount > 1 {
		return mcp.NewToolResultError("template, open_api, and proto options are mutually exclusive"), nil
	}

	args := []string{"profile"}
	if namespace != "" {
		args = append(args, "-n", namespace)
	}
	if ignoreCluster {
		args = append(args, "--ignore-cluster")
	}
	if templateOutput {
		args = append(args, "--template")
	} else if openAPIFile != "" {
		args = append(args, "--open-api", openAPIFile)
	} else if protoFile != "" {
		args = append(args, "--proto", protoFile)
	}
	if outputFormat != "" {
		args = append(args, "-o", outputFormat)
	}
	args = append(args, serviceName)

	result, err := runLinkerdCommand(ctx, args)
	return formatLinkerdCommandResult("linkerd profile", result, err)
}

// Commands used to manage FIPS-enabled installations of Linkerd.
//
// This command provides subcommands to manage FIPS-enabled installations of
// Linkerd.
//
// Usage:
//
//	linkerd fips [command]
//
// Examples:
//
//	# Audit all Linkerd proxies for FIPS modules
//	linkerd fips audit
func handleLinkerdFips(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	namespace := mcp.ParseString(request, "namespace", "")

	args := []string{"fips", "audit"}

	if namespace != "" {
		args = append(args, "-n", namespace)
	}

	result, err := runLinkerdCommand(ctx, args)
	return formatLinkerdCommandResult("linkerd fips audit", result, err)

}

// =================================
// Linkerd Diagnostics
// =================================

// Fetch metrics directly from Linkerd control plane containers.
//
//	This command initiates port-forward to each control plane process, and
//	queries the /metrics endpoint on them.
//
// Usage:
//
//	linkerd diagnostics controller-metrics [flags]
func handleLinkerdDiagnosticsControllerMetrics(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	namespace := mcp.ParseString(request, "namespace", "")
	waitDuration := mcp.ParseString(request, "wait", "")

	args := []string{"diagnostics", "controller-metrics"}

	if namespace != "" {
		args = append(args, "-n", namespace)
	}

	if waitDuration != "" {
		args = append(args, "--wait", waitDuration)
	}

	result, err := runLinkerdCommand(ctx, args)
	return formatLinkerdCommandResult("linkerd diagnostics controller-metrics", result, err)
}

// Introspect Linkerd's service discovery state.
//
// This command provides debug information about the internal state of the
// control-plane's destination controller. It queries the same Destination service
// endpoint as the linkerd-proxy's, and returns the profile associated with that
// destination.
//
// Usage:
//
//	linkerd diagnostics profile [flags] address
//
// Examples:
//
//	# Get the service profile for the service or endpoint at 10.20.2.4:8080
//	linkerd diagnostics profile 10.20.2.4:8080
func handleLinkerdDiagnosticsProfile(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	authority := mcp.ParseString(request, "authority", "")
	if authority == "" {
		return mcp.NewToolResultError("authority parameter is required"), nil
	}

	namespace := mcp.ParseString(request, "namespace", "")
	destinationPod := mcp.ParseString(request, "destination_pod", "")
	token := mcp.ParseString(request, "token", "")

	args := []string{"diagnostics", "profile"}

	if namespace != "" {
		args = append(args, "-n", namespace)
	}

	if destinationPod != "" {
		args = append(args, "--destination-pod", destinationPod)
	}

	if token != "" {
		args = append(args, "--token", token)
	}

	args = append(args, authority)

	result, err := runLinkerdCommand(ctx, args)
	return formatLinkerdCommandResult("linkerd diagnostics profile", result, err)

}

// Fetch metrics directly from Linkerd proxies.
//
//	This command initiates a port-forward to a given pod or set of pods, and
//	queries the /metrics endpoint on the Linkerd proxies.
//
// Examples:
//
//	# Get metrics from pod-foo-bar in the default namespace.
//	linkerd diagnostics proxy-metrics po/pod-foo-bar
//
//	# Get metrics from the web deployment in the emojivoto namespace.
//	linkerd diagnostics proxy-metrics -n emojivoto deploy/web
func handleLinkerdDiagnosticsProxyMetrics(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	resource := strings.TrimSpace(mcp.ParseString(request, "resource", ""))
	if resource == "" {
		return mcp.NewToolResultError("resource parameter is required"), nil
	}

	namespace := mcp.ParseString(request, "namespace", "")
	obfuscate := mcp.ParseString(request, "obfuscate", "") == "true"

	args := []string{"diagnostics", "proxy-metrics"}

	if namespace != "" {
		args = append(args, "-n", namespace)
	}

	if obfuscate {
		args = append(args, "--obfuscate")
	}

	args = append(args, resource)

	result, err := runLinkerdCommand(ctx, args)
	return formatLinkerdCommandResult("linkerd diagnostics proxy-metrics", result, err)
}

// Introspect Linkerd's service discovery state.
//
// This command provides debug information about the internal state of the
// control-plane's destination container. It queries the same Destination service
// endpoint as the linkerd-proxy's, and returns the addresses associated with that
// destination.
//
// Usage:
//
//	linkerd diagnostics endpoints [flags] authorities
//
// Examples:
//
//	# get all endpoints for the authorities emoji-svc.emojivoto.svc.cluster.local:8080 and web-svc.emojivoto.svc.cluster.local:80
//	linkerd diagnostics endpoints emoji-svc.emojivoto.svc.cluster.local:8080 web-svc.emojivoto.svc.cluster.local:80
//
//	# get that same information in json format
//	linkerd diagnostics endpoints -o json emoji-svc.emojivoto.svc.cluster.local:8080 web-svc.emojivoto.svc.cluster.local:80
//
//	# get the endpoints for authorities in Linkerd's control-plane itself
//	linkerd diagnostics endpoints web.linkerd-viz.svc.cluster.local:8084
func handleLinkerdDiagnosticsEndpoints(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	authority := mcp.ParseString(request, "authority", "")
	if authority == "" {
		return mcp.NewToolResultError("authority parameter is required"), nil
	}

	namespace := mcp.ParseString(request, "namespace", "")
	destinationPod := mcp.ParseString(request, "destination_pod", "")
	outputFormat := mcp.ParseString(request, "output", "")
	token := mcp.ParseString(request, "token", "")

	args := []string{"diagnostics", "endpoints"}

	if namespace != "" {
		args = append(args, "-n", namespace)
	}

	if destinationPod != "" {
		args = append(args, "--destination-pod", destinationPod)
	}

	if outputFormat != "" {
		args = append(args, "-o", outputFormat)
	}

	if token != "" {
		args = append(args, "--token", token)
	}

	args = append(args, authority)

	result, err := runLinkerdCommand(ctx, args)
	return formatLinkerdCommandResult("linkerd diagnostics endpoints", result, err)

}

// Introspect Linkerd's policy state.
//
// This command provides debug information about the internal state of the
// control-plane's policy controller. It queries the same control-plane
// endpoint as the linkerd-proxy's, and returns the policies associated with the
// given resource. If the resource is a Pod, inbound policy for that Pod is
// displayed. If the resource is a Service, outbound policy for that Service is
// displayed.
//
// Usage:
//
//	linkerd diagnostics policy [flags] resource port
//
// Examples:
//
//	# get the inbound policy for pod emoji-6d66d87995-bvrnn on port 8080
//	linkerd diagnostics policy -n emojivoto po/emoji-6d66d87995-bvrnn 8080
//
//	# get the outbound policy for Service emoji-svc on port 8080
//	linkerd diagnostics policy -n emojivoto svc/emoji-svc 8080
func handleLinkerdDiagnosticsPolicy(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	resource := strings.TrimSpace(mcp.ParseString(request, "resource", ""))
	if resource == "" {
		return mcp.NewToolResultError("resource parameter is required"), nil
	}

	port := strings.TrimSpace(mcp.ParseString(request, "port", ""))
	if port == "" {
		return mcp.NewToolResultError("port parameter is required"), nil
	}

	namespace := mcp.ParseString(request, "namespace", "")
	destinationPod := mcp.ParseString(request, "destination_pod", "")
	outputFormat := mcp.ParseString(request, "output", "")
	token := mcp.ParseString(request, "token", "")

	args := []string{"diagnostics", "policy"}

	if namespace != "" {
		args = append(args, "-n", namespace)
	}

	if destinationPod != "" {
		args = append(args, "--destination-pod", destinationPod)
	}

	if outputFormat != "" {
		args = append(args, "-o", outputFormat)
	}

	if token != "" {
		args = append(args, "--token", token)
	}

	args = append(args, resource, port)

	result, err := runLinkerdCommand(ctx, args)
	return formatLinkerdCommandResult("linkerd diagnostics policy", result, err)

}

// =================================
// Register Linkerd tools
// =================================
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
		mcp.WithString("identity_trust_anchors_pem", mcp.Description("PEM encoded trust anchors for --identity-trust-anchors-pem")),
		mcp.WithString("identity_trust_anchors_file", mcp.Description("Path to trust anchors file (--identity-trust-anchors-file)")),
		mcp.WithString("identity_clock_skew_allowance", mcp.Description("Duration for --identity-clock-skew-allowance, e.g. 20s")),
		mcp.WithString("identity_issuance_lifetime", mcp.Description("Certificate lifetime for --identity-issuance-lifetime")),
		mcp.WithString("identity_issuer_certificate_file", mcp.Description("Path for --identity-issuer-certificate-file")),
		mcp.WithString("identity_issuer_key_file", mcp.Description("Path for --identity-issuer-key-file")),
		mcp.WithString("identity_trust_domain", mcp.Description("Value for --identity-trust-domain")),
		mcp.WithString("identity_external_ca", mcp.Description("true/false to toggle --identity-external-ca")),
		mcp.WithString("identity_external_issuer", mcp.Description("true/false to toggle --identity-external-issuer")),
		mcp.WithString("ignore_cluster", mcp.Description("true/false to toggle --ignore-cluster")),
		mcp.WithString("admin_port", mcp.Description("Proxy metrics port (--admin-port)")),
		mcp.WithString("cluster_domain", mcp.Description("Custom cluster domain (--cluster-domain)")),
		mcp.WithString("control_port", mcp.Description("Proxy control port (--control-port)")),
		mcp.WithString("controller_gid", mcp.Description("Run control plane under this GID (--controller-gid)")),
		mcp.WithString("controller_log_level", mcp.Description("Log level for controller/web (--controller-log-level)")),
		mcp.WithString("controller_replicas", mcp.Description("Number of controller replicas (--controller-replicas)")),
		mcp.WithString("controller_uid", mcp.Description("Run control plane under this UID (--controller-uid)")),
		mcp.WithString("default_inbound_policy", mcp.Description("Default inbound policy (--default-inbound-policy)")),
		mcp.WithString("disable_h2_upgrade", mcp.Description("true/false for --disable-h2-upgrade")),
		mcp.WithString("disable_heartbeat", mcp.Description("true/false for --disable-heartbeat")),
		mcp.WithString("enable_endpoint_slices", mcp.Description("true/false for --enable-endpoint-slices")),
		mcp.WithString("enable_external_profiles", mcp.Description("true/false for --enable-external-profiles")),
		mcp.WithString("image_pull_policy", mcp.Description("Docker image pull policy (--image-pull-policy)")),
		mcp.WithString("inbound_port", mcp.Description("Proxy inbound port (--inbound-port)")),
		mcp.WithString("init_image", mcp.Description("Init container image (--init-image)")),
		mcp.WithString("init_image_version", mcp.Description("Init container image version (--init-image-version)")),
		mcp.WithString("linkerd_cni_enabled", mcp.Description("true/false to toggle --linkerd-cni-enabled")),
		mcp.WithString("outbound_port", mcp.Description("Proxy outbound port (--outbound-port)")),
		mcp.WithString("output", mcp.Description("Output format for manifests (-o/--output)")),
		mcp.WithString("proxy_cpu_limit", mcp.Description("Proxy CPU limit (--proxy-cpu-limit)")),
		mcp.WithString("proxy_cpu_request", mcp.Description("Proxy CPU request (--proxy-cpu-request)")),
		mcp.WithString("proxy_gid", mcp.Description("Proxy GID (--proxy-gid)")),
		mcp.WithString("proxy_image", mcp.Description("Proxy image (--proxy-image)")),
		mcp.WithString("proxy_log_level", mcp.Description("Proxy log level (--proxy-log-level)")),
		mcp.WithString("proxy_memory_limit", mcp.Description("Proxy memory limit (--proxy-memory-limit)")),
		mcp.WithString("proxy_memory_request", mcp.Description("Proxy memory request (--proxy-memory-request)")),
		mcp.WithString("proxy_uid", mcp.Description("Proxy UID (--proxy-uid)")),
		mcp.WithString("registry", mcp.Description("Image registry (--registry)")),
		mcp.WithString("skip_inbound_ports", mcp.Description("Ports to skip inbound proxying (--skip-inbound-ports)")),
		mcp.WithString("skip_outbound_ports", mcp.Description("Ports to skip outbound proxying (--skip-outbound-ports)")),
		mcp.WithString("set_overrides", mcp.Description("Comma-separated Helm style key=value overrides for --set")),
		mcp.WithString("set_string_overrides", mcp.Description("Comma-separated key=value overrides for --set-string")),
		mcp.WithString("set_file_overrides", mcp.Description("Comma-separated key=path overrides for --set-file")),
		mcp.WithString("values", mcp.Description("Comma-separated list of values files/URLs for -f/--values")),
	), telemetry.AdaptToolHandler(telemetry.WithTracing("linkerd_install", handleLinkerdInstall)))

	s.AddTool(mcp.NewTool("linkerd_patch_workload_injection",
		mcp.WithDescription("Enable, disable, or remove Linkerd proxy injection annotations on a workload's pod template"),
		mcp.WithString("workload_name", mcp.Description("Name of the workload (e.g. simple-app)"), mcp.Required()),
		mcp.WithString("namespace", mcp.Description("Namespace containing the workload (default: default)")),
		mcp.WithString("workload_type", mcp.Description("Workload type: namespace, deployment, statefulset, daemonset, job, cronjob, pod, replicaset, or replicationcontroller (default: deployment)")),
		mcp.WithString("inject_state", mcp.Description("Annotation value to set (enabled, disabled, ingress); ignored if remove_annotation is true")),
		mcp.WithString("remove_annotation", mcp.Description("Set to true to remove the annotation instead of setting it")),
	), telemetry.AdaptToolHandler(telemetry.WithTracing("linkerd_patch_workload_injection", handleLinkerdWorkloadInjection)))

	s.AddTool(mcp.NewTool("linkerd_install_cni",
		mcp.WithDescription("Install the Linkerd CNI components"),
		mcp.WithString("skip_checks", mcp.Description("Skip Kubernetes and environment checks")),
		mcp.WithString("admin_port", mcp.Description("Proxy metrics port for the CNI proxy (maps to --admin-port)")),
		mcp.WithString("cni_image", mcp.Description("Override the CNI plugin image (--cni-image)")),
		mcp.WithString("cni_image_version", mcp.Description("Override the CNI plugin image tag (--cni-image-version)")),
		mcp.WithString("cni_log_level", mcp.Description("Set the CNI plugin log level (--cni-log-level)")),
		mcp.WithString("control_port", mcp.Description("Proxy control port (--control-port)")),
		mcp.WithString("dest_cni_bin_dir", mcp.Description("Host directory to place the CNI binary (--dest-cni-bin-dir)")),
		mcp.WithString("dest_cni_net_dir", mcp.Description("Host directory to place the CNI config (--dest-cni-net-dir)")),
		mcp.WithString("inbound_port", mcp.Description("Proxy inbound port (--inbound-port)")),
		mcp.WithString("linkerd_version", mcp.Description("Linkerd image tag to use (--linkerd-version)")),
		mcp.WithString("outbound_port", mcp.Description("Proxy outbound port (--outbound-port)")),
		mcp.WithString("priority_class_name", mcp.Description("PriorityClass name for the DaemonSet (--priority-class-name)")),
		mcp.WithString("proxy_gid", mcp.Description("Run the proxy under this GID (--proxy-gid)")),
		mcp.WithString("proxy_uid", mcp.Description("Run the proxy under this UID (--proxy-uid)")),
		mcp.WithString("redirect_ports", mcp.Description("Ports to redirect to the proxy, comma-separated (--redirect-ports)")),
		mcp.WithString("registry", mcp.Description("Image registry for the plugin (--registry)")),
		mcp.WithString("set_overrides", mcp.Description("Comma-separated Helm style key=value overrides for --set")),
		mcp.WithString("set_string_overrides", mcp.Description("Comma-separated key=value pairs for --set-string")),
		mcp.WithString("set_file_overrides", mcp.Description("Comma-separated key=path pairs for --set-file")),
		mcp.WithString("skip_inbound_ports", mcp.Description("Ports that should bypass the proxy (--skip-inbound-ports)")),
		mcp.WithString("skip_outbound_ports", mcp.Description("Outbound ports that should bypass the proxy (--skip-outbound-ports)")),
		mcp.WithString("use_wait_flag", mcp.Description("Set to true to enable --use-wait-flag")),
		mcp.WithString("values", mcp.Description("Comma-separated list of values files or URLs for -f/--values")),
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
		mcp.WithString("proxy", mcp.Description("Set to true to print data-plane proxy versions (--proxy)")),
		mcp.WithString("namespace", mcp.Description("Namespace scope for proxy versions (-n/--namespace)")),
		mcp.WithString("short", mcp.Description("Set to true for short version output")),
	), telemetry.AdaptToolHandler(telemetry.WithTracing("linkerd_version", handleLinkerdVersion)))

	s.AddTool(mcp.NewTool("linkerd_authz",
		mcp.WithDescription("List Linkerd authorizations for a resource"),
		mcp.WithString("resource", mcp.Description("Resource to inspect, e.g. deploy/web"), mcp.Required()),
		mcp.WithString("namespace", mcp.Description("Namespace containing the resource")),
	), telemetry.AdaptToolHandler(telemetry.WithTracing("linkerd_authz", handleLinkerdAuthz)))

	s.AddTool(mcp.NewTool("linkerd_policy",
		mcp.WithDescription("Manage Linkerd policy operations like generate"),
		mcp.WithString("command", mcp.Description("Policy subcommand to execute (default: generate)")),
	), telemetry.AdaptToolHandler(telemetry.WithTracing("linkerd_policy", handleLinkerdPolicy)))

	s.AddTool(mcp.NewTool("linkerd_profile",
		mcp.WithDescription("Generate a service profile template or manifest"),
		mcp.WithString("service_name", mcp.Description("Service to profile (e.g. web-svc)"), mcp.Required()),
		mcp.WithString("namespace", mcp.Description("Namespace containing the service")),
		mcp.WithString("template", mcp.Description("Set to true to output a template (--template)")),
		mcp.WithString("open_api", mcp.Description("Path to an OpenAPI spec file (--open-api)")),
		mcp.WithString("proto", mcp.Description("Path to a protobuf definition file (--proto)")),
		mcp.WithString("ignore_cluster", mcp.Description("Set to true to enable --ignore-cluster")),
		mcp.WithString("output", mcp.Description("Output format (yaml or json)")),
	), telemetry.AdaptToolHandler(telemetry.WithTracing("linkerd_profile", handleLinkerdProfile)))

	s.AddTool(mcp.NewTool("linkerd_fips_audit",
		mcp.WithDescription("Audit Linkerd proxies for FIPS compliance"),
		mcp.WithString("namespace", mcp.Description("Namespace containing Linkerd components to audit")),
	), telemetry.AdaptToolHandler(telemetry.WithTracing("linkerd_fips_audit", handleLinkerdFips)))

	s.AddTool(mcp.NewTool("linkerd_diagnostics_proxy_metrics",
		mcp.WithDescription("Collect raw proxy metrics for Linkerd workloads"),
		mcp.WithString("resource", mcp.Description("Specific resource to query, e.g. deploy/web"), mcp.Required()),
		mcp.WithString("namespace", mcp.Description("Namespace containing the resource (defaults to current)")),
		mcp.WithString("obfuscate", mcp.Description("Set to true to obfuscate sensitive metric labels")),
	), telemetry.AdaptToolHandler(telemetry.WithTracing("linkerd_diagnostics_proxy_metrics", handleLinkerdDiagnosticsProxyMetrics)))

	s.AddTool(mcp.NewTool("linkerd_diagnostics_controller_metrics",
		mcp.WithDescription("Fetch metrics directly from Linkerd control-plane components"),
		mcp.WithString("namespace", mcp.Description("Namespace containing the control-plane pods")),
		mcp.WithString("component", mcp.Description("Specific control-plane component name")),
		mcp.WithString("wait", mcp.Description("Time allowed to fetch diagnostics, e.g. 30s")),
	), telemetry.AdaptToolHandler(telemetry.WithTracing("linkerd_diagnostics_controller_metrics", handleLinkerdDiagnosticsControllerMetrics)))

	s.AddTool(mcp.NewTool("linkerd_diagnostics_endpoints",
		mcp.WithDescription("Inspect Linkerd's service discovery endpoints for an authority"),
		mcp.WithString("authority", mcp.Description("Authority host:port to inspect"), mcp.Required()),
		mcp.WithString("namespace", mcp.Description("Namespace context for the query")),
		mcp.WithString("destination_pod", mcp.Description("Specific destination pod to query")),
		mcp.WithString("output", mcp.Description(`Output format ("table" or "json")`)),
		mcp.WithString("token", mcp.Description("Context token for destination API requests")),
	), telemetry.AdaptToolHandler(telemetry.WithTracing("linkerd_diagnostics_endpoints", handleLinkerdDiagnosticsEndpoints)))

	s.AddTool(mcp.NewTool("linkerd_diagnostics_policy",
		mcp.WithDescription("Inspect Linkerd's policy state for a resource"),
		mcp.WithString("resource", mcp.Description("Target resource (e.g. po/emoji-123, svc/web)"), mcp.Required()),
		mcp.WithString("port", mcp.Description("Port number to inspect"), mcp.Required()),
		mcp.WithString("namespace", mcp.Description("Namespace context for the query")),
		mcp.WithString("destination_pod", mcp.Description("Specific destination pod to query")),
		mcp.WithString("output", mcp.Description(`Output format ("yaml" or "json")`)),
		mcp.WithString("token", mcp.Description("Token used when querying the policy service")),
	), telemetry.AdaptToolHandler(telemetry.WithTracing("linkerd_diagnostics_policy", handleLinkerdDiagnosticsPolicy)))

	s.AddTool(mcp.NewTool("linkerd_diagnostics_profile",
		mcp.WithDescription("Inspect Linkerd's service discovery profile for an authority"),
		mcp.WithString("authority", mcp.Description("Authority host:port to inspect"), mcp.Required()),
		mcp.WithString("namespace", mcp.Description("Namespace context for the query")),
		mcp.WithString("destination_pod", mcp.Description("Specific destination pod to query")),
		mcp.WithString("token", mcp.Description("Context token for destination API requests")),
	), telemetry.AdaptToolHandler(telemetry.WithTracing("linkerd_diagnostics_profile", handleLinkerdDiagnosticsProfile)))
}
