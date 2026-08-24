package selfupdate

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"path"
	"strings"
)

// maxBinarySize caps how much is extracted from a release archive.
const maxBinarySize = 256 << 20

// VerifyChecksum checks data against the content of a released .sha256 file,
// which holds a single lowercase hex digest.
func VerifyChecksum(data []byte, checksumFile []byte) error {
	expected := strings.ToLower(strings.TrimSpace(string(checksumFile)))
	// The release workflow writes the bare digest, but tolerate the
	// "<digest>  <file>" form that sha256sum prints by default.
	if fields := strings.Fields(expected); len(fields) > 0 {
		expected = fields[0]
	}

	decoded, err := hex.DecodeString(expected)
	if err != nil || len(decoded) != sha256.Size {
		return errors.New("release checksum must contain exactly 64 hexadecimal characters")
	}

	actual := sha256.Sum256(data)
	if !bytes.Equal(actual[:], decoded) {
		return errors.New("SHA256 verification failed for the downloaded release asset")
	}
	return nil
}

// ExtractBinary reads the CLI binary out of a release archive. The archive
// format is picked from the asset name.
func ExtractBinary(assetName string, archive []byte) ([]byte, error) {
	switch {
	case strings.HasSuffix(assetName, ".tar.gz"):
		return extractTarGz(archive)
	case strings.HasSuffix(assetName, ".zip"):
		return extractZip(archive)
	default:
		return nil, fmt.Errorf("unsupported archive format: %s", assetName)
	}
}

// isBinaryEntry reports whether an archive entry is the CLI binary, ignoring
// any directory prefix the archive may carry.
func isBinaryEntry(name string) bool {
	base := path.Base(strings.ReplaceAll(name, `\`, "/"))
	return base == BinaryName || base == BinaryName+".exe"
}

func extractTarGz(archive []byte) ([]byte, error) {
	gz, err := gzip.NewReader(bytes.NewReader(archive))
	if err != nil {
		return nil, fmt.Errorf("read the release archive: %w", err)
	}
	defer gz.Close()

	reader := tar.NewReader(gz)
	for {
		header, err := reader.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("read the release archive: %w", err)
		}
		if header.Typeflag != tar.TypeReg || !isBinaryEntry(header.Name) {
			continue
		}

		binary, err := io.ReadAll(io.LimitReader(reader, maxBinarySize))
		if err != nil {
			return nil, fmt.Errorf("read the release archive: %w", err)
		}
		return binary, nil
	}

	return nil, fmt.Errorf("the release archive does not contain %s", BinaryName)
}

func extractZip(archive []byte) ([]byte, error) {
	reader, err := zip.NewReader(bytes.NewReader(archive), int64(len(archive)))
	if err != nil {
		return nil, fmt.Errorf("read the release archive: %w", err)
	}

	for _, file := range reader.File {
		if file.FileInfo().IsDir() || !isBinaryEntry(file.Name) {
			continue
		}

		entry, err := file.Open()
		if err != nil {
			return nil, fmt.Errorf("read the release archive: %w", err)
		}
		binary, err := io.ReadAll(io.LimitReader(entry, maxBinarySize))
		entry.Close()
		if err != nil {
			return nil, fmt.Errorf("read the release archive: %w", err)
		}
		return binary, nil
	}

	return nil, fmt.Errorf("the release archive does not contain %s", BinaryName)
}
