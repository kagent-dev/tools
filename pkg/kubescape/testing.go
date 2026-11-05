package kubescape

import (
	spdxv1beta1 "github.com/kubescape/storage/pkg/generated/clientset/versioned/typed/softwarecomposition/v1beta1"
	"k8s.io/client-go/kubernetes"
)

// NewKubescapeToolWithClients creates a KubescapeTool with pre-configured clients for testing
func NewKubescapeToolWithClients(
	k8sClient kubernetes.Interface,
	spdxClient spdxv1beta1.SpdxV1beta1Interface,
) *KubescapeTool {
	return &KubescapeTool{
		k8sClient:  k8sClient,
		spdxClient: spdxClient,
		initError:  nil,
	}
}

// NewKubescapeToolWithError creates a KubescapeTool with an initialization error for testing error paths
func NewKubescapeToolWithError(err error) *KubescapeTool {
	return &KubescapeTool{
		initError: err,
	}
}
