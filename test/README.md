# Tools Integration Test Suite

This directory contains a comprehensive test suite for validating all RegisterTools functions that use WithString parameters across the kagent-tools project.

## Overview

The test suite provides comprehensive validation for all MCP (Model Context Protocol) tools registered by the various packages in the project. It ensures that tool registration works correctly and validates the expected CLI command mappings for each tool.

## Test Structure

### Test Resources (`test/resources/`)

The test suite includes resource files containing test data for each package:

- **`k8s_tools.txt`** - 41 test cases for Kubernetes tools
- **`utils_tools.txt`** - 10 test cases for utility tools
- **`prometheus_tools.txt`** - 15 test cases for Prometheus tools
- **`cilium_tools.txt`** - 52 test cases for Cilium tools
- **`istio_tools.txt`** - 26 test cases for Istio tools
- **`argo_tools.txt`** - 23 test cases for Argo Rollouts tools
- **`helm_tools.txt`** - 19 test cases for Helm tools

**Total: 186 test cases**

### Test File Format

Each resource file uses the format:
```
{"tool": "tool_name", "arguments": {"arg1": "value1", "arg2": "value2"}}|expected_cli_command
```

For example:
```
{"tool": "k8s_get_resources", "arguments": {"resource_type": "pods", "namespace": "kube-system"}}|kubectl get pods -n kube-system
```

### Test Implementation (`test/tools_integration_test.go`)

The Go test file provides:

1. **TestToolsIntegration** - Main integration test that validates all tools
2. **TestToolRegistration** - Validates that tools can be registered without errors
3. **BenchmarkToolExecution** - Performance benchmarking for tool registration

## Test Coverage

### Packages Covered

- **K8s Package** (`pkg/k8s/`) - 41 tools including:
  - `k8s_get_resources`, `k8s_get_pod_logs`, `k8s_scale`
  - `k8s_patch_resource`, `k8s_apply_manifest`, `k8s_delete_resource`
  - `k8s_check_service_connectivity`, `k8s_execute_command`
  - `k8s_rollout`, `k8s_label_resource`, `k8s_annotate_resource`
  - And many more...

- **Utils Package** (`pkg/utils/`) - 10 tools including:
  - `shell`, `datetime_get_current_time`

- **Prometheus Package** (`pkg/prometheus/`) - 15 tools including:
  - `prometheus_query_tool`, `prometheus_query_range_tool`
  - `prometheus_label_names_tool`, `prometheus_targets_tool`
  - `prometheus_promql_tool`

- **Cilium Package** (`pkg/cilium/`) - 52 tools including:
  - `cilium_status_and_version`, `cilium_install_cilium`
  - `cilium_upgrade_cilium`, `cilium_connect_to_remote_cluster`
  - `cilium_get_daemon_status`, `cilium_get_endpoints_list`
  - And extensive debug tools...

- **Istio Package** (`pkg/istio/`) - 26 tools including:
  - `istio_proxy_status`, `istio_proxy_config`
  - `istio_install_istio`, `istio_generate_manifest`
  - `istio_analyze_cluster_configuration`

- **Argo Package** (`pkg/argo/`) - 23 tools including:
  - `argo_verify_argo_rollouts_controller_install`
  - `argo_rollouts_list`, `argo_promote_rollout`
  - `argo_pause_rollout`, `argo_set_rollout_image`

- **Helm Package** (`pkg/helm/`) - 19 tools including:
  - `helm_list_releases`, `helm_get_release`

## Running Tests

### Run All Tests
```bash
cd test
go test -v
```

### Run Specific Test
```bash
cd test
go test -v -run TestToolsIntegration
go test -v -run TestToolRegistration
```

### Run Benchmarks
```bash
cd test
go test -v -bench=.
```

## Test Features

### Validation
- **Tool Registration**: Ensures all tools can be registered without errors
- **Argument Validation**: Validates required arguments are present for each tool
- **Expected Command Mapping**: Verifies the expected CLI command for each tool configuration

### Coverage
- **Comprehensive Coverage**: Tests all 186 tool configurations across 7 packages
- **Required Arguments**: Validates that tools with required parameters fail appropriately when those parameters are missing
- **Optional Arguments**: Tests various combinations of optional parameters

### Extensibility
- **Easy Addition**: New tools can be added by simply updating the corresponding resource file
- **Format Consistency**: Standardized format makes it easy to add new test cases
- **Automatic Discovery**: Test framework automatically discovers and runs all test cases from resource files

## Example Test Cases

### Kubernetes Tools
```json
{"tool": "k8s_get_resources", "arguments": {"resource_type": "pods", "namespace": "kube-system"}}|kubectl get pods -n kube-system
{"tool": "k8s_scale", "arguments": {"name": "nginx-deployment", "replicas": 3}}|kubectl scale deployment nginx-deployment --replicas=3
{"tool": "k8s_execute_command", "arguments": {"pod_name": "nginx-pod", "command": "ls -la"}}|kubectl exec nginx-pod -- ls -la
```

### Prometheus Tools
```json
{"tool": "prometheus_query_tool", "arguments": {"query": "up"}}|curl -s 'http://localhost:9090/api/v1/query?query=up'
{"tool": "prometheus_query_range_tool", "arguments": {"query": "cpu_usage_percent", "start": "1640995200", "end": "1640998800", "step": "30s"}}|curl -s 'http://localhost:9090/api/v1/query_range?query=cpu_usage_percent&start=1640995200&end=1640998800&step=30s'
```

### Cilium Tools
```json
{"tool": "cilium_install_cilium", "arguments": {"cluster_name": "new-cluster", "cluster_id": "1"}}|cilium install --cluster-name new-cluster --cluster-id 1
{"tool": "cilium_get_daemon_status", "arguments": {"show_all_addresses": "true", "brief": "true"}}|cilium status --all-addresses --brief
```

## Architecture

The test suite follows a clean architecture:

1. **Resource Files** - Contain test data in a standardized format
2. **Test Loader** - Parses resource files and creates test cases
3. **Test Runner** - Executes tests and validates results
4. **MCP Server Setup** - Creates a test MCP server with all tools registered
5. **Validation Framework** - Validates tool registration and argument requirements

## Benefits

- **Complete Coverage**: Tests all 186 tool configurations
- **Regression Prevention**: Ensures tool registration continues to work as code evolves
- **Documentation**: Serves as living documentation of tool usage patterns
- **Validation**: Ensures required arguments are properly validated
- **Maintainability**: Easy to add new test cases as new tools are added

## Future Enhancements

Potential improvements for the test suite:

1. **Actual Execution Testing**: Execute tools and validate actual CLI command construction
2. **Error Case Testing**: Test invalid arguments and error handling
3. **Performance Testing**: More comprehensive benchmarking
4. **Integration Testing**: Test with actual external tools (kubectl, helm, etc.)
5. **Mocking Framework**: Mock external dependencies for isolated testing

---

This test suite provides a solid foundation for ensuring the reliability and correctness of all MCP tools in the kagent-tools project. 