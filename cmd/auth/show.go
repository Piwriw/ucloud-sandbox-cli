package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/spf13/cobra"
	"github.com/ucloud/ucloud-sandbox-cli/cmd"
	"github.com/ucloud/ucloud-sandbox-cli/internal/config"
	"github.com/ucloud/ucloud-sandbox-cli/internal/whoami"
	"github.com/ucloud/ucloud-sandbox-sdk-go/pkg/errdefs"
)

// maskedValue stands in for a secret, so the config can be shown without
// putting the secret itself on the screen.
const maskedValue = "****"

// apiKeyVisibleChars is how many characters of the API key are kept at each
// end, enough to tell keys apart without giving one away.
const apiKeyVisibleChars = 3

// minPartialMaskLen is the shortest API key shown partially; a shorter one
// would have too little left hidden, so it is masked entirely.
const minPartialMaskLen = 10

type showOperation struct{}

func (o *showOperation) Command() *cobra.Command {
	return &cobra.Command{
		Use:   "show",
		Short: "Show the configuration and the current identity",
		Args:  cobra.NoArgs,
	}
}

// showOutput is what show prints. WhoamiError explains a missing Whoami, so
// the config is still shown when the identity cannot be looked up.
type showOutput struct {
	Config      *config.Config   `json:"config"`
	Whoami      *whoami.Response `json:"whoami"`
	WhoamiError string           `json:"whoami_error,omitempty"`
}

func (o *showOperation) Run(ctx cmd.OperationContext) error {
	// The resolved config is shown, environment overrides included, because
	// that is what the other commands actually run with.
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	// Without a key there is nothing to look up, and the config alone does not
	// say what to do next.
	if cfg.APIKey == "" {
		return fmt.Errorf("not logged in; run '%s auth login' first, or set %s",
			ctx.Cmd.Root().Name(), config.EnvAPIKey)
	}

	out := showOutput{Config: maskConfig(cfg)}

	who, err := lookupIdentity(ctx, cfg)
	if err != nil {
		out.WhoamiError = err.Error()
	} else {
		out.Whoami = who
	}

	return writeJSON(ctx.Cmd.OutOrStdout(), &out)
}

// errInvalidAPIKey replaces the server's terse rejection of an API key with
// what the user can do about it.
var errInvalidAPIKey = errors.New("the API key is invalid or has expired; " +
	"check it and try again (API keys are managed at " + apiKeysURL + ")")

const apiKeysURL = "https://astraflow.ucloud.cn/modelverse/api-keys"

// lookupIdentity asks the control plane who cfg's API key belongs to.
func lookupIdentity(ctx context.Context, cfg *config.Config) (*whoami.Response, error) {
	client, err := config.NewClient(cfg)
	if err != nil {
		return nil, err
	}

	who, err := whoami.Get(ctx, client)
	if authErr := (*errdefs.AuthenticationError)(nil); errors.As(err, &authErr) {
		return nil, errInvalidAPIKey
	}

	return who, err
}

func writeJSON(w io.Writer, v any) error {
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(v); err != nil {
		return fmt.Errorf("print config: %w", err)
	}

	return nil
}

// maskConfig returns a copy of cfg with its secrets masked.
func maskConfig(cfg *config.Config) *config.Config {
	masked := *cfg
	masked.APIKey = maskAPIKey(masked.APIKey)

	// The copy above is shallow, so the registry map is still the caller's:
	// masking in place would blank the passwords they are holding.
	if cfg.Registries != nil {
		registries := make(map[string]config.RegistryAuth, len(cfg.Registries))
		for domain, auth := range cfg.Registries {
			auth.Password = maskSecret(auth.Password)
			registries[domain] = auth
		}
		masked.Registries = registries
	}

	return &masked
}

// maskAPIKey keeps a few characters at each end of the API key, so the user
// can tell which key is in use.
func maskAPIKey(key string) string {
	if len(key) < minPartialMaskLen {
		return maskSecret(key)
	}

	return key[:apiKeyVisibleChars] + maskedValue + key[len(key)-apiKeyVisibleChars:]
}

// maskSecret masks a sensitive config value, leaving an unset one unset so the
// output still shows what is missing.
func maskSecret(secret string) string {
	if secret == "" {
		return ""
	}

	return maskedValue
}
