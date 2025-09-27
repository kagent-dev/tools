package integration

import (
	"io"
	"runtime"
)

// getBinaryName returns the platform-specific binary name
func getBinaryName() string {
	switch runtime.GOOS {
	case "windows":
		return "windows-amd64.exe"
	case "darwin":
		if runtime.GOARCH == "arm64" {
			return "darwin-arm64"
		}
		return "darwin-amd64"
	default:
		if runtime.GOARCH == "arm64" {
			return "linux-arm64"
		}
		return "linux-amd64"
	}
}

// closeBody closes the response body while ignoring the returned error.
func closeBody(b io.ReadCloser) {
	if b != nil {
		_ = b.Close()
	}
}
