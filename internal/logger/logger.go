package logger

import (
	"context"
	"log/slog"
	"os"

	"go.opentelemetry.io/otel/trace"
)

var globalLogger *slog.Logger

// parseLogLevel converts a string log level to slog.Level
func parseLogLevel(level string) slog.Level {
	switch level {
	case "debug":
		return slog.LevelDebug
	case "info":
		return slog.LevelInfo
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// Init initializes the global logger
// If useStderr is true, logs will be written to stderr (for stdio mode)
// If useStderr is false, logs will be written to stdout (for HTTP mode)
// logLevel can be "debug", "info", "warn", or "error"
func Init(useStderr bool, logLevel string) {
	level := parseLogLevel(logLevel)
	opts := &slog.HandlerOptions{
		Level: level,
	}

	// Choose output destination based on mode
	output := os.Stdout
	if useStderr {
		output = os.Stderr
	}

	if os.Getenv("KAGENT_LOG_FORMAT") == "json" {
		globalLogger = slog.New(slog.NewJSONHandler(output, opts))
	} else {
		globalLogger = slog.New(slog.NewTextHandler(output, opts))
	}

	slog.SetDefault(globalLogger)
}

// InitWithEnv initializes the logger using environment variables
// This is a convenience function that defaults to stdout unless KAGENT_USE_STDERR is set
func InitWithEnv() {
	useStderr := os.Getenv("KAGENT_USE_STDERR") == "true"
	logLevel := os.Getenv("KAGENT_LOG_LEVEL")
	if logLevel == "" {
		logLevel = "info"
	}
	Init(useStderr, logLevel)
}

func Get() *slog.Logger {
	if globalLogger == nil {
		InitWithEnv()
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
