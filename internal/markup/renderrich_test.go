package markup

import (
	"strings"
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

// Security: sender-controlled URL/email/phone destinations and math sources are
// untrusted. Delimiters in those fields must not break out of renderer-generated
// Markdown structure. Formatting is defined only by the rich tree.
func TestEscapeLinkDest(t *testing.T) {
	// Destinations only need structural safety inside Markdown (...) destinations:
	// escape '\', '(', ')', and percent-encode whitespace/control so they cannot
	// terminate the destination or inject new lines. Other characters (including
	// '*') remain literal inside the destination and do not break structure.
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"ordinary-https", "https://e.com/path?q=1", "https://e.com/path?q=1"},
		{"ordinary-path", "/relative/path", "/relative/path"},
		{"close-paren-breakout", "https://evil.com) **bold**", `https://evil.com\)%20**bold**`},
		{"close-paren-simple", "https://evil.com)", `https://evil.com\)`},
		{"space-breakout", "https://a.com b", "https://a.com%20b"},
		{"newline-breakout", "https://a.com\n# forged", "https://a.com%0A#%20forged"},
		{"backslash", `https://a.com\x`, `https://a.com\\x`},
		{"open-paren", "https://a.com/foo(bar)", `https://a.com/foo\(bar\)`},
		{"crlf", "https://a.com\r\n]", "https://a.com%0D%0A]"},
		{"tab", "https://a.com\tx", "https://a.com%09x"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := escapeLinkDest(tc.in); got != tc.want {
				t.Fatalf("escapeLinkDest(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestEscapeMathSource(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"ordinary", "E=mc^2", "E=mc^2"},
		{"frac", `\frac{a}{b}`, `\frac{a}{b}`},
		{"dollar-breakout", `x$ **bold**`, `x\$ **bold**`},
		{"double-dollar-fence", "a\n$$\n# forged", "a\n\\$\\$\n# forged"},
		{"inline-newlines-kept-for-block", "a\nb", "a\nb"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := escapeMathSource(tc.in); got != tc.want {
				t.Fatalf("escapeMathSource(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestRenderUntrustedTargetsNoBreakout(t *testing.T) {
	c := &richCtx{truncated: new(bool)}
	cases := []struct {
		name string
		in   tg.RichTextClass
		// mustContain: structural fragments that must appear
		// mustNotContain: breakout artifacts that must not appear as raw MD
		want string
	}{
		{
			name: "url-ordinary",
			in:   &tg.TextURL{Text: plain("t"), URL: "https://e.com"},
			want: `[t](https://e.com)`,
		},
		{
			name: "url-paren-breakout",
			in:   &tg.TextURL{Text: plain("click"), URL: "https://evil.com)[pwn](http://x"},
			want: `[click](https://evil.com\)[pwn]\(http://x)`,
		},
		{
			name: "url-whitespace-breakout",
			in:   &tg.TextURL{Text: plain("t"), URL: "https://a.com) **inj**"},
			want: `[t](https://a.com\)%20**inj**)`,
		},
		{
			name: "url-newline-breakout",
			in:   &tg.TextURL{Text: plain("t"), URL: "https://a.com\n[x](http://evil)"},
			want: `[t](https://a.com%0A[x]\(http://evil\))`,
		},
		{
			name: "email-ordinary",
			in:   &tg.TextEmail{Text: plain("t"), Email: "a@b.c"},
			want: `[t](mailto:a@b.c)`,
		},
		{
			name: "email-breakout",
			in:   &tg.TextEmail{Text: plain("t"), Email: "a@b.c) **x**"},
			want: `[t](mailto:a@b.c\)%20**x**)`,
		},
		{
			name: "phone-ordinary",
			in:   &tg.TextPhone{Text: plain("t"), Phone: "+15551212"},
			want: `[t](tel:+15551212)`,
		},
		{
			name: "phone-breakout",
			in:   &tg.TextPhone{Text: plain("t"), Phone: "+1) **x**"},
			want: `[t](tel:+1\)%20**x**)`,
		},
		{
			name: "math-ordinary",
			in:   &tg.TextMath{Source: "E=mc^2"},
			want: `$E=mc^2$`,
		},
		{
			name: "math-dollar-breakout",
			in:   &tg.TextMath{Source: "x$ **bold**"},
			want: `$x\$ **bold**$`,
		},
		{
			name: "math-newline-inline",
			in:   &tg.TextMath{Source: "a\nb"},
			// inline math collapses newlines so the $...$ span stays one line
			want: `$a b$`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := renderRichText(tc.in, c); got != tc.want {
				t.Fatalf("%s: got %q, want %q", tc.name, got, tc.want)
			}
		})
	}
}

func TestRenderUntrustedMathBlockNoBreakout(t *testing.T) {
	c := &richCtx{truncated: new(bool)}
	cases := []struct {
		name string
		src  string
		want string
	}{
		{"ordinary", "E=mc^2", "$$\nE=mc^2\n$$"},
		{"fence-breakout", "x\n$$\n**forged**", "$$\nx\n\\$\\$\n**forged**\n$$"},
		{"dollar-inline", "a$b", "$$\na\\$b\n$$"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := renderPageBlock(&tg.PageBlockMath{Source: tc.src}, c)
			if got != tc.want {
				t.Fatalf("%s: got %q, want %q", tc.name, got, tc.want)
			}
		})
	}
}

// Integration: a full paragraph containing a malicious TextURL must not let the
// destination inject a second link or bold formatting outside the link dest.
func TestRenderRichMessageLinkDestInjectionInert(t *testing.T) {
	rm := tg.RichMessage{
		Blocks: []tg.PageBlockClass{
			&tg.PageBlockParagraph{
				Text: &tg.TextURL{
					Text: plain("click"),
					URL:  "https://evil.com) **injected** [x](http://pwn",
				},
			},
		},
	}
	md, _ := RenderRichMessage(rm, nil)
	// Destination must keep the payload inside a single (...) pair; the early
	// ')' must be escaped so "**injected**" is not free Markdown after the link.
	if strings.Contains(md, "](https://evil.com) **injected**") {
		t.Fatalf("link dest breakout: malicious URL closed the destination early:\n%s", md)
	}
	if !strings.Contains(md, `https://evil.com\)`) {
		t.Fatalf("expected escaped ')' in destination, got %q", md)
	}
	// Visible label still rendered (and escaped if needed).
	if !strings.HasPrefix(md, "[click](") {
		t.Fatalf("expected link label structure, got %q", md)
	}
}
