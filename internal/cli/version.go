package cli

import (
	"github.com/grigoreo-dev/tgc/internal/output"
	"github.com/grigoreo-dev/tgc/internal/version"
	"github.com/spf13/cobra"
)

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the tgc version",
	Args:  cobra.NoArgs,
	RunE: func(_ *cobra.Command, _ []string) error {
		output.Emit(map[string]any{"version": version.Version})
		return nil
	},
}

func init() {
	rootCmd.AddCommand(versionCmd)
}
