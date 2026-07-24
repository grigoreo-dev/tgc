package cli

import (
	"strconv"

	"github.com/grigoreo-dev/tgc/internal/client"
	"github.com/grigoreo-dev/tgc/internal/ops"
	"github.com/grigoreo-dev/tgc/internal/output"
	"github.com/spf13/cobra"
)

var (
	downloadOut    string
	downloadStdout bool
)

var downloadCmd = &cobra.Command{
	Use:   "download <chat> <message_id>",
	Short: "Download media from a message",
	Args:  cobra.ExactArgs(2),
	RunE: func(_ *cobra.Command, args []string) error {
		id, err := strconv.Atoi(args[1])
		if err != nil {
			return output.Errf("bad_args", "message_id must be a number")
		}
		conn, err := client.Connect(ProfileName())
		if err != nil {
			return err
		}
		defer conn.Close()
		res, err := ops.Download(conn, args[0], id, downloadOut, downloadStdout)
		if err != nil {
			return err
		}
		if !downloadStdout {
			output.Emit(res)
		}
		return nil
	},
}

func init() {
	downloadCmd.Flags().StringVarP(&downloadOut, "out", "o", "", "output file or directory (default: ~/.tgc/downloads/<file_id>/<name>)")
	downloadCmd.Flags().BoolVar(&downloadStdout, "stdout", false, "write raw bytes to stdout")
	rootCmd.AddCommand(downloadCmd)
}
