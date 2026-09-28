package selfupdate

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
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

// CanSudo reports whether a denied install can be retried through sudo: the
// platform has it, the process is not already root, and sudo is on PATH.
func CanSudo() bool {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		return false
	}
	_, err := exec.LookPath("sudo")
	return err == nil
}

// ApplyWithSudo replaces the binary at path with the new one through sudo.
// Only the final install step runs elevated: the binary is staged in the
// temporary directory as the current user and then handed to install(1),
// which unlinks the old file first so a running binary can be replaced.
func ApplyWithSudo(ctx context.Context, binary []byte, path string) error {
	sudo, err := exec.LookPath("sudo")
	if err != nil {
		return fmt.Errorf("locate sudo: %w", err)
	}

	staged, err := os.CreateTemp("", "."+BinaryName+".update-*")
	if err != nil {
		return fmt.Errorf("stage the new binary: %w", err)
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

	cmd := exec.CommandContext(ctx, sudo, "install", "-m", "0755", stagedPath, path)
	// sudo may need the terminal to ask for a password.
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("install the new binary with sudo: %w", err)
	}
	return nil
}

// PermissionError reports that the install location is not writable by the
// current user. It unwraps to the underlying error, so callers can match it
// with fs.ErrPermission.
type PermissionError struct {
	Path string
	Err  error
}

func (e *PermissionError) Error() string {
	return fmt.Sprintf("no permission to write to %s, re-run with elevated privileges (for example: sudo %s update)", e.Path, BinaryName)
}

func (e *PermissionError) Unwrap() error {
	return e.Err
}

// permissionError turns a denied write into a message that tells the user how
// to retry, because the CLI is commonly installed into a system directory.
func permissionError(err error, path string) error {
	if errors.Is(err, fs.ErrPermission) {
		return &PermissionError{Path: path, Err: err}
	}
	return fmt.Errorf("install the new binary: %w", err)
}
