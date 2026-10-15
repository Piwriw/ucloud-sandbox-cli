package auth

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/spf13/cobra"
	"github.com/ucloud/ucloud-sandbox-cli/cmd"
	"github.com/ucloud/ucloud-sandbox-cli/internal/config"
)

// fallbackEditor opens the config file when $EDITOR says nothing.
const fallbackEditor = "vim"

type editOperation struct{}

func (o *editOperation) Command() *cobra.Command {
	return &cobra.Command{
		Use:   "edit [editor]",
		Short: "Open the config file in an editor ($EDITOR, or " + fallbackEditor + ")",
		Args:  cobra.MaximumNArgs(1),
	}
}

func (o *editOperation) Run(ctx cmd.OperationContext) error {
	return editConfig(resolveEditor(ctx.Args))
}

// resolveEditor picks the editor to open the config file with: the one named
// on the command line, then $EDITOR, then a fallback.
func resolveEditor(args []string) string {
	if len(args) > 0 && strings.TrimSpace(args[0]) != "" {
		return strings.TrimSpace(args[0])
	}

	if editor := strings.TrimSpace(os.Getenv("EDITOR")); editor != "" {
		return editor
	}

	return fallbackEditor
}

// editConfig opens the config file in editor and reports whether what came
// back still parses.
func editConfig(editor string) error {
	path, err := config.Path()
	if err != nil {
		return err
	}

	// An editor started on a path whose directory does not exist cannot save,
	// so the file is created first. It also gives the user valid JSON to edit
	// rather than an empty buffer.
	if _, err := os.Stat(path); os.IsNotExist(err) {
		if err := config.Save(&config.Config{}); err != nil {
			return err
		}
	} else if err != nil {
		return fmt.Errorf("stat config: %w", err)
	}

	// $EDITOR may carry arguments of its own, for example "code --wait".
	parts := strings.Fields(editor)
	if len(parts) == 0 {
		return fmt.Errorf("no editor to open %s with", path)
	}

	command := exec.Command(parts[0], append(parts[1:], path)...)
	command.Stdin = os.Stdin
	command.Stdout = os.Stdout
	command.Stderr = os.Stderr

	if err := command.Run(); err != nil {
		return fmt.Errorf("run editor %q: %w", editor, err)
	}

	// Saying so now beats the next command failing on a file the user no
	// longer has in mind.
	if _, err := config.LoadFile(); err != nil {
		return fmt.Errorf("%s was saved but no longer parses: %w", path, err)
	}

	return nil
}
