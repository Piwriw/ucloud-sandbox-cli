package selfupdate

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

const (
	// BinaryName is the name of the binary inside the release archives.
	BinaryName = "ucloud-sandbox-cli"

	defaultAPIBaseURL = "https://api.github.com"
	defaultRepo       = "ucloud/ucloud-sandbox-cli"

	apiTimeout      = 30 * time.Second
	downloadTimeout = 10 * time.Minute

	// maxResponseSize caps how much is read from a release response so a
	// broken or hostile server cannot exhaust memory.
	maxResponseSize = 256 << 20
)

// Asset is a single file attached to a GitHub release.
type Asset struct {
	Name        string `json:"name"`
	DownloadURL string `json:"browser_download_url"`
	Size        int64  `json:"size"`
}

// Release is the subset of the GitHub release payload the updater needs.
type Release struct {
	TagName string  `json:"tag_name"`
	HTMLURL string  `json:"html_url"`
	Assets  []Asset `json:"assets"`
}

// Asset returns the release asset with the given name.
func (r *Release) Asset(name string) (Asset, bool) {
	for _, asset := range r.Assets {
		if asset.Name == name {
			return asset, true
		}
	}
	return Asset{}, false
}

// Client talks to the GitHub releases API.
type Client struct {
	HTTPClient *http.Client
	APIBaseURL string
	Repo       string
	// Token authenticates API calls. It only raises the rate limit, the
	// releases themselves are public.
	Token string
}

// NewClient creates a client pointing at the public repository.
func NewClient() *Client {
	return &Client{
		HTTPClient: &http.Client{},
		APIBaseURL: defaultAPIBaseURL,
		Repo:       defaultRepo,
		Token:      os.Getenv("GITHUB_TOKEN"),
	}
}

// AssetName returns the release asset holding the binary for the given
// platform, matching the naming used by install.sh and install.ps1.
func AssetName(goos, goarch string) (string, error) {
	switch goarch {
	case "amd64", "arm64":
	default:
		return "", fmt.Errorf("unsupported architecture %q, supported architectures are: amd64, arm64", goarch)
	}

	switch goos {
	case "linux", "darwin":
		return fmt.Sprintf("%s_%s_%s.tar.gz", BinaryName, goos, goarch), nil
	case "windows":
		return fmt.Sprintf("%s_%s_%s.zip", BinaryName, goos, goarch), nil
	default:
		return "", fmt.Errorf("unsupported operating system %q, supported operating systems are: linux, darwin, windows", goos)
	}
}

// LatestRelease fetches the latest published release of the repository.
func (c *Client) LatestRelease(ctx context.Context) (*Release, error) {
	ctx, cancel := context.WithTimeout(ctx, apiTimeout)
	defer cancel()

	url := fmt.Sprintf("%s/repos/%s/releases/latest", c.APIBaseURL, c.Repo)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", BinaryName)
	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}

	resp, err := c.httpClient().Do(req)
	if err != nil {
		return nil, fmt.Errorf("query the latest release: %w", err)
	}
	defer resp.Body.Close()

	switch {
	case resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusTooManyRequests:
		return nil, fmt.Errorf("query the latest release: GitHub rejected the request (%s), the API rate limit is likely exhausted, retry later or set GITHUB_TOKEN", resp.Status)
	case resp.StatusCode != http.StatusOK:
		return nil, fmt.Errorf("query the latest release: unexpected status %s", resp.Status)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseSize))
	if err != nil {
		return nil, fmt.Errorf("read the latest release: %w", err)
	}

	release := &Release{}
	if err := json.Unmarshal(body, release); err != nil {
		return nil, fmt.Errorf("parse the latest release: %w", err)
	}
	if release.TagName == "" {
		return nil, fmt.Errorf("the latest release does not carry a tag name")
	}
	return release, nil
}

// Download streams a release asset into dst.
func (c *Client) Download(ctx context.Context, url string, dst io.Writer) error {
	ctx, cancel := context.WithTimeout(ctx, downloadTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", BinaryName)

	resp, err := c.httpClient().Do(req)
	if err != nil {
		return fmt.Errorf("download %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download %s: unexpected status %s", url, resp.Status)
	}

	if _, err := io.Copy(dst, io.LimitReader(resp.Body, maxResponseSize)); err != nil {
		return fmt.Errorf("download %s: %w", url, err)
	}
	return nil
}

// DownloadBytes fetches a small release asset, such as a checksum file, into
// memory.
func (c *Client) DownloadBytes(ctx context.Context, url string) ([]byte, error) {
	buf := &bytes.Buffer{}
	if err := c.Download(ctx, url, buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func (c *Client) httpClient() *http.Client {
	if c.HTTPClient != nil {
		return c.HTTPClient
	}
	return http.DefaultClient
}
