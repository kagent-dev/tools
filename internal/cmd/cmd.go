package cmd

import (
	"context"
	"os/exec"
	"time"

	"github.com/kagent-dev/tools/internal/logger"
)

// ShellExecutor defines the interface for executing shell commands
type ShellExecutor interface {
	Exec(ctx context.Context, command string, args ...string) (output []byte, err error)
}

// DefaultShellExecutor implements ShellExecutor using os/exec
type DefaultShellExecutor struct{}

// Exec executes a command using os/exec.CommandContext
func (e *DefaultShellExecutor) Exec(ctx context.Context, command string, args ...string) ([]byte, error) {
	log := logger.WithContext(ctx)
	startTime := time.Now()

	log.Info("executing command",
		"command", command,
		"args", args,
	)

	cmd := exec.CommandContext(ctx, command, args...)
	output, err := cmd.CombinedOutput()

	duration := time.Since(startTime)

	if err != nil {
		log.Error("command execution failed",
			"command", command,
			"args", args,
			"error", err,
			"output", string(output),
			"duration", duration.Seconds(),
		)
	} else {
		log.Info("command execution successful",
			"command", command,
			"args", args,
			"duration", duration.Seconds(),
		)
	}

	return output, err
}

// Context key for shell executor injection
type contextKey string

const shellExecutorKey contextKey = "shellExecutor"

// WithShellExecutor returns a context with the given shell executor
func WithShellExecutor(ctx context.Context, executor ShellExecutor) context.Context {
	return context.WithValue(ctx, shellExecutorKey, executor)
}

// GetShellExecutor retrieves the shell executor from context, or returns default
func GetShellExecutor(ctx context.Context) ShellExecutor {
	if executor, ok := ctx.Value(shellExecutorKey).(ShellExecutor); ok {
		return executor
	}
	return &DefaultShellExecutor{}
}
