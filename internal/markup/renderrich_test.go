package markup

import (
	"testing"

	"github.com/gotd/td/tg"
)

func TestEscapeMarkdown(t *testing.T) {
	// Inline metacharacters escaped everywhere.
	in := `a*b_c` + "`" + `d[e]f|g\h~i=j`
	got := escapeMarkdown(in)
	want := `a\*b\_c` + "\\`" + `d\[e\]f\|g\\h\~i\=j`
	if got != want {
		t.Fatalf("escapeMarkdown(%q) = %q, want %q", in, got, want)
	}
}

// STRESS-TEST HARDENING (branch 5): escaping must also neutralise LEADING
// block-level Markdown so a bot cannot forge a heading/quote/list/fence.
func TestEscapeMarkdownLeadingBlockMeta(t *testing.T) {
	cases := map[string]string{
		"# heading":   `\# heading`,
		"> quote":     `\> quote`,
		"- bullet":    `\- bullet`,
		"+ bullet":    `\+ bullet`,
		"= setext":    `\= setext`,
		"1. ordered":  `1\. ordered`,
		"normal text": "normal text",
	}
	for in, want := range cases {
		if got := escapeMarkdown(in); got != want {
			t.Fatalf("escapeMarkdown(%q) = %q, want %q", in, got, want)
		}
	}
	// Multi-line: leading meta neutralised per line.
	if got := escapeMarkdown("ok\n# not a heading"); got != "ok\n\\# not a heading" {
		t.Fatalf("multiline leading escape = %q", got)
	}
}

func plain(s string) *tg.TextPlain { return &tg.TextPlain{Text: s} }

func TestRenderRichTextVariants(t *testing.T) {
	c := &richCtx{truncated: new(bool)}
	cases := []struct {
		name string
		in   tg.RichTextClass
		want string
	}{
		{"plain-escaped", plain("a*b"), `a\*b`},
		{"bold", &tg.TextBold{Text: plain("hi")}, `**hi**`},
		{"italic", &tg.TextItalic{Text: plain("hi")}, `_hi_`},
		{"strike", &tg.TextStrike{Text: plain("hi")}, `~~hi~~`},
		{"spoiler", &tg.TextSpoiler{Text: plain("hi")}, `||hi||`},
		{"fixed", &tg.TextFixed{Text: plain("x")}, "`x`"},
		{"marked", &tg.TextMarked{Text: plain("m")}, `==m==`},
		{"url", &tg.TextURL{Text: plain("t"), URL: "https://e.com"}, `[t](https://e.com)`},
		{"email", &tg.TextEmail{Text: plain("t"), Email: "a@b.c"}, `[t](mailto:a@b.c)`},
		{"concat", &tg.TextConcat{Texts: []tg.RichTextClass{plain("a"), &tg.TextBold{Text: plain("b")}}}, `a**b**`},
		{"empty", &tg.TextEmpty{}, ``},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := renderRichText(tc.in, c); got != tc.want {
				t.Fatalf("%s: got %q, want %q", tc.name, got, tc.want)
			}
		})
	}
}

func TestRenderRichTextMentionFallback(t *testing.T) {
	c := &richCtx{truncated: new(bool)}
	got := renderRichText(&tg.TextMentionName{UserID: 42, Text: plain("Bob")}, c)
	if got != "Bob" {
		t.Fatalf("mention w/o resolve = %q, want Bob", got)
	}
	c2 := &richCtx{resolve: map[int64]string{42: "@bob"}, truncated: new(bool)}
	got2 := renderRichText(&tg.TextMentionName{UserID: 42, Text: plain("Bob")}, c2)
	if got2 != "@bob" {
		t.Fatalf("mention w/ resolve = %q, want @bob", got2)
	}
}

func TestRenderRichTextDepthCap(t *testing.T) {
	// Build depth > maxRichDepth nested bolds.
	var t0 tg.RichTextClass = plain("x")
	for i := 0; i < maxRichDepth+5; i++ {
		t0 = &tg.TextBold{Text: t0}
	}
	trunc := new(bool)
	c := &richCtx{truncated: trunc}
	_ = renderRichText(t0, c)
	if !*trunc {
		t.Fatalf("expected truncated=true past depth cap")
	}
}
