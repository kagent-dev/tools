package main

import (
	"errors"
	"testing"

	"github.com/kagent-dev/tools/internal/logger"
	"github.com/stretchr/testify/require"
)

func envFrom(values map[string]string) func(string) (string, bool) {
	return func(key string) (string, bool) {
		value, ok := values[key]
		return value, ok
	}
}

func TestResolveServerConfig(t *testing.T) {
	testCases := []struct {
		name     string
		flags    serverFlags
		env      map[string]string
		expected serverConfig
		wantErr  string
	}{
		{name: "defaults", expected: serverConfig{}},
		{
			name:     "flags win over env",
			flags:    serverFlags{Tools: []string{"k8s"}, ToolsSet: true, ReadOnly: false, ReadOnlySet: true, Kubeconfig: "/flag"},
			env:      map[string]string{envTools: "helm", envReadOnly: "true", envKubeconfig: "/env", envDefaultKubeconfig: "/default"},
			expected: serverConfig{Tools: []string{"k8s"}, ReadOnly: false, Kubeconfig: "/flag"},
		},
		{
			name:     "env fallbacks when flags unset",
			env:      map[string]string{envTools: " k8s, helm ,,", envReadOnly: "true"},
			expected: serverConfig{Tools: []string{"k8s", "helm"}, ReadOnly: true},
		},
		{
			name:     "KUBECONFIG env is left to the CLIs",
			env:      map[string]string{envKubeconfig: "/a:/b", envDefaultKubeconfig: "/default"},
			expected: serverConfig{},
		},
		{
			name:     "plugin default kubeconfig used last",
			env:      map[string]string{envDefaultKubeconfig: "/plugin/config/kubeconfig"},
			expected: serverConfig{Kubeconfig: "/plugin/config/kubeconfig"},
		},
		{
			name:     "empty KUBECONFIG does not block default",
			env:      map[string]string{envKubeconfig: "", envDefaultKubeconfig: "/d"},
			expected: serverConfig{Kubeconfig: "/d"},
		},
		{
			name:    "invalid read-only env",
			env:     map[string]string{envReadOnly: "maybe"},
			wantErr: envReadOnly,
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			resolved, err := resolveServerConfig(testCase.flags, envFrom(testCase.env))
			if testCase.wantErr != "" {
				require.ErrorContains(t, err, testCase.wantErr)
				return
			}
			require.NoError(t, err)
			require.Equal(t, testCase.expected, resolved)
		})
	}
}

func TestApplyServerEnvironment(t *testing.T) {
	testCases := []struct {
		name      string
		config    serverConfig
		stdioMode bool
		expected  map[string]string
	}{
		{name: "nothing to export", expected: map[string]string{}},
		{
			name:     "kubeconfig exported for every CLI",
			config:   serverConfig{Kubeconfig: "/plugin/config/kubeconfig"},
			expected: map[string]string{envKubeconfig: "/plugin/config/kubeconfig"},
		},
		{
			name:      "stdio routes console output to stderr",
			stdioMode: true,
			expected:  map[string]string{logger.UseStderrEnv: "true"},
		},
		{
			name:      "stdio plugin mode exports both",
			config:    serverConfig{Kubeconfig: "/k"},
			stdioMode: true,
			expected:  map[string]string{envKubeconfig: "/k", logger.UseStderrEnv: "true"},
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			exported := map[string]string{}
			err := applyServerEnvironment(testCase.config, testCase.stdioMode, func(key, value string) error {
				exported[key] = value
				return nil
			})
			require.NoError(t, err)
			require.Equal(t, testCase.expected, exported)
		})
	}
}

func TestApplyServerEnvironmentPropagatesErrors(t *testing.T) {
	failingSetEnv := func(string, string) error { return errors.New("denied") }

	err := applyServerEnvironment(serverConfig{Kubeconfig: "/k"}, false, failingSetEnv)
	require.ErrorContains(t, err, envKubeconfig)

	err = applyServerEnvironment(serverConfig{}, true, failingSetEnv)
	require.ErrorContains(t, err, logger.UseStderrEnv)
}
