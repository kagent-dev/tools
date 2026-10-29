package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/require"
	"sigs.k8s.io/yaml"
)

const pluginSourceDir = "../plugin"

// Mirrors kagent go/core/pkg/agentplugins manifest and mcpServer types.
type pluginManifest struct {
	Schema      string   `json:"$schema"`
	Name        string   `json:"name"`
	Version     string   `json:"version,omitempty"`
	Description string   `json:"description,omitempty"`
	Homepage    string   `json:"homepage,omitempty"`
	Repository  string   `json:"repository,omitempty"`
	License     string   `json:"license,omitempty"`
	Keywords    []string `json:"keywords,omitempty"`
}

type mcpDocument struct {
	Schema  string                     `json:"$schema"`
	Servers map[string]json.RawMessage `json:"mcpServers"`
}

type mcpStdioServer struct {
	Type    string            `json:"type"`
	Command string            `json:"command"`
	Args    []string          `json:"args,omitempty"`
	Env     map[string]string `json:"env,omitempty"`
	CWD     string            `json:"cwd,omitempty"`
}

type skillFrontmatter struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

var (
	pluginNamePattern = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9.-]*[a-z0-9])?$`)
	skillNamePattern  = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)
	toolReference     = regexp.MustCompile("`((?:k8s|helm|istio|argo|cilium|kubescape|prometheus|datetime)_[a-z_]+)`")
	argumentReference = regexp.MustCompile("`([a-z][a-z_]*): [^`]+`")
)

// llmBackedTools need an LLM API key that neither the Helm chart nor the plugin configures,
// so skills must not depend on them.
var llmBackedTools = map[string]bool{
	"k8s_generate_resource":  true,
	"prometheus_promql_tool": true,
}

type toolInputSchema struct {
	Properties map[string]json.RawMessage `json:"properties"`
}

func decodeStrict[Document pluginManifest | mcpDocument | mcpStdioServer](t *testing.T, raw []byte) Document {
	t.Helper()
	var document Document
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	require.NoError(t, decoder.Decode(&document))
	return document
}

func TestPluginManifestsAreValid(t *testing.T) {
	for _, file := range []string{"plugin.json.tmpl", "skills-plugin.json.tmpl"} {
		raw, err := os.ReadFile(filepath.Join(pluginSourceDir, file))
		require.NoError(t, err)
		manifest := decodeStrict[pluginManifest](t, raw)
		require.Equal(t, "https://agent-plugins.org/schemas/1.0.0/plugin.schema.json", manifest.Schema, file)
		require.Regexp(t, pluginNamePattern, manifest.Name, file)
		require.LessOrEqual(t, len(manifest.Name), 64, file)
		require.Equal(t, "__VERSION__", manifest.Version, file)
	}
}

func TestPluginMCPConfigIsValid(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join(pluginSourceDir, "mcp.json"))
	require.NoError(t, err)
	document := decodeStrict[mcpDocument](t, raw)
	require.Equal(t, "https://agent-plugins.org/schemas/1.0.0/mcp.schema.json", document.Schema)
	require.Contains(t, document.Servers, "kagent-tools")

	server := decodeStrict[mcpStdioServer](t, document.Servers["kagent-tools"])
	require.Equal(t, "stdio", server.Type)
	require.Equal(t, "./bin/kagent-tools", server.Command)
	require.Contains(t, server.Args, "--stdio")
	require.NotContains(t, server.Env, "PLUGIN_ROOT")
	require.NotContains(t, server.Env, "PLUGIN_DATA")
	require.NotContains(t, server.Env, envTools, "operators set this on the Harness; mcp.json env would override it")
	require.NotContains(t, server.Env, envReadOnly, "operators set this on the Harness; mcp.json env would override it")
	require.NotContains(t, server.Env, "KUBECONFIG", "use KAGENT_TOOLS_DEFAULT_KUBECONFIG so Harness KUBECONFIG still wins")
	require.Equal(t, "${PLUGIN_ROOT}/config/kubeconfig", server.Env[envDefaultKubeconfig])
	require.FileExists(t, filepath.Join(pluginSourceDir, "config", "kubeconfig"))
}

// registeredTools returns the input properties of every tool the server registers, keyed by tool name.
func registeredTools(t *testing.T) map[string]toolInputSchema {
	t.Helper()
	ctx := context.Background()
	server := sdkmcp.NewServer(&sdkmcp.Implementation{Name: "plugin", Version: "test"}, nil)
	registerMCP(server, nil, "", false)
	serverTransport, clientTransport := sdkmcp.NewInMemoryTransports()
	go func() { _ = server.Run(ctx, serverTransport) }()
	client := sdkmcp.NewClient(&sdkmcp.Implementation{Name: "plugin-client", Version: "test"}, nil)
	session, err := client.Connect(ctx, clientTransport, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = session.Close() })
	tools := map[string]toolInputSchema{}
	for tool, err := range session.Tools(ctx, nil) {
		require.NoError(t, err)
		rawSchema, err := json.Marshal(tool.InputSchema)
		require.NoError(t, err)
		var schema toolInputSchema
		require.NoError(t, json.Unmarshal(rawSchema, &schema), tool.Name)
		tools[tool.Name] = schema
	}
	return tools
}

func readSkill(t *testing.T, skillName string) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(pluginSourceDir, "skills", skillName, "SKILL.md"))
	require.NoError(t, err, skillName)
	return string(raw)
}

func referencedTools(content string) []string {
	var tools []string
	for _, reference := range toolReference.FindAllStringSubmatch(content, -1) {
		tools = append(tools, reference[1])
	}
	return tools
}

func TestPluginSkillsAreValid(t *testing.T) {
	skillDirs, err := os.ReadDir(filepath.Join(pluginSourceDir, "skills"))
	require.NoError(t, err)
	require.NotEmpty(t, skillDirs)
	knownTools := registeredTools(t)

	for _, skillDir := range skillDirs {
		require.True(t, skillDir.IsDir(), skillDir.Name())
		content := readSkill(t, skillDir.Name())
		require.True(t, strings.HasPrefix(content, "---\n"), "%s: missing frontmatter", skillDir.Name())
		frontmatterEnd := strings.Index(content[4:], "\n---\n")
		require.Positive(t, frontmatterEnd, "%s: unterminated frontmatter", skillDir.Name())
		var frontmatter skillFrontmatter
		require.NoError(t, yaml.Unmarshal([]byte(content[4:4+frontmatterEnd]), &frontmatter))

		require.Equal(t, skillDir.Name(), frontmatter.Name)
		require.Regexp(t, skillNamePattern, frontmatter.Name)
		require.LessOrEqual(t, len(frontmatter.Name), 64)
		require.NotEmpty(t, frontmatter.Description)
		require.LessOrEqual(t, len(frontmatter.Description), 1024)

		references := referencedTools(content)
		require.NotEmpty(t, references, "%s: skill must reference at least one MCP tool", skillDir.Name())
		for _, toolName := range references {
			require.Containsf(t, knownTools, toolName, "%s references unknown tool %q", skillDir.Name(), toolName)
			require.Falsef(t, llmBackedTools[toolName], "%s references %q, which needs an unconfigured LLM key", skillDir.Name(), toolName)
		}
		requireArgumentsExist(t, skillDir.Name(), content, knownTools)
	}
}

// requireArgumentsExist checks that every `argument: value` a skill line names is an input
// property of a tool referenced on the same line, so playbooks cannot drift from tool schemas.
func requireArgumentsExist(t *testing.T, skillName, content string, knownTools map[string]toolInputSchema) {
	t.Helper()
	for lineNumber, line := range strings.Split(content, "\n") {
		arguments := argumentReference.FindAllStringSubmatch(line, -1)
		if len(arguments) == 0 {
			continue
		}
		lineTools := referencedTools(line)
		require.NotEmptyf(t, lineTools, "%s:%d names arguments without a tool on the same line", skillName, lineNumber+1)
		for _, argument := range arguments {
			found := false
			for _, toolName := range lineTools {
				if _, ok := knownTools[toolName].Properties[argument[1]]; ok {
					found = true
					break
				}
			}
			require.Truef(t, found, "%s:%d: no tool in %v accepts argument %q", skillName, lineNumber+1, lineTools, argument[1])
		}
	}
}
