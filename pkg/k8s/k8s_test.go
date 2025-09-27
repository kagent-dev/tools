package k8s

import (
	"context"
	"strings"
	"testing"

	"github.com/kagent-dev/tools/internal/cmd"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tmc/langchaingo/llms"
)

// Helper function to create a test K8sTool
func newTestK8sTool() *K8sTool {
	return NewK8sTool(nil)
}

// Helper function to create a test K8sTool with mock LLM
func newTestK8sToolWithLLM(llm llms.Model) *K8sTool {
	return NewK8sTool(llm)
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

func TestHandleGetAvailableAPIResources(t *testing.T) {
	ctx := context.Background()

	t.Run("success", func(t *testing.T) {
		mock := cmd.NewMockShellExecutor()
		expectedOutput := `NAME                              SHORTNAMES   APIVERSION                             NAMESPACED   KIND
pods                              po           v1                                     true         Pod
services                          svc          v1                                     true         Service`
		mock.AddCommandString("kubectl", []string{"api-resources"}, expectedOutput, nil)
		ctx := cmd.WithShellExecutor(ctx, mock)

		k8sTool := newTestK8sTool()

		req := &mcp.CallToolRequest{
			Params: &mcp.CallToolParamsRaw{
				Arguments: []byte("{}"),
			},
		}
		result, err := k8sTool.handleGetAvailableAPIResources(ctx, req)
		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.False(t, result.IsError)

		// Check that we got some content
		assert.NotEmpty(t, result.Content)
		assert.Contains(t, getResultText(result), "pods")
	})

	t.Run("kubectl command failure", func(t *testing.T) {
		mock := cmd.NewMockShellExecutor()
		mock.AddCommandString("kubectl", []string{"api-resources"}, "", assert.AnError)
		ctx := cmd.WithShellExecutor(ctx, mock)

		k8sTool := newTestK8sTool()

		req := &mcp.CallToolRequest{
			Params: &mcp.CallToolParamsRaw{
				Arguments: []byte("{}"),
			},
		}
		result, err := k8sTool.handleGetAvailableAPIResources(ctx, req)
		assert.NoError(t, err) // MCP handlers should not return Go errors
		assert.NotNil(t, result)
		assert.True(t, result.IsError)
	})
}

func TestHandleScaleDeployment(t *testing.T) {
	ctx := context.Background()

	t.Run("success", func(t *testing.T) {
		mock := cmd.NewMockShellExecutor()
		expectedOutput := `deployment.apps/test-deployment scaled`
		mock.AddCommandString("kubectl", []string{"scale", "deployment", "test-deployment", "--replicas", "5", "-n", "default"}, expectedOutput, nil)
		ctx := cmd.WithShellExecutor(ctx, mock)

		k8sTool := newTestK8sTool()

		req := &mcp.CallToolRequest{
			Params: &mcp.CallToolParamsRaw{
				Arguments: []byte(`{"name": "test-deployment", "replicas": 5}`),
			},
		}

		result, err := k8sTool.handleScaleDeployment(ctx, req)
		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.False(t, result.IsError)

		resultText := getResultText(result)
		assert.Contains(t, resultText, "test-deployment")
		assert.Contains(t, resultText, "scaled")
	})

	t.Run("missing name parameter", func(t *testing.T) {
		mock := cmd.NewMockShellExecutor()
		ctx := cmd.WithShellExecutor(context.Background(), mock)

		k8sTool := newTestK8sTool()

		req := &mcp.CallToolRequest{
			Params: &mcp.CallToolParamsRaw{
				Arguments: []byte(`{"replicas": 3}`),
			},
		}

		result, err := k8sTool.handleScaleDeployment(ctx, req)
		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.True(t, result.IsError)
		assert.Contains(t, getResultText(result), "name parameter is required")

		// Verify no commands were executed since parameters are missing
		callLog := mock.GetCallLog()
		assert.Len(t, callLog, 0)
	})

	t.Run("missing replicas parameter uses default", func(t *testing.T) {
		mock := cmd.NewMockShellExecutor()
		expectedOutput := `deployment.apps/test-deployment scaled`
		mock.AddCommandString("kubectl", []string{"scale", "deployment", "test-deployment", "--replicas", "1", "-n", "default"}, expectedOutput, nil)
		ctx := cmd.WithShellExecutor(ctx, mock)

		k8sTool := newTestK8sTool()

		req := &mcp.CallToolRequest{
			Params: &mcp.CallToolParamsRaw{
				Arguments: []byte(`{"name": "test-deployment"}`),
			},
		}

		result, err := k8sTool.handleScaleDeployment(ctx, req)
		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.False(t, result.IsError)

		resultText := getResultText(result)
		assert.Contains(t, resultText, "scaled")

		// Verify the command was executed with default replicas=1
		callLog := mock.GetCallLog()
		assert.Len(t, callLog, 1)
		assert.Equal(t, "kubectl", callLog[0].Command)
		assert.Equal(t, []string{"scale", "deployment", "test-deployment", "--replicas", "1", "-n", "default"}, callLog[0].Args)
	})
}

func TestHandleGetEvents(t *testing.T) {
	ctx := context.Background()

	t.Run("success", func(t *testing.T) {
		mock := cmd.NewMockShellExecutor()
		expectedOutput := `{"items": [{"metadata": {"name": "test-event"}, "message": "Test event message"}]}`
		mock.AddCommandString("kubectl", []string{"get", "events", "-o", "json", "--all-namespaces"}, expectedOutput, nil)
		ctx := cmd.WithShellExecutor(ctx, mock)

		k8sTool := newTestK8sTool()

		req := &mcp.CallToolRequest{
			Params: &mcp.CallToolParamsRaw{
				Arguments: []byte("{}"),
			},
		}
		result, err := k8sTool.handleGetEvents(ctx, req)
		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.False(t, result.IsError)

		resultText := getResultText(result)
		assert.Contains(t, resultText, "test-event")
	})

	t.Run("with namespace", func(t *testing.T) {
		mock := cmd.NewMockShellExecutor()
		expectedOutput := `{"items": []}`
		mock.AddCommandString("kubectl", []string{"get", "events", "-o", "json", "-n", "custom-namespace"}, expectedOutput, nil)
		ctx := cmd.WithShellExecutor(ctx, mock)

		k8sTool := newTestK8sTool()

		req := &mcp.CallToolRequest{
			Params: &mcp.CallToolParamsRaw{
				Arguments: []byte(`{"namespace": "custom-namespace"}`),
			},
		}

		result, err := k8sTool.handleGetEvents(ctx, req)
		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.False(t, result.IsError)
	})
}

func TestHandleDeleteResource(t *testing.T) {
	ctx := context.Background()

	t.Run("missing parameters", func(t *testing.T) {
		mock := cmd.NewMockShellExecutor()
		ctx := cmd.WithShellExecutor(context.Background(), mock)

		k8sTool := newTestK8sTool()

		req := &mcp.CallToolRequest{
			Params: &mcp.CallToolParamsRaw{
				Arguments: []byte(`{"resource_type": "pod"}`),
			},
		}

		result, err := k8sTool.handleDeleteResource(ctx, req)
		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.True(t, result.IsError)

		// Verify no commands were executed since parameters are missing
		callLog := mock.GetCallLog()
		assert.Len(t, callLog, 0)
	})

	t.Run("valid parameters", func(t *testing.T) {
		mock := cmd.NewMockShellExecutor()
		expectedOutput := `deployment.apps/test-deployment deleted`
		mock.AddCommandString("kubectl", []string{"delete", "deployment", "test-deployment", "-n", "default"}, expectedOutput, nil)
		ctx := cmd.WithShellExecutor(ctx, mock)

		k8sTool := newTestK8sTool()

		req := &mcp.CallToolRequest{
			Params: &mcp.CallToolParamsRaw{
				Arguments: []byte(`{"resource_type": "deployment", "resource_name": "test-deployment"}`),
			},
		}

		result, err := k8sTool.handleDeleteResource(ctx, req)
		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.False(t, result.IsError)

		resultText := getResultText(result)
		assert.Contains(t, resultText, "deleted")
	})
}

func TestHandleKubectlDescribeTool(t *testing.T) {
	ctx := context.Background()

	t.Run("missing parameters", func(t *testing.T) {
		mock := cmd.NewMockShellExecutor()
		ctx := cmd.WithShellExecutor(context.Background(), mock)

		k8sTool := newTestK8sTool()

		req := &mcp.CallToolRequest{
			Params: &mcp.CallToolParamsRaw{
				Arguments: []byte(`{"resource_type": "deployment"}`),
			},
		}

		result, err := k8sTool.handleKubectlDescribeTool(ctx, req)
		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.True(t, result.IsError)

		// Verify no commands were executed since parameters are missing
		callLog := mock.GetCallLog()
		assert.Len(t, callLog, 0)
	})

	t.Run("valid parameters", func(t *testing.T) {
		mock := cmd.NewMockShellExecutor()
		expectedOutput := `Name:               test-deployment
Namespace:          default
Labels:             app=test`
		mock.AddCommandString("kubectl", []string{"describe", "deployment", "test-deployment", "-n", "default"}, expectedOutput, nil)
		ctx := cmd.WithShellExecutor(ctx, mock)

		k8sTool := newTestK8sTool()

		req := &mcp.CallToolRequest{
			Params: &mcp.CallToolParamsRaw{
				Arguments: []byte(`{"resource_type": "deployment", "resource_name": "test-deployment", "namespace": "default"}`),
			},
		}

		result, err := k8sTool.handleKubectlDescribeTool(ctx, req)
		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.False(t, result.IsError)

		resultText := getResultText(result)
		assert.Contains(t, resultText, "test-deployment")
	})
}

func TestHandleKubectlGetEnhanced(t *testing.T) {
	ctx := context.Background()

	t.Run("missing resource_type", func(t *testing.T) {
		mock := cmd.NewMockShellExecutor()
		ctx := cmd.WithShellExecutor(context.Background(), mock)

		k8sTool := newTestK8sTool()
		req := &mcp.CallToolRequest{
			Params: &mcp.CallToolParamsRaw{
				Arguments: []byte("{}"),
			},
		}
		result, err := k8sTool.handleKubectlGetEnhanced(ctx, req)
		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.True(t, result.IsError)

		// Verify no commands were executed since parameters are missing
		callLog := mock.GetCallLog()
		assert.Len(t, callLog, 0)
	})

	t.Run("valid resource_type", func(t *testing.T) {
		mock := cmd.NewMockShellExecutor()
		expectedOutput := `NAME   READY   STATUS    RESTARTS   AGE`
		mock.AddCommandString("kubectl", []string{"get", "pods", "-o", "wide"}, expectedOutput, nil)
		ctx := cmd.WithShellExecutor(ctx, mock)

		k8sTool := newTestK8sTool()
		req := &mcp.CallToolRequest{
			Params: &mcp.CallToolParamsRaw{
				Arguments: []byte(`{"resource_type": "pods"}`),
			},
		}
		result, err := k8sTool.handleKubectlGetEnhanced(ctx, req)
		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.False(t, result.IsError)
	})
}

func TestHandleKubectlLogsEnhanced(t *testing.T) {
	ctx := context.Background()

	t.Run("missing pod_name", func(t *testing.T) {
		mock := cmd.NewMockShellExecutor()
		ctx := cmd.WithShellExecutor(context.Background(), mock)

		k8sTool := newTestK8sTool()
		req := &mcp.CallToolRequest{
			Params: &mcp.CallToolParamsRaw{
				Arguments: []byte("{}"),
			},
		}
		result, err := k8sTool.handleKubectlLogsEnhanced(ctx, req)
		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.True(t, result.IsError)

		// Verify no commands were executed since parameters are missing
		callLog := mock.GetCallLog()
		assert.Len(t, callLog, 0)
	})

	t.Run("valid pod_name", func(t *testing.T) {
		mock := cmd.NewMockShellExecutor()
		expectedOutput := `log line 1
log line 2`
		mock.AddCommandString("kubectl", []string{"logs", "test-pod", "-n", "default", "--tail", "50"}, expectedOutput, nil)
		ctx := cmd.WithShellExecutor(ctx, mock)

		k8sTool := newTestK8sTool()
		req := &mcp.CallToolRequest{
			Params: &mcp.CallToolParamsRaw{
				Arguments: []byte(`{"pod_name": "test-pod"}`),
			},
		}
		result, err := k8sTool.handleKubectlLogsEnhanced(ctx, req)
		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.False(t, result.IsError)
	})
}

func TestHandleApplyManifest(t *testing.T) {
	ctx := context.Background()
	t.Run("apply manifest from string", func(t *testing.T) {
		mock := cmd.NewMockShellExecutor()
		manifest := `apiVersion: v1
kind: Pod
metadata:
  name: test-pod
spec:
  containers:
  - name: test
    image: nginx`

		expectedOutput := `pod/test-pod created`
		// Use partial matcher to handle dynamic temp file names
		mock.AddPartialMatcherString("kubectl", []string{"apply", "-f"}, expectedOutput, nil)
		ctx := cmd.WithShellExecutor(ctx, mock)

		k8sTool := newTestK8sTool()

		req := &mcp.CallToolRequest{
			Params: &mcp.CallToolParamsRaw{
				Arguments: []byte(`{"manifest": "` + strings.ReplaceAll(manifest, "\n", "\\n") + `"}`),
			},
		}

		result, err := k8sTool.handleApplyManifest(ctx, req)
		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.False(t, result.IsError)

		// Verify the expected output
		content := getResultText(result)
		assert.Contains(t, content, "created")

		// Verify kubectl apply was called (we can't predict the exact temp file name)
		callLog := mock.GetCallLog()
		require.Len(t, callLog, 1)
		assert.Equal(t, "kubectl", callLog[0].Command)
		assert.Len(t, callLog[0].Args, 3) // apply, -f, <temp-file>
		assert.Equal(t, "apply", callLog[0].Args[0])
		assert.Equal(t, "-f", callLog[0].Args[1])
		// Third argument should be the temporary file path
		assert.Contains(t, callLog[0].Args[2], "manifest-")
	})

	t.Run("missing manifest parameter", func(t *testing.T) {
		mock := cmd.NewMockShellExecutor()
		ctx := cmd.WithShellExecutor(ctx, mock)

		k8sTool := newTestK8sTool()

		req := &mcp.CallToolRequest{
			Params: &mcp.CallToolParamsRaw{
				Arguments: []byte("{}"),
			},
		}

		result, err := k8sTool.handleApplyManifest(ctx, req)
		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.True(t, result.IsError)
		assert.Contains(t, getResultText(result), "manifest parameter is required")

		// Verify no commands were executed
		callLog := mock.GetCallLog()
		assert.Len(t, callLog, 0)
	})
}

func TestHandleExecCommand(t *testing.T) {
	ctx := context.Background()
	t.Run("exec command in pod", func(t *testing.T) {
		mock := cmd.NewMockShellExecutor()
		expectedOutput := `total 8
drwxr-xr-x 1 root root 4096 Jan  1 12:00 .
drwxr-xr-x 1 root root 4096 Jan  1 12:00 ..`

		// The implementation passes the command as a single string after --
		mock.AddCommandString("kubectl", []string{"exec", "mypod", "-n", "default", "--", "ls -la"}, expectedOutput, nil)
		ctx := cmd.WithShellExecutor(ctx, mock)

		k8sTool := newTestK8sTool()

		req := &mcp.CallToolRequest{
			Params: &mcp.CallToolParamsRaw{
				Arguments: []byte(`{"pod_name": "mypod", "namespace": "default", "command": "ls -la"}`),
			},
		}

		result, err := k8sTool.handleExecCommand(ctx, req)
		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.False(t, result.IsError)

		// Verify the expected output
		content := getResultText(result)
		assert.Contains(t, content, "total 8")

		// Verify the correct kubectl command was called
		callLog := mock.GetCallLog()
		require.Len(t, callLog, 1)
		assert.Equal(t, "kubectl", callLog[0].Command)
		assert.Equal(t, []string{"exec", "mypod", "-n", "default", "--", "ls -la"}, callLog[0].Args)
	})

	t.Run("missing required parameters", func(t *testing.T) {
		mock := cmd.NewMockShellExecutor()
		ctx := cmd.WithShellExecutor(context.Background(), mock)

		k8sTool := newTestK8sTool()

		req := &mcp.CallToolRequest{
			Params: &mcp.CallToolParamsRaw{
				Arguments: []byte(`{"pod_name": "mypod"}`),
			},
		}

		result, err := k8sTool.handleExecCommand(ctx, req)
		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.True(t, result.IsError)
		assert.Contains(t, getResultText(result), "pod_name and command parameters are required")

		// Verify no commands were executed since parameters are missing
		callLog := mock.GetCallLog()
		assert.Len(t, callLog, 0)
	})
}
