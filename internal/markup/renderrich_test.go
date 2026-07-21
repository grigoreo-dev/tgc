package markup

import (
	"strings"
	"testing"
	"unicode/utf8"

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

func h1(s string) tg.PageBlockClass { return &tg.PageBlockHeading1{Text: plain(s)} }

func TestRenderPageBlocks(t *testing.T) {
	c := &richCtx{truncated: new(bool)}
	cases := []struct {
		name string
		in   tg.PageBlockClass
		want string
	}{
		{"h1", &tg.PageBlockTitle{Text: plain("T")}, "# T"},
		{"heading2", &tg.PageBlockHeading2{Text: plain("H")}, "## H"},
		{"subheader", &tg.PageBlockSubheader{Text: plain("S")}, "**S**"},
		{"paragraph", &tg.PageBlockParagraph{Text: plain("body")}, "body"},
		{"divider", &tg.PageBlockDivider{}, "---"},
		{"preformatted", &tg.PageBlockPreformatted{Text: plain("echo hi"), Language: "sh"}, "```sh\necho hi\n```"},
		{"blockquote", &tg.PageBlockBlockquote{Text: plain("q")}, "> q"},
		{"math", &tg.PageBlockMath{Source: "E=mc^2"}, "$$\nE=mc^2\n$$"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := renderPageBlock(tc.in, c); got != tc.want {
				t.Fatalf("%s: got %q, want %q", tc.name, got, tc.want)
			}
		})
	}
}

func TestRenderPageBlockUnknownFallback(t *testing.T) {
	c := &richCtx{truncated: new(bool)}
	got := renderPageBlock(&tg.PageBlockUnsupported{}, c)
	if got == "" || !strings.Contains(got, "block:") {
		t.Fatalf("unknown block should degrade to placeholder, got %q", got)
	}
}

func TestRenderRichMessageJoinAndSize(t *testing.T) {
	rm := tg.RichMessage{Blocks: []tg.PageBlockClass{h1("A"), &tg.PageBlockParagraph{Text: plain("b")}}}
	md, trunc := RenderRichMessage(rm, nil)
	if md != "# A\n\nb" {
		t.Fatalf("join: got %q", md)
	}
	if trunc {
		t.Fatalf("small message should not be truncated")
	}
}

func TestRenderRichMessageSizeCap(t *testing.T) {
	big := strings.Repeat("x", maxRichBytes+100)
	rm := tg.RichMessage{Blocks: []tg.PageBlockClass{&tg.PageBlockParagraph{Text: plain(big)}}}
	md, trunc := RenderRichMessage(rm, nil)
	if !trunc || len(md) > maxRichBytes+8 {
		t.Fatalf("size cap not enforced: trunc=%v len=%d", trunc, len(md))
	}
}

// A multibyte rune straddling maxRichBytes must not be split: raw md[:maxRichBytes]
// would leave an incomplete UTF-8 sequence.
func TestRenderRichMessageSizeCapUTF8Boundary(t *testing.T) {
	// "世" is 3 UTF-8 bytes; place it so the first byte sits at maxRichBytes-1.
	const multi = "世"
	if utf8.RuneLen([]rune(multi)[0]) != 3 {
		t.Fatal("test fixture: expected 3-byte rune")
	}
	// Body length after render equals input for plain paragraph text.
	// maxRichBytes-1 ASCII bytes + 3-byte rune + more => crosses cap mid-rune.
	body := strings.Repeat("x", maxRichBytes-1) + multi + strings.Repeat("y", 50)
	rm := tg.RichMessage{Blocks: []tg.PageBlockClass{&tg.PageBlockParagraph{Text: plain(body)}}}
	md, trunc := RenderRichMessage(rm, nil)
	if !trunc {
		t.Fatalf("expected truncated=true when multibyte content exceeds cap")
	}
	if !utf8.ValidString(md) {
		t.Fatalf("truncated Markdown is not valid UTF-8 (len=%d)", len(md))
	}
	if !strings.HasSuffix(md, "\n[…]") {
		t.Fatalf("expected […] marker, got suffix %q", md[len(md)-min(20, len(md)):])
	}
	prefix := strings.TrimSuffix(md, "\n[…]")
	if len(prefix) > maxRichBytes {
		t.Fatalf("capped prefix exceeds maxRichBytes: %d > %d", len(prefix), maxRichBytes)
	}
	// Cap must fall at a rune boundary: incomplete leading bytes of multi must be dropped.
	if strings.Contains(prefix, multi) {
		t.Fatalf("prefix must not include the straddling rune when cut would exceed cap: %q", prefix[len(prefix)-8:])
	}
	if len(prefix) != maxRichBytes-1 {
		t.Fatalf("want prefix of %d ASCII bytes (cap before multibyte rune), got %d", maxRichBytes-1, len(prefix))
	}
	if !utf8.ValidString(prefix) {
		t.Fatalf("capped prefix is not valid UTF-8")
	}
}

func TestRenderPageBlockListCheckbox(t *testing.T) {
	c := &richCtx{truncated: new(bool)}
	block := &tg.PageBlockList{
		Items: []tg.PageListItemClass{
			&tg.PageListItemText{Text: plain("Done"), Checkbox: true, Checked: true},
			&tg.PageListItemText{Text: plain("Todo"), Checkbox: true, Checked: false},
			&tg.PageListItemText{Text: plain("plain")},
		},
	}
	got := renderPageBlock(block, c)
	want := "- [x] Done\n- [ ] Todo\n- plain"
	if got != want {
		t.Fatalf("checkbox list:\ngot  %q\nwant %q", got, want)
	}
}

func TestRenderPageBlockDetailsKeepsTitle(t *testing.T) {
	c := &richCtx{truncated: new(bool)}
	block := &tg.PageBlockDetails{
		Title: plain("More"),
		Blocks: []tg.PageBlockClass{
			&tg.PageBlockParagraph{Text: plain("nested body")},
		},
	}
	got := renderPageBlock(block, c)
	if !strings.Contains(got, "**More**") {
		t.Fatalf("Details Title dropped; got %q", got)
	}
	if !strings.Contains(got, "nested body") {
		t.Fatalf("Details nested block text missing; got %q", got)
	}
}

func TestRenderPageBlockDepthCap(t *testing.T) {
	// Nest PageBlockDetails deeper than maxRichDepth.
	var b tg.PageBlockClass = &tg.PageBlockParagraph{Text: plain("leaf")}
	for i := 0; i < maxRichDepth+5; i++ {
		b = &tg.PageBlockDetails{
			Title:  plain("L"),
			Blocks: []tg.PageBlockClass{b},
		}
	}
	trunc := new(bool)
	c := &richCtx{truncated: trunc}
	got := renderPageBlock(b, c)
	if !*trunc {
		t.Fatalf("expected truncated=true past block depth cap")
	}
	if !strings.Contains(got, "[…]") {
		t.Fatalf("expected […] marker on depth cap, got %q", got)
	}
}
