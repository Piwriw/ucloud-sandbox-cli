package cmd

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"runtime"
	"time"

	"github.com/schollz/progressbar/v3"
	"github.com/spf13/cobra"
	"github.com/ucloud/ucloud-sandbox-cli/internal/prompt"
	"github.com/ucloud/ucloud-sandbox-cli/internal/selfupdate"
)

// NewUpdateCmd creates the update command. currentVersion is the version
// stamped into this binary at build time.
func NewUpdateCmd(currentVersion string) *cobra.Command {
	var dryRun, yes bool

	cmd := &cobra.Command{
		Use:   "update",
		Short: "Update the CLI to the latest release",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			if ctx == nil {
				ctx = context.Background()
			}
			return runUpdate(ctx, currentVersion, dryRun, yes)
		},
	}

	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Only check for a newer release and show what would be downloaded")
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "Skip confirmation")

	return cmd
}

func runUpdate(ctx context.Context, currentVersion string, dryRun, yes bool) error {
	assetName, err := selfupdate.AssetName(runtime.GOOS, runtime.GOARCH)
	if err != nil {
		return err
	}

	executable, err := selfupdate.ExecutablePath()
	if err != nil {
		return err
	}

	fmt.Println("Checking for a newer release...")
	client := selfupdate.NewClient()
	release, err := client.LatestRelease(ctx)
	if err != nil {
		return err
	}

	displayedVersion := currentVersion
	if displayedVersion == "" {
		displayedVersion = "unknown"
	}
	fmt.Printf("Current version: %s\n", displayedVersion)
	fmt.Printf("Latest version:  %s\n", release.TagName)

	newer, err := selfupdate.IsNewer(currentVersion, release.TagName)
	if err != nil {
		if _, latestErr := selfupdate.Normalize(release.TagName); latestErr != nil {
			return latestErr
		}
		// A binary built outside of a release carries no comparable version,
		// so fall back to installing the latest release.
		fmt.Printf("Version %q is not a released version, updating to %s.\n", displayedVersion, release.TagName)
		newer = true
	}

	if !newer {
		fmt.Println("Already up to date.")
		return nil
	}

	asset, ok := release.Asset(assetName)
	if !ok {
		return fmt.Errorf("release %s does not provide %s", release.TagName, assetName)
	}
	checksumAsset, ok := release.Asset(assetName + ".sha256")
	if !ok {
		return fmt.Errorf("release %s does not provide %s.sha256", release.TagName, assetName)
	}

	if dryRun {
		fmt.Println()
		fmt.Printf("Platform:     %s/%s\n", runtime.GOOS, runtime.GOARCH)
		fmt.Printf("Download URL: %s\n", asset.DownloadURL)
		fmt.Printf("Checksum URL: %s\n", checksumAsset.DownloadURL)
		fmt.Printf("Install path: %s\n", executable)
		fmt.Println()
		fmt.Println("Dry run, nothing was downloaded or installed.")
		return nil
	}

	if !yes {
		fmt.Printf("Install path:    %s\n", executable)
		confirmed, err := prompt.Confirm(fmt.Sprintf("Update %s to %s?", displayedVersion, release.TagName))
		if err != nil {
			return err
		}
		if !confirmed {
			fmt.Println("Canceled.")
			return nil
		}
	}

	archive, err := download(ctx, client, asset)
	if err != nil {
		return err
	}

	fmt.Println("Verifying release SHA256...")
	checksum, err := client.DownloadBytes(ctx, checksumAsset.DownloadURL)
	if err != nil {
		return err
	}
	if err := selfupdate.VerifyChecksum(archive, checksum); err != nil {
		return err
	}

	binary, err := selfupdate.ExtractBinary(asset.Name, archive)
	if err != nil {
		return err
	}

	fmt.Printf("Installing %s to %s...\n", release.TagName, executable)
	if err := selfupdate.Apply(binary, executable); err != nil {
		return err
	}

	fmt.Printf("Updated to %s.\n", release.TagName)
	return nil
}

// download fetches a release asset while rendering a progress bar.
func download(ctx context.Context, client *selfupdate.Client, asset selfupdate.Asset) ([]byte, error) {
	bar := progressbar.NewOptions64(
		asset.Size,
		progressbar.OptionSetDescription("Downloading "+asset.Name),
		progressbar.OptionSetWriter(os.Stderr),
		progressbar.OptionShowBytes(true),
		progressbar.OptionShowCount(),
		progressbar.OptionThrottle(100*time.Millisecond),
		progressbar.OptionOnCompletion(func() { fmt.Fprintln(os.Stderr) }),
	)

	buf := &bytes.Buffer{}
	if err := client.Download(ctx, asset.DownloadURL, io.MultiWriter(buf, bar)); err != nil {
		return nil, err
	}
	if err := bar.Finish(); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}
