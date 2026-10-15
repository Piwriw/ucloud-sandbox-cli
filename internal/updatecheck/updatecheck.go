// Package updatecheck tells the user, once a day, that a newer release of the
// CLI is out.
package updatecheck

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/fatih/color"
	"github.com/ucloud/ucloud-sandbox-cli/internal/config"
	"github.com/ucloud/ucloud-sandbox-cli/internal/selfupdate"
)

const (
	stateFile = "update-check.json"

	// EnvDisable turns the check off when set to anything non-empty.
	EnvDisable = "UCLOUD_SANDBOX_NO_UPDATE_CHECK"

	// timeout bounds the lookup, since it holds up the command the user
	// actually ran.
	timeout = 3 * time.Second
)

// state is what is remembered between runs.
type state struct {
	LastCheck time.Time `json:"last_check"`
}

// Checker looks up the latest release and reports it when it is newer than
// Version.
type Checker struct {
	// Version is the version of the running binary.
	Version string
	// StatePath is where the time of the last check is kept.
	StatePath string
	// Latest returns the tag of the latest release.
	Latest func(ctx context.Context) (string, error)
	// Now returns the current time.
	Now func() time.Time
}

// New returns a Checker for version that asks GitHub for the latest release.
func New(version string) (*Checker, error) {
	dir, err := config.Dir()
	if err != nil {
		return nil, err
	}

	client := selfupdate.NewClient()
	return &Checker{
		Version:   version,
		StatePath: filepath.Join(dir, stateFile),
		Latest: func(ctx context.Context) (string, error) {
			release, err := client.LatestRelease(ctx)
			if err != nil {
				return "", err
			}
			return release.TagName, nil
		},
		Now: time.Now,
	}, nil
}

// Run checks for a newer release, at most once per local calendar day, and
// writes a notice to w when there is one.
//
// It never fails the command the user ran: a lookup that fails is reported as
// a warning on w, and anything else that goes wrong is dropped.
func (c *Checker) Run(ctx context.Context, w io.Writer, binary string) {
	if os.Getenv(EnvDisable) != "" {
		return
	}

	// A binary built outside of a release has nothing to compare against.
	if _, err := selfupdate.Normalize(c.Version); err != nil {
		return
	}

	now := c.Now()
	if last, ok := c.lastCheck(); ok && sameDay(last, now) {
		return
	}

	// Recorded before the lookup, so a network that fails it does not hold up
	// every command for the rest of the day.
	if err := c.saveLastCheck(now); err != nil {
		return
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	latest, err := c.Latest(ctx)
	if err != nil {
		color.New(color.FgYellow).Fprintf(w,
			"Warning: failed to check for a new version: %v\n", err)
		return
	}

	newer, err := selfupdate.IsNewer(c.Version, latest)
	if err != nil || !newer {
		return
	}

	notice := color.New(color.FgYellow)
	command := color.New(color.Bold)
	fmt.Fprintf(w, "%s %s\n",
		notice.Sprintf("A new version %s is available (current %s), run", latest, c.Version),
		command.Sprintf("%s update", binary))
}

func (c *Checker) lastCheck() (time.Time, bool) {
	data, err := os.ReadFile(c.StatePath)
	if err != nil {
		return time.Time{}, false
	}

	var s state
	if err := json.Unmarshal(data, &s); err != nil {
		return time.Time{}, false
	}

	return s.LastCheck, !s.LastCheck.IsZero()
}

func (c *Checker) saveLastCheck(t time.Time) error {
	data, err := json.Marshal(state{LastCheck: t})
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(c.StatePath), 0700); err != nil {
		return err
	}

	return os.WriteFile(c.StatePath, data, 0600)
}

// sameDay reports whether a and b fall on the same local calendar day.
func sameDay(a, b time.Time) bool {
	ay, am, ad := a.Local().Date()
	by, bm, bd := b.Local().Date()
	return ay == by && am == bm && ad == bd
}
