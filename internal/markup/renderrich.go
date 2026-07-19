package markup

import (
	"strings"

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
		return "[" + renderRichText(v.Text, c) + "](" + v.URL + ")"
	case *tg.TextEmail:
		return "[" + renderRichText(v.Text, c) + "](mailto:" + v.Email + ")"
	case *tg.TextPhone:
		return "[" + renderRichText(v.Text, c) + "](tel:" + v.Phone + ")"
	case *tg.TextConcat:
		var b strings.Builder
		for _, ch := range v.Texts {
			b.WriteString(renderRichText(ch, c))
		}
		return b.String()
	case *tg.TextAnchor:
		return renderRichText(v.Text, c)
	case *tg.TextMath:
		return "$" + v.Source + "$"
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
