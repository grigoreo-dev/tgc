package cli

import (
	"io"
	"os"
	"strconv"

	"github.com/grigoreo-dev/tgc/internal/client"
	"github.com/grigoreo-dev/tgc/internal/ops"
	"github.com/grigoreo-dev/tgc/internal/output"
	"github.com/spf13/cobra"
)

var (
	sendReply      int
	sendPlain      bool
	sendRich       string
	sendFiles      []string
	sendCaption    string
	sendAsDocument bool
	editPlain      bool
	deleteForMe    bool
)

func textArg(args []string, idx int) (string, error) {
	if len(args) > idx && args[idx] != "-" {
		return args[idx], nil
	}
	b, err := io.ReadAll(os.Stdin)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

var sendCmd = &cobra.Command{
	Use:   "send <chat> [text|-]",
	Short: "Send a message (Markdown by default; --file for media)",
	Args:  cobra.RangeArgs(1, 2),
	RunE: func(cmd *cobra.Command, args []string) error {
		conn, err := client.Connect(ProfileName())
		if err != nil {
			return err
		}
		defer conn.Close()

		if len(sendFiles) > 0 {
			results, err := ops.SendFiles(conn, args[0], sendFiles, ops.FileOpts{
				Caption: sendCaption, AsDocument: sendAsDocument, ReplyTo: sendReply, Plain: sendPlain,
			})
			if err != nil {
				return err
			}
			for _, r := range results {
				output.Emit(r)
			}
			return nil
		}

		text, err := textArg(args, 1)
		if err != nil {
			return err
		}
		if text == "" {
			return output.Errf("bad_args", "empty message: pass text, '-' for stdin, or --file")
		}
		res, err := ops.SendText(conn, args[0], text, ops.SendOpts{ReplyTo: sendReply, Plain: sendPlain, RichJSON: sendRich})
		if err != nil {
			return err
		}
		output.Emit(res)
		return nil
	},
}

var editCmd = &cobra.Command{
	Use:   "edit <chat> <message_id> <text>",
	Short: "Edit a message",
	Args:  cobra.ExactArgs(3),
	RunE: func(cmd *cobra.Command, args []string) error {
		id, err := strconv.Atoi(args[1])
		if err != nil {
			return output.Errf("bad_args", "message_id must be a number")
		}
		conn, err := client.Connect(ProfileName())
		if err != nil {
			return err
		}
		defer conn.Close()
		res, err := ops.EditText(conn, args[0], id, args[2], editPlain)
		if err != nil {
			return err
		}
		output.Emit(res)
		return nil
	},
}

var deleteCmd = &cobra.Command{
	Use:   "delete <chat> <message_id>...",
	Short: "Delete messages (for everyone by default; --for-me to keep for others)",
	Args:  cobra.MinimumNArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		var ids []int
		for _, a := range args[1:] {
			id, err := strconv.Atoi(a)
			if err != nil {
				return output.Errf("bad_args", "message_id must be a number, got %q", a)
			}
			ids = append(ids, id)
		}
		conn, err := client.Connect(ProfileName())
		if err != nil {
			return err
		}
		defer conn.Close()
		res, err := ops.Delete(conn, args[0], ids, deleteForMe)
		if err != nil {
			return err
		}
		output.Emit(res)
		return nil
	},
}

var forwardCmd = &cobra.Command{
	Use:   "forward <from_chat> <message_id> <to_chat>",
	Short: "Forward a message",
	Args:  cobra.ExactArgs(3),
	RunE: func(cmd *cobra.Command, args []string) error {
		id, err := strconv.Atoi(args[1])
		if err != nil {
			return output.Errf("bad_args", "message_id must be a number")
		}
		conn, err := client.Connect(ProfileName())
		if err != nil {
			return err
		}
		defer conn.Close()
		res, err := ops.Forward(conn, args[0], id, args[2])
		if err != nil {
			return err
		}
		output.Emit(res)
		return nil
	},
}

func init() {
	sendCmd.Flags().IntVar(&sendReply, "reply", 0, "reply to message id")
	sendCmd.Flags().BoolVar(&sendPlain, "plain", false, "disable Markdown parsing")
	sendCmd.Flags().StringVar(&sendRich, "rich", "", `expert rich_message payload as JSON, e.g. {"type":"markdown","markdown":"..."}`)
	sendCmd.Flags().StringArrayVar(&sendFiles, "file", nil, "file to send (repeat for album, max 10)")
	sendCmd.Flags().StringVar(&sendCaption, "caption", "", "caption for file/album")
	sendCmd.Flags().BoolVar(&sendAsDocument, "as-document", false, "send image as document (default: photo for image/*)")
	editCmd.Flags().BoolVar(&editPlain, "plain", false, "disable Markdown parsing")
	deleteCmd.Flags().BoolVar(&deleteForMe, "for-me", false, "delete only for me")
	rootCmd.AddCommand(sendCmd, editCmd, deleteCmd, forwardCmd)
}
