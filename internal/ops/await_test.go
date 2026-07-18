package ops

import (
	"testing"

	"github.com/gotd/td/tg"
)

func TestAcceptMessage(t *testing.T) {
	target := int64(555)
	// inbound text from target → accept
	m := &tg.Message{ID: 10, Message: "hi", PeerID: &tg.PeerUser{UserID: target}}
	if !acceptMessage(m, target, 0) {
		t.Fatal("inbound from target should be accepted")
	}
	// outgoing → reject
	mo := &tg.Message{ID: 11, Out: true, PeerID: &tg.PeerUser{UserID: target}}
	if acceptMessage(mo, target, 0) {
		t.Fatal("outgoing should be rejected")
	}
	// other chat → reject
	mx := &tg.Message{ID: 12, PeerID: &tg.PeerUser{UserID: 999}}
	if acceptMessage(mx, target, 0) {
		t.Fatal("other chat should be rejected")
	}
	// from-filter mismatch → reject
	mf := &tg.Message{ID: 13, PeerID: &tg.PeerUser{UserID: target}, FromID: &tg.PeerUser{UserID: 1}}
	if acceptMessage(mf, target, 2) {
		t.Fatal("from-filter mismatch should be rejected")
	}
	// from-filter match → accept
	mfm := &tg.Message{ID: 14, PeerID: &tg.PeerUser{UserID: target}, FromID: &tg.PeerUser{UserID: 2}}
	if !acceptMessage(mfm, target, 2) {
		t.Fatal("from-filter match should be accepted")
	}
}
