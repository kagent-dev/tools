package commandstest

import (
	"context"
	"strings"
	"testing"

	"github.com/kagent-dev/tools/internal/cmd"
	mcp "github.com/kagent-dev/tools/internal/mcp"
	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// PassthroughTestToken is the bearer token the passthrough tests send.
const PassthroughTestToken = "test-token"

// Exec is a command a tool is expected to run, as the caller asked for it
// (without the --kubeconfig pair), and the output the mock returns for it.
type Exec struct {
	Command string
	Args    []string
	Output  string
}

// ToolCall invokes one tool handler for the passthrough tests.
type ToolCall struct {
	Name  string
	Execs []Exec
	Run   func(ctx context.Context, req *sdkmcp.CallToolRequest) (*sdkmcp.CallToolResult, error)
}

// Call builds a ToolCall for a typed handler with input in that is expected to
// run execs.
func Call[In, Out any](name string, h func(context.Context, *sdkmcp.CallToolRequest, In) (*sdkmcp.CallToolResult, Out, error), in In, execs ...Exec) ToolCall {
	return ToolCall{
		Name:  name,
		Execs: execs,
		Run: func(ctx context.Context, req *sdkmcp.CallToolRequest) (*sdkmcp.CallToolResult, error) {
			res, _, err := h(ctx, req, in)
			return res, err
		},
	}
}

// RunTokenPassthroughTests checks a provider's TOKEN_PASSTHROUGH behaviour:
//
//   - cluster calls, passthrough on, bearer token: every command runs with a
//     per-call kubeconfig that carries the caller's token, never on argv;
//   - cluster calls, passthrough on, no bearer token: an error result, and no
//     command runs;
//   - local calls (no cluster access), passthrough on, no bearer token: the
//     commands run, without a per-call kubeconfig;
//   - all calls, passthrough off, bearer token: the token is ignored.
func RunTokenPassthroughTests(t *testing.T, cluster, local []ToolCall) {
	t.Helper()
	const token = PassthroughTestToken

	run := func(t *testing.T, c ToolCall, passthrough string, req *sdkmcp.CallToolRequest) ([]cmd.MockCall, *Kubeconfig) {
		t.Helper()
		t.Setenv(mcp.TokenPassthroughEnv, passthrough)
		mock := cmd.NewMockShellExecutor()
		for _, e := range c.Execs {
			mock.AddCommandString(e.Command, e.Args, e.Output, nil)
		}
		ctx, seen := WithPassthroughKubeconfig(t, context.Background(), mock)
		res, err := c.Run(ctx, req)
		require.NoError(t, err)
		require.NotNil(t, res)
		require.False(t, res.IsError, resultText(res))
		callLog := mock.GetCallLog()
		require.Len(t, callLog, len(c.Execs))
		for i, e := range c.Execs {
			assert.Equal(t, e.Command, callLog[i].Command)
			assert.Equal(t, e.Args, callLog[i].Args)
		}
		return callLog, seen
	}

	t.Run("cluster tools authenticate as the caller", func(t *testing.T) {
		for _, c := range cluster {
			t.Run(c.Name, func(t *testing.T) {
				callLog, seen := run(t, c, "true", RequestWithBearerToken(token))
				want := make([]string, len(c.Execs))
				for i := range want {
					want[i] = token
				}
				assert.Equal(t, want, seen.Tokens(), "every command must get a per-call kubeconfig with the caller's token")
				for _, call := range callLog {
					AssertTokenOnlyInKubeconfig(t, call.Args, seen, token)
				}
			})
		}
	})

	t.Run("cluster tools refuse to run without a bearer token", func(t *testing.T) {
		for _, c := range cluster {
			t.Run(c.Name, func(t *testing.T) {
				t.Setenv(mcp.TokenPassthroughEnv, "true")
				mock := cmd.NewMockShellExecutor()
				ctx := cmd.WithShellExecutor(context.Background(), mock)
				res, err := c.Run(ctx, &sdkmcp.CallToolRequest{})
				require.NoError(t, err)
				require.NotNil(t, res)
				assert.True(t, res.IsError, "missing token must be an error result")
				assert.Contains(t, resultText(res), mcp.ErrBearerTokenRequired.Error())
				assert.Empty(t, mock.GetCallLog(), "no command may run without the caller's token")
			})
		}
	})

	t.Run("local tools need no token", func(t *testing.T) {
		for _, c := range local {
			t.Run(c.Name, func(t *testing.T) {
				_, seen := run(t, c, "true", &sdkmcp.CallToolRequest{})
				assert.Empty(t, seen.Tokens(), "no per-call kubeconfig for a command that does not reach the cluster")
			})
		}
	})

	t.Run("passthrough off ignores the request's token", func(t *testing.T) {
		for _, c := range append(append([]ToolCall(nil), cluster...), local...) {
			t.Run(c.Name, func(t *testing.T) {
				callLog, seen := run(t, c, "false", RequestWithBearerToken(token))
				assert.Empty(t, seen.Tokens(), "no per-call kubeconfig with passthrough off")
				for _, call := range callLog {
					for _, a := range call.Args {
						assert.NotContains(t, a, token)
					}
				}
			})
		}
	})
}

func resultText(res *sdkmcp.CallToolResult) string {
	if res == nil {
		return ""
	}
	var b strings.Builder
	for _, c := range res.Content {
		if tc, ok := c.(*sdkmcp.TextContent); ok {
			b.WriteString(tc.Text)
		}
	}
	return b.String()
}
