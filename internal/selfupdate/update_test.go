package selfupdate

import (
	"context"
	"testing"

	"github.com/grigoreo-dev/tgc/internal/output"
	"github.com/grigoreo-dev/tgc/internal/version"
)

func TestUpdateRefusesDevBuild(t *testing.T) {
	old := version.Version
	defer func() { version.Version = old }()
	version.Version = "dev"

	_, err := Update(context.Background()) // dev short-circuits before network I/O
	if err == nil {
		t.Fatalf("Update on dev build: want error, got nil")
	}
	// internal/output.Error unwraps and exposes a stable Code.
	var oe *output.Error
	if !output.AsError(err, &oe) {
		t.Fatalf("Update dev err is not *output.Error: %v", err)
	}
	if oe.Code != "dev_build" {
		t.Fatalf("Update dev err code = %q, want dev_build", oe.Code)
	}
}
