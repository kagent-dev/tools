package utils

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/kagent-dev/tools/internal/cmd"
	mcp "github.com/kagent-dev/tools/internal/mcp"
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
		s := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "v0.0.1"}, nil)
		RegisterTools(s, false)
	})

	t.Run("read-only omits shell", func(t *testing.T) {
		s := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "v0.0.1"}, nil)
		RegisterTools(s, true)
	})
}

func TestHandleShellTool(t *testing.T) {
	mock := cmd.NewMockShellExecutor()
	mock.AddCommandString("echo", []string{"hi"}, "hi\n", nil)
	ctx := cmd.WithShellExecutor(context.Background(), mock)

	t.Run("success", func(t *testing.T) {
		res, _, err := handleShellTool(ctx, &mcp.CallToolRequest{}, shellParams{Command: "echo hi"})
		require.NoError(t, err)
		assert.False(t, res.IsError)
	})

	t.Run("missing command", func(t *testing.T) {
		res, _, err := handleShellTool(ctx, &mcp.CallToolRequest{}, shellParams{})
		require.NoError(t, err)
		assert.True(t, res.IsError)
		assert.Contains(t, getResultText(res), "command parameter is required")
	})

	t.Run("command error", func(t *testing.T) {
		m := cmd.NewMockShellExecutor()
		m.AddCommandString("false", []string{}, "", assert.AnError)
		errCtx := cmd.WithShellExecutor(context.Background(), m)
		res, _, err := handleShellTool(errCtx, &mcp.CallToolRequest{}, shellParams{Command: "false"})
		require.NoError(t, err)
		assert.True(t, res.IsError)
	})
}

func TestHandleMCPInspectTool(t *testing.T) {
	ctx := context.Background()

	t.Run("echoes input and headers", func(t *testing.T) {
		req := &mcp.CallToolRequest{
			Extra: &mcp.RequestExtra{
				Header: http.Header{
					"Authorization": []string{"Bearer test-token"},
					"X-Debug":       []string{"one", "two"},
				},
			},
		}

		result, output, err := handleMCPInspectTool(ctx, req, inspectInput{Echo: "hello"})
		require.NoError(t, err)
		require.NotNil(t, result)
		assert.False(t, result.IsError)

		// Credential-bearing headers are redacted: the transport hands handlers
		// the raw inbound header set, so echoing values would disclose the
		// caller's own bearer token. The header name is still reported so the
		// tool remains useful for debugging which headers arrived.
		expected := &inspectOutput{
			Echo: "hello",
			Headers: []inspectHeader{
				{Name: "Authorization", Values: []string{redactedPlaceholder}},
				{Name: "X-Debug", Values: []string{"one", "two"}},
			},
		}
		assert.Equal(t, expected, output)

		var rendered inspectOutput
		require.NoError(t, json.Unmarshal([]byte(getResultText(result)), &rendered))
		assert.Equal(t, *expected, rendered)

		// The secret must not appear anywhere in the rendered payload.
		assert.NotContains(t, getResultText(result), "test-token")
	})

	t.Run("redacts every sensitive header but keeps the value count", func(t *testing.T) {
		req := &mcp.CallToolRequest{
			Extra: &mcp.RequestExtra{
				Header: http.Header{
					"Authorization": []string{"Bearer a", "Bearer b"},
					"Cookie":        []string{"session=secret"},
					"X-Api-Key":     []string{"key-123"},
					"User-Agent":    []string{"probe/1.0"},
				},
			},
		}

		result, output, err := handleMCPInspectTool(ctx, req, inspectInput{})
		require.NoError(t, err)
		require.False(t, result.IsError)

		byName := map[string][]string{}
		for _, h := range output.Headers {
			byName[h.Name] = h.Values
		}

		// The count of received values is preserved, only the contents are hidden.
		assert.Equal(t, []string{redactedPlaceholder, redactedPlaceholder}, byName["Authorization"])
		assert.Equal(t, []string{redactedPlaceholder}, byName["Cookie"])
		assert.Equal(t, []string{redactedPlaceholder}, byName["X-Api-Key"])
		// Non-sensitive headers keep their values.
		assert.Equal(t, []string{"probe/1.0"}, byName["User-Agent"])

		rendered := getResultText(result)
		for _, secret := range []string{"Bearer a", "Bearer b", "session=secret", "key-123"} {
			assert.NotContains(t, rendered, secret)
		}
	})

	t.Run("works without headers", func(t *testing.T) {
		result, output, err := handleMCPInspectTool(ctx, &mcp.CallToolRequest{}, inspectInput{Echo: "stdio"})
		require.NoError(t, err)
		require.NotNil(t, result)
		assert.False(t, result.IsError)
		assert.Equal(t, &inspectOutput{Echo: "stdio", Headers: []inspectHeader{}}, output)
	})
}

func getResultText(result *mcp.CallToolResult) string {
	if result == nil || len(result.Content) == 0 {
		return ""
	}
	if textContent, ok := result.Content[0].(*mcp.TextContent); ok {
		return textContent.Text
	}
	return ""
}

// TestIsSensitiveHeader pins the redaction policy for mcp_inspect. An
// exact-match list proved insufficient (it missed GitLab's Private-Token, AWS's
// X-Amz-Security-Token and assorted X-*-Token variants), so the policy is
// deny-by-substring. This test guards both directions: known credential header
// names must be withheld, and innocuous headers must stay visible so the tool
// remains useful for debugging.
func TestIsSensitiveHeader(t *testing.T) {
	mustRedact := []string{
		"Authorization", "Proxy-Authorization", "Cookie", "Set-Cookie",
		"X-Api-Key", "X-Auth-Token", "X-Access-Token", "X-API-Token",
		"X-Amz-Security-Token", "Private-Token", "X-Gitlab-Token",
		"Authentication", "X-Credential", "X-Secret", "X-Password",
		"X-Session-Id", "X-JWT-Token", "X-Signature",
	}
	for _, name := range mustRedact {
		assert.True(t, isSensitiveHeader(http.CanonicalHeaderKey(name)),
			"%s carries a credential and must be redacted", name)
	}

	keepVisible := []string{
		"Accept", "Content-Type", "Content-Length", "User-Agent",
		"Accept-Encoding", "Traceparent", "X-Request-Id", "Cache-Control",
	}
	for _, name := range keepVisible {
		assert.False(t, isSensitiveHeader(http.CanonicalHeaderKey(name)),
			"%s is not a credential and should stay visible for debugging", name)
	}
}
