package template

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/ucloud/ucloud-sandbox-cli/internal/config"
	sdk "github.com/ucloud/ucloud-sandbox-sdk-go"
)

func newGetCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "get <template-id>",
		Aliases: []string{"show"},
		Short:   "Show template details with its builds as JSON",
		Args: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return fmt.Errorf("template is required")
			}
			if len(args) > 1 {
				return fmt.Errorf("only one template can be specified")
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			client, err := config.NewClient(cfg)
			if err != nil {
				return err
			}

			tpl, err := client.GetTemplate(cmd.Context(), args[0])
			if err != nil {
				return err
			}

			// The builds of a template are paginated, collect every page so the
			// output holds the complete build list.
			for tpl.NextToken != "" {
				page, err := client.GetTemplate(cmd.Context(), args[0], sdk.WithTemplateBuildsNextToken(tpl.NextToken))
				if err != nil {
					return err
				}
				tpl.Builds = append(tpl.Builds, page.Builds...)
				tpl.NextToken = page.NextToken
			}

			encoder := json.NewEncoder(os.Stdout)
			encoder.SetIndent("", "  ")
			return encoder.Encode(tpl)
		},
	}
}
