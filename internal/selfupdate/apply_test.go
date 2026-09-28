package selfupdate

import (
	"context"
	"io/fs"
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
	assert.ErrorIs(t, err, fs.ErrPermission)
}

// fakeSudo puts a sudo on PATH that runs its arguments unprivileged, so the
// install flow can be exercised without real elevation.
func fakeSudo(t *testing.T) {
	t.Helper()

	bin := t.TempDir()
	script := "#!/bin/sh\nexec \"$@\"\n"
	require.NoError(t, os.WriteFile(filepath.Join(bin, "sudo"), []byte(script), 0o755))
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func TestApplyWithSudo(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("sudo is not available on Windows")
	}
	fakeSudo(t)

	dir := t.TempDir()
	target := filepath.Join(dir, BinaryName)
	require.NoError(t, os.WriteFile(target, []byte("old binary"), 0o644))

	require.NoError(t, ApplyWithSudo(context.Background(), []byte("new binary"), target))

	content, err := os.ReadFile(target)
	require.NoError(t, err)
	assert.Equal(t, []byte("new binary"), content)

	info, err := os.Stat(target)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o755), info.Mode().Perm())
}

func TestApplyWithSudoReportsFailure(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("sudo is not available on Windows")
	}
	fakeSudo(t)

	target := filepath.Join(t.TempDir(), "missing", BinaryName)

	err := ApplyWithSudo(context.Background(), []byte("new binary"), target)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "install the new binary with sudo")
}

func TestCanSudo(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		assert.False(t, CanSudo())
		return
	}

	t.Setenv("PATH", t.TempDir())
	assert.False(t, CanSudo())

	fakeSudo(t)
	assert.True(t, CanSudo())
}

func TestExecutablePath(t *testing.T) {
	path, err := ExecutablePath()
	require.NoError(t, err)
	assert.True(t, filepath.IsAbs(path))
}
