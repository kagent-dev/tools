#!/bin/bash
set -e

# Start the server
echo "Starting server on port 18201..."
./bin/kagent-tools --http-port 18201 --tools utils,k8s,helm &
SERVER_PID=$!
echo "Server PID: $SERVER_PID"

# Wait for server to start
sleep 5

# Test health endpoint
echo "Testing health endpoint..."
curl -s http://localhost:18201/health
echo ""
echo "---"

# Test MCP client connection and tools list
echo "Testing MCP tools list..."
if [ -f ./bin/go-mcp-client ]; then
    ./bin/go-mcp-client --server http://localhost:18201/mcp list-tools
else
    echo "go-mcp-client not found, trying to build it..."
    go build -o ./bin/go-mcp-client ./cmd/client
    ./bin/go-mcp-client --server http://localhost:18201/mcp list-tools
fi

echo "---"
echo "Test complete!"

# Stop the server
kill $SERVER_PID 2>/dev/null || true
wait $SERVER_PID 2>/dev/null || true

