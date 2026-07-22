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

// tgc-fl8.3: full re-render must keep ordinary plain prefix semantics of messageToMap.
func TestComposeRichMapText(t *testing.T) {
	if got := composeRichMapText("plain", "rich"); got != "plain\n\nrich" {
		t.Fatalf("both = %q, want plain\\n\\nrich", got)
	}
	if got := composeRichMapText("", "rich only"); got != "rich only" {
		t.Fatalf("rich-only = %q, want %q", got, "rich only")
	}
	if got := composeRichMapText("plain only", ""); got != "plain only" {
		t.Fatalf("plain-only = %q, want %q", got, "plain only")
	}
}

// tgc-fl8.4: fetched users win; original users fill gaps (no regress to raw fallback).
func TestMergeRichResolve(t *testing.T) {
	base := map[int64]string{1: "Alice", 2: "Bob"}
	overlay := map[int64]string{2: "Robert", 3: "Carol"}
	got := mergeRichResolve(base, overlay)
	if got[1] != "Alice" {
		t.Fatalf("kept base user 1 = %q, want Alice", got[1])
	}
	if got[2] != "Robert" {
		t.Fatalf("overlay user 2 = %q, want Robert", got[2])
	}
	if got[3] != "Carol" {
		t.Fatalf("overlay-only user 3 = %q, want Carol", got[3])
	}
	if mergeRichResolve(nil, nil) != nil {
		t.Fatalf("empty merge must return nil")
	}
}

// applyFetchedRichRender is the pure post-fetch seam used by autofetchRichParts.
func TestApplyFetchedRichRenderPrefixAndTruncateFlag(t *testing.T) {
	full := tg.RichMessage{Blocks: []tg.PageBlockClass{
		&tg.PageBlockParagraph{Text: &tg.TextPlain{Text: "full body"}},
	}}
	mp := map[string]any{
		"id":             9,
		"text":           "plain\n\npartial",
		"rich":           true,
		"rich_truncated": true,
	}
	applyFetchedRichRender(mp, "plain", full, nil)
	if mp["text"] != "plain\n\nfull body" {
		t.Fatalf("text = %q, want plain\\n\\nfull body", mp["text"])
	}
	if mp["rich"] != true {
		t.Fatalf("rich flag cleared unexpectedly")
	}
	if _, ok := mp["rich_truncated"]; ok {
		t.Fatalf("successful full render must clear rich_truncated, still %v", mp["rich_truncated"])
	}

	// rich-only original (empty plain) must not invent a prefix.
	mp2 := map[string]any{"text": "partial", "rich": true, "rich_truncated": true}
	applyFetchedRichRender(mp2, "", full, nil)
	if mp2["text"] != "full body" {
		t.Fatalf("rich-only text = %q, want full body", mp2["text"])
	}
}

func TestApplyFetchedRichRenderMentionResolve(t *testing.T) {
	// Full tree mentions user 42; raw node text is "fallback".
	full := tg.RichMessage{Blocks: []tg.PageBlockClass{
		&tg.PageBlockParagraph{Text: &tg.TextMentionName{
			UserID: 42,
			Text:   &tg.TextPlain{Text: "fallback"},
		}},
	}}

	// Fetched response has the user → resolved display name.
	mp := map[string]any{"text": "fallback", "rich": true, "rich_truncated": true}
	applyFetchedRichRender(mp, "", full, map[int64]string{42: "Alice"})
	if mp["text"] != "Alice" {
		t.Fatalf("fetched-user mention = %q, want Alice", mp["text"])
	}

	// Fetch lacks user; original resolve still available → keep original, not raw fallback.
	mp2 := map[string]any{"text": "fallback", "rich": true, "rich_truncated": true}
	resolve := mergeRichResolve(map[int64]string{42: "FromHistory"}, nil)
	applyFetchedRichRender(mp2, "", full, resolve)
	if mp2["text"] != "FromHistory" {
		t.Fatalf("original-user fallback = %q, want FromHistory", mp2["text"])
	}

	// Overlay preferred over base when both present.
	merged := mergeRichResolve(map[int64]string{42: "FromHistory"}, map[int64]string{42: "FromFetch"})
	mp3 := map[string]any{"text": "fallback", "rich": true, "rich_truncated": true}
	applyFetchedRichRender(mp3, "hi", full, merged)
	if mp3["text"] != "hi\n\nFromFetch" {
		t.Fatalf("prefer fetch resolve with prefix = %q, want hi\\n\\nFromFetch", mp3["text"])
	}
}

func TestApplyFetchedRichRenderRetainsTruncatedWhenRendererTruncates(t *testing.T) {
	// Depth past maxRichDepth forces renderer truncated=true.
	var t0 tg.RichTextClass = &tg.TextPlain{Text: "x"}
	for i := 0; i < 70; i++ {
		t0 = &tg.TextBold{Text: t0}
	}
	full := tg.RichMessage{Blocks: []tg.PageBlockClass{&tg.PageBlockParagraph{Text: t0}}}
	mp := map[string]any{"text": "partial", "rich": true, "rich_truncated": true}
	applyFetchedRichRender(mp, "", full, nil)
	if mp["rich_truncated"] != true {
		t.Fatalf("renderer truncation must keep rich_truncated, got %v", mp["rich_truncated"])
	}
	if mp["text"] == "" || mp["text"] == "partial" {
		t.Fatalf("expected re-rendered truncated text, got %q", mp["text"])
	}
}

func TestMessagesResponseUsersAndPlain(t *testing.T) {
	u := &tg.User{ID: 7, FirstName: "Zoe"}
	m := &tg.Message{ID: 3, Message: "caption"}
	res := &tg.MessagesMessages{
		Messages: []tg.MessageClass{m},
		Users:    []tg.UserClass{u},
	}
	if plain := messagePlainByID(res, 3); plain != "caption" {
		t.Fatalf("plain = %q, want caption", plain)
	}
	if plain := messagePlainByID(res, 99); plain != "" {
		t.Fatalf("missing id plain = %q, want empty", plain)
	}
	got := richResolveMap(messagesResponseUsers(res))
	if got[7] != "Zoe" {
		t.Fatalf("response users resolve[7] = %q, want Zoe", got[7])
	}
}
