package cmd

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDefaultShellExecutor(t *testing.T) {
	executor := &DefaultShellExecutor{}

	// Test successful command
	output, err := executor.Exec(context.Background(), "echo", "hello")
	assert.NoError(t, err)
	assert.Equal(t, "hello\n", string(output))

	// Test command with error
	_, err = executor.Exec(context.Background(), "nonexistent-command")
	assert.Error(t, err)
}

func TestMockShellExecutor(t *testing.T) {
	mock := NewMockShellExecutor()

	t.Run("unmocked command returns error", func(t *testing.T) {
		_, err := mock.Exec(context.Background(), "unmocked", "command")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "no mock found for command")
	})

	t.Run("mocked command returns expected result", func(t *testing.T) {
		expectedOutput := "mocked output"
		mock.AddCommandString("kubectl", []string{"get", "pods"}, expectedOutput, nil)

		output, err := mock.Exec(context.Background(), "kubectl", "get", "pods")
		assert.NoError(t, err)
		assert.Equal(t, expectedOutput, string(output))
	})
}

func TestContextShellExecutor(t *testing.T) {
	t.Run("default executor when no context value", func(t *testing.T) {
		ctx := context.Background()
		executor := GetShellExecutor(ctx)

		_, ok := executor.(*DefaultShellExecutor)
		assert.True(t, ok, "should return DefaultShellExecutor when no context value")
	})

	t.Run("mock executor from context", func(t *testing.T) {
		mock := NewMockShellExecutor()
		ctx := WithShellExecutor(context.Background(), mock)

		executor := GetShellExecutor(ctx)
		assert.Equal(t, mock, executor, "should return the mock executor from context")
	})
}

func TestDefaultShellExecutorWithContext(t *testing.T) {
	executor := &DefaultShellExecutor{}

	t.Run("cancelled context", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel() // Cancel immediately

		_, err := executor.Exec(ctx, "sleep", "10")
		assert.Error(t, err)
	})

	t.Run("successful command with context", func(t *testing.T) {
		ctx := context.Background()
		output, err := executor.Exec(ctx, "echo", "test")
		assert.NoError(t, err)
		assert.Contains(t, string(output), "test")
	})

	t.Run("command with multiple args", func(t *testing.T) {
		ctx := context.Background()
		output, err := executor.Exec(ctx, "echo", "arg1", "arg2", "arg3")
		assert.NoError(t, err)
		assert.Contains(t, string(output), "arg1")
		assert.Contains(t, string(output), "arg2")
		assert.Contains(t, string(output), "arg3")
	})

	t.Run("command that fails", func(t *testing.T) {
		ctx := context.Background()
		_, err := executor.Exec(ctx, "false") // 'false' always exits with error code 1
		assert.Error(t, err)
	})
}

func TestWithShellExecutor(t *testing.T) {
	mock := NewMockShellExecutor()
	ctx := WithShellExecutor(context.Background(), mock)

	// Verify the executor is in the context
	value := ctx.Value(shellExecutorKey)
	assert.NotNil(t, value)
	assert.Equal(t, mock, value)
}

func TestGetShellExecutorReturnsDefault(t *testing.T) {
	// Create a context without a shell executor
	ctx := context.Background()
	executor := GetShellExecutor(ctx)

	// Should return DefaultShellExecutor
	_, ok := executor.(*DefaultShellExecutor)
	assert.True(t, ok)
}

func TestShellExecutorInterface(t *testing.T) {
	// Verify DefaultShellExecutor implements ShellExecutor interface
	var _ ShellExecutor = (*DefaultShellExecutor)(nil)

	// Verify MockShellExecutor implements ShellExecutor interface
	var _ ShellExecutor = (*MockShellExecutor)(nil)
}
