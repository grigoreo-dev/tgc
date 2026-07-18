package cli

import (
	"github.com/grigoreo-dev/tgc/internal/config"
	"github.com/grigoreo-dev/tgc/internal/output"
	"github.com/spf13/cobra"
)

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Inspect tgc configuration",
}

var configPathCmd = &cobra.Command{
	Use:   "path",
	Short: "Show the active config directory and how it was selected",
	RunE: func(cmd *cobra.Command, args []string) error {
		output.Emit(runConfigPath())
		return nil
	},
}

// runConfigPath reports the active config dir, its source, the selected profile,
// and — when env shadows an existing local .tgc — the shadowed local path.
func runConfigPath() map[string]any {
	dir, source := config.DirSource()
	profile := ProfileName()
	if profile == "" {
		profile = "default"
	}
	res := map[string]any{
		"config_dir": dir,
		"source":     source,
		"profile":    profile,
	}
	if source != "local" {
		if local := config.LocalDir(); local != "" && local != dir {
			res["shadowed_local"] = local
		}
	}
	return res
}

func init() {
	configCmd.AddCommand(configPathCmd)
	rootCmd.AddCommand(configCmd)
}
