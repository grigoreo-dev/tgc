package selfupdate

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/grigoreo-dev/tgc/internal/version"
)

func TestCacheRoundTrip(t *testing.T) {
	t.Setenv("TGC_CONFIG_DIR", t.TempDir())
	if err := WriteCache("v1.5.0"); err != nil {
		t.Fatalf("WriteCache err: %v", err)
	}
	_, latest, ok := readCache()
	if !ok || latest != "v1.5.0" {
		t.Fatalf("readCache = %q, %v; want v1.5.0, true", latest, ok)
	}
}

func TestReadCacheCorrupt(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("TGC_CONFIG_DIR", dir)
	os.WriteFile(cachePath(), []byte("{not json"), 0o600)
	if _, _, ok := readCache(); ok {
		t.Fatalf("readCache on corrupt file: ok=true, want false")
	}
}

func TestStartupNotifyWarns(t *testing.T) {
	t.Setenv("TGC_CONFIG_DIR", t.TempDir())
	old := version.Version
	defer func() { version.Version = old }()
	version.Version = "1.0.0"
	WriteCache("v2.0.0")

	var buf bytes.Buffer
	StartupNotify(&buf)
	if !strings.Contains(buf.String(), `"update_available"`) {
		t.Fatalf("StartupNotify output = %q, want update_available warning", buf.String())
	}
}

func TestStartupNotifyDisabled(t *testing.T) {
	t.Setenv("TGC_CONFIG_DIR", t.TempDir())
	t.Setenv("TGC_NO_UPDATE_CHECK", "1")
	old := version.Version
	defer func() { version.Version = old }()
	version.Version = "1.0.0"
	WriteCache("v2.0.0")

	var buf bytes.Buffer
	StartupNotify(&buf)
	if buf.Len() != 0 {
		t.Fatalf("StartupNotify with TGC_NO_UPDATE_CHECK wrote %q, want nothing", buf.String())
	}
}
