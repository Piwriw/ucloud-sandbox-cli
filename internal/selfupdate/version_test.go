package selfupdate

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNormalize(t *testing.T) {
	cases := []struct {
		input   string
		want    string
		wantErr bool
	}{
		{input: "v1.2.3", want: "v1.2.3"},
		{input: "1.2.3", want: "v1.2.3"},
		{input: " v1.2.3 ", want: "v1.2.3"},
		{input: "v1.3", want: "v1.3.0"},
		{input: "v1.2.3-rc.1", want: "v1.2.3-rc.1"},
		{input: UnpublishedVersion, wantErr: true},
		{input: "", wantErr: true},
		{input: "v1.2.x", wantErr: true},
	}

	for _, c := range cases {
		t.Run(c.input, func(t *testing.T) {
			got, err := Normalize(c.input)
			if c.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, c.want, got)
		})
	}
}

func TestIsNewer(t *testing.T) {
	cases := []struct {
		name    string
		current string
		latest  string
		want    bool
		wantErr bool
	}{
		{name: "patch bump", current: "v1.3.2", latest: "v1.3.3", want: true},
		{name: "minor bump", current: "v1.3.3", latest: "v1.4.0", want: true},
		{name: "major bump", current: "v1.9.9", latest: "v2.0.0", want: true},
		{name: "same version", current: "v1.3.3", latest: "v1.3.3", want: false},
		{name: "older release", current: "v1.3.3", latest: "v1.3.2", want: false},
		{name: "missing v prefix", current: "1.3.2", latest: "1.3.3", want: true},
		{name: "release beats pre-release", current: "v1.3.3-rc.1", latest: "v1.3.3", want: true},
		{name: "pre-release loses to release", current: "v1.3.3", latest: "v1.3.3-rc.1", want: false},
		{name: "unpublished current", current: UnpublishedVersion, latest: "v1.3.3", wantErr: true},
		{name: "invalid latest", current: "v1.3.3", latest: "nightly", wantErr: true},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := IsNewer(c.current, c.latest)
			if c.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, c.want, got)
		})
	}
}
