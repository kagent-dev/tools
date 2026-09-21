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

func handlePromql(ctx context.Context, request *mcp.CallToolRequest, in promqlInput) (*mcp.CallToolResult, mcp.TextOutput, error) {
	queryDescription := in.QueryDescription
	if queryDescription == "" {
		return mcp.TextError("query_description is required")
	}

	llm, err := openai.New()
	if err != nil {
		return mcp.TextError("failed to create LLM client: " + err.Error())
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
		return mcp.TextError("failed to generate content: " + err.Error())
	}

	choices := resp.Choices
	if len(choices) < 1 {
		return mcp.TextError("empty response from model")
	}
	c1 := choices[0]
	return mcp.TextResult(c1.Content)
}
