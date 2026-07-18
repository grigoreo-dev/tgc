package cli

import (
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
	"github.com/grigoreo-dev/tgc/internal/config"
	"github.com/grigoreo-dev/tgc/internal/output"
	"github.com/spf13/cobra"
)

var initProfile string

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Create a local ./.tgc config directory in the current directory",
	RunE: func(cmd *cobra.Command, args []string) error {
		name := initProfile
		if name == "" {
			name = "default"
		}
		res, err := runInit(name)
		if err != nil {
			return err
		}
		output.Emit(res)
		return nil
	},
}

// localConfig is the on-disk shape of ./.tgc/config.toml. Kept local to init;
// mirrors config.Config's toml tags.
type localConfig struct {
	DefaultProfile string `toml:"default_profile"`
	APIID          int    `toml:"api_id"`
	APIHash        string `toml:"api_hash"`
}

// runInit creates ./.tgc in CWD (additive, git-init style) and returns the
// result map. It never uses walk-up: init always targets CWD.
func runInit(profile string) (map[string]any, error) {
	wd, err := os.Getwd()
	if err != nil {
		return nil, output.Errf("io_error", "cannot determine working directory: %v", err)
	}
	tgcDir := filepath.Join(wd, ".tgc")
	if err := os.MkdirAll(tgcDir, 0o700); err != nil {
		return nil, output.Errf("io_error", "cannot create %s: %v", tgcDir, err)
	}

	// .gitignore = * (create if missing; do not clobber an existing one)
	giPath := filepath.Join(tgcDir, ".gitignore")
	if _, err := os.Stat(giPath); os.IsNotExist(err) {
		if err := os.WriteFile(giPath, []byte("*\n"), 0o600); err != nil {
			return nil, output.Errf("io_error", "cannot write %s: %v", giPath, err)
		}
	}

	// Load existing local config (if any) for additive merge.
	cfgPath := filepath.Join(tgcDir, "config.toml")
	var lc localConfig
	if b, err := os.ReadFile(cfgPath); err == nil {
		_ = toml.Unmarshal(b, &lc) // best effort; empty on parse failure
	}

	// Fill only empty fields.
	if lc.DefaultProfile == "" {
		lc.DefaultProfile = profile
	}
	inheritedCreds := false
	if lc.APIID == 0 || lc.APIHash == "" {
		id, hash := config.GlobalCredentials()
		if lc.APIID == 0 && id != 0 {
			lc.APIID = id
		}
		if lc.APIHash == "" && hash != "" {
			lc.APIHash = hash
		}
	}
	if lc.APIID != 0 && lc.APIHash != "" {
		inheritedCreds = true
	}

	// Write config.toml atomically (temp + rename in tgcDir).
	tmp, err := os.CreateTemp(tgcDir, "config-*.toml.tmp")
	if err != nil {
		return nil, output.Errf("io_error", "cannot write config: %v", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	_ = tmp.Chmod(0o600)
	if err := toml.NewEncoder(tmp).Encode(&lc); err != nil {
		tmp.Close()
		return nil, output.Errf("io_error", "cannot encode config: %v", err)
	}
	if err := tmp.Close(); err != nil {
		return nil, output.Errf("io_error", "cannot write config: %v", err)
	}
	if err := os.Rename(tmpName, cfgPath); err != nil {
		return nil, output.Errf("io_error", "cannot finalize config: %v", err)
	}

	res := map[string]any{"path": tgcDir, "inherited_creds": inheritedCreds}
	if !inheritedCreds {
		res["next"] = "set TGC_API_ID/TGC_API_HASH or edit .tgc/config.toml, then run: tgc auth login"
	}
	return res, nil
}

func init() {
	initCmd.Flags().StringVar(&initProfile, "profile", "", "default profile name for this local config (default: \"default\")")
	rootCmd.AddCommand(initCmd)
}
