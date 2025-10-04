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
