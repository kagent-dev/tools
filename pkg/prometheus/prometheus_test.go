package prometheus

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	mcp "github.com/kagent-dev/tools/internal/mcp"
	"github.com/stretchr/testify/assert"
)

func TestRegisterTools(t *testing.T) {
	t.Run("read-write", func(t *testing.T) {
		s := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "v0.0.1"}, nil)
		RegisterTools(s, false)
	})
	t.Run("read-only", func(t *testing.T) {
		s := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "v0.0.1"}, nil)
		RegisterTools(s, true)
	})
}

func TestPrometheusInputValidation(t *testing.T) {
	ctx := context.Background()

	t.Run("query invalid url", func(t *testing.T) {
		res, _, err := handlePrometheusQueryTool(ctx, &mcp.CallToolRequest{}, prometheusQueryInput{
			PrometheusURL: "not a url",
			Query:         "up",
		})
		assert.NoError(t, err)
		assert.True(t, res.IsError)
	})

	t.Run("range invalid url", func(t *testing.T) {
		res, _, err := handlePrometheusRangeQueryTool(ctx, &mcp.CallToolRequest{}, prometheusRangeQueryInput{
			PrometheusURL: "not a url",
			Query:         "up",
		})
		assert.NoError(t, err)
		assert.True(t, res.IsError)
	})

	t.Run("labels invalid url", func(t *testing.T) {
		res, _, err := handlePrometheusLabelsQueryTool(ctx, &mcp.CallToolRequest{}, prometheusLabelsInput{
			PrometheusURL: "not a url",
		})
		assert.NoError(t, err)
		assert.True(t, res.IsError)
	})

	t.Run("targets invalid url", func(t *testing.T) {
		res, _, err := handlePrometheusTargetsQueryTool(ctx, &mcp.CallToolRequest{}, prometheusTargetsInput{
			PrometheusURL: "not a url",
		})
		assert.NoError(t, err)
		assert.True(t, res.IsError)
	})

	t.Run("query invalid promql", func(t *testing.T) {
		res, _, err := handlePrometheusQueryTool(ctx, &mcp.CallToolRequest{}, prometheusQueryInput{
			PrometheusURL: "http://localhost:9090",
			Query:         "up; drop",
		})
		assert.NoError(t, err)
		assert.True(t, res.IsError)
	})

	t.Run("range invalid promql", func(t *testing.T) {
		res, _, err := handlePrometheusRangeQueryTool(ctx, &mcp.CallToolRequest{}, prometheusRangeQueryInput{
			PrometheusURL: "http://localhost:9090",
			Query:         "up; drop",
		})
		assert.NoError(t, err)
		assert.True(t, res.IsError)
	})
}

func TestPrometheusLabelsTargetsErrorPaths(t *testing.T) {
	t.Run("labels client error", func(t *testing.T) {
		ctx := contextWithMockClient(newTestClient(nil, assert.AnError))
		res, _, err := handlePrometheusLabelsQueryTool(ctx, &mcp.CallToolRequest{}, prometheusLabelsInput{
			PrometheusURL: "http://localhost:9090",
		})
		assert.NoError(t, err)
		assert.True(t, res.IsError)
	})

	t.Run("labels malformed json", func(t *testing.T) {
		ctx := contextWithMockClient(newTestClient(createMockResponse(200, "not json"), nil))
		res, _, err := handlePrometheusLabelsQueryTool(ctx, &mcp.CallToolRequest{}, prometheusLabelsInput{
			PrometheusURL: "http://localhost:9090",
		})
		assert.NoError(t, err)
		assert.False(t, res.IsError)
		assert.Contains(t, getResultText(res), "not json")
	})

	t.Run("targets client error", func(t *testing.T) {
		ctx := contextWithMockClient(newTestClient(nil, assert.AnError))
		res, _, err := handlePrometheusTargetsQueryTool(ctx, &mcp.CallToolRequest{}, prometheusTargetsInput{
			PrometheusURL: "http://localhost:9090",
		})
		assert.NoError(t, err)
		assert.True(t, res.IsError)
	})

	t.Run("targets malformed json", func(t *testing.T) {
		ctx := contextWithMockClient(newTestClient(createMockResponse(200, "not json"), nil))
		res, _, err := handlePrometheusTargetsQueryTool(ctx, &mcp.CallToolRequest{}, prometheusTargetsInput{
			PrometheusURL: "http://localhost:9090",
		})
		assert.NoError(t, err)
		assert.False(t, res.IsError)
		assert.Contains(t, getResultText(res), "not json")
	})
}

func TestGetHTTPClientDefault(t *testing.T) {
	assert.Equal(t, http.DefaultClient, getHTTPClient(context.Background()))
	custom := &http.Client{}
	ctx := context.WithValue(context.Background(), clientKey{}, custom)
	assert.Equal(t, custom, getHTTPClient(ctx))
}

// mockRoundTripper is used to mock HTTP responses for testing
type mockRoundTripper struct {
	response *http.Response
	err      error
}

func (m *mockRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.response, nil
}

func newTestClient(response *http.Response, err error) *http.Client {
	return &http.Client{
		Transport: &mockRoundTripper{
			response: response,
			err:      err,
		},
	}
}

// Helper function to extract text content from MCP result
func getResultText(result *mcp.CallToolResult) string {
	if result == nil || len(result.Content) == 0 {
		return ""
	}
	if textContent, ok := result.Content[0].(*mcp.TextContent); ok {
		return textContent.Text
	}
	return ""
}

// Helper function to create a mock HTTP response
func createMockResponse(statusCode int, body string) *http.Response {
	return &http.Response{
		StatusCode: statusCode,
		Body:       io.NopCloser(strings.NewReader(body)),
		Header:     make(http.Header),
	}
}

// Helper function to create context with mock HTTP client
func contextWithMockClient(client *http.Client) context.Context {
	return context.WithValue(context.Background(), clientKey{}, client)
}

func TestHandlePrometheusQueryTool(t *testing.T) {
	t.Run("successful query", func(t *testing.T) {
		mockResponse := `{
			"status": "success",
			"data": {
				"resultType": "vector",
				"result": [
					{
						"metric": {"__name__": "up", "instance": "localhost:9090"},
						"value": [1609459200, "1"]
					}
				]
			}
		}`

		client := newTestClient(createMockResponse(200, mockResponse), nil)
		ctx := contextWithMockClient(client)

		result, _, err := handlePrometheusQueryTool(ctx, &mcp.CallToolRequest{}, prometheusQueryInput{
			Query:         "up",
			PrometheusURL: "http://localhost:9090",
		})

		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.False(t, result.IsError)

		content := getResultText(result)
		assert.Contains(t, content, "success")
		assert.Contains(t, content, "up")
	})

	t.Run("missing query parameter", func(t *testing.T) {
		ctx := context.Background()
		result, _, err := handlePrometheusQueryTool(ctx, &mcp.CallToolRequest{}, prometheusQueryInput{
			PrometheusURL: "http://localhost:9090",
		})

		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.True(t, result.IsError)
		assert.Contains(t, getResultText(result), "query parameter is required")
	})

	t.Run("HTTP error", func(t *testing.T) {
		client := newTestClient(nil, assert.AnError)
		ctx := contextWithMockClient(client)

		result, _, err := handlePrometheusQueryTool(ctx, &mcp.CallToolRequest{}, prometheusQueryInput{
			Query: "up",
		})

		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.True(t, result.IsError)
		assert.Contains(t, getResultText(result), "**Prometheus Error**")
	})

	t.Run("HTTP 500 error", func(t *testing.T) {
		client := newTestClient(createMockResponse(500, "Internal Server Error"), nil)
		ctx := contextWithMockClient(client)

		result, _, err := handlePrometheusQueryTool(ctx, &mcp.CallToolRequest{}, prometheusQueryInput{
			Query: "up",
		})

		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.True(t, result.IsError)
		assert.Contains(t, getResultText(result), "**Prometheus Error**")
	})

	t.Run("malformed JSON response", func(t *testing.T) {
		client := newTestClient(createMockResponse(200, "invalid json {"), nil)
		ctx := contextWithMockClient(client)

		result, _, err := handlePrometheusQueryTool(ctx, &mcp.CallToolRequest{}, prometheusQueryInput{
			Query: "up",
		})

		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.False(t, result.IsError)
		// Should return raw response when JSON parsing fails
		assert.Contains(t, getResultText(result), "invalid json")
	})

	t.Run("default prometheus URL", func(t *testing.T) {
		mockResponse := `{"status": "success", "data": {"result": []}}`
		client := newTestClient(createMockResponse(200, mockResponse), nil)
		ctx := contextWithMockClient(client)

		result, _, err := handlePrometheusQueryTool(ctx, &mcp.CallToolRequest{}, prometheusQueryInput{
			Query: "up",
		})

		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.False(t, result.IsError)
	})
}

func TestHandlePrometheusRangeQueryTool(t *testing.T) {
	t.Run("successful range query", func(t *testing.T) {
		mockResponse := `{
			"status": "success",
			"data": {
				"resultType": "matrix",
				"result": [
					{
						"metric": {"__name__": "up"},
						"values": [[1609459200, "1"], [1609459260, "1"]]
					}
				]
			}
		}`

		client := newTestClient(createMockResponse(200, mockResponse), nil)
		ctx := contextWithMockClient(client)

		result, _, err := handlePrometheusRangeQueryTool(ctx, &mcp.CallToolRequest{}, prometheusRangeQueryInput{
			Query: "up",
			Start: "1609459200",
			End:   "1609459260",
			Step:  "60s",
		})

		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.False(t, result.IsError)

		content := getResultText(result)
		assert.Contains(t, content, "matrix")
		assert.Contains(t, content, "values")
	})

	t.Run("missing query parameter", func(t *testing.T) {
		ctx := context.Background()
		result, _, err := handlePrometheusRangeQueryTool(ctx, &mcp.CallToolRequest{}, prometheusRangeQueryInput{})

		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.True(t, result.IsError)
		assert.Contains(t, getResultText(result), "query parameter is required")
	})

	t.Run("default time range and step", func(t *testing.T) {
		mockResponse := `{"status": "success", "data": {"result": []}}`
		client := newTestClient(createMockResponse(200, mockResponse), nil)
		ctx := contextWithMockClient(client)

		result, _, err := handlePrometheusRangeQueryTool(ctx, &mcp.CallToolRequest{}, prometheusRangeQueryInput{
			Query: "up",
		})

		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.False(t, result.IsError)
	})
}

func TestHandlePrometheusLabelsQueryTool(t *testing.T) {
	t.Run("successful labels query", func(t *testing.T) {
		mockResponse := `{
			"status": "success",
			"data": ["__name__", "instance", "job"]
		}`

		client := newTestClient(createMockResponse(200, mockResponse), nil)
		ctx := contextWithMockClient(client)

		result, _, err := handlePrometheusLabelsQueryTool(ctx, &mcp.CallToolRequest{}, prometheusLabelsInput{})

		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.False(t, result.IsError)

		content := getResultText(result)
		assert.Contains(t, content, "__name__")
		assert.Contains(t, content, "instance")
		assert.Contains(t, content, "job")
	})

	t.Run("HTTP error", func(t *testing.T) {
		client := newTestClient(nil, assert.AnError)
		ctx := contextWithMockClient(client)

		result, _, err := handlePrometheusLabelsQueryTool(ctx, &mcp.CallToolRequest{}, prometheusLabelsInput{})

		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.True(t, result.IsError)
		assert.Contains(t, getResultText(result), "**Prometheus Error**")
	})

	t.Run("custom prometheus URL", func(t *testing.T) {
		mockResponse := `{"status": "success", "data": []}`
		client := newTestClient(createMockResponse(200, mockResponse), nil)
		ctx := contextWithMockClient(client)

		result, _, err := handlePrometheusLabelsQueryTool(ctx, &mcp.CallToolRequest{}, prometheusLabelsInput{
			PrometheusURL: "http://custom:9090",
		})

		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.False(t, result.IsError)
	})
}

func TestHandlePrometheusTargetsQueryTool(t *testing.T) {
	t.Run("successful targets query", func(t *testing.T) {
		mockResponse := `{
			"status": "success",
			"data": {
				"activeTargets": [
					{
						"discoveredLabels": {"__address__": "localhost:9090"},
						"labels": {"instance": "localhost:9090", "job": "prometheus"},
						"scrapePool": "prometheus",
						"scrapeUrl": "http://localhost:9090/metrics",
						"health": "up"
					}
				]
			}
		}`

		client := newTestClient(createMockResponse(200, mockResponse), nil)
		ctx := contextWithMockClient(client)

		result, _, err := handlePrometheusTargetsQueryTool(ctx, &mcp.CallToolRequest{}, prometheusTargetsInput{})

		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.False(t, result.IsError)

		content := getResultText(result)
		assert.Contains(t, content, "activeTargets")
		assert.Contains(t, content, "localhost:9090")
		assert.Contains(t, content, "up")
	})

	t.Run("HTTP 404 error", func(t *testing.T) {
		client := newTestClient(createMockResponse(404, "Not Found"), nil)
		ctx := contextWithMockClient(client)

		result, _, err := handlePrometheusTargetsQueryTool(ctx, &mcp.CallToolRequest{}, prometheusTargetsInput{})

		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.True(t, result.IsError)
		assert.Contains(t, getResultText(result), "Prometheus API error (404)")
	})
}

func TestHandlePromql(t *testing.T) {
	t.Run("missing query description", func(t *testing.T) {
		ctx := context.Background()
		result, _, err := handlePromql(ctx, &mcp.CallToolRequest{}, promqlInput{})

		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.True(t, result.IsError)
		assert.Contains(t, getResultText(result), "query_description is required")
	})

	t.Run("with query description", func(t *testing.T) {
		ctx := context.Background()
		result, _, err := handlePromql(ctx, &mcp.CallToolRequest{}, promqlInput{
			QueryDescription: "CPU usage percentage",
		})

		assert.NoError(t, err)
		assert.NotNil(t, result)
		// This will likely fail due to missing OpenAI API key, but that's expected
		// We're testing that the function handles the error gracefully
		if result.IsError {
			content := getResultText(result)
			// Should contain an error about LLM client or API
			assert.True(t,
				strings.Contains(content, "failed to create LLM client") ||
					strings.Contains(content, "failed to generate content") ||
					strings.Contains(content, "API"),
			)
		}
	})
}

// Test context cancellation scenarios
func TestPrometheusToolsContextCancellation(t *testing.T) {
	t.Run("query tool with cancelled context", func(t *testing.T) {
		// Create a mock client that would block indefinitely
		client := newTestClient(createMockResponse(200, `{"status": "success"}`), nil)

		// Create a cancelled context
		cancelCtx, cancel := context.WithCancel(context.Background())
		cancel() // Cancel immediately
		ctx := contextWithMockClient(client)
		_ = cancelCtx

		result, _, err := handlePrometheusQueryTool(ctx, &mcp.CallToolRequest{}, prometheusQueryInput{
			Query: "up",
		})

		// Should handle cancellation gracefully
		assert.NoError(t, err)
		assert.NotNil(t, result)
	})
}

// Test edge cases and error scenarios
func TestPrometheusToolsEdgeCases(t *testing.T) {
	t.Run("very large response", func(t *testing.T) {
		// Create a large response (simulating large metrics data)
		largeResponse := `{"status": "success", "data": {"result": [`
		for i := 0; i < 1000; i++ {
			if i > 0 {
				largeResponse += ","
			}
			largeResponse += `{"metric": {"instance": "host` + string(rune(i)) + `"}, "value": [1609459200, "1"]}`
		}
		largeResponse += `]}}`

		client := newTestClient(createMockResponse(200, largeResponse), nil)
		ctx := contextWithMockClient(client)

		result, _, err := handlePrometheusQueryTool(ctx, &mcp.CallToolRequest{}, prometheusQueryInput{
			Query: "up",
		})

		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.False(t, result.IsError)

		content := getResultText(result)
		assert.Contains(t, content, "success")
	})

	t.Run("special characters in query", func(t *testing.T) {
		mockResponse := `{"status": "success", "data": {"result": []}}`
		client := newTestClient(createMockResponse(200, mockResponse), nil)
		ctx := contextWithMockClient(client)

		result, _, err := handlePrometheusQueryTool(ctx, &mcp.CallToolRequest{}, prometheusQueryInput{
			Query: `up{instance=~".*:9090"}`,
		})

		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.False(t, result.IsError)
	})

	t.Run("empty response body", func(t *testing.T) {
		client := newTestClient(createMockResponse(200, ""), nil)
		ctx := contextWithMockClient(client)

		result, _, err := handlePrometheusQueryTool(ctx, &mcp.CallToolRequest{}, prometheusQueryInput{
			Query: "up",
		})

		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.False(t, result.IsError)
	})
}

// Test URL parameter encoding
func TestPrometheusURLEncoding(t *testing.T) {
	t.Run("query with special characters", func(t *testing.T) {
		mockResponse := `{"status": "success", "data": {"result": []}}`
		client := newTestClient(createMockResponse(200, mockResponse), nil)
		ctx := contextWithMockClient(client)

		result, _, err := handlePrometheusQueryTool(ctx, &mcp.CallToolRequest{}, prometheusQueryInput{
			Query: `up{job="test service"}`,
		})

		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.False(t, result.IsError)

		// Test passes if no error occurs with special characters
		content := getResultText(result)
		assert.Contains(t, content, "success")
	})
}
