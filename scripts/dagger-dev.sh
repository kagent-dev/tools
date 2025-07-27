#!/bin/bash
set -e

# Dagger Development Script for kagent-tools
# This script provides convenient commands for local development using Dagger

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Helper functions
log_info() {
    echo -e "${BLUE}ℹ️  $1${NC}"
}

log_success() {
    echo -e "${GREEN}✅ $1${NC}"
}

log_warning() {
    echo -e "${YELLOW}⚠️  $1${NC}"
}

log_error() {
    echo -e "${RED}❌ $1${NC}"
}

log_step() {
    echo -e "${BLUE}🔄 $1${NC}"
}

# Check if dagger is installed
check_dagger() {
    if ! command -v dagger &> /dev/null; then
        log_error "Dagger CLI is not installed"
        echo "Install it with:"
        echo "  brew install dagger/tap/dagger  # macOS"
        echo "  curl -L https://dl.dagger.io/dagger/install.sh | sh  # Linux/Windows"
        exit 1
    fi
    log_info "Using Dagger $(dagger version)"
}

# Check if Docker is running
check_docker() {
    if ! docker info &> /dev/null; then
        log_error "Docker is not running"
        echo "Please start Docker and try again"
        exit 1
    fi
}

# Get version information
get_version_info() {
    VERSION=$(git describe --tags --always --dirty 2>/dev/null || echo "v0.0.0-dev")
    GIT_COMMIT=$(git rev-parse --short HEAD 2>/dev/null || echo "unknown")
    
    log_info "Version: $VERSION"
    log_info "Git Commit: $GIT_COMMIT"
}

# Run tests
test() {
    log_step "Running tests..."
    cd "$PROJECT_ROOT"
    dagger call test --source=.
    log_success "Tests completed"
}

# Run linter
lint() {
    log_step "Running linter..."
    cd "$PROJECT_ROOT"
    dagger call lint --source=.
    log_success "Linting completed"
}

# Format code
format() {
    log_step "Formatting code..."
    cd "$PROJECT_ROOT"
    dagger call format --source=. export --path=./formatted
    
    # Copy formatted code back
    if [ -d "./formatted" ]; then
        cp -r ./formatted/* .
        rm -rf ./formatted
        log_success "Code formatted"
    fi
}

# Build binaries
build() {
    log_step "Building binaries..."
    cd "$PROJECT_ROOT"
    get_version_info
    
    dagger call build \
        --source=. \
        --version="$VERSION" \
        --git-commit="$GIT_COMMIT" \
        export --path=./bin
        
    log_success "Binaries built in ./bin/"
    ls -la ./bin/
}

# Build Docker image
build_docker() {
    local platform=${1:-"linux/amd64"}
    log_step "Building Docker image for $platform..."
    cd "$PROJECT_ROOT"
    get_version_info
    
    dagger call build-docker \
        --source=. \
        --version="$VERSION" \
        --git-commit="$GIT_COMMIT" \
        --platform="$platform"
        
    log_success "Docker image built"
}

# Run full CI pipeline
ci() {
    log_step "Running full CI pipeline..."
    cd "$PROJECT_ROOT"
    get_version_info
    
    dagger call ci \
        --source=. \
        --version="$VERSION" \
        --git-commit="$GIT_COMMIT" \
        export --path=./ci-output
        
    log_success "CI pipeline completed - artifacts in ./ci-output/"
}

# Run E2E tests
e2e() {
    log_step "Running E2E tests..."
    cd "$PROJECT_ROOT"
    dagger call e2e --source=.
    log_success "E2E tests completed"
}

# Create release
release() {
    local publish=${1:-false}
    log_step "Creating release (publish: $publish)..."
    cd "$PROJECT_ROOT"
    get_version_info
    
    if [ "$publish" = "true" ]; then
        if [ -z "$REGISTRY_TOKEN" ]; then
            log_warning "REGISTRY_TOKEN not set, skipping publish"
            publish="false"
        fi
    fi
    
    dagger call release \
        --source=. \
        --version="$VERSION" \
        --git-commit="$GIT_COMMIT" \
        --publish="$publish" \
        ${REGISTRY_TOKEN:+--registry-token=env:REGISTRY_TOKEN}
        
    log_success "Release completed"
}

# Clean up build artifacts
clean() {
    log_step "Cleaning up build artifacts..."
    cd "$PROJECT_ROOT"
    
    rm -rf ./bin/ ./ci-output/ ./release-artifacts/ ./formatted/
    
    # Clean dagger cache
    dagger cache prune --all 2>/dev/null || true
    
    log_success "Cleanup completed"
}

# Development workflow
dev() {
    log_step "Running development workflow..."
    
    log_step "Step 1/4: Formatting code..."
    format
    
    log_step "Step 2/4: Running linter..."
    lint
    
    log_step "Step 3/4: Running tests..."
    test
    
    log_step "Step 4/4: Building binaries..."
    build
    
    log_success "Development workflow completed!"
}

# Quick check
quick() {
    log_step "Running quick checks..."
    
    log_step "Running tests..."
    test
    
    log_step "Running linter..."
    lint
    
    log_success "Quick checks completed!"
}

# Show usage
usage() {
    cat << EOF
Dagger Development Script for kagent-tools

Usage: $0 <command> [options]

Commands:
  test          Run Go tests with coverage
  lint          Run golangci-lint static analysis
  format        Format Go code with go fmt
  build         Build multi-platform binaries
  build-docker  Build Docker image [platform]
  ci            Run full CI pipeline
  e2e           Run end-to-end tests
  release       Create release [publish]
  clean         Clean up build artifacts
  dev           Run full development workflow
  quick         Run quick checks (test + lint)
  help          Show this help message

Examples:
  $0 test                    # Run tests
  $0 build                   # Build binaries
  $0 build-docker linux/arm64  # Build ARM64 Docker image
  $0 release true            # Create and publish release
  $0 dev                     # Full development workflow
  $0 quick                   # Quick test and lint

Environment Variables:
  REGISTRY_TOKEN             # Token for Docker registry authentication
  
Prerequisites:
  - Dagger CLI installed
  - Docker running
  - Git repository
EOF
}

# Main function
main() {
    # Check prerequisites
    check_dagger
    check_docker
    
    # Parse command
    case "${1:-help}" in
        test)
            test
            ;;
        lint)
            lint
            ;;
        format)
            format
            ;;
        build)
            build
            ;;
        build-docker)
            build_docker "$2"
            ;;
        ci)
            ci
            ;;
        e2e)
            e2e
            ;;
        release)
            release "$2"
            ;;
        clean)
            clean
            ;;
        dev)
            dev
            ;;
        quick)
            quick
            ;;
        help|--help|-h)
            usage
            ;;
        *)
            log_error "Unknown command: $1"
            echo
            usage
            exit 1
            ;;
    esac
}

# Run main function with all arguments
main "$@" 