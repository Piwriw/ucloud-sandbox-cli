package sandbox

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/ucloud/ucloud-sandbox-cli/cmd"
	"github.com/ucloud/ucloud-sandbox-cli/cmd/flags"
	"github.com/ucloud/ucloud-sandbox-sdk-go/pkg/api"
	"github.com/ucloud/ucloud-sandbox-sdk-go/pkg/errdefs"
	sdksandbox "github.com/ucloud/ucloud-sandbox-sdk-go/pkg/sandbox"
	"github.com/ucloud/ucloud-sandbox-sdk-go/pkg/sandbox/commands"
	"github.com/ucloud/ucloud-sandbox-sdk-go/pkg/sandbox/pty"
	"golang.org/x/term"
)

// Terminal size used when stdin reports none.
const (
	defaultCols = 80
	defaultRows = 24
)

type connectOperation struct {
	req api.ConnectSandbox

	detached bool

	user            string
	commandsOptions commands.Options
}

func (o *connectOperation) Command() *cobra.Command {
	c := &cobra.Command{
		Use:     "connect <sandbox-id>",
		Aliases: []string{"conn"},
		Short:   "Connect a terminal to a sandbox",
		Args:    cobra.ExactArgs(1),
	}

	connectSandboxVarP(c.Flags(), &o.req)

	c.Flags().BoolVarP(&o.detached, "detached", "", false, "Show the connected sandbox instead of connecting a terminal to it")

	sandboxUserVarP(c.Flags(), &o.user)
	commandsOptionsVarP(c.Flags(), &o.commandsOptions)

	return c
}

// The registration helpers below are shared by every command that connects to
// a sandbox and runs something in it — connect, create, and later exec.

// connectSandboxVarP registers the flags of a connect request.
func connectSandboxVarP(fs *pflag.FlagSet, req *api.ConnectSandbox) {
	fs.Int32VarP(&req.Timeout, "timeout", "", 0, "Seconds from now after which the sandbox expires")
	flags.NullableBoolVarP(fs, &req.Memory, "memory", "", "Restore the memory of a paused sandbox")
}

// sandboxUserVarP registers the flag picking the user a session runs as. An
// empty user leaves the choice to the sandbox.
func sandboxUserVarP(fs *pflag.FlagSet, user *string) {
	fs.StringVarP(user, "user", "u", "", "User to start the session as")
}

// commandsOptionsVarP registers the flags shaping a command or terminal
// session inside a sandbox.
func commandsOptionsVarP(fs *pflag.FlagSet, opts *commands.Options) {
	fs.StringVarP(&opts.Cwd, "cwd", "c", "", "Working directory of the session")
	flags.StringMapVarP(fs, &opts.EnvVars, "env", "e", "Environment variable of the session")
}

func (o *connectOperation) Run(ctx cmd.OperationContext) error {
	// Connecting resumes a paused sandbox, so the terminal can be opened
	// either way.
	sbx, err := ctx.Client.Sandboxes().Connect(ctx, ctx.Args[0], o.req)
	if err != nil {
		return err
	}

	fmt.Printf("Sandbox connected: %s\n", sbx.SandboxID)
	if o.detached {
		return cmd.ShowJSON(sbx)
	}

	fmt.Printf("Terminal connecting to sandbox %s\n", sbx.SandboxID)

	if err := connectTerminal(ctx, sbx, o.user, o.commandsOptions); err != nil {
		return err
	}

	fmt.Printf("Closed the terminal of sandbox %s\n", sbx.SandboxID)

	return nil
}

func connectTerminal(ctx cmd.OperationContext, sbx *sdksandbox.Sandbox, user string, opts commands.Options) error {
	fd := int(os.Stdin.Fd())
	if !term.IsTerminal(fd) {
		return fmt.Errorf("connect needs a terminal on stdin")
	}

	cols, rows, err := term.GetSize(fd)
	if err != nil {
		cols, rows = defaultCols, defaultRows
	}

	ptySize := pty.Size{
		Rows: rows,
		Cols: cols,
	}
	terminal := sbx.Pty()
	handle, err := terminal.Create(ctx, ptySize, opts)
	if err != nil {
		return fmt.Errorf("failed to create pty: %w", err)
	}
	// Stdin, resizes and reconnects all address the shell by its PID, which
	// stays the same across reconnects.
	pid := handle.PID

	oldState, err := term.MakeRaw(fd)
	if err != nil {
		return err
	}
	defer term.Restore(fd, oldState)

	// Forward stdin to PTY. A keystroke that fails to send, while the stream
	// is being re-established say, is dropped rather than ending the session.
	go func() {
		buf := make([]byte, 256)
		for {
			n, err := os.Stdin.Read(buf)
			if err != nil {
				return
			}
			if err := terminal.SendStdin(ctx, pid, buf[:n]); err != nil && ctx.Err() != nil {
				return
			}
		}
	}()

	resize := func(ctx context.Context, size pty.Size) error {
		return terminal.Resize(ctx, pid, size)
	}
	stopResize := watchTerminalResize(ctx, fd, resize, cols, rows)
	defer stopResize()

	for {
		// Forward PTY output to stdout. Draining it here rather than in a
		// goroutine keeps output from an old stream ahead of a new one's.
		for data := range handle.Output() {
			os.Stdout.Write(data)
		}

		// The shell's own exit code is not the CLI's: a session ended by
		// "exit 1" is still a session that ran.
		_, err := handle.Wait(ctx)
		if err == nil || ctx.Err() != nil {
			return err
		}

		// The stream dropped but the shell may well still be running in the
		// sandbox, so attach to it again rather than end the session.
		handle, err = reconnectTerminal(ctx, terminal, pid, err)
		if err != nil {
			return err
		}
		if handle == nil {
			return nil
		}

		// Nothing printed while detached is replayed. A resize makes
		// full-screen programs redraw, so they at least come back intact.
		if cols, rows, err := term.GetSize(fd); err == nil {
			_ = resize(ctx, pty.Size{Cols: cols, Rows: rows})
		}
	}
}

// Reconnect attempts after a terminal's stream drops, and the delay before the
// first; each later attempt waits twice as long as the one before.
const (
	maxReconnectAttempts = 5
	reconnectBaseDelay   = 500 * time.Millisecond
)

// reconnectTerminal attaches to the terminal pid again after its stream failed
// with cause. It returns a nil handle, and no error, when the terminal turns
// out to have exited meanwhile.
func reconnectTerminal(ctx context.Context, terminal *pty.Pty, pid int, cause error) (*pty.Handle, error) {
	delay := reconnectBaseDelay
	for attempt := 1; attempt <= maxReconnectAttempts; attempt++ {
		// The terminal is in raw mode, so a bare "\n" would not return the
		// cursor to the start of the line.
		fmt.Fprintf(os.Stderr, "\r\nTerminal connection lost (%v), reconnecting (%d/%d)...\r\n", cause, attempt, maxReconnectAttempts)

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(delay):
		}
		delay *= 2

		handle, err := terminal.Connect(ctx, pid, commands.Options{})
		if err == nil {
			fmt.Fprint(os.Stderr, "Terminal reconnected\r\n")
			return handle, nil
		}
		if errdefs.IsNotFound(err) {
			// The shell exited while no stream was attached, so its exit was
			// never seen; there is nothing left to reconnect to.
			return nil, nil
		}
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		cause = err
	}

	return nil, fmt.Errorf("failed to reconnect to the terminal after %d attempts: %w", maxReconnectAttempts, cause)
}
