package cli

import (
	"context"
	"time"

	"github.com/grigoreo-dev/tgc/internal/output"
	"github.com/grigoreo-dev/tgc/internal/selfupdate"
	"github.com/spf13/cobra"
)

var selfRefreshCache bool

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

func init() {
	selfCheckCmd.Flags().BoolVar(&selfRefreshCache, "refresh-cache", false, "internal: refresh the update cache silently")
	_ = selfCheckCmd.Flags().MarkHidden("refresh-cache")
	selfCmd.AddCommand(selfCheckCmd, selfUpdateCmd)
	rootCmd.AddCommand(selfCmd)
}
