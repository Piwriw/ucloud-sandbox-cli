package template

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	sdk "github.com/ucloud/ucloud-sandbox-sdk-go"
)

func logEntry(ms int64, level, step, message string) sdk.BuildLogEntry {
	return sdk.BuildLogEntry{
		Timestamp: time.UnixMilli(ms).UTC(),
		Level:     level,
		Step:      step,
		Message:   message,
	}
}

func TestFormatBuildLogEntry(t *testing.T) {
	entry := logEntry(0, "info", "step-1", "building")
	entry.Timestamp = time.Date(2026, 8, 21, 10, 30, 15, 123000000, time.Local)

	assert.Equal(t, "2026-08-21 10:30:15.123 [INFO] [step-1] building", formatBuildLogEntry(entry))
}

func TestFormatBuildLogEntryWithoutStep(t *testing.T) {
	entry := logEntry(0, "error", "", "failed")
	entry.Timestamp = time.Date(2026, 8, 21, 10, 30, 15, 0, time.Local)

	assert.Equal(t, "2026-08-21 10:30:15.000 [ERROR] failed", formatBuildLogEntry(entry))
}

func TestBuildLogsCursorSinglePage(t *testing.T) {
	cursor := newBuildLogsCursor()
	page := []sdk.BuildLogEntry{
		logEntry(1000, "info", "", "a"),
		logEntry(2000, "info", "", "b"),
	}

	fresh, more := cursor.advance(page, false)

	assert.Equal(t, page, fresh)
	assert.False(t, more)
}

func TestBuildLogsCursorEmptyPage(t *testing.T) {
	cursor := newBuildLogsCursor()

	fresh, more := cursor.advance(nil, true)

	assert.Empty(t, fresh)
	assert.False(t, more)
}

func TestBuildLogsCursorDropsRepeatedEntriesOfCursorMillisecond(t *testing.T) {
	cursor := newBuildLogsCursor()
	first := []sdk.BuildLogEntry{
		logEntry(1000, "info", "", "a"),
		logEntry(2000, "info", "", "b"),
		logEntry(2000, "info", "", "c"),
	}

	fresh, more := cursor.advance(first, true)
	require.Len(t, fresh, 3)
	assert.True(t, more)
	assert.Equal(t, int64(2000), cursor.ms)

	// The endpoint replays the entries of the cursor millisecond.
	second := []sdk.BuildLogEntry{
		logEntry(2000, "info", "", "b"),
		logEntry(2000, "info", "", "c"),
		logEntry(2000, "info", "", "d"),
		logEntry(3000, "info", "", "e"),
	}

	fresh, more = cursor.advance(second, false)
	require.Len(t, fresh, 2)
	assert.Equal(t, "d", fresh[0].Message)
	assert.Equal(t, "e", fresh[1].Message)
	assert.False(t, more)
}

func TestBuildLogsCursorStepsOverExhaustedMillisecond(t *testing.T) {
	cursor := newBuildLogsCursor()
	page := make([]sdk.BuildLogEntry, buildLogsPageSize)
	for i := range page {
		page[i] = logEntry(5000, "info", "", "same-millisecond")
	}

	// Entries of one millisecond that fill a whole page collapse into a single
	// signature, so the next identical page must not loop forever.
	fresh, more := cursor.advance(page, true)
	require.Len(t, fresh, buildLogsPageSize)
	assert.True(t, more)

	fresh, more = cursor.advance(page, true)
	assert.Empty(t, fresh)
	assert.True(t, more)
	assert.Equal(t, int64(5001), cursor.ms)
}

type fakeBuildLogsClient struct {
	pages [][]sdk.BuildLogEntry
	calls int
}

func (f *fakeBuildLogsClient) GetTemplateBuildLogs(ctx context.Context, templateID, buildID string, opts ...sdk.BuildLogsOption) ([]sdk.BuildLogEntry, error) {
	if f.calls >= len(f.pages) {
		return nil, nil
	}
	page := f.pages[f.calls]
	f.calls++
	return page, nil
}

func TestPrintBuildLogsStopsOnPartialPage(t *testing.T) {
	client := &fakeBuildLogsClient{pages: [][]sdk.BuildLogEntry{
		{logEntry(1000, "info", "", "a")},
		{logEntry(2000, "info", "", "b")},
	}}

	require.NoError(t, printBuildLogs(context.Background(), client, "tpl", "build", ""))
	assert.Equal(t, 1, client.calls)
}

func TestPrintBuildLogsFollowsPages(t *testing.T) {
	full := make([]sdk.BuildLogEntry, buildLogsPageSize)
	for i := range full {
		full[i] = logEntry(int64(1000+i), "info", "", "entry")
	}
	client := &fakeBuildLogsClient{pages: [][]sdk.BuildLogEntry{
		full,
		{logEntry(9000, "info", "", "last")},
	}}

	require.NoError(t, printBuildLogs(context.Background(), client, "tpl", "build", ""))
	assert.Equal(t, 2, client.calls)
}
