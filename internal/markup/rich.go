package markup

import (
	"encoding/json"
	"strings"

	"github.com/gotd/td/tg"

	"github.com/grigoreo-dev/tgc/internal/output"
)

// HasBlockContent reports whether md has block-level markup (headings/tables/lists/fences/quotes).
func HasBlockContent(md string) bool {
	for _, line := range strings.Split(md, "\n") {
		t := strings.TrimSpace(line)
		if strings.HasPrefix(t, "#") || strings.HasPrefix(t, "|") ||
			strings.HasPrefix(t, "- ") || strings.HasPrefix(t, "* ") ||
			strings.HasPrefix(t, "```") || strings.HasPrefix(t, "> ") {
			return true
		}
	}
	return false
}

// TryRichMarkdown wraps Markdown in InputRichMessageMarkdown for sendMessage.rich_message.
func TryRichMarkdown(md string) tg.InputRichMessageClass {
	return &tg.InputRichMessageMarkdown{Markdown: md}
}

// ParseRichJSON decodes expert --rich payload:
//
//	{"type":"markdown","markdown":"..."} | {"type":"html","html":"..."}
func ParseRichJSON(raw json.RawMessage) (tg.InputRichMessageClass, error) {
	var head struct {
		Type     string `json:"type"`
		Markdown string `json:"markdown"`
		HTML     string `json:"html"`
	}
	if err := json.Unmarshal(raw, &head); err != nil {
		return nil, output.Errf("bad_args", "invalid --rich JSON: %v", err)
	}
	switch strings.ToLower(head.Type) {
	case "markdown", "md", "":
		if head.Markdown == "" {
			return nil, output.Errf("bad_args", "--rich markdown payload requires markdown field")
		}
		return &tg.InputRichMessageMarkdown{Markdown: head.Markdown}, nil
	case "html":
		if head.HTML == "" {
			return nil, output.Errf("bad_args", "--rich html payload requires html field")
		}
		return &tg.InputRichMessageHTML{HTML: head.HTML}, nil
	default:
		return nil, output.Errf("bad_args", "unsupported --rich type %q (markdown|html)", head.Type)
	}
}
