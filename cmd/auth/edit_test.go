package auth

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/ucloud/ucloud-sandbox-cli/internal/config"
)

func TestResolveEditor(t *testing.T) {
	t.Run("the command line wins", func(t *testing.T) {
		t.Setenv("EDITOR", "emacs")
		assert.Equal(t, "nano", resolveEditor([]string{"nano"}))
	})

	t.Run("then $EDITOR", func(t *testing.T) {
		t.Setenv("EDITOR", "emacs")
		assert.Equal(t, "emacs", resolveEditor(nil))
	})

	t.Run("an empty $EDITOR falls back", func(t *testing.T) {
		t.Setenv("EDITOR", "")
		assert.Equal(t, fallbackEditor, resolveEditor(nil))
	})

	t.Run("a blank $EDITOR falls back", func(t *testing.T) {
		t.Setenv("EDITOR", "   ")
		assert.Equal(t, fallbackEditor, resolveEditor(nil))
	})

	t.Run("a blank argument falls back", func(t *testing.T) {
		t.Setenv("EDITOR", "")
		assert.Equal(t, fallbackEditor, resolveEditor([]string{"  "}))
	})
}

func TestEditConfigCreatesTheFileFirst(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	// A script standing in for the editor: it records the path it was handed.
	marker := filepath.Join(home, "opened")
	editor := filepath.Join(home, "fake-editor")
	require.NoError(t, os.WriteFile(editor,
		[]byte("#!/bin/sh\nprintf '%s' \"$1\" > "+marker+"\n"), 0700))

	require.NoError(t, editConfig(editor))

	path := filepath.Join(home, ".ucloud-sandbox-cli", "config.json")

	opened, err := os.ReadFile(marker)
	require.NoError(t, err)
	assert.Equal(t, path, string(opened), "the editor is handed the config file")

	// The file the editor opened is valid JSON listing every setting.
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Contains(t, string(data), `"api_key"`)
	assert.Contains(t, string(data), `"registries"`)
}

func TestEditConfigKeepsAnExistingFile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	require.NoError(t, config.Save(&config.Config{APIKey: "keep-me", Region: "cn-sh"}))

	require.NoError(t, editConfig("true")) // a no-op "editor"

	cfg, err := config.LoadFile()
	require.NoError(t, err)
	assert.Equal(t, "keep-me", cfg.APIKey)
	assert.Equal(t, "cn-sh", cfg.Region)
}

func TestEditConfigReportsAnUnparseableResult(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	path := filepath.Join(home, ".ucloud-sandbox-cli", "config.json")

	// An "editor" that leaves broken JSON behind.
	editor := filepath.Join(home, "breaking-editor")
	require.NoError(t, os.WriteFile(editor,
		[]byte("#!/bin/sh\nprintf 'not json' > \"$1\"\n"), 0700))

	err := editConfig(editor)
	require.Error(t, err)
	assert.Contains(t, err.Error(), path)
	assert.Contains(t, err.Error(), "no longer parses")
}

func TestEditConfigReportsAMissingEditor(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	err := editConfig("no-such-editor-exists")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no-such-editor-exists")
}
