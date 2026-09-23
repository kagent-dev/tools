// Package mcp adapts the modelcontextprotocol/go-sdk server to the kagent-tools
// providers. It supplies the typed-output helpers (TextOutput/TextResult/
// TextError/TextOf) and the instrumented AddTool registration path used by every
// provider, and centralizes tracing/metrics instrumentation as a single
// receiving middleware.
//
// The SDK's own types are used directly — providers import
// github.com/modelcontextprotocol/go-sdk/mcp as sdkmcp rather than going through
// aliases here. Only what carries repository-specific behaviour is defined or
// wrapped in this package.
package mcp

import (
	"context"
	"net/http"
	"reflect"
	"strings"
	"sync"
	"time"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/kagent-dev/tools/internal/metrics"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
)

// NewToolResultText returns a successful text result.
func NewToolResultText(text string) *sdk.CallToolResult {
	return &sdk.CallToolResult{Content: []sdk.Content{&sdk.TextContent{Text: text}}}
}

// NewToolResultError returns a tool-level error result (IsError=true). Handlers
// return this together with a nil Go error, per MCP convention.
func NewToolResultError(message string) *sdk.CallToolResult {
	return &sdk.CallToolResult{Content: []sdk.Content{&sdk.TextContent{Text: message}}, IsError: true}
}

// Header returns the HTTP headers carried with the request, or nil for stdio /
// in-process calls. Used for bearer-token passthrough.
func Header(req *sdk.CallToolRequest) http.Header {
	if req != nil && req.Extra != nil {
		return req.Extra.Header
	}
	return nil
}

// TextOutput is the typed output for tools whose result is raw CLI text. It is
// the shared wrapper described by the repository's typed-I/O convention: rather
// than registering a handler with Out=any, a text tool returns a concrete
// TextOutput so the SDK can infer an output schema and populate
// CallToolResult.StructuredContent.
type TextOutput struct {
	Output string `json:"output"`
}

// TextResult is the typed equivalent of NewToolResultText for a handler whose
// Out type is TextOutput. The human-readable text stays in Content, so existing
// clients (and pre-SEP-2106 clients that only read Content) are unaffected,
// while StructuredContent carries the same value as a typed object.
func TextResult(text string) (*sdk.CallToolResult, TextOutput, error) {
	return NewToolResultText(text), TextOutput{Output: text}, nil
}

// TextError is the typed equivalent of NewToolResultError for a handler whose
// Out type is TextOutput. It returns an empty TextOutput so the zero value still
// satisfies the inferred output schema on the error path.
func TextError(message string) (*sdk.CallToolResult, TextOutput, error) {
	return NewToolResultError(message), TextOutput{}, nil
}

// TextOf extracts the concatenated text content of a result as a TextOutput, so
// handlers that build a *CallToolResult in a helper can still return a typed
// output value. A nil result yields an empty TextOutput.
func TextOf(res *sdk.CallToolResult) TextOutput {
	return TextOutput{Output: toolResultText(res)}
}

// toolResultText returns the concatenated text content of a result, used to
// report a tool-level failure on a span. A nil result yields "".
func toolResultText(res *sdk.CallToolResult) string {
	if res == nil {
		return ""
	}
	var b strings.Builder
	for _, content := range res.Content {
		if textContent, ok := content.(*sdk.TextContent); ok {
			b.WriteString(textContent.Text)
		}
	}
	return b.String()
}

// providerByTool maps a registered tool name to its provider for metric labels.
var providerByTool sync.Map

// AddTool registers a typed tool and records its provider for metrics. The input
// schema is inferred from In's json/jsonschema struct tags by the SDK, then
// relaxed by relaxInputSchema to preserve the pre-migration tool-calling contract.
func AddTool[In, Out any](s *sdk.Server, provider string, t *sdk.Tool, h sdk.ToolHandlerFor[In, Out]) {
	providerByTool.Store(t.Name, provider)
	metrics.KagentToolsMCPRegisteredTools.WithLabelValues(t.Name, provider).Set(1)
	relaxInputSchema[In](t)
	sdk.AddTool(s, t, h)
}

// relaxInputSchema restores the input-validation contract the providers were
// written against. The previous mark3labs API made every argument optional
// unless explicitly marked Required() and ignored unknown arguments. The go-sdk
// instead infers every non-omitempty struct field as required and sets
// additionalProperties:false, so a client that omits an optional field (or sends
// an extra one) is rejected before the handler runs. That silently broke tools
// such as k8s_get_resources, where only resource_type is truly required.
//
// We re-infer the schema, drop the required list, and allow additional
// properties. Handlers continue to validate their own mandatory inputs and
// return a tool error when one is missing, so correctness is unchanged.
func relaxInputSchema[In any](t *sdk.Tool) {
	if t.InputSchema != nil {
		return // caller supplied an explicit schema; respect it
	}
	if reflect.TypeFor[In]() == reflect.TypeFor[any]() {
		return // SDK has dedicated handling for an "any" input
	}
	schema, err := jsonschema.For[In](nil)
	if err != nil || schema.Type != "object" {
		return // fall back to the SDK's own inference
	}
	schema.Required = nil
	schema.AdditionalProperties = nil
	t.InputSchema = schema
}

func providerOf(tool string) string {
	if v, ok := providerByTool.Load(tool); ok {
		return v.(string)
	}
	return ""
}

// ToolMiddleware instruments every tools/call with an OTel span and Prometheus
// invocation counters. Register once via server.AddReceivingMiddleware.
func ToolMiddleware() sdk.Middleware {
	return func(next sdk.MethodHandler) sdk.MethodHandler {
		return func(ctx context.Context, method string, req sdk.Request) (sdk.Result, error) {
			if method != "tools/call" {
				return next(ctx, method, req)
			}

			toolName := ""
			if ctr, ok := req.(*sdk.CallToolRequest); ok && ctr.Params != nil {
				toolName = ctr.Params.Name
			}
			provider := providerOf(toolName)

			tracer := otel.Tracer("kagent-tools/mcp")
			ctx, span := tracer.Start(ctx, "mcp.tool."+toolName)
			defer span.End()
			span.SetAttributes(
				attribute.String("mcp.tool.name", toolName),
				attribute.String("mcp.tool.provider", provider),
			)

			metrics.KagentToolsMCPInvocationsTotal.WithLabelValues(toolName, provider).Inc()
			start := time.Now()

			res, err := next(ctx, method, req)

			span.SetAttributes(attribute.Float64("mcp.tool.duration_seconds", time.Since(start).Seconds()))

			failed := err != nil
			var toolErrMessage string
			if ctres, ok := res.(*sdk.CallToolResult); ok && ctres != nil && ctres.IsError {
				failed = true
				toolErrMessage = toolResultText(ctres)
			}
			if failed {
				metrics.KagentToolsMCPInvocationsFailureTotal.WithLabelValues(toolName, provider).Inc()
				span.SetAttributes(attribute.Bool("mcp.tool.is_error", true))
				// Tool-level failures (IsError=true) arrive with a nil Go error.
				// They must still mark the span, otherwise the failure counter
				// and the traces disagree and the span stays neither Ok nor
				// Error.
				switch {
				case err != nil:
					span.RecordError(err)
					span.SetStatus(codes.Error, err.Error())
				case toolErrMessage != "":
					span.SetStatus(codes.Error, toolErrMessage)
				default:
					span.SetStatus(codes.Error, "tool returned IsError")
				}
			} else {
				span.SetStatus(codes.Ok, "ok")
			}
			return res, err
		}
	}
}
