package commands

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewCommandBuilder(t *testing.T) {
	cb := NewCommandBuilder("test-command")

	assert.Equal(t, "test-command", cb.command)
	assert.Empty(t, cb.args)
	assert.Empty(t, cb.namespace)
	assert.Empty(t, cb.context)
	assert.Empty(t, cb.kubeconfig)
	assert.Empty(t, cb.output)
	assert.NotNil(t, cb.labels)
	assert.NotNil(t, cb.annotations)
	assert.Equal(t, DefaultTimeout, cb.timeout)
	assert.Equal(t, DefaultCacheTTL, cb.cacheTTL)
	assert.True(t, cb.validate)
	assert.False(t, cb.cached)
	assert.False(t, cb.dryRun)
	assert.False(t, cb.force)
	assert.False(t, cb.wait)
}

func TestCommandBuilderFactories(t *testing.T) {
	tests := []struct {
		name     string
		factory  func() *CommandBuilder
		expected string
	}{
		{"kubectl", KubectlBuilder, "kubectl"},
		{"helm", HelmBuilder, "helm"},
		{"istioctl", IstioCtlBuilder, "istioctl"},
		{"cilium", CiliumBuilder, "cilium"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cb := tt.factory()
			assert.Equal(t, tt.expected, cb.command)
		})
	}
}

func TestArgoRolloutsBuilder(t *testing.T) {
	cb := ArgoRolloutsBuilder()

	assert.Equal(t, "kubectl", cb.command)
	assert.Equal(t, []string{"argo", "rollouts"}, cb.args)
}

func TestCommandBuilderWithArgs(t *testing.T) {
	cb := NewCommandBuilder("test").WithArgs("arg1", "arg2")

	assert.Equal(t, []string{"arg1", "arg2"}, cb.args)

	// Test chaining
	cb.WithArgs("arg3")
	assert.Equal(t, []string{"arg1", "arg2", "arg3"}, cb.args)
}

func TestCommandBuilderWithNamespace(t *testing.T) {
	cb := NewCommandBuilder("test").WithNamespace("default")

	assert.Equal(t, "default", cb.namespace)

	// Test invalid namespace - should not set the namespace
	cb.WithNamespace("invalid..namespace")
	assert.Equal(t, "default", cb.namespace) // Should remain unchanged
}

func TestCommandBuilderWithContext(t *testing.T) {
	cb := NewCommandBuilder("test").WithContext("minikube")

	assert.Equal(t, "minikube", cb.context)
}

func TestCommandBuilderWithKubeconfig(t *testing.T) {
	cb := NewCommandBuilder("test").WithKubeconfig("/path/to/config")

	assert.Equal(t, "/path/to/config", cb.kubeconfig)
}

func TestCommandBuilderWithOutput(t *testing.T) {
	validOutputs := []string{"json", "yaml", "wide", "name"}

	for _, output := range validOutputs {
		cb := NewCommandBuilder("test").WithOutput(output)
		assert.Equal(t, output, cb.output)
	}

	// Test invalid output
	cb := NewCommandBuilder("test").WithOutput("invalid")
	assert.Empty(t, cb.output)
}

func TestCommandBuilderWithLabel(t *testing.T) {
	cb := NewCommandBuilder("test").WithLabel("app", "web")

	assert.Equal(t, "web", cb.labels["app"])
}

func TestCommandBuilderWithLabels(t *testing.T) {
	labels := map[string]string{
		"app":     "web",
		"version": "v1.0.0",
	}

	cb := NewCommandBuilder("test").WithLabels(labels)

	assert.Equal(t, labels["app"], cb.labels["app"])
	assert.Equal(t, labels["version"], cb.labels["version"])
}

func TestCommandBuilderWithAnnotation(t *testing.T) {
	cb := NewCommandBuilder("test").WithAnnotation("simple-key", "value")

	// The annotation should be accepted if it's a valid format
	assert.Equal(t, "value", cb.annotations["simple-key"])

	// Test with invalid annotation - still gets added but logs an error
	cb2 := NewCommandBuilder("test").WithAnnotation("invalid..key", "value")
	assert.Equal(t, "value", cb2.annotations["invalid..key"]) // Invalid annotations are still added but logged
}

func TestCommandBuilderWithTimeout(t *testing.T) {
	timeout := 60 * time.Second
	cb := NewCommandBuilder("test").WithTimeout(timeout)

	assert.Equal(t, timeout, cb.timeout)
}

func TestCommandBuilderWithFlags(t *testing.T) {
	cb := NewCommandBuilder("test").
		WithDryRun(true).
		WithForce(true).
		WithWait(true).
		WithValidation(false)

	assert.True(t, cb.dryRun)
	assert.True(t, cb.force)
	assert.True(t, cb.wait)
	assert.False(t, cb.validate)
}

func TestCommandBuilderWithCache(t *testing.T) {
	cb := NewCommandBuilder("test").WithCache(true)

	assert.True(t, cb.cached)
}

func TestCommandBuilderWithCacheTTL(t *testing.T) {
	ttl := 10 * time.Minute
	cb := NewCommandBuilder("test").WithCacheTTL(ttl)

	assert.Equal(t, ttl, cb.cacheTTL)
}

func TestCommandBuilderWithCacheKey(t *testing.T) {
	cb := NewCommandBuilder("test").WithCacheKey("custom-key")

	assert.Equal(t, "custom-key", cb.cacheKey)
}

func TestCommandBuilderBuild(t *testing.T) {
	cb := NewCommandBuilder("kubectl").
		WithArgs("get", "pods").
		WithNamespace("default").
		WithContext("minikube").
		WithKubeconfig("/path/to/config").
		WithOutput("json").
		WithLabel("app", "web").
		WithDryRun(true).
		WithForce(true).
		WithWait(true).
		WithValidation(false)

	command, args, err := cb.Build()
	require.NoError(t, err)

	assert.Equal(t, "kubectl", command)
	assert.Contains(t, args, "get")
	assert.Contains(t, args, "pods")
	assert.Contains(t, args, "--namespace")
	assert.Contains(t, args, "default")
	assert.Contains(t, args, "--context")
	assert.Contains(t, args, "minikube")
	assert.Contains(t, args, "--kubeconfig")
	assert.Contains(t, args, "/path/to/config")
	assert.Contains(t, args, "--output")
	assert.Contains(t, args, "json")
	assert.Contains(t, args, "--selector")
	assert.Contains(t, args, "app=web")
	assert.Contains(t, args, "--dry-run=client")
	assert.Contains(t, args, "--force")
	assert.Contains(t, args, "--wait")
	assert.Contains(t, args, "--validate=false")
}

func TestCommandBuilderBuildWithTimeout(t *testing.T) {
	cb := NewCommandBuilder("kubectl").
		WithArgs("delete", "pod", "test-pod").
		WithTimeout(45 * time.Second)

	command, args, err := cb.Build()
	require.NoError(t, err)

	assert.Equal(t, "kubectl", command)
	assert.Contains(t, args, "--timeout")
	assert.Contains(t, args, "45s")
}

func TestCommandBuilderBuildWithMultipleLabels(t *testing.T) {
	cb := NewCommandBuilder("kubectl").
		WithArgs("get", "pods").
		WithLabel("app", "web").
		WithLabel("version", "v1.0.0")

	command, args, err := cb.Build()
	require.NoError(t, err)

	assert.Equal(t, "kubectl", command)
	assert.Contains(t, args, "--selector")

	// Find the selector argument
	var selectorValue string
	for i, arg := range args {
		if arg == "--selector" && i+1 < len(args) {
			selectorValue = args[i+1]
			break
		}
	}

	assert.Contains(t, selectorValue, "app=web")
	assert.Contains(t, selectorValue, "version=v1.0.0")
}

func TestGetPods(t *testing.T) {
	namespace := "default"
	labels := map[string]string{"app": "web"}

	cb := GetPods(namespace, labels)

	assert.Equal(t, "kubectl", cb.command)
	assert.Contains(t, cb.args, "get")
	assert.Contains(t, cb.args, "pods")
	assert.Equal(t, namespace, cb.namespace)
	assert.Equal(t, labels, cb.labels)
	assert.True(t, cb.cached)
	assert.Empty(t, cb.output) // No default output format
}

func TestGetServices(t *testing.T) {
	namespace := "default"
	labels := map[string]string{"app": "web"}

	cb := GetServices(namespace, labels)

	assert.Equal(t, "kubectl", cb.command)
	assert.Contains(t, cb.args, "get")
	assert.Contains(t, cb.args, "services")
	assert.Equal(t, namespace, cb.namespace)
	assert.Equal(t, labels, cb.labels)
	assert.True(t, cb.cached)
	assert.Empty(t, cb.output) // No default output format
}

func TestGetDeployments(t *testing.T) {
	namespace := "default"
	labels := map[string]string{"app": "web"}

	cb := GetDeployments(namespace, labels)

	assert.Equal(t, "kubectl", cb.command)
	assert.Contains(t, cb.args, "get")
	assert.Contains(t, cb.args, "deployments")
	assert.Equal(t, namespace, cb.namespace)
	assert.Equal(t, labels, cb.labels)
	assert.True(t, cb.cached)
	assert.Empty(t, cb.output) // No default output format
}

func TestDescribeResource(t *testing.T) {
	resourceType := "pod"
	resourceName := "test-pod"
	namespace := "default"

	cb := DescribeResource(resourceType, resourceName, namespace)

	assert.Equal(t, "kubectl", cb.command)
	assert.Contains(t, cb.args, "describe")
	assert.Contains(t, cb.args, resourceType)
	assert.Contains(t, cb.args, resourceName)
	assert.Equal(t, namespace, cb.namespace)
	assert.True(t, cb.cached)
	assert.Equal(t, 2*time.Minute, cb.cacheTTL)
}

func TestGetLogs(t *testing.T) {
	podName := "test-pod"
	namespace := "default"
	options := LogOptions{
		Container:     "app",
		Follow:        true,
		Previous:      false,
		Timestamps:    true,
		TailLines:     100,
		SinceTime:     "2023-01-01T00:00:00Z",
		SinceDuration: "1h",
	}

	cb := GetLogs(podName, namespace, options)

	assert.Equal(t, "kubectl", cb.command)
	assert.Contains(t, cb.args, "logs")
	assert.Contains(t, cb.args, podName)
	assert.Equal(t, namespace, cb.namespace)
	assert.Contains(t, cb.args, "--container")
	assert.Contains(t, cb.args, "app")
	assert.Contains(t, cb.args, "--follow")
	assert.Contains(t, cb.args, "--timestamps")
	assert.Contains(t, cb.args, "--tail")
	assert.Contains(t, cb.args, "100")
	assert.Contains(t, cb.args, "--since-time")
	assert.Contains(t, cb.args, "2023-01-01T00:00:00Z")
	assert.Contains(t, cb.args, "--since")
	assert.Contains(t, cb.args, "1h")
	assert.False(t, cb.cached)
}

func TestGetLogsWithPrevious(t *testing.T) {
	podName := "test-pod"
	namespace := "default"
	options := LogOptions{
		Previous: true,
	}

	cb := GetLogs(podName, namespace, options)

	assert.Contains(t, cb.args, "--previous")
}

func TestApplyResource(t *testing.T) {
	filename := "/path/to/resource.yaml"
	namespace := "default"
	options := ApplyOptions{
		DryRun:   true,
		Force:    true,
		Wait:     true,
		Validate: false,
	}

	cb := ApplyResource(filename, namespace, options)

	assert.Equal(t, "kubectl", cb.command)
	assert.Contains(t, cb.args, "apply")
	assert.Contains(t, cb.args, "-f")
	assert.Contains(t, cb.args, filename)
	assert.Equal(t, namespace, cb.namespace)
	assert.True(t, cb.dryRun)
	assert.True(t, cb.force)
	assert.True(t, cb.wait)
	assert.False(t, cb.validate)
	assert.False(t, cb.cached)
}

func TestDeleteResource(t *testing.T) {
	resourceType := "pod"
	resourceName := "test-pod"
	namespace := "default"
	options := DeleteOptions{
		Force:       true,
		GracePeriod: 30,
		Wait:        true,
	}

	cb := DeleteResource(resourceType, resourceName, namespace, options)

	assert.Equal(t, "kubectl", cb.command)
	assert.Contains(t, cb.args, "delete")
	assert.Contains(t, cb.args, resourceType)
	assert.Contains(t, cb.args, resourceName)
	assert.Equal(t, namespace, cb.namespace)
	assert.True(t, cb.force)
	assert.True(t, cb.wait)
	assert.False(t, cb.cached)
}

func TestHelmInstall(t *testing.T) {
	releaseName := "test-release"
	chart := "bitnami/nginx"
	namespace := "default"
	options := HelmInstallOptions{
		CreateNamespace: true,
		DryRun:          true,
		Wait:            true,
		ValuesFile:      "/path/to/values.yaml",
		SetValues:       map[string]string{"image.tag": "1.20"},
	}

	cb := HelmInstall(releaseName, chart, namespace, options)

	assert.Equal(t, "helm", cb.command)
	assert.Contains(t, cb.args, "install")
	assert.Contains(t, cb.args, releaseName)
	assert.Contains(t, cb.args, chart)
	assert.Equal(t, namespace, cb.namespace)
	assert.True(t, cb.dryRun)
	assert.True(t, cb.wait)
	assert.False(t, cb.cached)
}

func TestHelmList(t *testing.T) {
	namespace := "default"
	options := HelmListOptions{
		AllNamespaces: true,
		Output:        "json",
	}

	cb := HelmList(namespace, options)

	assert.Equal(t, "helm", cb.command)
	assert.Contains(t, cb.args, "list")
	assert.Equal(t, namespace, cb.namespace)
	assert.Equal(t, "json", cb.output)
	assert.True(t, cb.cached)
}

func TestIstioProxyStatus(t *testing.T) {
	podName := "test-pod"
	namespace := "default"

	cb := IstioProxyStatus(podName, namespace)

	assert.Equal(t, "istioctl", cb.command)
	assert.Contains(t, cb.args, "proxy-status")
	assert.Contains(t, cb.args, podName)
	assert.Equal(t, namespace, cb.namespace)
	assert.True(t, cb.cached)
}

func TestCiliumStatus(t *testing.T) {
	cb := CiliumStatus()

	assert.Equal(t, "cilium", cb.command)
	assert.Contains(t, cb.args, "status")
	assert.Empty(t, cb.output) // CiliumStatus doesn't set output format
	assert.True(t, cb.cached)
}

func TestArgoRolloutsGet(t *testing.T) {
	rolloutName := "test-rollout"
	namespace := "default"

	cb := ArgoRolloutsGet(rolloutName, namespace)

	assert.Equal(t, "kubectl", cb.command)
	assert.Contains(t, cb.args, "argo")
	assert.Contains(t, cb.args, "rollouts")
	assert.Contains(t, cb.args, "get")
	assert.Contains(t, cb.args, "rollout")
	assert.Contains(t, cb.args, rolloutName)
	assert.Equal(t, namespace, cb.namespace)
	assert.Empty(t, cb.output) // ArgoRolloutsGet doesn't set output format
	assert.True(t, cb.cached)
}

func TestCommandBuilderChaining(t *testing.T) {
	cb := NewCommandBuilder("kubectl").
		WithArgs("get", "pods").
		WithNamespace("default").
		WithOutput("json").
		WithLabel("app", "web").
		WithTimeout(60 * time.Second).
		WithCache(true).
		WithCacheTTL(10 * time.Minute)

	assert.Equal(t, "kubectl", cb.command)
	assert.Equal(t, []string{"get", "pods"}, cb.args)
	assert.Equal(t, "default", cb.namespace)
	assert.Equal(t, "json", cb.output)
	assert.Equal(t, "web", cb.labels["app"])
	assert.Equal(t, 60*time.Second, cb.timeout)
	assert.True(t, cb.cached)
	assert.Equal(t, 10*time.Minute, cb.cacheTTL)
}

func TestCommandBuilderEmptyNamespace(t *testing.T) {
	cb := GetPods("", nil)

	assert.Empty(t, cb.namespace)
}

func TestCommandBuilderEmptyLabels(t *testing.T) {
	cb := GetPods("default", nil)

	assert.Empty(t, cb.labels)
}

func TestLogOptionsDefaults(t *testing.T) {
	options := LogOptions{}

	assert.False(t, options.Follow)
	assert.False(t, options.Previous)
	assert.False(t, options.Timestamps)
	assert.Equal(t, 0, options.TailLines)
	assert.Empty(t, options.SinceTime)
	assert.Empty(t, options.SinceDuration)
}

func TestApplyOptionsDefaults(t *testing.T) {
	options := ApplyOptions{}

	assert.False(t, options.DryRun)
	assert.False(t, options.Force)
	assert.False(t, options.Wait)
	assert.False(t, options.Validate)
}

func TestDeleteOptionsDefaults(t *testing.T) {
	options := DeleteOptions{}

	assert.False(t, options.Force)
	assert.Equal(t, 0, options.GracePeriod)
	assert.False(t, options.Wait)
}

func TestHelmInstallOptionsDefaults(t *testing.T) {
	options := HelmInstallOptions{}

	assert.False(t, options.CreateNamespace)
	assert.False(t, options.DryRun)
	assert.False(t, options.Wait)
	assert.Empty(t, options.ValuesFile)
	assert.Nil(t, options.SetValues)
}

func TestHelmListOptionsDefaults(t *testing.T) {
	options := HelmListOptions{}

	assert.False(t, options.AllNamespaces)
	assert.Empty(t, options.Output)
}

// Mock tests for Execute method - these would need a mock for utils.RunCommandWithContext
func TestCommandBuilderExecuteWithoutCache(t *testing.T) {
	cb := NewCommandBuilder("echo").
		WithArgs("hello", "world").
		WithCache(false)

	// This test would need mocking to work properly
	// For now, we'll just verify the command building part
	command, args, err := cb.Build()
	require.NoError(t, err)

	assert.Equal(t, "echo", command)
	assert.Contains(t, args, "hello")
	assert.Contains(t, args, "world")
}

func TestCommandBuilderExecuteWithCache(t *testing.T) {
	cb := NewCommandBuilder("echo").
		WithArgs("hello", "world").
		WithCache(true)

	// This test would need mocking to work properly
	// For now, we'll just verify the command building part
	command, args, err := cb.Build()
	require.NoError(t, err)

	assert.Equal(t, "echo", command)
	assert.Contains(t, args, "hello")
	assert.Contains(t, args, "world")
	assert.True(t, cb.cached)
}
