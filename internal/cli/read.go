package cli

import (
	"strconv"

	"github.com/grigoreo-dev/tgc/internal/client"
	"github.com/grigoreo-dev/tgc/internal/ops"
	"github.com/grigoreo-dev/tgc/internal/output"
	"github.com/spf13/cobra"
)

var readOpts ops.ReadOpts
var contextRadius int

var readCmd = &cobra.Command{
	Use:   "read <chat>",
	Short: "Read chat history (newest first)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		conn, err := client.Connect(ProfileName())
		if err != nil {
			return err
		}
		defer conn.Close()
		msgs, err := ops.Read(conn, args[0], readOpts)
		if err != nil {
			return err
		}
		for _, m := range msgs {
			output.Emit(m)
		}
		return nil
	},
}

var contextCmd = &cobra.Command{
	Use:   "context <chat> <message_id>",
	Short: "Show a message with surrounding context",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		msgID, err := strconv.Atoi(args[1])
		if err != nil {
			return output.Errf("bad_args", "message_id must be a number, got %q", args[1])
		}
		conn, err := client.Connect(ProfileName())
		if err != nil {
			return err
		}
		defer conn.Close()
		msgs, err := ops.Context(conn, args[0], msgID, contextRadius)
		if err != nil {
			return err
		}
		for _, m := range msgs {
			output.Emit(m)
		}
		return nil
	},
}

func init() {
	readCmd.Flags().IntVar(&readOpts.Limit, "limit", 20, "max messages")
	readCmd.Flags().IntVar(&readOpts.BeforeID, "before", 0, "messages older than this id")
	readCmd.Flags().IntVar(&readOpts.AfterID, "after", 0, "messages newer than this id")
	readCmd.Flags().StringVar(&readOpts.Since, "since", "", "start date (YYYY-MM-DD or RFC3339)")
	readCmd.Flags().StringVar(&readOpts.Until, "until", "", "end date (YYYY-MM-DD or RFC3339)")
	readCmd.Flags().StringVar(&readOpts.From, "from", "", "only from this sender (selector)")
	readCmd.Flags().StringVar(&readOpts.Search, "search", "", "server-side search within chat")
	contextCmd.Flags().IntVar(&contextRadius, "radius", 10, "messages around the target")
	rootCmd.AddCommand(readCmd, contextCmd)
}
