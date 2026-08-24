package selfupdate

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestVerifyChecksum(t *testing.T) {
	data := []byte("release payload")
	sum := sha256.Sum256(data)
	digest := hex.EncodeToString(sum[:])

	cases := []struct {
		name     string
		checksum string
		wantErr  string
	}{
		{name: "bare digest", checksum: digest + "\n"},
		{name: "uppercase digest", checksum: "  " + digest + "  "},
		{name: "sha256sum output", checksum: digest + "  ucloud-sandbox-cli_linux_amd64.tar.gz\n"},
		{name: "mismatch", checksum: hex.EncodeToString(make([]byte, sha256.Size)), wantErr: "SHA256 verification failed"},
		{name: "too short", checksum: "abc123", wantErr: "64 hexadecimal characters"},
		{name: "not hex", checksum: "zz" + digest[2:], wantErr: "64 hexadecimal characters"},
		{name: "empty", checksum: "", wantErr: "64 hexadecimal characters"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := VerifyChecksum(data, []byte(c.checksum))
			if c.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), c.wantErr)
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestExtractBinaryTarGz(t *testing.T) {
	archive := makeTarGz(t, map[string][]byte{
		"README.md":  []byte("docs"),
		BinaryName:   []byte("binary content"),
		"extra.json": []byte("{}"),
	})

	binary, err := ExtractBinary("ucloud-sandbox-cli_linux_amd64.tar.gz", archive)
	require.NoError(t, err)
	assert.Equal(t, []byte("binary content"), binary)
}

func TestExtractBinaryTarGzWithDirectoryPrefix(t *testing.T) {
	archive := makeTarGz(t, map[string][]byte{"dist/" + BinaryName: []byte("binary content")})

	binary, err := ExtractBinary("ucloud-sandbox-cli_darwin_arm64.tar.gz", archive)
	require.NoError(t, err)
	assert.Equal(t, []byte("binary content"), binary)
}

func TestExtractBinaryZip(t *testing.T) {
	archive := makeZip(t, map[string][]byte{
		"LICENSE":           []byte("license"),
		BinaryName + ".exe": []byte("windows binary"),
	})

	binary, err := ExtractBinary("ucloud-sandbox-cli_windows_amd64.zip", archive)
	require.NoError(t, err)
	assert.Equal(t, []byte("windows binary"), binary)
}

func TestExtractBinaryErrors(t *testing.T) {
	t.Run("unsupported format", func(t *testing.T) {
		_, err := ExtractBinary("ucloud-sandbox-cli_linux_amd64.tar.xz", []byte("data"))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unsupported archive format")
	})

	t.Run("binary missing from tar.gz", func(t *testing.T) {
		archive := makeTarGz(t, map[string][]byte{"README.md": []byte("docs")})
		_, err := ExtractBinary("ucloud-sandbox-cli_linux_amd64.tar.gz", archive)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "does not contain")
	})

	t.Run("binary missing from zip", func(t *testing.T) {
		archive := makeZip(t, map[string][]byte{"README.md": []byte("docs")})
		_, err := ExtractBinary("ucloud-sandbox-cli_windows_amd64.zip", archive)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "does not contain")
	})

	t.Run("corrupted tar.gz", func(t *testing.T) {
		_, err := ExtractBinary("ucloud-sandbox-cli_linux_amd64.tar.gz", []byte("not an archive"))
		require.Error(t, err)
	})

	t.Run("corrupted zip", func(t *testing.T) {
		_, err := ExtractBinary("ucloud-sandbox-cli_windows_amd64.zip", []byte("not an archive"))
		require.Error(t, err)
	})
}

func makeTarGz(t *testing.T, files map[string][]byte) []byte {
	t.Helper()

	buf := &bytes.Buffer{}
	gz := gzip.NewWriter(buf)
	writer := tar.NewWriter(gz)

	for name, content := range files {
		require.NoError(t, writer.WriteHeader(&tar.Header{
			Name:     name,
			Mode:     0o755,
			Size:     int64(len(content)),
			Typeflag: tar.TypeReg,
		}))
		_, err := writer.Write(content)
		require.NoError(t, err)
	}

	require.NoError(t, writer.Close())
	require.NoError(t, gz.Close())
	return buf.Bytes()
}

func makeZip(t *testing.T, files map[string][]byte) []byte {
	t.Helper()

	buf := &bytes.Buffer{}
	writer := zip.NewWriter(buf)

	for name, content := range files {
		entry, err := writer.Create(name)
		require.NoError(t, err)
		_, err = entry.Write(content)
		require.NoError(t, err)
	}

	require.NoError(t, writer.Close())
	return buf.Bytes()
}
