#!/bin/bash

# Integration Test Runner for MCP SDK Migration
# This script runs comprehensive integration tests for the new MCP SDK implementation

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Function to print colored output
print_status() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

print_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1"
}

print_warning() {
    echo -e "${YELLOW}[WARNING]${NC} $1"
}

print_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# Check if we're in the right directory
if [ ! -f "../../go.mod" ]; then
    print_error "Please run this script from the test/integration directory"
    exit 1
fi

# Check if binary exists, build if necessary
BINARY_PATH="../../bin/kagent-tools-linux-amd64"
if [[ "$OSTYPE" == "darwin"* ]]; then
    BINARY_PATH="../../bin/kagent-tools-darwin-amd64"
elif [[ "$OSTYPE" == "msys" || "$OSTYPE" == "win32" ]]; then
    BINARY_PATH="../../bin/kagent-tools-windows-amd64.exe"
fi

if [ ! -f "$BINARY_PATH" ]; then
    print_warning "Binary not found at $BINARY_PATH, building..."
    cd ../..
    make build
    cd test/integration
    if [ ! -f "$BINARY_PATH" ]; then
        print_error "Failed to build binary"
        exit 1
    fi
fi

print_success "Binary found at $BINARY_PATH"

# Set environment variables for testing
export LOG_LEVEL=debug
export OTEL_SERVICE_NAME=kagent-tools-integration-test

print_status "Starting integration tests..."

# Run different test suites
TEST_SUITES=(
    "binary_verification_test.go"
    "mcp_integration_test.go"
    "stdio_transport_test.go" 
    "http_transport_test.go"
    "tool_categories_test.go"
    "comprehensive_integration_test.go"
)

FAILED_TESTS=()
PASSED_TESTS=()

for suite in "${TEST_SUITES[@]}"; do
    print_status "Running test suite: $suite"
    
    if go test -v -timeout=300s "./$suite"; then
        print_success "✓ $suite passed"
        PASSED_TESTS+=("$suite")
    else
        print_error "✗ $suite failed"
        FAILED_TESTS+=("$suite")
    fi
    
    echo ""
done

# Run all tests together for comprehensive coverage
print_status "Running comprehensive integration test suite..."
if go test -v -timeout=600s ./...; then
    print_success "✓ Comprehensive test suite passed"
    PASSED_TESTS+=("comprehensive")
else
    print_error "✗ Comprehensive test suite failed"
    FAILED_TESTS+=("comprehensive")
fi

# Print summary
echo ""
print_status "=== Integration Test Summary ==="
echo ""

if [ ${#PASSED_TESTS[@]} -gt 0 ]; then
    print_success "Passed tests (${#PASSED_TESTS[@]}):"
    for test in "${PASSED_TESTS[@]}"; do
        echo -e "  ${GREEN}✓${NC} $test"
    done
fi

if [ ${#FAILED_TESTS[@]} -gt 0 ]; then
    echo ""
    print_error "Failed tests (${#FAILED_TESTS[@]}):"
    for test in "${FAILED_TESTS[@]}"; do
        echo -e "  ${RED}✗${NC} $test"
    done
    echo ""
    print_error "Some integration tests failed. Please check the output above for details."
    exit 1
else
    echo ""
    print_success "All integration tests passed! 🎉"
    print_status "The MCP SDK migration integration tests are working correctly."
fi

# Cleanup any remaining processes
print_status "Cleaning up any remaining test processes..."
pkill -f "kagent-tools" || true

print_success "Integration test run completed."