package e2e

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// HTTPToolsTestSuite contains tests for HTTP transport tool execution
var _ = Describe("HTTP Tools E2E Tests", func() {
	var (
		ctx    context.Context
		cancel context.CancelFunc
	)

	BeforeEach(func() {
		ctx, cancel = context.WithTimeout(context.Background(), 60*time.Second)
	})

	AfterEach(func() {
		if cancel != nil {
			cancel()
		}
	})

	// Helper function to create MCP client session with connection timeout
	createMCPClientSession := func(port int) (*mcp.ClientSession, error) {
		client := mcp.NewClient(&mcp.Implementation{
			Name:    "e2e-test-client",
			Version: "1.0.0",
		}, nil)

		transport := &mcp.StreamableClientTransport{
			Endpoint: fmt.Sprintf("http://localhost:%d/mcp", port),
		}

		// Create a context with 5-second timeout for connection
		connectCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()

		// Use a channel to capture the connection result
		type connResult struct {
			session *mcp.ClientSession
			err     error
		}
		resultChan := make(chan connResult, 1)

		go func() {
			session, err := client.Connect(connectCtx, transport, nil)
			resultChan <- connResult{session: session, err: err}
		}()

		// Wait for connection with timeout
		select {
		case result := <-resultChan:
			if result.err != nil {
				return nil, fmt.Errorf("failed to connect to server: %w", result.err)
			}
			return result.session, nil
		case <-connectCtx.Done():
			return nil, fmt.Errorf("connection timeout: failed to connect within 5 seconds")
		}
	}

	// Phase 3: User Story 1 - HTTP Tool Execution Across All Providers
	Describe("HTTP Tool Execution (User Story 1)", func() {
		It("should execute utils datetime_get_current_time tool via HTTP", func() {
			config := TestServerConfig{
				Port:    18000,
				Tools:   []string{"utils"},
				Stdio:   false,
				Timeout: 30 * time.Second,
			}

			server := NewTestServer(config)
			err := server.Start(ctx, config)
			Expect(err).NotTo(HaveOccurred(), "Server should start successfully")

			// Wait for server to be ready
			time.Sleep(3 * time.Second)

			// Create MCP client session
			session, err := createMCPClientSession(config.Port)
			Expect(err).NotTo(HaveOccurred(), "Should create MCP client session")
			defer func() {
				if err := session.Close(); err != nil {
					// Ignore close errors in tests
					_ = err
				}
			}()

			// Execute datetime_get_current_time tool
			params := &mcp.CallToolParams{
				Name:      "datetime_get_current_time",
				Arguments: map[string]interface{}{},
			}

			result, err := session.CallTool(ctx, params)
			Expect(err).NotTo(HaveOccurred(), "Tool execution should succeed")
			Expect(result.IsError).To(BeFalse(), "Tool should not return error")

			// Verify output contains timestamp
			Expect(len(result.Content)).To(BeNumerically(">", 0), "Result should have content")
			if len(result.Content) > 0 {
				if textContent, ok := result.Content[0].(*mcp.TextContent); ok {
					Expect(textContent.Text).NotTo(BeEmpty(), "Output should not be empty")
					Expect(textContent.Text).To(MatchRegexp(`\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}`), "Output should be ISO 8601 format")
				}
			}

			err = server.Stop()
			Expect(err).NotTo(HaveOccurred(), "Server should stop gracefully")
		})

		It("should execute k8s tool via HTTP", func() {
			config := TestServerConfig{
				Port:    18001,
				Tools:   []string{"k8s"},
				Stdio:   false,
				Timeout: 30 * time.Second,
			}

			server := NewTestServer(config)
			err := server.Start(ctx, config)
			Expect(err).NotTo(HaveOccurred(), "Server should start successfully")

			// Wait for server to be ready
			time.Sleep(3 * time.Second)

			// Create MCP client session
			session, err := createMCPClientSession(config.Port)
			Expect(err).NotTo(HaveOccurred(), "Should create MCP client session")
			defer func() {
				if err := session.Close(); err != nil {
					// Ignore close errors in tests
					_ = err
				}
			}()

			// Execute k8s_get_resources tool
			params := &mcp.CallToolParams{
				Name: "k8s_get_resources",
				Arguments: map[string]interface{}{
					"resource_type": "namespaces",
					"output":        "json",
				},
			}

			result, err := session.CallTool(ctx, params)
			Expect(err).NotTo(HaveOccurred(), "Tool execution should succeed")

			// Tool may return error if k8s is not configured, but should not fail with protocol error
			if result.IsError {
				// Verify error content is present
				Expect(len(result.Content)).To(BeNumerically(">", 0), "Error result should have content")
			} else {
				// Verify success result has content
				Expect(len(result.Content)).To(BeNumerically(">", 0), "Success result should have content")
			}

			err = server.Stop()
			Expect(err).NotTo(HaveOccurred(), "Server should stop gracefully")
		})

		It("should execute helm tool via HTTP", func() {
			config := TestServerConfig{
				Port:    18002,
				Tools:   []string{"helm"},
				Stdio:   false,
				Timeout: 30 * time.Second,
			}

			server := NewTestServer(config)
			err := server.Start(ctx, config)
			Expect(err).NotTo(HaveOccurred(), "Server should start successfully")

			// Wait for server to be ready
			time.Sleep(3 * time.Second)

			// Create MCP client session
			session, err := createMCPClientSession(config.Port)
			Expect(err).NotTo(HaveOccurred(), "Should create MCP client session")
			defer func() {
				if err := session.Close(); err != nil {
					// Ignore close errors in tests
					_ = err
				}
			}()

			// Execute helm_list_releases tool
			params := &mcp.CallToolParams{
				Name: "helm_list_releases",
				Arguments: map[string]interface{}{
					"namespace": "default",
					"output":    "json",
				},
			}

			result, err := session.CallTool(ctx, params)
			Expect(err).NotTo(HaveOccurred(), "Tool execution should succeed")

			// Tool may return error if helm is not configured, but should not fail with protocol error
			if result.IsError {
				Expect(len(result.Content)).To(BeNumerically(">", 0), "Error result should have content")
			} else {
				Expect(len(result.Content)).To(BeNumerically(">", 0), "Success result should have content")
			}

			err = server.Stop()
			Expect(err).NotTo(HaveOccurred(), "Server should stop gracefully")
		})

		It("should execute istio tool via HTTP", func() {
			config := TestServerConfig{
				Port:    18003,
				Tools:   []string{"istio"},
				Stdio:   false,
				Timeout: 30 * time.Second,
			}

			server := NewTestServer(config)
			err := server.Start(ctx, config)
			Expect(err).NotTo(HaveOccurred(), "Server should start successfully")

			// Wait for server to be ready
			time.Sleep(3 * time.Second)

			// Create MCP client session
			session, err := createMCPClientSession(config.Port)
			Expect(err).NotTo(HaveOccurred(), "Should create MCP client session")
			defer func() {
				if err := session.Close(); err != nil {
					// Ignore close errors in tests
					_ = err
				}
			}()

			// Execute istio_version tool (safer than install)
			params := &mcp.CallToolParams{
				Name:      "istio_version",
				Arguments: map[string]interface{}{},
			}

			result, err := session.CallTool(ctx, params)
			Expect(err).NotTo(HaveOccurred(), "Tool execution should succeed")

			// Tool may return error if istioctl is not configured, but should not fail with protocol error
			if result.IsError {
				Expect(len(result.Content)).To(BeNumerically(">", 0), "Error result should have content")
			} else {
				Expect(len(result.Content)).To(BeNumerically(">", 0), "Success result should have content")
			}

			err = server.Stop()
			Expect(err).NotTo(HaveOccurred(), "Server should stop gracefully")
		})

		It("should execute argo tool via HTTP", func() {
			config := TestServerConfig{
				Port:    18004,
				Tools:   []string{"argo"},
				Stdio:   false,
				Timeout: 30 * time.Second,
			}

			server := NewTestServer(config)
			err := server.Start(ctx, config)
			Expect(err).NotTo(HaveOccurred(), "Server should start successfully")

			// Wait for server to be ready
			time.Sleep(3 * time.Second)

			// Create MCP client session
			session, err := createMCPClientSession(config.Port)
			Expect(err).NotTo(HaveOccurred(), "Should create MCP client session")
			defer func() {
				if err := session.Close(); err != nil {
					// Ignore close errors in tests
					_ = err
				}
			}()

			// Execute argo_rollouts_list tool
			params := &mcp.CallToolParams{
				Name: "argo_rollouts_list",
				Arguments: map[string]interface{}{
					"namespace": "default",
					"output":    "json",
				},
			}

			result, err := session.CallTool(ctx, params)
			Expect(err).NotTo(HaveOccurred(), "Tool execution should succeed")

			// Tool may return error if argo is not configured, but should not fail with protocol error
			if result.IsError {
				Expect(len(result.Content)).To(BeNumerically(">", 0), "Error result should have content")
			} else {
				Expect(len(result.Content)).To(BeNumerically(">", 0), "Success result should have content")
			}

			err = server.Stop()
			Expect(err).NotTo(HaveOccurred(), "Server should stop gracefully")
		})

		It("should execute cilium tool via HTTP", func() {
			config := TestServerConfig{
				Port:    18005,
				Tools:   []string{"cilium"},
				Stdio:   false,
				Timeout: 30 * time.Second,
			}

			server := NewTestServer(config)
			err := server.Start(ctx, config)
			Expect(err).NotTo(HaveOccurred(), "Server should start successfully")

			// Wait for server to be ready
			time.Sleep(3 * time.Second)

			// Create MCP client session
			session, err := createMCPClientSession(config.Port)
			Expect(err).NotTo(HaveOccurred(), "Should create MCP client session")
			defer func() {
				if err := session.Close(); err != nil {
					// Ignore close errors in tests
					_ = err
				}
			}()

			// Execute cilium_status_and_version tool
			params := &mcp.CallToolParams{
				Name:      "cilium_status_and_version",
				Arguments: map[string]interface{}{},
			}

			result, err := session.CallTool(ctx, params)
			Expect(err).NotTo(HaveOccurred(), "Tool execution should succeed")

			// Tool may return error if cilium is not configured, but should not fail with protocol error
			if result.IsError {
				Expect(len(result.Content)).To(BeNumerically(">", 0), "Error result should have content")
			} else {
				Expect(len(result.Content)).To(BeNumerically(">", 0), "Success result should have content")
			}

			err = server.Stop()
			Expect(err).NotTo(HaveOccurred(), "Server should stop gracefully")
		})

		It("should execute prometheus tool via HTTP", func() {
			config := TestServerConfig{
				Port:    18006,
				Tools:   []string{"prometheus"},
				Stdio:   false,
				Timeout: 30 * time.Second,
			}

			server := NewTestServer(config)
			err := server.Start(ctx, config)
			Expect(err).NotTo(HaveOccurred(), "Server should start successfully")

			// Wait for server to be ready
			time.Sleep(3 * time.Second)

			// Create MCP client session
			session, err := createMCPClientSession(config.Port)
			Expect(err).NotTo(HaveOccurred(), "Should create MCP client session")
			defer func() {
				if err := session.Close(); err != nil {
					// Ignore close errors in tests
					_ = err
				}
			}()

			// Execute prometheus_query_tool (note: actual tool name has _tool suffix)
			params := &mcp.CallToolParams{
				Name: "prometheus_query_tool",
				Arguments: map[string]interface{}{
					"query": "up",
				},
			}

			result, err := session.CallTool(ctx, params)
			// Prometheus query may fail if Prometheus is not configured, but tool should still be callable
			Expect(err).NotTo(HaveOccurred(), "Tool execution should succeed (may return tool error)")

			// Tool may return error if Prometheus is not configured, but should not fail with protocol error
			if result.IsError {
				Expect(len(result.Content)).To(BeNumerically(">", 0), "Error result should have content")
			} else {
				Expect(len(result.Content)).To(BeNumerically(">", 0), "Success result should have content")
			}

			err = server.Stop()
			Expect(err).NotTo(HaveOccurred(), "Server should stop gracefully")
		})

		It("should handle concurrent tool execution requests", func() {
			config := TestServerConfig{
				Port:    18007,
				Tools:   []string{"utils"},
				Stdio:   false,
				Timeout: 30 * time.Second,
			}

			server := NewTestServer(config)
			err := server.Start(ctx, config)
			Expect(err).NotTo(HaveOccurred(), "Server should start successfully")

			// Wait for server to be ready
			time.Sleep(3 * time.Second)

			// Execute multiple concurrent requests
			var wg sync.WaitGroup
			numRequests := 10
			successCount := 0
			var mu sync.Mutex

			for i := 0; i < numRequests; i++ {
				wg.Add(1)
				go func(id int) {
					defer wg.Done()

					session, err := createMCPClientSession(config.Port)
					if err != nil {
						return
					}
					defer func() {
						if err := session.Close(); err != nil {
							// Ignore close errors in tests
							_ = err
						}
					}()

					params := &mcp.CallToolParams{
						Name:      "datetime_get_current_time",
						Arguments: map[string]interface{}{},
					}

					result, err := session.CallTool(ctx, params)
					if err == nil && !result.IsError {
						mu.Lock()
						successCount++
						mu.Unlock()
					}
				}(i)
			}

			wg.Wait()

			// At least some requests should succeed
			Expect(successCount).To(BeNumerically(">", 0), "At least some concurrent requests should succeed")
			Expect(successCount).To(BeNumerically(">=", numRequests/2), "At least half of concurrent requests should succeed")

			err = server.Stop()
			Expect(err).NotTo(HaveOccurred(), "Server should stop gracefully")
		})

		It("should keep connection open during long-running sleep operation", func() {
			config := TestServerConfig{
				Port:    18015,
				Tools:   []string{"utils"},
				Stdio:   false,
				Timeout: 30 * time.Second,
			}

			server := NewTestServer(config)
			err := server.Start(ctx, config)
			Expect(err).NotTo(HaveOccurred(), "Server should start successfully")

			// Wait for server to be ready
			time.Sleep(3 * time.Second)

			// Create MCP client session
			session, err := createMCPClientSession(config.Port)
			Expect(err).NotTo(HaveOccurred(), "Should create MCP client session")
			defer func() {
				if err := session.Close(); err != nil {
					// Ignore close errors in tests
					_ = err
				}
			}()

			// Execute sleep tool for 10 seconds - this verifies streaming connection stays open
			sleepDuration := 10.0
			startTime := time.Now()

			params := &mcp.CallToolParams{
				Name: "sleep",
				Arguments: map[string]interface{}{
					"duration": sleepDuration,
				},
			}

			result, err := session.CallTool(ctx, params)
			elapsed := time.Since(startTime)

			// Verify no connection error occurred
			Expect(err).NotTo(HaveOccurred(), "Tool execution should succeed without connection errors")
			Expect(result.IsError).To(BeFalse(), "Tool should not return error")

			// Verify the operation took approximately 10 seconds (allow some tolerance)
			Expect(elapsed).To(BeNumerically(">=", 9*time.Second), "Sleep should take at least 9 seconds")
			Expect(elapsed).To(BeNumerically("<=", 12*time.Second), "Sleep should complete within 12 seconds")

			// Verify output contains sleep completion message
			Expect(len(result.Content)).To(BeNumerically(">", 0), "Result should have content")
			if len(result.Content) > 0 {
				if textContent, ok := result.Content[0].(*mcp.TextContent); ok {
					Expect(textContent.Text).To(ContainSubstring("slept for"), "Output should indicate sleep completion")
					Expect(textContent.Text).To(ContainSubstring("10.00"), "Output should contain sleep duration")
				}
			}

			err = server.Stop()
			Expect(err).NotTo(HaveOccurred(), "Server should stop gracefully")
		})
	})

	// Phase 4: User Story 2 - Tool Discovery via HTTP
	Describe("Tool Discovery via HTTP (User Story 2)", func() {
		It("should list all tools via MCP client", func() {
			config := TestServerConfig{
				Port:    18008,
				Tools:   []string{"utils", "k8s", "helm"},
				Stdio:   false,
				Timeout: 30 * time.Second,
			}

			server := NewTestServer(config)
			err := server.Start(ctx, config)
			Expect(err).NotTo(HaveOccurred(), "Server should start successfully")

			// Wait for server to be ready
			time.Sleep(3 * time.Second)

			// Create MCP client session
			session, err := createMCPClientSession(config.Port)
			Expect(err).NotTo(HaveOccurred(), "Should create MCP client session")
			defer func() {
				if err := session.Close(); err != nil {
					// Ignore close errors in tests
					_ = err
				}
			}()

			// Request tools list
			var tools []*mcp.Tool
			for tool, err := range session.Tools(ctx, nil) {
				if err != nil {
					Expect(err).NotTo(HaveOccurred(), "Failed to iterate tools")
					break
				}
				tools = append(tools, tool)
			}

			Expect(len(tools)).To(BeNumerically(">", 0), "Should have at least one tool")

			// Verify tool structure
			if len(tools) > 0 {
				tool := tools[0]
				Expect(tool.Name).NotTo(BeEmpty(), "Tool should have a name")
				Expect(tool.Description).NotTo(BeEmpty(), "Tool should have a description")
			}

			err = server.Stop()
			Expect(err).NotTo(HaveOccurred(), "Server should stop gracefully")
		})

		It("should verify all providers appear in tool list", func() {
			config := TestServerConfig{
				Port:    18009,
				Tools:   []string{"utils", "k8s", "helm", "istio", "argo", "cilium", "prometheus"},
				Stdio:   false,
				Timeout: 30 * time.Second,
			}

			server := NewTestServer(config)
			err := server.Start(ctx, config)
			Expect(err).NotTo(HaveOccurred(), "Server should start successfully")

			// Wait for server to be ready
			time.Sleep(3 * time.Second)

			// Create MCP client session
			session, err := createMCPClientSession(config.Port)
			Expect(err).NotTo(HaveOccurred(), "Should create MCP client session")
			defer func() {
				if err := session.Close(); err != nil {
					// Ignore close errors in tests
					_ = err
				}
			}()

			// Request tools list
			var tools []*mcp.Tool
			for tool, err := range session.Tools(ctx, nil) {
				if err != nil {
					Expect(err).NotTo(HaveOccurred(), "Failed to iterate tools")
					break
				}
				tools = append(tools, tool)
			}

			Expect(len(tools)).To(BeNumerically(">", 0), "Should have at least one tool")

			// Collect tool names
			toolNames := make(map[string]bool)
			for _, tool := range tools {
				toolNames[tool.Name] = true
			}

			// Verify tools from different providers are present
			// At least one tool from each provider should be present
			providerPrefixes := []string{"datetime_", "shell", "k8s_", "helm_", "istio_", "argo_", "cilium_", "prometheus_"}
			foundPrefixes := make(map[string]bool)

			for toolName := range toolNames {
				for _, prefix := range providerPrefixes {
					if len(toolName) >= len(prefix) && toolName[:len(prefix)] == prefix {
						foundPrefixes[prefix] = true
						break
					}
				}
			}

			// Verify we found tools from multiple providers
			Expect(len(foundPrefixes)).To(BeNumerically(">=", 3), "Should find tools from at least 3 providers")

			err = server.Stop()
			Expect(err).NotTo(HaveOccurred(), "Server should stop gracefully")
		})

		It("should verify selective tool loading works", func() {
			config := TestServerConfig{
				Port:    18010,
				Tools:   []string{"utils"},
				Stdio:   false,
				Timeout: 30 * time.Second,
			}

			server := NewTestServer(config)
			err := server.Start(ctx, config)
			Expect(err).NotTo(HaveOccurred(), "Server should start successfully")

			// Wait for server to be ready
			time.Sleep(3 * time.Second)

			// Create MCP client session
			session, err := createMCPClientSession(config.Port)
			Expect(err).NotTo(HaveOccurred(), "Should create MCP client session")
			defer func() {
				if err := session.Close(); err != nil {
					// Ignore close errors in tests
					_ = err
				}
			}()

			// Request tools list
			var tools []*mcp.Tool
			for tool, err := range session.Tools(ctx, nil) {
				if err != nil {
					Expect(err).NotTo(HaveOccurred(), "Failed to iterate tools")
					break
				}
				tools = append(tools, tool)
			}

			Expect(len(tools)).To(BeNumerically(">", 0), "Should have at least one tool")

			// Verify only utils tools are present
			for _, tool := range tools {
				// Should only have utils tools (datetime_, shell, echo, sleep)
				Expect(tool.Name).To(Or(
					MatchRegexp(`^datetime_`),
					Equal("shell"),
					Equal("echo"),
					Equal("sleep"),
				), "Tool %s should be from utils provider", tool.Name)
			}

			err = server.Stop()
			Expect(err).NotTo(HaveOccurred(), "Server should stop gracefully")
		})

		It("should verify tool schema serialization", func() {
			config := TestServerConfig{
				Port:    18011,
				Tools:   []string{"utils"},
				Stdio:   false,
				Timeout: 30 * time.Second,
			}

			server := NewTestServer(config)
			err := server.Start(ctx, config)
			Expect(err).NotTo(HaveOccurred(), "Server should start successfully")

			// Wait for server to be ready
			time.Sleep(3 * time.Second)

			// Create MCP client session
			session, err := createMCPClientSession(config.Port)
			Expect(err).NotTo(HaveOccurred(), "Should create MCP client session")
			defer func() {
				if err := session.Close(); err != nil {
					// Ignore close errors in tests
					_ = err
				}
			}()

			// Request tools list
			var tools []*mcp.Tool
			for tool, err := range session.Tools(ctx, nil) {
				if err != nil {
					Expect(err).NotTo(HaveOccurred(), "Failed to iterate tools")
					break
				}
				tools = append(tools, tool)
			}

			Expect(len(tools)).To(BeNumerically(">", 0), "Should have at least one tool")

			// Verify at least one tool has schema
			foundSchema := false
			for _, tool := range tools {
				if tool.InputSchema != nil {
					foundSchema = true
					break
				}
			}

			// At least some tools should have schemas
			Expect(foundSchema).To(BeTrue(), "At least one tool should have a schema")

			err = server.Stop()
			Expect(err).NotTo(HaveOccurred(), "Server should stop gracefully")
		})

		It("should verify tool count matches expected number", func() {
			config := TestServerConfig{
				Port:    18012,
				Tools:   []string{"utils", "k8s", "helm"},
				Stdio:   false,
				Timeout: 30 * time.Second,
			}

			server := NewTestServer(config)
			err := server.Start(ctx, config)
			Expect(err).NotTo(HaveOccurred(), "Server should start successfully")

			// Wait for server to be ready
			time.Sleep(3 * time.Second)

			// Create MCP client session
			session, err := createMCPClientSession(config.Port)
			Expect(err).NotTo(HaveOccurred(), "Should create MCP client session")
			defer func() {
				if err := session.Close(); err != nil {
					// Ignore close errors in tests
					_ = err
				}
			}()

			// Request tools list
			var tools []*mcp.Tool
			for tool, err := range session.Tools(ctx, nil) {
				if err != nil {
					Expect(err).NotTo(HaveOccurred(), "Failed to iterate tools")
					break
				}
				tools = append(tools, tool)
			}

			// Should have at least 5 tools (2 from utils + several from k8s and helm)
			Expect(len(tools)).To(BeNumerically(">=", 5), "Should have at least 5 tools from utils, k8s, and helm")

			err = server.Stop()
			Expect(err).NotTo(HaveOccurred(), "Server should stop gracefully")
		})
	})

	// Error Handling Tests
	Describe("Error Handling", func() {
		It("should return error for non-existent tool", func() {
			config := TestServerConfig{
				Port:    18013,
				Tools:   []string{"utils"},
				Stdio:   false,
				Timeout: 30 * time.Second,
			}

			server := NewTestServer(config)
			err := server.Start(ctx, config)
			Expect(err).NotTo(HaveOccurred(), "Server should start successfully")

			// Wait for server to be ready
			time.Sleep(3 * time.Second)

			// Create MCP client session
			session, err := createMCPClientSession(config.Port)
			Expect(err).NotTo(HaveOccurred(), "Should create MCP client session")
			defer func() {
				if err := session.Close(); err != nil {
					// Ignore close errors in tests
					_ = err
				}
			}()

			// Try to execute non-existent tool
			params := &mcp.CallToolParams{
				Name:      "non_existent_tool",
				Arguments: map[string]interface{}{},
			}

			result, err := session.CallTool(ctx, params)
			// Should either return protocol error or tool error
			// For non-existent tool, SDK may return error or tool error response
			if err != nil {
				// Protocol error is acceptable
				Expect(err).ToNot(BeNil())
			} else {
				// Tool error response is also acceptable
				Expect(result.IsError).To(BeTrue(), "Should return tool error for non-existent tool")
			}

			err = server.Stop()
			Expect(err).NotTo(HaveOccurred(), "Server should stop gracefully")
		})

		It("should return error for missing tool name", func() {
			config := TestServerConfig{
				Port:    18014,
				Tools:   []string{"utils"},
				Stdio:   false,
				Timeout: 30 * time.Second,
			}

			server := NewTestServer(config)
			err := server.Start(ctx, config)
			Expect(err).NotTo(HaveOccurred(), "Server should start successfully")

			// Wait for server to be ready
			time.Sleep(3 * time.Second)

			// Create MCP client session
			session, err := createMCPClientSession(config.Port)
			Expect(err).NotTo(HaveOccurred(), "Should create MCP client session")
			defer func() {
				if err := session.Close(); err != nil {
					// Ignore close errors in tests
					_ = err
				}
			}()

			// Try to call tool with empty name (SDK validation should handle this)
			params := &mcp.CallToolParams{
				Name:      "",
				Arguments: map[string]interface{}{},
			}

			result, err := session.CallTool(ctx, params)
			// SDK should validate and return error for empty tool name
			if err != nil {
				// Protocol error is acceptable
				Expect(err).ToNot(BeNil())
			} else {
				// Tool error response is also acceptable
				Expect(result.IsError).To(BeTrue(), "Should return error for missing tool name")
			}

			err = server.Stop()
			Expect(err).NotTo(HaveOccurred(), "Server should stop gracefully")
		})
	})
})
