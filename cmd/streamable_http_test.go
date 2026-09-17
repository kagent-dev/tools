package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/mark3labs/mcp-go/server"
)

const initializeRequest = `{"jsonrpc":"2.0","id":1,"method":"initialize",` +
	`"params":{"protocolVersion":"2025-03-26","capabilities":{},` +
	`"clientInfo":{"name":"test","version":"1"}}}`

// sessionRecorder counts the sessions the MCP server registers and releases.
// A session that is registered but never unregistered is retained state, which
// is what leaks when a client never sends DELETE.
type sessionRecorder struct {
	mu           sync.Mutex
	registered   int
	unregistered int
}

func (r *sessionRecorder) hooks() *server.Hooks {
	hooks := &server.Hooks{}
	hooks.AddOnRegisterSession(func(_ context.Context, _ server.ClientSession) {
		r.mu.Lock()
		defer r.mu.Unlock()
		r.registered++
	})
	hooks.AddOnUnregisterSession(func(_ context.Context, _ server.ClientSession) {
		r.mu.Lock()
		defer r.mu.Unlock()
		r.unregistered++
	})
	return hooks
}

func (r *sessionRecorder) counts() (int, int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.registered, r.unregistered
}

// newRecordedServer starts the streamable HTTP transport wired exactly as the
// server wires it, with session hooks attached.
func newRecordedServer(t *testing.T, idleTTL time.Duration) (*httptest.Server, *sessionRecorder) {
	t.Helper()

	recorder := &sessionRecorder{}
	mcpServer := server.NewMCPServer("test-server", "test", server.WithHooks(recorder.hooks()))
	streamableServer := newStreamableHTTPServer(mcpServer, idleTTL)
	t.Cleanup(func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = streamableServer.Shutdown(shutdownCtx)
	})

	httpServer := httptest.NewServer(streamableServer)
	t.Cleanup(httpServer.Close)
	return httpServer, recorder
}

// initializeSession sends an initialize request and returns the session ID the
// server assigned to it.
func initializeSession(t *testing.T, httpServer *httptest.Server) string {
	t.Helper()

	req, err := http.NewRequest(http.MethodPost, httpServer.URL, strings.NewReader(initializeRequest))
	if err != nil {
		t.Fatalf("build initialize request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")

	resp, err := httpServer.Client().Do(req)
	if err != nil {
		t.Fatalf("send initialize request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("initialize returned status %d, want %d", resp.StatusCode, http.StatusOK)
	}
	sessionID := resp.Header.Get("Mcp-Session-Id")
	if sessionID == "" {
		t.Fatal("initialize response carried no Mcp-Session-Id header")
	}
	return sessionID
}

// waitForUnregistered polls until the expected number of sessions has been
// released, so the sweeper's own tick interval does not make the test flaky.
func waitForUnregistered(t *testing.T, recorder *sessionRecorder, want int, timeout time.Duration) {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for {
		_, unregistered := recorder.counts()
		if unregistered >= want {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("released %d sessions after %s, want %d", unregistered, timeout, want)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// A client that ends its session explicitly must have it released.
func TestStreamableHTTPServerReleasesSessionOnDelete(t *testing.T) {
	httpServer, recorder := newRecordedServer(t, 0)
	sessionID := initializeSession(t, httpServer)

	if registered, _ := recorder.counts(); registered != 1 {
		t.Fatalf("registered %d sessions, want 1", registered)
	}

	req, err := http.NewRequest(http.MethodDelete, httpServer.URL, nil)
	if err != nil {
		t.Fatalf("build delete request: %v", err)
	}
	req.Header.Set("Mcp-Session-Id", sessionID)

	resp, err := httpServer.Client().Do(req)
	if err != nil {
		t.Fatalf("send delete request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("delete returned status %d, want %d", resp.StatusCode, http.StatusOK)
	}
	waitForUnregistered(t, recorder, 1, 2*time.Second)
}

// A client that goes away without a DELETE must not retain its session: the
// idle sweeper is what bounds memory for POST-only clients.
func TestStreamableHTTPServerSweepsIdleSession(t *testing.T) {
	httpServer, recorder := newRecordedServer(t, 100*time.Millisecond)
	initializeSession(t, httpServer)

	if registered, _ := recorder.counts(); registered != 1 {
		t.Fatalf("registered %d sessions, want 1", registered)
	}
	waitForUnregistered(t, recorder, 1, 10*time.Second)
}
