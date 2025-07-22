package e2e

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"testing"
)

// TestE2EK8s is the main test runner for Kubernetes E2E tests
func TestE2EK8s(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Tools E2E Suite")
}
