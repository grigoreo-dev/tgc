package ops

import (
	"testing"

	"github.com/gotd/td/tg"
)

func richMsg(text string, blocks ...tg.PageBlockClass) *tg.Message {
	m := &tg.Message{ID: 1, Message: text}
	if len(blocks) > 0 {
		m.SetRichMessage(tg.RichMessage{Blocks: blocks})
	}
	return m
}

func TestMessageToMapNonRichUnchanged(t *testing.T) {
	m := &tg.Message{ID: 1, Message: "hello"}
	out := messageToMap(m, nil, nil, 5)
	if out["text"] != "hello" {
		t.Fatalf("text = %v, want hello", out["text"])
	}
	if _, ok := out["rich"]; ok {
		t.Fatalf("non-rich message must not carry rich key")
	}
}

func TestMessageToMapRichOnly(t *testing.T) {
	m := richMsg("", &tg.PageBlockParagraph{Text: &tg.TextBold{Text: &tg.TextPlain{Text: "hi"}}})
	out := messageToMap(m, nil, nil, 5)
	if out["text"] != "**hi**" {
		t.Fatalf("rich text = %q, want **hi**", out["text"])
	}
	if out["rich"] != true {
		t.Fatalf("rich flag not set")
	}
}

func TestMessageToMapBothConcatenated(t *testing.T) {
	m := richMsg("plain", &tg.PageBlockParagraph{Text: &tg.TextPlain{Text: "rich"}})
	out := messageToMap(m, nil, nil, 5)
	if out["text"] != "plain\n\nrich" {
		t.Fatalf("concat text = %q, want plain\\n\\nrich", out["text"])
	}
	if out["rich"] != true {
		t.Fatalf("rich flag not set on concat")
	}
}
