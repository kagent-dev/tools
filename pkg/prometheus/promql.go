package prometheus

import (
	"context"
	_ "embed"
	"encoding/json"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/tmc/langchaingo/llms"
	"github.com/tmc/langchaingo/llms/openai"
)

//go:embed promql_prompt.md
var promqlPrompt string

func handlePromql(ctx context.Context, request *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var args map[string]interface{}
	if err := json.Unmarshal(request.Params.Arguments, &args); err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "failed to parse arguments"}},
			IsError: true,
		}, nil
	}

	queryDescription := ""
	if val, ok := args["query_description"].(string); ok {
		queryDescription = val
	}

	if queryDescription == "" {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "query_description is required"}},
			IsError: true,
		}, nil
	}

	llm, err := openai.New()
	if err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "failed to create LLM client: " + err.Error()}},
			IsError: true,
		}, nil
	}

	contents := []llms.MessageContent{
		{
			Role: llms.ChatMessageTypeSystem,
			Parts: []llms.ContentPart{
				llms.TextContent{Text: promqlPrompt},
			},
		},

		{
			Role: llms.ChatMessageTypeHuman,
			Parts: []llms.ContentPart{
				llms.TextContent{Text: queryDescription},
			},
		},
	}

	resp, err := llm.GenerateContent(ctx, contents, llms.WithModel("gpt-4o-mini"))
	if err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "failed to generate content: " + err.Error()}},
			IsError: true,
		}, nil
	}

	choices := resp.Choices
	if len(choices) < 1 {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "empty response from model"}},
			IsError: true,
		}, nil
	}
	c1 := choices[0]
	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: c1.Content}},
	}, nil
}
