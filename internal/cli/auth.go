package cli

import (
	"os"
	"path/filepath"

	"github.com/grigoreo-dev/tgc/internal/client"
	"github.com/grigoreo-dev/tgc/internal/config"
	"github.com/grigoreo-dev/tgc/internal/output"
	"github.com/spf13/cobra"
)

var authCmd = &cobra.Command{Use: "auth", Short: "Manage Telegram sessions"}

var (
	loginPhone    string
	loginBotToken string
	loginAPIID    int
	loginAPIHash  string
	exportOut     string
)

var authLoginCmd = &cobra.Command{
	Use:   "login",
	Short: "Log in interactively (user) or with --bot-token (bot)",
	RunE: func(_ *cobra.Command, _ []string) error {
		if loginAPIID != 0 || loginAPIHash != "" {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			if loginAPIID != 0 {
				cfg.APIID = loginAPIID
			}
			if loginAPIHash != "" {
				cfg.APIHash = loginAPIHash
			}
			if err := config.Save(cfg); err != nil {
				return err
			}
		}
		conn, err := client.ConnectForLogin(ProfileName(), loginPhone, loginBotToken)
		if err != nil {
			return err
		}
		defer conn.Close()
		self := conn.Client.Self
		ptype := "user"
		if loginBotToken != "" {
			ptype = "bot"
		}
		userID := int64(0)
		username := ""
		if self != nil {
			userID = self.ID
			username = selfUsername(self)
		}
		output.Emit(map[string]any{
			"status": "ok", "profile": conn.Profile.Name, "type": ptype,
			"user_id": userID, "username": username,
		})
		return nil
	},
}

var authExportCmd = &cobra.Command{
	Use:   "export",
	Short: "Export session as a portable string",
	RunE: func(_ *cobra.Command, _ []string) error {
		conn, err := client.Connect(ProfileName())
		if err != nil {
			return err
		}
		defer conn.Close()
		s, err := conn.Client.ExportStringSession()
		if err != nil {
			return err
		}
		if exportOut != "" {
			if err := os.WriteFile(exportOut, []byte(s), 0o600); err != nil {
				return err
			}
			output.Emit(map[string]any{"status": "ok", "path": exportOut})
			return nil
		}
		output.Emit(map[string]any{"session": s})
		return nil
	},
}

var authImportCmd = &cobra.Command{
	Use:   "import [file]",
	Short: "Import a session string (arg=file, stdin, or TGC_SESSION env)",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(_ *cobra.Command, args []string) error {
		s, err := readSessionInput(args)
		if err != nil {
			return err
		}
		p, err := config.ResolveProfile(ProfileName())
		if err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(p.Dir, "session.txt"), []byte(s), 0o600); err != nil {
			return err
		}
		conn, err := client.Connect(p.Name)
		if err != nil {
			_ = os.Remove(filepath.Join(p.Dir, "session.txt"))
			return err
		}
		defer conn.Close()
		ptype := "user"
		if conn.Client.Self != nil && conn.Client.Self.Bot {
			ptype = "bot"
		}
		if err := config.SetProfileType(p, ptype); err != nil {
			return err
		}
		output.Emit(map[string]any{"status": "ok", "profile": p.Name, "type": ptype})
		return nil
	},
}

var authListCmd = &cobra.Command{
	Use:   "list",
	Short: "List profiles",
	RunE: func(_ *cobra.Command, _ []string) error {
		cfg, err := config.Load()
		if err != nil {
			return err
		}
		def := cfg.DefaultProfile
		if def == "" {
			def = "default"
		}
		profiles, err := config.ListProfiles()
		if err != nil {
			return err
		}
		for _, p := range profiles {
			output.Emit(map[string]any{"name": p.Name, "type": p.Type, "default": p.Name == def})
		}
		return nil
	},
}

var authLogoutCmd = &cobra.Command{
	Use:   "logout [profile]",
	Short: "Delete a profile and its session",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(_ *cobra.Command, args []string) error {
		name := ProfileName()
		if len(args) == 1 {
			name = args[0]
		}
		if name == "" {
			name = "default"
		}
		if err := config.DeleteProfile(name); err != nil {
			return err
		}
		output.Emit(map[string]any{"status": "ok", "profile": name})
		return nil
	},
}

func init() {
	authLoginCmd.Flags().StringVar(&loginPhone, "phone", "", "phone number (international format)")
	authLoginCmd.Flags().StringVar(&loginBotToken, "bot-token", "", "bot token from @BotFather")
	authLoginCmd.Flags().IntVar(&loginAPIID, "api-id", 0, "Telegram api_id (saved to config)")
	authLoginCmd.Flags().StringVar(&loginAPIHash, "api-hash", "", "Telegram api_hash (saved to config)")
	authExportCmd.Flags().StringVarP(&exportOut, "out", "o", "", "write session to file instead of stdout")
	authCmd.AddCommand(authLoginCmd, authExportCmd, authImportCmd, authListCmd, authLogoutCmd)
	rootCmd.AddCommand(authCmd)
}
