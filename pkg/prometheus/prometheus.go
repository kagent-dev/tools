package prometheus

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/kagent-dev/tools/internal/logger"
	"github.com/kagent-dev/tools/internal/security"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// clientKey is the context key for the http client.
type clientKey struct{}

func getHTTPClient(ctx context.Context) *http.Client {
	if client, ok := ctx.Value(clientKey{}).(*http.Client); ok && client != nil {
		return client
	}
	return http.DefaultClient
}

// Prometheus tools using direct HTTP API calls

func handlePrometheusQueryTool(ctx context.Context, request *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var args map[string]interface{}
	if err := json.Unmarshal(request.Params.Arguments, &args); err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "failed to parse arguments"}},
			IsError: true,
		}, nil
	}

	prometheusURL := "http://localhost:9090"
	query := ""

	if val, ok := args["prometheus_url"].(string); ok && val != "" {
		prometheusURL = val
	}
	if val, ok := args["query"].(string); ok {
		query = val
	}

	if query == "" {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "query parameter is required"}},
			IsError: true,
		}, nil
	}

	// Validate prometheus URL
	if err := security.ValidateURL(prometheusURL); err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("Invalid Prometheus URL: %v", err)}},
			IsError: true,
		}, nil
	}

	// Validate PromQL query
	if err := security.ValidatePromQLQuery(query); err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("Invalid PromQL query: %v", err)}},
			IsError: true,
		}, nil
	}

	// Make request to Prometheus API
	apiURL := fmt.Sprintf("%s/api/v1/query", prometheusURL)
	params := url.Values{}
	params.Add("query", query)
	params.Add("time", fmt.Sprintf("%d", time.Now().Unix()))

	fullURL := fmt.Sprintf("%s?%s", apiURL, params.Encode())

	client := getHTTPClient(ctx)
	req, err := http.NewRequestWithContext(ctx, "GET", fullURL, nil)
	if err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("failed to create request: %v", err)}},
			IsError: true,
		}, nil
	}

	resp, err := client.Do(req)
	if err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("failed to query Prometheus: %v", err)}},
			IsError: true,
		}, nil
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("failed to read response: %v", err)}},
			IsError: true,
		}, nil
	}

	if resp.StatusCode != http.StatusOK {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("Prometheus API error (%d): %s", resp.StatusCode, string(body))}},
			IsError: true,
		}, nil
	}

	// Parse the JSON response to pretty-print it
	var result interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: string(body)}},
		}, nil
	}

	prettyJSON, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: string(body)}},
		}, nil
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: string(prettyJSON)}},
	}, nil
}

func handlePrometheusRangeQueryTool(ctx context.Context, request *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var args map[string]interface{}
	if err := json.Unmarshal(request.Params.Arguments, &args); err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "failed to parse arguments"}},
			IsError: true,
		}, nil
	}

	prometheusURL := "http://localhost:9090"
	query := ""
	start := ""
	end := ""
	step := "15s"

	if val, ok := args["prometheus_url"].(string); ok && val != "" {
		prometheusURL = val
	}
	if val, ok := args["query"].(string); ok {
		query = val
	}
	if val, ok := args["start"].(string); ok {
		start = val
	}
	if val, ok := args["end"].(string); ok {
		end = val
	}
	if val, ok := args["step"].(string); ok && val != "" {
		step = val
	}

	if query == "" {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "query parameter is required"}},
			IsError: true,
		}, nil
	}

	// Validate prometheus URL
	if err := security.ValidateURL(prometheusURL); err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("Invalid Prometheus URL: %v", err)}},
			IsError: true,
		}, nil
	}

	// Validate PromQL query
	if err := security.ValidatePromQLQuery(query); err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("Invalid PromQL query: %v", err)}},
			IsError: true,
		}, nil
	}

	// Validate time parameters if provided
	if start != "" {
		if err := security.ValidateCommandInput(start); err != nil {
			return &mcp.CallToolResult{
				Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("Invalid start time: %v", err)}},
				IsError: true,
			}, nil
		}
	}
	if end != "" {
		if err := security.ValidateCommandInput(end); err != nil {
			return &mcp.CallToolResult{
				Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("Invalid end time: %v", err)}},
				IsError: true,
			}, nil
		}
	}
	if step != "" {
		if err := security.ValidateCommandInput(step); err != nil {
			return &mcp.CallToolResult{
				Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("Invalid step parameter: %v", err)}},
				IsError: true,
			}, nil
		}
	}

	// Use default time range if not specified
	if start == "" {
		start = fmt.Sprintf("%d", time.Now().Add(-1*time.Hour).Unix())
	}
	if end == "" {
		end = fmt.Sprintf("%d", time.Now().Unix())
	}

	// Make request to Prometheus API
	apiURL := fmt.Sprintf("%s/api/v1/query_range", prometheusURL)
	params := url.Values{}
	params.Add("query", query)
	params.Add("start", start)
	params.Add("end", end)
	params.Add("step", step)

	fullURL := fmt.Sprintf("%s?%s", apiURL, params.Encode())

	client := getHTTPClient(ctx)
	req, err := http.NewRequestWithContext(ctx, "GET", fullURL, nil)
	if err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "failed to create request: " + err.Error()}},
			IsError: true,
		}, nil
	}

	resp, err := client.Do(req)
	if err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "failed to query Prometheus: " + err.Error()}},
			IsError: true,
		}, nil
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "failed to read response: " + err.Error()}},
			IsError: true,
		}, nil
	}

	if resp.StatusCode != http.StatusOK {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("Prometheus API error (%d): %s", resp.StatusCode, string(body))}},
			IsError: true,
		}, nil
	}

	// Parse the JSON response to pretty-print it
	var result interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: string(body)}},
		}, nil
	}

	prettyJSON, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: string(body)}},
		}, nil
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: string(prettyJSON)}},
	}, nil
}

func handlePrometheusLabelsQueryTool(ctx context.Context, request *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var args map[string]interface{}
	if err := json.Unmarshal(request.Params.Arguments, &args); err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "failed to parse arguments"}},
			IsError: true,
		}, nil
	}

	prometheusURL := "http://localhost:9090"
	if val, ok := args["prometheus_url"].(string); ok && val != "" {
		prometheusURL = val
	}

	// Validate prometheus URL
	if err := security.ValidateURL(prometheusURL); err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("Invalid Prometheus URL: %v", err)}},
			IsError: true,
		}, nil
	}

	// Make request to Prometheus API for labels
	apiURL := fmt.Sprintf("%s/api/v1/labels", prometheusURL)

	client := getHTTPClient(ctx)
	req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	if err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("failed to create request: %v", err)}},
			IsError: true,
		}, nil
	}

	resp, err := client.Do(req)
	if err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("failed to query Prometheus: %v", err)}},
			IsError: true,
		}, nil
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("failed to read response: %v", err)}},
			IsError: true,
		}, nil
	}

	if resp.StatusCode != http.StatusOK {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("Prometheus API error (%d): %s", resp.StatusCode, string(body))}},
			IsError: true,
		}, nil
	}

	// Parse the JSON response to pretty-print it
	var result interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: string(body)}},
		}, nil
	}

	prettyJSON, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: string(body)}},
		}, nil
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: string(prettyJSON)}},
	}, nil
}

func handlePrometheusTargetsQueryTool(ctx context.Context, request *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var args map[string]interface{}
	if err := json.Unmarshal(request.Params.Arguments, &args); err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "failed to parse arguments"}},
			IsError: true,
		}, nil
	}

	prometheusURL := "http://localhost:9090"
	if val, ok := args["prometheus_url"].(string); ok && val != "" {
		prometheusURL = val
	}

	// Validate prometheus URL
	if err := security.ValidateURL(prometheusURL); err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("Invalid Prometheus URL: %v", err)}},
			IsError: true,
		}, nil
	}

	// Make request to Prometheus API for targets
	apiURL := fmt.Sprintf("%s/api/v1/targets", prometheusURL)

	client := getHTTPClient(ctx)
	req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	if err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "failed to create request: " + err.Error()}},
			IsError: true,
		}, nil
	}

	resp, err := client.Do(req)
	if err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "failed to query Prometheus: " + err.Error()}},
			IsError: true,
		}, nil
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "failed to read response: " + err.Error()}},
			IsError: true,
		}, nil
	}

	if resp.StatusCode != http.StatusOK {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("Prometheus API error (%d): %s", resp.StatusCode, string(body))}},
			IsError: true,
		}, nil
	}

	// Parse the JSON response to pretty-print it
	var result interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: string(body)}},
		}, nil
	}

	prettyJSON, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: string(body)}},
		}, nil
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: string(prettyJSON)}},
	}, nil
}

func RegisterTools(s *mcp.Server) error {
	logger.Get().Info("RegisterTools initialized")
	// Prometheus query tool
	s.AddTool(&mcp.Tool{
		Name:        "prometheus_query_tool",
		Description: "Execute a PromQL query against Prometheus",
		InputSchema: &jsonschema.Schema{
			Type: "object",
			Properties: map[string]*jsonschema.Schema{
				"query": {
					Type:        "string",
					Description: "PromQL query to execute",
				},
				"prometheus_url": {
					Type:        "string",
					Description: "Prometheus server URL (default: http://localhost:9090)",
				},
			},
			Required: []string{"query"},
		},
	}, handlePrometheusQueryTool)

	// Prometheus range query tool
	s.AddTool(&mcp.Tool{
		Name:        "prometheus_query_range_tool",
		Description: "Execute a PromQL range query against Prometheus",
		InputSchema: &jsonschema.Schema{
			Type: "object",
			Properties: map[string]*jsonschema.Schema{
				"query": {
					Type:        "string",
					Description: "PromQL query to execute",
				},
				"start": {
					Type:        "string",
					Description: "Start time (Unix timestamp or relative time)",
				},
				"end": {
					Type:        "string",
					Description: "End time (Unix timestamp or relative time)",
				},
				"step": {
					Type:        "string",
					Description: "Query resolution step (default: 15s)",
				},
				"prometheus_url": {
					Type:        "string",
					Description: "Prometheus server URL (default: http://localhost:9090)",
				},
			},
			Required: []string{"query"},
		},
	}, handlePrometheusRangeQueryTool)

	// Prometheus label names tool
	s.AddTool(&mcp.Tool{
		Name:        "prometheus_label_names_tool",
		Description: "Get all available labels from Prometheus",
		InputSchema: &jsonschema.Schema{
			Type: "object",
			Properties: map[string]*jsonschema.Schema{
				"prometheus_url": {
					Type:        "string",
					Description: "Prometheus server URL (default: http://localhost:9090)",
				},
			},
		},
	}, handlePrometheusLabelsQueryTool)

	// Prometheus targets tool
	s.AddTool(&mcp.Tool{
		Name:        "prometheus_targets_tool",
		Description: "Get all Prometheus targets and their status",
		InputSchema: &jsonschema.Schema{
			Type: "object",
			Properties: map[string]*jsonschema.Schema{
				"prometheus_url": {
					Type:        "string",
					Description: "Prometheus server URL (default: http://localhost:9090)",
				},
			},
		},
	}, handlePrometheusTargetsQueryTool)

	// Prometheus PromQL tool
	s.AddTool(&mcp.Tool{
		Name:        "prometheus_promql_tool",
		Description: "Generate a PromQL query",
		InputSchema: &jsonschema.Schema{
			Type: "object",
			Properties: map[string]*jsonschema.Schema{
				"query_description": {
					Type:        "string",
					Description: "A string describing the query to generate",
				},
			},
			Required: []string{"query_description"},
		},
	}, handlePromql)

	return nil
}
