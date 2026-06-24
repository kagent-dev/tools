package prometheus

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/kagent-dev/tools/internal/errors"
	mcp "github.com/kagent-dev/tools/internal/mcp"
	"github.com/kagent-dev/tools/internal/security"
)

// clientKey is the context key for the http client.
type clientKey struct{}

func getHTTPClient(ctx context.Context) *http.Client {
	if client, ok := ctx.Value(clientKey{}).(*http.Client); ok && client != nil {
		return client
	}
	return http.DefaultClient
}

// prometheusErrResult adapts ToolError to an MCP error result.
func prometheusErrResult(toolErr *errors.ToolError) *mcp.CallToolResult {
	return toolErr.ToMCPResult()
}

type prometheusQueryInput struct {
	Query         string `json:"query" jsonschema:"PromQL query to execute"`
	PrometheusURL string `json:"prometheus_url" jsonschema:"Prometheus server URL (default: http://localhost:9090)"`
}

func handlePrometheusQueryTool(ctx context.Context, request *mcp.CallToolRequest, in prometheusQueryInput) (*mcp.CallToolResult, any, error) {
	prometheusURL := in.PrometheusURL
	if prometheusURL == "" {
		prometheusURL = "http://localhost:9090"
	}
	query := in.Query

	if query == "" {
		return mcp.NewToolResultError("query parameter is required"), nil, nil
	}

	// Validate prometheus URL
	if err := security.ValidateURL(prometheusURL); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Invalid Prometheus URL: %v", err)), nil, nil
	}

	// Validate PromQL query
	if err := security.ValidatePromQLQuery(query); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Invalid PromQL query: %v", err)), nil, nil
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
		toolErr := errors.NewPrometheusError("create_request", err).
			WithContext("prometheus_url", prometheusURL).
			WithContext("query", query)
		return prometheusErrResult(toolErr), nil, nil
	}

	resp, err := client.Do(req)
	if err != nil {
		toolErr := errors.NewPrometheusError("query_execution", err).
			WithContext("prometheus_url", prometheusURL).
			WithContext("query", query).
			WithContext("api_url", apiURL)
		return prometheusErrResult(toolErr), nil, nil
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		toolErr := errors.NewPrometheusError("read_response", err).
			WithContext("prometheus_url", prometheusURL).
			WithContext("query", query).
			WithContext("status_code", resp.StatusCode)
		return prometheusErrResult(toolErr), nil, nil
	}

	if resp.StatusCode != http.StatusOK {
		toolErr := errors.NewPrometheusError("api_error", fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))).
			WithContext("prometheus_url", prometheusURL).
			WithContext("query", query).
			WithContext("status_code", resp.StatusCode).
			WithContext("response_body", string(body))
		return prometheusErrResult(toolErr), nil, nil
	}

	// Parse the JSON response to pretty-print it
	var result interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		return mcp.NewToolResultText(string(body)), nil, nil
	}

	prettyJSON, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return mcp.NewToolResultText(string(body)), nil, nil
	}

	return mcp.NewToolResultText(string(prettyJSON)), nil, nil
}

type prometheusRangeQueryInput struct {
	Query         string `json:"query" jsonschema:"PromQL query to execute"`
	Start         string `json:"start" jsonschema:"Start time (Unix timestamp or relative time)"`
	End           string `json:"end" jsonschema:"End time (Unix timestamp or relative time)"`
	Step          string `json:"step" jsonschema:"Query resolution step (default: 15s)"`
	PrometheusURL string `json:"prometheus_url" jsonschema:"Prometheus server URL (default: http://localhost:9090)"`
}

func handlePrometheusRangeQueryTool(ctx context.Context, request *mcp.CallToolRequest, in prometheusRangeQueryInput) (*mcp.CallToolResult, any, error) {
	prometheusURL := in.PrometheusURL
	if prometheusURL == "" {
		prometheusURL = "http://localhost:9090"
	}
	query := in.Query
	start := in.Start
	end := in.End
	step := in.Step
	if step == "" {
		step = "15s"
	}

	if query == "" {
		return mcp.NewToolResultError("query parameter is required"), nil, nil
	}

	// Validate prometheus URL
	if err := security.ValidateURL(prometheusURL); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Invalid Prometheus URL: %v", err)), nil, nil
	}

	// Validate PromQL query
	if err := security.ValidatePromQLQuery(query); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Invalid PromQL query: %v", err)), nil, nil
	}

	// Validate time parameters if provided
	if start != "" {
		if err := security.ValidateCommandInput(start); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Invalid start time: %v", err)), nil, nil
		}
	}
	if end != "" {
		if err := security.ValidateCommandInput(end); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Invalid end time: %v", err)), nil, nil
		}
	}
	if step != "" {
		if err := security.ValidateCommandInput(step); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Invalid step parameter: %v", err)), nil, nil
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
		return mcp.NewToolResultError("failed to create request: " + err.Error()), nil, nil
	}

	resp, err := client.Do(req)
	if err != nil {
		return mcp.NewToolResultError("failed to query Prometheus: " + err.Error()), nil, nil
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return mcp.NewToolResultError("failed to read response: " + err.Error()), nil, nil
	}

	if resp.StatusCode != http.StatusOK {
		return mcp.NewToolResultError(fmt.Sprintf("Prometheus API error (%d): %s", resp.StatusCode, string(body))), nil, nil
	}

	// Parse the JSON response to pretty-print it
	var result interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		return mcp.NewToolResultText(string(body)), nil, nil
	}

	prettyJSON, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return mcp.NewToolResultText(string(body)), nil, nil
	}

	return mcp.NewToolResultText(string(prettyJSON)), nil, nil
}

type prometheusLabelsInput struct {
	PrometheusURL string `json:"prometheus_url" jsonschema:"Prometheus server URL (default: http://localhost:9090)"`
}

func handlePrometheusLabelsQueryTool(ctx context.Context, request *mcp.CallToolRequest, in prometheusLabelsInput) (*mcp.CallToolResult, any, error) {
	prometheusURL := in.PrometheusURL
	if prometheusURL == "" {
		prometheusURL = "http://localhost:9090"
	}

	// Validate prometheus URL
	if err := security.ValidateURL(prometheusURL); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Invalid Prometheus URL: %v", err)), nil, nil
	}

	// Make request to Prometheus API for labels
	apiURL := fmt.Sprintf("%s/api/v1/labels", prometheusURL)

	client := getHTTPClient(ctx)
	req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	if err != nil {
		toolErr := errors.NewPrometheusError("create_request", err).
			WithContext("prometheus_url", prometheusURL).
			WithContext("api_url", apiURL)
		return prometheusErrResult(toolErr), nil, nil
	}

	resp, err := client.Do(req)
	if err != nil {
		toolErr := errors.NewPrometheusError("query_execution", err).
			WithContext("prometheus_url", prometheusURL).
			WithContext("api_url", apiURL)
		return prometheusErrResult(toolErr), nil, nil
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		toolErr := errors.NewPrometheusError("read_response", err).
			WithContext("prometheus_url", prometheusURL).
			WithContext("api_url", apiURL).
			WithContext("status_code", resp.StatusCode)
		return prometheusErrResult(toolErr), nil, nil
	}

	if resp.StatusCode != http.StatusOK {
		toolErr := errors.NewPrometheusError("api_error", fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))).
			WithContext("prometheus_url", prometheusURL).
			WithContext("api_url", apiURL).
			WithContext("status_code", resp.StatusCode).
			WithContext("response_body", string(body))
		return prometheusErrResult(toolErr), nil, nil
	}

	// Parse the JSON response to pretty-print it
	var result interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		return mcp.NewToolResultText(string(body)), nil, nil
	}

	prettyJSON, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return mcp.NewToolResultText(string(body)), nil, nil
	}

	return mcp.NewToolResultText(string(prettyJSON)), nil, nil
}

type prometheusTargetsInput struct {
	PrometheusURL string `json:"prometheus_url" jsonschema:"Prometheus server URL (default: http://localhost:9090)"`
}

func handlePrometheusTargetsQueryTool(ctx context.Context, request *mcp.CallToolRequest, in prometheusTargetsInput) (*mcp.CallToolResult, any, error) {
	prometheusURL := in.PrometheusURL
	if prometheusURL == "" {
		prometheusURL = "http://localhost:9090"
	}

	// Validate prometheus URL
	if err := security.ValidateURL(prometheusURL); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Invalid Prometheus URL: %v", err)), nil, nil
	}

	// Make request to Prometheus API for targets
	apiURL := fmt.Sprintf("%s/api/v1/targets", prometheusURL)

	client := getHTTPClient(ctx)
	req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	if err != nil {
		return mcp.NewToolResultError("failed to create request: " + err.Error()), nil, nil
	}

	resp, err := client.Do(req)
	if err != nil {
		return mcp.NewToolResultError("failed to query Prometheus: " + err.Error()), nil, nil
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return mcp.NewToolResultError("failed to read response: " + err.Error()), nil, nil
	}

	if resp.StatusCode != http.StatusOK {
		return mcp.NewToolResultError(fmt.Sprintf("Prometheus API error (%d): %s", resp.StatusCode, string(body))), nil, nil
	}

	// Parse the JSON response to pretty-print it
	var result interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		return mcp.NewToolResultText(string(body)), nil, nil
	}

	prettyJSON, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return mcp.NewToolResultText(string(body)), nil, nil
	}

	return mcp.NewToolResultText(string(prettyJSON)), nil, nil
}

func RegisterTools(s *mcp.Server, readOnly bool) {
	mcp.AddTool(s, "prometheus", &mcp.Tool{
		Name:        "prometheus_query_tool",
		Description: "Execute a PromQL query against Prometheus",
	}, handlePrometheusQueryTool)

	mcp.AddTool(s, "prometheus", &mcp.Tool{
		Name:        "prometheus_query_range_tool",
		Description: "Execute a PromQL range query against Prometheus",
	}, handlePrometheusRangeQueryTool)

	mcp.AddTool(s, "prometheus", &mcp.Tool{
		Name:        "prometheus_label_names_tool",
		Description: "Get all available labels from Prometheus",
	}, handlePrometheusLabelsQueryTool)

	mcp.AddTool(s, "prometheus", &mcp.Tool{
		Name:        "prometheus_targets_tool",
		Description: "Get all Prometheus targets and their status",
	}, handlePrometheusTargetsQueryTool)

	mcp.AddTool(s, "prometheus", &mcp.Tool{
		Name:        "prometheus_promql_tool",
		Description: "Generate a PromQL query",
	}, handlePromql)
}
