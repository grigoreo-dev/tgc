package cli

import (
	"context"
	"os"
	"path/filepath"
	"time"

	"github.com/grigoreo-dev/tgc/internal/output"
	"github.com/grigoreo-dev/tgc/internal/selfupdate"
	"github.com/grigoreo-dev/tgc/internal/setup"
	"github.com/spf13/cobra"
)

var selfRefreshCache bool

var (
	selfSetupShell  string
	selfSetupRemove bool
)

var selfCmd = &cobra.Command{
	Use:   "self",
	Short: "Manage the tgc binary (update, check)",
}

var selfCheckCmd = &cobra.Command{
	Use:   "check",
	Short: "Check whether a newer tgc release is available",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		res, err := selfupdate.Check(ctx)
		if err != nil {
			return err
		}
		if selfRefreshCache {
			_ = selfupdate.WriteCache(res.Latest)
			return nil // silent: background refresher
		}
		output.Emit(map[string]any{
			"current":          res.Current,
			"latest":           res.Latest,
			"update_available": res.UpdateAvailable,
		})
		return nil
	},
}

var selfUpdateCmd = &cobra.Command{
	Use:   "update",
	Short: "Update tgc to the latest release",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		res, err := selfupdate.Update(ctx)
		if err != nil {
			return err
		}
		output.Emit(map[string]any{
			"updated": res.UpdateAvailable,
			"current": res.Current,
			"latest":  res.Latest,
		})
		return nil
	},
}

var selfSetupCmd = &cobra.Command{
	Use:   "setup",
	Short: "Install PATH configuration and shell completion",
	Long: `Install per-user PATH configuration and shell completion for bash, zsh, or fish.

Detects the shell from $SHELL when --shell is omitted. Idempotent: re-running
updates only the managed marker block / marked completion files.

Use --remove to reverse setup (managed block and marked files only).`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runSelfSetup(selfSetupShell, selfSetupRemove)
	},
}

// runSelfSetup wires real process environment into setup.Run / setup.Remove.
func runSelfSetup(shell string, remove bool) error {
	env := realSetupEnv()
	var (
		res *setup.Result
		err error
	)
	if remove {
		res, err = setup.Remove(env, shell)
	} else {
		binDir, berr := executableDir()
		if berr != nil {
			return output.Errf("io_error", "resolve executable path: %v", berr)
		}
		res, err = setup.Run(env, binDir, shell, completionGenerator())
	}
	if err != nil {
		return err
	}
	if len(res.Skipped) > 0 {
		output.Warnf("setup_skipped",
			"left intact unmarked files (not managed by tgc): %v", res.Skipped)
	}
	output.Emit(res)
	return nil
}

// realSetupEnv builds setup.Env from the current process environment.
func realSetupEnv() setup.Env {
	return setup.Env{
		Home:          os.Getenv("HOME"),
		XDGDataHome:   os.Getenv("XDG_DATA_HOME"),
		XDGConfigHome: os.Getenv("XDG_CONFIG_HOME"),
		Path:          os.Getenv("PATH"),
		Shell:         os.Getenv("SHELL"),
	}
}

// executableDir returns the directory of the resolved tgc binary.
func executableDir() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved
	}
	return filepath.Dir(exe), nil
}

func init() {
	selfCheckCmd.Flags().BoolVar(&selfRefreshCache, "refresh-cache", false, "internal: refresh the update cache silently")
	_ = selfCheckCmd.Flags().MarkHidden("refresh-cache")

	selfSetupCmd.Flags().StringVar(&selfSetupShell, "shell", "", "shell to configure (bash|zsh|fish); default: basename of $SHELL")
	selfSetupCmd.Flags().BoolVar(&selfSetupRemove, "remove", false, "remove managed PATH block and marked completion files")

	selfCmd.AddCommand(selfCheckCmd, selfUpdateCmd, selfSetupCmd)
	rootCmd.AddCommand(selfCmd)
}
