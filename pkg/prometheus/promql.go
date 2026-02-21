package prometheus

import (
	"context"
	_ "embed"
	"encoding/json"

	"github.com/kagent-dev/tools/internal/mcpcompat"
	"github.com/tmc/langchaingo/llms"
	"github.com/tmc/langchaingo/llms/openai"
)

//go:embed promql_prompt.md
var promqlPrompt string

type promqlRequest struct {
	QueryDescription string `json:"query_description"`
}

func handlePromql(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var req promqlRequest
	if request.Params != nil && request.Params.Arguments != nil {
		if err := json.Unmarshal(request.Params.Arguments, &req); err != nil {
			return mcp.NewToolResultError("invalid arguments: " + err.Error()), nil
		}
	}

	queryDescription := req.QueryDescription
	if queryDescription == "" {
		return mcp.NewToolResultError("query_description is required"), nil
	}

	llm, err := openai.New()
	if err != nil {
		return mcp.NewToolResultError("failed to create LLM client: " + err.Error()), nil
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
		return mcp.NewToolResultError("failed to generate content: " + err.Error()), nil
	}

	choices := resp.Choices
	if len(choices) < 1 {
		return mcp.NewToolResultError("empty response from model"), nil
	}
	c1 := choices[0]
	return mcp.NewToolResultText(c1.Content), nil
}
