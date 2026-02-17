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

func TestRedactArgsForLog(t *testing.T) {
	t.Run("redacts token value", func(t *testing.T) {
		args := []string{"get", "pods", "--token", "secret-token-123", "-n", "default"}
		redacted := RedactArgsForLog(args)
		require.Len(t, redacted, 6)
		assert.Equal(t, "get", redacted[0])
		assert.Equal(t, "pods", redacted[1])
		assert.Equal(t, "--token", redacted[2])
		assert.Equal(t, "<REDACTED>", redacted[3])
		assert.Equal(t, "-n", redacted[4])
		assert.Equal(t, "default", redacted[5])
	})
	t.Run("empty args returns nil", func(t *testing.T) {
		assert.Nil(t, RedactArgsForLog(nil))
		assert.Nil(t, RedactArgsForLog([]string{}))
	})
	t.Run("args without token unchanged", func(t *testing.T) {
		args := []string{"get", "pods", "-n", "default"}
		redacted := RedactArgsForLog(args)
		assert.Equal(t, args, redacted)
	})
	t.Run("--token at end with no value", func(t *testing.T) {
		args := []string{"get", "pods", "--token"}
		redacted := RedactArgsForLog(args)
		assert.Equal(t, args, redacted)
	})
	t.Run("logged output does not contain token", func(t *testing.T) {
		var buf bytes.Buffer
		log := slog.New(slog.NewTextHandler(&buf, nil))
		args := []string{"get", "pods", "--token", "secret-token-123"}
		log.Info("executing command", "command", "kubectl", "args", RedactArgsForLog(args))
		output := buf.String()
		assert.Contains(t, output, "<REDACTED>")
		assert.NotContains(t, output, "secret-token-123")
	})
}

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
	assert.NotPanics(t, func() { Init(false) })
	assert.NotPanics(t, func() { Init(true) })
}

func TestSync(t *testing.T) {
	assert.NotPanics(t, Sync)
}
