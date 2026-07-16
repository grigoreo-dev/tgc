package markup

import (
	"encoding/json"
	"testing"

	"github.com/gotd/td/tg"
)

func TestHasBlockContent(t *testing.T) {
	if HasBlockContent("just plain text") {
		t.Fatal("plain text is not block content")
	}
	if !HasBlockContent("# heading\ntext") {
		t.Fatal("heading is block content")
	}
	if !HasBlockContent("| a |\n|---|\n| 1 |") {
		t.Fatal("table is block content")
	}
}

func TestTryRichMarkdown(t *testing.T) {
	rm := TryRichMarkdown("# Hi\n\nbody")
	md, ok := rm.(*tg.InputRichMessageMarkdown)
	if !ok || md.Markdown != "# Hi\n\nbody" {
		t.Fatalf("got %#v", rm)
	}
}

func TestParseRichJSONMarkdown(t *testing.T) {
	raw := `{"type":"markdown","markdown":"# x"}`
	rm, err := ParseRichJSON(json.RawMessage(raw))
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := rm.(*tg.InputRichMessageMarkdown); !ok {
		t.Fatalf("want markdown constructor, got %T", rm)
	}
}
