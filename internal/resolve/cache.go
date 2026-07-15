package resolve

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

type dialogCacheFile struct {
	SavedAt time.Time `json:"saved_at"`
	Peers   []Peer    `json:"peers"`
}

func cachePath(profileDir string) string { return filepath.Join(profileDir, "dialogs.json") }

func saveDialogCache(profileDir string, peers []Peer) error {
	b, err := json.Marshal(dialogCacheFile{SavedAt: time.Now(), Peers: peers})
	if err != nil {
		return err
	}
	return os.WriteFile(cachePath(profileDir), b, 0o600)
}

// loadDialogCache returns cached peers if the cache is younger than ttlSeconds.
func loadDialogCache(profileDir string, ttlSeconds int) ([]Peer, bool) {
	b, err := os.ReadFile(cachePath(profileDir))
	if err != nil {
		return nil, false
	}
	var f dialogCacheFile
	if json.Unmarshal(b, &f) != nil {
		return nil, false
	}
	if time.Since(f.SavedAt) > time.Duration(ttlSeconds)*time.Second {
		return nil, false
	}
	return f.Peers, true
}
