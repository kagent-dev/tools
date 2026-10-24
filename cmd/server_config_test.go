package main

import (
	"testing"

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
			env:      map[string]string{envTools: "helm", envReadOnly: "true", "KUBECONFIG": "/env", envDefaultKubeconfig: "/default"},
			expected: serverConfig{Tools: []string{"k8s"}, ReadOnly: false, Kubeconfig: "/flag"},
		},
		{
			name:     "env fallbacks when flags unset",
			env:      map[string]string{envTools: " k8s, helm ,,", envReadOnly: "true"},
			expected: serverConfig{Tools: []string{"k8s", "helm"}, ReadOnly: true},
		},
		{
			name:     "KUBECONFIG env is left to the CLIs",
			env:      map[string]string{"KUBECONFIG": "/a:/b", envDefaultKubeconfig: "/default"},
			expected: serverConfig{},
		},
		{
			name:     "plugin default kubeconfig used last",
			env:      map[string]string{envDefaultKubeconfig: "/plugin/config/kubeconfig"},
			expected: serverConfig{Kubeconfig: "/plugin/config/kubeconfig"},
		},
		{
			name:     "empty KUBECONFIG does not block default",
			env:      map[string]string{"KUBECONFIG": "", envDefaultKubeconfig: "/d"},
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
