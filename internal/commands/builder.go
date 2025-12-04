package commands

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/kagent-dev/tools/internal/cache"
	"github.com/kagent-dev/tools/internal/cmd"
	"github.com/kagent-dev/tools/internal/errors"
	"github.com/kagent-dev/tools/internal/logger"
	"github.com/kagent-dev/tools/internal/security"
	"github.com/kagent-dev/tools/internal/telemetry"
	"go.opentelemetry.io/otel/attribute"
)

const (
	// DefaultTimeout is the default timeout for command execution
	DefaultTimeout = 2 * time.Minute
	// DefaultCacheTTL is the default cache TTL
	DefaultCacheTTL = 1 * time.Minute
)

// CommandBuilder provides a fluent interface for building CLI commands
type CommandBuilder struct {
	command     string
	args        []string
	namespace   string
	context     string
	kubeconfig  string
	output      string
	labels      map[string]string
	annotations map[string]string
	timeout     time.Duration
	useTimeout  bool
	dryRun      bool
	force       bool
	wait        bool
	validate    bool
	cached      bool
	cacheTTL    time.Duration
	cacheKey    string
}

// NewCommandBuilder creates a new command builder
func NewCommandBuilder(command string) *CommandBuilder {
	return &CommandBuilder{
		command:     command,
		args:        make([]string, 0),
		labels:      make(map[string]string),
		annotations: make(map[string]string),
		timeout:     DefaultTimeout,
		useTimeout:  false, // Only enable timeout when explicitly requested
		validate:    true,
		cacheTTL:    DefaultCacheTTL,
	}
}

// KubectlBuilder creates a kubectl command builder
func KubectlBuilder() *CommandBuilder {
	return NewCommandBuilder("kubectl")
}

// HelmBuilder creates a helm command builder
func HelmBuilder() *CommandBuilder {
	return NewCommandBuilder("helm")
}

// IstioCtlBuilder creates an istioctl command builder
func IstioCtlBuilder() *CommandBuilder {
	return NewCommandBuilder("istioctl")
}

// CiliumBuilder creates a cilium command builder
func CiliumBuilder() *CommandBuilder {
	return NewCommandBuilder("cilium")
}

// ArgoRolloutsBuilder creates an argo rollouts command builder
func ArgoRolloutsBuilder() *CommandBuilder {
	return NewCommandBuilder("kubectl").WithArgs("argo", "rollouts")
}

// WithArgs adds arguments to the command
func (cb *CommandBuilder) WithArgs(args ...string) *CommandBuilder {
	cb.args = append(cb.args, args...)
	return cb
}

// WithNamespace sets the namespace
func (cb *CommandBuilder) WithNamespace(namespace string) *CommandBuilder {
	if err := security.ValidateNamespace(namespace); err != nil {
		logger.Get().Error("Invalid namespace", "namespace", namespace, "error", err)
		return cb
	}
	cb.namespace = namespace
	return cb
}

// WithContext sets the Kubernetes context
func (cb *CommandBuilder) WithContext(context string) *CommandBuilder {
	if err := security.ValidateCommandInput(context); err != nil {
		logger.Get().Error("Invalid context", "context", context, "error", err)
		return cb
	}
	cb.context = context
	return cb
}

// WithKubeconfig sets the kubeconfig file
func (cb *CommandBuilder) WithKubeconfig(kubeconfig string) *CommandBuilder {
	if kubeconfig != "" {
		if err := security.ValidateFilePath(kubeconfig); err != nil {
			logger.Get().Error("Invalid kubeconfig path", "kubeconfig", kubeconfig, "error", err)
			return cb
		}
		cb.kubeconfig = kubeconfig
	}
	return cb
}

// WithOutput sets the output format
func (cb *CommandBuilder) WithOutput(output string) *CommandBuilder {
	validOutputs := []string{"json", "yaml", "wide", "name", "custom-columns", "custom-columns-file", "go-template", "go-template-file", "jsonpath", "jsonpath-file"}

	valid := false
	for _, validOutput := range validOutputs {
		if output == validOutput {
			valid = true
			break
		}
	}

	if !valid {
		logger.Get().Error("Invalid output format", "output", output)
		return cb
	}

	cb.output = output
	return cb
}

// WithLabel adds a label selector
func (cb *CommandBuilder) WithLabel(key, value string) *CommandBuilder {
	if err := security.ValidateK8sLabel(key, value); err != nil {
		logger.Get().Error("Invalid label", "key", key, "value", value, "error", err)
		return cb
	}
	cb.labels[key] = value
	return cb
}

// WithLabels adds multiple label selectors
func (cb *CommandBuilder) WithLabels(labels map[string]string) *CommandBuilder {
	for key, value := range labels {
		cb.WithLabel(key, value)
	}
	return cb
}

// WithAnnotation adds an annotation
func (cb *CommandBuilder) WithAnnotation(key, value string) *CommandBuilder {
	if err := security.ValidateK8sLabel(key, value); err != nil {
		logger.Get().Error("Invalid annotation", "key", key, "value", value, "error", err)
		return cb
	}
	cb.annotations[key] = value
	return cb
}

// WithTimeout sets the command timeout
func (cb *CommandBuilder) WithTimeout(timeout time.Duration) *CommandBuilder {
	cb.useTimeout = true
	cb.timeout = timeout
	return cb
}

// WithDryRun enables dry run mode
func (cb *CommandBuilder) WithDryRun(dryRun bool) *CommandBuilder {
	cb.dryRun = dryRun
	return cb
}

// WithForce enables force mode
func (cb *CommandBuilder) WithForce(force bool) *CommandBuilder {
	cb.force = force
	return cb
}

// WithWait enables wait mode
func (cb *CommandBuilder) WithWait(wait bool) *CommandBuilder {
	cb.wait = wait
	return cb
}

// WithValidation enables/disables validation
func (cb *CommandBuilder) WithValidation(validate bool) *CommandBuilder {
	cb.validate = validate
	return cb
}

// WithCache enables caching of the command result
func (cb *CommandBuilder) WithCache(cached bool) *CommandBuilder {
	cb.cached = cached
	return cb
}

// WithCacheTTL sets the cache TTL
func (cb *CommandBuilder) WithCacheTTL(ttl time.Duration) *CommandBuilder {
	cb.cacheTTL = ttl
	return cb
}

// WithCacheKey sets a custom cache key
func (cb *CommandBuilder) WithCacheKey(key string) *CommandBuilder {
	cb.cacheKey = key
	return cb
}

// Build constructs the final command arguments
func (cb *CommandBuilder) Build() (string, []string, error) {
	args := make([]string, 0, len(cb.args)+20)

	// Add main arguments
	args = append(args, cb.args...)

	// Add namespace if specified
	if cb.namespace != "" {
		args = append(args, "--namespace", cb.namespace)
	}

	// Add context if specified
	if cb.context != "" {
		args = append(args, "--context", cb.context)
	}

	// Add kubeconfig if specified
	if cb.kubeconfig != "" {
		args = append(args, "--kubeconfig", cb.kubeconfig)
	}

	// Add output format
	if cb.output != "" {
		args = append(args, "--output", cb.output)
	}

	// Add label selectors
	if len(cb.labels) > 0 {
		var labelSelectors []string
		for key, value := range cb.labels {
			if value != "" {
				labelSelectors = append(labelSelectors, fmt.Sprintf("%s=%s", key, value))
			} else {
				labelSelectors = append(labelSelectors, key)
			}
		}
		if len(labelSelectors) > 0 {
			args = append(args, "--selector", strings.Join(labelSelectors, ","))
		}
	}

	// Add timeout when explicitly requested
	if cb.timeout > 0 && cb.useTimeout {
		args = append(args, "--timeout", cb.timeout.String())
	}

	// Add dry run
	if cb.dryRun {
		args = append(args, "--dry-run=client")
	}

	// Add force
	if cb.force {
		args = append(args, "--force")
	}

	// Add wait
	if cb.wait {
		args = append(args, "--wait")
	}

	// Add validation
	if !cb.validate {
		args = append(args, "--validate=false")
	}

	return cb.command, args, nil
}

// Execute runs the command
func (cb *CommandBuilder) Execute(ctx context.Context) (string, error) {
	log := logger.WithContext(ctx)
	_, span := telemetry.StartSpan(ctx, "commands.execute",
		attribute.String("command", cb.command),
		attribute.StringSlice("args", cb.args),
		attribute.Bool("cached", cb.cached),
	)
	defer span.End()

	command, args, err := cb.Build()
	if err != nil {
		telemetry.RecordError(span, err, "Command build failed")
		log.Error("failed to build command",
			"command", cb.command,
			"error", err,
		)
		return "", err
	}

	span.SetAttributes(
		attribute.String("built_command", command),
		attribute.StringSlice("built_args", args),
	)

	log.Debug("executing command",
		"command", command,
		"args", args,
		"cached", cb.cached,
	)

	// Generate cache key if caching is enabled
	if cb.cached {
		telemetry.AddEvent(span, "execution.cached")
		return cb.executeWithCache(ctx, command, args)
	}

	// Execute the command
	telemetry.AddEvent(span, "execution.direct")
	result, err := cb.executeCommand(ctx, command, args)
	if err != nil {
		telemetry.RecordError(span, err, "Command execution failed")
		return "", err
	}

	telemetry.RecordSuccess(span, "Command executed successfully")
	span.SetAttributes(
		attribute.Int("result_length", len(result)),
	)

	return result, nil
}

func (cb *CommandBuilder) executeWithCache(ctx context.Context, command string, args []string) (string, error) {
	log := logger.WithContext(ctx)
	_, span := telemetry.StartSpan(ctx, "commands.executeWithCache",
		attribute.String("command", command),
		attribute.StringSlice("args", args),
		attribute.Bool("cached", true),
	)
	defer span.End()

	cacheKey := cb.cacheKey
	if cacheKey == "" {
		cacheKey = cache.CacheKey(append([]string{command}, args...)...)
	}

	log.Info("executing cached command",
		"command", command,
		"args", args,
		"cache_key", cacheKey,
		"cache_ttl", cb.cacheTTL.String(),
	)

	// Try to get from cache first
	cacheInstance := cache.GetCacheByCommand(command)

	telemetry.AddEvent(span, "cache.lookup",
		attribute.String("cache_key", cacheKey),
		attribute.String("cache_ttl", cb.cacheTTL.String()),
	)

	result, err := cache.CacheResult(cacheInstance, cacheKey, cb.cacheTTL, func() (string, error) {
		telemetry.AddEvent(span, "cache.miss.executing_command")
		log.Debug("cache miss, executing command",
			"command", command,
			"args", args,
		)
		return cb.executeCommand(ctx, command, args)
	})

	if err != nil {
		telemetry.RecordError(span, err, "Cached command execution failed")
		log.Error("cached command execution failed",
			"command", command,
			"args", args,
			"cache_key", cacheKey,
			"error", err,
		)
		return "", err
	}

	telemetry.RecordSuccess(span, "Cached command executed successfully")
	log.Info("cached command execution successful",
		"command", command,
		"args", args,
		"cache_key", cacheKey,
		"result_length", len(result),
	)

	span.SetAttributes(
		attribute.String("cache_key", cacheKey),
		attribute.Int("result_length", len(result)),
	)

	return result, nil
}

// executeCommand executes the actual command
func (cb *CommandBuilder) executeCommand(ctx context.Context, command string, args []string) (string, error) {
	executor := cmd.GetShellExecutor(ctx)
	output, err := executor.Exec(ctx, command, args...)
	if err != nil {
		// Create appropriate error based on command type
		var toolError *errors.ToolError
		switch command {
		case "kubectl":
			toolError = errors.NewKubernetesError(strings.Join(args, " "), err)
		case "helm":
			toolError = errors.NewHelmError(strings.Join(args, " "), err)
		case "istioctl":
			toolError = errors.NewIstioError(strings.Join(args, " "), err)
		case "cilium":
			toolError = errors.NewCiliumError(strings.Join(args, " "), err)
		default:
			toolError = errors.NewCommandError(command, err)
		}
		return string(output), toolError
	}

	return string(output), nil
}

// Common command patterns as helper functions

// GetPods creates a command to get pods
func GetPods(namespace string, labels map[string]string) *CommandBuilder {
	builder := KubectlBuilder().WithArgs("get", "pods")

	if namespace != "" {
		builder = builder.WithNamespace(namespace)
	}

	if len(labels) > 0 {
		builder = builder.WithLabels(labels)
	}

	return builder.WithCache(true)
}

// GetServices creates a command to get services
func GetServices(namespace string, labels map[string]string) *CommandBuilder {
	builder := KubectlBuilder().WithArgs("get", "services")

	if namespace != "" {
		builder = builder.WithNamespace(namespace)
	}

	if len(labels) > 0 {
		builder = builder.WithLabels(labels)
	}

	return builder.WithCache(true)
}

// GetDeployments creates a command to get deployments
func GetDeployments(namespace string, labels map[string]string) *CommandBuilder {
	builder := KubectlBuilder().WithArgs("get", "deployments")

	if namespace != "" {
		builder = builder.WithNamespace(namespace)
	}

	if len(labels) > 0 {
		builder = builder.WithLabels(labels)
	}

	return builder.WithCache(true)
}

// DescribeResource creates a command to describe a resource
func DescribeResource(resourceType, resourceName, namespace string) *CommandBuilder {
	builder := KubectlBuilder().WithArgs("describe", resourceType, resourceName)

	if namespace != "" {
		builder = builder.WithNamespace(namespace)
	}

	return builder.WithCache(true).WithCacheTTL(2 * time.Minute)
}

// GetLogs creates a command to get logs
func GetLogs(podName, namespace string, options LogOptions) *CommandBuilder {
	builder := KubectlBuilder().WithArgs("logs", podName)

	if namespace != "" {
		builder = builder.WithNamespace(namespace)
	}

	if options.Container != "" {
		builder = builder.WithArgs("--container", options.Container)
	}

	if options.Follow {
		builder = builder.WithArgs("--follow")
	}

	if options.Previous {
		builder = builder.WithArgs("--previous")
	}

	if options.Timestamps {
		builder = builder.WithArgs("--timestamps")
	}

	if options.TailLines > 0 {
		builder = builder.WithArgs("--tail", fmt.Sprintf("%d", options.TailLines))
	}

	if options.SinceTime != "" {
		builder = builder.WithArgs("--since-time", options.SinceTime)
	}

	if options.SinceDuration != "" {
		builder = builder.WithArgs("--since", options.SinceDuration)
	}

	// Don't cache logs by default as they change frequently
	return builder.WithCache(false)
}

// LogOptions represents options for log commands
type LogOptions struct {
	Container     string
	Follow        bool
	Previous      bool
	Timestamps    bool
	TailLines     int
	SinceTime     string
	SinceDuration string
}

// ApplyResource creates a command to apply a resource
func ApplyResource(filename string, namespace string, options ApplyOptions) *CommandBuilder {
	builder := KubectlBuilder().WithArgs("apply", "-f", filename)

	if namespace != "" {
		builder = builder.WithNamespace(namespace)
	}

	if options.DryRun {
		builder = builder.WithDryRun(true)
	}

	if options.Force {
		builder = builder.WithForce(true)
	}

	if options.Wait {
		builder = builder.WithWait(true)
	}

	if !options.Validate {
		builder = builder.WithValidation(false)
	}

	return builder.WithCache(false) // Don't cache apply operations
}

// ApplyOptions represents options for apply commands
type ApplyOptions struct {
	DryRun   bool
	Force    bool
	Wait     bool
	Validate bool
}

// DeleteResource creates a command to delete a resource
func DeleteResource(resourceType, resourceName, namespace string, options DeleteOptions) *CommandBuilder {
	builder := KubectlBuilder().WithArgs("delete", resourceType, resourceName)

	if namespace != "" {
		builder = builder.WithNamespace(namespace)
	}

	if options.Force {
		builder = builder.WithForce(true)
	}

	if options.GracePeriod >= 0 {
		builder = builder.WithArgs("--grace-period", fmt.Sprintf("%d", options.GracePeriod))
	}

	if options.Wait {
		builder = builder.WithWait(true)
	}

	return builder.WithCache(false) // Don't cache delete operations
}

// DeleteOptions represents options for delete commands
type DeleteOptions struct {
	Force       bool
	GracePeriod int
	Wait        bool
}

// HelmInstall creates a command to install a Helm chart
func HelmInstall(releaseName, chart, namespace string, options HelmInstallOptions) *CommandBuilder {
	builder := HelmBuilder().WithArgs("install", releaseName, chart)

	if namespace != "" {
		builder = builder.WithNamespace(namespace)
	}

	if options.CreateNamespace {
		builder = builder.WithArgs("--create-namespace")
	}

	if options.DryRun {
		builder = builder.WithDryRun(true)
	}

	if options.Wait {
		builder = builder.WithWait(true)
	}

	if options.ValuesFile != "" {
		builder = builder.WithArgs("--values", options.ValuesFile)
	}

	for key, value := range options.SetValues {
		builder = builder.WithArgs("--set", fmt.Sprintf("%s=%s", key, value))
	}

	return builder.WithCache(false) // Don't cache install operations
}

// HelmInstallOptions represents options for Helm install commands
type HelmInstallOptions struct {
	CreateNamespace bool
	DryRun          bool
	Wait            bool
	ValuesFile      string
	SetValues       map[string]string
}

// HelmList creates a command to list Helm releases
func HelmList(namespace string, options HelmListOptions) *CommandBuilder {
	builder := HelmBuilder().WithArgs("list")

	if namespace != "" {
		builder = builder.WithNamespace(namespace)
	}

	if options.AllNamespaces {
		builder = builder.WithArgs("--all-namespaces")
	}

	if options.Output != "" {
		builder = builder.WithOutput(options.Output)
	}

	return builder.WithCache(true).WithCacheTTL(2 * time.Minute)
}

// HelmListOptions represents options for Helm list commands
type HelmListOptions struct {
	AllNamespaces bool
	Output        string
}

// IstioProxyStatus creates a command to get Istio proxy status
func IstioProxyStatus(podName, namespace string) *CommandBuilder {
	builder := IstioCtlBuilder().WithArgs("proxy-status")

	if namespace != "" {
		builder = builder.WithNamespace(namespace)
	}

	if podName != "" {
		builder = builder.WithArgs(podName)
	}

	return builder.WithCache(true).WithCacheTTL(30 * time.Second)
}

// CiliumStatus creates a command to get Cilium status
func CiliumStatus() *CommandBuilder {
	return CiliumBuilder().WithArgs("status").WithCache(true).WithCacheTTL(30 * time.Second)
}

// ArgoRolloutsGet creates a command to get Argo rollouts
func ArgoRolloutsGet(rolloutName, namespace string) *CommandBuilder {
	builder := ArgoRolloutsBuilder().WithArgs("get", "rollout")

	if rolloutName != "" {
		builder = builder.WithArgs(rolloutName)
	}

	if namespace != "" {
		builder = builder.WithNamespace(namespace)
	}

	return builder.WithCache(true).WithCacheTTL(1 * time.Minute)
}
