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

// A Part=true rich message must be flagged rich_truncated even without a fetch,
// so no-fetch paths (await live handler, global search) never silently emit a
// partial rich body.
func TestMessageToMapPartFlagsTruncated(t *testing.T) {
	m := &tg.Message{ID: 1}
	m.SetRichMessage(tg.RichMessage{Part: true, Blocks: []tg.PageBlockClass{
		&tg.PageBlockParagraph{Text: &tg.TextPlain{Text: "partial"}},
	}})
	out := messageToMap(m, nil, nil, 5)
	if out["rich"] != true {
		t.Fatalf("rich flag not set on Part message")
	}
	if out["rich_truncated"] != true {
		t.Fatalf("rich_truncated must be set for a Part=true message, got %v", out["rich_truncated"])
	}
}

func TestRichPartIDs(t *testing.T) {
	partMsg := &tg.Message{ID: 7}
	partMsg.SetRichMessage(tg.RichMessage{Part: true, Blocks: []tg.PageBlockClass{&tg.PageBlockParagraph{Text: &tg.TextPlain{Text: "x"}}}})
	plainMsg := &tg.Message{ID: 8, Message: "hi"}
	res := &tg.MessagesMessages{Messages: []tg.MessageClass{partMsg, plainMsg}}
	got := richPartIDs(res)
	if len(got) != 1 || got[0] != 7 {
		t.Fatalf("richPartIDs = %v, want [7]", got)
	}
}
