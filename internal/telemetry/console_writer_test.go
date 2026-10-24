package telemetry

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestConsoleWriterUsesStderrWhenRequested(t *testing.T) {
	t.Setenv("KAGENT_USE_STDERR", "true")
	require.Same(t, os.Stderr, consoleWriter())
}

func TestConsoleWriterDefaultsToStdout(t *testing.T) {
	t.Setenv("KAGENT_USE_STDERR", "")
	require.Same(t, os.Stdout, consoleWriter())
}
