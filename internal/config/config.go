// Package config manages tgc configuration and named profiles.
package config

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/grigoreo-dev/tgc/internal/output"
)

type Config struct {
	DefaultProfile string `toml:"default_profile"`
	APIID          int    `toml:"api_id"`
	APIHash        string `toml:"api_hash"`
}

type Profile struct {
	Name        string
	Dir         string
	SessionPath string
	Type        string // "user", "bot", or "" (not logged in yet)
}

// Dir returns the tgc config root, honoring TGC_CONFIG_DIR and XDG_CONFIG_HOME.
func Dir() string {
	if d := os.Getenv("TGC_CONFIG_DIR"); d != "" {
		return d
	}
	if x := os.Getenv("XDG_CONFIG_HOME"); x != "" {
		return filepath.Join(x, "tgc")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "tgc")
}

func configPath() string { return filepath.Join(Dir(), "config.toml") }

func Load() (*Config, error) {
	var c Config
	b, err := os.ReadFile(configPath())
	if err != nil {
		if os.IsNotExist(err) {
			return &c, nil
		}
		return nil, err
	}
	if err := toml.Unmarshal(b, &c); err != nil {
		return nil, output.Errf("bad_config", "cannot parse %s: %v", configPath(), err)
	}
	return &c, nil
}

func Save(c *Config) error {
	if err := os.MkdirAll(Dir(), 0o700); err != nil {
		return err
	}
	f, err := os.OpenFile(configPath(), os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	return toml.NewEncoder(f).Encode(c)
}

// ResolveProfile picks a profile: explicit name, then config default_profile,
// then "default". It creates the profile directory.
func ResolveProfile(explicit string) (*Profile, error) {
	name := explicit
	if name == "" {
		c, err := Load()
		if err != nil {
			return nil, err
		}
		name = c.DefaultProfile
	}
	if name == "" {
		name = "default"
	}
	dir := filepath.Join(Dir(), "profiles", name)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	p := &Profile{Name: name, Dir: dir, SessionPath: filepath.Join(dir, "session.db")}
	if b, err := os.ReadFile(filepath.Join(dir, "type")); err == nil {
		p.Type = strings.TrimSpace(string(b))
	}
	return p, nil
}

func SetProfileType(p *Profile, t string) error {
	return os.WriteFile(filepath.Join(p.Dir, "type"), []byte(t), 0o600)
}

func ListProfiles() ([]Profile, error) {
	root := filepath.Join(Dir(), "profiles")
	entries, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out []Profile
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		p, err := ResolveProfile(e.Name())
		if err != nil {
			return nil, err
		}
		out = append(out, *p)
	}
	return out, nil
}

func DeleteProfile(name string) error {
	if name == "" {
		return output.Errf("bad_args", "profile name required")
	}
	return os.RemoveAll(filepath.Join(Dir(), "profiles", name))
}

// APICredentials returns api_id/api_hash: env first, then config.
func APICredentials(c *Config) (int, string, error) {
	id, hash := c.APIID, c.APIHash
	if v := os.Getenv("TGC_API_ID"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			return 0, "", output.Errf("bad_config", "TGC_API_ID is not a number: %q", v)
		}
		id = n
	}
	if v := os.Getenv("TGC_API_HASH"); v != "" {
		hash = v
	}
	if id == 0 || hash == "" {
		return 0, "", output.Errf("no_api_credentials",
			"api_id/api_hash not set; get them at https://my.telegram.org and set TGC_API_ID/TGC_API_HASH or run `tgc auth login`")
	}
	return id, hash, nil
}

// homeDir returns the user's home directory, or "" if it cannot be determined
// (e.g. HOME unset in a container). Callers treat "" as "no home boundary".
func homeDir() string {
	h, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return h
}

// findLocalDir walks up from the current working directory looking for a
// directory that contains a ".tgc" subdirectory. The nearest match wins. The
// climb stops after inspecting $HOME (when known) or the filesystem root. It is
// NOT bounded by git-root: a shared workspace/.tgc must be reachable from a
// subproject that has its own .git. Returns "" when no local .tgc exists.
func findLocalDir() string {
	wd, err := os.Getwd()
	if err != nil {
		return ""
	}
	return findLocalDirFrom(wd)
}

func findLocalDirFrom(start string) string {
	home := homeDir()
	dir := start
	for {
		candidate := filepath.Join(dir, ".tgc")
		if fi, err := os.Stat(candidate); err == nil && fi.IsDir() {
			return candidate
		}
		// stat error (missing/permission) => keep climbing, never fatal.
		if home != "" && dir == home {
			return "" // inspected $HOME inclusively; stop.
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "" // reached FS root.
		}
		dir = parent
	}
}
