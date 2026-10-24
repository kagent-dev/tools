# kagent-tools OCI Skills Bundle Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make the `kagent-tools` image usable unchanged both as the existing MCP server and as a kagent 1.x Agent Plugin (OCI skills bundle) that carries all CLIs, the MCP server (stdio), and skills.

**Architecture:** Restructure the image so everything lives under the Agent Plugin root `/opt/kagent-tools` (`plugin.json`, `mcp.json`, `bin/*`, `config/kubeconfig`, `skills/*`), plus a skills-only root `/opt/kagent-tools-skills`. Legacy paths (`/tool-server`, `/bin/*`) become symlinks outside both roots. kagent mounts the image read-only, copies selected skills, and starts `./bin/kagent-tools --stdio` from `mcp.json`. The server gains env fallbacks for flags, a kubeconfig resolution chain, and a stdio-safe trace exporter.

**Tech Stack:** Go 1.27, cobra, MCP Go SDK, testify, Docker buildx (distroless), kagent `api.kagent.dev/v1alpha3`.

**Spec:** `docs/superpowers/specs/2026-10-24-kagent-tools-skills-bundle-design.md`

---

## File map

| File | Action | Responsibility |
|---|---|---|
| `cmd/server_config.go` | create | `resolveServerConfig`: flag/env precedence, kubeconfig chain |
| `cmd/server_config_test.go` | create | precedence table tests |
| `cmd/main.go` | modify | wire `resolveServerConfig`, export `KUBECONFIG`, force stderr in stdio |
| `internal/telemetry/tracing.go` | modify | console exporter → stderr when `KAGENT_USE_STDERR=true` |
| `internal/telemetry/console_writer_test.go` | create | test for writer choice |
| `plugin/plugin.json.tmpl` | create | full plugin manifest (`__VERSION__` placeholder) |
| `plugin/skills-plugin.json.tmpl` | create | skills-only manifest |
| `plugin/mcp.json` | create | stdio server declaration |
| `plugin/config/kubeconfig` | create | egress-gateway kubeconfig |
| `plugin/skills/<7 skills>/SKILL.md` | create | playbooks |
| `cmd/plugin_bundle_test.go` | create | validates `plugin/` source tree + skill tool names |
| `cmd/plugin_image_test.go` | create | validates an exported image tree (opt-in via env) |
| `scripts/verify-plugin-image.sh` | create | export image, run image test, stdio smoke |
| `Dockerfile` | modify | assemble plugin roots, symlinks, entrypoint |
| `Makefile` | modify | `plugin-verify` target |
| `.github/workflows/ci.yaml` | modify | run `make plugin-verify` |
| `docs/kagent-skills.md` | create | user guide (RBAC, EgressPolicy, AgentTemplate) |
| `README.md`, `AGENTS.md` | modify | links + `plugin/` in repo structure |

Task 0 is in `kagent-dev/kagent` and can proceed in parallel. Tasks 1–9 are in this repo.

---

### Task 0 (upstream, `kagent-dev/kagent`): Do not byte-limit platform-mounted packages

**Files:**
- Modify: `go/core/pkg/agentplugins/materialize.go` (`fetchSource`, `validatedPackageRoot`, `validatePackage`, `copySkill`)
- Test: `go/core/pkg/agentplugins/materialize_test.go`

- [ ] **Step 1: Write the failing test** (append to `materialize_test.go`)

```go
func TestMountedPluginMayExceedPackageByteLimit(t *testing.T) {
	mounted := t.TempDir()
	root := filepath.Join(mounted, "opt", "bundle")
	require.NoError(t, os.MkdirAll(filepath.Join(root, "bin"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(root, "skills", "small"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "plugin.json"),
		[]byte(`{"$schema":"https://agent-plugins.org/schemas/1.0.0/plugin.schema.json","name":"bundle"}`), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "skills", "small", "SKILL.md"),
		[]byte("---\nname: small\ndescription: d\n---\nbody\n"), 0o644))
	big, err := os.Create(filepath.Join(root, "bin", "big"))
	require.NoError(t, err)
	require.NoError(t, big.Truncate(maxPackageBytes+1)) // sparse; counted by size
	require.NoError(t, big.Close())

	out := t.TempDir()
	_, err = Materialize(context.Background(), agentplugin.Resources{
		Plugins: []agentplugin.Bundle{{
			Source: agentplugin.Source{Directory: mounted, Path: "opt/bundle"},
			Skills: []string{"small"},
		}},
	}, Paths{Packages: filepath.Join(out, "packages"), Skills: filepath.Join(out, "skills")})
	require.NoError(t, err)
	require.FileExists(t, filepath.Join(out, "skills", "small", "SKILL.md"))
}

func TestCopiedSkillStillByteLimited(t *testing.T) {
	src := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(src, "SKILL.md"), []byte("---\nname: s\ndescription: d\n---\n"), 0o644))
	big, err := os.Create(filepath.Join(src, "big"))
	require.NoError(t, err)
	require.NoError(t, big.Truncate(maxPackageBytes+1))
	require.NoError(t, big.Close())
	err = copySkill(src, filepath.Join(t.TempDir(), "s"))
	require.ErrorContains(t, err, "exceeds")
}
```

(Match the `agentplugin.Bundle` field names to `go/api/agentplugin/types.go`. If the bundle type is named differently, use that name.)

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd go && go test ./core/pkg/agentplugins/ -run 'TestMountedPluginMayExceed|TestCopiedSkillStillByteLimited' -v`
Expected: FAIL, the first with `artifact exceeds 104857600 bytes` and the second with no error.

- [ ] **Step 3: Implement**

In `materialize.go`:

```go
// validatePackage checks entry count, file types, and symlink containment.
// limitBytes is false for read-only platform mounts, which are never copied
// or downloaded as a whole; copySkill enforces its own byte budget.
func validatePackage(root string, limitBytes bool) error {
	var entries, bytes int64
	return filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		// … unchanged …
		if info.Mode().IsRegular() {
			bytes += info.Size()
			if limitBytes && bytes > maxPackageBytes {
				return fmt.Errorf("artifact exceeds %d bytes", maxPackageBytes)
			}
		}
		// … unchanged …
	})
}

func validatedPackageRoot(root, relative string, limitBytes bool) (string, error) {
	selected, err := containedPath(root, relative)
	if err != nil {
		return "", err
	}
	if err := validatePackage(selected, limitBytes); err != nil {
		return "", err
	}
	return selected, nil
}
```

In `fetchSource`: the `source.Directory` branch calls `validatedPackageRoot(source.Directory, source.Path, false)`. Every Git/S3 call site passes `true`.

In `copySkill`, call `validatePackage(source, true)` before walking, so a copied skill stays bounded to 100 MiB.

- [ ] **Step 4: Run the package tests**

Run: `cd go && go test ./core/pkg/agentplugins/... -v`
Expected: PASS (new and existing).

- [ ] **Step 5: Update docs and open the PR**

In `docs/architecture/configuration-and-compilation.md` § "OCI skill delivery", replace "Package limits and path/symlink validation still apply." with:
"Path, symlink, file-type, and entry-count validation apply to every package. The 100 MiB byte limit applies to Git/S3 downloads and to each skill copied into the harness discovery directory, not to read-only mounted images."

```bash
git checkout -b feat/mounted-plugin-size
git add go/core/pkg/agentplugins docs/architecture/configuration-and-compilation.md
git commit -s -m "feat(agentplugins): do not byte-limit platform-mounted plugin packages"
```

---

### Task 1: Server config resolution (flags > env > defaults, kubeconfig chain)

**Files:**
- Create: `cmd/server_config.go`
- Test: `cmd/server_config_test.go`

- [ ] **Step 1: Write the failing test**

```go
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
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./cmd -run TestResolveServerConfig -v`
Expected: FAIL to compile (`undefined: serverFlags`).

- [ ] **Step 3: Implement `cmd/server_config.go`**

```go
package main

import (
	"fmt"
	"strconv"
	"strings"
)

const (
	envTools             = "KAGENT_TOOLS_TOOLS"
	envReadOnly          = "KAGENT_TOOLS_READ_ONLY"
	envDefaultKubeconfig = "KAGENT_TOOLS_DEFAULT_KUBECONFIG"
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
		if value, ok := lookupEnv("KUBECONFIG"); ok && value != "" {
			return resolved, nil
		}
		if value, ok := lookupEnv(envDefaultKubeconfig); ok {
			resolved.Kubeconfig = value
		}
	}
	return resolved, nil
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
```

Note: the "defaults" case expects `Tools: nil`. `flags.Tools` is nil in that case, so `require.Equal` holds.

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test ./cmd -run TestResolveServerConfig -v`
Expected: PASS (7 subtests).

- [ ] **Step 5: Commit**

```bash
git add cmd/server_config.go cmd/server_config_test.go
git commit -s -m "feat: resolve tools, read-only and kubeconfig from env fallbacks"
```

---

### Task 2: Wire config into `run()` and make stdio output protocol-safe

**Files:**
- Modify: `cmd/main.go` (`run`, around lines 114–158)
- Modify: `internal/telemetry/tracing.go:146-148`
- Test: `internal/telemetry/console_writer_test.go`

- [ ] **Step 1: Write the failing telemetry test**

```go
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
```

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./internal/telemetry -run TestConsoleWriter -v`
Expected: FAIL (`undefined: consoleWriter`).

- [ ] **Step 3: Implement in `internal/telemetry/tracing.go`**

Add `"os"` to this file's imports if it is missing, then:

```go
// consoleWriter keeps the fallback span exporter off stdout when stdout
// carries the MCP stdio protocol.
func consoleWriter() *os.File {
	if os.Getenv("KAGENT_USE_STDERR") == "true" {
		return os.Stderr
	}
	return os.Stdout
}
```

Replace the fallback in `createExporter`:

```go
	if cfg.Endpoint == "" {
		return stdouttrace.New(stdouttrace.WithWriter(consoleWriter()), stdouttrace.WithPrettyPrint())
	}
```

- [ ] **Step 4: Wire `cmd/main.go`**

In `run()`, directly after the `if metricsPort == 0 { … }` block and **before** `logger.Init(stdio)`, insert:

```go
	effectiveConfig, err := resolveServerConfig(serverFlags{
		Tools:       tools,
		ToolsSet:    cmd.Flags().Changed("tools"),
		ReadOnly:    readOnly,
		ReadOnlySet: cmd.Flags().Changed("read-only"),
		Kubeconfig:  *kubeconfig,
	}, os.LookupEnv)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	tools, readOnly = effectiveConfig.Tools, effectiveConfig.ReadOnly
	if effectiveConfig.Kubeconfig != "" {
		// Only k8s and kubescape receive the path explicitly; exporting it makes
		// helm, istioctl, cilium and argo target the same cluster.
		_ = os.Setenv("KUBECONFIG", effectiveConfig.Kubeconfig)
	}
	if stdio {
		_ = os.Setenv("KAGENT_USE_STDERR", "true")
	}
```

Then change the later `err := telemetry.SetupOTelSDK(ctx)` to `err = telemetry.SetupOTelSDK(ctx)`, because `err` is now declared. Change `registerMCP(mcpSrv, tools, *kubeconfig, readOnly)` to `registerMCP(mcpSrv, tools, effectiveConfig.Kubeconfig, readOnly)`.

- [ ] **Step 5: Run the tests and a manual stdio check**

Run: `go test ./cmd ./internal/telemetry -v 2>&1 | tail -20`
Expected: PASS.

Run:
```bash
go build -o /tmp/kt ./cmd && printf '%s\n' \
 '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"smoke","version":"0"}}}' \
 '{"jsonrpc":"2.0","method":"notifications/initialized"}' \
 '{"jsonrpc":"2.0","id":2,"method":"tools/list"}' \
 | KAGENT_TOOLS_TOOLS=utils /tmp/kt --stdio 2>/dev/null | head -c 400; echo
```
Expected: stdout contains only JSON-RPC lines (`"id":1` and `"id":2` with `datetime_get_current_time`) and no pretty-printed span JSON.

- [ ] **Step 6: Commit**

```bash
git add cmd/main.go internal/telemetry/tracing.go internal/telemetry/console_writer_test.go
git commit -s -m "fix: keep stdio MCP stream clean and apply env config fallbacks"
```

---

### Task 3: Plugin source tree (manifests, mcp.json, kubeconfig)

**Files:** create everything under `plugin/` listed below.

- [ ] **Step 1: Create `plugin/plugin.json.tmpl`**

```json
{
  "$schema": "https://agent-plugins.org/schemas/1.0.0/plugin.schema.json",
  "name": "kagent-tools",
  "version": "__VERSION__",
  "description": "Kubernetes, Helm, Istio, Argo Rollouts, Cilium, Kubescape and Prometheus tools with operational skills for kagent agents.",
  "homepage": "https://github.com/kagent-dev/tools",
  "repository": "https://github.com/kagent-dev/tools",
  "license": "Apache-2.0",
  "keywords": ["kubernetes", "helm", "istio", "argo-rollouts", "cilium", "kubescape", "prometheus", "mcp"]
}
```

- [ ] **Step 2: Create `plugin/skills-plugin.json.tmpl`**

```json
{
  "$schema": "https://agent-plugins.org/schemas/1.0.0/plugin.schema.json",
  "name": "kagent-tools-skills",
  "version": "__VERSION__",
  "description": "Operational skills for the kagent-tools MCP server (use with a kagent-tools RemoteMCPServer).",
  "homepage": "https://github.com/kagent-dev/tools",
  "repository": "https://github.com/kagent-dev/tools",
  "license": "Apache-2.0"
}
```

- [ ] **Step 3: Create `plugin/mcp.json`**

```json
{
  "$schema": "https://agent-plugins.org/schemas/1.0.0/mcp.schema.json",
  "mcpServers": {
    "kagent-tools": {
      "type": "stdio",
      "command": "./bin/kagent-tools",
      "args": ["--stdio"],
      "env": {
        "PATH": "${PLUGIN_ROOT}/bin:/usr/local/bin:/usr/bin:/bin",
        "KAGENT_TOOLS_DEFAULT_KUBECONFIG": "${PLUGIN_ROOT}/config/kubeconfig",
        "KAGENT_USE_STDERR": "true",
        "HOME": "${PLUGIN_DATA}",
        "XDG_CACHE_HOME": "${PLUGIN_DATA}/cache",
        "XDG_CONFIG_HOME": "${PLUGIN_DATA}/config",
        "XDG_DATA_HOME": "${PLUGIN_DATA}/data"
      }
    }
  }
}
```

- [ ] **Step 4: Create `plugin/config/kubeconfig`**

```yaml
# Used when kagent-tools runs as a kagent Agent Plugin. Requests go through the
# kagent/Substrate egress gateway, which replaces the placeholder bearer token
# with a ServiceAccount token from an EgressPolicy. No credentials live here.
apiVersion: v1
kind: Config
clusters:
  - name: in-cluster
    cluster:
      server: https://kubernetes.default.svc
      certificate-authority: /run/kagent/egress/trust-bundle.pem
users:
  - name: kagent-egress
    user:
      token: kagent-credential-injected
contexts:
  - name: default
    context:
      cluster: in-cluster
      user: kagent-egress
current-context: default
```

- [ ] **Step 5: Commit**

```bash
git add plugin/plugin.json.tmpl plugin/skills-plugin.json.tmpl plugin/mcp.json plugin/config/kubeconfig
git commit -s -m "feat(plugin): add kagent Agent Plugin manifests and egress kubeconfig"
```

---

### Task 4: Plugin source validation test

**Files:**
- Create: `cmd/plugin_bundle_test.go`
- Modify: `go.mod` (promote `sigs.k8s.io/yaml` to a direct dependency through `go mod tidy`)

- [ ] **Step 1: Write the test** (it fails now because `plugin/skills` does not exist)

```go
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
```

- [ ] **Step 2: Run the tests**

Run: `go mod tidy && go test ./cmd -run 'TestPlugin' -v`
Expected: `TestPluginManifestsAreValid` and `TestPluginMCPConfigIsValid` PASS; `TestPluginSkillsAreValid` FAILS (`no such file or directory … plugin/skills`).

- [ ] **Step 3: Commit the test** (Task 5 makes it pass)

```bash
git add cmd/plugin_bundle_test.go go.mod go.sum
git commit -s -m "test(plugin): validate plugin manifests, mcp.json and skills"
```

---

### Task 5: Skills

**Files:** create `plugin/skills/<name>/SKILL.md` for each of the 7 skills below, with the exact content shown.

- [ ] **Step 1: `plugin/skills/k8s-troubleshooting/SKILL.md`**

```markdown
---
name: k8s-troubleshooting
description: Diagnose unhealthy Kubernetes workloads (CrashLoopBackOff, Pending, ImagePullBackOff, OOMKilled, failing readiness, unreachable Services) using the kagent-tools k8s_* MCP tools. Use when a pod, deployment or service is not working as expected.
---
# Kubernetes troubleshooting

Work from symptoms to cause. Gather facts before proposing changes.

## 1. Locate the failing object (read-only)
1. `k8s_get_resources` with `resource_type: pods` and the namespace (`all_namespaces: true` if unknown). Note STATUS, RESTARTS, and READY.
2. `k8s_describe_resource` on the failing pod. Read the **Events** section and the container `State`/`Last State`.
3. `k8s_get_events` for the namespace to find scheduling, pull, or probe failures.

## 2. Classify
| Symptom | Next step |
|---|---|
| `CrashLoopBackOff`, `Error` | `k8s_get_pod_logs` (also with `previous: true` if supported) for the crashing container |
| `OOMKilled` | Compare limits in `k8s_get_resource_yaml` with usage. Recommend new limits |
| `Pending` | Describe the pod: insufficient CPU/memory, taints, node selectors, or unbound PVCs |
| `ImagePullBackOff` | Check the image name/tag and `imagePullSecrets` in `k8s_get_resource_yaml` |
| Readiness failing | Compare the probe path/port in YAML with the container port. Check logs |
| Service unreachable | `k8s_get_resources` for `endpoints`/`endpointslices`, compare Service selector with pod labels, then `k8s_check_service_connectivity` |

## 3. Remediate (mutating; confirm with the user first)
Only after the user agrees: `k8s_patch_resource`, `k8s_scale`, `k8s_rollout` (restart/undo), or `k8s_apply_manifest`. Show the exact change before applying it. Afterwards, re-run step 1 and report the result.

## Notes
- Mutating tools are absent when the server runs read-only. In that case, provide the patch as YAML for the user to apply.
- Do not use `k8s_delete_resource` to "restart" pods; use `k8s_rollout` restart.
```

- [ ] **Step 2: `plugin/skills/helm-release-management/SKILL.md`**

```markdown
---
name: helm-release-management
description: Inspect, upgrade, roll forward and uninstall Helm releases with the kagent-tools helm_* MCP tools. Use for questions about installed charts, release status, values, failed upgrades, or adding chart repositories.
---
# Helm release management

## Inspect (read-only)
1. `helm_list_releases` (namespace or all namespaces) to find the release and its STATUS (`deployed`, `failed`, `pending-*`).
2. `helm_get_release` for values, manifest, notes, or hooks of a single release.
3. To check what the release created, use `k8s_get_resources` in the release namespace.

## Change (mutating; confirm with the user first)
- Add or refresh chart sources: `helm_repo_add`, then `helm_repo_update`.
- Upgrade or install: `helm_upgrade` with the chart, version, and only the values that change. Restate the chart version and the value diff before running it.
- A release stuck in `pending-upgrade`/`pending-install` usually needs the previous operation to finish or be rolled back. Explain this before acting.
- Remove: `helm_uninstall`, only after explicit confirmation that names the release and namespace.

## Verify
After any change, run `helm_list_releases` and `k8s_get_resources` for the workload pods. Report the new revision and pod health.
```

- [ ] **Step 3: `plugin/skills/istio-traffic-debugging/SKILL.md`**

```markdown
---
name: istio-traffic-debugging
description: Debug Istio service-mesh traffic (503s, mTLS failures, routing not applied, sidecar or ambient/waypoint issues) using the kagent-tools istio_* MCP tools. Use when requests between mesh services fail or routing rules do not take effect.
---
# Istio traffic debugging

## 1. Mesh health (read-only)
1. `istio_version` for control-plane and data-plane versions.
2. `istio_proxy_status` to check that every proxy is `SYNCED`. A `STALE` or `NOT SENT` proxy points to a control-plane push problem.
3. `istio_analyze_cluster_configuration` for the namespace. Fix reported errors first.

## 2. Follow one request
1. Identify the source pod and destination service.
2. `istio_proxy_config` on the source pod for `routes`, `clusters`, and `endpoints`. Confirm the destination host and subset exist and endpoints are `HEALTHY`.
3. For ambient mode, use `istio_ztunnel_config`, `istio_list_waypoints`, and `istio_waypoint_status` to confirm the waypoint is attached and programmed.
4. Read the VirtualService, DestinationRule, and AuthorizationPolicy objects with `k8s_get_resource_yaml`.

## 3. Common causes
| Symptom | Likely cause |
|---|---|
| 503 `UF`/`URX` | No healthy endpoints, or a DestinationRule subset with no matching pod labels |
| 503 `NR` | No route: a VirtualService host or gateway mismatch |
| `RBAC: access denied` | AuthorizationPolicy |
| TLS errors | PeerAuthentication or DestinationRule `tls.mode` mismatch |

## 4. Change (mutating; confirm first)
Use `istio_apply_waypoint`/`istio_delete_waypoint`, or `k8s_apply_manifest` for Istio CRs. Show the manifest first. Re-run step 2 to verify.
```

- [ ] **Step 4: `plugin/skills/argo-rollouts-progressive-delivery/SKILL.md`**

```markdown
---
name: argo-rollouts-progressive-delivery
description: Operate Argo Rollouts canary and blue-green releases (status, promote, pause, change image) with the kagent-tools argo_* MCP tools. Use when a Rollout is progressing, paused, degraded, or needs a new image.
---
# Argo Rollouts progressive delivery

## Inspect (read-only)
1. `argo_verify_argo_rollouts_controller_install` and `argo_verify_kubectl_plugin_install` once per cluster.
2. `argo_rollouts_list` in the namespace for phase, step, and weight.
3. If a Rollout is `Degraded`, describe it with `k8s_describe_resource` (`resource_type: rollout`) and check AnalysisRun results with `k8s_get_resources` (`resource_type: analysisrun`).

## Act (mutating; confirm with the user first)
- Start a release: `argo_set_rollout_image` with the container name and the full image reference.
- Hold: `argo_pause_rollout`.
- Advance one step or fully promote: `argo_promote_rollout`. State which applies before running it.
- Gateway API traffic routing: `argo_verify_gateway_plugin` before the first release.

## Verify
After each action, run `argo_rollouts_list` again. Report the step and weight, and stop if analysis fails.
```

- [ ] **Step 5: `plugin/skills/cilium-network-debugging/SKILL.md`**

```markdown
---
name: cilium-network-debugging
description: Diagnose Cilium CNI networking (agent health, endpoint and identity state, network policy drops, services, BGP, cluster mesh) with the kagent-tools cilium_* MCP tools. Use when pods cannot reach each other or services, or a policy blocks traffic unexpectedly.
---
# Cilium network debugging

## 1. Health (read-only)
1. `cilium_status_and_version` for an overall status and version.
2. `cilium_get_daemon_status` on the node that hosts the affected pod.
3. `cilium_list_cluster_nodes` to confirm all nodes are known.

## 2. Endpoint and identity
1. `cilium_get_endpoints_list`, then `cilium_get_endpoint_details` for the source and destination pods. Check that they are in the `ready` state.
2. `cilium_get_identity_details` for both identities. Check that their labels match what the policy selects.
3. `cilium_display_selectors` and `cilium_display_policy_node_information` to see which policies apply.
4. `cilium_validate_cilium_network_policies` to catch invalid policies.

## 3. Services and routing
- `cilium_list_services` / `cilium_get_service_information` for ClusterIP and backend mapping.
- `cilium_list_bgp_peers` / `cilium_list_bgp_routes` for BGP setups.
- `cilium_show_cluster_mesh_status` for multi-cluster setups.

## 4. Change (mutating; confirm first)
Prefer fixing the CiliumNetworkPolicy manifest through `k8s_apply_manifest` over imperative cilium state changes. Never run `cilium_uninstall_cilium` or `cilium_flush_ipsec_state` without explicit, specific confirmation.
```

- [ ] **Step 6: `plugin/skills/kubescape-security-posture/SKILL.md`**

```markdown
---
name: kubescape-security-posture
description: Review cluster security posture with Kubescape results (configuration scans, image vulnerabilities, application profiles, network neighborhoods) using the kagent-tools kubescape_* MCP tools. Use for security reviews, CVE questions, or hardening recommendations.
---
# Kubescape security posture

## 1. Preconditions
`kubescape_check_health` must report the operator and storage as healthy. Otherwise, explain that the Kubescape operator must be installed first.

## 2. Configuration risks
1. `kubescape_list_configuration_scans` in the namespace (or cluster-wide).
2. `kubescape_get_configuration_scan` for a workload. Group failed controls by severity and give the YAML fix for each.

## 3. Vulnerabilities
1. `kubescape_list_vulnerability_manifests` / `kubescape_list_vulnerabilities`, filtered to Critical and High first.
2. `kubescape_get_vulnerability_details` for a CVE: affected package, fixed version, and which workloads use the image.

## 4. Runtime behaviour
`kubescape_list_application_profiles` / `kubescape_get_application_profile` and `kubescape_list_network_neighborhoods` / `kubescape_get_network_neighborhood` show the observed syscalls, processes, and connections. Use them to propose least-privilege seccomp or NetworkPolicies.

Report findings as a prioritised table with severity, resource, issue, and fix. This skill is read-only; apply fixes only through other skills after confirmation.
```

- [ ] **Step 7: `plugin/skills/prometheus-observability/SKILL.md`**

```markdown
---
name: prometheus-observability
description: Answer questions about cluster and application metrics (error rates, latency, saturation, scrape health) with the kagent-tools prometheus_* MCP tools. Use when investigating performance, alerts, or whether a target is scraped. Requires PROMETHEUS_URL to be configured.
---
# Prometheus observability

## 1. Check connectivity
`prometheus_targets_tool` lists scrape targets. If it fails, tell the user that `PROMETHEUS_URL` (and an EgressPolicy for it when running inside a kagent agent) must be configured.

## 2. Discover before querying
`prometheus_label_names_tool` shows the available labels, so queries use real label names.

## 3. Query
- Current value: `prometheus_query_tool`. Example: `sum(rate(http_requests_total{namespace="shop",code=~"5.."}[5m])) by (service)`.
- Trend: `prometheus_query_range_tool` with a range and step (for example, 1h with a 60s step).
- Draft complex PromQL with `prometheus_promql_tool`, then run it.

## 4. Interpret
Use RED for services (rate, errors, duration) and USE for resources (utilisation, saturation, errors). Always state the query you ran and the time window. Correlate spikes with `k8s_get_events` and rollouts.
```

- [ ] **Step 8: Run the plugin tests**

Run: `go test ./cmd -run 'TestPlugin' -v`
Expected: PASS. If a tool name is reported unknown, check the exact name in `pkg/<provider>` `RegisterTools` and fix the SKILL.md. Do not rename tools.

- [ ] **Step 9: Commit**

```bash
git add plugin/skills
git commit -s -m "feat(plugin): add operational skills for each tool provider"
```

---

### Task 6: Dockerfile — dual-use image with plugin roots

**Files:** Modify `Dockerfile` (builder `go build` line and the whole final stage).

- [ ] **Step 1: Strip the server binary**

In the builder stage, change the build command to:

```dockerfile
    CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH} go build -a -trimpath -ldflags "-s -w $LDFLAGS" -o tool-server ./cmd
```

- [ ] **Step 2: Add an assembly stage** (insert before the final `FROM gcr.io/distroless/static:nonroot`)

```dockerfile
### STAGE 3: assemble kagent Agent Plugin roots
FROM tools AS plugin
ARG VERSION=dev
COPY plugin /src/plugin
COPY --from=builder /workspace/tool-server /downloads/kagent-tools
RUN set -eu; \
    root=/out/opt/kagent-tools; skills_root=/out/opt/kagent-tools-skills; \
    plugin_version="${VERSION#v}"; \
    mkdir -p "$root/bin" "$root/config" "$skills_root" /out/usr/bin; \
    for binary in kagent-tools kubectl helm istioctl kubectl-argo-rollouts cilium; do \
      install -m 0755 "/downloads/$binary" "$root/bin/$binary"; \
    done; \
    sed "s/__VERSION__/${plugin_version}/" /src/plugin/plugin.json.tmpl > "$root/plugin.json"; \
    sed "s/__VERSION__/${plugin_version}/" /src/plugin/skills-plugin.json.tmpl > "$skills_root/plugin.json"; \
    install -m 0644 /src/plugin/mcp.json "$root/mcp.json"; \
    install -m 0644 /src/plugin/config/kubeconfig "$root/config/kubeconfig"; \
    cp -R /src/plugin/skills "$root/skills"; \
    cp -R /src/plugin/skills "$skills_root/skills"; \
    find /out/opt -type d -exec chmod 0755 {} +; \
    find /out/opt -type f ! -path '*/bin/*' -exec chmod 0644 {} +; \
    ln -s /opt/kagent-tools/bin/kagent-tools /out/tool-server; \
    for binary in kubectl helm istioctl kubectl-argo-rollouts cilium; do \
      ln -s "/opt/kagent-tools/bin/$binary" "/out/usr/bin/$binary"; \
    done; \
    chown -R 65532:65532 /out/opt
```

- [ ] **Step 3: Replace the final stage**

```dockerfile
FROM gcr.io/distroless/static:nonroot

WORKDIR /
USER 65532:65532
ENV PATH=/opt/kagent-tools/bin:$PATH:/bin

# /opt/kagent-tools        -> kagent Agent Plugin root (CLIs + stdio MCP server + skills)
# /opt/kagent-tools-skills -> skills-only Agent Plugin root
# /tool-server, /bin/*     -> backward-compatible symlinks (distroless /bin -> usr/bin)
COPY --from=plugin /out/ /

ARG VERSION

LABEL org.opencontainers.image.source=https://github.com/kagent-dev/tools
LABEL org.opencontainers.image.description="Kagent MCP tools server and kagent Agent Plugin (skills bundle)"
LABEL org.opencontainers.image.authors="Kagent Creators 🤖"
LABEL org.opencontainers.image.version="$VERSION"

ENTRYPOINT ["/opt/kagent-tools/bin/kagent-tools"]
```

The `tools` stage already downloads `kubectl`, `helm`, `istioctl`, `kubectl-argo-rollouts`, and `cilium` into `/downloads`. Wolfi provides `install`, `sed`, `find`, and `ln`.

- [ ] **Step 4: Build and check basic behaviour**

Run: `make docker-build`
Expected: the build succeeds.

Run: `IMG=$(make -s -n docker-build | grep -oE -- '-t [^ ]+' | head -1 | cut -d' ' -f2); docker run --rm "$IMG" --version`
Expected: the version block is printed.

Run: `docker run --rm --entrypoint /tool-server "$IMG" --version`
Expected: the same output, so the legacy path works.

- [ ] **Step 5: Commit**

```bash
git add Dockerfile
git commit -s -m "feat(image): lay out image as kagent Agent Plugin with legacy symlinks"
```

---

### Task 7: Image verification (layout, limits, stdio smoke) + `make plugin-verify`

**Files:**
- Create: `cmd/plugin_image_test.go`
- Create: `scripts/verify-plugin-image.sh`
- Modify: `Makefile` (new target after `docker-build`)

- [ ] **Step 1: Create `cmd/plugin_image_test.go`** (skipped unless `KAGENT_TOOLS_IMAGE_ROOT` is set)

```go
package main

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

const (
	kagentMaxPackageEntries = 10_000
	kagentMaxPackageBytes   = 100 << 20
)

type packageStats struct {
	Entries int64
	Bytes   int64
}

// walkPluginPackage mirrors kagent's validatePackage checks for one plugin root.
func walkPluginPackage(t *testing.T, root string) packageStats {
	t.Helper()
	var stats packageStats
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		require.NoError(t, err)
		stats.Entries++
		info, err := entry.Info()
		require.NoError(t, err)
		mode := info.Mode()
		require.Truef(t, mode.IsRegular() || mode.IsDir() || mode&os.ModeSymlink != 0, "unsupported file %s (%s)", path, mode)
		if mode.IsRegular() {
			stats.Bytes += info.Size()
		}
		if mode&os.ModeSymlink != 0 {
			resolved, err := filepath.EvalSymlinks(path)
			require.NoError(t, err)
			require.Truef(t, strings.HasPrefix(resolved, root+string(filepath.Separator)), "symlink %s escapes %s", path, root)
		}
		return nil
	})
	require.NoError(t, err)
	return stats
}

func TestPluginImageLayout(t *testing.T) {
	imageRoot := os.Getenv("KAGENT_TOOLS_IMAGE_ROOT")
	if imageRoot == "" {
		t.Skip("set KAGENT_TOOLS_IMAGE_ROOT to an exported image filesystem (scripts/verify-plugin-image.sh)")
	}

	fullRoot := filepath.Join(imageRoot, "opt", "kagent-tools")
	for _, binary := range []string{"kagent-tools", "kubectl", "helm", "istioctl", "kubectl-argo-rollouts", "cilium"} {
		info, err := os.Stat(filepath.Join(fullRoot, "bin", binary))
		require.NoError(t, err, binary)
		require.NotZero(t, info.Mode().Perm()&0o111, "%s must be executable", binary)
	}
	for _, file := range []string{"plugin.json", "mcp.json", "config/kubeconfig"} {
		require.FileExists(t, filepath.Join(fullRoot, file))
	}
	fullStats := walkPluginPackage(t, fullRoot)
	require.Less(t, fullStats.Entries, int64(kagentMaxPackageEntries))
	t.Logf("full plugin root: %d entries, %d MiB (requires kagent mounted-package size exemption)", fullStats.Entries, fullStats.Bytes>>20)

	skillsRoot := filepath.Join(imageRoot, "opt", "kagent-tools-skills")
	require.FileExists(t, filepath.Join(skillsRoot, "plugin.json"))
	require.NoFileExists(t, filepath.Join(skillsRoot, "mcp.json"))
	skillsStats := walkPluginPackage(t, skillsRoot)
	require.Less(t, skillsStats.Entries, int64(kagentMaxPackageEntries))
	require.Less(t, skillsStats.Bytes, int64(kagentMaxPackageBytes))

	fullSkills, err := os.ReadDir(filepath.Join(fullRoot, "skills"))
	require.NoError(t, err)
	onlySkills, err := os.ReadDir(filepath.Join(skillsRoot, "skills"))
	require.NoError(t, err)
	require.Equal(t, len(fullSkills), len(onlySkills))

	target, err := os.Readlink(filepath.Join(imageRoot, "tool-server"))
	require.NoError(t, err)
	require.Equal(t, "/opt/kagent-tools/bin/kagent-tools", target)
}
```

- [ ] **Step 2: Create `scripts/verify-plugin-image.sh`** (`chmod +x`)

```bash
#!/usr/bin/env bash
# Verifies that a built kagent-tools image is a valid kagent Agent Plugin.
set -euo pipefail

image="${1:?usage: $0 <image>}"
workdir="$(mktemp -d)"
container=""
cleanup() {
  [[ -n "$container" ]] && docker rm -f "$container" >/dev/null 2>&1 || true
  rm -rf "$workdir"
}
trap cleanup EXIT

container="$(docker create "$image")"
docker export "$container" | tar -x -C "$workdir"

KAGENT_TOOLS_IMAGE_ROOT="$workdir" go test ./cmd -run TestPluginImageLayout -count=1 -v

echo "--- stdio smoke test"
# Keep stdin open briefly: the server exits on EOF before answering.
response="$({ printf '%s\n' \
  '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"smoke","version":"0"}}}' \
  '{"jsonrpc":"2.0","method":"notifications/initialized"}' \
  '{"jsonrpc":"2.0","id":2,"method":"tools/list"}'; sleep 5; } \
  | docker run -i --rm --entrypoint /opt/kagent-tools/bin/kagent-tools "$image" --stdio 2>/dev/null)"

for tool in k8s_get_resources helm_list_releases istio_proxy_status argo_rollouts_list cilium_status_and_version; do
  grep -q "\"name\":\"$tool\"" <<<"$response" || { echo "missing tool $tool in stdio tools/list"; exit 1; }
done
if grep -qv '^{"jsonrpc"' <<<"$response"; then
  echo "non JSON-RPC output on stdout:"; grep -v '^{"jsonrpc"' <<<"$response" | head -5; exit 1
fi
echo "plugin image OK: $image"
```

- [ ] **Step 3: Add the Makefile target** (after the `docker-build` target)

```makefile
.PHONY: plugin-verify
plugin-verify: docker-build ## Verify the image is a valid kagent Agent Plugin (layout, limits, stdio)
	./scripts/verify-plugin-image.sh $(TOOLS_IMG)
```

- [ ] **Step 4: Run it**

Run: `make plugin-verify`
Expected: `TestPluginImageLayout` PASS, the log line reports ~650 MiB for the full root, and the last line is `plugin image OK: …`.

Run: `go test ./cmd -run TestPluginImageLayout -v`
Expected: SKIP (no env set), so `make test` is unaffected.

- [ ] **Step 5: Commit**

```bash
git add cmd/plugin_image_test.go scripts/verify-plugin-image.sh Makefile
git commit -s -m "test(plugin): verify image layout, kagent limits and stdio protocol"
```

---

### Task 8: CI

**Files:** Modify `.github/workflows/ci.yaml`. The existing `build` job exports multi-arch output to `/dev/null`, so add a separate single-arch job that loads the image.

- [ ] **Step 1: Add the job** (after `go-unit-tests`)

```yaml
  plugin-verify:
    env:
      VERSION: v0.0.1-test
    runs-on: ubuntu-latest
    steps:
      - name: Checkout repository
        uses: actions/checkout@v5
      - name: Set up Go
        uses: actions/setup-go@v6
        with:
          go-version-file: 'go.mod'
          cache: false
      - name: Set up Docker Buildx
        uses: docker/setup-buildx-action@v3
        with:
          version: v0.23.0
          use: 'true'
      - name: Verify kagent Agent Plugin image
        env:
          DOCKER_BUILDKIT: 1
          BUILDX_BUILDER_NAME: kagent-builder-v0.23.0
          DOCKER_BUILD_ARGS: >-
            --cache-from=type=gha
            --load
            --platform=linux/amd64
          DOCKER_REPO: "${{ github.repository_owner }}/tools"
          DOCKER_BUILDER: "docker buildx"
        run: make plugin-verify
```

- [ ] **Step 2: Validate the workflow syntax**

Run: `docker run --rm -v "$PWD:/repo" -w /repo rhysd/actionlint:latest .github/workflows/ci.yaml`
Expected: no errors.

- [ ] **Step 3: Commit**

```bash
git add .github/workflows/ci.yaml
git commit -s -m "ci: verify kagent Agent Plugin image layout"
```

---

### Task 9: Documentation

**Files:**
- Create: `docs/kagent-skills.md`
- Modify: `README.md` (add a "Use as kagent skills bundle" link in the usage section)
- Modify: `AGENTS.md` (add `plugin/` to Repository Structure; add a "Plugin bundle" note under Adding a New Tool: "new tool providers should add or extend a skill in `plugin/skills` and keep `cmd/plugin_bundle_test.go` passing")

- [ ] **Step 1: Write `docs/kagent-skills.md`** with these sections, in this order:
  1. **What you get:** the two plugin roots (`opt/kagent-tools`, `opt/kagent-tools-skills`) and the 7 skills with one-line descriptions (copy from the frontmatter).
  2. **Requirements:** kagent ≥ the release that contains the Task 0 change, the Go ADK (`kagent`) harness, the image pinned by digest (`docker buildx imagetools inspect ghcr.io/kagent-dev/kagent/tools:<tag>` prints it).
  3. **Cluster access:** the ServiceAccount, token Secret, ClusterRoleBinding (`view` example), and EgressPolicy manifests from spec §4.4, verbatim.
  4. **Agent:** the AgentTemplate + Agent manifests from spec §4.4, verbatim.
  5. **Tuning via Harness env:** `KAGENT_TOOLS_READ_ONLY=true`, `KAGENT_TOOLS_TOOLS=k8s,helm`, `PROMETHEUS_URL=…` (plus an EgressPolicy origin for it), and `OTEL_EXPORTER_OTLP_ENDPOINT`.
  6. **Skills-only mode (current kagent):** an AgentTemplate with `path: opt/kagent-tools-skills` plus a `RemoteMCPServer` pointing at the Helm-deployed server. Warn: never select both roots in one agent.
  7. **Troubleshooting:** `artifact exceeds 104857600 bytes` means kagent lacks the Task 0 change, so use skills-only mode. A 401/403 from tools means the EgressPolicy/Secret/RBAC is wrong. A connection refused or blocked error means the EgressPolicy is missing from `egressPolicyRefs`. `exec`/streamed logs are best effort through the egress gateway. "Plugin is Claude-format" cannot happen with this image, because `plugin.json` sits at the root.

- [ ] **Step 2: Update the README and AGENTS.md** as described in **Files** above.

- [ ] **Step 3: Run the full repo checks**

Run: `make fmt && make lint && make test`
Expected: all pass. `go test -cover ./cmd` must not lower coverage, because Tasks 1, 4, and 7 add tests.

- [ ] **Step 4: Commit**

```bash
git add docs/kagent-skills.md README.md AGENTS.md
git commit -s -m "docs: using kagent-tools as a kagent 1.x skills bundle"
```

---

### Task 10 (opt-in): E2E on kagent 1.x

**Files:** Create `test/e2e/kagent_plugin/manifests.yaml` (spec §4.4 manifests with `IMAGE_DIGEST` substituted by `envsubst`) and `test/e2e/kagent_plugin/run.sh`.

- [ ] **Step 1: Write `run.sh`**, which:
  (a) requires `KAGENT_V1_E2E=1`, `KUBECONFIG` pointing to a Kind cluster with kagent ≥ the Task 0 release, and `IMAGE` (pushed, digest-pinned);
  (b) runs `envsubst < manifests.yaml | kubectl apply -f -`;
  (c) waits with `kubectl wait --for=condition=Ready agent/k8s-operator -n kagent --timeout=300s`;
  (d) invokes the agent through the kagent CLI or A2A endpoint with the prompt "Use list_skills, then list pods in kube-system using your tools" and greps the response for `k8s-troubleshooting` and `coredns`;
  (e) best effort: asks for the logs of one coredns pod, and prints a WARNING instead of failing when it gets no output.

- [ ] **Step 2: Run it against a local Kind + kagent install**, record the results (including the streaming behaviour) in `docs/kagent-skills.md` § Troubleshooting, and commit.

```bash
git add test/e2e/kagent_plugin docs/kagent-skills.md
git commit -s -m "test(e2e): opt-in kagent 1.x plugin bundle scenario"
```

---

## Spec coverage check

| Spec section | Task |
|---|---|
| §4.1 layout, symlinks, both roots | 6, 7 |
| §4.2 mcp.json | 3, 4 |
| §4.3 env fallbacks, kubeconfig chain, KUBECONFIG export, stdio stderr fix | 1, 2 |
| §4.4 kubeconfig + RBAC/EgressPolicy docs | 3, 9 |
| §4.5 skills + tool-name validation | 4, 5 |
| §5 behaviour, fallback, troubleshooting | 9 |
| §6 upstream change | 0 |
| §7 testing layers | 1, 2, 4, 7, 8, 10 |
