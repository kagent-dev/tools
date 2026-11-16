package k8s

import (
	"context"
	"strings"
	"testing"

	"github.com/kagent-dev/tools/internal/cmd"
	"github.com/kagent-dev/tools/internal/commands/commandstest"
	mcp "github.com/kagent-dev/tools/internal/mcp"
	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

// TestBearerTokenNotInFailedResult covers the reported leak: a failing kubectl
// call's tool result must not contain the caller's token.
func TestBearerTokenNotInFailedResult(t *testing.T) {
	const token = "eyJhbGciOiJSUzI1NiJ9.leak-check-token.sig"
	mock := cmd.NewMockShellExecutor()
	mock.AddPartialMatcherString("kubectl", []string{"get", "secrets"}, `Error from server (Forbidden): secrets is forbidden`, assert.AnError)
	ctx, seen := commandstest.WithPassthroughKubeconfig(t, context.Background(), mock)

	exp := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exp))
	prev := otel.GetTracerProvider()
	otel.SetTracerProvider(tp)
	t.Cleanup(func() {
		otel.SetTracerProvider(prev)
		_ = tp.Shutdown(context.Background())
	})

	// Run the handler behind the same middleware the server installs, so the
	// tool span (status from the result text) is covered too.
	k8sTool := newTestK8sToolWithPassthrough(true)
	req := requestWithBearerToken(token)
	req.Params = &sdkmcp.CallToolParamsRaw{Name: "k8s_get_resources"}
	h := mcp.ToolMiddleware()(func(ctx context.Context, _ string, r sdkmcp.Request) (sdkmcp.Result, error) {
		res, _, err := k8sTool.handleKubectlGetEnhanced(ctx, r.(*sdkmcp.CallToolRequest), getResourcesInput{ResourceType: "secrets"})
		return res, err
	})
	res, err := h(ctx, "tools/call", req)
	require.NoError(t, err)
	result, ok := res.(*sdkmcp.CallToolResult)
	require.True(t, ok)
	require.NotNil(t, result)
	assert.True(t, result.IsError)
	text := getResultText(result)
	assert.NotContains(t, text, token)
	assert.Contains(t, text, "get secrets")
	for _, c := range result.Content {
		if tc, ok := c.(*sdkmcp.TextContent); ok {
			assert.False(t, strings.Contains(tc.Text, token))
		}
	}
	callLog := mock.GetCallLog()
	require.Len(t, callLog, 1)
	commandstest.AssertTokenOnlyInKubeconfig(t, callLog[0].Args, seen, token)

	spans := exp.GetSpans()
	names := map[string]bool{}
	for _, s := range spans {
		names[s.Name] = true
		assert.NotContains(t, s.Status.Description, token, "span %s status", s.Name)
		for _, a := range s.Attributes {
			assert.NotContains(t, a.Value.Emit(), token, "span %s attribute %s", s.Name, a.Key)
		}
		for _, ev := range s.Events {
			for _, a := range ev.Attributes {
				assert.NotContains(t, a.Value.Emit(), token, "span %s event %s", s.Name, ev.Name)
			}
		}
	}
	assert.True(t, names["mcp.tool.k8s_get_resources"], "tool span recorded, got %v", names)
	assert.True(t, names["commands.execute"], "command span recorded, got %v", names)
}
