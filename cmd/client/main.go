package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

/*
*
* Model context protocol client
*
* Initialize the client and connect to the MCP server using http transport
*
* Usage:
*   kagent-client --server <address> list-tools
*   kagent-client --server <address> call-tool <tool-name> [--args <json-string>]
*
* Examples:
*   kagent-client --server http://localhost:30885/mcp list-tools
*   kagent-client --server http://localhost:30885/mcp call-tool echo --args '{"message":"Hello, World!"}'
*
* @author Dimetron
* @date 2025-11-05
* @version 1.0.0
* @package main
* @link https://github.com/kagent-dev/tools
 */
func main() {
	serverFlag := flag.String("server", "", "MCP server address (e.g., http://localhost:30885/mcp)")
	argsFlag := flag.String("args", "{}", "Tool arguments as JSON string (for call-tool command)")
	flag.Parse()

	if *serverFlag == "" {
		fmt.Fprintf(os.Stderr, "Error: --server flag is required\n")
		fmt.Fprintf(os.Stderr, "Usage: %s --server <address> <command> [options]\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "Commands:\n")
		fmt.Fprintf(os.Stderr, "  list-tools              List available tools\n")
		fmt.Fprintf(os.Stderr, "  call-tool <tool-name>    Call a tool with optional arguments\n")
		os.Exit(1)
	}

	if flag.NArg() == 0 {
		fmt.Fprintf(os.Stderr, "Error: command is required\n")
		fmt.Fprintf(os.Stderr, "Usage: %s --server <address> <command> [options]\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "Commands:\n")
		fmt.Fprintf(os.Stderr, "  list-tools              List available tools\n")
		fmt.Fprintf(os.Stderr, "  call-tool <tool-name>    Call a tool with optional arguments\n")
		os.Exit(1)
	}

	command := flag.Arg(0)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// Create client
	client := mcp.NewClient(&mcp.Implementation{
		Name:    "kagent-client",
		Version: "1.0.0",
	}, nil)

	// Create HTTP transport
	transport := &mcp.StreamableClientTransport{
		Endpoint: *serverFlag,
	}

	// Connect to server
	session, err := client.Connect(ctx, transport, nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: failed to connect to server: %v\n", err)
		os.Exit(1)
	}
	defer func() {
		if err := session.Close(); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to close session: %v\n", err)
		}
	}()

	// Execute command
	switch command {
	case "list-tools":
		err = listTools(ctx, session)
	case "call-tool":
		if flag.NArg() < 2 {
			fmt.Fprintf(os.Stderr, "Error: tool name is required for call-tool command\n")
			fmt.Fprintf(os.Stderr, "Usage: %s --server <address> call-tool <tool-name> [--args <json-string>]\n", os.Args[0])
			os.Exit(1)
		}
		toolName := flag.Arg(1)
		err = callTool(ctx, session, toolName, *argsFlag)
	default:
		fmt.Fprintf(os.Stderr, "Error: unknown command: %s\n", command)
		fmt.Fprintf(os.Stderr, "Available commands: list-tools, call-tool\n")
		os.Exit(1)
	}

	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

// listTools lists all available tools from the MCP server
func listTools(ctx context.Context, session *mcp.ClientSession) error {
	var tools []*mcp.Tool
	for tool, err := range session.Tools(ctx, nil) {
		if err != nil {
			return fmt.Errorf("failed to iterate tools: %w", err)
		}
		tools = append(tools, tool)
	}

	if len(tools) == 0 {
		fmt.Println("No tools available")
		return nil
	}

	fmt.Printf("Available tools (%d):\n\n", len(tools))
	for _, tool := range tools {
		fmt.Printf("Name: %s\n", tool.Name)
		if tool.Description != "" {
			fmt.Printf("  Description: %s\n", tool.Description)
		}
		if tool.InputSchema != nil {
			fmt.Printf("  Has input schema: yes\n")
		}
		fmt.Println()
	}

	return nil
}

// callTool calls a tool with the given name and arguments
func callTool(ctx context.Context, session *mcp.ClientSession, toolName string, argsJSON string) error {
	// Parse arguments JSON
	var arguments map[string]interface{}
	if argsJSON != "" && argsJSON != "{}" {
		if err := json.Unmarshal([]byte(argsJSON), &arguments); err != nil {
			return fmt.Errorf("invalid JSON arguments: %w", err)
		}
	} else {
		arguments = make(map[string]interface{})
	}

	// Call the tool
	params := &mcp.CallToolParams{
		Name:      toolName,
		Arguments: arguments,
	}

	result, err := session.CallTool(ctx, params)
	if err != nil {
		return fmt.Errorf("failed to call tool: %w", err)
	}

	// Handle error response
	if result.IsError {
		var errorMsg strings.Builder
		for _, content := range result.Content {
			if textContent, ok := content.(*mcp.TextContent); ok {
				errorMsg.WriteString(textContent.Text)
			}
		}
		return fmt.Errorf("tool execution failed: %s", errorMsg.String())
	}

	// Display result
	if len(result.Content) == 0 {
		fmt.Println("Tool executed successfully (no output)")
		return nil
	}

	for _, content := range result.Content {
		switch c := content.(type) {
		case *mcp.TextContent:
			fmt.Println(c.Text)
		case *mcp.ImageContent:
			fmt.Printf("Image: data=%s\n", c.Data)
		default:
			// Try to marshal as JSON for unknown types
			if jsonBytes, err := json.MarshalIndent(content, "", "  "); err == nil {
				fmt.Println(string(jsonBytes))
			} else {
				fmt.Printf("Content: %+v\n", content)
			}
		}
	}

	return nil
}
