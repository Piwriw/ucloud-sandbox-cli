package selfupdate

import (
	"fmt"
	"strings"

	"golang.org/x/mod/semver"
)

// UnpublishedVersion is the placeholder stamped into binaries that are not
// built from a tagged commit.
const UnpublishedVersion = "unpublished"

// Normalize canonicalizes a release tag such as "1.2.3" or "v1.2.3" into the
// "vX.Y.Z" form used by golang.org/x/mod/semver.
func Normalize(version string) (string, error) {
	v := strings.TrimSpace(version)
	if v != "" && !strings.HasPrefix(v, "v") {
		v = "v" + v
	}
	if !semver.IsValid(v) {
		return "", fmt.Errorf("invalid version %q", version)
	}
	return semver.Canonical(v), nil
}

// IsNewer reports whether latest is a strictly newer release than current.
func IsNewer(current, latest string) (bool, error) {
	currentVersion, err := Normalize(current)
	if err != nil {
		return false, err
	}
	latestVersion, err := Normalize(latest)
	if err != nil {
		return false, err
	}
	return semver.Compare(latestVersion, currentVersion) > 0, nil
}
