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
)

var searchOpts ops.SearchOpts

var chatsCmd = &cobra.Command{
	Use:   "chats",
	Short: "List dialogs (cached 5m; --fresh to refresh)",
	RunE: func(_ *cobra.Command, _ []string) error {
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
	RunE: func(_ *cobra.Command, args []string) error {
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
	RunE: func(_ *cobra.Command, args []string) error {
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
	Short: "Search chats and messages; --type to narrow, --chat to search inside one chat",
	Args:  cobra.ExactArgs(1),
	RunE: func(_ *cobra.Command, args []string) error {
		if err := ops.ValidateSearchOpts(searchOpts); err != nil {
			return err // bad_args before connecting
		}
		conn, err := client.Connect(ProfileName())
		if err != nil {
			return err
		}
		defer conn.Close()
		if conn.Profile.Type == "bot" {
			return output.Errf("bot_unsupported",
				"search is not available for bot accounts")
		}
		rows, err := ops.Search(conn, args[0], searchOpts)
		if err != nil {
			return err
		}
		for _, r := range rows {
			output.Emit(r)
		}
		return nil
	},
}

func init() {
	chatsCmd.Flags().IntVar(&chatsLimit, "limit", 50, "max dialogs")
	chatsCmd.Flags().StringVar(&chatsType, "type", "", "filter: user|group|channel")
	chatsCmd.Flags().BoolVar(&chatsFresh, "fresh", false, "bypass dialog cache")
	membersCmd.Flags().IntVar(&membersLimit, "limit", 200, "max members")
	searchCmd.Flags().StringVar(&searchOpts.Type, "type", "", "narrow results: chats|messages|user|group|channel")
	searchCmd.Flags().StringVar(&searchOpts.Chat, "chat", "", "search inside one chat (peer selector)")
	searchCmd.Flags().StringVar(&searchOpts.From, "from", "", "only from this sender (requires --chat)")
	searchCmd.Flags().StringVar(&searchOpts.Since, "since", "", "start date (YYYY-MM-DD or RFC3339)")
	searchCmd.Flags().StringVar(&searchOpts.Until, "until", "", "end date (YYYY-MM-DD or RFC3339)")
	searchCmd.Flags().IntVar(&searchOpts.Limit, "limit", 20, "max results per section")
	rootCmd.AddCommand(chatsCmd, infoCmd, membersCmd, searchCmd)
}
