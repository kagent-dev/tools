package main

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/kagent-dev/tools/internal/logger"
)

const (
	envTools             = "KAGENT_TOOLS_TOOLS"
	envReadOnly          = "KAGENT_TOOLS_READ_ONLY"
	envDefaultKubeconfig = "KAGENT_TOOLS_DEFAULT_KUBECONFIG"
	envKubeconfig        = "KUBECONFIG"
)

// serverFlags captures CLI flag values and whether they were set explicitly.
type serverFlags struct {
	Tools       []string
	ToolsSet    bool
	ReadOnly    bool
	ReadOnlySet bool
	Kubeconfig  string
}

// serverConfig is the effective configuration after flags and environment.
// Kubeconfig is empty when the CLIs should use KUBECONFIG or in-cluster config.
type serverConfig struct {
	Tools      []string
	ReadOnly   bool
	Kubeconfig string
}

// resolveServerConfig applies flag > environment > default precedence. Env
// fallbacks let kagent Harness env tune the server when it runs as a plugin
// stdio MCP server whose arguments are fixed in the image.
func resolveServerConfig(flags serverFlags, lookupEnv func(string) (string, bool)) (serverConfig, error) {
	resolved := serverConfig{Tools: flags.Tools, ReadOnly: flags.ReadOnly, Kubeconfig: flags.Kubeconfig}

	if !flags.ToolsSet {
		if value, ok := lookupEnv(envTools); ok {
			resolved.Tools = splitToolList(value)
		}
	}
	if !flags.ReadOnlySet {
		if value, ok := lookupEnv(envReadOnly); ok && value != "" {
			parsed, err := strconv.ParseBool(value)
			if err != nil {
				return serverConfig{}, fmt.Errorf("invalid %s %q: %w", envReadOnly, value, err)
			}
			resolved.ReadOnly = parsed
		}
	}
	if resolved.Kubeconfig == "" {
		if value, ok := lookupEnv(envKubeconfig); ok && value != "" {
			return resolved, nil
		}
		if value, ok := lookupEnv(envDefaultKubeconfig); ok {
			resolved.Kubeconfig = value
		}
	}
	return resolved, nil
}

// applyServerEnvironment exports process environment derived from the
// effective configuration. Only k8s and kubescape receive the kubeconfig path
// explicitly; exporting KUBECONFIG makes helm, istioctl, cilium and argo target
// the same cluster. In stdio mode stdout carries the MCP protocol, so console
// output is routed to stderr.
func applyServerEnvironment(config serverConfig, stdioMode bool, setEnv func(key, value string) error) error {
	if config.Kubeconfig != "" {
		if err := setEnv(envKubeconfig, config.Kubeconfig); err != nil {
			return fmt.Errorf("export %s: %w", envKubeconfig, err)
		}
	}
	if stdioMode {
		if err := setEnv(logger.UseStderrEnv, "true"); err != nil {
			return fmt.Errorf("export %s: %w", logger.UseStderrEnv, err)
		}
	}
	return nil
}

func splitToolList(value string) []string {
	var toolNames []string
	for _, part := range strings.Split(value, ",") {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			toolNames = append(toolNames, trimmed)
		}
	}
	return toolNames
}
