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
	RunE: func(_ *cobra.Command, _ []string) error {
		output.Emit(runConfigPath())
		return nil
	},
}

// runConfigPath reports the active config dir, its source, the effective
// profile, and — when env shadows an existing local .tgc — the shadowed local
// path.
func runConfigPath() map[string]any {
	dir, source := config.DirSource()
	profile := effectiveProfile()
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

// effectiveProfile resolves the profile the way ResolveProfile does, but
// read-only (no directory creation): explicit --profile/TGC_PROFILE, then the
// active config's default_profile, then "default". This mirrors the real
// resolution so `tgc config path` reports the profile that commands will use.
func effectiveProfile() string {
	if p := ProfileName(); p != "" {
		return p
	}
	if c, err := config.Load(); err == nil && c.DefaultProfile != "" {
		return c.DefaultProfile
	}
	return "default"
}

func init() {
	configCmd.AddCommand(configPathCmd)
	rootCmd.AddCommand(configCmd)
}
