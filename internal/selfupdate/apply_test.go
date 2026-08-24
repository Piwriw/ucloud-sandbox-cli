package selfupdate

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestApply(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, BinaryName)
	require.NoError(t, os.WriteFile(target, []byte("old binary"), 0o755))

	require.NoError(t, Apply([]byte("new binary"), target))

	content, err := os.ReadFile(target)
	require.NoError(t, err)
	assert.Equal(t, []byte("new binary"), content)

	if runtime.GOOS != "windows" {
		info, err := os.Stat(target)
		require.NoError(t, err)
		assert.Equal(t, os.FileMode(0o755), info.Mode().Perm())
	}
}

func TestApplyLeavesNoStagedFile(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, BinaryName)
	require.NoError(t, os.WriteFile(target, []byte("old binary"), 0o755))

	require.NoError(t, Apply([]byte("new binary"), target))

	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	for _, entry := range entries {
		assert.NotContains(t, entry.Name(), ".update-")
	}
}

func TestApplyReportsPermissionError(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("directory permissions are not enforced the same way on Windows")
	}
	if os.Geteuid() == 0 {
		t.Skip("root bypasses directory permissions")
	}

	dir := t.TempDir()
	target := filepath.Join(dir, BinaryName)
	require.NoError(t, os.WriteFile(target, []byte("old binary"), 0o755))
	require.NoError(t, os.Chmod(dir, 0o500))
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })

	err := Apply([]byte("new binary"), target)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no permission to write to")
}

func TestExecutablePath(t *testing.T) {
	path, err := ExecutablePath()
	require.NoError(t, err)
	assert.True(t, filepath.IsAbs(path))
}
