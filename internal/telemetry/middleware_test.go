package telemetry

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/sdk/trace"
)

func TestHTTPMiddleware(t *testing.T) {
	provider, _ := setupTracing()
	defer func() { _ = provider.Shutdown(context.Background()) }()

	var gotHeaders map[string]string
	var gotTraceID, gotSpanID string
	next := http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		gotHeaders = ExtractHTTPHeaders(r.Context())
		gotTraceID, gotSpanID = ExtractTraceInfo(r.Context())
	})

	req := httptest.NewRequest(http.MethodGet, "/mcp", nil)
	req.Header.Set("Authorization", "Bearer abc")
	req.Header.Set("User-Agent", "agent/1.0")
	req.Header.Set("X-Ignored", "nope")

	HTTPMiddleware(next).ServeHTTP(httptest.NewRecorder(), req)

	require.NotNil(t, gotHeaders)
	assert.Equal(t, "Bearer abc", gotHeaders["Authorization"])
	assert.Equal(t, "agent/1.0", gotHeaders["User-Agent"])
	assert.NotContains(t, gotHeaders, "X-Ignored")
	// No inbound trace context here, so trace/span IDs stay empty.
	assert.Empty(t, gotTraceID)
	assert.Empty(t, gotSpanID)
}

func TestExtractHelpersDefaults(t *testing.T) {
	assert.Empty(t, ExtractHTTPHeaders(context.Background()))
	tid, sid := ExtractTraceInfo(context.Background())
	assert.Empty(t, tid)
	assert.Empty(t, sid)
}

// InMemoryExporter is a simple in-memory exporter for testing
type InMemoryExporter struct {
	spans []trace.ReadOnlySpan
}

func (e *InMemoryExporter) ExportSpans(ctx context.Context, spans []trace.ReadOnlySpan) error {
	e.spans = append(e.spans, spans...)
	return nil
}

func (e *InMemoryExporter) Shutdown(ctx context.Context) error {
	return nil
}

func (e *InMemoryExporter) GetSpans() []trace.ReadOnlySpan {
	return e.spans
}

// setupTracing initializes OpenTelemetry with in-memory exporter for testing
func setupTracing() (*trace.TracerProvider, *InMemoryExporter) {
	exporter := &InMemoryExporter{}
	provider := trace.NewTracerProvider(
		trace.WithSampler(trace.AlwaysSample()),
		trace.WithSpanProcessor(trace.NewSimpleSpanProcessor(exporter)),
	)
	otel.SetTracerProvider(provider)
	return provider, exporter
}

func TestStartSpan(t *testing.T) {
	provider, exporter := setupTracing()
	defer func() {
		if err := provider.Shutdown(context.Background()); err != nil {
			t.Errorf("Failed to shutdown provider: %v", err)
		}
	}()

	_, span := StartSpan(context.Background(), "test-span",
		attribute.String("key1", "value1"),
		attribute.Int("key2", 42),
	)
	span.End()

	if err := provider.ForceFlush(context.Background()); err != nil {
		t.Errorf("Failed to flush provider: %v", err)
	}

	spans := exporter.GetSpans()
	assert.Len(t, spans, 1)
	assert.Equal(t, "test-span", spans[0].Name())
}

func TestStartSpanNoAttributes(t *testing.T) {
	provider, exporter := setupTracing()
	defer func() {
		if err := provider.Shutdown(context.Background()); err != nil {
			t.Errorf("Failed to shutdown provider: %v", err)
		}
	}()

	_, span := StartSpan(context.Background(), "test-span")
	span.End()

	if err := provider.ForceFlush(context.Background()); err != nil {
		t.Errorf("Failed to flush provider: %v", err)
	}

	spans := exporter.GetSpans()
	assert.Len(t, spans, 1)
	assert.Equal(t, "test-span", spans[0].Name())
}

func TestRecordError(t *testing.T) {
	provider, exporter := setupTracing()
	defer func() {
		if err := provider.Shutdown(context.Background()); err != nil {
			t.Errorf("Failed to shutdown provider: %v", err)
		}
	}()

	_, span := StartSpan(context.Background(), "test-span")
	RecordError(span, errors.New("test error"), "test error")
	span.End()

	if err := provider.ForceFlush(context.Background()); err != nil {
		t.Errorf("Failed to flush provider: %v", err)
	}

	spans := exporter.GetSpans()
	assert.Len(t, spans, 1)
	assert.Equal(t, "test-span", spans[0].Name())
	assert.Equal(t, codes.Error, spans[0].Status().Code)
	assert.Equal(t, "test error", spans[0].Status().Description)
}

func TestRecordSuccess(t *testing.T) {
	provider, exporter := setupTracing()
	defer func() {
		if err := provider.Shutdown(context.Background()); err != nil {
			t.Errorf("Failed to shutdown provider: %v", err)
		}
	}()

	_, span := StartSpan(context.Background(), "test-span")
	RecordSuccess(span, "operation completed successfully")
	span.End()

	if err := provider.ForceFlush(context.Background()); err != nil {
		t.Errorf("Failed to flush provider: %v", err)
	}

	spans := exporter.GetSpans()
	assert.Len(t, spans, 1)
	assert.Equal(t, "test-span", spans[0].Name())
	assert.Equal(t, codes.Ok, spans[0].Status().Code)
}

func TestAddEvent(t *testing.T) {
	provider, exporter := setupTracing()
	defer func() {
		if err := provider.Shutdown(context.Background()); err != nil {
			t.Errorf("Failed to shutdown provider: %v", err)
		}
	}()

	_, span := StartSpan(context.Background(), "test-span")
	AddEvent(span, "test-event",
		attribute.String("event_key", "event_value"),
		attribute.Int("event_num", 123),
	)
	span.End()

	if err := provider.ForceFlush(context.Background()); err != nil {
		t.Errorf("Failed to flush provider: %v", err)
	}

	spans := exporter.GetSpans()
	assert.Len(t, spans, 1)
	events := spans[0].Events()
	assert.Len(t, events, 1)
	assert.Equal(t, "test-event", events[0].Name)
}

func TestAddEventNoAttributes(t *testing.T) {
	provider, exporter := setupTracing()
	defer func() {
		if err := provider.Shutdown(context.Background()); err != nil {
			t.Errorf("Failed to shutdown provider: %v", err)
		}
	}()

	_, span := StartSpan(context.Background(), "test-span")
	AddEvent(span, "test-event")
	span.End()

	if err := provider.ForceFlush(context.Background()); err != nil {
		t.Errorf("Failed to flush provider: %v", err)
	}

	spans := exporter.GetSpans()
	assert.Len(t, spans, 1)
	events := spans[0].Events()
	assert.Len(t, events, 1)
	assert.Equal(t, "test-event", events[0].Name)
}
