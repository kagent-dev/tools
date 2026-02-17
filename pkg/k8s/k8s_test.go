package k8s

import (
	"context"
	"net/http"
	"testing"

	"github.com/kagent-dev/tools/internal/cmd"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tmc/langchaingo/llms"
)

// Helper function to create a test K8sTool
func newTestK8sTool() *K8sTool {
	return NewK8sTool(nil)
}

// newTestK8sToolWithPassthrough creates a K8sTool with token passthrough set for testing.
func newTestK8sToolWithPassthrough(passthrough bool) *K8sTool {
	t := NewK8sTool(nil)
	t.tokenPassthrough = passthrough
	return t
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
	if textContent, ok := result.Content[0].(mcp.TextContent); ok {
		return textContent.Text
	}
	return ""
}

// Helper function to create an http.Header with Bearer token authorization
func headerWithBearerToken(token string) http.Header {
	h := http.Header{}
	h.Set("Authorization", "Bearer "+token)
	return h
}

// Helper function to create a CallToolRequest with Bearer token
func requestWithBearerToken(token string, args map[string]interface{}) mcp.CallToolRequest {
	req := mcp.CallToolRequest{}
	req.Header = headerWithBearerToken(token)
	req.Params.Arguments = args
	return req
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

		req := mcp.CallToolRequest{}
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

		req := mcp.CallToolRequest{}
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

		req := mcp.CallToolRequest{}
		req.Params.Arguments = map[string]interface{}{
			"name":     "test-deployment",
			"replicas": float64(5), // JSON numbers come as float64
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

		req := mcp.CallToolRequest{}
		req.Params.Arguments = map[string]interface{}{
			// Missing name parameter (this is the required one)
			"replicas": float64(3),
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

		req := mcp.CallToolRequest{}
		req.Params.Arguments = map[string]interface{}{
			"name": "test-deployment",
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

		req := mcp.CallToolRequest{}
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

		req := mcp.CallToolRequest{}
		req.Params.Arguments = map[string]interface{}{
			"namespace": "custom-namespace",
		}

		result, err := k8sTool.handleGetEvents(ctx, req)
		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.False(t, result.IsError)
	})
}

func TestHandlePatchResource(t *testing.T) {
	ctx := context.Background()

	t.Run("missing parameters", func(t *testing.T) {
		mock := cmd.NewMockShellExecutor()
		ctx := cmd.WithShellExecutor(context.Background(), mock)

		k8sTool := newTestK8sTool()

		req := mcp.CallToolRequest{}
		req.Params.Arguments = map[string]interface{}{
			"resource_type": "deployment",
			// Missing resource_name and patch
		}

		result, err := k8sTool.handlePatchResource(ctx, req)
		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.True(t, result.IsError)

		// Verify no commands were executed since parameters are missing
		callLog := mock.GetCallLog()
		assert.Len(t, callLog, 0)
	})

	t.Run("valid parameters", func(t *testing.T) {
		mock := cmd.NewMockShellExecutor()
		expectedOutput := `deployment.apps/test-deployment patched`
		mock.AddCommandString("kubectl", []string{"patch", "deployment", "test-deployment", "-p", `{"spec":{"replicas":5}}`, "-n", "default"}, expectedOutput, nil)
		ctx := cmd.WithShellExecutor(ctx, mock)

		k8sTool := newTestK8sTool()

		req := mcp.CallToolRequest{}
		req.Params.Arguments = map[string]interface{}{
			"resource_type": "deployment",
			"resource_name": "test-deployment",
			"patch":         `{"spec":{"replicas":5}}`,
		}

		result, err := k8sTool.handlePatchResource(ctx, req)
		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.False(t, result.IsError)

		resultText := getResultText(result)
		assert.Contains(t, resultText, "patched")
	})
}

func TestHandleDeleteResource(t *testing.T) {
	ctx := context.Background()

	t.Run("missing parameters", func(t *testing.T) {
		mock := cmd.NewMockShellExecutor()
		ctx := cmd.WithShellExecutor(context.Background(), mock)

		k8sTool := newTestK8sTool()

		req := mcp.CallToolRequest{}
		req.Params.Arguments = map[string]interface{}{
			"resource_type": "pod",
			// Missing resource_name
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

		req := mcp.CallToolRequest{}
		req.Params.Arguments = map[string]interface{}{
			"resource_type": "deployment",
			"resource_name": "test-deployment",
		}

		result, err := k8sTool.handleDeleteResource(ctx, req)
		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.False(t, result.IsError)

		resultText := getResultText(result)
		assert.Contains(t, resultText, "deleted")
	})
}

func TestHandleCheckServiceConnectivity(t *testing.T) {
	ctx := context.Background()

	t.Run("missing service_name", func(t *testing.T) {
		mock := cmd.NewMockShellExecutor()
		ctx := cmd.WithShellExecutor(context.Background(), mock)

		k8sTool := newTestK8sTool()

		req := mcp.CallToolRequest{}
		req.Params.Arguments = map[string]interface{}{}

		result, err := k8sTool.handleCheckServiceConnectivity(ctx, req)
		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.True(t, result.IsError)

		// Verify no commands were executed since parameters are missing
		callLog := mock.GetCallLog()
		assert.Len(t, callLog, 0)
	})

	t.Run("valid service_name", func(t *testing.T) {
		mock := cmd.NewMockShellExecutor()

		// Mock the pod creation, wait, and exec commands using partial matchers
		mock.AddPartialMatcherString("kubectl", []string{"run", "*", "--image=curlimages/curl", "-n", "default", "--restart=Never", "--", "sleep", "3600"}, "pod/curl-test-123 created", nil)
		mock.AddPartialMatcherString("kubectl", []string{"wait", "--for=condition=ready", "*", "-n", "default", "--timeout=60s"}, "pod/curl-test-123 condition met", nil)
		mock.AddPartialMatcherString("kubectl", []string{"exec", "*", "-n", "default", "--", "curl", "-s", "test-service.default.svc.cluster.local:80"}, "Connection successful", nil)
		mock.AddPartialMatcherString("kubectl", []string{"delete", "pod", "*", "-n", "default", "--ignore-not-found"}, "pod deleted", nil)

		ctx := cmd.WithShellExecutor(ctx, mock)

		k8sTool := newTestK8sTool()

		req := mcp.CallToolRequest{}
		req.Params.Arguments = map[string]interface{}{
			"service_name": "test-service.default.svc.cluster.local:80",
		}

		result, err := k8sTool.handleCheckServiceConnectivity(ctx, req)
		assert.NoError(t, err)
		assert.NotNil(t, result)
		// Should attempt connectivity check (may succeed or fail but validates params)
	})
}

func TestHandleKubectlDescribeTool(t *testing.T) {
	ctx := context.Background()

	t.Run("missing parameters", func(t *testing.T) {
		mock := cmd.NewMockShellExecutor()
		ctx := cmd.WithShellExecutor(context.Background(), mock)

		k8sTool := newTestK8sTool()

		req := mcp.CallToolRequest{}
		req.Params.Arguments = map[string]interface{}{
			"resource_type": "deployment",
			// Missing resource_name
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

		req := mcp.CallToolRequest{}
		req.Params.Arguments = map[string]interface{}{
			"resource_type": "deployment",
			"resource_name": "test-deployment",
			"namespace":     "default",
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
		req := mcp.CallToolRequest{}
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
		req := mcp.CallToolRequest{}
		req.Params.Arguments = map[string]interface{}{"resource_type": "pods"}
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
		req := mcp.CallToolRequest{}
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
		req := mcp.CallToolRequest{}
		req.Params.Arguments = map[string]interface{}{"pod_name": "test-pod"}
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

		req := mcp.CallToolRequest{}
		req.Params.Arguments = map[string]interface{}{
			"manifest": manifest,
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

		req := mcp.CallToolRequest{}
		req.Params.Arguments = map[string]interface{}{
			// Missing manifest parameter
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

		req := mcp.CallToolRequest{}
		req.Params.Arguments = map[string]interface{}{
			"pod_name":  "mypod",
			"namespace": "default",
			"command":   "ls -la",
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

		req := mcp.CallToolRequest{}
		req.Params.Arguments = map[string]interface{}{
			"pod_name": "mypod",
			// Missing command parameter
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

func TestHandleRollout(t *testing.T) {
	ctx := context.Background()
	t.Run("rollout restart deployment", func(t *testing.T) {
		mock := cmd.NewMockShellExecutor()
		expectedOutput := `deployment.apps/myapp restarted`

		mock.AddCommandString("kubectl", []string{"rollout", "restart", "deployment/myapp", "-n", "default"}, expectedOutput, nil)
		ctx := cmd.WithShellExecutor(ctx, mock)

		k8sTool := newTestK8sTool()

		req := mcp.CallToolRequest{}
		req.Params.Arguments = map[string]interface{}{
			"action":        "restart",
			"resource_type": "deployment",
			"resource_name": "myapp",
			"namespace":     "default",
		}

		result, err := k8sTool.handleRollout(ctx, req)
		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.False(t, result.IsError)

		// Verify the expected output
		content := getResultText(result)
		assert.Contains(t, content, "restarted")

		// Verify the correct kubectl command was called
		callLog := mock.GetCallLog()
		require.Len(t, callLog, 1)
		assert.Equal(t, "kubectl", callLog[0].Command)
		assert.Equal(t, []string{"rollout", "restart", "deployment/myapp", "-n", "default"}, callLog[0].Args)
	})

	t.Run("missing required parameters", func(t *testing.T) {
		mock := cmd.NewMockShellExecutor()
		ctx := cmd.WithShellExecutor(context.Background(), mock)

		k8sTool := newTestK8sTool()

		req := mcp.CallToolRequest{}
		req.Params.Arguments = map[string]interface{}{
			"action": "restart",
			// Missing resource_type and resource_name
		}

		result, err := k8sTool.handleRollout(ctx, req)
		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.True(t, result.IsError)
		assert.Contains(t, getResultText(result), "required")

		// Verify no commands were executed since parameters are missing
		callLog := mock.GetCallLog()
		assert.Len(t, callLog, 0)
	})
}

// Mock LLM for testing
type mockLLM struct {
	called   int
	response *llms.ContentResponse
	error    error
}

func newMockLLM(response *llms.ContentResponse, err error) *mockLLM {
	return &mockLLM{
		response: response,
		error:    err,
	}
}

func (m *mockLLM) Call(ctx context.Context, prompt string, options ...llms.CallOption) (string, error) {
	return "", nil
}

func (m *mockLLM) GenerateContent(ctx context.Context, _ []llms.MessageContent, options ...llms.CallOption) (*llms.ContentResponse, error) {
	m.called++
	return m.response, m.error
}

func TestHandleGenerateResource(t *testing.T) {
	ctx := context.Background()

	t.Run("success", func(t *testing.T) {
		expectedYAML := `apiVersion: security.istio.io/v1beta1
kind: PeerAuthentication
metadata:
  name: default
  namespace: foo
spec:
  mtls:
    mode: STRICT`

		mockLLM := newMockLLM(&llms.ContentResponse{
			Choices: []*llms.ContentChoice{
				{Content: expectedYAML},
			},
		}, nil)

		k8sTool := newTestK8sToolWithLLM(mockLLM)

		req := mcp.CallToolRequest{}
		req.Params.Arguments = map[string]interface{}{
			"resource_type":        "istio_auth_policy",
			"resource_description": "A peer authentication policy for strict mTLS",
		}

		result, err := k8sTool.handleGenerateResource(ctx, req)
		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.False(t, result.IsError)

		resultText := getResultText(result)
		assert.Contains(t, resultText, "PeerAuthentication")
		assert.Contains(t, resultText, "STRICT")

		// Verify the mock was called
		assert.Equal(t, 1, mockLLM.called)
	})

	t.Run("missing parameters", func(t *testing.T) {
		k8sTool := newTestK8sTool()

		req := mcp.CallToolRequest{}
		req.Params.Arguments = map[string]interface{}{
			"resource_type": "istio_auth_policy",
			// Missing resource_description
		}

		result, err := k8sTool.handleGenerateResource(ctx, req)
		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.True(t, result.IsError)
		assert.Contains(t, getResultText(result), "required")
	})

	t.Run("no LLM model", func(t *testing.T) {
		k8sTool := newTestK8sTool() // No LLM model

		req := mcp.CallToolRequest{}
		req.Params.Arguments = map[string]interface{}{
			"resource_type":        "istio_auth_policy",
			"resource_description": "A peer authentication policy for strict mTLS",
		}

		result, err := k8sTool.handleGenerateResource(ctx, req)
		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.True(t, result.IsError)
		assert.Contains(t, getResultText(result), "No LLM client present")
	})

	t.Run("invalid resource type", func(t *testing.T) {
		mockLLM := newMockLLM(&llms.ContentResponse{
			Choices: []*llms.ContentChoice{
				{Content: "test"},
			},
		}, nil)

		k8sTool := newTestK8sToolWithLLM(mockLLM)

		req := mcp.CallToolRequest{}
		req.Params.Arguments = map[string]interface{}{
			"resource_type":        "invalid_resource_type",
			"resource_description": "A test resource",
		}

		result, err := k8sTool.handleGenerateResource(ctx, req)
		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.True(t, result.IsError)
		assert.Contains(t, getResultText(result), "resource type invalid_resource_type not found")

		// Verify the mock was not called
		assert.Equal(t, 0, mockLLM.called)
	})
}

// Test additional handlers that were missing tests
func TestHandleAnnotateResource(t *testing.T) {
	ctx := context.Background()

	t.Run("success", func(t *testing.T) {
		mock := cmd.NewMockShellExecutor()
		expectedOutput := `deployment.apps/test-deployment annotated`
		mock.AddCommandString("kubectl", []string{"annotate", "deployment", "test-deployment", "key1=value1", "key2=value2", "-n", "default"}, expectedOutput, nil)
		ctx := cmd.WithShellExecutor(ctx, mock)

		k8sTool := newTestK8sTool()

		req := mcp.CallToolRequest{}
		req.Params.Arguments = map[string]interface{}{
			"resource_type": "deployment",
			"resource_name": "test-deployment",
			"annotations":   "key1=value1 key2=value2",
			"namespace":     "default",
		}

		result, err := k8sTool.handleAnnotateResource(ctx, req)
		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.False(t, result.IsError)

		resultText := getResultText(result)
		assert.Contains(t, resultText, "annotated")
	})

	t.Run("missing parameters", func(t *testing.T) {
		mock := cmd.NewMockShellExecutor()
		ctx := cmd.WithShellExecutor(context.Background(), mock)

		k8sTool := newTestK8sTool()

		req := mcp.CallToolRequest{}
		req.Params.Arguments = map[string]interface{}{
			"resource_type": "deployment",
			// Missing resource_name and annotations
		}

		result, err := k8sTool.handleAnnotateResource(ctx, req)
		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.True(t, result.IsError)
		assert.Contains(t, getResultText(result), "required")

		// Verify no commands were executed since parameters are missing
		callLog := mock.GetCallLog()
		assert.Len(t, callLog, 0)
	})
}

func TestHandleLabelResource(t *testing.T) {
	ctx := context.Background()

	t.Run("success", func(t *testing.T) {
		mock := cmd.NewMockShellExecutor()
		expectedOutput := `deployment.apps/test-deployment labeled`
		mock.AddCommandString("kubectl", []string{"label", "deployment", "test-deployment", "env=prod", "version=1.0", "-n", "default"}, expectedOutput, nil)
		ctx := cmd.WithShellExecutor(ctx, mock)

		k8sTool := newTestK8sTool()

		req := mcp.CallToolRequest{}
		req.Params.Arguments = map[string]interface{}{
			"resource_type": "deployment",
			"resource_name": "test-deployment",
			"labels":        "env=prod version=1.0",
			"namespace":     "default",
		}

		result, err := k8sTool.handleLabelResource(ctx, req)
		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.False(t, result.IsError)

		resultText := getResultText(result)
		assert.Contains(t, resultText, "labeled")
	})

	t.Run("missing parameters", func(t *testing.T) {
		mock := cmd.NewMockShellExecutor()
		ctx := cmd.WithShellExecutor(context.Background(), mock)

		k8sTool := newTestK8sTool()

		req := mcp.CallToolRequest{}
		req.Params.Arguments = map[string]interface{}{
			"resource_type": "deployment",
			// Missing resource_name and labels
		}

		result, err := k8sTool.handleLabelResource(ctx, req)
		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.True(t, result.IsError)
		assert.Contains(t, getResultText(result), "required")

		// Verify no commands were executed since parameters are missing
		callLog := mock.GetCallLog()
		assert.Len(t, callLog, 0)
	})
}

func TestHandleRemoveAnnotation(t *testing.T) {
	ctx := context.Background()

	t.Run("success", func(t *testing.T) {
		mock := cmd.NewMockShellExecutor()
		expectedOutput := `deployment.apps/test-deployment annotated`
		mock.AddCommandString("kubectl", []string{"annotate", "deployment", "test-deployment", "key1-", "-n", "default"}, expectedOutput, nil)
		ctx := cmd.WithShellExecutor(ctx, mock)

		k8sTool := newTestK8sTool()

		req := mcp.CallToolRequest{}
		req.Params.Arguments = map[string]interface{}{
			"resource_type":  "deployment",
			"resource_name":  "test-deployment",
			"annotation_key": "key1",
			"namespace":      "default",
		}

		result, err := k8sTool.handleRemoveAnnotation(ctx, req)
		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.False(t, result.IsError)

		resultText := getResultText(result)
		assert.Contains(t, resultText, "annotated")
	})

	t.Run("missing parameters", func(t *testing.T) {
		mock := cmd.NewMockShellExecutor()
		ctx := cmd.WithShellExecutor(context.Background(), mock)

		k8sTool := newTestK8sTool()

		req := mcp.CallToolRequest{}
		req.Params.Arguments = map[string]interface{}{
			"resource_type": "deployment",
			// Missing resource_name and annotation_key
		}

		result, err := k8sTool.handleRemoveAnnotation(ctx, req)
		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.True(t, result.IsError)
		assert.Contains(t, getResultText(result), "required")

		// Verify no commands were executed since parameters are missing
		callLog := mock.GetCallLog()
		assert.Len(t, callLog, 0)
	})
}

func TestHandleRemoveLabel(t *testing.T) {
	ctx := context.Background()

	t.Run("success", func(t *testing.T) {
		mock := cmd.NewMockShellExecutor()
		expectedOutput := `deployment.apps/test-deployment labeled`
		mock.AddCommandString("kubectl", []string{"label", "deployment", "test-deployment", "env-", "-n", "default"}, expectedOutput, nil)
		ctx := cmd.WithShellExecutor(ctx, mock)

		k8sTool := newTestK8sTool()

		req := mcp.CallToolRequest{}
		req.Params.Arguments = map[string]interface{}{
			"resource_type": "deployment",
			"resource_name": "test-deployment",
			"label_key":     "env",
			"namespace":     "default",
		}

		result, err := k8sTool.handleRemoveLabel(ctx, req)
		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.False(t, result.IsError)

		resultText := getResultText(result)
		assert.Contains(t, resultText, "labeled")
	})

	t.Run("missing parameters", func(t *testing.T) {
		mock := cmd.NewMockShellExecutor()
		ctx := cmd.WithShellExecutor(context.Background(), mock)

		k8sTool := newTestK8sTool()

		req := mcp.CallToolRequest{}
		req.Params.Arguments = map[string]interface{}{
			"resource_type": "deployment",
			// Missing resource_name and label_key
		}

		result, err := k8sTool.handleRemoveLabel(ctx, req)
		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.True(t, result.IsError)
		assert.Contains(t, getResultText(result), "required")

		// Verify no commands were executed since parameters are missing
		callLog := mock.GetCallLog()
		assert.Len(t, callLog, 0)
	})
}

func TestHandleCreateResourceFromURL(t *testing.T) {
	ctx := context.Background()

	t.Run("success", func(t *testing.T) {
		mock := cmd.NewMockShellExecutor()
		expectedOutput := `deployment.apps/test-deployment created`
		mock.AddCommandString("kubectl", []string{"create", "-f", "https://example.com/manifest.yaml", "-n", "default"}, expectedOutput, nil)
		ctx := cmd.WithShellExecutor(ctx, mock)

		k8sTool := newTestK8sTool()

		req := mcp.CallToolRequest{}
		req.Params.Arguments = map[string]interface{}{
			"url":       "https://example.com/manifest.yaml",
			"namespace": "default",
		}

		result, err := k8sTool.handleCreateResourceFromURL(ctx, req)
		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.False(t, result.IsError)

		resultText := getResultText(result)
		assert.Contains(t, resultText, "created")
	})

	t.Run("missing url parameter", func(t *testing.T) {
		mock := cmd.NewMockShellExecutor()
		ctx := cmd.WithShellExecutor(context.Background(), mock)

		k8sTool := newTestK8sTool()

		req := mcp.CallToolRequest{}
		req.Params.Arguments = map[string]interface{}{
			// Missing url parameter
		}

		result, err := k8sTool.handleCreateResourceFromURL(ctx, req)
		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.True(t, result.IsError)
		assert.Contains(t, getResultText(result), "url parameter is required")

		// Verify no commands were executed since parameters are missing
		callLog := mock.GetCallLog()
		assert.Len(t, callLog, 0)
	})
}

func TestHandleGetClusterConfiguration(t *testing.T) {
	ctx := context.Background()

	t.Run("success", func(t *testing.T) {
		mock := cmd.NewMockShellExecutor()
		expectedOutput := `apiVersion: v1
clusters:
- cluster:
    server: https://kubernetes.default.svc
  name: default
contexts:
- context:
    cluster: default
    user: default
  name: default
current-context: default
kind: Config
preferences: {}
users:
- name: default`
		mock.AddCommandString("kubectl", []string{"config", "view", "-o", "json"}, expectedOutput, nil)
		ctx := cmd.WithShellExecutor(ctx, mock)

		k8sTool := newTestK8sTool()

		req := mcp.CallToolRequest{}
		result, err := k8sTool.handleGetClusterConfiguration(ctx, req)
		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.False(t, result.IsError)

		resultText := getResultText(result)
		assert.Contains(t, resultText, "current-context")
		assert.Contains(t, resultText, "clusters")
	})
}

// Tests for Bearer token passing to kubectl commands
func TestBearerTokenPassthrough(t *testing.T) {
	ctx := context.Background()

	t.Run("get resources with bearer token", func(t *testing.T) {
		mock := cmd.NewMockShellExecutor()
		expectedOutput := `NAME   READY   STATUS    RESTARTS   AGE`
		mock.AddCommandString("kubectl", []string{"get", "pods", "-o", "wide", "--token", "test-token-123"}, expectedOutput, nil)
		ctx := cmd.WithShellExecutor(ctx, mock)

		k8sTool := newTestK8sToolWithPassthrough(true)
		req := requestWithBearerToken("test-token-123", map[string]interface{}{"resource_type": "pods"})
		result, err := k8sTool.handleKubectlGetEnhanced(ctx, req)
		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.False(t, result.IsError)

		// Verify the command was executed with the token
		callLog := mock.GetCallLog()
		require.Len(t, callLog, 1)
		assert.Equal(t, "kubectl", callLog[0].Command)
		assert.Contains(t, callLog[0].Args, "--token")
		assert.Contains(t, callLog[0].Args, "test-token-123")
	})

	t.Run("scale deployment with bearer token", func(t *testing.T) {
		mock := cmd.NewMockShellExecutor()
		expectedOutput := `deployment.apps/test-deployment scaled`
		mock.AddCommandString("kubectl", []string{"scale", "deployment", "test-deployment", "--replicas", "5", "-n", "default", "--token", "my-auth-token"}, expectedOutput, nil)
		ctx := cmd.WithShellExecutor(ctx, mock)

		k8sTool := newTestK8sToolWithPassthrough(true)
		req := requestWithBearerToken("my-auth-token", map[string]interface{}{
			"name":     "test-deployment",
			"replicas": float64(5),
		})

		result, err := k8sTool.handleScaleDeployment(ctx, req)
		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.False(t, result.IsError)

		// Verify the command was executed with the token
		callLog := mock.GetCallLog()
		require.Len(t, callLog, 1)
		assert.Contains(t, callLog[0].Args, "--token")
		assert.Contains(t, callLog[0].Args, "my-auth-token")
	})

	t.Run("get pod logs with bearer token", func(t *testing.T) {
		mock := cmd.NewMockShellExecutor()
		expectedOutput := `log line 1
log line 2`
		mock.AddCommandString("kubectl", []string{"logs", "test-pod", "-n", "default", "--tail", "50", "--token", "logs-token"}, expectedOutput, nil)
		ctx := cmd.WithShellExecutor(ctx, mock)

		k8sTool := newTestK8sToolWithPassthrough(true)
		req := requestWithBearerToken("logs-token", map[string]interface{}{"pod_name": "test-pod"})
		result, err := k8sTool.handleKubectlLogsEnhanced(ctx, req)
		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.False(t, result.IsError)

		callLog := mock.GetCallLog()
		require.Len(t, callLog, 1)
		assert.Contains(t, callLog[0].Args, "--token")
		assert.Contains(t, callLog[0].Args, "logs-token")
	})

	t.Run("delete resource with bearer token", func(t *testing.T) {
		mock := cmd.NewMockShellExecutor()
		expectedOutput := `deployment.apps/test-deployment deleted`
		mock.AddCommandString("kubectl", []string{"delete", "deployment", "test-deployment", "-n", "default", "--token", "delete-token"}, expectedOutput, nil)
		ctx := cmd.WithShellExecutor(ctx, mock)

		k8sTool := newTestK8sToolWithPassthrough(true)
		req := requestWithBearerToken("delete-token", map[string]interface{}{
			"resource_type": "deployment",
			"resource_name": "test-deployment",
		})

		result, err := k8sTool.handleDeleteResource(ctx, req)
		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.False(t, result.IsError)

		callLog := mock.GetCallLog()
		require.Len(t, callLog, 1)
		assert.Contains(t, callLog[0].Args, "--token")
		assert.Contains(t, callLog[0].Args, "delete-token")
	})

	t.Run("patch resource with bearer token", func(t *testing.T) {
		mock := cmd.NewMockShellExecutor()
		expectedOutput := `deployment.apps/test-deployment patched`
		mock.AddCommandString("kubectl", []string{"patch", "deployment", "test-deployment", "-p", `{"spec":{"replicas":5}}`, "-n", "default", "--token", "patch-token"}, expectedOutput, nil)
		ctx := cmd.WithShellExecutor(ctx, mock)

		k8sTool := newTestK8sToolWithPassthrough(true)
		req := requestWithBearerToken("patch-token", map[string]interface{}{
			"resource_type": "deployment",
			"resource_name": "test-deployment",
			"patch":         `{"spec":{"replicas":5}}`,
		})

		result, err := k8sTool.handlePatchResource(ctx, req)
		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.False(t, result.IsError)

		callLog := mock.GetCallLog()
		require.Len(t, callLog, 1)
		assert.Contains(t, callLog[0].Args, "--token")
		assert.Contains(t, callLog[0].Args, "patch-token")
	})

	t.Run("describe resource with bearer token", func(t *testing.T) {
		mock := cmd.NewMockShellExecutor()
		expectedOutput := `Name: test-deployment`
		mock.AddCommandString("kubectl", []string{"describe", "deployment", "test-deployment", "-n", "default", "--token", "describe-token"}, expectedOutput, nil)
		ctx := cmd.WithShellExecutor(ctx, mock)

		k8sTool := newTestK8sToolWithPassthrough(true)
		req := requestWithBearerToken("describe-token", map[string]interface{}{
			"resource_type": "deployment",
			"resource_name": "test-deployment",
			"namespace":     "default",
		})

		result, err := k8sTool.handleKubectlDescribeTool(ctx, req)
		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.False(t, result.IsError)

		callLog := mock.GetCallLog()
		require.Len(t, callLog, 1)
		assert.Contains(t, callLog[0].Args, "--token")
		assert.Contains(t, callLog[0].Args, "describe-token")
	})

	t.Run("rollout with bearer token", func(t *testing.T) {
		mock := cmd.NewMockShellExecutor()
		expectedOutput := `deployment.apps/myapp restarted`
		mock.AddCommandString("kubectl", []string{"rollout", "restart", "deployment/myapp", "-n", "default", "--token", "rollout-token"}, expectedOutput, nil)
		ctx := cmd.WithShellExecutor(ctx, mock)

		k8sTool := newTestK8sToolWithPassthrough(true)
		req := requestWithBearerToken("rollout-token", map[string]interface{}{
			"action":        "restart",
			"resource_type": "deployment",
			"resource_name": "myapp",
			"namespace":     "default",
		})

		result, err := k8sTool.handleRollout(ctx, req)
		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.False(t, result.IsError)

		callLog := mock.GetCallLog()
		require.Len(t, callLog, 1)
		assert.Contains(t, callLog[0].Args, "--token")
		assert.Contains(t, callLog[0].Args, "rollout-token")
	})

	t.Run("get events with bearer token", func(t *testing.T) {
		mock := cmd.NewMockShellExecutor()
		expectedOutput := `{"items": []}`
		mock.AddCommandString("kubectl", []string{"get", "events", "-o", "json", "--all-namespaces", "--token", "events-token"}, expectedOutput, nil)
		ctx := cmd.WithShellExecutor(ctx, mock)

		k8sTool := newTestK8sToolWithPassthrough(true)
		req := requestWithBearerToken("events-token", nil)
		result, err := k8sTool.handleGetEvents(ctx, req)
		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.False(t, result.IsError)

		callLog := mock.GetCallLog()
		require.Len(t, callLog, 1)
		assert.Contains(t, callLog[0].Args, "--token")
		assert.Contains(t, callLog[0].Args, "events-token")
	})

	t.Run("exec command with bearer token", func(t *testing.T) {
		mock := cmd.NewMockShellExecutor()
		expectedOutput := `total 8`
		mock.AddCommandString("kubectl", []string{"exec", "mypod", "-n", "default", "--", "ls -la", "--token", "exec-token"}, expectedOutput, nil)
		ctx := cmd.WithShellExecutor(ctx, mock)

		k8sTool := newTestK8sToolWithPassthrough(true)
		req := requestWithBearerToken("exec-token", map[string]interface{}{
			"pod_name":  "mypod",
			"namespace": "default",
			"command":   "ls -la",
		})

		result, err := k8sTool.handleExecCommand(ctx, req)
		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.False(t, result.IsError)

		callLog := mock.GetCallLog()
		require.Len(t, callLog, 1)
		assert.Contains(t, callLog[0].Args, "--token")
		assert.Contains(t, callLog[0].Args, "exec-token")
	})

	t.Run("annotate resource with bearer token", func(t *testing.T) {
		mock := cmd.NewMockShellExecutor()
		expectedOutput := `deployment.apps/test-deployment annotated`
		mock.AddCommandString("kubectl", []string{"annotate", "deployment", "test-deployment", "key1=value1", "--token", "annotate-token"}, expectedOutput, nil)
		ctx := cmd.WithShellExecutor(ctx, mock)

		k8sTool := newTestK8sToolWithPassthrough(true)
		req := requestWithBearerToken("annotate-token", map[string]interface{}{
			"resource_type": "deployment",
			"resource_name": "test-deployment",
			"annotations":   "key1=value1",
		})

		result, err := k8sTool.handleAnnotateResource(ctx, req)
		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.False(t, result.IsError)

		callLog := mock.GetCallLog()
		require.Len(t, callLog, 1)
		assert.Contains(t, callLog[0].Args, "--token")
		assert.Contains(t, callLog[0].Args, "annotate-token")
	})

	t.Run("label resource with bearer token", func(t *testing.T) {
		mock := cmd.NewMockShellExecutor()
		expectedOutput := `deployment.apps/test-deployment labeled`
		mock.AddCommandString("kubectl", []string{"label", "deployment", "test-deployment", "env=prod", "--token", "label-token"}, expectedOutput, nil)
		ctx := cmd.WithShellExecutor(ctx, mock)

		k8sTool := newTestK8sToolWithPassthrough(true)
		req := requestWithBearerToken("label-token", map[string]interface{}{
			"resource_type": "deployment",
			"resource_name": "test-deployment",
			"labels":        "env=prod",
		})

		result, err := k8sTool.handleLabelResource(ctx, req)
		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.False(t, result.IsError)

		callLog := mock.GetCallLog()
		require.Len(t, callLog, 1)
		assert.Contains(t, callLog[0].Args, "--token")
		assert.Contains(t, callLog[0].Args, "label-token")
	})

	t.Run("api resources with bearer token", func(t *testing.T) {
		mock := cmd.NewMockShellExecutor()
		expectedOutput := `NAME   SHORTNAMES   APIVERSION   NAMESPACED   KIND`
		mock.AddCommandString("kubectl", []string{"api-resources", "--token", "api-token"}, expectedOutput, nil)
		ctx := cmd.WithShellExecutor(ctx, mock)

		k8sTool := newTestK8sToolWithPassthrough(true)
		req := requestWithBearerToken("api-token", nil)
		result, err := k8sTool.handleGetAvailableAPIResources(ctx, req)
		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.False(t, result.IsError)

		callLog := mock.GetCallLog()
		require.Len(t, callLog, 1)
		assert.Contains(t, callLog[0].Args, "--token")
		assert.Contains(t, callLog[0].Args, "api-token")
	})

	t.Run("cluster configuration with bearer token", func(t *testing.T) {
		mock := cmd.NewMockShellExecutor()
		expectedOutput := `{"current-context": "default"}`
		mock.AddCommandString("kubectl", []string{"config", "view", "-o", "json", "--token", "config-token"}, expectedOutput, nil)
		ctx := cmd.WithShellExecutor(ctx, mock)

		k8sTool := newTestK8sToolWithPassthrough(true)
		req := requestWithBearerToken("config-token", nil)
		result, err := k8sTool.handleGetClusterConfiguration(ctx, req)
		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.False(t, result.IsError)

		callLog := mock.GetCallLog()
		require.Len(t, callLog, 1)
		assert.Contains(t, callLog[0].Args, "--token")
		assert.Contains(t, callLog[0].Args, "config-token")
	})

	t.Run("remove annotation with bearer token", func(t *testing.T) {
		mock := cmd.NewMockShellExecutor()
		expectedOutput := `deployment.apps/test-deployment annotated`
		mock.AddCommandString("kubectl", []string{"annotate", "deployment", "test-deployment", "key1-", "--token", "remove-anno-token"}, expectedOutput, nil)
		ctx := cmd.WithShellExecutor(ctx, mock)

		k8sTool := newTestK8sToolWithPassthrough(true)
		req := requestWithBearerToken("remove-anno-token", map[string]interface{}{
			"resource_type":  "deployment",
			"resource_name":  "test-deployment",
			"annotation_key": "key1",
		})

		result, err := k8sTool.handleRemoveAnnotation(ctx, req)
		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.False(t, result.IsError)

		callLog := mock.GetCallLog()
		require.Len(t, callLog, 1)
		assert.Contains(t, callLog[0].Args, "--token")
		assert.Contains(t, callLog[0].Args, "remove-anno-token")
	})

	t.Run("remove label with bearer token", func(t *testing.T) {
		mock := cmd.NewMockShellExecutor()
		expectedOutput := `deployment.apps/test-deployment labeled`
		mock.AddCommandString("kubectl", []string{"label", "deployment", "test-deployment", "env-", "--token", "remove-label-token"}, expectedOutput, nil)
		ctx := cmd.WithShellExecutor(ctx, mock)

		k8sTool := newTestK8sToolWithPassthrough(true)
		req := requestWithBearerToken("remove-label-token", map[string]interface{}{
			"resource_type": "deployment",
			"resource_name": "test-deployment",
			"label_key":     "env",
		})

		result, err := k8sTool.handleRemoveLabel(ctx, req)
		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.False(t, result.IsError)

		callLog := mock.GetCallLog()
		require.Len(t, callLog, 1)
		assert.Contains(t, callLog[0].Args, "--token")
		assert.Contains(t, callLog[0].Args, "remove-label-token")
	})

	t.Run("create resource from URL with bearer token", func(t *testing.T) {
		mock := cmd.NewMockShellExecutor()
		expectedOutput := `deployment.apps/test-deployment created`
		mock.AddCommandString("kubectl", []string{"create", "-f", "https://example.com/manifest.yaml", "-n", "default", "--token", "url-token"}, expectedOutput, nil)
		ctx := cmd.WithShellExecutor(ctx, mock)

		k8sTool := newTestK8sToolWithPassthrough(true)
		req := requestWithBearerToken("url-token", map[string]interface{}{
			"url":       "https://example.com/manifest.yaml",
			"namespace": "default",
		})

		result, err := k8sTool.handleCreateResourceFromURL(ctx, req)
		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.False(t, result.IsError)

		callLog := mock.GetCallLog()
		require.Len(t, callLog, 1)
		assert.Contains(t, callLog[0].Args, "--token")
		assert.Contains(t, callLog[0].Args, "url-token")
	})

	t.Run("apply manifest with bearer token", func(t *testing.T) {
		mock := cmd.NewMockShellExecutor()
		manifest := `apiVersion: v1
kind: Pod
metadata:
  name: test-pod`
		expectedOutput := `pod/test-pod created`
		// Use partial matcher since temp file name is dynamic
		mock.AddPartialMatcherString("kubectl", []string{"apply", "-f"}, expectedOutput, nil)
		ctx := cmd.WithShellExecutor(ctx, mock)

		k8sTool := newTestK8sToolWithPassthrough(true)
		req := requestWithBearerToken("apply-token", map[string]interface{}{
			"manifest": manifest,
		})

		result, err := k8sTool.handleApplyManifest(ctx, req)
		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.False(t, result.IsError)

		callLog := mock.GetCallLog()
		require.Len(t, callLog, 1)
		assert.Contains(t, callLog[0].Args, "--token")
		assert.Contains(t, callLog[0].Args, "apply-token")
	})

	t.Run("returns error when passthrough true and authorization header missing", func(t *testing.T) {
		k8sTool := newTestK8sToolWithPassthrough(true)
		req := mcp.CallToolRequest{}
		req.Params.Arguments = map[string]interface{}{"resource_type": "pods"}
		result, err := k8sTool.handleKubectlGetEnhanced(ctx, req)
		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.True(t, result.IsError)
		assert.Contains(t, getResultText(result), "Bearer token required")
	})

	t.Run("no token when passthrough false and authorization header missing", func(t *testing.T) {
		mock := cmd.NewMockShellExecutor()
		expectedOutput := `NAME   READY   STATUS    RESTARTS   AGE`
		// No --token in expected args when passthrough is false
		mock.AddCommandString("kubectl", []string{"get", "pods", "-o", "wide"}, expectedOutput, nil)
		ctx := cmd.WithShellExecutor(ctx, mock)

		k8sTool := newTestK8sToolWithPassthrough(false)
		req := mcp.CallToolRequest{}
		req.Params.Arguments = map[string]interface{}{"resource_type": "pods"}
		// No Header set on request
		result, err := k8sTool.handleKubectlGetEnhanced(ctx, req)
		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.False(t, result.IsError)

		// Verify no --token was added
		callLog := mock.GetCallLog()
		require.Len(t, callLog, 1)
		assert.NotContains(t, callLog[0].Args, "--token")
	})

	t.Run("no token when passthrough false and authorization header is not bearer", func(t *testing.T) {
		mock := cmd.NewMockShellExecutor()
		expectedOutput := `NAME   READY   STATUS    RESTARTS   AGE`
		// No --token when passthrough is false
		mock.AddCommandString("kubectl", []string{"get", "pods", "-o", "wide"}, expectedOutput, nil)
		ctx := cmd.WithShellExecutor(ctx, mock)

		k8sTool := newTestK8sToolWithPassthrough(false)
		req := mcp.CallToolRequest{}
		req.Header = http.Header{}
		req.Header.Set("Authorization", "Basic dXNlcjpwYXNz")
		req.Params.Arguments = map[string]interface{}{"resource_type": "pods"}
		result, err := k8sTool.handleKubectlGetEnhanced(ctx, req)
		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.False(t, result.IsError)

		// Verify no --token was added
		callLog := mock.GetCallLog()
		require.Len(t, callLog, 1)
		assert.NotContains(t, callLog[0].Args, "--token")
	})
}
