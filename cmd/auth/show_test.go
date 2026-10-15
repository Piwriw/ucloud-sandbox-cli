package auth

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/ucloud/ucloud-sandbox-cli/cmd"
	"github.com/ucloud/ucloud-sandbox-cli/internal/config"
)

func TestMaskAPIKey(t *testing.T) {
	assert.Equal(t, "", maskAPIKey(""))
	assert.Equal(t, maskedValue, maskAPIKey("short"))
	assert.Equal(t, maskedValue, maskAPIKey("123456789"))
	assert.Equal(t, "123"+maskedValue+"890", maskAPIKey("1234567890"))
	assert.Equal(t, "umi"+maskedValue+"q76", maskAPIKey("uminferapikey-1ixrmnntoq76"))
}

func TestMaskConfig(t *testing.T) {
	cfg := &config.Config{
		APIKey:       "secret-api-key",
		Region:       "cn-sh",
		Domain:       "cn-sh.sandbox.ucloudai.com",
		InsecureHTTP: true,
		Registries: map[string]config.RegistryAuth{
			"docker.io": {Username: "builder", Password: "secret-password"},
			"ghcr.io":   {Username: "other", Password: "other-password"},
		},
	}

	masked := maskConfig(cfg)

	assert.Equal(t, "sec"+maskedValue+"key", masked.APIKey)

	// Everything that is not a secret is shown as it is.
	assert.Equal(t, "cn-sh", masked.Region)
	assert.Equal(t, "cn-sh.sandbox.ucloudai.com", masked.Domain)
	assert.True(t, masked.InsecureHTTP)

	// Every registry's password is masked, not just the first one.
	require.Len(t, masked.Registries, 2)
	for domain, auth := range masked.Registries {
		assert.Equal(t, maskedValue, auth.Password, "registry %s", domain)
		assert.NotEqual(t, maskedValue, auth.Username, "registry %s", domain)
	}

	// The caller's config, map included, is left alone.
	assert.Equal(t, "secret-api-key", cfg.APIKey)
	assert.Equal(t, "secret-password", cfg.Registries["docker.io"].Password)
}

func TestMaskConfigKeepsUnsetSecretsUnset(t *testing.T) {
	masked := maskConfig(&config.Config{})

	assert.Equal(t, "", masked.APIKey)
	assert.Nil(t, masked.Registries)
}

// runShow runs show with HOME isolated and returns its parsed output.
func runShow(t *testing.T) map[string]any {
	t.Helper()

	op := &showOperation{}
	c := op.Command()
	var out bytes.Buffer
	c.SetOut(&out)

	require.NoError(t, op.Run(cmd.OperationContext{Context: context.Background(), Cmd: c}))

	var shown map[string]any
	require.NoError(t, json.Unmarshal(out.Bytes(), &shown))
	return shown
}

func TestShowAppliesEnvironmentOverrides(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	require.NoError(t, config.Save(&config.Config{APIKey: "file-api-key-value", Region: "cn-sh"}))

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// The key the environment set is the one used.
		assert.Equal(t, "env-api-key-value", r.Header.Get("X-API-Key"))
		_, _ = w.Write([]byte(`{"company": {"id": 1, "name": "Acme"}, "project": {"id": 2, "name": "Proj"}}`))
	}))
	t.Cleanup(srv.Close)

	t.Setenv("UCLOUD_SANDBOX_API_URL", srv.URL)
	t.Setenv("UCLOUD_SANDBOX_API_KEY", "env-api-key-value")
	t.Setenv("UCLOUD_SANDBOX_REGION", "us-ca")

	shown := runShow(t)

	cfg := shown["config"].(map[string]any)
	assert.Equal(t, "env"+maskedValue+"lue", cfg["api_key"])
	assert.Equal(t, "us-ca", cfg["region"])

	who := shown["whoami"].(map[string]any)
	assert.Equal(t, "Acme", who["company"].(map[string]any)["name"])
	assert.Equal(t, "Proj", who["project"].(map[string]any)["name"])
	assert.NotContains(t, shown, "whoami_error")
}

func TestShowExplainsARejectedAPIKey(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"message": "invalid api key"}`))
	}))
	t.Cleanup(srv.Close)

	t.Setenv("UCLOUD_SANDBOX_API_URL", srv.URL)
	t.Setenv("UCLOUD_SANDBOX_API_KEY", "bad-api-key-value")

	shown := runShow(t)

	assert.Nil(t, shown["whoami"])
	assert.Equal(t, errInvalidAPIKey.Error(), shown["whoami_error"])
}

func TestShowAsksToLogInWithoutAnAPIKey(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("UCLOUD_SANDBOX_API_KEY", "")

	op := &showOperation{}
	c := op.Command()
	var out bytes.Buffer
	c.SetOut(&out)

	err := op.Run(cmd.OperationContext{Context: context.Background(), Cmd: c})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not logged in")
	assert.Contains(t, err.Error(), "auth login")
	assert.Contains(t, err.Error(), config.EnvAPIKey)
	assert.Empty(t, out.String())
}
