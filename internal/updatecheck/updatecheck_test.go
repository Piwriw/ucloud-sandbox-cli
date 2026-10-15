package updatecheck

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/fatih/color"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newChecker returns a Checker on a temporary state file, whose lookups are
// counted and answered with latest.
func newChecker(t *testing.T, version, latest string, now time.Time) (*Checker, *int) {
	t.Helper()

	color.NoColor = true
	t.Cleanup(func() { color.NoColor = false })
	t.Setenv(EnvDisable, "")

	calls := 0
	return &Checker{
		Version:   version,
		StatePath: filepath.Join(t.TempDir(), "dir", stateFile),
		Latest: func(context.Context) (string, error) {
			calls++
			return latest, nil
		},
		Now: func() time.Time { return now },
	}, &calls
}

func run(c *Checker) string {
	var out strings.Builder
	c.Run(context.Background(), &out, "cli")
	return out.String()
}

func TestRunReportsANewerRelease(t *testing.T) {
	c, calls := newChecker(t, "v1.0.0", "v1.1.0", time.Now())

	assert.Equal(t, "A new version v1.1.0 is available (current v1.0.0), run cli update\n", run(c))
	assert.Equal(t, 1, *calls)
}

func TestRunStaysQuietWhenUpToDate(t *testing.T) {
	c, calls := newChecker(t, "v1.1.0", "v1.1.0", time.Now())

	assert.Empty(t, run(c))
	assert.Equal(t, 1, *calls)
}

func TestRunChecksOncePerDay(t *testing.T) {
	morning := time.Date(2026, 10, 15, 9, 0, 0, 0, time.Local)
	c, calls := newChecker(t, "v1.0.0", "v1.1.0", morning)

	assert.NotEmpty(t, run(c))

	// Later the same day nothing is looked up or shown.
	c.Now = func() time.Time { return morning.Add(12 * time.Hour) }
	assert.Empty(t, run(c))
	assert.Equal(t, 1, *calls)

	// The next day it checks again.
	c.Now = func() time.Time { return morning.Add(24 * time.Hour) }
	assert.NotEmpty(t, run(c))
	assert.Equal(t, 2, *calls)
}

func TestRunRecordsAFailedLookup(t *testing.T) {
	now := time.Now()
	c, _ := newChecker(t, "v1.0.0", "", now)

	calls := 0
	c.Latest = func(context.Context) (string, error) {
		calls++
		return "", errors.New("network down")
	}

	assert.Equal(t, "Warning: failed to check for a new version: network down\n", run(c))

	// The failure is neither retried nor reported again the same day.
	assert.Empty(t, run(c))
	assert.Equal(t, 1, calls)
}

func TestRunSkipsUnreleasedBuilds(t *testing.T) {
	for _, version := range []string{"", "unpublished", "dev"} {
		c, calls := newChecker(t, version, "v1.1.0", time.Now())

		assert.Empty(t, run(c), "version %q", version)
		assert.Equal(t, 0, *calls, "version %q", version)
	}
}

func TestRunCanBeDisabled(t *testing.T) {
	c, calls := newChecker(t, "v1.0.0", "v1.1.0", time.Now())
	t.Setenv(EnvDisable, "1")

	assert.Empty(t, run(c))
	assert.Equal(t, 0, *calls)

	// Nothing is written either.
	_, err := os.Stat(c.StatePath)
	assert.True(t, os.IsNotExist(err))
}

func TestRunRecoversFromACorruptStateFile(t *testing.T) {
	c, calls := newChecker(t, "v1.0.0", "v1.1.0", time.Now())
	require.NoError(t, os.MkdirAll(filepath.Dir(c.StatePath), 0700))
	require.NoError(t, os.WriteFile(c.StatePath, []byte("not json"), 0600))

	assert.NotEmpty(t, run(c))
	assert.Equal(t, 1, *calls)
}

func TestSameDay(t *testing.T) {
	day := time.Date(2026, 10, 15, 0, 0, 0, 0, time.Local)

	assert.True(t, sameDay(day, day.Add(23*time.Hour+59*time.Minute)))
	assert.False(t, sameDay(day, day.Add(24*time.Hour)))
	assert.False(t, sameDay(day, day.Add(-time.Minute)))
}
