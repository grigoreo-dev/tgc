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

	// Basic group: resolve.Resolve yields target = -plainID (chatTDLibID),
	// while the wire PeerChat carries the raw positive ChatID. acceptMessage
	// must compare TDLib-vs-TDLib, so a PeerChat{42} matches target -42.
	chatTarget := int64(-42)
	mc := &tg.Message{ID: 20, PeerID: &tg.PeerChat{ChatID: 42}}
	if !acceptMessage(mc, chatTarget, 0) {
		t.Fatal("basic group message should be accepted when target matches (TDLib form)")
	}
	mcx := &tg.Message{ID: 21, PeerID: &tg.PeerChat{ChatID: 99}}
	if acceptMessage(mcx, chatTarget, 0) {
		t.Fatal("basic group message from a different chat should be rejected")
	}

	// Channel/megagroup: target = -(10^12 + plainID) (channelTDLibID), wire
	// PeerChannel carries the raw positive ChannelID.
	chanTarget := int64(-(1_000_000_000_000 + 55))
	mch := &tg.Message{ID: 22, PeerID: &tg.PeerChannel{ChannelID: 55}}
	if !acceptMessage(mch, chanTarget, 0) {
		t.Fatal("channel message should be accepted when target matches (TDLib form)")
	}
	mchx := &tg.Message{ID: 23, PeerID: &tg.PeerChannel{ChannelID: 77}}
	if acceptMessage(mchx, chanTarget, 0) {
		t.Fatal("channel message from a different channel should be rejected")
	}
}
