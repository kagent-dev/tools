package mcp

import (
	"sync"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// ToolRegistry is a shared registry for tool handlers
type ToolRegistry struct {
	mu       sync.RWMutex
	handlers map[string]mcp.ToolHandler
	tools    map[string]*mcp.Tool
}

// NewToolRegistry creates a new tool registry
func NewToolRegistry() *ToolRegistry {
	return &ToolRegistry{
		handlers: make(map[string]mcp.ToolHandler),
		tools:    make(map[string]*mcp.Tool),
	}
}

// Register registers a tool and its handler
func (tr *ToolRegistry) Register(tool *mcp.Tool, handler mcp.ToolHandler) {
	tr.mu.Lock()
	defer tr.mu.Unlock()
	tr.handlers[tool.Name] = handler
	tr.tools[tool.Name] = tool
}

// GetHandler returns a tool handler by name
func (tr *ToolRegistry) GetHandler(name string) (mcp.ToolHandler, bool) {
	tr.mu.RLock()
	defer tr.mu.RUnlock()
	handler, ok := tr.handlers[name]
	return handler, ok
}

// GetTool returns a tool by name
func (tr *ToolRegistry) GetTool(name string) (*mcp.Tool, bool) {
	tr.mu.RLock()
	defer tr.mu.RUnlock()
	tool, ok := tr.tools[name]
	return tool, ok
}

// ListTools returns all registered tools
func (tr *ToolRegistry) ListTools() []*mcp.Tool {
	tr.mu.RLock()
	defer tr.mu.RUnlock()
	tools := make([]*mcp.Tool, 0, len(tr.tools))
	for _, tool := range tr.tools {
		tools = append(tools, tool)
	}
	return tools
}

// Count returns the number of registered tools
func (tr *ToolRegistry) Count() int {
	tr.mu.RLock()
	defer tr.mu.RUnlock()
	return len(tr.tools)
}
