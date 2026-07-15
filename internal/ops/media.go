package ops

import (
	"github.com/grigoreo-dev/tgc/internal/client"
	"github.com/grigoreo-dev/tgc/internal/output"
)

// FileOpts controls media sending: caption, document override, reply and
// Markdown vs plain caption parsing.
type FileOpts struct {
	Caption    string
	AsDocument bool // force image/* as document; default for images is photo
	ReplyTo    int
	Plain      bool
}

// SendFiles is a stub until media sending lands (Task 10).
func SendFiles(conn *client.Conn, selector string, files []string, o FileOpts) ([]map[string]any, error) {
	return nil, output.Errf("not_implemented", "file sending is not implemented yet")
}
