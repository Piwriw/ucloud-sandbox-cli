package selfupdate

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAssetName(t *testing.T) {
	cases := []struct {
		goos    string
		goarch  string
		want    string
		wantErr bool
	}{
		{goos: "linux", goarch: "amd64", want: "ucloud-sandbox-cli_linux_amd64.tar.gz"},
		{goos: "linux", goarch: "arm64", want: "ucloud-sandbox-cli_linux_arm64.tar.gz"},
		{goos: "darwin", goarch: "arm64", want: "ucloud-sandbox-cli_darwin_arm64.tar.gz"},
		{goos: "windows", goarch: "amd64", want: "ucloud-sandbox-cli_windows_amd64.zip"},
		{goos: "windows", goarch: "arm64", want: "ucloud-sandbox-cli_windows_arm64.zip"},
		{goos: "freebsd", goarch: "amd64", wantErr: true},
		{goos: "linux", goarch: "386", wantErr: true},
	}

	for _, c := range cases {
		t.Run(c.goos+"/"+c.goarch, func(t *testing.T) {
			got, err := AssetName(c.goos, c.goarch)
			if c.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, c.want, got)
		})
	}
}

func TestReleaseAsset(t *testing.T) {
	release := &Release{Assets: []Asset{
		{Name: "ucloud-sandbox-cli_linux_amd64.tar.gz", DownloadURL: "https://example.com/a"},
		{Name: "ucloud-sandbox-cli_linux_amd64.tar.gz.sha256", DownloadURL: "https://example.com/b"},
	}}

	asset, ok := release.Asset("ucloud-sandbox-cli_linux_amd64.tar.gz")
	require.True(t, ok)
	assert.Equal(t, "https://example.com/a", asset.DownloadURL)

	_, ok = release.Asset("ucloud-sandbox-cli_darwin_arm64.tar.gz")
	assert.False(t, ok)
}

func TestLatestRelease(t *testing.T) {
	var gotPath, gotAuth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"tag_name": "v1.3.3",
			"html_url": "https://example.com/release",
			"assets": [
				{"name": "ucloud-sandbox-cli_linux_amd64.tar.gz", "browser_download_url": "https://example.com/a", "size": 42}
			]
		}`))
	}))
	defer server.Close()

	client := &Client{HTTPClient: server.Client(), APIBaseURL: server.URL, Repo: "ucloud/ucloud-sandbox-cli", Token: "secret"}
	release, err := client.LatestRelease(context.Background())
	require.NoError(t, err)

	assert.Equal(t, "/repos/ucloud/ucloud-sandbox-cli/releases/latest", gotPath)
	assert.Equal(t, "Bearer secret", gotAuth)
	assert.Equal(t, "v1.3.3", release.TagName)
	require.Len(t, release.Assets, 1)
	assert.Equal(t, int64(42), release.Assets[0].Size)
}

func TestLatestReleaseErrors(t *testing.T) {
	cases := []struct {
		name    string
		status  int
		body    string
		message string
	}{
		{name: "rate limited", status: http.StatusForbidden, body: `{}`, message: "GITHUB_TOKEN"},
		{name: "not found", status: http.StatusNotFound, body: `{}`, message: "unexpected status"},
		{name: "broken payload", status: http.StatusOK, body: `not json`, message: "parse the latest release"},
		{name: "no tag", status: http.StatusOK, body: `{}`, message: "tag name"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(c.status)
				_, _ = w.Write([]byte(c.body))
			}))
			defer server.Close()

			client := &Client{HTTPClient: server.Client(), APIBaseURL: server.URL, Repo: "ucloud/ucloud-sandbox-cli"}
			_, err := client.LatestRelease(context.Background())
			require.Error(t, err)
			assert.Contains(t, err.Error(), c.message)
		})
	}
}

func TestDownload(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/missing" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_, _ = w.Write([]byte("payload"))
	}))
	defer server.Close()

	client := &Client{HTTPClient: server.Client()}

	buf := &bytes.Buffer{}
	require.NoError(t, client.Download(context.Background(), server.URL+"/asset", buf))
	assert.Equal(t, "payload", buf.String())

	data, err := client.DownloadBytes(context.Background(), server.URL+"/asset")
	require.NoError(t, err)
	assert.Equal(t, []byte("payload"), data)

	err = client.Download(context.Background(), server.URL+"/missing", &bytes.Buffer{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unexpected status")
}
