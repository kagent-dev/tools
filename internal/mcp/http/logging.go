package http

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/kagent-dev/tools/internal/logger"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// RequestLogger handles structured logging for HTTP requests and responses.
type RequestLogger struct {
	correlationID string
	startTime     time.Time
	requestID     string
}

// NewRequestLogger creates a new request logger with correlation ID.
func NewRequestLogger(requestID string) *RequestLogger {
	return &RequestLogger{
		correlationID: requestID,
		startTime:     time.Now(),
		requestID:     requestID,
	}
}

// LogRequest logs the incoming HTTP request.
func (rl *RequestLogger) LogRequest(ctx context.Context, method, path, contentType string, params interface{}) {
	attrs := []slog.Attr{
		slog.String("request_id", rl.requestID),
		slog.String("method", method),
		slog.String("path", path),
		slog.String("content_type", contentType),
	}

	// Add span attributes if in a trace context
	if span := trace.SpanFromContext(ctx); span != nil {
		span.SetAttributes(
			attribute.String("http.method", method),
			attribute.String("http.target", path),
			attribute.String("http.request_id", rl.requestID),
		)
	}

	logger.Get().LogAttrs(ctx, slog.LevelDebug, "HTTP request received", attrs...)
}

// LogResponse logs the outgoing HTTP response.
func (rl *RequestLogger) LogResponse(ctx context.Context, statusCode int, responseSize int64) {
	duration := time.Since(rl.startTime)

	attrs := []slog.Attr{
		slog.String("request_id", rl.requestID),
		slog.Int("status_code", statusCode),
		slog.Int64("response_size", responseSize),
		slog.String("duration", duration.String()),
		slog.Float64("duration_ms", duration.Seconds()*1000),
	}

	// Log appropriate level based on status code
	logLevel := slog.LevelDebug
	if statusCode >= 400 {
		logLevel = slog.LevelWarn
		if statusCode >= 500 {
			logLevel = slog.LevelError
		}
	}

	// Add span attributes if in a trace context
	if span := trace.SpanFromContext(ctx); span != nil {
		span.SetAttributes(
			attribute.Int("http.status_code", statusCode),
			attribute.Int64("http.response_size", responseSize),
		)
	}

	logger.Get().LogAttrs(ctx, logLevel, "HTTP response sent", attrs...)
}

// LogError logs an HTTP error.
func (rl *RequestLogger) LogError(ctx context.Context, statusCode int, errCode, errMessage string) {
	attrs := []slog.Attr{
		slog.String("request_id", rl.requestID),
		slog.Int("status_code", statusCode),
		slog.String("error_code", errCode),
		slog.String("error_message", errMessage),
	}

	// Add span error attributes if in a trace context
	if span := trace.SpanFromContext(ctx); span != nil {
		span.SetAttributes(
			attribute.Int("http.status_code", statusCode),
			attribute.String("error.code", errCode),
			attribute.String("error.message", errMessage),
		)
	}

	logger.Get().LogAttrs(ctx, slog.LevelError, "HTTP error", attrs...)
}

// LogToolExecution logs tool execution details.
func LogToolExecution(ctx context.Context, toolName string, duration time.Duration, success bool, errorMsg string) {
	tracer := otel.Tracer("kagent-tools/http")
	ctx, span := tracer.Start(ctx, fmt.Sprintf("tool.%s.execute", toolName))
	defer span.End()

	span.SetAttributes(
		attribute.String("tool.name", toolName),
		attribute.Float64("execution_time_ms", duration.Seconds()*1000),
		attribute.Bool("success", success),
	)

	attrs := []slog.Attr{
		slog.String("tool_name", toolName),
		slog.Float64("execution_time_ms", duration.Seconds()*1000),
		slog.Bool("success", success),
	}

	if !success && errorMsg != "" {
		attrs = append(attrs, slog.String("error", errorMsg))
	}

	level := slog.LevelDebug
	if !success {
		level = slog.LevelError
	}

	logger.Get().LogAttrs(ctx, level, "Tool execution", attrs...)
}

// EnableDebugLogging enables verbose debug logging for HTTP layer.
func EnableDebugLogging() {
	logger.Get().Debug("HTTP debug logging enabled")
}

// DisableDebugLogging disables verbose debug logging for HTTP layer.
func DisableDebugLogging() {
	logger.Get().Debug("HTTP debug logging disabled")
}

// LogMetrics logs server metrics.
func LogMetrics(ctx context.Context, server *Server) {
	attrs := []slog.Attr{
		slog.Int("port", server.GetPort()),
		slog.String("uptime", server.GetUptime().String()),
		slog.Int("connected_clients", server.GetConnectedClients()),
		slog.Int64("total_requests", server.GetTotalRequests()),
	}

	logger.Get().LogAttrs(ctx, slog.LevelDebug, "Server metrics", attrs...)
}
