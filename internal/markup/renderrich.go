package markup

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/gotd/td/tg"
)

// Caps guarding against maliciously deep / large rich trees.
const (
	maxRichDepth = 64
	maxRichBytes = 64 * 1024
)

// richCtx carries per-render state through the recursive walk.
type richCtx struct {
	resolve   map[int64]string // optional user_id -> display name
	depth     int
	truncated *bool // set true if any cap was hit
}

// inlineMDEscaper escapes Markdown metacharacters that are significant anywhere
// in a line.
var inlineMDEscaper = strings.NewReplacer(
	`\`, `\\`,
	"`", "\\`",
	`*`, `\*`,
	`_`, `\_`,
	`[`, `\[`,
	`]`, `\]`,
	`|`, `\|`,
	`~`, `\~`,
	`=`, `\=`,
)

// escapeMarkdown escapes Markdown metacharacters in UNTRUSTED plain text so a
// sender cannot forge formatting/links/blocks. Formatting is applied only by the
// tree structure, never by text content. It escapes inline metacharacters
// everywhere AND neutralises leading block-level markers (#, >, -, +, =, and an
// ordered-list "N.") per line, because those only trigger at line start.
func escapeMarkdown(s string) string {
	lines := strings.Split(s, "\n")
	for i, ln := range lines {
		esc := inlineMDEscaper.Replace(ln)
		esc = escapeLeadingBlockMeta(esc)
		lines[i] = esc
	}
	return strings.Join(lines, "\n")
}

// escapeLeadingBlockMeta backslash-escapes a leading block-level Markdown marker
// on a single (already inline-escaped) line.
func escapeLeadingBlockMeta(ln string) string {
	trimmed := strings.TrimLeft(ln, " ")
	lead := ln[:len(ln)-len(trimmed)]
	if len(trimmed) > 0 {
		switch trimmed[0] {
		case '#', '>', '-', '+', '=':
			return lead + `\` + trimmed
		}
		// N. ordered list: one-or-more digits then '.'
		i := 0
		for i < len(trimmed) && trimmed[i] >= '0' && trimmed[i] <= '9' {
			i++
		}
		if i > 0 && i < len(trimmed) && trimmed[i] == '.' {
			return lead + trimmed[:i] + `\.` + trimmed[i+1:]
		}
	}
	return ln
}

// escapeLinkDest sanitizes a sender-controlled URL/email/phone value for use
// inside a Markdown link destination: [label](DEST). Parentheses and backslashes
// are escaped so they cannot close or restructure the destination. Literal '%'
// is encoded as %25 so sender percent-sequences cannot be confused with our
// own percent-encoding of whitespace/controls. Unicode White_Space (incl.
// U+00A0, U+3000), line/paragraph separators (U+2028/U+2029), and control
// characters (incl. DEL) are percent-encoded as UTF-8 bytes so downstream
// Markdown consumers cannot treat them as destination-ending whitespace or
// line breaks. Other characters (including '*') are preserved.
func escapeLinkDest(s string) string {
	var b strings.Builder
	b.Grow(len(s) + 8)
	for i := 0; i < len(s); {
		c := s[i]
		if c == '\\' || c == '(' || c == ')' {
			b.WriteByte('\\')
			b.WriteByte(c)
			i++
			continue
		}
		// Encode literal '%' so "%0A" (sender text) ≠ "%0A" (our newline encoding).
		if c == '%' {
			b.WriteString("%25")
			i++
			continue
		}
		r, size := utf8.DecodeRuneInString(s[i:])
		if r == utf8.RuneError && size == 1 {
			fmt.Fprintf(&b, "%%%02X", c)
			i++
			continue
		}
		// Controls (C0/C1/DEL) and Unicode White_Space / line separators.
		if unicode.IsControl(r) || unicode.IsSpace(r) {
			for j := 0; j < size; j++ {
				fmt.Fprintf(&b, "%%%02X", s[i+j])
			}
			i += size
			continue
		}
		b.WriteString(s[i : i+size])
		i += size
	}
	return b.String()
}

// escapeMathSource sanitizes a sender-controlled math expression for use inside
// $...$ or $$...$$ fences under a parser-independent emitted invariant:
//
//  1. Every source '$' is emitted with an odd number of consecutive backslashes
//     immediately before it (so escape-processing consumers treat that '$' as
//     escaped). When the sender already provided an odd run, no extra '\' is
//     added — blind ReplaceAll("$", "\\$") would turn "\$" into "\\$" and
//     re-open the dollar.
//  2. The escaped body never ends with an odd trailing backslash run, so the
//     renderer-added closing fence '$' cannot be escape-neutralized.
//
// Ordinary LaTeX backslash commands (no '$', no odd trailing '\') are preserved.
func escapeMathSource(s string) string {
	return escapeMathBody(s, false)
}

// escapeInlineMathSource collapses every Unicode White_Space / line-paragraph
// separator rune to a single ASCII space (so inline $...$ cannot span lines),
// then applies the same math-body invariant as escapeMathSource.
func escapeInlineMathSource(s string) string {
	return escapeMathBody(s, true)
}

// escapeMathBody walks s once, optionally collapsing White_Space, and enforces
// the dollar/trailing-backslash invariant used by both inline and block math.
func escapeMathBody(s string, collapseSpace bool) string {
	var b strings.Builder
	b.Grow(len(s) + 8)
	bs := 0 // consecutive backslashes just written
	for _, r := range s {
		if collapseSpace && unicode.IsSpace(r) {
			b.WriteByte(' ')
			bs = 0
			continue
		}
		if r == '\\' {
			b.WriteByte('\\')
			bs++
			continue
		}
		if r == '$' {
			// Need an odd backslash run before '$'. Pad only when current run is even.
			if bs%2 == 0 {
				b.WriteByte('\\')
			}
			b.WriteByte('$')
			bs = 0
			continue
		}
		b.WriteRune(r)
		bs = 0
	}
	// Pad trailing odd backslash run so the renderer-added closing fence is safe.
	if bs%2 == 1 {
		b.WriteByte('\\')
	}
	return b.String()
}

// renderRichText renders an inline RichText tree to Markdown.
func renderRichText(t tg.RichTextClass, c *richCtx) string {
	if t == nil {
		return ""
	}
	if c.depth >= maxRichDepth {
		*c.truncated = true
		return ""
	}
	c.depth++
	defer func() { c.depth-- }()

	switch v := t.(type) {
	case *tg.TextEmpty:
		return ""
	case *tg.TextPlain:
		return escapeMarkdown(v.Text)
	case *tg.TextBold:
		return "**" + renderRichText(v.Text, c) + "**"
	case *tg.TextItalic:
		return "_" + renderRichText(v.Text, c) + "_"
	case *tg.TextUnderline:
		return "<u>" + renderRichText(v.Text, c) + "</u>"
	case *tg.TextStrike:
		return "~~" + renderRichText(v.Text, c) + "~~"
	case *tg.TextSpoiler:
		return "||" + renderRichText(v.Text, c) + "||"
	case *tg.TextFixed:
		return "`" + renderRichText(v.Text, c) + "`"
	case *tg.TextMarked:
		return "==" + renderRichText(v.Text, c) + "=="
	case *tg.TextSubscript:
		return "<sub>" + renderRichText(v.Text, c) + "</sub>"
	case *tg.TextSuperscript:
		return "<sup>" + renderRichText(v.Text, c) + "</sup>"
	case *tg.TextURL:
		return "[" + renderRichText(v.Text, c) + "](" + escapeLinkDest(v.URL) + ")"
	case *tg.TextEmail:
		return "[" + renderRichText(v.Text, c) + "](mailto:" + escapeLinkDest(v.Email) + ")"
	case *tg.TextPhone:
		return "[" + renderRichText(v.Text, c) + "](tel:" + escapeLinkDest(v.Phone) + ")"
	case *tg.TextConcat:
		var b strings.Builder
		for _, ch := range v.Texts {
			b.WriteString(renderRichText(ch, c))
		}
		return b.String()
	case *tg.TextAnchor:
		return renderRichText(v.Text, c)
	case *tg.TextMath:
		return "$" + escapeInlineMathSource(v.Source) + "$"
	case *tg.TextCustomEmoji:
		return escapeMarkdown(v.Alt)
	case *tg.TextMentionName:
		if name, ok := c.resolve[v.UserID]; ok {
			return name
		}
		return renderRichText(v.Text, c)
	case *tg.TextMention:
		return renderRichText(v.Text, c)
	default:
		// Fallback: any other RichText variant that has an inner Text field is
		// handled by extractInnerRichText; else empty. Never panic, never
		// silent-lose where inner text exists.
		return extractInnerRichText(t, c)
	}
}

// extractInnerRichText pulls inner text from RichText variants not explicitly
// handled above (TextHashtag/Cashtag/BotCommand/BankCard/AutoURL/AutoEmail/
// AutoPhone/Date/WithEntities/Image), falling back to "" only when none exists.
func extractInnerRichText(t tg.RichTextClass, c *richCtx) string {
	type hasText interface{ GetText() tg.RichTextClass }
	if ht, ok := t.(hasText); ok {
		return renderRichText(ht.GetText(), c)
	}
	return ""
}

// tlTypeName returns a short constructor name for a TL object for fallbacks.
func tlTypeName(v any) string {
	s := strings.TrimPrefix(fmt.Sprintf("%T", v), "*tg.")
	return s
}

// captionText renders a PageCaption's text (best-effort).
func captionText(caption tg.PageCaption, c *richCtx) string {
	return renderRichText(caption.Text, c)
}

func heading(level int, text tg.RichTextClass, c *richCtx) string {
	if level < 1 {
		level = 1
	}
	if level > 6 {
		level = 6
	}
	return strings.Repeat("#", level) + " " + renderRichText(text, c)
}

func renderPageBlock(b tg.PageBlockClass, c *richCtx) string {
	if b == nil {
		return ""
	}
	if c.depth >= maxRichDepth {
		*c.truncated = true
		return "[…]"
	}
	c.depth++
	defer func() { c.depth-- }()

	switch v := b.(type) {
	case *tg.PageBlockTitle:
		return heading(1, v.Text, c)
	case *tg.PageBlockHeading1:
		return heading(1, v.Text, c)
	case *tg.PageBlockSubtitle:
		return heading(2, v.Text, c)
	case *tg.PageBlockHeading2:
		return heading(2, v.Text, c)
	case *tg.PageBlockHeader:
		return heading(3, v.Text, c)
	case *tg.PageBlockHeading3:
		return heading(3, v.Text, c)
	case *tg.PageBlockHeading4:
		return heading(4, v.Text, c)
	case *tg.PageBlockHeading5:
		return heading(5, v.Text, c)
	case *tg.PageBlockHeading6:
		return heading(6, v.Text, c)
	case *tg.PageBlockSubheader:
		return "**" + renderRichText(v.Text, c) + "**"
	case *tg.PageBlockKicker:
		return "**" + renderRichText(v.Text, c) + "**"
	case *tg.PageBlockParagraph:
		return renderRichText(v.Text, c)
	case *tg.PageBlockPreformatted:
		return "```" + v.Language + "\n" + renderRichText(v.Text, c) + "\n```"
	case *tg.PageBlockBlockquote:
		out := "> " + renderRichText(v.Text, c)
		if caption := renderRichText(v.Caption, c); caption != "" {
			out += "\n> — " + caption
		}
		return out
	case *tg.PageBlockPullquote:
		return "> " + renderRichText(v.Text, c)
	case *tg.PageBlockBlockquoteBlocks:
		var lines []string
		for _, nb := range v.Blocks {
			lines = append(lines, "> "+renderPageBlock(nb, c))
		}
		return strings.Join(lines, "\n")
	case *tg.PageBlockDivider:
		return "---"
	case *tg.PageBlockMath:
		return "$$\n" + escapeMathSource(v.Source) + "\n$$"
	case *tg.PageBlockList:
		var lines []string
		for _, it := range v.Items {
			lines = append(lines, "- "+renderListItem(it, c))
		}
		return strings.Join(lines, "\n")
	case *tg.PageBlockOrderedList:
		var lines []string
		for _, it := range v.Items {
			lines = append(lines, renderOrderedItem(it, c))
		}
		return strings.Join(lines, "\n")
	case *tg.PageBlockDetails:
		out := "**" + renderRichText(v.Title, c) + "**"
		for _, nb := range v.Blocks {
			out += "\n> " + renderPageBlock(nb, c)
		}
		return out
	case *tg.PageBlockPhoto:
		return "![photo](" + captionText(v.Caption, c) + ")"
	case *tg.PageBlockVideo:
		return "[video: " + captionText(v.Caption, c) + "]"
	case *tg.PageBlockAudio:
		return "[audio: " + captionText(v.Caption, c) + "]"
	case *tg.PageBlockCover:
		return renderPageBlock(v.Cover, c)
	case *tg.PageBlockCollage:
		var lines []string
		for _, it := range v.Items {
			lines = append(lines, renderPageBlock(it, c))
		}
		return strings.Join(lines, "\n")
	case *tg.PageBlockSlideshow:
		var lines []string
		for _, it := range v.Items {
			lines = append(lines, renderPageBlock(it, c))
		}
		return strings.Join(lines, "\n")
	case *tg.PageBlockFooter:
		return "---\n" + renderRichText(v.Text, c)
	case *tg.PageBlockAuthorDate:
		return "_" + renderRichText(v.Author, c) + "_"
	case *tg.PageBlockAnchor:
		return "" // invisible anchor
	case *tg.PageBlockTable:
		var rows []string
		for ri, row := range v.Rows {
			var cells []string
			for _, cell := range row.Cells {
				cells = append(cells, renderRichText(cell.Text, c))
			}
			rows = append(rows, "| "+strings.Join(cells, " | ")+" |")
			if ri == 0 {
				sep := make([]string, len(row.Cells))
				for i := range sep {
					sep[i] = "---"
				}
				rows = append(rows, "| "+strings.Join(sep, " | ")+" |")
			}
		}
		return strings.Join(rows, "\n")
	default:
		inner := ""
		if ht, ok := b.(interface{ GetText() tg.RichTextClass }); ok {
			inner = renderRichText(ht.GetText(), c)
		}
		out := "[block: " + tlTypeName(v) + "]"
		if inner != "" {
			out += " " + inner
		}
		return out
	}
}

func renderListItem(it tg.PageListItemClass, c *richCtx) string {
	if t, ok := it.(*tg.PageListItemText); ok {
		text := renderRichText(t.Text, c)
		if t.Checkbox {
			if t.Checked {
				return "[x] " + text
			}
			return "[ ] " + text
		}
		return text
	}
	if b, ok := it.(*tg.PageListItemBlocks); ok {
		var parts []string
		for _, nb := range b.Blocks {
			parts = append(parts, renderPageBlock(nb, c))
		}
		return strings.Join(parts, " ")
	}
	return ""
}

func renderOrderedItem(it tg.PageListOrderedItemClass, c *richCtx) string {
	switch v := it.(type) {
	case *tg.PageListOrderedItemText:
		num := v.Num
		if num == "" {
			num = "1."
		} else if !strings.HasSuffix(num, ".") {
			num += "."
		}
		return num + " " + renderRichText(v.Text, c)
	case *tg.PageListOrderedItemBlocks:
		var parts []string
		for _, nb := range v.Blocks {
			parts = append(parts, renderPageBlock(nb, c))
		}
		return "1. " + strings.Join(parts, " ")
	}
	return ""
}

// RenderRichMessage renders a RichMessage block tree to Markdown. resolve is an
// OPTIONAL user_id->display-name map (may be nil); no network is performed.
// Returns truncated=true if any depth/size cap was hit.
func RenderRichMessage(rm tg.RichMessage, resolve map[int64]string) (string, bool) {
	trunc := false
	c := &richCtx{resolve: resolve, truncated: &trunc}
	var chunks []string
	for _, b := range rm.Blocks {
		s := renderPageBlock(b, c)
		if s == "" {
			continue
		}
		chunks = append(chunks, s)
	}
	md := strings.Join(chunks, "\n\n")
	if len(md) > maxRichBytes {
		// Cut at or below the byte cap without splitting a UTF-8 rune.
		n := maxRichBytes
		for n > 0 && !utf8.RuneStart(md[n]) {
			n--
		}
		md = md[:n] + "\n[…]"
		trunc = true
	}
	return md, trunc
}
