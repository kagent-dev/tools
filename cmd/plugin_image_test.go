package main

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

const (
	kagentMaxPackageEntries = 10_000
	kagentMaxPackageBytes   = 100 << 20
)

type packageStats struct {
	Entries int64
	Bytes   int64
}

// walkPluginPackage mirrors kagent's validatePackage checks for one plugin root.
func walkPluginPackage(t *testing.T, root string) packageStats {
	t.Helper()
	// Resolve the root itself so in-root symlinks compare equal on hosts where
	// the temp dir is behind a symlink (macOS /var -> /private/var).
	root, err := filepath.EvalSymlinks(root)
	require.NoError(t, err)
	var stats packageStats
	err = filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		require.NoError(t, err)
		stats.Entries++
		info, err := entry.Info()
		require.NoError(t, err)
		mode := info.Mode()
		require.Truef(t, mode.IsRegular() || mode.IsDir() || mode&os.ModeSymlink != 0, "unsupported file %s (%s)", path, mode)
		if mode.IsRegular() {
			stats.Bytes += info.Size()
		}
		if mode&os.ModeSymlink != 0 {
			resolved, err := filepath.EvalSymlinks(path)
			require.NoError(t, err)
			require.Truef(t, strings.HasPrefix(resolved, root+string(filepath.Separator)), "symlink %s escapes %s", path, root)
		}
		return nil
	})
	require.NoError(t, err)
	return stats
}

// bundledBinaries lists the executables under /opt/kagent-tools/bin; the MCP
// server comes first, the CLIs it wraps follow and also get /usr/bin links.
var bundledBinaries = []string{"kagent-tools", "kubectl", "helm", "istioctl", "kubectl-argo-rollouts", "cilium"}

func TestPluginImageLayout(t *testing.T) {
	imageRoot := os.Getenv("KAGENT_TOOLS_IMAGE_ROOT")
	if imageRoot == "" {
		t.Skip("set KAGENT_TOOLS_IMAGE_ROOT to an exported image filesystem (scripts/verify-plugin-image.sh)")
	}

	fullRoot := filepath.Join(imageRoot, "opt", "kagent-tools")
	for _, binary := range bundledBinaries {
		info, err := os.Stat(filepath.Join(fullRoot, "bin", binary))
		require.NoError(t, err, binary)
		require.NotZero(t, info.Mode().Perm()&0o111, "%s must be executable", binary)
	}
	for _, file := range []string{"plugin.json", "mcp.json", "config/kubeconfig"} {
		require.FileExists(t, filepath.Join(fullRoot, file))
	}
	fullStats := walkPluginPackage(t, fullRoot)
	require.Less(t, fullStats.Entries, int64(kagentMaxPackageEntries))
	t.Logf("full plugin root: %d entries, %d MiB (requires kagent mounted-package size exemption)", fullStats.Entries, fullStats.Bytes>>20)

	skillsRoot := filepath.Join(imageRoot, "opt", "kagent-tools-skills")
	require.FileExists(t, filepath.Join(skillsRoot, "plugin.json"))
	require.NoFileExists(t, filepath.Join(skillsRoot, "mcp.json"))
	skillsStats := walkPluginPackage(t, skillsRoot)
	require.Less(t, skillsStats.Entries, int64(kagentMaxPackageEntries))
	require.Less(t, skillsStats.Bytes, int64(kagentMaxPackageBytes))

	fullSkills, err := os.ReadDir(filepath.Join(fullRoot, "skills"))
	require.NoError(t, err)
	onlySkills, err := os.ReadDir(filepath.Join(skillsRoot, "skills"))
	require.NoError(t, err)
	require.Equal(t, len(fullSkills), len(onlySkills))

	legacyLinks := map[string]string{"tool-server": "/opt/kagent-tools/bin/kagent-tools"}
	for _, binary := range bundledBinaries[1:] {
		legacyLinks[filepath.Join("usr", "bin", binary)] = "/opt/kagent-tools/bin/" + binary
	}
	for link, expectedTarget := range legacyLinks {
		target, err := os.Readlink(filepath.Join(imageRoot, link))
		require.NoError(t, err, link)
		require.Equal(t, expectedTarget, target, link)
	}
}
