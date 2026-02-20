package prometheus

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/kagent-dev/tools/internal/errors"
	"github.com/kagent-dev/tools/internal/mcpcompat"
	"github.com/kagent-dev/tools/internal/mcpcompat/server"
	"github.com/kagent-dev/tools/internal/security"
	"github.com/kagent-dev/tools/internal/telemetry"
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

type queryRequest struct {
	PrometheusURL string `json:"prometheus_url"`
	Query         string `json:"query"`
}

type rangeQueryRequest struct {
	PrometheusURL string `json:"prometheus_url"`
	Query         string `json:"query"`
	Start         string `json:"start"`
	End           string `json:"end"`
	Step          string `json:"step"`
}

type prometheusURLRequest struct {
	PrometheusURL string `json:"prometheus_url"`
}

func decodeArgs(request mcp.CallToolRequest, out interface{}) error {
	if request.Params == nil || request.Params.Arguments == nil {
		return nil
	}
	return json.Unmarshal(request.Params.Arguments, out)
}

func prettyJSONOrRaw(body []byte) string {
	var out bytes.Buffer
	if err := json.Indent(&out, body, "", "  "); err != nil {
		return string(body)
	}
	return out.String()
}

func handlePrometheusQueryTool(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var reqData queryRequest
	if err := decodeArgs(request, &reqData); err != nil {
		return mcp.NewToolResultError("invalid arguments: " + err.Error()), nil
	}

	prometheusURL := reqData.PrometheusURL
	if prometheusURL == "" {
		prometheusURL = "http://localhost:9090"
	}
	query := reqData.Query

	if query == "" {
		return mcp.NewToolResultError("query parameter is required"), nil
	}

	// Validate prometheus URL
	if err := security.ValidateURL(prometheusURL); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Invalid Prometheus URL: %v", err)), nil
	}

	// Validate PromQL query
	if err := security.ValidatePromQLQuery(query); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Invalid PromQL query: %v", err)), nil
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
		return toolErr.ToMCPResult(), nil
	}

	resp, err := client.Do(req)
	if err != nil {
		toolErr := errors.NewPrometheusError("query_execution", err).
			WithContext("prometheus_url", prometheusURL).
			WithContext("query", query).
			WithContext("api_url", apiURL)
		return toolErr.ToMCPResult(), nil
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		toolErr := errors.NewPrometheusError("read_response", err).
			WithContext("prometheus_url", prometheusURL).
			WithContext("query", query).
			WithContext("status_code", resp.StatusCode)
		return toolErr.ToMCPResult(), nil
	}

	if resp.StatusCode != http.StatusOK {
		toolErr := errors.NewPrometheusError("api_error", fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))).
			WithContext("prometheus_url", prometheusURL).
			WithContext("query", query).
			WithContext("status_code", resp.StatusCode).
			WithContext("response_body", string(body))
		return toolErr.ToMCPResult(), nil
	}

	return mcp.NewToolResultText(prettyJSONOrRaw(body)), nil
}

func handlePrometheusRangeQueryTool(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var reqData rangeQueryRequest
	if err := decodeArgs(request, &reqData); err != nil {
		return mcp.NewToolResultError("invalid arguments: " + err.Error()), nil
	}

	prometheusURL := reqData.PrometheusURL
	if prometheusURL == "" {
		prometheusURL = "http://localhost:9090"
	}
	query := reqData.Query
	start := reqData.Start
	end := reqData.End
	step := reqData.Step
	if step == "" {
		step = "15s"
	}

	if query == "" {
		return mcp.NewToolResultError("query parameter is required"), nil
	}

	// Validate prometheus URL
	if err := security.ValidateURL(prometheusURL); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Invalid Prometheus URL: %v", err)), nil
	}

	// Validate PromQL query
	if err := security.ValidatePromQLQuery(query); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Invalid PromQL query: %v", err)), nil
	}

	// Validate time parameters if provided
	if start != "" {
		if err := security.ValidateCommandInput(start); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Invalid start time: %v", err)), nil
		}
	}
	if end != "" {
		if err := security.ValidateCommandInput(end); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Invalid end time: %v", err)), nil
		}
	}
	if step != "" {
		if err := security.ValidateCommandInput(step); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Invalid step parameter: %v", err)), nil
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
		return mcp.NewToolResultError("failed to create request: " + err.Error()), nil
	}

	resp, err := client.Do(req)
	if err != nil {
		return mcp.NewToolResultError("failed to query Prometheus: " + err.Error()), nil
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return mcp.NewToolResultError("failed to read response: " + err.Error()), nil
	}

	if resp.StatusCode != http.StatusOK {
		return mcp.NewToolResultError(fmt.Sprintf("Prometheus API error (%d): %s", resp.StatusCode, string(body))), nil
	}

	return mcp.NewToolResultText(prettyJSONOrRaw(body)), nil
}

func handlePrometheusLabelsQueryTool(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var reqData prometheusURLRequest
	if err := decodeArgs(request, &reqData); err != nil {
		return mcp.NewToolResultError("invalid arguments: " + err.Error()), nil
	}

	prometheusURL := reqData.PrometheusURL
	if prometheusURL == "" {
		prometheusURL = "http://localhost:9090"
	}

	// Validate prometheus URL
	if err := security.ValidateURL(prometheusURL); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Invalid Prometheus URL: %v", err)), nil
	}

	// Make request to Prometheus API for labels
	apiURL := fmt.Sprintf("%s/api/v1/labels", prometheusURL)

	client := getHTTPClient(ctx)
	req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	if err != nil {
		toolErr := errors.NewPrometheusError("create_request", err).
			WithContext("prometheus_url", prometheusURL).
			WithContext("api_url", apiURL)
		return toolErr.ToMCPResult(), nil
	}

	resp, err := client.Do(req)
	if err != nil {
		toolErr := errors.NewPrometheusError("query_execution", err).
			WithContext("prometheus_url", prometheusURL).
			WithContext("api_url", apiURL)
		return toolErr.ToMCPResult(), nil
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		toolErr := errors.NewPrometheusError("read_response", err).
			WithContext("prometheus_url", prometheusURL).
			WithContext("api_url", apiURL).
			WithContext("status_code", resp.StatusCode)
		return toolErr.ToMCPResult(), nil
	}

	if resp.StatusCode != http.StatusOK {
		toolErr := errors.NewPrometheusError("api_error", fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))).
			WithContext("prometheus_url", prometheusURL).
			WithContext("api_url", apiURL).
			WithContext("status_code", resp.StatusCode).
			WithContext("response_body", string(body))
		return toolErr.ToMCPResult(), nil
	}

	return mcp.NewToolResultText(prettyJSONOrRaw(body)), nil
}

func handlePrometheusTargetsQueryTool(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var reqData prometheusURLRequest
	if err := decodeArgs(request, &reqData); err != nil {
		return mcp.NewToolResultError("invalid arguments: " + err.Error()), nil
	}

	prometheusURL := reqData.PrometheusURL
	if prometheusURL == "" {
		prometheusURL = "http://localhost:9090"
	}

	// Validate prometheus URL
	if err := security.ValidateURL(prometheusURL); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Invalid Prometheus URL: %v", err)), nil
	}

	// Make request to Prometheus API for targets
	apiURL := fmt.Sprintf("%s/api/v1/targets", prometheusURL)

	client := getHTTPClient(ctx)
	req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	if err != nil {
		return mcp.NewToolResultError("failed to create request: " + err.Error()), nil
	}

	resp, err := client.Do(req)
	if err != nil {
		return mcp.NewToolResultError("failed to query Prometheus: " + err.Error()), nil
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return mcp.NewToolResultError("failed to read response: " + err.Error()), nil
	}

	if resp.StatusCode != http.StatusOK {
		return mcp.NewToolResultError(fmt.Sprintf("Prometheus API error (%d): %s", resp.StatusCode, string(body))), nil
	}

	return mcp.NewToolResultText(prettyJSONOrRaw(body)), nil
}

func RegisterTools(s *server.MCPServer, readOnly bool) {
	s.AddTool(mcp.NewTool("prometheus_query_tool",
		mcp.WithDescription("Execute a PromQL query against Prometheus"),
		mcp.WithString("query", mcp.Description("PromQL query to execute"), mcp.Required()),
		mcp.WithString("prometheus_url", mcp.Description("Prometheus server URL (default: http://localhost:9090)")),
	), telemetry.AdaptToolHandler(telemetry.WithTracing("prometheus_query_tool", handlePrometheusQueryTool)))

	s.AddTool(mcp.NewTool("prometheus_query_range_tool",
		mcp.WithDescription("Execute a PromQL range query against Prometheus"),
		mcp.WithString("query", mcp.Description("PromQL query to execute"), mcp.Required()),
		mcp.WithString("start", mcp.Description("Start time (Unix timestamp or relative time)")),
		mcp.WithString("end", mcp.Description("End time (Unix timestamp or relative time)")),
		mcp.WithString("step", mcp.Description("Query resolution step (default: 15s)")),
		mcp.WithString("prometheus_url", mcp.Description("Prometheus server URL (default: http://localhost:9090)")),
	), telemetry.AdaptToolHandler(telemetry.WithTracing("prometheus_query_range_tool", handlePrometheusRangeQueryTool)))

	s.AddTool(mcp.NewTool("prometheus_label_names_tool",
		mcp.WithDescription("Get all available labels from Prometheus"),
		mcp.WithString("prometheus_url", mcp.Description("Prometheus server URL (default: http://localhost:9090)")),
	), telemetry.AdaptToolHandler(telemetry.WithTracing("prometheus_label_names_tool", handlePrometheusLabelsQueryTool)))

	s.AddTool(mcp.NewTool("prometheus_targets_tool",
		mcp.WithDescription("Get all Prometheus targets and their status"),
		mcp.WithString("prometheus_url", mcp.Description("Prometheus server URL (default: http://localhost:9090)")),
	), telemetry.AdaptToolHandler(telemetry.WithTracing("prometheus_targets_tool", handlePrometheusTargetsQueryTool)))

	s.AddTool(mcp.NewTool("prometheus_promql_tool",
		mcp.WithDescription("Generate a PromQL query"),
		mcp.WithString("query_description", mcp.Description("A string describing the query to generate"), mcp.Required()),
	), telemetry.AdaptToolHandler(telemetry.WithTracing("prometheus_promql_tool", handlePromql)))
}
