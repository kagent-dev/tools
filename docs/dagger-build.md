# Dagger Build Pipeline

This document describes the Dagger build pipeline for the kagent-tools project. Dagger provides a portable, language-agnostic CI/CD pipeline that can run locally or in any CI environment.

## Overview

The Dagger pipeline provides the following functionality:

- **Testing**: Run Go tests with coverage
- **Linting**: Static code analysis with golangci-lint  
- **Formatting**: Code formatting with go fmt
- **Building**: Multi-platform binary builds (Linux, macOS, Windows)
- **Docker**: Container image building with all required tools
- **CI/CD**: Complete continuous integration pipeline
- **Publishing**: Docker image publishing to registries
- **E2E Testing**: End-to-end test execution

## Prerequisites

1. **Install Dagger CLI**:
   ```bash
   # macOS
   brew install dagger/tap/dagger
   
   # Linux/Windows - download from https://github.com/dagger/dagger/releases
   curl -L https://dl.dagger.io/dagger/install.sh | sh
   ```

2. **Docker**: Ensure Docker is running locally for container operations

## Available Functions

### Core Functions

#### `test`
Runs all Go tests with coverage reporting.

```bash
dagger call test --source=.
```

#### `lint` 
Runs golangci-lint static code analysis.

```bash
dagger call lint --source=.
```

#### `format`
Formats Go code and returns the formatted source directory.

```bash
dagger call format --source=. export --path=./formatted
```

#### `build`
Creates multi-platform binaries for Linux, macOS, and Windows.

```bash
# Basic build
dagger call build --source=.

# Build with custom version and git commit
dagger call build --source=. --version="v1.0.0" --git-commit="abc123" export --path=./build
```

#### `build-docker`
Builds a Docker container image with all required tools.

```bash
# Basic Docker build
dagger call build-docker --source=.

# Build for specific platform with version
dagger call build-docker --source=. --version="v1.0.0" --platform="linux/arm64"
```

### Pipeline Functions

#### `ci`
Runs the complete CI pipeline: format, lint, test, and build.

```bash
dagger call ci --source=. export --path=./ci-output
```

#### `e2e`
Runs end-to-end tests.

```bash
dagger call e2e --source=.
```

#### `release`
Complete release pipeline with optional publishing.

```bash
# Release without publishing
dagger call release --source=. --version="v1.0.0"

# Release with publishing (requires registry token)
dagger call release --source=. --version="v1.0.0" --publish=true --registry-token=env:REGISTRY_TOKEN
```

#### `publish`
Publishes Docker image to a registry.

```bash
dagger call publish --source=. --version="v1.0.0" --registry-token=env:GITHUB_TOKEN
```

## Local Development Workflow

### Quick Test and Build
```bash
# Run tests
dagger call test --source=.

# Build locally
dagger call build --source=. export --path=./bin

# Build Docker image
dagger call build-docker --source=. up
```

### Full CI Pipeline
```bash
# Run complete CI pipeline
dagger call ci --source=. export --path=./dist
```

### Release Workflow
```bash
# Get git info
VERSION=$(git describe --tags --always --dirty 2>/dev/null || echo "v0.0.0-dev")
GIT_COMMIT=$(git rev-parse --short HEAD)

# Create release
dagger call release --source=. --version="$VERSION" --git-commit="$GIT_COMMIT"
```

## CI/CD Integration

### GitHub Actions

Create `.github/workflows/dagger.yml`:

```yaml
name: Dagger CI

on:
  push:
    branches: [ main, develop ]
  pull_request:
    branches: [ main ]

jobs:
  dagger:
    runs-on: ubuntu-latest
    steps:
    - uses: actions/checkout@v4
      
    - name: Setup Dagger
      uses: dagger/dagger-for-github@v6
      with:
        version: "latest"
        
    - name: Run CI Pipeline
      run: |
        VERSION=${GITHUB_REF_NAME}-${GITHUB_SHA::8}
        dagger call ci --source=. --version="$VERSION" --git-commit="$GITHUB_SHA"
        
    - name: Build and Push Docker Image
      if: github.ref == 'refs/heads/main'
      env:
        REGISTRY_TOKEN: ${{ secrets.GITHUB_TOKEN }}
      run: |
        VERSION=${GITHUB_REF_NAME}-${GITHUB_SHA::8}
        dagger call publish --source=. --version="$VERSION" --registry-token=env:REGISTRY_TOKEN
```

### GitLab CI

Create `.gitlab-ci.yml`:

```yaml
stages:
  - test
  - build
  - release

variables:
  DAGGER_VERSION: "latest"

.dagger: &dagger
  image: docker:27-dind
  services:
    - docker:27-dind
  before_script:
    - apk add --no-cache curl
    - curl -L https://dl.dagger.io/dagger/install.sh | sh
    - mv bin/dagger /usr/local/bin/

ci:
  <<: *dagger
  stage: test
  script:
    - dagger call ci --source=.

build-docker:
  <<: *dagger
  stage: build
  script:
    - dagger call build-docker --source=. --version="$CI_COMMIT_REF_NAME-$CI_COMMIT_SHORT_SHA"

publish:
  <<: *dagger
  stage: release
  only:
    - main
  script:
    - dagger call publish --source=. --version="$CI_COMMIT_REF_NAME-$CI_COMMIT_SHORT_SHA" --registry-token=env:CI_JOB_TOKEN
```

## Configuration

### Build Configuration

The pipeline uses a `BuildConfig` struct with the following default values:

```go
type BuildConfig struct {
    Version                   string // "v0.0.0-dev"
    GitCommit                 string // "unknown"  
    BuildDate                 string // Current date
    DockerRegistry            string // "ghcr.io"
    DockerRepo                string // "kagent-dev/kagent"
    ToolsIstioVersion        string // "1.26.2"
    ToolsArgoRolloutsVersion string // "1.8.3"
    ToolsKubectlVersion      string // "1.33.2"
    ToolsHelmVersion         string // "3.18.4"
    ToolsCiliumVersion       string // "0.18.5"
}
```

### Environment Variables

- `REGISTRY_TOKEN`: Token for Docker registry authentication
- `GITHUB_TOKEN`: GitHub token for publishing to GHCR
- `DOCKER_REGISTRY`: Override default registry
- `VERSION`: Override build version
- `GIT_COMMIT`: Override git commit hash

## Troubleshooting

### Common Issues

1. **Docker not running**: Ensure Docker daemon is running locally
2. **Permission errors**: Make sure user has Docker permissions
3. **Network issues**: Check firewall/proxy settings for tool downloads
4. **Build failures**: Check Go module dependencies are up to date

### Debug Mode

Run with verbose output:
```bash
dagger call --debug test --source=.
```

### Cache Management

Clear Dagger cache:
```bash
dagger cache prune
```

## Multi-Platform Builds

The pipeline supports building for multiple platforms:

- `linux/amd64`
- `linux/arm64`  
- `darwin/amd64`
- `darwin/arm64`
- `windows/amd64`

For Docker images, specify platform:
```bash
dagger call build-docker --source=. --platform="linux/arm64"
```

## Performance Tips

1. **Use local cache**: Dagger caches layers automatically
2. **Parallel builds**: Dagger runs operations in parallel when possible
3. **Incremental builds**: Only changed layers are rebuilt
4. **Remote caching**: Use remote cache for CI environments

## Examples

### Complete Local Development Flow

```bash
#!/bin/bash
set -e

echo "🧪 Running tests..."
dagger call test --source=.

echo "🔍 Running linter..."
dagger call lint --source=.

echo "🔨 Building binaries..."
dagger call build --source=. export --path=./bin

echo "🐳 Building Docker image..."
dagger call build-docker --source=.

echo "✅ All done!"
```

### Release Script

```bash
#!/bin/bash
set -e

# Get version info
VERSION=$(git describe --tags --always --dirty 2>/dev/null || echo "v0.0.0-dev")
GIT_COMMIT=$(git rev-parse --short HEAD)

echo "🚀 Creating release $VERSION ($GIT_COMMIT)"

# Run full release pipeline
dagger call release --source=. --version="$VERSION" --git-commit="$GIT_COMMIT"

echo "✅ Release complete!"
``` 