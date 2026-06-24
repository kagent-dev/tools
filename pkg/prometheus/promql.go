package prometheus

import (
	"context"
	_ "embed"

	mcp "github.com/kagent-dev/tools/internal/mcp"
	"github.com/tmc/langchaingo/llms"
	"github.com/tmc/langchaingo/llms/openai"
)

//go:embed promql_prompt.md
var promqlPrompt string

type promqlInput struct {
	QueryDescription string `json:"query_description" jsonschema:"A string describing the query to generate"`
}

func handlePromql(ctx context.Context, request *mcp.CallToolRequest, in promqlInput) (*mcp.CallToolResult, any, error) {
	queryDescription := in.QueryDescription
	if queryDescription == "" {
		return mcp.NewToolResultError("query_description is required"), nil, nil
	}

	llm, err := openai.New()
	if err != nil {
		return mcp.NewToolResultError("failed to create LLM client: " + err.Error()), nil, nil
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
		return mcp.NewToolResultError("failed to generate content: " + err.Error()), nil, nil
	}

	choices := resp.Choices
	if len(choices) < 1 {
		return mcp.NewToolResultError("empty response from model"), nil, nil
	}
	c1 := choices[0]
	return mcp.NewToolResultText(c1.Content), nil, nil
}
