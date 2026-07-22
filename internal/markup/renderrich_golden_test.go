package markup

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/grigoreo-dev/tgc/internal/richfixture"
)

func TestRenderRichMessageGolden(t *testing.T) {
	rm, err := richfixture.Decode(filepath.Join("testdata", "richmessage_alltypes.bin"))
	if err != nil {
		t.Fatal(err)
	}
	got, _ := RenderRichMessage(rm, nil)
	goldenPath := filepath.Join("testdata", "richmessage_alltypes.golden.md")
	if os.Getenv("UPDATE_GOLDEN") == "1" {
		if err := os.WriteFile(goldenPath, []byte(got), 0o644); err != nil {
			t.Fatalf("write golden: %v", err)
		}
	}
	want, err := richfixture.LoadGolden(goldenPath)
	if err != nil {
		t.Fatalf("read golden (run with UPDATE_GOLDEN=1 first): %v", err)
	}
	if got != want {
		t.Fatalf("golden mismatch.\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}
