package selfupdate

import (
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"syscall"
	"time"

	"github.com/grigoreo-dev/tgc/internal/version"
)

const cacheTTL = 24 * time.Hour

// StartupNotify is the non-blocking startup routine. It prints a stderr warning
// if a newer version is cached, and spawns a detached refresher if the cache is
// stale. It performs NO inline network I/O.
func StartupNotify(w io.Writer) {
	if version.IsDev() || os.Getenv("TGC_NO_UPDATE_CHECK") != "" {
		return
	}
	checkedAt, latest, ok := readCache()
	if ok && latest != "" {
		if newer, err := Newer(version.Version, latest); err == nil && newer {
			line, _ := json.Marshal(map[string]any{
				"warning": "update_available",
				"current": version.Version,
				"latest":  latest,
				"message": "run `tgc self update`",
			})
			_, _ = w.Write(append(line, '\n'))
		}
	}
	if !ok || time.Since(checkedAt) > cacheTTL {
		_ = spawnRefresh()
	}
}

// spawnRefresh launches `tgc self check --refresh-cache` fully detached so it
// cannot touch the parent's stdio or leave a zombie. darwin/linux only.
func spawnRefresh() error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	devnull, err := os.OpenFile(os.DevNull, os.O_RDWR, 0)
	if err != nil {
		return err
	}
	defer devnull.Close()
	cmd := exec.Command(exe, "self", "check", "--refresh-cache")
	cmd.Stdin = devnull
	cmd.Stdout = devnull
	cmd.Stderr = devnull
	// Sentinel so the child's OnInitialize skips its own StartupNotify (no recursion).
	cmd.Env = append(os.Environ(), "TGC_UPDATE_REFRESH=1")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Process.Release()
}
