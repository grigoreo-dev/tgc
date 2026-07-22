package ops

import (
	"testing"
	"time"

	"github.com/gotd/td/tg"
)

func partRich(id int, peer tg.PeerClass, plain string) *tg.Message {
	m := &tg.Message{ID: id, Message: plain, PeerID: peer}
	m.SetRichMessage(tg.RichMessage{Part: true, Blocks: []tg.PageBlockClass{
		&tg.PageBlockParagraph{Text: &tg.TextPlain{Text: "partial"}},
	}})
	return m
}

func fullRichBlocks() []tg.PageBlockClass {
	return []tg.PageBlockClass{&tg.PageBlockParagraph{Text: &tg.TextPlain{Text: "full"}}}
}

// Two peers can share the same message ID; global plans must keep them distinct.
func TestPlanGlobalRichFetchesDistinctSameMsgID(t *testing.T) {
	u1 := &tg.User{ID: 10, AccessHash: 111, FirstName: "A"}
	u2 := &tg.User{ID: 20, AccessHash: 222, FirstName: "B"}
	m1 := partRich(42, &tg.PeerUser{UserID: 10}, "p1")
	m2 := partRich(42, &tg.PeerUser{UserID: 20}, "p2")
	res := &tg.MessagesMessages{
		Messages: []tg.MessageClass{m1, m2},
		Users:    []tg.UserClass{u1, u2},
	}
	maps := collectMessages(res, 0, timeZero())
	plans := planGlobalRichFetches(res, maps, nil)
	if len(plans) != 2 {
		t.Fatalf("plans = %d, want 2 (same msg id across peers)", len(plans))
	}
	seen := map[globalRichKey]bool{}
	for _, p := range plans {
		k := globalRichKey{ChatID: p.ChatID, MsgID: p.MsgID}
		if seen[k] {
			t.Fatalf("duplicate plan key %+v", k)
		}
		seen[k] = true
		if p.MsgID != 42 {
			t.Fatalf("msg id = %d, want 42", p.MsgID)
		}
		if p.Peer == nil {
			t.Fatalf("peer must be derived for chat %d", p.ChatID)
		}
	}
	if !seen[globalRichKey{ChatID: 10, MsgID: 42}] || !seen[globalRichKey{ChatID: 20, MsgID: 42}] {
		t.Fatalf("keys = %v, want chat 10 and 20 with id 42", seen)
	}
}

func TestInputPeerFromSearchPeerUserGroupChannel(t *testing.T) {
	users := map[int64]*tg.User{
		7: {ID: 7, AccessHash: 99},
	}
	chats := map[int64]tg.ChatClass{
		42:  &tg.Chat{ID: 42, Title: "basic"},
		55:  &tg.Channel{ID: 55, AccessHash: 12345, Title: "chan"},
		66:  &tg.Channel{ID: 66, AccessHash: 67890, Title: "mega", Megagroup: true},
	}

	// User with access hash from response.
	ip, ok := inputPeerFromSearchPeer(&tg.PeerUser{UserID: 7}, users, chats, nil)
	if !ok {
		t.Fatal("user peer must resolve from response users")
	}
	iu, ok := ip.(*tg.InputPeerUser)
	if !ok || iu.UserID != 7 || iu.AccessHash != 99 {
		t.Fatalf("user InputPeer = %#v", ip)
	}

	// Basic group — no access hash.
	ip, ok = inputPeerFromSearchPeer(&tg.PeerChat{ChatID: 42}, users, chats, nil)
	if !ok {
		t.Fatal("basic group must resolve")
	}
	ic, ok := ip.(*tg.InputPeerChat)
	if !ok || ic.ChatID != 42 {
		t.Fatalf("chat InputPeer = %#v", ip)
	}

	// Channel with access hash.
	ip, ok = inputPeerFromSearchPeer(&tg.PeerChannel{ChannelID: 55}, users, chats, nil)
	if !ok {
		t.Fatal("channel must resolve from chats")
	}
	ich, ok := ip.(*tg.InputPeerChannel)
	if !ok || ich.ChannelID != 55 || ich.AccessHash != 12345 {
		t.Fatalf("channel InputPeer = %#v", ip)
	}

	// Megagroup is still InputPeerChannel with its access hash.
	ip, ok = inputPeerFromSearchPeer(&tg.PeerChannel{ChannelID: 66}, users, chats, nil)
	if !ok {
		t.Fatal("megagroup must resolve from chats")
	}
	ich, ok = ip.(*tg.InputPeerChannel)
	if !ok || ich.ChannelID != 66 || ich.AccessHash != 67890 {
		t.Fatalf("megagroup InputPeer = %#v", ip)
	}
}

func TestInputPeerFromSearchPeerStorageFallback(t *testing.T) {
	// User missing from response users; storage supplies InputPeer.
	storage := func(tdlibID int64) tg.InputPeerClass {
		if tdlibID == 99 {
			return &tg.InputPeerUser{UserID: 99, AccessHash: 555}
		}
		return nil
	}
	ip, ok := inputPeerFromSearchPeer(&tg.PeerUser{UserID: 99}, nil, nil, storage)
	if !ok {
		t.Fatal("storage fallback must resolve user")
	}
	iu, ok := ip.(*tg.InputPeerUser)
	if !ok || iu.UserID != 99 || iu.AccessHash != 555 {
		t.Fatalf("storage user = %#v", ip)
	}
}

func TestPlanGlobalRichFetchesOnlyPart(t *testing.T) {
	u := &tg.User{ID: 1, AccessHash: 1}
	part := partRich(1, &tg.PeerUser{UserID: 1}, "a")
	// Non-Part rich (full inline body).
	full := &tg.Message{ID: 2, Message: "b", PeerID: &tg.PeerUser{UserID: 1}}
	full.SetRichMessage(tg.RichMessage{Part: false, Blocks: fullRichBlocks()})
	plain := &tg.Message{ID: 3, Message: "c", PeerID: &tg.PeerUser{UserID: 1}}
	res := &tg.MessagesMessages{
		Messages: []tg.MessageClass{part, full, plain},
		Users:    []tg.UserClass{u},
	}
	maps := collectMessages(res, 0, timeZero())
	plans := planGlobalRichFetches(res, maps, nil)
	if len(plans) != 1 || plans[0].MsgID != 1 {
		t.Fatalf("plans = %+v, want only msg 1", plans)
	}
}

// Service messages must not shift plan/map alignment (id-keyed, not positional).
func TestPlanGlobalRichFetchesSkipsServiceNoMisalign(t *testing.T) {
	u := &tg.User{ID: 5, AccessHash: 50}
	svc := &tg.MessageService{ID: 10, PeerID: &tg.PeerUser{UserID: 5}}
	part := partRich(11, &tg.PeerUser{UserID: 5}, "x")
	res := &tg.MessagesMessages{
		Messages: []tg.MessageClass{svc, part},
		Users:    []tg.UserClass{u},
	}
	maps := collectMessages(res, 0, timeZero())
	if len(maps) != 1 {
		t.Fatalf("maps len = %d, want 1 (service dropped)", len(maps))
	}
	plans := planGlobalRichFetches(res, maps, nil)
	if len(plans) != 1 || plans[0].MsgID != 11 || plans[0].ChatID != 5 {
		t.Fatalf("plans = %+v, want single id=11 chat=5", plans)
	}
	// Map lookup by composite key must find the planned message.
	byKey := indexMapsByGlobalKey(maps)
	mp := byKey[globalRichKey{ChatID: plans[0].ChatID, MsgID: plans[0].MsgID}]
	if mp == nil || mp["id"] != 11 {
		t.Fatalf("map alignment failed: %v", mp)
	}
}

// Budget/truncation semantics remain the pure applyFetchedRichRender seam
// (already covered in messages_rich_test); verify global map index preserves
// rich_truncated until apply clears it for a successful full render.
func TestGlobalKeyIndexAndApplyPreservesBudgetSemantics(t *testing.T) {
	mpA := map[string]any{"id": 1, "chat_id": int64(10), "text": "plain\n\npartial", "rich": true, "rich_truncated": true}
	mpB := map[string]any{"id": 1, "chat_id": int64(20), "text": "plain\n\npartial", "rich": true, "rich_truncated": true}
	byKey := indexMapsByGlobalKey([]map[string]any{mpA, mpB})
	if len(byKey) != 2 {
		t.Fatalf("index size = %d, want 2", len(byKey))
	}
	full := tg.RichMessage{Blocks: fullRichBlocks()}
	// Apply only to peer 10; peer 20 stays truncated (simulates budget exhaustion path).
	applyFetchedRichRender(byKey[globalRichKey{ChatID: 10, MsgID: 1}], "plain", full, nil)
	if mpA["text"] != "plain\n\nfull" {
		t.Fatalf("peer 10 text = %q", mpA["text"])
	}
	if _, ok := mpA["rich_truncated"]; ok {
		t.Fatalf("peer 10 should clear rich_truncated after full render")
	}
	if mpB["rich_truncated"] != true {
		t.Fatalf("peer 20 must remain truncated when not fetched")
	}
	if mpB["text"] != "plain\n\npartial" {
		t.Fatalf("peer 20 text must stay partial, got %q", mpB["text"])
	}
}

// timeZero is a named zero time for readability in global-search tests.
func timeZero() time.Time { return time.Time{} }
