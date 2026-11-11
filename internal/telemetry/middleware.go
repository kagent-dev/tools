package telemetry

import (
	"context"
	"net/http"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
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

// HTTPMiddleware wraps the MCP HTTP handler with a server span per request and
// propagates the request's trace context to the tool-call spans.
//
// The server span is started by otelhttp as a child of the incoming W3C
// traceparent. The standalone SSE stream (GET) is not traced: it stays open for
// the whole session, so its span would last for hours and parent nothing.
//
// The MCP SDK does not run tool handlers on the HTTP request's context: a
// stateful session's handlers run on the context of the request that opened the
// session (initialize). What does reach the handlers, per request, is the
// request's header (CallToolRequest.Extra.Header). So the middleware writes the
// server span's context into the traceparent header of a copy of the request,
// and the tool-call span (see internal/mcp.ToolMiddleware) takes its parent from
// there. No header value is recorded on any span.
//
// It also stashes a few request headers plus the trace and span IDs in the
// request context. The migrated tool handlers no longer read these context
// values: the official SDK delivers headers on the request itself (see
// mcp.Header, which reads req.Extra.Header), so ExtractHTTPHeaders and
// ExtractTraceInfo currently have no non-test callers. They are kept because the
// values remain useful for debugging and for callers that still pass the
// request context through.
func HTTPMiddleware(next http.Handler) http.Handler {
	return otelhttp.NewHandler(contextMiddleware(next), "mcp",
		otelhttp.WithFilter(func(r *http.Request) bool {
			return r.Method != http.MethodGet
		}),
	)
}

// contextMiddleware propagates the active trace context to the request header
// and the request context. See HTTPMiddleware.
func contextMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		propagator := otel.GetTextMapPropagator()

		// otelhttp has already extracted the incoming trace context and started
		// the server span. Extract here only for requests it did not trace, and
		// never on top of an active span, which would replace the server span
		// with its remote parent.
		if !trace.SpanContextFromContext(ctx).IsValid() {
			ctx = propagator.Extract(ctx, propagation.HeaderCarrier(r.Header))
		}

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

		// Clone deep-copies the header, so the caller's request is not modified.
		r = r.Clone(ctx)
		if span.SpanContext().IsValid() {
			propagator.Inject(ctx, propagation.HeaderCarrier(r.Header))
		}

		// Call next handler with enhanced context
		next.ServeHTTP(w, r)
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
