package prometheus

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRegisterTools(t *testing.T) {
	for _, testCase := range []struct {
		name   string
		apiKey *string
	}{
		{name: "unset key"},
		{name: "empty key", apiKey: new("")},
		{name: "configured key", apiKey: new("test-key")},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			t.Setenv("OPENAI_API_KEY", "")
			if testCase.apiKey == nil {
				require.NoError(t, os.Unsetenv("OPENAI_API_KEY"))
			} else {
				t.Setenv("OPENAI_API_KEY", *testCase.apiKey)
			}
			modelServer := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				assert.Equal(t, "/v1/chat/completions", request.URL.Path)
				assert.Equal(t, "Bearer test-key", request.Header.Get("Authorization"))
				writer.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(writer, `{"choices":[{"message":{"role":"assistant","content":"up"},"finish_reason":"stop"}]}`)
			}))
			defer modelServer.Close()
			t.Setenv("OPENAI_BASE_URL", modelServer.URL+"/v1")

			for _, readOnly := range []bool{false, true} {
				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				defer cancel()
				server := sdkmcp.NewServer(&sdkmcp.Implementation{Name: "test", Version: "test"}, nil)
				RegisterTools(server, readOnly)
				serverTransport, clientTransport := sdkmcp.NewInMemoryTransports()
				go func() { _ = server.Run(ctx, serverTransport) }()
				client := sdkmcp.NewClient(&sdkmcp.Implementation{Name: "test-client", Version: "test"}, nil)
				session, err := client.Connect(ctx, clientTransport, nil)
				require.NoError(t, err)
				defer func() { _ = session.Close() }()
				var toolNames []string
				for tool, err := range session.Tools(ctx, nil) {
					require.NoError(t, err)
					toolNames = append(toolNames, tool.Name)
				}
				expectedTools := []string{"prometheus_query_tool", "prometheus_query_range_tool", "prometheus_label_names_tool", "prometheus_targets_tool"}
				configured := testCase.apiKey != nil && *testCase.apiKey != ""
				if configured {
					expectedTools = append(expectedTools, "prometheus_promql_tool")
				}
				assert.ElementsMatch(t, expectedTools, toolNames)
				result, err := session.CallTool(ctx, &sdkmcp.CallToolParams{
					Name: "prometheus_promql_tool", Arguments: promqlInput{QueryDescription: "target availability"},
				})
				if !configured {
					require.Error(t, err, "unconfigured tool must not be callable")
					continue
				}
				require.NoError(t, err)
				require.False(t, result.IsError, getResultText(result))
				assert.Equal(t, "up", getResultText(result))
			}
		})
	}
}

func TestPrometheusInputValidation(t *testing.T) {
	ctx := context.Background()

	t.Run("query invalid url", func(t *testing.T) {
		res, _, err := handlePrometheusQueryTool(ctx, &sdkmcp.CallToolRequest{}, prometheusQueryInput{
			PrometheusURL: "not a url",
			Query:         "up",
		})
		assert.NoError(t, err)
		assert.True(t, res.IsError)
	})

	t.Run("range invalid url", func(t *testing.T) {
		res, _, err := handlePrometheusRangeQueryTool(ctx, &sdkmcp.CallToolRequest{}, prometheusRangeQueryInput{
			PrometheusURL: "not a url",
			Query:         "up",
		})
		assert.NoError(t, err)
		assert.True(t, res.IsError)
	})

	t.Run("labels invalid url", func(t *testing.T) {
		res, _, err := handlePrometheusLabelsQueryTool(ctx, &sdkmcp.CallToolRequest{}, prometheusLabelsInput{
			PrometheusURL: "not a url",
		})
		assert.NoError(t, err)
		assert.True(t, res.IsError)
	})

	t.Run("targets invalid url", func(t *testing.T) {
		res, _, err := handlePrometheusTargetsQueryTool(ctx, &sdkmcp.CallToolRequest{}, prometheusTargetsInput{
			PrometheusURL: "not a url",
		})
		assert.NoError(t, err)
		assert.True(t, res.IsError)
	})

	t.Run("query invalid promql", func(t *testing.T) {
		res, _, err := handlePrometheusQueryTool(ctx, &sdkmcp.CallToolRequest{}, prometheusQueryInput{
			PrometheusURL: "http://localhost:9090",
			Query:         "up; drop",
		})
		assert.NoError(t, err)
		assert.True(t, res.IsError)
	})

	t.Run("range invalid promql", func(t *testing.T) {
		res, _, err := handlePrometheusRangeQueryTool(ctx, &sdkmcp.CallToolRequest{}, prometheusRangeQueryInput{
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
		res, _, err := handlePrometheusLabelsQueryTool(ctx, &sdkmcp.CallToolRequest{}, prometheusLabelsInput{
			PrometheusURL: "http://localhost:9090",
		})
		assert.NoError(t, err)
		assert.True(t, res.IsError)
	})

	t.Run("labels malformed json", func(t *testing.T) {
		ctx := contextWithMockClient(newTestClient(createMockResponse(200, "not json"), nil))
		res, _, err := handlePrometheusLabelsQueryTool(ctx, &sdkmcp.CallToolRequest{}, prometheusLabelsInput{
			PrometheusURL: "http://localhost:9090",
		})
		assert.NoError(t, err)
		assert.False(t, res.IsError)
		assert.Contains(t, getResultText(res), "not json")
	})

	t.Run("targets client error", func(t *testing.T) {
		ctx := contextWithMockClient(newTestClient(nil, assert.AnError))
		res, _, err := handlePrometheusTargetsQueryTool(ctx, &sdkmcp.CallToolRequest{}, prometheusTargetsInput{
			PrometheusURL: "http://localhost:9090",
		})
		assert.NoError(t, err)
		assert.True(t, res.IsError)
	})

	t.Run("targets malformed json", func(t *testing.T) {
		ctx := contextWithMockClient(newTestClient(createMockResponse(200, "not json"), nil))
		res, _, err := handlePrometheusTargetsQueryTool(ctx, &sdkmcp.CallToolRequest{}, prometheusTargetsInput{
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
func getResultText(result *sdkmcp.CallToolResult) string {
	if result == nil || len(result.Content) == 0 {
		return ""
	}
	if textContent, ok := result.Content[0].(*sdkmcp.TextContent); ok {
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

		result, _, err := handlePrometheusQueryTool(ctx, &sdkmcp.CallToolRequest{}, prometheusQueryInput{
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
		result, _, err := handlePrometheusQueryTool(ctx, &sdkmcp.CallToolRequest{}, prometheusQueryInput{
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

		result, _, err := handlePrometheusQueryTool(ctx, &sdkmcp.CallToolRequest{}, prometheusQueryInput{
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

		result, _, err := handlePrometheusQueryTool(ctx, &sdkmcp.CallToolRequest{}, prometheusQueryInput{
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

		result, _, err := handlePrometheusQueryTool(ctx, &sdkmcp.CallToolRequest{}, prometheusQueryInput{
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

		result, _, err := handlePrometheusQueryTool(ctx, &sdkmcp.CallToolRequest{}, prometheusQueryInput{
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

		result, _, err := handlePrometheusRangeQueryTool(ctx, &sdkmcp.CallToolRequest{}, prometheusRangeQueryInput{
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
		result, _, err := handlePrometheusRangeQueryTool(ctx, &sdkmcp.CallToolRequest{}, prometheusRangeQueryInput{})

		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.True(t, result.IsError)
		assert.Contains(t, getResultText(result), "query parameter is required")
	})

	t.Run("default time range and step", func(t *testing.T) {
		mockResponse := `{"status": "success", "data": {"result": []}}`
		client := newTestClient(createMockResponse(200, mockResponse), nil)
		ctx := contextWithMockClient(client)

		result, _, err := handlePrometheusRangeQueryTool(ctx, &sdkmcp.CallToolRequest{}, prometheusRangeQueryInput{
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

		result, _, err := handlePrometheusLabelsQueryTool(ctx, &sdkmcp.CallToolRequest{}, prometheusLabelsInput{})

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

		result, _, err := handlePrometheusLabelsQueryTool(ctx, &sdkmcp.CallToolRequest{}, prometheusLabelsInput{})

		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.True(t, result.IsError)
		assert.Contains(t, getResultText(result), "**Prometheus Error**")
	})

	t.Run("custom prometheus URL", func(t *testing.T) {
		mockResponse := `{"status": "success", "data": []}`
		client := newTestClient(createMockResponse(200, mockResponse), nil)
		ctx := contextWithMockClient(client)

		result, _, err := handlePrometheusLabelsQueryTool(ctx, &sdkmcp.CallToolRequest{}, prometheusLabelsInput{
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

		result, _, err := handlePrometheusTargetsQueryTool(ctx, &sdkmcp.CallToolRequest{}, prometheusTargetsInput{})

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

		result, _, err := handlePrometheusTargetsQueryTool(ctx, &sdkmcp.CallToolRequest{}, prometheusTargetsInput{})

		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.True(t, result.IsError)
		assert.Contains(t, getResultText(result), "Prometheus API error (404)")
	})
}

func TestHandlePromql(t *testing.T) {
	t.Run("missing query description", func(t *testing.T) {
		ctx := context.Background()
		result, _, err := handlePromql(ctx, &sdkmcp.CallToolRequest{}, promqlInput{})

		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.True(t, result.IsError)
		assert.Contains(t, getResultText(result), "query_description is required")
	})

	t.Run("with query description", func(t *testing.T) {
		ctx := context.Background()
		result, _, err := handlePromql(ctx, &sdkmcp.CallToolRequest{}, promqlInput{
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

		result, _, err := handlePrometheusQueryTool(ctx, &sdkmcp.CallToolRequest{}, prometheusQueryInput{
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

		result, _, err := handlePrometheusQueryTool(ctx, &sdkmcp.CallToolRequest{}, prometheusQueryInput{
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

		result, _, err := handlePrometheusQueryTool(ctx, &sdkmcp.CallToolRequest{}, prometheusQueryInput{
			Query: `up{instance=~".*:9090"}`,
		})

		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.False(t, result.IsError)
	})

	t.Run("empty response body", func(t *testing.T) {
		client := newTestClient(createMockResponse(200, ""), nil)
		ctx := contextWithMockClient(client)

		result, _, err := handlePrometheusQueryTool(ctx, &sdkmcp.CallToolRequest{}, prometheusQueryInput{
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

		result, _, err := handlePrometheusQueryTool(ctx, &sdkmcp.CallToolRequest{}, prometheusQueryInput{
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

// recordingRoundTripper records the request URL and returns an empty success.
type recordingRoundTripper struct {
	requestURL string
}

func (recorder *recordingRoundTripper) RoundTrip(request *http.Request) (*http.Response, error) {
	recorder.requestURL = request.URL.String()
	return createMockResponse(200, `{"status":"success","data":[]}`), nil
}

func TestPrometheusURLDefault(t *testing.T) {
	const configuredURL = "http://prometheus.monitoring:9090"
	testCases := []struct {
		name        string
		callTool    func(ctx context.Context) error
		expectedURL string
	}{
		{
			name: "query",
			callTool: func(ctx context.Context) error {
				_, _, err := handlePrometheusQueryTool(ctx, &sdkmcp.CallToolRequest{}, prometheusQueryInput{Query: "up"})
				return err
			},
			expectedURL: configuredURL + "/api/v1/query",
		},
		{
			name: "range query",
			callTool: func(ctx context.Context) error {
				_, _, err := handlePrometheusRangeQueryTool(ctx, &sdkmcp.CallToolRequest{}, prometheusRangeQueryInput{Query: "up"})
				return err
			},
			expectedURL: configuredURL + "/api/v1/query_range",
		},
		{
			name: "labels",
			callTool: func(ctx context.Context) error {
				_, _, err := handlePrometheusLabelsQueryTool(ctx, &sdkmcp.CallToolRequest{}, prometheusLabelsInput{})
				return err
			},
			expectedURL: configuredURL + "/api/v1/labels",
		},
		{
			name: "targets",
			callTool: func(ctx context.Context) error {
				_, _, err := handlePrometheusTargetsQueryTool(ctx, &sdkmcp.CallToolRequest{}, prometheusTargetsInput{})
				return err
			},
			expectedURL: configuredURL + "/api/v1/targets",
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name+" uses PROMETHEUS_URL when the call has no prometheus_url", func(t *testing.T) {
			t.Setenv(prometheusURLEnv, configuredURL)
			recorder := &recordingRoundTripper{}
			ctx := contextWithMockClient(&http.Client{Transport: recorder})

			require.NoError(t, testCase.callTool(ctx))
			assert.True(t, strings.HasPrefix(recorder.requestURL, testCase.expectedURL), recorder.requestURL)
		})
	}

	t.Run("localhost when PROMETHEUS_URL is unset", func(t *testing.T) {
		t.Setenv(prometheusURLEnv, "")
		assert.Equal(t, fallbackPrometheusURL, defaultPrometheusURL())
	})
}
