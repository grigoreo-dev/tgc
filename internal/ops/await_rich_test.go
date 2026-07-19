package ops

import (
	"testing"

	"github.com/gotd/td/tg"
)

func TestAwaitProjectionRendersRich(t *testing.T) {
	m := &tg.Message{ID: 3}
	m.SetRichMessage(tg.RichMessage{Blocks: []tg.PageBlockClass{
		&tg.PageBlockParagraph{Text: &tg.TextPlain{Text: "live rich"}},
	}})
	// await live path builds the map via messageToMap(raw, users, nil, target)
	out := messageToMap(m, nil, nil, 99)
	if out["text"] != "live rich" || out["rich"] != true {
		t.Fatalf("await rich projection: text=%v rich=%v", out["text"], out["rich"])
	}
}
