// A comprehensive Dagger build pipeline for KagentTools
//
// This module provides a complete CI/CD pipeline for building, testing,
// and publishing the kagent-tools MCP server. It supports multi-platform
// builds, Docker containerization, and automated testing.

package main

import (
	"context"
	"dagger/kagent-tools/internal/dagger"
	"fmt"
	"os"
	"time"
)

type KagentTools struct{}

const (
	FromImageGo = "chainguard/wolfi-base:latest"
)

// Build configuration
type BuildConfig struct {
	Version                  string
	GitCommit                string
	BuildDate                string
	DockerRegistry           string
	DockerRepo               string
	ToolsIstioVersion        string
	ToolsArgoRolloutsVersion string
	ToolsKubectlVersion      string
	ToolsHelmVersion         string
	ToolsCiliumVersion       string
}

// Default build configuration
func defaultBuildConfig() *BuildConfig {
	return &BuildConfig{
		DockerRegistry:           "ghcr.io",
		DockerRepo:               "kagent-dev/kagent",
		Version:                  "v0.0.0-dev",
		GitCommit:                "unknown",
		BuildDate:                time.Now().UTC().Format("2006-01-02"),
		ToolsIstioVersion:        "1.26.2",
		ToolsArgoRolloutsVersion: "1.8.3",
		ToolsKubectlVersion:      "1.33.2",
		ToolsHelmVersion:         "3.18.4",
		ToolsCiliumVersion:       "0.18.5",
	}
}

// Test runs all Go tests with coverage
func (m *KagentTools) Test(ctx context.Context, source *dagger.Directory) *dagger.Container {

	return dag.Container().
		From(FromImageGo).
		WithMountedDirectory("/src", source).
		WithWorkdir("/src").
		WithExec([]string{"apk", "add", "--no-cache", "git", "make", "go"}).
		WithExec([]string{"go", "mod", "tidy", "-v"}).
		WithExec([]string{"go", "mod", "download"}).
		WithExec([]string{"go", "test", "-tags=test", "-v", "-cover", "./pkg/...", "./internal/..."})
}

// Lint runs golangci-lint on the codebase
func (m *KagentTools) Lint(ctx context.Context, source *dagger.Directory) *dagger.Container {
	return dag.Container().
		From("golangci/golangci-lint:v1.63.4-alpine").
		WithMountedDirectory("/src", source).
		WithWorkdir("/src").
		WithExec([]string{"golangci-lint", "run", "--build-tags=test", "./..."})
}

// Format runs go fmt on the codebase
func (m *KagentTools) Format(ctx context.Context, source *dagger.Directory) *dagger.Directory {
	return dag.Container().
		From(FromImageGo).
		WithMountedDirectory("/src", source).
		WithWorkdir("/src").
		WithExec([]string{"go", "fmt", "./..."}).
		Directory("/src")
}

// Build creates binaries for multiple platforms
func (m *KagentTools) Build(ctx context.Context, source *dagger.Directory,
	// +optional
	version string,
	// +optional
	gitCommit string) *dagger.Directory {

	config := defaultBuildConfig()
	if version != "" {
		config.Version = version
	}
	if gitCommit != "" {
		config.GitCommit = gitCommit
	}

	ldflags := fmt.Sprintf("-X github.com/kagent-dev/tools/internal/version.Version=%s -X github.com/kagent-dev/tools/internal/version.GitCommit=%s -X github.com/kagent-dev/tools/internal/version.BuildDate=%s",
		config.Version, config.GitCommit, config.BuildDate)

	platforms := []struct {
		os   string
		arch string
	}{
		{"linux", "amd64"},
		{"linux", "arm64"},
		{"darwin", "amd64"},
		{"darwin", "arm64"},
		{"windows", "amd64"},
	}

	builder := dag.Container().
		From(FromImageGo).
		WithMountedDirectory("/src", source).
		WithWorkdir("/src").
		WithExec([]string{"go", "mod", "download"})

	for _, platform := range platforms {
		suffix := ""
		if platform.os == "windows" {
			suffix = ".exe"
		}

		binaryName := fmt.Sprintf("kagent-tools-%s-%s%s", platform.os, platform.arch, suffix)

		builder = builder.WithEnvVariable("CGO_ENABLED", "0").
			WithEnvVariable("GOOS", platform.os).
			WithEnvVariable("GOARCH", platform.arch).
			WithExec([]string{"go", "build", "-ldflags", ldflags, "-o", fmt.Sprintf("/build/%s", binaryName), "./cmd"})
	}

	return builder.Directory("/build")
}

// BuildDocker creates a Docker image for the tools
func (m *KagentTools) BuildDocker(ctx context.Context, source *dagger.Directory,
	// +optional
	version string,
	// +optional
	gitCommit string,
	// +optional
	platform string) *dagger.Container {

	config := defaultBuildConfig()
	if version != "" {
		config.Version = version
	}
	if gitCommit != "" {
		config.GitCommit = gitCommit
	}

	ldflags := fmt.Sprintf("-X github.com/kagent-dev/tools/internal/version.Version=%s -X github.com/kagent-dev/tools/internal/version.GitCommit=%s -X github.com/kagent-dev/tools/internal/version.BuildDate=%s",
		config.Version, config.GitCommit, config.BuildDate)

	// Extract architecture from platform
	arch := os.Getenv("GOARCH")
	if platform == "" {
		platform = "linux/" + os.Getenv("GOARCH")
	}

	// Stage 1: Download tools
	toolsStage := dag.Container().
		From(FromImageGo).
		WithExec([]string{"apk", "update"}).
		WithExec([]string{"apk", "add", "curl", "openssl", "bash", "git", "ca-certificates"}).
		WithWorkdir("/downloads")

	// Download kubectl
	toolsStage = toolsStage.WithExec([]string{"sh", "-c", fmt.Sprintf(`
		curl -LO "https://dl.k8s.io/release/v%s/bin/linux/%s/kubectl" && \
		chmod +x kubectl && \
		./kubectl version --client
	`, config.ToolsKubectlVersion, arch)})

	// Download Helm
	toolsStage = toolsStage.WithExec([]string{"sh", "-c", fmt.Sprintf(`
		curl -Lo helm.tar.gz https://get.helm.sh/helm-v%s-linux-%s.tar.gz && \
		tar -xvf helm.tar.gz && \
		mv linux-%s/helm /downloads/helm && \
		chmod +x /downloads/helm && \
		/downloads/helm version
	`, config.ToolsHelmVersion, arch, arch)})

	// Download Istio
	toolsStage = toolsStage.WithExec([]string{"sh", "-c", fmt.Sprintf(`
		curl -L https://istio.io/downloadIstio | ISTIO_VERSION=%s TARGET_ARCH=%s sh - && \
		mv istio-*/bin/istioctl /downloads/ && \
		rm -rf istio-* && \
		/downloads/istioctl --help
	`, config.ToolsIstioVersion, arch)})

	// Download Argo Rollouts
	toolsStage = toolsStage.WithExec([]string{"sh", "-c", fmt.Sprintf(`
		curl -Lo /downloads/kubectl-argo-rollouts https://github.com/argoproj/argo-rollouts/releases/download/v%s/kubectl-argo-rollouts-linux-%s && \
		chmod +x /downloads/kubectl-argo-rollouts && \
		/downloads/kubectl-argo-rollouts version
	`, config.ToolsArgoRolloutsVersion, arch)})

	// Download Cilium
	toolsStage = toolsStage.WithExec([]string{"sh", "-c", fmt.Sprintf(`
		curl -Lo cilium.tar.gz https://github.com/cilium/cilium-cli/releases/download/v%s/cilium-linux-%s.tar.gz && \
		tar -xvf cilium.tar.gz && \
		mv cilium /downloads/cilium && \
		chmod +x /downloads/cilium && \
		rm -rf cilium.tar.gz && \
		/downloads/cilium version
	`, config.ToolsCiliumVersion, arch)})

	// Stage 2: Build the Go application
	buildStage := dag.Container().
		From("cgr.dev/chainguard/go:latest").
		WithMountedDirectory("/workspace", source).
		WithWorkdir("/workspace").
		WithExec([]string{"go", "mod", "download"}).
		WithExec([]string{"sh", "-c", fmt.Sprintf(`
			CGO_ENABLED=0 GOOS=linux GOARCH=%s go build -a -ldflags "%s" -o tool-server cmd/main.go
		`, arch, ldflags)})

	// Final stage: Combine everything
	return dag.Container().
		From("gcr.io/distroless/static:nonroot").
		WithWorkdir("/").
		WithUser("65532:65532").
		WithEnvVariable("PATH", "$PATH:/bin").
		WithFile("/bin/kubectl", toolsStage.File("/downloads/kubectl")).
		WithFile("/bin/istioctl", toolsStage.File("/downloads/istioctl")).
		WithFile("/bin/helm", toolsStage.File("/downloads/helm")).
		WithFile("/bin/kubectl-argo-rollouts", toolsStage.File("/downloads/kubectl-argo-rollouts")).
		WithFile("/bin/cilium", toolsStage.File("/downloads/cilium")).
		WithFile("/tool-server", buildStage.File("/workspace/tool-server")).
		WithLabel("org.opencontainers.image.source", "https://github.com/kagent-dev/tools").
		WithLabel("org.opencontainers.image.description", "Kagent MCP tools server").
		WithLabel("org.opencontainers.image.authors", "Kagent Creators 🤖").
		WithLabel("org.opencontainers.image.version", config.Version).
		WithEntrypoint([]string{"/tool-server"})
}

// E2E runs end-to-end tests
func (m *KagentTools) E2E(ctx context.Context, source *dagger.Directory) *dagger.Container {
	return dag.Container().
		From(FromImageGo).
		WithMountedDirectory("/src", source).
		WithWorkdir("/src").
		WithExec([]string{"apk", "add", "--no-cache", "git", "make", "docker"}).
		WithExec([]string{"go", "mod", "download"}).
		WithExec([]string{"go", "test", "-v", "-tags=test", "-cover", "./test/e2e/", "-timeout", "5m"})
}

// Publish publishes the Docker image to a registry
func (m *KagentTools) Publish(ctx context.Context, source *dagger.Directory,
	// +optional
	version string,
	// +optional
	gitCommit string,
	// +optional
	registry string,
	registryToken *dagger.Secret) (string, error) {

	config := defaultBuildConfig()
	if version != "" {
		config.Version = version
	}
	if gitCommit != "" {
		config.GitCommit = gitCommit
	}
	if registry == "" {
		registry = config.DockerRegistry
	}

	container := m.BuildDocker(ctx, source, version, gitCommit, "linux/amd64")

	imageRef := fmt.Sprintf("%s/%s/tools:%s", registry, config.DockerRepo, config.Version)

	// Push the image
	published, err := container.WithRegistryAuth(registry, "token", registryToken).Publish(ctx, imageRef)
	if err != nil {
		return "", fmt.Errorf("failed to publish image: %w", err)
	}

	return published, nil
}

// CI runs the complete CI pipeline: format, lint, test, build
func (m *KagentTools) CI(ctx context.Context, source *dagger.Directory,
	// +optional
	version string,
	// +optional
	gitCommit string) (*dagger.Directory, error) {

	// Run format check
	formatted := m.Format(ctx, source)

	// Run linting
	lintResult := m.Lint(ctx, formatted)
	if _, err := lintResult.Stdout(ctx); err != nil {
		return nil, fmt.Errorf("linting failed: %w", err)
	}

	// Run tests
	testResult := m.Test(ctx, formatted)
	if _, err := testResult.Stdout(ctx); err != nil {
		return nil, fmt.Errorf("tests failed: %w", err)
	}

	// Build binaries
	buildResult := m.Build(ctx, formatted, version, gitCommit)

	return buildResult, nil
}

// Release performs a complete release: CI + Docker build + optional publish
func (m *KagentTools) Release(ctx context.Context, source *dagger.Directory,
	// +optional
	version string,
	// +optional
	gitCommit string,
	// +optional
	publish bool,
	// +optional
	registryToken *dagger.Secret) (*dagger.Container, error) {

	// Run CI pipeline
	_, err := m.CI(ctx, source, version, gitCommit)
	if err != nil {
		return nil, fmt.Errorf("CI pipeline failed: %w", err)
	}

	// Build Docker image
	dockerImage := m.BuildDocker(ctx, source, version, gitCommit, "linux/amd64")

	// Optionally publish
	if publish && registryToken != nil {
		published, err := m.Publish(ctx, source, version, gitCommit, "", registryToken)
		if err != nil {
			return nil, fmt.Errorf("publish failed: %w", err)
		}
		fmt.Printf("Published image: %s\n", published)
	}

	return dockerImage, nil
}
