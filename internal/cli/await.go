package cli

import (
	"time"

	"github.com/grigoreo-dev/tgc/internal/client"
	"github.com/grigoreo-dev/tgc/internal/ops"
	"github.com/grigoreo-dev/tgc/internal/output"
	"github.com/spf13/cobra"
)

var (
	awaitTimeout  int
	awaitDebounce int
	awaitFrom     string
)

// awaitOnConn waits for the chat's unread batch on an ALREADY-CONNECTED watch
// connection, emits it, and marks it read. Shared by `tgc await` (which opens
// its own conn) and `send --await-reply` (which reuses the send connection so
// the dispatcher is already live when the message is sent — no gap).
func awaitOnConn(conn *client.Conn, selector string, timeout, debounce int, from string) error {
	msgs, lastID, chatID, timedOut, err := ops.Await(conn, selector, ops.AwaitOpts{
		Timeout:  time.Duration(timeout) * time.Second,
		Debounce: time.Duration(debounce) * time.Second,
		From:     from,
	})
	if err != nil {
		return err
	}
	if timedOut {
		output.Emit(map[string]any{"status": "timeout", "chat_id": chatID, "waited": timeout})
		return nil
	}
	for _, m := range msgs {
		output.Emit(m)
	}
	if err := ops.MarkRead(conn, selector, lastID); err != nil {
		output.Warnf("mark_read_failed", "%v", err) // stderr, non-fatal
	}
	return nil
}

var awaitCmd = &cobra.Command{
	Use:   "await <chat>",
	Short: "Wait for the chat's incoming messages, print them, mark read, exit",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		conn, err := client.ConnectWatch(ProfileName())
		if err != nil {
			return err
		}
		defer conn.Close()
		return awaitOnConn(conn, args[0], awaitTimeout, awaitDebounce, awaitFrom)
	},
}

func init() {
	awaitCmd.Flags().IntVar(&awaitTimeout, "timeout", 300, "max seconds to wait")
	awaitCmd.Flags().IntVar(&awaitDebounce, "debounce", 2, "seconds of quiet before returning a batch (0=off)")
	awaitCmd.Flags().StringVar(&awaitFrom, "from", "", "only messages from this sender (selector)")
	rootCmd.AddCommand(awaitCmd)
}
