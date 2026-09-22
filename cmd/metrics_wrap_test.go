package main

import (
	"context"
	"fmt"
	"testing"

	"github.com/kagent-dev/tools/internal/metrics"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	promtest "github.com/prometheus/client_golang/prometheus/testutil"
)

// newTestServer creates a fresh MCP server and resets the metric counters so
// tests do not interfere with each other.
func newTestServer() *server.MCPServer {
	metrics.KagentToolsMCPInvocationsTotal.Reset()
	metrics.KagentToolsMCPInvocationsFailureTotal.Reset()
	return server.NewMCPServer("test-server", "test")
}

// invokeWrapped registers handler on s, wraps all handlers with metrics, then
// calls the wrapped handler for toolName and returns its result.
func invokeWrapped(t *testing.T, s *server.MCPServer, toolName string, provider string, handler server.ToolHandlerFunc) (*mcp.CallToolResult, error) {
	t.Helper()
	s.AddTool(mcp.Tool{Name: toolName}, handler)
	wrapToolHandlersWithMetrics(s, map[string]string{toolName: provider})
	st, ok := s.ListTools()[toolName]
	if !ok {
		t.Fatalf("tool %q not found after wrapping", toolName)
	}
	return st.Handler(context.Background(), mcp.CallToolRequest{})
}

// TestWrapToolHandlersWithMetrics_IsErrorIncrementsFailureCounter is the
// critical regression test for the bug identified in PR review:
//
//	Handlers signal tool-level failures via NewToolResultError(...), nil
//	(result.IsError=true, Go error=nil), so checking only `err != nil` would
//	never count these as failures.
//
// To replicate manually:
//
//	go test -v -run TestWrapToolHandlersWithMetrics_IsErrorIncrementsFailureCounter ./cmd/
func TestWrapToolHandlersWithMetrics_IsErrorIncrementsFailureCounter(t *testing.T) {
	s := newTestServer()

	result, err := invokeWrapped(t, s, "failing_tool", "test",
		func(_ context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			// This is the pattern used 214 times across pkg/ - returns a tool-level
			// error with IsError=true but a nil Go error.
			return mcp.NewToolResultError("kubectl: resource not found"), nil
		},
	)

	if err != nil {
		t.Fatalf("expected nil Go error from handler, got: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected result.IsError=true")
	}

	total := promtest.ToFloat64(metrics.KagentToolsMCPInvocationsTotal.WithLabelValues("failing_tool", "test"))
	if total != 1 {
		t.Errorf("invocations_total: expected 1, got %v", total)
	}

	failures := promtest.ToFloat64(metrics.KagentToolsMCPInvocationsFailureTotal.WithLabelValues("failing_tool", "test"))
	if failures != 1 {
		t.Errorf("invocations_failure_total: expected 1, got %v (IsError=true was not counted as failure)", failures)
	}
}

// TestWrapToolHandlersWithMetrics_SuccessDoesNotIncrementFailureCounter verifies
// that a successful tool call does not touch the failure counter.
//
// To replicate manually:
//
//	go test -v -run TestWrapToolHandlersWithMetrics_SuccessDoesNotIncrementFailureCounter ./cmd/
func TestWrapToolHandlersWithMetrics_SuccessDoesNotIncrementFailureCounter(t *testing.T) {
	s := newTestServer()

	_, err := invokeWrapped(t, s, "success_tool", "test",
		func(_ context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return mcp.NewToolResultText("all good"), nil
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
		t.Errorf("invocations_failure_total: expected 0 for a successful call, got %v", failures)
	}
}

// TestWrapToolHandlersWithMetrics_GoErrorIncrementsFailureCounter verifies
// that a real Go error (e.g. infrastructure failure) is also counted.
//
// To replicate manually:
//
//	go test -v -run TestWrapToolHandlersWithMetrics_GoErrorIncrementsFailureCounter ./cmd/
func TestWrapToolHandlersWithMetrics_GoErrorIncrementsFailureCounter(t *testing.T) {
	s := newTestServer()

	_, err := invokeWrapped(t, s, "broken_tool", "test",
		func(_ context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return nil, fmt.Errorf("connection refused")
		},
	)

	if err == nil {
		t.Fatal("expected a Go error, got nil")
	}

	failures := promtest.ToFloat64(metrics.KagentToolsMCPInvocationsFailureTotal.WithLabelValues("broken_tool", "test"))
	if failures != 1 {
		t.Errorf("invocations_failure_total: expected 1 for Go error, got %v", failures)
	}
}
