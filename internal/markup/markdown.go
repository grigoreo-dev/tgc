// Package markup converts agent-friendly Markdown into Telegram message
// text + entities (UTF-16 offsets), degrading block elements gracefully.
package markup

import (
	"strings"
	"unicode/utf16"

	"github.com/gotd/td/tg"
)

// ParsePlain returns text unchanged with no entities.
func ParsePlain(s string) (string, []tg.MessageEntityClass, error) {
	return s, nil, nil
}

// Parse converts a Markdown subset to text + entities.
// Supported inline: **bold**, *italic*, _italic_, `code`, ~~strike~~, [t](url).
// Block level: ``` fences → Pre, # headings → bold line, - lists → bullets,
// > quotes → Blockquote, tables → Pre, everything else passes through.
func Parse(md string) (string, []tg.MessageEntityClass, error) {
	var out strings.Builder
	var ents []tg.MessageEntityClass
	lines := strings.Split(md, "\n")
	i := 0
	for i < len(lines) {
		line := lines[i]
		switch {
		case strings.HasPrefix(line, "```"):
			lang := strings.TrimPrefix(line, "```")
			var code []string
			i++
			for i < len(lines) && !strings.HasPrefix(lines[i], "```") {
				code = append(code, lines[i])
				i++
			}
			if i < len(lines) {
				i++ // skip closing fence
			}
			body := strings.Join(code, "\n")
			start := utf16len(out.String())
			out.WriteString(body)
			ents = append(ents, &tg.MessageEntityPre{Offset: start, Length: utf16len(body), Language: lang})
		case strings.HasPrefix(line, "|"):
			var table []string
			for i < len(lines) && strings.HasPrefix(lines[i], "|") {
				table = append(table, lines[i])
				i++
			}
			body := strings.Join(table, "\n")
			start := utf16len(out.String())
			out.WriteString(body)
			ents = append(ents, &tg.MessageEntityPre{Offset: start, Length: utf16len(body)})
		case strings.HasPrefix(line, "#"):
			title := strings.TrimSpace(strings.TrimLeft(line, "#"))
			start := utf16len(out.String())
			out.WriteString(title)
			ents = append(ents, &tg.MessageEntityBold{Offset: start, Length: utf16len(title)})
			i++
		case strings.HasPrefix(line, "- ") || strings.HasPrefix(line, "* "):
			item := line[2:]
			text, sub := parseInline(item, utf16len(out.String())+utf16len("• "))
			out.WriteString("• " + text)
			ents = append(ents, sub...)
			i++
		case strings.HasPrefix(line, "> "):
			quoted := strings.TrimPrefix(line, "> ")
			start := utf16len(out.String())
			text, sub := parseInline(quoted, start)
			out.WriteString(text)
			ents = append(ents, &tg.MessageEntityBlockquote{Offset: start, Length: utf16len(text)})
			ents = append(ents, sub...)
			i++
		default:
			text, sub := parseInline(line, utf16len(out.String()))
			out.WriteString(text)
			ents = append(ents, sub...)
			i++
		}
		if i <= len(lines)-1 {
			out.WriteString("\n")
		}
	}
	return out.String(), ents, nil
}

func utf16len(s string) int {
	return len(utf16.Encode([]rune(s)))
}

// parseInline converts inline Markdown markers to text + entities.
// base is the UTF-16 offset of this fragment in the full message.
func parseInline(s string, base int) (string, []tg.MessageEntityClass) {
	var out strings.Builder
	var ents []tg.MessageEntityClass
	i := 0
	for i < len(s) {
		// Links: [label](url)
		if s[i] == '[' {
			if label, url, next, ok := takeLink(s, i); ok {
				start := base + utf16len(out.String())
				out.WriteString(label)
				ents = append(ents, &tg.MessageEntityTextURL{
					Offset: start,
					Length: utf16len(label),
					URL:    url,
				})
				i = next
				continue
			}
		}
		// Bold: **text**
		if strings.HasPrefix(s[i:], "**") {
			if inner, next, ok := takeDelim(s, i, "**"); ok {
				start := base + utf16len(out.String())
				out.WriteString(inner)
				ents = append(ents, &tg.MessageEntityBold{Offset: start, Length: utf16len(inner)})
				i = next
				continue
			}
		}
		// Strike: ~~text~~
		if strings.HasPrefix(s[i:], "~~") {
			if inner, next, ok := takeDelim(s, i, "~~"); ok {
				start := base + utf16len(out.String())
				out.WriteString(inner)
				ents = append(ents, &tg.MessageEntityStrike{Offset: start, Length: utf16len(inner)})
				i = next
				continue
			}
		}
		// Inline code: `text`
		if s[i] == '`' {
			if inner, next, ok := takeDelim(s, i, "`"); ok {
				start := base + utf16len(out.String())
				out.WriteString(inner)
				ents = append(ents, &tg.MessageEntityCode{Offset: start, Length: utf16len(inner)})
				i = next
				continue
			}
		}
		// Italic: *text* or _text_ (single, not part of **)
		if s[i] == '*' || s[i] == '_' {
			delim := string(s[i])
			if inner, next, ok := takeDelim(s, i, delim); ok {
				start := base + utf16len(out.String())
				out.WriteString(inner)
				ents = append(ents, &tg.MessageEntityItalic{Offset: start, Length: utf16len(inner)})
				i = next
				continue
			}
		}
		// Literal rune
		r, size := decodeRune(s[i:])
		out.WriteRune(r)
		i += size
	}
	return out.String(), ents
}

func takeDelim(s string, start int, delim string) (inner string, next int, ok bool) {
	dlen := len(delim)
	if start+dlen > len(s) || s[start:start+dlen] != delim {
		return "", 0, false
	}
	j := start + dlen
	for j+dlen <= len(s) {
		if s[j:j+dlen] == delim {
			return s[start+dlen : j], j + dlen, true
		}
		j++
	}
	return "", 0, false
}

func takeLink(s string, start int) (label, url string, next int, ok bool) {
	// [label](url)
	if start >= len(s) || s[start] != '[' {
		return "", "", 0, false
	}
	closeBracket := strings.IndexByte(s[start+1:], ']')
	if closeBracket < 0 {
		return "", "", 0, false
	}
	closeBracket += start + 1
	if closeBracket+1 >= len(s) || s[closeBracket+1] != '(' {
		return "", "", 0, false
	}
	closeParen := strings.IndexByte(s[closeBracket+2:], ')')
	if closeParen < 0 {
		return "", "", 0, false
	}
	closeParen += closeBracket + 2
	label = s[start+1 : closeBracket]
	url = s[closeBracket+2 : closeParen]
	return label, url, closeParen + 1, true
}

func decodeRune(s string) (rune, int) {
	if s == "" {
		return 0, 0
	}
	r := []rune(s)
	// size in bytes of first rune
	return r[0], len(string(r[0]))
}
