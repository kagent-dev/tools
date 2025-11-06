package mcp

import (
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewHTTPTransportValidation(t *testing.T) {
	server := mcp.NewServer(&mcp.Implementation{Name: "test"}, nil)

	tests := []struct {
		name    string
		server  *mcp.Server
		cfg     HTTPTransportConfig
		wantErr string
	}{
		{
			name:    "nil server",
			server:  nil,
			cfg:     HTTPTransportConfig{Port: 8080},
			wantErr: "mcp server must not be nil",
		},
		{
			name:    "invalid port",
			server:  server,
			cfg:     HTTPTransportConfig{Port: -1},
			wantErr: "invalid port",
		},
		{
			name:   "negative write timeout",
			server: server,
			cfg: HTTPTransportConfig{
				Port:         8080,
				WriteTimeout: -1,
			},
			wantErr: "write timeout",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewHTTPTransport(tt.server, tt.cfg)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}

	t.Run("defaults are applied", func(t *testing.T) {
		transport, err := NewHTTPTransport(server, HTTPTransportConfig{Port: 8080})
		require.NoError(t, err)

		assert.Equal(t, 8080, transport.configuredPort)
		assert.Equal(t, defaultReadTimeout, transport.readTimeout)
		assert.Equal(t, defaultReadTimeout, transport.idleTimeout)
		assert.Equal(t, defaultShutdownTimeout, transport.shutdownTimeout)
	})
}

func TestHTTPTransportStartStop(t *testing.T) {
	server := mcp.NewServer(&mcp.Implementation{Name: "test-start-stop"}, nil)

	transport, err := NewHTTPTransport(server, HTTPTransportConfig{
		Port:            0,
		ReadTimeout:     2 * time.Second,
		WriteTimeout:    0,
		ShutdownTimeout: 2 * time.Second,
	})
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	require.NoError(t, transport.Start(ctx))

	t.Cleanup(func() {
		stopCtx, stopCancel := context.WithTimeout(context.Background(), time.Second)
		defer stopCancel()
		_ = transport.Stop(stopCtx)
	})

	require.True(t, transport.IsRunning())

	require.Eventually(t, func() bool {
		if transport.port == 0 {
			return false
		}
		resp, err := http.Get(fmt.Sprintf("http://127.0.0.1:%d/health", transport.port))
		if err != nil {
			return false
		}
		defer resp.Body.Close()
		return resp.StatusCode == http.StatusOK
	}, 2*time.Second, 50*time.Millisecond)

	err = transport.Start(ctx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "already running")

	stopCtx, stopCancel := context.WithTimeout(context.Background(), time.Second)
	defer stopCancel()
	require.NoError(t, transport.Stop(stopCtx))
	require.False(t, transport.IsRunning())
}
