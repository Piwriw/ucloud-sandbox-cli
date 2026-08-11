package volume

import "github.com/spf13/cobra"

// NewVolumeCmd returns the root volume command group.
func NewVolumeCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "volume",
		Aliases: []string{"vol"},
		Short:   "Manage volumes",
	}
	cmd.AddCommand(newCreateCmd())
	cmd.AddCommand(newDeleteCmd())
	cmd.AddCommand(newListCmd())
	return cmd
}
