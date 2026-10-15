package auth

import (
	"strings"
	"testing"
	"time"

	"github.com/fatih/color"
	"github.com/stretchr/testify/assert"
	"github.com/ucloud/ucloud-sandbox-cli/internal/whoami"
)

func TestWriteLoginSuccess(t *testing.T) {
	color.NoColor = true
	t.Cleanup(func() { color.NoColor = false })

	now := time.Date(2026, 10, 15, 12, 0, 0, 0, time.Local)

	t.Run("aligns every known part", func(t *testing.T) {
		var out strings.Builder
		writeLoginSuccess(&out, &whoami.Response{
			Company: whoami.Company{ID: 1, Name: "Acme"},
			Project: whoami.Project{ID: 2},
			APIKey:  &whoami.APIKey{KeyID: "key-1", Name: "my key"},
		}, now)

		assert.Equal(t, "✔ Logged in successfully\n"+
			"  Company  Acme\n"+
			"  Project  2\n"+
			"  API key  my key\n", out.String())
	})

	t.Run("shows when the API key expires", func(t *testing.T) {
		expire := now.Add(30 * 24 * time.Hour)

		var out strings.Builder
		writeLoginSuccess(&out, &whoami.Response{
			Company: whoami.Company{Name: "Acme"},
			APIKey:  &whoami.APIKey{Name: "my key", ExpireTime: &expire},
		}, now)

		assert.Equal(t, "✔ Logged in successfully\n"+
			"  Company  Acme\n"+
			"  API key  my key\n"+
			"  Expires  in 30 days (2026-11-14 12:00)\n", out.String())
	})

	t.Run("nothing known", func(t *testing.T) {
		var out strings.Builder
		writeLoginSuccess(&out, &whoami.Response{}, now)

		assert.Equal(t, "✔ Logged in successfully\n", out.String())
	})
}

func TestDescribeExpiry(t *testing.T) {
	now := time.Date(2026, 10, 15, 12, 0, 0, 0, time.Local)

	text, style := describeExpiry(now.Add(30*24*time.Hour), now)
	assert.Equal(t, "in 30 days (2026-11-14 12:00)", text)
	assert.True(t, style.Equals(color.New(color.Bold)))

	text, style = describeExpiry(now.Add(3*24*time.Hour), now)
	assert.Equal(t, "in 3 days (2026-10-18 12:00), renew it soon", text)
	assert.True(t, style.Equals(color.New(color.FgYellow, color.Bold)))

	text, style = describeExpiry(now.Add(-time.Hour), now)
	assert.Equal(t, "expired at 2026-10-15 11:00", text)
	assert.True(t, style.Equals(color.New(color.FgRed, color.Bold)))
}

func TestFormatRemaining(t *testing.T) {
	assert.Equal(t, "1 day", formatRemaining(36*time.Hour))
	assert.Equal(t, "9 days", formatRemaining(9*24*time.Hour+23*time.Hour))
	assert.Equal(t, "5 hours", formatRemaining(5*time.Hour+59*time.Minute))
	assert.Equal(t, "1 hour", formatRemaining(time.Hour))
	assert.Equal(t, "42 minutes", formatRemaining(42*time.Minute))
	assert.Equal(t, "1 minute", formatRemaining(10*time.Second))
}
