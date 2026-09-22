package utils

import (
	"context"
	"testing"

	"github.com/kagent-dev/tools/internal/cmd"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestKubeconfigManager(t *testing.T) {
	// Preserve and restore global state to avoid cross-test interference.
	original := GetKubeconfig()
	t.Cleanup(func() { SetKubeconfig(original) })

	t.Run("set and get", func(t *testing.T) {
		SetKubeconfig("/tmp/my-kubeconfig")
		assert.Equal(t, "/tmp/my-kubeconfig", GetKubeconfig())
	})

	t.Run("AddKubeconfigArgs with path set", func(t *testing.T) {
		SetKubeconfig("/tmp/kc")
		got := AddKubeconfigArgs([]string{"get", "pods"})
		assert.Equal(t, []string{"--kubeconfig", "/tmp/kc", "get", "pods"}, got)
	})

	t.Run("AddKubeconfigArgs with empty path", func(t *testing.T) {
		SetKubeconfig("")
		got := AddKubeconfigArgs([]string{"get", "pods"})
		assert.Equal(t, []string{"get", "pods"}, got)
	})
}

func TestShellTool(t *testing.T) {
	t.Run("executes command", func(t *testing.T) {
		mock := cmd.NewMockShellExecutor()
		mock.AddCommandString("echo", []string{"hello"}, "hello\n", nil)
		ctx := cmd.WithShellExecutor(context.Background(), mock)

		out, err := shellTool(ctx, shellParams{Command: "echo hello"})
		require.NoError(t, err)
		assert.Equal(t, "hello\n", out)

		callLog := mock.GetCallLog()
		require.Len(t, callLog, 1)
		assert.Equal(t, "echo", callLog[0].Command)
		assert.Equal(t, []string{"hello"}, callLog[0].Args)
	})

	t.Run("empty command", func(t *testing.T) {
		_, err := shellTool(context.Background(), shellParams{Command: "   "})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "empty command")
	})

	t.Run("command failure propagates", func(t *testing.T) {
		mock := cmd.NewMockShellExecutor()
		mock.AddCommandString("false", []string{}, "", assert.AnError)
		ctx := cmd.WithShellExecutor(context.Background(), mock)

		_, err := shellTool(ctx, shellParams{Command: "false"})
		require.Error(t, err)
	})
}

func TestRegisterTools(t *testing.T) {
	t.Run("read-write registers shell", func(t *testing.T) {
		s := server.NewMCPServer("test", "v0.0.1")
		RegisterTools(s, false)
	})

	t.Run("read-only omits shell", func(t *testing.T) {
		s := server.NewMCPServer("test", "v0.0.1")
		RegisterTools(s, true)
	})
}

func TestHandleShellTool(t *testing.T) {
	mock := cmd.NewMockShellExecutor()
	mock.AddCommandString("echo", []string{"hi"}, "hi\n", nil)
	ctx := cmd.WithShellExecutor(context.Background(), mock)

	t.Run("success", func(t *testing.T) {
		req := mcp.CallToolRequest{}
		req.Params.Arguments = map[string]interface{}{"command": "echo hi"}
		res, err := handleShellTool(ctx, req)
		require.NoError(t, err)
		assert.False(t, res.IsError)
	})

	t.Run("missing command", func(t *testing.T) {
		req := mcp.CallToolRequest{}
		req.Params.Arguments = map[string]interface{}{}
		res, err := handleShellTool(ctx, req)
		require.NoError(t, err)
		assert.True(t, res.IsError)
		assert.Contains(t, getResultText(res), "command parameter is required")
	})

	t.Run("command error", func(t *testing.T) {
		m := cmd.NewMockShellExecutor()
		m.AddCommandString("false", []string{}, "", assert.AnError)
		errCtx := cmd.WithShellExecutor(context.Background(), m)
		req := mcp.CallToolRequest{}
		req.Params.Arguments = map[string]interface{}{"command": "false"}
		res, err := handleShellTool(errCtx, req)
		require.NoError(t, err)
		assert.True(t, res.IsError)
	})
}

func getResultText(result *mcp.CallToolResult) string {
	if result == nil || len(result.Content) == 0 {
		return ""
	}
	if textContent, ok := result.Content[0].(mcp.TextContent); ok {
		return textContent.Text
	}
	return ""
}
