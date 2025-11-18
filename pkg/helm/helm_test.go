package helm

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/kagent-dev/tools/internal/cmd"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRegisterTools(t *testing.T) {
	server := mcp.NewServer(&mcp.Implementation{
		Name:    "test-server",
		Version: "v0.0.1",
	}, nil)
	err := RegisterTools(server)
	assert.NoError(t, err)
}

// Helper function to create MCP request with arguments
func createMCPRequest(args map[string]interface{}) *mcp.CallToolRequest {
	argsJSON, _ := json.Marshal(args)
	return &mcp.CallToolRequest{
		Params: &mcp.CallToolParamsRaw{
			Arguments: argsJSON,
		},
	}
}

// Helper function to extract text content from MCP result
func getResultText(result *mcp.CallToolResult) string {
	if result == nil || len(result.Content) == 0 {
		return ""
	}
	if textContent, ok := result.Content[0].(*mcp.TextContent); ok {
		return textContent.Text
	}
	return ""
}

// Test Helm List Releases
func TestHandleHelmListReleases(t *testing.T) {
	t.Run("basic_list_releases", func(t *testing.T) {
		mock := cmd.NewMockShellExecutor()
		expectedOutput := `NAME    NAMESPACE       REVISION        STATUS          CHART
app1    default         1               deployed        my-chart-1.0.0
app2    default         2               deployed        my-chart-2.0.0`

		mock.AddCommandString("helm", []string{"list"}, expectedOutput, nil)
		ctx := cmd.WithShellExecutor(context.Background(), mock)

		request := createMCPRequest(map[string]interface{}{})
		result, err := handleHelmListReleases(ctx, request)

		assert.NoError(t, err)
		assert.False(t, result.IsError)

		content := getResultText(result)
		assert.Contains(t, content, "app1")
		assert.Contains(t, content, "app2")

		// Verify the correct command was called
		callLog := mock.GetCallLog()
		require.Len(t, callLog, 1)
		assert.Equal(t, "helm", callLog[0].Command)
		assert.Equal(t, []string{"list"}, callLog[0].Args)
	})

	t.Run("helm command failure", func(t *testing.T) {
		mock := cmd.NewMockShellExecutor()
		mock.AddCommandString("helm", []string{"list"}, "", assert.AnError)
		ctx := cmd.WithShellExecutor(context.Background(), mock)

		request := createMCPRequest(map[string]interface{}{})
		result, err := handleHelmListReleases(ctx, request)

		assert.NoError(t, err) // MCP handlers should not return Go errors
		assert.True(t, result.IsError)
		assert.Contains(t, getResultText(result), "list failed")
	})

	t.Run("list_with_namespace", func(t *testing.T) {
		mock := cmd.NewMockShellExecutor()
		expectedOutput := "app1 kube-system"
		mock.AddCommandString("helm", []string{"list", "-n", "kube-system"}, expectedOutput, nil)
		ctx := cmd.WithShellExecutor(context.Background(), mock)

		request := createMCPRequest(map[string]interface{}{
			"namespace": "kube-system",
		})
		result, err := handleHelmListReleases(ctx, request)

		assert.NoError(t, err)
		assert.False(t, result.IsError)
	})

	t.Run("list_all_namespaces", func(t *testing.T) {
		mock := cmd.NewMockShellExecutor()
		expectedOutput := "releases across namespaces"
		mock.AddCommandString("helm", []string{"list", "-A"}, expectedOutput, nil)
		ctx := cmd.WithShellExecutor(context.Background(), mock)

		request := createMCPRequest(map[string]interface{}{
			"all_namespaces": "true",
		})
		result, err := handleHelmListReleases(ctx, request)

		assert.NoError(t, err)
		assert.False(t, result.IsError)
	})

	t.Run("list_all_releases", func(t *testing.T) {
		mock := cmd.NewMockShellExecutor()
		expectedOutput := "all releases"
		mock.AddCommandString("helm", []string{"list", "-a"}, expectedOutput, nil)
		ctx := cmd.WithShellExecutor(context.Background(), mock)

		request := createMCPRequest(map[string]interface{}{
			"all": "true",
		})
		result, err := handleHelmListReleases(ctx, request)

		assert.NoError(t, err)
		assert.False(t, result.IsError)
	})

	t.Run("list_with_status_filters", func(t *testing.T) {
		mock := cmd.NewMockShellExecutor()
		expectedOutput := "filtered releases"
		mock.AddCommandString("helm", []string{"list", "--uninstalled", "--uninstalling", "--failed", "--deployed"}, expectedOutput, nil)
		ctx := cmd.WithShellExecutor(context.Background(), mock)

		request := createMCPRequest(map[string]interface{}{
			"deployed":     "true",
			"failed":       "true",
			"uninstalled":  "true",
			"uninstalling": "true",
		})
		result, err := handleHelmListReleases(ctx, request)

		assert.NoError(t, err)
		assert.False(t, result.IsError)
	})
}

// Test Helm Get Release
func TestHandleHelmGetRelease(t *testing.T) {
	t.Run("get release all resources", func(t *testing.T) {
		mock := cmd.NewMockShellExecutor()
		expectedOutput := `REVISION: 1
RELEASED: Mon Jan 01 12:00:00 UTC 2023
CHART: myapp-1.0.0
VALUES:
replicaCount: 3`

		mock.AddCommandString("helm", []string{"get", "all", "myapp", "-n", "default"}, expectedOutput, nil)
		ctx := cmd.WithShellExecutor(context.Background(), mock)

		request := createMCPRequest(map[string]interface{}{
			"name":      "myapp",
			"namespace": "default",
		})

		result, err := handleHelmGetRelease(ctx, request)

		assert.NoError(t, err)
		assert.False(t, result.IsError)
		assert.Contains(t, getResultText(result), "REVISION: 1")

		// Verify the correct command was called
		callLog := mock.GetCallLog()
		require.Len(t, callLog, 1)
		assert.Equal(t, "helm", callLog[0].Command)
		assert.Equal(t, []string{"get", "all", "myapp", "-n", "default"}, callLog[0].Args)
	})

	t.Run("missing required parameters", func(t *testing.T) {
		mock := cmd.NewMockShellExecutor()
		ctx := cmd.WithShellExecutor(context.Background(), mock)

		// Test missing name
		request := createMCPRequest(map[string]interface{}{
			"namespace": "default",
		})

		result, err := handleHelmGetRelease(ctx, request)
		assert.NoError(t, err)
		assert.True(t, result.IsError)
		assert.Contains(t, getResultText(result), "name parameter is required")

		// Verify no commands were executed
		callLog := mock.GetCallLog()
		assert.Len(t, callLog, 0)
	})
}

func TestHandleHelmUpgradeRelease(t *testing.T) {
	// Success with many flags (omit values path validation)
	mock := cmd.NewMockShellExecutor()
	mock.AddCommandString("helm", []string{"upgrade", "myrel", "charts/app", "-n", "default", "--version", "1.2.3", "--set", "a=b", "--install", "--dry-run", "--wait", "--timeout", "30s"}, "upgraded", nil)
	ctx := cmd.WithShellExecutor(context.Background(), mock)

	req := createMCPRequest(map[string]interface{}{
		"name":      "myrel",
		"chart":     "charts/app",
		"namespace": "default",
		"version":   "1.2.3",
		"set":       "a=b",
		"install":   "true",
		"dry_run":   "true",
		"wait":      "true",
	})
	res, err := handleHelmUpgradeRelease(ctx, req)
	require.NoError(t, err)
	assert.False(t, res.IsError)

	// Invalid release name
	res2, err := handleHelmUpgradeRelease(ctx, createMCPRequest(map[string]interface{}{
		"name":  "INVALID_@",
		"chart": "c/app",
	}))
	require.NoError(t, err)
	assert.True(t, res2.IsError)
}

func TestHandleHelmUninstall(t *testing.T) {
	mock := cmd.NewMockShellExecutor()
	mock.AddCommandString("helm", []string{"uninstall", "myrel", "-n", "prod", "--dry-run", "--wait"}, "uninstalled", nil)
	ctx := cmd.WithShellExecutor(context.Background(), mock)

	req := createMCPRequest(map[string]interface{}{
		"name":      "myrel",
		"namespace": "prod",
		"dry_run":   "true",
		"wait":      "true",
	})
	res, err := handleHelmUninstall(ctx, req)
	require.NoError(t, err)
	assert.False(t, res.IsError)

	// Missing args
	res2, err := handleHelmUninstall(ctx, createMCPRequest(map[string]interface{}{"name": "x"}))
	require.NoError(t, err)
	assert.True(t, res2.IsError)
}

func TestHandleHelmRepoAddAndUpdate(t *testing.T) {
	// Repo add
	mock := cmd.NewMockShellExecutor()
	mock.AddCommandString("helm", []string{"repo", "add", "metrics-server", "https://kubernetes-sigs.github.io/metrics-server/"}, "repo added", nil)
	ctx := cmd.WithShellExecutor(context.Background(), mock)

	res, err := handleHelmRepoAdd(ctx, createMCPRequest(map[string]interface{}{
		"name": "metrics-server", "url": "https://kubernetes-sigs.github.io/metrics-server/",
	}))
	require.NoError(t, err)
	assert.False(t, res.IsError)

	// Repo update
	mock2 := cmd.NewMockShellExecutor()
	mock2.AddCommandString("helm", []string{"repo", "update"}, "updated", nil)
	ctx2 := cmd.WithShellExecutor(context.Background(), mock2)
	res2, err := handleHelmRepoUpdate(ctx2, createMCPRequest(map[string]interface{}{}))
	require.NoError(t, err)
	assert.False(t, res2.IsError)
}

// Additional tests for improved coverage
func TestHandleHelmGetReleaseWithResource(t *testing.T) {
	t.Run("get release with custom resource", func(t *testing.T) {
		mock := cmd.NewMockShellExecutor()
		expectedOutput := "values output"
		mock.AddCommandString("helm", []string{"get", "values", "myapp", "-n", "default"}, expectedOutput, nil)
		ctx := cmd.WithShellExecutor(context.Background(), mock)

		request := createMCPRequest(map[string]interface{}{
			"name":      "myapp",
			"namespace": "default",
			"resource":  "values",
		})

		result, err := handleHelmGetRelease(ctx, request)
		assert.NoError(t, err)
		assert.False(t, result.IsError)
	})
}

func TestHandleHelmUpgradeReleaseErrors(t *testing.T) {
	mock := cmd.NewMockShellExecutor()
	ctx := cmd.WithShellExecutor(context.Background(), mock)

	t.Run("missing name", func(t *testing.T) {
		res, err := handleHelmUpgradeRelease(ctx, createMCPRequest(map[string]interface{}{
			"chart": "charts/app",
		}))
		require.NoError(t, err)
		assert.True(t, res.IsError)
		assert.Contains(t, getResultText(res), "name and chart parameters are required")
	})

	t.Run("missing chart", func(t *testing.T) {
		res, err := handleHelmUpgradeRelease(ctx, createMCPRequest(map[string]interface{}{
			"name": "myrel",
		}))
		require.NoError(t, err)
		assert.True(t, res.IsError)
		assert.Contains(t, getResultText(res), "name and chart parameters are required")
	})
}

func TestHandleHelmRepoAddErrors(t *testing.T) {
	mock := cmd.NewMockShellExecutor()
	ctx := cmd.WithShellExecutor(context.Background(), mock)

	t.Run("missing name", func(t *testing.T) {
		res, err := handleHelmRepoAdd(ctx, createMCPRequest(map[string]interface{}{
			"url": "https://example.com",
		}))
		require.NoError(t, err)
		assert.True(t, res.IsError)
		assert.Contains(t, getResultText(res), "name and url parameters are required")
	})

	t.Run("missing url", func(t *testing.T) {
		res, err := handleHelmRepoAdd(ctx, createMCPRequest(map[string]interface{}{
			"name": "myrepo",
		}))
		require.NoError(t, err)
		assert.True(t, res.IsError)
		assert.Contains(t, getResultText(res), "name and url parameters are required")
	})
}

// TestHandleHelmRepoAddCilium tests adding Cilium helm repository
func TestHandleHelmRepoAddCilium(t *testing.T) {
	mock := cmd.NewMockShellExecutor()
	mock.AddCommandString("helm", []string{"repo", "add", "cilium", "https://helm.cilium.io"}, "repo added successfully", nil)
	ctx := cmd.WithShellExecutor(context.Background(), mock)

	res, err := handleHelmRepoAdd(ctx, createMCPRequest(map[string]interface{}{
		"name": "cilium",
		"url":  "https://helm.cilium.io",
	}))
	require.NoError(t, err)
	assert.False(t, res.IsError)
	assert.Contains(t, getResultText(res), "repo added successfully")
}

// Test Helm Template
func TestHandleHelmTemplate(t *testing.T) {
	t.Run("template basic", func(t *testing.T) {
		mock := cmd.NewMockShellExecutor()
		expectedOutput := `---
# Source: myapp/templates/deployment.yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: myapp
  namespace: default
spec:
  replicas: 3`

		mock.AddCommandString("helm", []string{"template", "myapp", "charts/myapp"}, expectedOutput, nil)
		ctx := cmd.WithShellExecutor(context.Background(), mock)

		request := createMCPRequest(map[string]interface{}{
			"name":  "myapp",
			"chart": "charts/myapp",
		})

		result, err := handleHelmTemplate(ctx, request)

		assert.NoError(t, err)
		assert.False(t, result.IsError)
		content := getResultText(result)
		assert.Contains(t, content, "apiVersion: apps/v1")
		assert.Contains(t, content, "kind: Deployment")

		// Verify the correct command was called
		callLog := mock.GetCallLog()
		require.Len(t, callLog, 1)
		assert.Equal(t, "helm", callLog[0].Command)
		assert.Equal(t, []string{"template", "myapp", "charts/myapp"}, callLog[0].Args)
	})

	t.Run("template with namespace", func(t *testing.T) {
		mock := cmd.NewMockShellExecutor()
		expectedOutput := "apiVersion: v1"
		mock.AddCommandString("helm", []string{"template", "myapp", "charts/myapp", "-n", "prod"}, expectedOutput, nil)
		ctx := cmd.WithShellExecutor(context.Background(), mock)

		request := createMCPRequest(map[string]interface{}{
			"name":      "myapp",
			"chart":     "charts/myapp",
			"namespace": "prod",
		})

		result, err := handleHelmTemplate(ctx, request)

		assert.NoError(t, err)
		assert.False(t, result.IsError)
	})

	t.Run("template with version", func(t *testing.T) {
		mock := cmd.NewMockShellExecutor()
		expectedOutput := "apiVersion: v1"
		mock.AddCommandString("helm", []string{"template", "myapp", "charts/myapp", "--version", "1.2.3"}, expectedOutput, nil)
		ctx := cmd.WithShellExecutor(context.Background(), mock)

		request := createMCPRequest(map[string]interface{}{
			"name":    "myapp",
			"chart":   "charts/myapp",
			"version": "1.2.3",
		})

		result, err := handleHelmTemplate(ctx, request)

		assert.NoError(t, err)
		assert.False(t, result.IsError)
	})

	t.Run("template with set values", func(t *testing.T) {
		mock := cmd.NewMockShellExecutor()
		expectedOutput := "apiVersion: v1"
		mock.AddCommandString("helm", []string{"template", "myapp", "charts/myapp", "--set", "replicas=5", "--set", "image=myimage:latest"}, expectedOutput, nil)
		ctx := cmd.WithShellExecutor(context.Background(), mock)

		request := createMCPRequest(map[string]interface{}{
			"name":  "myapp",
			"chart": "charts/myapp",
			"set":   "replicas=5,image=myimage:latest",
		})

		result, err := handleHelmTemplate(ctx, request)

		assert.NoError(t, err)
		assert.False(t, result.IsError)
	})

	t.Run("template with all options", func(t *testing.T) {
		mock := cmd.NewMockShellExecutor()
		expectedOutput := "apiVersion: v1"
		mock.AddCommandString("helm", []string{"template", "myapp", "charts/myapp", "-n", "staging", "--version", "2.0.0", "-f", "/path/to/values.yaml", "--set", "key=val"}, expectedOutput, nil)
		ctx := cmd.WithShellExecutor(context.Background(), mock)

		request := createMCPRequest(map[string]interface{}{
			"name":      "myapp",
			"chart":     "charts/myapp",
			"namespace": "staging",
			"version":   "2.0.0",
			"values":    "/path/to/values.yaml",
			"set":       "key=val",
		})

		result, err := handleHelmTemplate(ctx, request)

		assert.NoError(t, err)
		assert.False(t, result.IsError)
	})

	t.Run("template missing required parameters", func(t *testing.T) {
		mock := cmd.NewMockShellExecutor()
		ctx := cmd.WithShellExecutor(context.Background(), mock)

		// Missing name
		request := createMCPRequest(map[string]interface{}{
			"chart": "charts/myapp",
		})

		result, err := handleHelmTemplate(ctx, request)
		assert.NoError(t, err)
		assert.True(t, result.IsError)
		assert.Contains(t, getResultText(result), "name and chart parameters are required")

		// Verify no commands were executed
		callLog := mock.GetCallLog()
		assert.Len(t, callLog, 0)
	})

	t.Run("template invalid release name", func(t *testing.T) {
		mock := cmd.NewMockShellExecutor()
		ctx := cmd.WithShellExecutor(context.Background(), mock)

		request := createMCPRequest(map[string]interface{}{
			"name":  "INVALID_@#$",
			"chart": "charts/myapp",
		})

		result, err := handleHelmTemplate(ctx, request)
		assert.NoError(t, err)
		assert.True(t, result.IsError)
		assert.Contains(t, getResultText(result), "Invalid release name")
	})

	t.Run("template invalid namespace", func(t *testing.T) {
		mock := cmd.NewMockShellExecutor()
		ctx := cmd.WithShellExecutor(context.Background(), mock)

		request := createMCPRequest(map[string]interface{}{
			"name":      "myapp",
			"chart":     "charts/myapp",
			"namespace": "INVALID_@",
		})

		result, err := handleHelmTemplate(ctx, request)
		assert.NoError(t, err)
		assert.True(t, result.IsError)
		assert.Contains(t, getResultText(result), "Invalid namespace")
	})

	t.Run("template command failure", func(t *testing.T) {
		mock := cmd.NewMockShellExecutor()
		mock.AddCommandString("helm", []string{"template", "myapp", "charts/myapp"}, "", assert.AnError)
		ctx := cmd.WithShellExecutor(context.Background(), mock)

		request := createMCPRequest(map[string]interface{}{
			"name":  "myapp",
			"chart": "charts/myapp",
		})

		result, err := handleHelmTemplate(ctx, request)
		assert.NoError(t, err)
		assert.True(t, result.IsError)
		assert.Contains(t, getResultText(result), "Helm template command failed")
	})
}
