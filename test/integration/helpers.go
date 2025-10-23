package integration

import (
	"fmt"
	"net/http"
	"net/url"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// createHTTPTransport creates an HTTP transport for MCP communication
// This helper is used by all integration tests that need HTTP/SSE transport
// Implements: T028 - Integration Test Helpers (HTTP transport)
func createHTTPTransport(serverURL string) mcp.Transport {
	// Parse the URL
	parsedURL, err := url.Parse(serverURL)
	if err != nil {
		panic(fmt.Sprintf("invalid server URL: %v", err))
	}

	// Create HTTP client
	httpClient := &http.Client{}

	// Create SSE client transport using the SDK
	// The SDK provides SSEClientTransport for HTTP/SSE communication
	transport := &mcp.SSEClientTransport{
		Endpoint:   parsedURL.String(),
		HTTPClient: httpClient,
	}

	return transport
}

// AssertToolExists checks if a tool with the given name exists in the tools list
// Implements: T028 - Integration Test Helpers (assertion helper)
func AssertToolExists(tools []*mcp.Tool, name string) bool {
	for _, tool := range tools {
		if tool.Name == name {
			return true
		}
	}
	return false
}

// getBinaryName returns the platform-specific binary name
// Implements: T028 - Integration Test Helpers (binary resolution)
func getBinaryName() string {
	return "../bin/kagent-tools"
}
