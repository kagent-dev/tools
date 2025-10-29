package k8s

import (
	"context"
	"strings"
	"testing"

	"github.com/kagent-dev/tools/internal/cmd"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Helper function to create a test K8sTool
func newTestK8sTool() *K8sTool {
	return NewK8sTool(nil)
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

	t.Run("success with default output", func(t *testing.T) {
		mock := cmd.NewMockShellExecutor()
		expectedOutput := `NAMESPACE   LAST SEEN   TYPE      REASON    OBJECT              MESSAGE
default     5m          Normal    Created   pod/test-pod        Created container test`
		mock.AddCommandString("kubectl", []string{"get", "events", "--all-namespaces", "-o", "wide"}, expectedOutput, nil)
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
		assert.Contains(t, resultText, "test-pod")
	})

	t.Run("with namespace", func(t *testing.T) {
		mock := cmd.NewMockShellExecutor()
		expectedOutput := `LAST SEEN   TYPE     REASON    OBJECT         MESSAGE
5m          Normal   Started   pod/my-pod     Started container`
		mock.AddCommandString("kubectl", []string{"get", "events", "-n", "custom-namespace", "-o", "wide"}, expectedOutput, nil)
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

	t.Run("with json output format", func(t *testing.T) {
		mock := cmd.NewMockShellExecutor()
		expectedOutput := `{"items": [{"metadata": {"name": "test-event"}, "message": "Test event message"}]}`
		mock.AddCommandString("kubectl", []string{"get", "events", "--all-namespaces", "-o", "json"}, expectedOutput, nil)
		ctx := cmd.WithShellExecutor(ctx, mock)

		k8sTool := newTestK8sTool()

		req := &mcp.CallToolRequest{
			Params: &mcp.CallToolParamsRaw{
				Arguments: []byte(`{"output": "json"}`),
			},
		}

		result, err := k8sTool.handleGetEvents(ctx, req)
		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.False(t, result.IsError)

		resultText := getResultText(result)
		assert.Contains(t, resultText, "test-event")
	})

	t.Run("with yaml output format and namespace", func(t *testing.T) {
		mock := cmd.NewMockShellExecutor()
		expectedOutput := `apiVersion: v1
items:
- kind: Event
  metadata:
    name: test-event
    namespace: kube-system`
		mock.AddCommandString("kubectl", []string{"get", "events", "-n", "kube-system", "-o", "yaml"}, expectedOutput, nil)
		ctx := cmd.WithShellExecutor(ctx, mock)

		k8sTool := newTestK8sTool()

		req := &mcp.CallToolRequest{
			Params: &mcp.CallToolParamsRaw{
				Arguments: []byte(`{"namespace": "kube-system", "output": "yaml"}`),
			},
		}

		result, err := k8sTool.handleGetEvents(ctx, req)
		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.False(t, result.IsError)

		resultText := getResultText(result)
		assert.Contains(t, resultText, "test-event")
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
				Arguments: []byte("{}"),
			},
		}
		result, err := k8sTool.handleKubectlDescribeTool(ctx, req)
		assert.NoError(t, err)
		assert.True(t, result.IsError)
	})

	t.Run("valid parameters", func(t *testing.T) {
		mock := cmd.NewMockShellExecutor()
		expectedOutput := `Name:         test-pod
Namespace:    default
Status:       Running`
		mock.AddCommandString("kubectl", []string{"describe", "pod", "test-pod", "-n", "default"}, expectedOutput, nil)
		ctx := cmd.WithShellExecutor(ctx, mock)

		k8sTool := newTestK8sTool()
		req := &mcp.CallToolRequest{
			Params: &mcp.CallToolParamsRaw{
				Arguments: []byte(`{"resource_type": "pod", "resource_name": "test-pod", "namespace": "default"}`),
			},
		}
		result, err := k8sTool.handleKubectlDescribeTool(ctx, req)
		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.False(t, result.IsError)
		assert.Contains(t, getResultText(result), "test-pod")
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

// Test helper functions for better coverage
func TestParseString(t *testing.T) {
	t.Run("parse valid string parameter", func(t *testing.T) {
		req := &mcp.CallToolRequest{
			Params: &mcp.CallToolParamsRaw{
				Arguments: []byte(`{"key": "value"}`),
			},
		}
		result := parseString(req, "key", "default")
		assert.Equal(t, "value", result)
	})

	t.Run("parse with default value when key missing", func(t *testing.T) {
		req := &mcp.CallToolRequest{
			Params: &mcp.CallToolParamsRaw{
				Arguments: []byte(`{"other_key": "value"}`),
			},
		}
		result := parseString(req, "key", "default")
		assert.Equal(t, "default", result)
	})

	t.Run("parse with null arguments", func(t *testing.T) {
		req := &mcp.CallToolRequest{
			Params: &mcp.CallToolParamsRaw{
				Arguments: nil,
			},
		}
		result := parseString(req, "key", "default")
		assert.Equal(t, "default", result)
	})

	t.Run("parse with invalid JSON arguments", func(t *testing.T) {
		req := &mcp.CallToolRequest{
			Params: &mcp.CallToolParamsRaw{
				Arguments: []byte(`{invalid json`),
			},
		}
		result := parseString(req, "key", "default")
		assert.Equal(t, "default", result)
	})

	t.Run("parse non-string value", func(t *testing.T) {
		req := &mcp.CallToolRequest{
			Params: &mcp.CallToolParamsRaw{
				Arguments: []byte(`{"key": 123}`),
			},
		}
		result := parseString(req, "key", "default")
		assert.Equal(t, "default", result)
	})
}

func TestParseInt(t *testing.T) {
	t.Run("parse valid integer parameter", func(t *testing.T) {
		req := &mcp.CallToolRequest{
			Params: &mcp.CallToolParamsRaw{
				Arguments: []byte(`{"count": 42}`),
			},
		}
		result := parseInt(req, "count", 0)
		assert.Equal(t, 42, result)
	})

	t.Run("parse float as integer", func(t *testing.T) {
		req := &mcp.CallToolRequest{
			Params: &mcp.CallToolParamsRaw{
				Arguments: []byte(`{"count": 42.5}`),
			},
		}
		result := parseInt(req, "count", 0)
		assert.Equal(t, 42, result)
	})

	t.Run("parse string as integer", func(t *testing.T) {
		req := &mcp.CallToolRequest{
			Params: &mcp.CallToolParamsRaw{
				Arguments: []byte(`{"count": "42"}`),
			},
		}
		result := parseInt(req, "count", 0)
		assert.Equal(t, 42, result)
	})

	t.Run("parse with default when key missing", func(t *testing.T) {
		req := &mcp.CallToolRequest{
			Params: &mcp.CallToolParamsRaw{
				Arguments: []byte(`{"other": 10}`),
			},
		}
		result := parseInt(req, "count", 99)
		assert.Equal(t, 99, result)
	})

	t.Run("parse with null arguments", func(t *testing.T) {
		req := &mcp.CallToolRequest{
			Params: &mcp.CallToolParamsRaw{
				Arguments: nil,
			},
		}
		result := parseInt(req, "count", 99)
		assert.Equal(t, 99, result)
	})

	t.Run("parse invalid string as integer", func(t *testing.T) {
		req := &mcp.CallToolRequest{
			Params: &mcp.CallToolParamsRaw{
				Arguments: []byte(`{"count": "not-a-number"}`),
			},
		}
		result := parseInt(req, "count", 99)
		assert.Equal(t, 99, result)
	})

	t.Run("parse invalid JSON arguments", func(t *testing.T) {
		req := &mcp.CallToolRequest{
			Params: &mcp.CallToolParamsRaw{
				Arguments: []byte(`{invalid json`),
			},
		}
		result := parseInt(req, "count", 99)
		assert.Equal(t, 99, result)
	})
}

func TestToolResultHelpers(t *testing.T) {
	t.Run("newToolResultError creates error result", func(t *testing.T) {
		result := newToolResultError("test error message")
		assert.NotNil(t, result)
		assert.True(t, result.IsError)
		assert.NotEmpty(t, result.Content)
		assert.Contains(t, getResultText(result), "test error message")
	})

	t.Run("newToolResultText creates success result", func(t *testing.T) {
		result := newToolResultText("test output")
		assert.NotNil(t, result)
		assert.False(t, result.IsError)
		assert.NotEmpty(t, result.Content)
		assert.Contains(t, getResultText(result), "test output")
	})
}

func TestKubectlGetEnhancedEdgeCases(t *testing.T) {
	ctx := context.Background()

	t.Run("with namespace specified", func(t *testing.T) {
		mock := cmd.NewMockShellExecutor()
		expectedOutput := `NAME        READY   STATUS    RESTARTS   AGE
test-pod    1/1     Running   0          1d`
		mock.AddCommandString("kubectl", []string{"get", "pods", "-n", "test-ns", "-o", "wide"}, expectedOutput, nil)
		ctx := cmd.WithShellExecutor(ctx, mock)

		k8sTool := newTestK8sTool()
		req := &mcp.CallToolRequest{
			Params: &mcp.CallToolParamsRaw{
				Arguments: []byte(`{"resource_type": "pods", "namespace": "test-ns"}`),
			},
		}
		result, err := k8sTool.handleKubectlGetEnhanced(ctx, req)
		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.False(t, result.IsError)
	})

	t.Run("with all namespaces flag", func(t *testing.T) {
		mock := cmd.NewMockShellExecutor()
		expectedOutput := `NAMESPACE   NAME        READY   STATUS
default     test-pod    1/1     Running`
		mock.AddCommandString("kubectl", []string{"get", "pods", "--all-namespaces", "-o", "wide"}, expectedOutput, nil)
		ctx := cmd.WithShellExecutor(ctx, mock)

		k8sTool := newTestK8sTool()
		req := &mcp.CallToolRequest{
			Params: &mcp.CallToolParamsRaw{
				Arguments: []byte(`{"resource_type": "pods", "all_namespaces": "true"}`),
			},
		}
		result, err := k8sTool.handleKubectlGetEnhanced(ctx, req)
		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.False(t, result.IsError)
	})

	t.Run("with resource name specified", func(t *testing.T) {
		mock := cmd.NewMockShellExecutor()
		expectedOutput := `NAME        READY   STATUS    RESTARTS   AGE
specific    1/1     Running   0          1d`
		mock.AddCommandString("kubectl", []string{"get", "pods", "specific", "-o", "wide"}, expectedOutput, nil)
		ctx := cmd.WithShellExecutor(ctx, mock)

		k8sTool := newTestK8sTool()
		req := &mcp.CallToolRequest{
			Params: &mcp.CallToolParamsRaw{
				Arguments: []byte(`{"resource_type": "pods", "resource_name": "specific"}`),
			},
		}
		result, err := k8sTool.handleKubectlGetEnhanced(ctx, req)
		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.False(t, result.IsError)
	})

	t.Run("with custom output format", func(t *testing.T) {
		mock := cmd.NewMockShellExecutor()
		expectedOutput := `apiVersion: v1
kind: Pod
metadata:
  name: test-pod`
		mock.AddCommandString("kubectl", []string{"get", "pods", "-o", "yaml"}, expectedOutput, nil)
		ctx := cmd.WithShellExecutor(ctx, mock)

		k8sTool := newTestK8sTool()
		req := &mcp.CallToolRequest{
			Params: &mcp.CallToolParamsRaw{
				Arguments: []byte(`{"resource_type": "pods", "output": "yaml"}`),
			},
		}
		result, err := k8sTool.handleKubectlGetEnhanced(ctx, req)
		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.False(t, result.IsError)
	})
}
