package cli

import (
	"github.com/grigoreo-dev/tgc/internal/client"
	"github.com/grigoreo-dev/tgc/internal/ops"
	"github.com/grigoreo-dev/tgc/internal/output"
	"github.com/spf13/cobra"
)

var (
	chatsLimit   int
	chatsType    string
	chatsFresh   bool
	membersLimit int
	searchMsgs   bool
	searchLimit  int
)

var chatsCmd = &cobra.Command{
	Use:   "chats",
	Short: "List dialogs (cached 5m; --fresh to refresh)",
	RunE: func(cmd *cobra.Command, args []string) error {
		conn, err := client.Connect(ProfileName())
		if err != nil {
			return err
		}
		defer conn.Close()
		peers, err := ops.Chats(conn, chatsFresh, chatsLimit, chatsType)
		if err != nil {
			return err
		}
		for _, p := range peers {
			output.Emit(p)
		}
		return nil
	},
}

var infoCmd = &cobra.Command{
	Use:   "info <chat>",
	Short: "Show chat/user card",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		conn, err := client.Connect(ProfileName())
		if err != nil {
			return err
		}
		defer conn.Close()
		card, err := ops.Info(conn, args[0])
		if err != nil {
			return err
		}
		output.Emit(card)
		return nil
	},
}

var membersCmd = &cobra.Command{
	Use:   "members <group>",
	Short: "List group members",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		conn, err := client.Connect(ProfileName())
		if err != nil {
			return err
		}
		defer conn.Close()
		items, err := ops.Members(conn, args[0], membersLimit)
		if err != nil {
			return err
		}
		for _, m := range items {
			output.Emit(m)
		}
		return nil
	},
}

var searchCmd = &cobra.Command{
	Use:   "search <query>",
	Short: "Search chats/contacts; --messages for global message search",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		conn, err := client.Connect(ProfileName())
		if err != nil {
			return err
		}
		defer conn.Close()
		if searchMsgs {
			items, err := ops.SearchMessages(conn, args[0], searchLimit)
			if err != nil {
				return err
			}
			for _, m := range items {
				output.Emit(m)
			}
			return nil
		}
		peers, err := ops.SearchChats(conn, args[0], "", searchLimit)
		if err != nil {
			return err
		}
		for _, p := range peers {
			output.Emit(p)
		}
		return nil
	},
}

func init() {
	chatsCmd.Flags().IntVar(&chatsLimit, "limit", 50, "max dialogs")
	chatsCmd.Flags().StringVar(&chatsType, "type", "", "filter: user|group|channel")
	chatsCmd.Flags().BoolVar(&chatsFresh, "fresh", false, "bypass dialog cache")
	membersCmd.Flags().IntVar(&membersLimit, "limit", 200, "max members")
	searchCmd.Flags().BoolVar(&searchMsgs, "messages", false, "global message search")
	searchCmd.Flags().IntVar(&searchLimit, "limit", 20, "max results")
	rootCmd.AddCommand(chatsCmd, infoCmd, membersCmd, searchCmd)
}
