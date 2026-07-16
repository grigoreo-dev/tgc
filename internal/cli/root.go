package cli

import (
	"os"

	"github.com/grigoreo-dev/tgc/internal/output"
	"github.com/grigoreo-dev/tgc/internal/selfupdate"
	"github.com/grigoreo-dev/tgc/internal/version"
	"github.com/spf13/cobra"
)

var (
	flagProfile string
	flagPretty  bool
)

var rootCmd = &cobra.Command{
	Use:           "tgc",
	Short:         "Agent-first Telegram CLI",
	Long:          "tgc is a Telegram client for terminals and AI agents.\nDefault output is compact JSONL; use --pretty for humans.",
	SilenceUsage:  true,
	SilenceErrors: true,
}

// ProfileName returns the selected profile: --profile flag, TGC_PROFILE env, or "".
func ProfileName() string {
	if flagProfile != "" {
		return flagProfile
	}
	return os.Getenv("TGC_PROFILE")
}

// Pretty reports whether human-readable output was requested.
func Pretty() bool { return flagPretty }

func Execute() {
	rootCmd.PersistentFlags().StringVar(&flagProfile, "profile", "", "profile name (default from config or TGC_PROFILE)")
	rootCmd.PersistentFlags().BoolVar(&flagPretty, "pretty", false, "human-readable output")
	rootCmd.Version = version.Version
	rootCmd.SetVersionTemplate(`{"version":"{{.Version}}"}` + "\n")
	cobra.OnInitialize(func() {
		output.SetPretty(flagPretty)
		// Skip the notify inside the background refresher to avoid recursion.
		if os.Getenv("TGC_UPDATE_REFRESH") == "" {
			selfupdate.StartupNotify(os.Stderr)
		}
	})
	if err := rootCmd.Execute(); err != nil {
		output.FailErr(err)
	}
}
