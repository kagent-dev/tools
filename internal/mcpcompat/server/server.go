package server

import (
	"context"
	"io"
	"net/http"
	"time"

	compat "github.com/kagent-dev/tools/internal/mcpcompat"
	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

type MCPServer struct {
	inner *sdkmcp.Server
}

type ToolHandlerFunc = compat.ToolHandlerFunc

func NewMCPServer(name, version string) *MCPServer {
	return &MCPServer{inner: sdkmcp.NewServer(&sdkmcp.Implementation{Name: name, Version: version}, nil)}
}

func (s *MCPServer) AddTool(t compat.Tool, h ToolHandlerFunc) {
	compat.RegisterLegacyTool(s.inner, t, h)
}

func (s *MCPServer) Inner() *sdkmcp.Server { return s.inner }

func (s *MCPServer) Run(ctx context.Context, t sdkmcp.Transport) error {
	return s.inner.Run(ctx, t)
}

type StreamableHTTPOption func(*sdkmcp.StreamableHTTPOptions)

func WithHeartbeatInterval(_ time.Duration) StreamableHTTPOption {
	return func(_ *sdkmcp.StreamableHTTPOptions) {}
}

func NewStreamableHTTPServer(s *MCPServer, opts ...StreamableHTTPOption) http.Handler {
	o := &sdkmcp.StreamableHTTPOptions{}
	for _, opt := range opts {
		opt(o)
	}
	return sdkmcp.NewStreamableHTTPHandler(func(_ *http.Request) *sdkmcp.Server { return s.inner }, o)
}

type StdioServer struct{ s *MCPServer }

func NewStdioServer(s *MCPServer) *StdioServer { return &StdioServer{s: s} }

func (ss *StdioServer) Listen(ctx context.Context, in io.Reader, out io.Writer) error {
	reader, ok := in.(io.ReadCloser)
	if !ok {
		reader = io.NopCloser(in)
	}
	writer, ok := out.(io.WriteCloser)
	if !ok {
		writer = nopWriteCloser{out}
	}
	return ss.s.inner.Run(ctx, &sdkmcp.IOTransport{Reader: reader, Writer: writer})
}

type nopWriteCloser struct{ io.Writer }

func (nopWriteCloser) Close() error { return nil }
