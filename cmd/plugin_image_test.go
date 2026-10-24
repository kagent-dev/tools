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
	var stats packageStats
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
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

func TestPluginImageLayout(t *testing.T) {
	imageRoot := os.Getenv("KAGENT_TOOLS_IMAGE_ROOT")
	if imageRoot == "" {
		t.Skip("set KAGENT_TOOLS_IMAGE_ROOT to an exported image filesystem (scripts/verify-plugin-image.sh)")
	}

	fullRoot := filepath.Join(imageRoot, "opt", "kagent-tools")
	for _, binary := range []string{"kagent-tools", "kubectl", "helm", "istioctl", "kubectl-argo-rollouts", "cilium"} {
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

	target, err := os.Readlink(filepath.Join(imageRoot, "tool-server"))
	require.NoError(t, err)
	require.Equal(t, "/opt/kagent-tools/bin/kagent-tools", target)
}
