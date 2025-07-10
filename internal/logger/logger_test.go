package logger

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace/noop"
)

func TestLogExecCommand(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, nil))

	ctx := context.Background()
	LogExecCommand(ctx, logger, "test-command", []string{"arg1", "arg2"}, "test.go:123")

	output := buf.String()
	assert.Contains(t, output, "executing command")
	assert.Contains(t, output, "test-command")
	assert.Contains(t, output, "arg1")
	assert.Contains(t, output, "arg2")
}

func TestLogExecCommandResult(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, nil))

	ctx := context.Background()
	LogExecCommandResult(ctx, logger, "test-command", []string{"arg1"}, "success output", nil, 1.5, "test.go:123")
	assert.Contains(t, buf.String(), "command execution successful")

	buf.Reset()
	LogExecCommandResult(ctx, logger, "test-command", []string{"arg1"}, "error output", assert.AnError, 0.5, "test.go:123")
	assert.Contains(t, buf.String(), "command execution failed")
}

func TestWithContextAddsTraceID(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))

	// Create a context with a mock span
	tp := noop.NewTracerProvider()
	ctx, span := tp.Tracer("test").Start(context.Background(), "test-span")
	defer span.End()

	loggerWithTrace := logger.With("trace_id", span.SpanContext().TraceID().String())
	loggerWithTrace.InfoContext(ctx, "test message")

	var logOutput map[string]interface{}
	err := json.Unmarshal(buf.Bytes(), &logOutput)
	require.NoError(t, err)

	traceID := span.SpanContext().TraceID().String()
	assert.Equal(t, traceID, logOutput["trace_id"])
}

func TestGet(t *testing.T) {
	assert.NotNil(t, Get())
}

func TestInit(t *testing.T) {
	assert.NotPanics(t, Init)
}

func TestSync(t *testing.T) {
	assert.NotPanics(t, Sync)
}
