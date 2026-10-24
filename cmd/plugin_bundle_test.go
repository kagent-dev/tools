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
)

func decodeStrict(t *testing.T, raw []byte, target any) {
	t.Helper()
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	require.NoError(t, decoder.Decode(target))
}

func TestPluginManifestsAreValid(t *testing.T) {
	for _, file := range []string{"plugin.json.tmpl", "skills-plugin.json.tmpl"} {
		raw, err := os.ReadFile(filepath.Join(pluginSourceDir, file))
		require.NoError(t, err)
		var manifest pluginManifest
		decodeStrict(t, raw, &manifest)
		require.Equal(t, "https://agent-plugins.org/schemas/1.0.0/plugin.schema.json", manifest.Schema, file)
		require.Regexp(t, pluginNamePattern, manifest.Name, file)
		require.LessOrEqual(t, len(manifest.Name), 64, file)
		require.Equal(t, "__VERSION__", manifest.Version, file)
	}
}

func TestPluginMCPConfigIsValid(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join(pluginSourceDir, "mcp.json"))
	require.NoError(t, err)
	var document mcpDocument
	decodeStrict(t, raw, &document)
	require.Equal(t, "https://agent-plugins.org/schemas/1.0.0/mcp.schema.json", document.Schema)
	require.Contains(t, document.Servers, "kagent-tools")

	var server mcpStdioServer
	decodeStrict(t, document.Servers["kagent-tools"], &server)
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

func registeredToolNames(t *testing.T) map[string]bool {
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
	names := map[string]bool{}
	for tool, err := range session.Tools(ctx, nil) {
		require.NoError(t, err)
		names[tool.Name] = true
	}
	return names
}

func TestPluginSkillsAreValid(t *testing.T) {
	skillDirs, err := os.ReadDir(filepath.Join(pluginSourceDir, "skills"))
	require.NoError(t, err)
	require.NotEmpty(t, skillDirs)
	knownTools := registeredToolNames(t)

	for _, skillDir := range skillDirs {
		require.True(t, skillDir.IsDir(), skillDir.Name())
		raw, err := os.ReadFile(filepath.Join(pluginSourceDir, "skills", skillDir.Name(), "SKILL.md"))
		require.NoError(t, err, skillDir.Name())

		content := string(raw)
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

		references := toolReference.FindAllStringSubmatch(content, -1)
		require.NotEmpty(t, references, "%s: skill must reference at least one MCP tool", skillDir.Name())
		for _, reference := range references {
			require.Truef(t, knownTools[reference[1]], "%s references unknown tool %q", skillDir.Name(), reference[1])
		}
	}
}
