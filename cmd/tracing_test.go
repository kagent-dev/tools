package main

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	mcpserver "github.com/kagent-dev/tools/internal/mcp"
	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
)

const (
	// Each traceparent stands for a different caller trace, so the test can
	// tell which request a span was parented on.
	initializeTraceparent = "00-11111111111111111111111111111111-1111111111111111-01"
	okCallTraceparent     = "00-22222222222222222222222222222222-2222222222222222-01"
	toolErrTraceparent    = "00-33333333333333333333333333333333-3333333333333333-01"
	goErrTraceparent      = "00-44444444444444444444444444444444-4444444444444444-01"
	closeTraceparent      = "00-55555555555555555555555555555555-5555555555555555-01"

	// Values that must never reach a span.
	tracingTestBearer   = "Bearer tracing-test-credential"
	tracingTestArgument = "tracing-test-argument"
)

type tracingTestInput struct {
	Value string `json:"value,omitempty" jsonschema:"an arbitrary value"`
}

// traceparentTransport sets the traceparent and Authorization headers on every
// request, the way a traced MCP client in an agent framework would.
type traceparentTransport struct {
	mu          sync.Mutex
	traceparent string
}

func (t *traceparentTransport) set(traceparent string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.traceparent = traceparent
}

func (t *traceparentTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	t.mu.Lock()
	traceparent := t.traceparent
	t.mu.Unlock()

	r = r.Clone(r.Context())
	r.Header.Set("traceparent", traceparent)
	r.Header.Set("Authorization", tracingTestBearer)
	return http.DefaultTransport.RoundTrip(r)
}

func mustSpanContext(t *testing.T, traceparent string) trace.SpanContext {
	t.Helper()
	ctx := propagation.TraceContext{}.Extract(context.Background(),
		propagation.MapCarrier{"traceparent": traceparent})
	sc := trace.SpanContextFromContext(ctx)
	require.True(t, sc.IsValid(), "invalid traceparent %q", traceparent)
	return sc
}

// TestMCPHTTPTracing drives tool calls through the MCP HTTP handler as run()
// wires it and checks the resulting spans: one server span per request, a child
// of the incoming traceparent, and one "tools/call <tool>" span under it.
func TestMCPHTTPTracing(t *testing.T) {
	exporter := tracetest.NewInMemoryExporter()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
	prevProvider, prevPropagator := otel.GetTracerProvider(), otel.GetTextMapPropagator()
	otel.SetTracerProvider(provider)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{}, propagation.Baggage{}))
	t.Cleanup(func() {
		otel.SetTracerProvider(prevProvider)
		otel.SetTextMapPropagator(prevPropagator)
		_ = provider.Shutdown(context.Background())
	})

	srv := sdkmcp.NewServer(&sdkmcp.Implementation{Name: "tracing", Version: "test"}, nil)
	srv.AddReceivingMiddleware(mcpserver.ToolMiddleware())
	mcpserver.AddTool(srv, "test", &sdkmcp.Tool{Name: "tracing_ok", Description: "succeeds"},
		func(context.Context, *sdkmcp.CallToolRequest, tracingTestInput) (*sdkmcp.CallToolResult, mcpserver.TextOutput, error) {
			return mcpserver.TextResult("ok")
		})
	mcpserver.AddTool(srv, "test", &sdkmcp.Tool{Name: "tracing_tool_error", Description: "returns an error result"},
		func(context.Context, *sdkmcp.CallToolRequest, tracingTestInput) (*sdkmcp.CallToolResult, mcpserver.TextOutput, error) {
			return mcpserver.TextError("resource not found")
		})
	mcpserver.AddTool(srv, "test", &sdkmcp.Tool{Name: "tracing_go_error", Description: "returns a Go error"},
		func(context.Context, *sdkmcp.CallToolRequest, tracingTestInput) (*sdkmcp.CallToolResult, mcpserver.TextOutput, error) {
			return nil, mcpserver.TextOutput{}, errors.New("command failed")
		})

	httpServer := httptest.NewServer(newMCPHTTPHandler(srv, time.Hour))
	t.Cleanup(httpServer.Close)

	rt := &traceparentTransport{traceparent: initializeTraceparent}
	client := sdkmcp.NewClient(&sdkmcp.Implementation{Name: "tracing-client", Version: "test"}, nil)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	session, err := client.Connect(ctx, &sdkmcp.StreamableClientTransport{
		Endpoint:   httpServer.URL,
		HTTPClient: &http.Client{Transport: rt},
	}, nil)
	require.NoError(t, err)

	calls := []struct {
		tool        string
		traceparent string
		wantStatus  codes.Code
	}{
		{"tracing_ok", okCallTraceparent, codes.Ok},
		{"tracing_tool_error", toolErrTraceparent, codes.Error},
		{"tracing_go_error", goErrTraceparent, codes.Error},
	}
	for _, c := range calls {
		rt.set(c.traceparent)
		res, err := session.CallTool(ctx, &sdkmcp.CallToolParams{
			Name:      c.tool,
			Arguments: map[string]string{"value": tracingTestArgument},
		})
		require.NoError(t, err, c.tool)
		require.Equal(t, c.wantStatus == codes.Error, res.IsError, c.tool)
	}

	rt.set(closeTraceparent)
	require.NoError(t, session.Close())
	// Close waits for in-flight requests, so every server span has ended.
	httpServer.Close()

	spans := exporter.GetSpans()
	for _, c := range calls {
		t.Run(c.tool, func(t *testing.T) {
			incoming := mustSpanContext(t, c.traceparent)

			var serverSpans []tracetest.SpanStub
			for _, s := range spans {
				if s.SpanKind == trace.SpanKindServer && s.Parent.SpanID() == incoming.SpanID() {
					serverSpans = append(serverSpans, s)
				}
			}
			require.Len(t, serverSpans, 1, "want one server span for the tools/call request")
			server := serverSpans[0]
			assert.Equal(t, incoming.TraceID(), server.SpanContext.TraceID())
			assert.True(t, server.Parent.IsRemote(), "server span parent should be the remote caller")

			var toolSpans []tracetest.SpanStub
			for _, s := range spans {
				if s.Name == "tools/call "+c.tool {
					toolSpans = append(toolSpans, s)
				}
			}
			require.Len(t, toolSpans, 1, "want one tool-call span")
			tool := toolSpans[0]
			assert.Equal(t, incoming.TraceID(), tool.SpanContext.TraceID(),
				"tool span must join the tools/call request's trace, not the session's first one")
			assert.Equal(t, server.SpanContext.SpanID(), tool.Parent.SpanID(),
				"tool span must be a child of the server span")
			assert.Equal(t, c.wantStatus, tool.Status.Code)

			attrs := map[string]string{}
			for _, kv := range tool.Attributes {
				attrs[string(kv.Key)] = kv.Value.Emit()
			}
			assert.Equal(t, "tools/call", attrs["mcp.method.name"])
			assert.Equal(t, c.tool, attrs["gen_ai.tool.name"])
		})
	}

	// No span may carry request headers, credentials, or tool arguments.
	for _, s := range spans {
		for _, kv := range s.Attributes {
			key := strings.ToLower(string(kv.Key))
			for _, banned := range []string{"header", "authorization", "token"} {
				assert.NotContains(t, key, banned, "span %q attribute %q", s.Name, kv.Key)
			}
			value := kv.Value.Emit()
			assert.NotContains(t, value, "tracing-test-credential", "span %q attribute %q", s.Name, kv.Key)
			assert.NotContains(t, value, tracingTestArgument, "span %q attribute %q", s.Name, kv.Key)
		}
	}
}
