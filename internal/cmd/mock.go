package cmd

import (
	"context"
	"fmt"
	"strings"
	"sync"
)

// MockCall represents a recorded command execution for testing
type MockCall struct {
	Command string
	Args    []string
}

// MockShellExecutor is a mock implementation of ShellExecutor for testing
type MockShellExecutor struct {
	mu           sync.Mutex
	callLog      []MockCall
	commandMocks map[string]map[string]struct {
		output string
		err    error
	}
	partialMatchers []struct {
		command string
		args    []string
		output  string
		err     error
	}
}

// NewMockShellExecutor creates a new mock shell executor
func NewMockShellExecutor() *MockShellExecutor {
	return &MockShellExecutor{
		commandMocks: make(map[string]map[string]struct {
			output string
			err    error
		}),
	}
}

// AddCommandString mocks a command with specific arguments and a string output
func (m *MockShellExecutor) AddCommandString(command string, args []string, output string, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	argsKey := strings.Join(args, " ")
	if _, ok := m.commandMocks[command]; !ok {
		m.commandMocks[command] = make(map[string]struct {
			output string
			err    error
		})
	}
	m.commandMocks[command][argsKey] = struct {
		output string
		err    error
	}{output, err}
}

// AddPartialMatcherString mocks a command with partial argument matching
func (m *MockShellExecutor) AddPartialMatcherString(command string, args []string, output string, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.partialMatchers = append(m.partialMatchers, struct {
		command string
		args    []string
		output  string
		err     error
	}{command, args, output, err})
}

// Exec records the call and returns a mocked output or error
func (m *MockShellExecutor) Exec(ctx context.Context, command string, args ...string) ([]byte, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.callLog = append(m.callLog, MockCall{Command: command, Args: args})

	// Check for exact match first
	argsKey := strings.Join(args, " ")
	if mocks, ok := m.commandMocks[command]; ok {
		if mock, ok := mocks[argsKey]; ok {
			return []byte(mock.output), mock.err
		}
	}

	// Check for partial match
	for _, matcher := range m.partialMatchers {
		if matcher.command == command && argsContain(args, matcher.args) {
			return []byte(matcher.output), matcher.err
		}
	}

	return nil, fmt.Errorf("no mock found for command: %s %v", command, args)
}

// GetCallLog returns the history of commands executed
func (m *MockShellExecutor) GetCallLog() []MockCall {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.callLog
}

// argsContain checks if all elements of subset are in set
func argsContain(set, subset []string) bool {
	for _, sub := range subset {
		found := false
		for _, s := range set {
			if strings.Contains(s, sub) {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}
