# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working in this repository.

**The repository guide lives in [`AGENTS.md`](./AGENTS.md).** It covers the architecture,
tool-provider layout, the typed MCP input/output contract, error handling, caching, testing,
CI/CD, commit conventions and the "what not to do" list. Read it before making changes; it is
the single source of truth and is kept current.

This file only adds the few things AGENTS.md does not spell out.

## Architecture Overview

A Go MCP (Model Context Protocol) server that wraps Kubernetes and cloud-native CLIs
(`kubectl`, `helm`, `istioctl`, `cilium`, `kubectl-argo-rollouts`, `kubescape`,
the Prometheus HTTP API) behind a single typed MCP interface. It does not reimplement the
tools' behaviour; it validates input, invokes the CLI, and returns a typed result.

Two design points that are easy to get wrong:

- **Registration goes through `internal/mcp`, not the SDK directly.** `mcp.AddTool` records the
  provider for metrics and relaxes the inferred input schema so optional fields stay optional.
  Never call `sdk.AddTool` from a provider.
- **Every handler returns a concrete `Out` type.** The SDK infers an output schema from it,
  populates `structuredContent`, and validates the value on every call — including error paths.
  See "Typed MCP Inputs and Outputs" in AGENTS.md for the three pitfalls that break tools.

## Run Locally

```bash
go run ./cmd                          # defaults to stdio
./bin/kagent-tools --stdio            # stdio transport
./bin/kagent-tools --http --port 8084 # HTTP transport
```

Useful flags: `--tools k8s,helm` (limit providers), `--kubeconfig <path>`,
`--read-only` (do not register write tools), `--metrics-port`.

## Development Practices

- Run the narrowest useful test first, then broaden: `go test -tags=test -v -cover ./pkg/<provider>`
  before `make test`.
- `make test` = build + lint + all tests. `make test-only` skips build/lint.
- Use the mock shell executor for unit tests; never shell out to real CLIs in unit tests.
- Keep functions focused and testable, and use `context` for cancellation in long-running work.

### Test Coverage

- The project targets 80% coverage; every `pkg/` package currently exceeds it (lowest is
  `pkg/kubescape` at ~85%, highest ~99%).
- **CI does not enforce a coverage gate.** The `go-unit-tests` job runs `go test -v -cover`,
  which reports coverage but does not fail the build on a threshold. Treat 80% as the
  repository standard to maintain, not as an automated gate — check it yourself with
  `go test -cover ./pkg/...`.
- `internal/commands` and `internal/cmd` are below 80% and predate that standard.

## Logging

Structured logging lives in `internal/logger` (not `pkg/logger`). Prefer the package-level
logger used by the surrounding code.

## Commit Messages

Conventional Commits, with a `Signed-off-by` trailer (DCO is enforced on pull requests):
`feat`, `fix`, `docs`, `refactor`, `test`, `chore`, `perf`, `ci`.

## Additional Resources

- [AGENTS.md](AGENTS.md) — the repository guide (authoritative)
- [DEVELOPMENT.md](DEVELOPMENT.md) — setup and code standards
- [CONTRIBUTION.md](CONTRIBUTION.md) — contribution process and PR guidelines
- [docs/quickstart.md](docs/quickstart.md) — quick start guide
