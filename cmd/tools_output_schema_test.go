package main

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/require"
)

// TestEveryToolHasValidOutputSchema is the wire-level guard for the typed-output
// migration: registering every provider tool must produce a resolvable output
// schema for each tool whose Out type is not `any`. AddTool panics when schema
// inference or resolution fails (for example a third-party k8s type with a
// custom JSON marshaller), so a panic here means the server would not start.
func TestEveryToolHasValidOutputSchema(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	srv := sdkmcp.NewServer(&sdkmcp.Implementation{Name: "schema", Version: "test"}, nil)

	require.NotPanics(t, func() {
		registerMCP(srv, nil, "", false)
	}, "registerMCP must not panic: every Out type must infer a valid output schema")

	serverT, clientT := sdkmcp.NewInMemoryTransports()
	go func() { _ = srv.Run(ctx, serverT) }()

	client := sdkmcp.NewClient(&sdkmcp.Implementation{Name: "schema-client", Version: "test"}, nil)
	session, err := client.Connect(ctx, clientT, nil)
	require.NoError(t, err)
	defer func() { _ = session.Close() }()

	checked := 0
	for tool, err := range session.Tools(ctx, nil) {
		require.NoError(t, err)
		require.NotEmpty(t, tool.Name)

		// Every migrated tool carries an inferred output schema: Out is a concrete
		// type rather than `any`. Tools left with Out=any would have none.
		require.NotNilf(t, tool.OutputSchema,
			"tool %q has no output schema; its handler still returns Out=any", tool.Name)

		// Clients implementing the MCP 2025-06-18 Tool definition require
		// object roots. A single array output rejects the whole tools/list
		// response even when the SDK can infer and serialize that schema.
		t.Run(tool.Name, func(t *testing.T) {
			inputSchema, err := json.Marshal(tool.InputSchema)
			require.NoError(t, err)
			outputSchema, err := json.Marshal(tool.OutputSchema)
			require.NoError(t, err)
			for _, schema := range []struct {
				name  string
				value []byte
			}{
				{name: "inputSchema", value: inputSchema},
				{name: "outputSchema", value: outputSchema},
			} {
				var root struct {
					Type string `json:"type"`
				}
				require.NoErrorf(t, json.Unmarshal(schema.value, &root),
					"%s must have a single object root: %s", schema.name, schema.value)
				require.Equal(t, "object", root.Type, schema.name)
			}
		})
		checked++
	}
	require.NotEmpty(t, checked, "expected the server to advertise tools")
}
