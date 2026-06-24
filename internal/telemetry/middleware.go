package telemetry

import (
	"context"
	"net/http"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

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
