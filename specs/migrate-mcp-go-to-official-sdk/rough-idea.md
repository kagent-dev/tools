# Rough Idea

## Summary

Migrate `github.com/mark3labs/mcp-go` to the official MCP Go SDK at `https://github.com/modelcontextprotocol/go-sdk`.

## Context

The project currently depends on the community-maintained MCP Go SDK (`github.com/mark3labs/mcp-go v0.43.2`). The official MCP Go SDK has been released at `github.com/modelcontextprotocol/go-sdk`. The migration should ensure all existing functionality is preserved while adopting the officially-supported library.

## Current State

- **Dependency**: `github.com/mark3labs/mcp-go v0.43.2`
- **Usage**: Tool registration, MCP server setup, transport handling (stdio, HTTP/SSE), tool result types
- **Files affected**: `cmd/main.go`, all `pkg/*/` tool packages
- **CLAUDE.md** already references `github.com/modelcontextprotocol/go-sdk` as the active technology

## Goal

Replace all usage of `github.com/mark3labs/mcp-go` with `github.com/modelcontextprotocol/go-sdk` across the codebase, maintaining full feature parity and test coverage requirements.
