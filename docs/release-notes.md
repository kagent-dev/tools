# Release notes

## Unreleased

### Kubescape vulnerability details: structured output change

`kubescape_get_vulnerability_details` now uses an object-rooted output schema so clients implementing the MCP 2025-06-18 Tool schema can accept the server's `tools/list` response.

Its successful `structuredContent` changes from an array of matches to an object containing that array:

```json
{"matches": [{"vulnerability": {"id": "CVE-2021-1234"}}]}
```

Consumers of `structuredContent` must read the `matches` property instead of treating the result as an array. The JSON above is illustrative; match entries retain their existing fields. The text content continues to contain the original JSON array, and failures remain tool errors (`isError: true`).
