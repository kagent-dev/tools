package main

import (
	"bufio"
	"context"
	"os"
	"sort"
	"strings"
	"testing"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// registeredToolNames spins up an in-process MCP server with every provider
// registered (readOnly=false so mutating tools are included too), connects an
// in-memory client, and returns the set of advertised tool names — the same
// list a real MCP client would see over the wire.
func registeredToolNames(t *testing.T) map[string]bool {
	t.Helper()
	ctx := context.Background()

	srv := sdkmcp.NewServer(&sdkmcp.Implementation{Name: "regression", Version: "test"}, nil)
	registerMCP(srv, nil, "", false) // nil providers => register them all

	serverT, clientT := sdkmcp.NewInMemoryTransports()
	go func() { _ = srv.Run(ctx, serverT) }()

	client := sdkmcp.NewClient(&sdkmcp.Implementation{Name: "regression-client", Version: "test"}, nil)
	session, err := client.Connect(ctx, clientT, nil)
	require.NoError(t, err)
	defer func() { _ = session.Close() }()

	names := make(map[string]bool)
	for tool, err := range session.Tools(ctx, nil) {
		require.NoError(t, err)
		names[tool.Name] = true
	}
	require.NotEmpty(t, names, "expected the server to advertise tools")
	return names
}

// readGoldenToolNames loads the committed list of tool names, ignoring blank
// lines and '#' comments.
func readGoldenToolNames(t *testing.T, path string) []string {
	t.Helper()
	f, err := os.Open(path)
	require.NoError(t, err)
	defer func() { _ = f.Close() }()

	var names []string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		names = append(names, line)
	}
	require.NoError(t, sc.Err())
	return names
}

// TestNoToolNameRegressions guards the go-sdk migration: every tool name shipped
// in the v0.2.1 release must still be registered under the same name. New tools
// are allowed; renames or removals are caught here.
func TestNoToolNameRegressions(t *testing.T) {
	current := registeredToolNames(t)
	old := readGoldenToolNames(t, "testdata/tool_names_v0.2.1.txt")

	var missing []string
	for _, name := range old {
		if !current[name] {
			missing = append(missing, name)
		}
	}
	sort.Strings(missing)

	assert.Emptyf(t, missing, "%d tool(s) from v0.2.1 are missing/renamed in the current build: %v", len(missing), missing)
}
