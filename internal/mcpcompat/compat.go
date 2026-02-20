package mcp

import (
	"context"
	"encoding/json"
	"net/http"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

type (
	CallToolRequest = sdkmcp.CallToolRequest
	CallToolResult  = sdkmcp.CallToolResult
	Content         = sdkmcp.Content
	TextContent     = sdkmcp.TextContent
	Tool            = sdkmcp.Tool
)

type ToolHandlerFunc func(context.Context, CallToolRequest) (*CallToolResult, error)

// RegisterLegacyTool bridges old handler signature to the official SDK.
func RegisterLegacyTool(s *sdkmcp.Server, t Tool, h ToolHandlerFunc) {
	sdkmcp.AddTool(s, &t, func(ctx context.Context, req *sdkmcp.CallToolRequest, _ map[string]any) (*sdkmcp.CallToolResult, any, error) {
		if req == nil {
			req = &sdkmcp.CallToolRequest{}
		}
		res, err := h(ctx, *req)
		return res, nil, err
	})
}

func ParseString(req CallToolRequest, key, defaultVal string) string {
	if req.Params == nil || req.Params.Arguments == nil {
		return defaultVal
	}
	var args map[string]interface{}
	if err := json.Unmarshal(req.Params.Arguments, &args); err != nil {
		return defaultVal
	}
	if v, ok := args[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return defaultVal
}

func ParseInt(req CallToolRequest, key string, defaultVal int) int {
	if req.Params == nil || req.Params.Arguments == nil {
		return defaultVal
	}
	var args map[string]interface{}
	if err := json.Unmarshal(req.Params.Arguments, &args); err != nil {
		return defaultVal
	}
	if v, ok := args[key]; ok {
		switch vv := v.(type) {
		case float64:
			return int(vv)
		case int:
			return vv
		}
	}
	return defaultVal
}

func NewToolResultText(text string) *CallToolResult {
	return &CallToolResult{Content: []Content{&TextContent{Text: text}}}
}

func NewToolResultError(text string) *CallToolResult {
	return &CallToolResult{IsError: true, Content: []Content{&TextContent{Text: text}}}
}

func AsTextContent(c Content) (*TextContent, bool) {
	tc, ok := c.(*TextContent)
	return tc, ok
}

type toolConfig struct {
	description string
	properties  map[string]map[string]interface{}
	required    []string
}

type ToolOption func(*toolConfig)

func WithDescription(desc string) ToolOption { return func(c *toolConfig) { c.description = desc } }

func WithString(name string, opts ...PropOption) ToolOption {
	return func(c *toolConfig) {
		prop := map[string]interface{}{"type": "string"}
		cfg := applyPropOpts(opts)
		if cfg.description != "" {
			prop["description"] = cfg.description
		}
		if cfg.defaultVal != "" {
			prop["default"] = cfg.defaultVal
		}
		c.properties[name] = prop
		if cfg.required {
			c.required = append(c.required, name)
		}
	}
}

func WithNumber(name string, opts ...PropOption) ToolOption {
	return func(c *toolConfig) {
		prop := map[string]interface{}{"type": "number"}
		cfg := applyPropOpts(opts)
		if cfg.description != "" {
			prop["description"] = cfg.description
		}
		c.properties[name] = prop
		if cfg.required {
			c.required = append(c.required, name)
		}
	}
}

func NewTool(name string, opts ...ToolOption) Tool {
	cfg := &toolConfig{properties: make(map[string]map[string]interface{})}
	for _, o := range opts {
		o(cfg)
	}
	schema := map[string]interface{}{"type": "object", "properties": cfg.properties}
	if len(cfg.required) > 0 {
		schema["required"] = cfg.required
	}
	rawSchema, _ := json.Marshal(schema)
	return Tool{Name: name, Description: cfg.description, InputSchema: json.RawMessage(rawSchema)}
}

type PropOption func(*propConfig)

type propConfig struct {
	required    bool
	description string
	defaultVal  string
}

func applyPropOpts(opts []PropOption) propConfig {
	var cfg propConfig
	for _, o := range opts {
		o(&cfg)
	}
	return cfg
}

func Description(desc string) PropOption  { return func(c *propConfig) { c.description = desc } }
func Required() PropOption                { return func(c *propConfig) { c.required = true } }
func DefaultString(val string) PropOption { return func(c *propConfig) { c.defaultVal = val } }

type httpHeaderContextKey struct{}

func WithHTTPHeaders(ctx context.Context, headers http.Header) context.Context {
	return context.WithValue(ctx, httpHeaderContextKey{}, headers)
}

func HTTPHeadersFromContext(ctx context.Context) http.Header {
	if h, ok := ctx.Value(httpHeaderContextKey{}).(http.Header); ok {
		return h
	}
	return http.Header{}
}
