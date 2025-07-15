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
