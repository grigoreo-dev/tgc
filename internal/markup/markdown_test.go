package markup

import (
	"testing"

	"github.com/gotd/td/tg"
)

func TestParseBold(t *testing.T) {
	text, ents, err := Parse("hello **world**")
	if err != nil {
		t.Fatal(err)
	}
	if text != "hello world" {
		t.Fatalf("text: %q", text)
	}
	if len(ents) != 1 {
		t.Fatalf("want 1 entity, got %d", len(ents))
	}
	b, ok := ents[0].(*tg.MessageEntityBold)
	if !ok {
		t.Fatalf("want Bold, got %T", ents[0])
	}
	if b.Offset != 6 || b.Length != 5 {
		t.Fatalf("offset/length: %d/%d", b.Offset, b.Length)
	}
}

func TestParseInlineCodeAndLink(t *testing.T) {
	text, ents, err := Parse("run `ls` or [docs](https://example.com)")
	if err != nil {
		t.Fatal(err)
	}
	if text != "run ls or docs" {
		t.Fatalf("text: %q", text)
	}
	var haveCode, haveURL bool
	for _, e := range ents {
		switch e.(type) {
		case *tg.MessageEntityCode:
			haveCode = true
		case *tg.MessageEntityTextURL:
			haveURL = true
		}
	}
	if !haveCode || !haveURL {
		t.Fatalf("code=%v url=%v ents=%+v", haveCode, haveURL, ents)
	}
}

func TestParseCodeBlockWithLang(t *testing.T) {
	text, ents, err := Parse("```go\nfmt.Println(1)\n```")
	if err != nil {
		t.Fatal(err)
	}
	if text != "fmt.Println(1)" {
		t.Fatalf("text: %q", text)
	}
	pre, ok := ents[0].(*tg.MessageEntityPre)
	if !ok || pre.Language != "go" {
		t.Fatalf("want pre(go), got %+v", ents[0])
	}
}

func TestHeadingDegradesToBoldLine(t *testing.T) {
	text, ents, err := Parse("# Title\nbody")
	if err != nil {
		t.Fatal(err)
	}
	if text != "Title\nbody" {
		t.Fatalf("text: %q", text)
	}
	b, ok := ents[0].(*tg.MessageEntityBold)
	if !ok || b.Offset != 0 || b.Length != 5 {
		t.Fatalf("heading must become bold line: %+v", ents[0])
	}
}

func TestListBullets(t *testing.T) {
	text, _, err := Parse("- one\n- two")
	if err != nil {
		t.Fatal(err)
	}
	if text != "• one\n• two" {
		t.Fatalf("text: %q", text)
	}
}

func TestUTF16OffsetsWithEmoji(t *testing.T) {
	text, ents, err := Parse("🔥 **hot**")
	if err != nil {
		t.Fatal(err)
	}
	if text != "🔥 hot" {
		t.Fatalf("text: %q", text)
	}
	b := ents[0].(*tg.MessageEntityBold)
	if b.Offset != 3 || b.Length != 3 {
		t.Fatalf("UTF-16 offsets wrong: %d/%d", b.Offset, b.Length)
	}
}

func TestPlainNoParsing(t *testing.T) {
	text, ents, err := ParsePlain("**not bold**")
	if err != nil || text != "**not bold**" || len(ents) != 0 {
		t.Fatalf("plain must not parse: %q %v %v", text, ents, err)
	}
}
