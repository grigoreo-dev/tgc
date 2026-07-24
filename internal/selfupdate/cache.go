package selfupdate

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"github.com/grigoreo-dev/tgc/internal/config"
)

type cacheFile struct {
	CheckedAt time.Time `json:"checked_at"`
	Latest    string    `json:"latest"`
}

// cachePath is the machine-global update-check cache. It must NOT use
// config.Dir(): that prefers a local ./.tgc and would fragment the 24h
// dedup across projects (tgc-1zm).
func cachePath() string { return filepath.Join(config.GlobalDir(), "update-check.json") }

// WriteCache atomically records the latest known release tag.
func WriteCache(latest string) error {
	dir := config.GlobalDir()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	b, err := json.Marshal(cacheFile{CheckedAt: time.Now(), Latest: latest})
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".update-check-")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(b); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
		return err
	}
	_ = tmp.Close()
	return os.Rename(tmpName, cachePath())
}

func readCache() (time.Time, string, bool) {
	b, err := os.ReadFile(cachePath())
	if err != nil {
		return time.Time{}, "", false
	}
	var f cacheFile
	if json.Unmarshal(b, &f) != nil {
		return time.Time{}, "", false
	}
	return f.CheckedAt, f.Latest, true
}
