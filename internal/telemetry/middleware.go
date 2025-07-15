package telemetry

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

type ToolHandler func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error)

// contextKey is used for storing HTTP context in the request context
type contextKey string

const (
	HTTPHeadersKey contextKey = "http_headers"
	TraceIDKey     contextKey = "trace_id"
	SpanIDKey      contextKey = "span_id"
)

// HTTPMiddleware wraps an HTTP handler to extract headers and propagate context
func HTTPMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		// Extract OpenTelemetry context from HTTP headers
		propagator := otel.GetTextMapPropagator()
		ctx = propagator.Extract(ctx, propagation.HeaderCarrier(r.Header))

		// Store relevant HTTP headers in context for tool handlers
		headers := make(map[string]string)
		for name, values := range r.Header {
			if len(values) > 0 {
				// Store important headers for debugging/tracing
				switch name {
				case "X-Request-ID", "X-Correlation-ID", "X-Trace-ID",
					"User-Agent", "Authorization", "X-Forwarded-For":
					headers[name] = values[0]
				}
			}
		}

		// Add headers to context
		ctx = context.WithValue(ctx, HTTPHeadersKey, headers)

		// Extract trace information if available
		span := trace.SpanFromContext(ctx)
		if span.SpanContext().HasTraceID() {
			ctx = context.WithValue(ctx, TraceIDKey, span.SpanContext().TraceID().String())
			ctx = context.WithValue(ctx, SpanIDKey, span.SpanContext().SpanID().String())
		}

		// Call next handler with enhanced context
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// ExtractHTTPHeaders retrieves HTTP headers from context
func ExtractHTTPHeaders(ctx context.Context) map[string]string {
	if headers, ok := ctx.Value(HTTPHeadersKey).(map[string]string); ok {
		return headers
	}
	return make(map[string]string)
}

// ExtractTraceInfo retrieves trace information from context
func ExtractTraceInfo(ctx context.Context) (traceID, spanID string) {
	if tid, ok := ctx.Value(TraceIDKey).(string); ok {
		traceID = tid
	}
	if sid, ok := ctx.Value(SpanIDKey).(string); ok {
		spanID = sid
	}
	return traceID, spanID
}

func WithTracing(toolName string, handler ToolHandler) ToolHandler {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		tracer := otel.Tracer("kagent-tools/mcp")

		spanName := fmt.Sprintf("mcp.tool.%s", toolName)
		ctx, span := tracer.Start(ctx, spanName)
		defer span.End()

		// Extract HTTP headers from context and add as span attributes
		headers := ExtractHTTPHeaders(ctx)
		for key, value := range headers {
			span.SetAttributes(attribute.String(fmt.Sprintf("http.header.%s", key), value))
		}

		// Extract parent trace information
		parentTraceID, parentSpanID := ExtractTraceInfo(ctx)
		if parentTraceID != "" {
			span.SetAttributes(
				attribute.String("http.parent_trace_id", parentTraceID),
				attribute.String("http.parent_span_id", parentSpanID),
			)
		}

		span.SetAttributes(
			attribute.String("mcp.tool.name", toolName),
			attribute.String("mcp.request.id", request.Params.Name),
		)

		if request.Params.Arguments != nil {
			if argsJSON, err := json.Marshal(request.Params.Arguments); err == nil {
				span.SetAttributes(attribute.String("mcp.request.arguments", string(argsJSON)))
			}
		}

		span.AddEvent("tool.execution.start")
		startTime := time.Now()

		result, err := handler(ctx, request)

		duration := time.Since(startTime)
		span.SetAttributes(attribute.Float64("mcp.tool.duration_seconds", duration.Seconds()))

		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
			span.AddEvent("tool.execution.error", trace.WithAttributes(
				attribute.String("error.message", err.Error()),
			))
		} else {
			span.SetStatus(codes.Ok, "tool execution completed successfully")
			span.AddEvent("tool.execution.success")

			if result != nil {
				span.SetAttributes(attribute.Bool("mcp.result.is_error", result.IsError))
				if result.Content != nil {
					span.SetAttributes(attribute.Int("mcp.result.content_count", len(result.Content)))
				}
			}
		}

		return result, err
	}
}

func StartSpan(ctx context.Context, operationName string, attrs ...attribute.KeyValue) (context.Context, trace.Span) {
	tracer := otel.Tracer("kagent-tools")
	ctx, span := tracer.Start(ctx, operationName)

	if len(attrs) > 0 {
		span.SetAttributes(attrs...)
	}

	return ctx, span
}

func RecordError(span trace.Span, err error, message string) {
	span.RecordError(err)
	span.SetStatus(codes.Error, message)
}

func RecordSuccess(span trace.Span, message string) {
	span.SetStatus(codes.Ok, message)
}

func AddEvent(span trace.Span, name string, attrs ...attribute.KeyValue) {
	span.AddEvent(name, trace.WithAttributes(attrs...))
}

// AdaptToolHandler adapts a telemetry.ToolHandler to a server.ToolHandlerFunc.
func AdaptToolHandler(th ToolHandler) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return th(ctx, req)
	}
}
