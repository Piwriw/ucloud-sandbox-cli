package auth

import (
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
	"github.com/ucloud/ucloud-sandbox-cli/cmd"
	"github.com/ucloud/ucloud-sandbox-cli/internal/config"
	"github.com/ucloud/ucloud-sandbox-cli/internal/prompt"
	"github.com/ucloud/ucloud-sandbox-cli/internal/whoami"
)

type loginOperation struct{}

func (o *loginOperation) Command() *cobra.Command {
	return &cobra.Command{
		Use:   "login",
		Short: "Authenticate with UCloud Sandbox",
		Args:  cobra.NoArgs,
	}
}

func (o *loginOperation) Run(ctx cmd.OperationContext) error {
	apiKey, err := prompt.AskAPIKey()
	if err != nil {
		return err
	}

	region, err := prompt.AskRegion(true)
	if err != nil {
		return err
	}

	// The key is checked against the config the other commands would run
	// with, environment overrides included, before anything is saved.
	resolved, err := config.Load()
	if err != nil {
		return err
	}
	applyLogin(resolved, apiKey, region)

	who, err := lookupIdentity(ctx, resolved)
	if errors.Is(err, errInvalidAPIKey) {
		// Already says what went wrong and what to do; a prefix adds nothing.
		return err
	}
	if err != nil {
		return fmt.Errorf("verify API key: %w", err)
	}

	// The file is read rather than replaced, so settings logging in says
	// nothing about -- the domain, the registry credentials -- survive it.
	cfg, err := config.LoadFile()
	if err != nil {
		return err
	}
	applyLogin(cfg, apiKey, region)

	if err := config.Save(cfg); err != nil {
		return err
	}

	writeLoginSuccess(ctx.Cmd.OutOrStdout(), who, time.Now())

	return nil
}

// expirySoon is how close to expiring an API key is when the login calls
// attention to it.
const expirySoon = 10 * 24 * time.Hour

// writeLoginSuccess reports a login with who it logged in as, one aligned line
// per known part of the identity, and how long the API key has left when it
// expires at all.
//
// color leaves the text plain when stdout is not a terminal or NO_COLOR is
// set.
func writeLoginSuccess(w io.Writer, who *whoami.Response, now time.Time) {
	check := color.New(color.FgGreen, color.Bold)
	label := color.New(color.Faint)
	value := color.New(color.Bold)

	fmt.Fprintf(w, "%s Logged in successfully\n", check.Sprint("✔"))

	type row struct {
		field whoami.Field
		style *color.Color
	}
	var rows []row
	for _, f := range who.Fields() {
		rows = append(rows, row{field: f, style: value})
	}
	if who.APIKey != nil && who.APIKey.ExpireTime != nil {
		text, style := describeExpiry(*who.APIKey.ExpireTime, now)
		rows = append(rows, row{field: whoami.Field{Label: "Expires", Value: text}, style: style})
	}

	width := 0
	for _, r := range rows {
		width = max(width, len(r.field.Label))
	}

	for _, r := range rows {
		// Padded before styling, so the escape codes do not count toward the
		// width.
		fmt.Fprintf(w, "  %s  %s\n",
			label.Sprintf("%-*s", width, r.field.Label), r.style.Sprint(r.field.Value))
	}
}

// describeExpiry says when an API key expires, styled by how soon that is.
func describeExpiry(expire, now time.Time) (string, *color.Color) {
	at := expire.Local().Format("2006-01-02 15:04")
	left := expire.Sub(now)

	switch {
	case left <= 0:
		return "expired at " + at, color.New(color.FgRed, color.Bold)
	case left < expirySoon:
		return fmt.Sprintf("in %s (%s), renew it soon", formatRemaining(left), at),
			color.New(color.FgYellow, color.Bold)
	default:
		return fmt.Sprintf("in %s (%s)", formatRemaining(left), at), color.New(color.Bold)
	}
}

// formatRemaining renders a positive duration in its largest whole unit, from
// days down to minutes.
func formatRemaining(d time.Duration) string {
	plural := func(n int, unit string) string {
		if n == 1 {
			return "1 " + unit
		}
		return fmt.Sprintf("%d %ss", n, unit)
	}

	switch {
	case d >= 24*time.Hour:
		return plural(int(d/(24*time.Hour)), "day")
	case d >= time.Hour:
		return plural(int(d/time.Hour), "hour")
	default:
		// Under a minute still reads as a minute rather than "0 minutes".
		return plural(max(1, int(d/time.Minute)), "minute")
	}
}

// applyLogin sets what the login prompts asked for on cfg.
func applyLogin(cfg *config.Config, apiKey, region string) {
	cfg.APIKey = apiKey

	// Skipping the region prompt leaves the configured one alone.
	if region != "" {
		cfg.Region = region
	}
}
