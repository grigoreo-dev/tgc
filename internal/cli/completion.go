package cli

import (
	"io"

	"github.com/grigoreo-dev/tgc/internal/output"
	"github.com/grigoreo-dev/tgc/internal/setup"
	"github.com/spf13/cobra"
)

var completionCmd = &cobra.Command{
	Use:   "completion [bash|zsh|fish]",
	Short: "Generate shell completion script to stdout",
	Long: `Generate a completion script for bash, zsh, or fish and write it to stdout.

This only generates the script; it does not install it. To install PATH
configuration and completion files, use:

  tgc self setup

Examples:
  tgc completion bash
  tgc completion zsh > ~/.local/share/zsh/site-functions/_tgc
  tgc completion fish`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return writeCompletionScript(args[0], cmd.OutOrStdout())
	},
}

func writeCompletionScript(shell string, w io.Writer) error {
	switch shell {
	case "bash":
		return rootCmd.GenBashCompletionV2(w, true)
	case "zsh":
		return rootCmd.GenZshCompletion(w)
	case "fish":
		return rootCmd.GenFishCompletion(w, true)
	default:
		return output.Errf("bad_args",
			"unsupported shell %q; supported: bash, zsh, fish", shell)
	}
}

// completionGenerator returns a setup.Generator backed by Cobra completion
// generation on rootCmd. Exported for reuse by self update (Task 4).
func completionGenerator() setup.Generator {
	return func(shell string, w io.Writer) error {
		return writeCompletionScript(shell, w)
	}
}

func init() {
	// Disable Cobra's default hidden completion command so our explicit
	// generator is the single public surface.
	rootCmd.CompletionOptions.DisableDefaultCmd = true
	rootCmd.AddCommand(completionCmd)
}
