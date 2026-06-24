package mcp

import (
	"context"
	"fmt"
	"net/http"
	"testing"

	"github.com/kagent-dev/tools/internal/metrics"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
	promtest "github.com/prometheus/client_golang/prometheus/testutil"
)

// invokeMiddleware runs ToolMiddleware around next for a tools/call to toolName
// (registered to provider) and returns the result/error.
func invokeMiddleware(toolName, provider string, next sdk.MethodHandler) (sdk.Result, error) {
	metrics.KagentToolsMCPInvocationsTotal.Reset()
	metrics.KagentToolsMCPInvocationsFailureTotal.Reset()
	providerByTool.Store(toolName, provider)

	h := ToolMiddleware()(next)
	req := &sdk.CallToolRequest{Params: &sdk.CallToolParamsRaw{Name: toolName}}
	return h(context.Background(), "tools/call", req)
}

func TestHeader(t *testing.T) {
	assert := func(cond bool, msg string) {
		if !cond {
			t.Fatal(msg)
		}
	}

	// nil request and request without Extra yield no headers.
	assert(Header(nil) == nil, "nil request should give nil header")
	assert(Header(&sdk.CallToolRequest{}) == nil, "request without Extra should give nil header")

	h := http.Header{"Authorization": []string{"Bearer t"}}
	req := &sdk.CallToolRequest{Extra: &sdk.RequestExtra{Header: h}}
	if got := Header(req).Get("Authorization"); got != "Bearer t" {
		t.Fatalf("expected passthrough header, got %q", got)
	}
}

func TestAddToolRecordsProvider(t *testing.T) {
	metrics.KagentToolsMCPRegisteredTools.Reset()
	s := NewServer(&Implementation{Name: "t", Version: "v"}, nil)

	type in struct {
		Name string `json:"name"`
	}
	AddTool(s, "myprovider", &Tool{Name: "my_tool"}, func(_ context.Context, _ *CallToolRequest, _ in) (*CallToolResult, any, error) {
		return NewToolResultText("ok"), nil, nil
	})

	if got := providerOf("my_tool"); got != "myprovider" {
		t.Errorf("providerOf: expected myprovider, got %q", got)
	}
	if got := providerOf("unknown_tool"); got != "" {
		t.Errorf("providerOf unknown: expected empty, got %q", got)
	}
	if v := promtest.ToFloat64(metrics.KagentToolsMCPRegisteredTools.WithLabelValues("my_tool", "myprovider")); v != 1 {
		t.Errorf("registered_tools metric: expected 1, got %v", v)
	}
}

// TestToolMiddleware_IsErrorIncrementsFailureCounter is the regression test for
// the bug identified in PR review: handlers signal tool-level failures via
// NewToolResultError(...) (IsError=true, Go error=nil), so checking only
// `err != nil` would never count these as failures.
func TestToolMiddleware_IsErrorIncrementsFailureCounter(t *testing.T) {
	result, err := invokeMiddleware("failing_tool", "test",
		func(_ context.Context, _ string, _ sdk.Request) (sdk.Result, error) {
			return NewToolResultError("kubectl: resource not found"), nil
		},
	)
	if err != nil {
		t.Fatalf("expected nil Go error, got: %v", err)
	}
	if ctr, ok := result.(*sdk.CallToolResult); !ok || !ctr.IsError {
		t.Fatal("expected result.IsError=true")
	}

	total := promtest.ToFloat64(metrics.KagentToolsMCPInvocationsTotal.WithLabelValues("failing_tool", "test"))
	if total != 1 {
		t.Errorf("invocations_total: expected 1, got %v", total)
	}
	failures := promtest.ToFloat64(metrics.KagentToolsMCPInvocationsFailureTotal.WithLabelValues("failing_tool", "test"))
	if failures != 1 {
		t.Errorf("invocations_failure_total: expected 1, got %v (IsError=true was not counted)", failures)
	}
}

// TestToolMiddleware_SuccessDoesNotIncrementFailureCounter verifies a successful
// call leaves the failure counter untouched.
func TestToolMiddleware_SuccessDoesNotIncrementFailureCounter(t *testing.T) {
	_, err := invokeMiddleware("success_tool", "test",
		func(_ context.Context, _ string, _ sdk.Request) (sdk.Result, error) {
			return NewToolResultText("all good"), nil
		},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	total := promtest.ToFloat64(metrics.KagentToolsMCPInvocationsTotal.WithLabelValues("success_tool", "test"))
	if total != 1 {
		t.Errorf("invocations_total: expected 1, got %v", total)
	}
	failures := promtest.ToFloat64(metrics.KagentToolsMCPInvocationsFailureTotal.WithLabelValues("success_tool", "test"))
	if failures != 0 {
		t.Errorf("invocations_failure_total: expected 0, got %v", failures)
	}
}

// TestToolMiddleware_GoErrorIncrementsFailureCounter verifies a real Go error is
// counted as a failure.
func TestToolMiddleware_GoErrorIncrementsFailureCounter(t *testing.T) {
	_, err := invokeMiddleware("broken_tool", "test",
		func(_ context.Context, _ string, _ sdk.Request) (sdk.Result, error) {
			return nil, fmt.Errorf("connection refused")
		},
	)
	if err == nil {
		t.Fatal("expected a Go error, got nil")
	}

	failures := promtest.ToFloat64(metrics.KagentToolsMCPInvocationsFailureTotal.WithLabelValues("broken_tool", "test"))
	if failures != 1 {
		t.Errorf("invocations_failure_total: expected 1, got %v", failures)
	}
}
