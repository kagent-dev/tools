package telemetry

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

type ToolHandler func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error)

func WithTracing(toolName string, handler ToolHandler) ToolHandler {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		tracer := otel.Tracer("kagent-tools/mcp")

		spanName := fmt.Sprintf("mcp.tool.%s", toolName)
		ctx, span := tracer.Start(ctx, spanName)
		defer span.End()

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
