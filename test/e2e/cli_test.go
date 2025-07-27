package e2e

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// Test suite setup
var _ = Describe("KAgent Tools E2E Tests", func() {
	var (
		ctx    context.Context
		cancel context.CancelFunc
	)

	BeforeEach(func() {
		ctx, cancel = context.WithTimeout(context.Background(), 60*time.Second)

		// Set OTEL environment variables for testing
		os.Setenv("OTEL_SERVICE_NAME", "kagent-tools-e2e-test")
		os.Setenv("LOG_LEVEL", "debug")
	})

	AfterEach(func() {
		if cancel != nil {
			cancel()
		}
		os.Unsetenv("OTEL_SERVICE_NAME")
		os.Unsetenv("LOG_LEVEL")
	})

	Describe("HTTP Server Tests", func() {
		It("should start and stop HTTP server successfully", func() {
			config := TestServerConfig{
				Port:    8085,
				Stdio:   false,
				Timeout: 60 * time.Second,
			}

			server := NewTestServer(config)

			// Start server
			err := server.Start(ctx, config)
			Expect(err).NotTo(HaveOccurred(), "Server should start successfully")

			// Test health endpoint
			resp, err := http.Get(fmt.Sprintf("http://localhost:%d/health", config.Port))
			Expect(err).NotTo(HaveOccurred(), "Health endpoint should be accessible")
			Expect(resp.StatusCode).To(Equal(http.StatusOK))
			resp.Body.Close()

			// Check server output
			output := server.GetOutput()
			Expect(output).To(ContainSubstring("Running KAgent Tools Server"))
			Expect(output).To(ContainSubstring(fmt.Sprintf(":%d", config.Port)))

			// Stop server
			err = server.Stop()
			Expect(err).NotTo(HaveOccurred(), "Server should stop gracefully")

			// Wait for server to fully shutdown
			shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer shutdownCancel()

			err = server.waitForShutdown(shutdownCtx, config.Port)
			Expect(err).NotTo(HaveOccurred(), "Server should shut down completely")
		})

		It("should start server with specific tools", func() {
			config := TestServerConfig{
				Port:    8086,
				Tools:   []string{"utils", "k8s"},
				Stdio:   false,
				Timeout: 30 * time.Second,
			}

			server := NewTestServer(config)

			// Start server
			err := server.Start(ctx, config)
			Expect(err).NotTo(HaveOccurred(), "Server should start successfully")

			// Wait for server to be ready
			time.Sleep(3 * time.Second)

			// Check server output for tool registration
			output := server.GetOutput()
			Expect(output).To(ContainSubstring("RegisterTools initialized"))
			Expect(output).To(ContainSubstring("utils"))
			Expect(output).To(ContainSubstring("k8s"))

			// Stop server
			err = server.Stop()
			Expect(err).NotTo(HaveOccurred(), "Server should stop gracefully")
		})

		It("should start server with all tools enabled", func() {
			config := TestServerConfig{
				Port:    8087,
				Stdio:   false,
				Timeout: 30 * time.Second,
			}

			server := NewTestServer(config)

			// Start server
			err := server.Start(ctx, config)
			Expect(err).NotTo(HaveOccurred(), "Server should start successfully")

			// Wait for server to be ready
			time.Sleep(3 * time.Second)

			// Check server output for all tools registration
			output := server.GetOutput()
			Expect(output).To(ContainSubstring("RegisterTools initialized"))
			Expect(output).To(ContainSubstring("Running KAgent Tools Server"))

			// Stop server
			err = server.Stop()
			Expect(err).NotTo(HaveOccurred(), "Server should stop gracefully")
		})

		It("should start server with kubeconfig parameter", func() {
			// Create a temporary kubeconfig file
			tempDir := GinkgoT().TempDir()
			kubeconfigPath := filepath.Join(tempDir, "kubeconfig")

			kubeconfigContent := `apiVersion: v1
kind: Config
clusters:
- cluster:
    server: https://test-cluster
  name: test-cluster
contexts:
- context:
    cluster: test-cluster
    user: test-user
  name: test-context
current-context: test-context
users:
- name: test-user
  user:
    token: test-token
`

			err := os.WriteFile(kubeconfigPath, []byte(kubeconfigContent), 0644)
			Expect(err).NotTo(HaveOccurred(), "Should create temporary kubeconfig file")

			config := TestServerConfig{
				Port:       8088,
				Kubeconfig: kubeconfigPath,
				Stdio:      false,
				Timeout:    30 * time.Second,
			}

			server := NewTestServer(config)

			// Start server
			err = server.Start(ctx, config)
			Expect(err).NotTo(HaveOccurred(), "Server should start successfully")

			// Wait for server to be ready
			time.Sleep(3 * time.Second)

			// Check server output for kubeconfig setting
			output := server.GetOutput()
			Expect(output).To(ContainSubstring("RegisterTools initialized"))
			Expect(output).To(ContainSubstring("Running KAgent Tools Server"))

			// Stop server
			err = server.Stop()
			Expect(err).NotTo(HaveOccurred(), "Server should stop gracefully")
		})

		It("should handle invalid tool names gracefully", func() {
			config := TestServerConfig{
				Port:    18190,
				Tools:   []string{"invalid-tool", "utils"},
				Stdio:   false,
				Timeout: 30 * time.Second,
			}

			server := NewTestServer(config)

			// Start server
			err := server.Start(ctx, config)
			Expect(err).NotTo(HaveOccurred(), "Server should start even with invalid tools")

			// Wait for server to be ready
			time.Sleep(3 * time.Second)

			// Check server output for error about invalid tool
			output := server.GetOutput()
			Expect(output).To(ContainSubstring("Unknown tool specified"))
			Expect(output).To(ContainSubstring("invalid-tool"))

			// Valid tools should still be registered
			Expect(output).To(ContainSubstring("RegisterTools initialized"))
			Expect(output).To(ContainSubstring("utils"))

			// Stop server
			err = server.Stop()
			Expect(err).NotTo(HaveOccurred(), "Server should stop gracefully")
		})

		It("should handle graceful shutdown", func() {
			config := TestServerConfig{
				Port:    8100,
				Stdio:   false,
				Timeout: 30 * time.Second,
			}

			server := NewTestServer(config)

			// Start server
			err := server.Start(ctx, config)
			Expect(err).NotTo(HaveOccurred(), "Server should start successfully")

			// Test health endpoint to ensure server is fully ready
			resp, err := http.Get(fmt.Sprintf("http://localhost:%d/health", config.Port))
			Expect(err).NotTo(HaveOccurred(), "Health endpoint should be accessible")
			Expect(resp.StatusCode).To(Equal(http.StatusOK))
			resp.Body.Close()

			// Stop server and measure shutdown time
			start := time.Now()
			err = server.Stop()
			duration := time.Since(start)

			Expect(err).NotTo(HaveOccurred(), "Server should stop gracefully")
			Expect(duration).To(BeNumerically("<", 10*time.Second), "Shutdown should complete within reasonable time")

			// Check server output for graceful shutdown
			output := server.GetOutput()
			Expect(output).To(ContainSubstring("Running KAgent Tools Server"))

			// Wait for server to fully shutdown
			shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer shutdownCancel()

			err = server.waitForShutdown(shutdownCtx, config.Port)
			Expect(err).NotTo(HaveOccurred(), "Server should shut down completely")
		})

		It("should handle concurrent requests", func() {
			config := TestServerConfig{
				Port:    8088,
				Tools:   []string{"utils", "k8s"},
				Stdio:   false,
				Timeout: 30 * time.Second,
			}

			server := NewTestServer(config)
			err := server.Start(ctx, config)
			Expect(err).NotTo(HaveOccurred(), "Server should start successfully")

			// Wait for server to be ready
			time.Sleep(3 * time.Second)

			// Create multiple concurrent requests
			var wg sync.WaitGroup
			for i := 0; i < 10; i++ {
				wg.Add(1)
				go func(id int) {
					defer wg.Done()
					resp, err := http.Get(fmt.Sprintf("http://localhost:%d/health", config.Port))
					Expect(err).NotTo(HaveOccurred(), "Concurrent request %d should succeed", id)
					Expect(resp.StatusCode).To(Equal(http.StatusOK))
					resp.Body.Close()
				}(i)
			}

			wg.Wait()
			err = server.Stop()
			Expect(err).NotTo(HaveOccurred(), "Server should stop gracefully")
		})

		It("should expose metrics endpoint", func() {
			config := TestServerConfig{
				Port:    18190,
				Tools:   []string{"utils"},
				Stdio:   false,
				Timeout: 30 * time.Second,
			}

			server := NewTestServer(config)
			err := server.Start(ctx, config)
			Expect(err).NotTo(HaveOccurred(), "Server should start successfully")

			// Wait for server to be ready
			time.Sleep(3 * time.Second)

			// Test metrics endpoint
			resp, err := http.Get(fmt.Sprintf("http://localhost:%d/metrics", config.Port))
			Expect(err).NotTo(HaveOccurred(), "Metrics endpoint should be accessible")
			Expect(resp.StatusCode).To(Equal(http.StatusOK))

			// Read and verify metrics content
			body, err := io.ReadAll(resp.Body)
			Expect(err).NotTo(HaveOccurred())
			resp.Body.Close()

			metricsContent := string(body)
			Expect(metricsContent).To(ContainSubstring("go_"))
			Expect(metricsContent).To(ContainSubstring("process_"))

			err = server.Stop()
			Expect(err).NotTo(HaveOccurred(), "Server should stop gracefully")
		})
	})

	Describe("STDIO Server Tests", func() {
		It("should start STDIO server successfully", func() {
			config := TestServerConfig{
				Stdio:   true,
				Timeout: 30 * time.Second,
			}

			server := NewTestServer(config)

			// Start server
			err := server.Start(ctx, config)
			Expect(err).NotTo(HaveOccurred(), "Server should start successfully")

			// Wait for server to be ready
			time.Sleep(3 * time.Second)

			// Check server output for STDIO mode
			output := server.GetOutput()
			Expect(output).To(ContainSubstring("Running KAgent Tools Server STDIO"))

			// Stop server
			err = server.Stop()
			Expect(err).NotTo(HaveOccurred(), "Server should stop gracefully")
		})
	})

	Describe("Tool Registration Tests", func() {
		It("should register single tool correctly", func() {
			config := TestServerConfig{
				Port:    8087,
				Tools:   []string{"k8s"},
				Timeout: 30 * time.Second,
			}

			server := NewTestServer(config)
			err := server.Start(ctx, config)
			Expect(err).NotTo(HaveOccurred(), "Server should start successfully")

			// Wait for server to be ready
			time.Sleep(3 * time.Second)

			// Verify registered tools
			output := server.GetOutput()
			Expect(output).To(ContainSubstring("Running KAgent Tools Server"))
			Expect(output).To(ContainSubstring("k8s"))

			// Test health endpoint
			resp, err := http.Get(fmt.Sprintf("http://localhost:%d/health", config.Port))
			Expect(err).NotTo(HaveOccurred(), "Health endpoint should be accessible")
			Expect(resp.StatusCode).To(Equal(http.StatusOK))
			resp.Body.Close()

			err = server.Stop()
			Expect(err).NotTo(HaveOccurred(), "Server should stop gracefully")
		})

		It("should register multiple tools correctly", func() {
			config := TestServerConfig{
				Port:    8088,
				Tools:   []string{"k8s", "prometheus", "utils"},
				Timeout: 30 * time.Second,
			}

			server := NewTestServer(config)
			err := server.Start(ctx, config)
			Expect(err).NotTo(HaveOccurred(), "Server should start successfully")

			// Wait for server to be ready
			time.Sleep(3 * time.Second)

			// Verify registered tools
			output := server.GetOutput()
			Expect(output).To(ContainSubstring("Running KAgent Tools Server"))
			for _, tool := range []string{"k8s", "prometheus", "utils"} {
				Expect(output).To(ContainSubstring(tool))
			}

			// Test health endpoint
			resp, err := http.Get(fmt.Sprintf("http://localhost:%d/health", config.Port))
			Expect(err).NotTo(HaveOccurred(), "Health endpoint should be accessible")
			Expect(resp.StatusCode).To(Equal(http.StatusOK))
			resp.Body.Close()

			err = server.Stop()
			Expect(err).NotTo(HaveOccurred(), "Server should stop gracefully")
		})

		It("should register all tools implicitly", func() {
			config := TestServerConfig{
				Port:    18190,
				Tools:   []string{},
				Timeout: 30 * time.Second,
			}

			server := NewTestServer(config)
			err := server.Start(ctx, config)
			Expect(err).NotTo(HaveOccurred(), "Server should start successfully")

			// Wait for server to be ready
			time.Sleep(3 * time.Second)

			// Verify all tools are registered
			output := server.GetOutput()
			Expect(output).To(ContainSubstring("RegisterTools initialized"))
			Expect(output).To(ContainSubstring("Running KAgent Tools Server"))

			// Test health endpoint
			resp, err := http.Get(fmt.Sprintf("http://localhost:%d/health", config.Port))
			Expect(err).NotTo(HaveOccurred(), "Health endpoint should be accessible")
			Expect(resp.StatusCode).To(Equal(http.StatusOK))
			resp.Body.Close()

			err = server.Stop()
			Expect(err).NotTo(HaveOccurred(), "Server should stop gracefully")
		})
	})

	Describe("Error Handling Tests", func() {
		It("should handle malformed requests gracefully", func() {
			config := TestServerConfig{
				Port:    8089,
				Tools:   []string{"utils"},
				Stdio:   false,
				Timeout: 10 * time.Second,
			}

			server := NewTestServer(config)
			err := server.Start(ctx, config)
			Expect(err).NotTo(HaveOccurred(), "Server should start successfully")

			// Test malformed request
			req, err := http.NewRequest("POST", fmt.Sprintf("http://localhost:%d/nonexistent", config.Port), strings.NewReader("invalid json"))
			Expect(err).NotTo(HaveOccurred())
			req.Header.Set("Content-Type", "application/json")

			client := &http.Client{}
			resp, err := client.Do(req)
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.StatusCode).To(Equal(http.StatusBadRequest))
			resp.Body.Close()

			err = server.Stop()
			Expect(err).NotTo(HaveOccurred(), "Server should stop gracefully")
		})
	})

	Describe("Environment and Configuration Tests", func() {
		It("should handle environment variables correctly", func() {
			// Set environment variables
			originalEnv := os.Environ()
			defer func() {
				os.Clearenv()
				for _, env := range originalEnv {
					parts := strings.SplitN(env, "=", 2)
					if len(parts) == 2 {
						os.Setenv(parts[0], parts[1])
					}
				}
			}()

			os.Setenv("LOG_LEVEL", "info")
			os.Setenv("OTEL_SERVICE_NAME", "test-kagent-tools")

			config := TestServerConfig{
				Port:    18195,
				Stdio:   false,
				Timeout: 30 * time.Second,
			}

			server := NewTestServer(config)

			// Start server
			err := server.Start(ctx, config)
			Expect(err).NotTo(HaveOccurred(), "Server should start successfully")

			// Wait for server to be ready
			time.Sleep(3 * time.Second)

			// Check server output
			output := server.GetOutput()
			Expect(output).To(ContainSubstring("Starting kagent-tools-server"))

			// Stop server
			err = server.Stop()
			Expect(err).NotTo(HaveOccurred(), "Server should stop gracefully")
		})

		It("should validate server binary exists and is executable", func() {
			// Check if server binary exists
			binaryName := getBinaryName()
			binaryPath := fmt.Sprintf("../../bin/%s", binaryName)
			_, err := os.Stat(binaryPath)
			if os.IsNotExist(err) {
				Skip("Server binary not found, skipping test. Run 'make build' first.")
			}
			Expect(err).NotTo(HaveOccurred(), "Server binary should exist")

			// Test --help flag
			helpCtx, helpCancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer helpCancel()

			cmd := exec.CommandContext(helpCtx, binaryPath, "--help")
			output, err := cmd.CombinedOutput()
			Expect(err).NotTo(HaveOccurred(), "Server should respond to --help flag")

			outputStr := string(output)
			Expect(outputStr).To(ContainSubstring("KAgent tool server"))
			Expect(outputStr).To(ContainSubstring("--port"))
			Expect(outputStr).To(ContainSubstring("--stdio"))
			Expect(outputStr).To(ContainSubstring("--tools"))
			Expect(outputStr).To(ContainSubstring("--kubeconfig"))
		})
	})

	Describe("Concurrent Server Instances", func() {
		It("should run multiple server instances concurrently", func() {
			var wg sync.WaitGroup
			numServers := 3
			servers := make([]*TestServer, numServers)

			// Start multiple servers on different ports
			for i := 0; i < numServers; i++ {
				wg.Add(1)
				go func(index int) {
					defer wg.Done()

					config := TestServerConfig{
						Port:    18092 + index,
						Tools:   []string{"utils"},
						Stdio:   false,
						Timeout: 30 * time.Second,
					}

					server := NewTestServer(config)
					servers[index] = server

					err := server.Start(ctx, config)
					Expect(err).NotTo(HaveOccurred(), "Server %d should start successfully", index)

					// Wait for server to be ready
					time.Sleep(3 * time.Second)

					// Test health endpoint
					resp, err := http.Get(fmt.Sprintf("http://localhost:%d/health", config.Port))
					Expect(err).NotTo(HaveOccurred(), "Health endpoint should be accessible for server %d", index)
					if resp != nil {
						resp.Body.Close()
					}
				}(i)
			}

			wg.Wait()

			// Stop all servers
			for i, server := range servers {
				if server != nil {
					err := server.Stop()
					Expect(err).NotTo(HaveOccurred(), "Server %d should stop gracefully", i)
				}
			}
		})
	})

	Describe("Telemetry Tests", func() {
		It("should initialize telemetry correctly", func() {
			config := TestServerConfig{
				Port:    18092,
				Tools:   []string{"utils"},
				Timeout: 30 * time.Second,
			}

			server := NewTestServer(config)
			err := server.Start(ctx, config)
			Expect(err).NotTo(HaveOccurred(), "Server should start successfully")

			// Wait for server to be ready
			time.Sleep(3 * time.Second)

			// Check server output for telemetry initialization
			output := server.GetOutput()
			Expect(output).To(ContainSubstring("Starting kagent-tools-server"))

			// Make a request to generate telemetry
			resp, err := http.Get(fmt.Sprintf("http://localhost:%d/health", config.Port))
			Expect(err).NotTo(HaveOccurred(), "Health endpoint should be accessible")
			Expect(resp.StatusCode).To(Equal(http.StatusOK))
			resp.Body.Close()

			// Check server output for successful startup
			output = server.GetOutput()
			Expect(output).To(ContainSubstring("Running KAgent Tools Server"))

			err = server.Stop()
			Expect(err).NotTo(HaveOccurred(), "Server should stop gracefully")
		})
	})
})

// Helper functions for test setup
func init() {
	// Ensure the binary exists before running tests
	binaryName := getBinaryName()
	binaryPath := fmt.Sprintf("../../bin/%s", binaryName)
	if _, err := os.Stat(binaryPath); os.IsNotExist(err) {
		// Try to build the binary
		cmd := exec.Command("make", "build")
		cmd.Dir = "../.."
		if err := cmd.Run(); err != nil {
			panic(fmt.Sprintf("Failed to build server binary: %v", err))
		}
	}
}
