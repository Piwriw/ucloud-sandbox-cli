package selfupdate

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
)

// ExecutablePath returns the path of the running binary with symlinks
// resolved, which is the file an update has to replace.
func ExecutablePath() (string, error) {
	path, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("locate the running binary: %w", err)
	}

	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", fmt.Errorf("resolve %s: %w", path, err)
	}
	return resolved, nil
}

// Apply replaces the binary at path with the new one. The replacement is
// staged in the same directory and moved into place, so the binary is never
// left half-written.
func Apply(binary []byte, path string) error {
	dir := filepath.Dir(path)

	staged, err := os.CreateTemp(dir, "."+BinaryName+".update-*")
	if err != nil {
		return permissionError(err, dir)
	}
	stagedPath := staged.Name()
	defer os.Remove(stagedPath)

	if _, err := staged.Write(binary); err != nil {
		staged.Close()
		return fmt.Errorf("write the new binary: %w", err)
	}
	if err := staged.Close(); err != nil {
		return fmt.Errorf("write the new binary: %w", err)
	}
	if err := os.Chmod(stagedPath, 0o755); err != nil {
		return fmt.Errorf("make the new binary executable: %w", err)
	}

	// Windows refuses to overwrite a running executable, but it does allow
	// renaming it out of the way first.
	if runtime.GOOS == "windows" {
		previous := path + ".old"
		os.Remove(previous)
		if err := os.Rename(path, previous); err != nil {
			return permissionError(err, path)
		}
		if err := os.Rename(stagedPath, path); err != nil {
			// Put the old binary back so the CLI stays usable.
			os.Rename(previous, path)
			return permissionError(err, path)
		}
		// The running process still holds the old file, so this only
		// succeeds on a later run. Leaving it behind is harmless.
		os.Remove(previous)
		return nil
	}

	if err := os.Rename(stagedPath, path); err != nil {
		return permissionError(err, path)
	}
	return nil
}

// permissionError turns a denied write into a message that tells the user how
// to retry, because the CLI is commonly installed into a system directory.
func permissionError(err error, path string) error {
	if errors.Is(err, fs.ErrPermission) {
		return fmt.Errorf("no permission to write to %s, re-run with elevated privileges (for example: sudo %s update)", path, BinaryName)
	}
	return fmt.Errorf("install the new binary: %w", err)
}
