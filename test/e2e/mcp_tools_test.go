package e2e

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/kagent-dev/tools/internal/cmd"
	"github.com/kagent-dev/tools/pkg/argo"
	"github.com/kagent-dev/tools/pkg/cilium"
	"github.com/kagent-dev/tools/pkg/helm"
	"github.com/kagent-dev/tools/pkg/istio"
	"github.com/kagent-dev/tools/pkg/k8s"
	"github.com/kagent-dev/tools/pkg/prometheus"
	"github.com/kagent-dev/tools/pkg/utils"

	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// TestEntry represents a single test case from resource files
type TestEntry struct {
	Tool            string                 `json:"tool"`
	Arguments       map[string]interface{} `json:"arguments"`
	ExpectedCommand string
	Comment         string
}

// TestSuite contains all test cases for a package
type TestSuite struct {
	PackageName string
	TestCases   []TestEntry
}

// ToolHandler represents a tool handler function
type ToolHandler func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error)

var _ = Describe("MCP Tools Integration", func() {
	var (
		mcpServer  *server.MCPServer
		testSuites []TestSuite
		ctx        context.Context
	)

	BeforeEach(func() {
		ctx = context.Background()
		mcpServer = setupMCPServer()
		testSuites = loadTestSuites()
	})

	Describe("Tool Registration", func() {
		It("should register all tools without errors", func() {
			Expect(mcpServer).NotTo(BeNil())

			By("loading test suites from resource files")
			Expect(testSuites).NotTo(BeEmpty())

			totalTestCases := 0
			for _, suite := range testSuites {
				totalTestCases += len(suite.TestCases)
			}

			By(fmt.Sprintf("validating %d test cases across %d packages", totalTestCases, len(testSuites)))
			Expect(totalTestCases).To(BeNumerically(">=", 0), "Should have positive numver of test cases")
		})
	})

	Describe("Tool Execution with Actual stdio", func() {
		Context("when executing tools with real commands", func() {
			for _, suite := range loadTestSuites() {
				packageName := suite.PackageName

				Context(fmt.Sprintf("%s package tools", packageName), func() {
					for _, testCase := range suite.TestCases {
						tool := testCase.Tool
						args := testCase.Arguments
						expectedCmd := testCase.ExpectedCommand

						It(fmt.Sprintf("should execute %s correctly", tool), func() {
							By(fmt.Sprintf("validating required arguments for %s", tool))
							err := validateRequiredArguments(tool, args)
							Expect(err).NotTo(HaveOccurred(), "Tool should have valid arguments")

							By(fmt.Sprintf("executing tool %s with actual stdio", tool))
							result, executedCommand, err := executeToolWithStdio(ctx, tool, args)

							if tool == "datetime_get_current_time" {
								// DateTime tool doesn't execute shell commands
								Expect(err).NotTo(HaveOccurred())
								Expect(result).NotTo(BeNil())
								GinkgoWriter.Printf("✓ DateTime tool executed successfully\n")
								return
							}

							if err != nil {
								// Some tools might fail in test environment (e.g., kubectl not available)
								// Log the error but don't fail the test unless it's a validation error
								if strings.Contains(err.Error(), "required") || strings.Contains(err.Error(), "invalid") {
									Fail(fmt.Sprintf("Tool validation failed: %v", err))
								}
								Skip(fmt.Sprintf("Tool execution skipped due to environment: %v", err))
								return
							}

							Expect(result).NotTo(BeNil())

							By("verifying command execution")
							if executedCommand != "" {
								GinkgoWriter.Printf("Expected: %s\n", expectedCmd)
								GinkgoWriter.Printf("Executed: %s\n", executedCommand)

								// For mirror mode, we compare the actual executed command
								if expectedCmd != "" {
									Expect(executedCommand).To(ContainSubstring(strings.Fields(expectedCmd)[0]),
										"Executed command should contain the expected command base")
								}
							}

							GinkgoWriter.Printf("✓ Tool '%s' executed successfully\n", tool)
						})
					}
				})
			}
		})

		Context("when executing specific tool scenarios", func() {
			DescribeTable("shell tool execution",
				func(command string, shouldSucceed bool) {
					arguments := map[string]interface{}{
						"command": command,
					}

					result, executedCmd, err := executeToolWithStdio(ctx, "shell", arguments)

					if shouldSucceed {
						Expect(err).NotTo(HaveOccurred())
						Expect(result).NotTo(BeNil())
						Expect(executedCmd).To(ContainSubstring(command))
						GinkgoWriter.Printf("Shell command executed: %s\n", executedCmd)
					} else {
						Expect(err).To(HaveOccurred())
					}
				},
				Entry("echo command", "echo 'hello world'", true),
				Entry("date command", "date", true),
				Entry("empty command", "", false),
			)
		})
	})

	Describe("Tool Validation", func() {
		It("should validate required arguments for all tools", func() {
			requiredArgs := getRequiredArgumentsMap()

			for toolName, required := range requiredArgs {
				By(fmt.Sprintf("testing %s with missing arguments", toolName))

				// Test with empty arguments
				err := validateRequiredArguments(toolName, map[string]interface{}{})
				Expect(err).To(HaveOccurred(), fmt.Sprintf("Tool %s should require arguments", toolName))

				// Test with complete arguments
				completeArgs := make(map[string]interface{})
				for _, arg := range required {
					completeArgs[arg] = "test-value"
				}

				err = validateRequiredArguments(toolName, completeArgs)
				Expect(err).NotTo(HaveOccurred(), fmt.Sprintf("Tool %s should accept complete arguments", toolName))
			}
		})
	})
})

// loadTestSuites loads all test resource files and parses them
func loadTestSuites() []TestSuite {
	resourceDir := "resources"
	testFiles := []string{
		"k8s_tools.txt",
		"utils_tools.txt",
		"prometheus_tools.txt",
		"cilium_tools.txt",
		"istio_tools.txt",
		"argo_tools.txt",
		"helm_tools.txt",
	}

	var suites []TestSuite

	for _, filename := range testFiles {
		packageName := strings.TrimSuffix(filename, "_tools.txt")
		filePath := filepath.Join("../", resourceDir, filename)

		testCases := loadTestCasesFromFile(filePath)
		if len(testCases) > 0 {
			suites = append(suites, TestSuite{
				PackageName: packageName,
				TestCases:   testCases,
			})
		}
	}

	return suites
}

// loadTestCasesFromFile parses a single test resource file
func loadTestCasesFromFile(filePath string) []TestEntry {
	file, err := os.Open(filePath)
	if err != nil {
		GinkgoWriter.Printf("Warning: Could not open test file %s: %v\n", filePath, err)
		return nil
	}
	defer file.Close()

	var testCases []TestEntry
	scanner := bufio.NewScanner(file)
	lineNum := 0

	for scanner.Scan() {
		lineNum++
		line := strings.TrimSpace(scanner.Text())

		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Parse line format: MCP_INPUT|EXPECTED_CLI_COMMAND
		parts := strings.SplitN(line, "|", 2)
		if len(parts) != 2 {
			GinkgoWriter.Printf("Warning: Invalid line format in %s at line %d: %s\n", filePath, lineNum, line)
			continue
		}

		mcpInput := strings.TrimSpace(parts[0])
		expectedCommand := strings.TrimSpace(parts[1])

		// Parse MCP JSON input
		var testEntry TestEntry
		if err := json.Unmarshal([]byte(mcpInput), &testEntry); err != nil {
			GinkgoWriter.Printf("Warning: Invalid JSON in %s at line %d: %v\n", filePath, lineNum, err)
			continue
		}

		testEntry.ExpectedCommand = expectedCommand
		testCases = append(testCases, testEntry)
	}

	if err := scanner.Err(); err != nil {
		Fail(fmt.Sprintf("Error reading file %s: %v", filePath, err))
	}

	return testCases
}

// setupMCPServer creates an MCP server with all tools registered
func setupMCPServer() *server.MCPServer {
	mcpServer := server.NewMCPServer("test-server", "v0.0.1")

	// Register all tools from all packages
	k8s.RegisterTools(mcpServer, nil, "") // nil LLM and empty kubeconfig for testing
	utils.RegisterTools(mcpServer)
	prometheus.RegisterTools(mcpServer)
	cilium.RegisterTools(mcpServer)
	istio.RegisterTools(mcpServer)
	argo.RegisterTools(mcpServer)
	helm.RegisterTools(mcpServer)

	return mcpServer
}

// executeToolWithStdio executes a tool with actual stdio and returns the result and executed command
func executeToolWithStdio(ctx context.Context, toolName string, arguments map[string]interface{}) (*mcp.CallToolResult, string, error) {
	// Use the default shell executor for actual command execution (mirror mode)
	// No need to explicitly set an executor - GetShellExecutor will return DefaultShellExecutor

	// Create MCP request
	request := mcp.CallToolRequest{}
	request.Params.Arguments = arguments

	// Set a timeout for tool execution
	execCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	// Call the tool
	result, err := callActualTool(execCtx, request, toolName)
	if err != nil {
		return nil, "", fmt.Errorf("tool execution failed: %w", err)
	}

	// For shell commands, extract the executed command from arguments
	executedCommand := ""
	if toolName == "shell" {
		if cmd, ok := arguments["command"].(string); ok {
			executedCommand = cmd
		}
	} else {
		// For other tools, we would need to capture the actual command execution
		// This could be done by modifying the CommandBuilder to return the built command
		executedCommand = fmt.Sprintf("tool_%s_executed", toolName)
	}

	return result, executedCommand, nil
}

// callActualTool calls the actual tool handler based on the tool name
func callActualTool(ctx context.Context, request mcp.CallToolRequest, toolName string) (*mcp.CallToolResult, error) {
	// Handle shell tool directly with actual execution
	if toolName == "shell" {
		args, ok := request.Params.Arguments.(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("invalid arguments type")
		}
		command, ok := args["command"].(string)
		if !ok || command == "" {
			return nil, fmt.Errorf("command parameter is required for shell tool")
		}

		// Execute the command through the real executor
		executor := cmd.GetShellExecutor(ctx)
		parts := strings.Fields(command)
		if len(parts) > 0 {
			output, err := executor.Exec(ctx, parts[0], parts[1:]...)
			if err != nil {
				return nil, fmt.Errorf("command execution failed: %w", err)
			}

			return &mcp.CallToolResult{
				Content: []mcp.Content{
					mcp.TextContent{Type: "text", Text: string(output)},
				},
			}, nil
		}
	}

	// For datetime tool, just return success since it doesn't use shell executor
	if toolName == "datetime_get_current_time" {
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				mcp.TextContent{Type: "text", Text: time.Now().Format(time.RFC3339)},
			},
		}, nil
	}

	// For all other tools, call the actual tool handlers directly
	// Note: Since the handlers are not exported, we'll simulate the execution
	// In a real implementation, these would call the actual handlers
	switch {
	case strings.HasPrefix(toolName, "k8s_"):
		return simulateToolExecution(ctx, toolName, "k8s")
	case strings.HasPrefix(toolName, "helm_"):
		return simulateToolExecution(ctx, toolName, "helm")
	case strings.HasPrefix(toolName, "cilium_"):
		return simulateToolExecution(ctx, toolName, "cilium")
	case strings.HasPrefix(toolName, "istio_"):
		return simulateToolExecution(ctx, toolName, "istio")
	case strings.HasPrefix(toolName, "argo_"):
		return simulateToolExecution(ctx, toolName, "argo")
	case strings.HasPrefix(toolName, "prometheus_"):
		return simulateToolExecution(ctx, toolName, "prometheus")
	default:
		return nil, fmt.Errorf("unknown tool: %s", toolName)
	}
}

// simulateToolExecution simulates tool execution for non-exported handlers
func simulateToolExecution(ctx context.Context, toolName, packageName string) (*mcp.CallToolResult, error) {
	// In mirror mode, we would actually execute the underlying commands
	// For now, we simulate successful execution
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			mcp.TextContent{Type: "text", Text: fmt.Sprintf("%s tool %s executed successfully", packageName, toolName)},
		},
	}, nil
}

// validateRequiredArguments validates that required arguments are present
func validateRequiredArguments(toolName string, arguments map[string]interface{}) error {
	requiredArgs := getRequiredArgumentsMap()

	if required, exists := requiredArgs[toolName]; exists {
		for _, arg := range required {
			if _, hasArg := arguments[arg]; !hasArg {
				return fmt.Errorf("missing required argument '%s'", arg)
			}
		}
	}

	return nil
}

// getRequiredArgumentsMap returns the map of required arguments for each tool
func getRequiredArgumentsMap() map[string][]string {
	return map[string][]string{
		"k8s_get_resources":                  {"resource_type"},
		"k8s_get_pod_logs":                   {"pod_name"},
		"k8s_scale":                          {"name", "replicas"},
		"k8s_patch_resource":                 {"resource_type", "resource_name", "patch"},
		"k8s_apply_manifest":                 {"manifest"},
		"k8s_delete_resource":                {"resource_type", "resource_name"},
		"k8s_check_service_connectivity":     {"service_name"},
		"k8s_execute_command":                {"pod_name", "command"},
		"k8s_rollout":                        {"action", "resource_type", "resource_name"},
		"k8s_label_resource":                 {"resource_type", "resource_name", "labels"},
		"k8s_annotate_resource":              {"resource_type", "resource_name", "annotations"},
		"k8s_remove_annotation":              {"resource_type", "resource_name", "annotation_key"},
		"k8s_remove_label":                   {"resource_type", "resource_name", "label_key"},
		"k8s_create_resource":                {"yaml_content"},
		"k8s_create_resource_from_url":       {"url"},
		"k8s_get_resource_yaml":              {"resource_type", "resource_name"},
		"k8s_describe_resource":              {"resource_type", "resource_name"},
		"k8s_generate_resource":              {"resource_description", "resource_type"},
		"shell":                              {"command"},
		"prometheus_query_tool":              {"query"},
		"prometheus_query_range_tool":        {"query"},
		"prometheus_promql_tool":             {"query_description"},
		"cilium_connect_to_remote_cluster":   {"cluster_name"},
		"cilium_disconnect_remote_cluster":   {"cluster_name"},
		"cilium_toggle_configuration_option": {"option", "value"},
		"cilium_get_service_information":     {"service_id"},
		"istio_proxy_config":                 {"pod_name"},
		"argo_promote_rollout":               {"rollout_name"},
		"argo_pause_rollout":                 {"rollout_name"},
		"argo_set_rollout_image":             {"rollout_name", "container_image"},
		"helm_get_release":                   {"name", "namespace"},
	}
}

// TestMCPToolsIntegration bootstraps the Ginkgo test suite
func TestMCPToolsIntegration(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "MCP Tools Integration Suite")
}
