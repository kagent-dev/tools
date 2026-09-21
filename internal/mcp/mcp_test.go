package mcp

import (
	"context"
	"fmt"
	"net/http"
	"testing"

	"github.com/google/jsonschema-go/jsonschema"
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
	AddTool(s, "myprovider", &Tool{Name: "my_tool"}, func(_ context.Context, _ *CallToolRequest, _ in) (*CallToolResult, TextOutput, error) {
		return TextResult("ok")
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

// TestAddToolRelaxesInputSchema is the regression test for the go-sdk migration
// bug where every non-omitempty input field became required and extra fields
// were rejected (additionalProperties:false). Pre-migration only explicitly
// marked fields were required and unknown fields were ignored. A client must be
// able to call a tool sending only the fields it cares about (e.g.
// k8s_get_resources with just resource_type), so the inferred Required list and
// additionalProperties restriction must be cleared.
func TestAddToolRelaxesInputSchema(t *testing.T) {
	s := NewServer(&Implementation{Name: "t", Version: "v"}, nil)

	type in struct {
		ResourceType  string `json:"resource_type"`
		ResourceName  string `json:"resource_name"`
		Namespace     string `json:"namespace"`
		AllNamespaces bool   `json:"all_namespaces"`
		Output        string `json:"output"`
	}
	tool := &Tool{Name: "relax_tool"}
	AddTool(s, "p", tool, func(_ context.Context, _ *CallToolRequest, _ in) (*CallToolResult, TextOutput, error) {
		return TextResult("ok")
	})

	schema, ok := tool.InputSchema.(*jsonschema.Schema)
	if !ok {
		t.Fatalf("InputSchema not set to *jsonschema.Schema, got %T", tool.InputSchema)
	}
	if len(schema.Required) != 0 {
		t.Errorf("expected no required fields, got %v", schema.Required)
	}
	if schema.AdditionalProperties != nil {
		t.Errorf("expected additionalProperties unconstrained, got %#v", schema.AdditionalProperties)
	}
	if _, present := schema.Properties["resource_type"]; !present {
		t.Errorf("expected properties to be preserved, got %v", schema.Properties)
	}

	// The relaxed schema must accept a payload that omits optional fields, which
	// is exactly what the e2e client sends and what previously failed. Exercise
	// the real client->server path with a typed, deliberately partial argument
	// value rather than validating a fixture by hand.
	serverT, clientT := sdk.NewInMemoryTransports()
	ctx := context.Background()
	if _, err := s.Connect(ctx, serverT, nil); err != nil {
		t.Fatalf("server connect: %v", err)
	}
	client := sdk.NewClient(&sdk.Implementation{Name: "relax-client", Version: "v"}, nil)
	session, err := client.Connect(ctx, clientT, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	defer func() { _ = session.Close() }()

	// A partial argument value: only resource_type is set, every other field is
	// omitted. Pre-migration this call was rejected for missing required fields.
	type partialArgs struct {
		ResourceType string `json:"resource_type"`
	}
	result, err := session.CallTool(ctx, &sdk.CallToolParams{
		Name:      "relax_tool",
		Arguments: partialArgs{ResourceType: "namespace"},
	})
	if err != nil {
		t.Fatalf("partial payload should be accepted, got: %v", err)
	}
	if result.IsError {
		t.Fatalf("partial payload returned a tool error: %v", result)
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
