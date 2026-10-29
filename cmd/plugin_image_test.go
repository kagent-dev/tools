package main

import (
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"
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

// TestSkillsImageMatchesToolsImage checks that the skills-only image ships
// the tools image's opt/kagent-tools-skills and nothing else.
func TestSkillsImageMatchesToolsImage(t *testing.T) {
	toolsRoot := os.Getenv("KAGENT_TOOLS_IMAGE_ROOT")
	skillsImageRoot := os.Getenv("KAGENT_TOOLS_SKILLS_IMAGE_ROOT")
	if toolsRoot == "" || skillsImageRoot == "" {
		t.Skip("set KAGENT_TOOLS_IMAGE_ROOT and KAGENT_TOOLS_SKILLS_IMAGE_ROOT to exported image filesystems (scripts/verify-plugin-image.sh)")
	}

	// docker export adds .dockerenv, dev, etc, proc, and sys to every container.
	top, err := os.ReadDir(skillsImageRoot)
	require.NoError(t, err)
	for _, entry := range top {
		require.Contains(t, []string{".dockerenv", "dev", "etc", "proc", "sys", "opt"}, entry.Name(), "unexpected top-level entry in the skills image")
	}
	opt, err := os.ReadDir(filepath.Join(skillsImageRoot, "opt"))
	require.NoError(t, err)
	require.Len(t, opt, 1)
	require.Equal(t, "kagent-tools-skills", opt[0].Name())

	want := snapshotTree(t, filepath.Join(toolsRoot, "opt", "kagent-tools-skills"))
	got := snapshotTree(t, filepath.Join(skillsImageRoot, "opt", "kagent-tools-skills"))
	require.NotEmpty(t, want)
	require.Equal(t, slices.Sorted(maps.Keys(want)), slices.Sorted(maps.Keys(got)))
	for path, entry := range want {
		require.Equal(t, entry, got[path], path)
	}
}

// snapshotTree maps each path under root to its mode, plus the content of a
// file or the target of a symlink.
func snapshotTree(t *testing.T, root string) map[string]string {
	t.Helper()
	tree := map[string]string{}
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		require.NoError(t, err)
		rel, err := filepath.Rel(root, path)
		require.NoError(t, err)
		info, err := entry.Info()
		require.NoError(t, err)
		tree[rel] = info.Mode().String()
		switch {
		case info.Mode().IsRegular():
			content, err := os.ReadFile(path)
			require.NoError(t, err)
			tree[rel] += "\n" + string(content)
		case info.Mode()&os.ModeSymlink != 0:
			target, err := os.Readlink(path)
			require.NoError(t, err)
			tree[rel] += " -> " + target
		}
		return nil
	})
	require.NoError(t, err)
	return tree
}
