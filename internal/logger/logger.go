package logger

import (
	"context"
	"log/slog"
	"os"

	"go.opentelemetry.io/otel/trace"
)

var globalLogger *slog.Logger

func Init() {
	opts := &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}

	if os.Getenv("KAGENT_LOG_FORMAT") == "json" {
		globalLogger = slog.New(slog.NewJSONHandler(os.Stdout, opts))
	} else {
		globalLogger = slog.New(slog.NewTextHandler(os.Stdout, opts))
	}

	slog.SetDefault(globalLogger)
}

func Get() *slog.Logger {
	if globalLogger == nil {
		Init()
	}
	return globalLogger
}

func WithContext(ctx context.Context) *slog.Logger {
	logger := Get()
	span := trace.SpanFromContext(ctx)
	if span.SpanContext().IsValid() {
		logger = logger.With(
			"trace_id", span.SpanContext().TraceID().String(),
			"span_id", span.SpanContext().SpanID().String(),
		)
	}
	return logger
}

func LogExecCommand(ctx context.Context, logger *slog.Logger, command string, args []string, caller string) {
	logger.Info("executing command",
		"command", command,
		"args", args,
		"caller", caller,
	)
}

func LogExecCommandResult(ctx context.Context, logger *slog.Logger, command string, args []string, output string, err error, duration float64, caller string) {
	if err != nil {
		logger.Error("command execution failed",
			"command", command,
			"args", args,
			"error", err.Error(),
			"duration_seconds", duration,
			"caller", caller,
		)
	} else {
		logger.Info("command execution successful",
			"command", command,
			"args", args,
			"output", output,
			"duration_seconds", duration,
			"caller", caller,
		)
	}
}

func Sync() {
	// No-op for slog, but kept for compatibility
}
