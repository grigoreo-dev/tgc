package markup

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/gotd/td/bin"
	"github.com/gotd/td/tg"
)

func loadRichFixture(t *testing.T) tg.RichMessage {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", "richmessage_alltypes.bin"))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	var rm tg.RichMessage
	if err := rm.Decode(&bin.Buffer{Buf: raw}); err != nil {
		t.Fatalf("fixture decode failed — gotd version may have changed; "+
			"re-capture with internal/markup/testdata/README.md instructions: %v", err)
	}
	return rm
}

func TestRenderRichMessageGolden(t *testing.T) {
	rm := loadRichFixture(t)
	got, _ := RenderRichMessage(rm, nil)
	goldenPath := filepath.Join("testdata", "richmessage_alltypes.golden.md")
	if os.Getenv("UPDATE_GOLDEN") == "1" {
		if err := os.WriteFile(goldenPath, []byte(got), 0o644); err != nil {
			t.Fatalf("write golden: %v", err)
		}
	}
	want, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("read golden (run with UPDATE_GOLDEN=1 first): %v", err)
	}
	if got != string(want) {
		t.Fatalf("golden mismatch.\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}
