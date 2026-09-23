package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/require"
)

const initializeRequestBody = `{"jsonrpc":"2.0","id":1,"method":"initialize",` +
	`"params":{"protocolVersion":"2025-03-26","capabilities":{},` +
	`"clientInfo":{"name":"lifecycle-test","version":"1"}}}`

const listToolsRequestBody = `{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}`

// newTestTransport starts the streamable HTTP transport wired exactly as run()
// wires it, and returns the server plus a live HTTP endpoint.
func newTestTransport(t *testing.T, idleTTL time.Duration) (*sdkmcp.Server, *httptest.Server) {
	t.Helper()

	srv := sdkmcp.NewServer(&sdkmcp.Implementation{Name: "lifecycle", Version: "test"}, nil)
	httpServer := httptest.NewServer(newStreamableHTTPHandler(srv, idleTTL))
	t.Cleanup(httpServer.Close)

	return srv, httpServer
}

// postJSON sends an MCP POST, optionally resuming an existing session.
func postJSON(t *testing.T, httpServer *httptest.Server, body, sessionID string) *http.Response {
	t.Helper()

	req, err := http.NewRequest(http.MethodPost, httpServer.URL, strings.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	if sessionID != "" {
		req.Header.Set("Mcp-Session-Id", sessionID)
	}

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	t.Cleanup(func() { _ = resp.Body.Close() })

	return resp
}

// registeredSessions counts the sessions the server still holds. This is the
// state that leaks when a session is never released.
func registeredSessions(srv *sdkmcp.Server) int {
	total := 0
	for range srv.Sessions() {
		total++
	}
	return total
}

// TestStreamableHTTPReclaimsIdleSessions is the regression guard for the session
// leak: a client that never sends DELETE must not retain its session forever.
// Before SessionTimeout was wired up, every abandoned session stayed registered
// for the lifetime of the process and heap grew with the number of sessions
// ever created.
func TestStreamableHTTPReclaimsIdleSessions(t *testing.T) {
	const sessions = 25
	const idleTTL = 100 * time.Millisecond

	srv, httpServer := newTestTransport(t, idleTTL)

	for i := 0; i < sessions; i++ {
		resp := postJSON(t, httpServer, initializeRequestBody, "")
		require.Equal(t, http.StatusOK, resp.StatusCode)
	}
	require.Equal(t, sessions, registeredSessions(srv),
		"each initialize should register a session")

	require.Eventually(t, func() bool {
		return registeredSessions(srv) == 0
	}, 5*time.Second, 20*time.Millisecond,
		"idle sessions must be reclaimed instead of leaking")
}

// TestStreamableHTTPKeepsActiveSessionAlive guards against the opposite failure:
// the idle reaper must not evict a client that is still making requests. The
// timer is reset by each request, so traffic spaced within the TTL must keep the
// session usable.
func TestStreamableHTTPKeepsActiveSessionAlive(t *testing.T) {
	const idleTTL = 200 * time.Millisecond

	srv, httpServer := newTestTransport(t, idleTTL)

	resp := postJSON(t, httpServer, initializeRequestBody, "")
	require.Equal(t, http.StatusOK, resp.StatusCode)
	sessionID := resp.Header.Get("Mcp-Session-Id")
	require.NotEmpty(t, sessionID, "server should assign a session id")

	// Keep the session busy across four TTL windows. Each request lands well
	// inside the TTL, so the session must survive every one of them.
	for i := 0; i < 4; i++ {
		time.Sleep(idleTTL / 2)
		resp := postJSON(t, httpServer, listToolsRequestBody, sessionID)
		require.Equalf(t, http.StatusOK, resp.StatusCode,
			"active client was evicted on request %d", i+1)
	}

	require.Equal(t, 1, registeredSessions(srv))
}

// TestStreamableHTTPReleasesSessionOnDelete covers the orderly path: a client
// that terminates its session must have all of its state released immediately,
// without waiting for the idle TTL.
func TestStreamableHTTPReleasesSessionOnDelete(t *testing.T) {
	// A long TTL proves DELETE is what freed the session, not the reaper.
	srv, httpServer := newTestTransport(t, time.Hour)

	resp := postJSON(t, httpServer, initializeRequestBody, "")
	require.Equal(t, http.StatusOK, resp.StatusCode)
	sessionID := resp.Header.Get("Mcp-Session-Id")
	require.NotEmpty(t, sessionID)
	require.Equal(t, 1, registeredSessions(srv))

	deleteReq, err := http.NewRequest(http.MethodDelete, httpServer.URL, nil)
	require.NoError(t, err)
	deleteReq.Header.Set("Mcp-Session-Id", sessionID)

	deleteResp, err := http.DefaultClient.Do(deleteReq)
	require.NoError(t, err)
	defer func() { _ = deleteResp.Body.Close() }()
	_, _ = io.Copy(io.Discard, deleteResp.Body)

	require.Equal(t, http.StatusNoContent, deleteResp.StatusCode)
	require.Equal(t, 0, registeredSessions(srv),
		"DELETE must release the session immediately")
}
