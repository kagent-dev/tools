// Package mcp adapts the modelcontextprotocol/go-sdk server to the kagent-tools
// providers. It re-exports the SDK types the providers need, supplies result
// constructors compatible with the previous mark3labs helpers, and centralizes
// tracing/metrics instrumentation as a single receiving middleware so provider
// packages register tools with one typed call and no per-tool wrapping.
package mcp

import (
	"context"
	"net/http"
	"sync"
	"time"

	"github.com/kagent-dev/tools/internal/metrics"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
)

// Re-exported SDK types so provider packages depend on a single import.
type (
	// Server is the MCP server tools are registered on.
	Server = sdk.Server
	// Tool describes a tool's name, description and (inferred) input schema.
	Tool = sdk.Tool
	// CallToolRequest is the server-side request passed to a tool handler.
	CallToolRequest = sdk.CallToolRequest
	// CallToolResult is the result returned by a tool handler.
	CallToolResult = sdk.CallToolResult
	// Implementation identifies the server to clients.
	Implementation = sdk.Implementation
	// Content is a single piece of tool result content.
	Content = sdk.Content
	// TextContent is textual tool result content.
	TextContent = sdk.TextContent
	// RequestExtra carries transport-level extras (e.g. HTTP headers) on a request.
	RequestExtra = sdk.RequestExtra
)

// NewServer constructs a new MCP server.
var NewServer = sdk.NewServer

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

// providerByTool maps a registered tool name to its provider for metric labels.
var providerByTool sync.Map

// AddTool registers a typed tool and records its provider for metrics. The input
// schema is inferred from In's json/jsonschema struct tags by the SDK.
func AddTool[In, Out any](s *sdk.Server, provider string, t *sdk.Tool, h sdk.ToolHandlerFor[In, Out]) {
	providerByTool.Store(t.Name, provider)
	metrics.KagentToolsMCPRegisteredTools.WithLabelValues(t.Name, provider).Set(1)
	sdk.AddTool(s, t, h)
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
			if ctres, ok := res.(*sdk.CallToolResult); ok && ctres != nil && ctres.IsError {
				failed = true
			}
			if failed {
				metrics.KagentToolsMCPInvocationsFailureTotal.WithLabelValues(toolName, provider).Inc()
				if err != nil {
					span.RecordError(err)
					span.SetStatus(codes.Error, err.Error())
				}
			} else {
				span.SetStatus(codes.Ok, "ok")
			}
			return res, err
		}
	}
}
