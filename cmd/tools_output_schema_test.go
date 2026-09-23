package main

import (
	"context"
	"encoding/json"
	"testing"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/require"
)

// TestEveryToolHasValidOutputSchema is the wire-level guard for the typed-output
// migration: registering every provider tool must produce a resolvable output
// schema for each tool whose Out type is not `any`. AddTool panics when schema
// inference or resolution fails (for example a third-party k8s type with a
// custom JSON marshaller), so a panic here means the server would not start.
func TestEveryToolHasValidOutputSchema(t *testing.T) {
	ctx := context.Background()

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

		// The advertised schema must survive a JSON round-trip (it is sent on the
		// wire as part of tools/list).
		_, err := json.Marshal(tool.OutputSchema)
		require.NoErrorf(t, err, "tool %q output schema is not JSON-serializable", tool.Name)
		checked++
	}
	require.NotEmpty(t, checked, "expected the server to advertise tools")
}
