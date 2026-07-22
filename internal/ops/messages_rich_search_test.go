package ops

import (
	"testing"
	"time"

	"github.com/gotd/td/tg"

	"github.com/grigoreo-dev/tgc/internal/markup"
	"github.com/grigoreo-dev/tgc/internal/resolve"
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

func fullRichBlocksText(s string) []tg.PageBlockClass {
	return []tg.PageBlockClass{&tg.PageBlockParagraph{Text: &tg.TextPlain{Text: s}}}
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
		if seen[p.Key] {
			t.Fatalf("duplicate plan key %+v", p.Key)
		}
		seen[p.Key] = true
		if p.Key.MsgID != 42 {
			t.Fatalf("msg id = %d, want 42", p.Key.MsgID)
		}
		if p.Peer == nil {
			t.Fatalf("peer must be derived for key %+v", p.Key)
		}
	}
	want1 := globalRichKey{Kind: peerKindUser, PeerID: 10, MsgID: 42}
	want2 := globalRichKey{Kind: peerKindUser, PeerID: 20, MsgID: 42}
	if !seen[want1] || !seen[want2] {
		t.Fatalf("keys = %v, want %+v and %+v", seen, want1, want2)
	}
}

// PeerUser(1), PeerChat(1), PeerChannel(1) share a numeric id but must stay
// distinct internal keys; public chat_id may still be 1 for all three.
func TestPlanGlobalRichFetchesDistinctPeerKindsSameNumericID(t *testing.T) {
	u := &tg.User{ID: 1, AccessHash: 11}
	basic := &tg.Chat{ID: 1, Title: "basic"}
	ch := &tg.Channel{ID: 1, AccessHash: 22, Title: "chan"}
	mUser := partRich(42, &tg.PeerUser{UserID: 1}, "u-partial")
	mChat := partRich(42, &tg.PeerChat{ChatID: 1}, "g-partial")
	mChan := partRich(42, &tg.PeerChannel{ChannelID: 1}, "c-partial")
	res := &tg.MessagesMessages{
		Messages: []tg.MessageClass{mUser, mChat, mChan},
		Users:    []tg.UserClass{u},
		Chats:    []tg.ChatClass{basic, ch},
	}
	maps := collectMessages(res, 0, timeZero())
	if len(maps) != 3 {
		t.Fatalf("maps = %d, want 3", len(maps))
	}
	// Historical public chat_id is plain peerID for all three (=1).
	for i, mp := range maps {
		if mp["chat_id"] != int64(1) || mp["id"] != 42 {
			t.Fatalf("map[%d] public ids = chat_id=%v id=%v, want 1/42", i, mp["chat_id"], mp["id"])
		}
	}

	plans := planGlobalRichFetches(res, maps, nil)
	if len(plans) != 3 {
		t.Fatalf("plans = %d, want 3 distinct peer kinds", len(plans))
	}
	byKey := indexMapsByGlobalKey(maps)
	if len(byKey) != 3 {
		t.Fatalf("index size = %d, want 3 (kinds must not collide)", len(byKey))
	}
	kUser := globalRichKey{Kind: peerKindUser, PeerID: 1, MsgID: 42}
	kChat := globalRichKey{Kind: peerKindChat, PeerID: 1, MsgID: 42}
	kChan := globalRichKey{Kind: peerKindChannel, PeerID: 1, MsgID: 42}
	for _, k := range []globalRichKey{kUser, kChat, kChan} {
		if byKey[k] == nil {
			t.Fatalf("missing map for key %+v", k)
		}
	}

	// Apply full render only to the user map; chat/channel stay partial.
	full := tg.RichMessage{Blocks: fullRichBlocksText("user-full")}
	applyFetchedRichRender(byKey[kUser], "u-partial", full, nil)
	if byKey[kUser]["text"] != "u-partial\n\nuser-full" {
		t.Fatalf("user text = %q", byKey[kUser]["text"])
	}
	if byKey[kChat]["text"] == byKey[kUser]["text"] || byKey[kChan]["text"] == byKey[kUser]["text"] {
		t.Fatalf("apply must not bleed across peer kinds: chat=%q chan=%q user=%q",
			byKey[kChat]["text"], byKey[kChan]["text"], byKey[kUser]["text"])
	}
	if byKey[kChat]["rich_truncated"] != true || byKey[kChan]["rich_truncated"] != true {
		t.Fatalf("unfetched kinds must remain rich_truncated")
	}
}

func TestInputPeerFromSearchPeerUserGroupChannel(t *testing.T) {
	users := map[int64]*tg.User{
		7: {ID: 7, AccessHash: 99},
	}
	chats := map[int64]tg.ChatClass{
		42: &tg.Chat{ID: 42, Title: "basic"},
		55: &tg.Channel{ID: 55, AccessHash: 12345, Title: "chan"},
		66: &tg.Channel{ID: 66, AccessHash: 67890, Title: "mega", Megagroup: true},
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

// Channel/megagroup without a usable access hash must not produce a zero-hash
// InputPeerChannel plan (omit entirely; stay truncated).
func TestInputPeerChannelOmitsWithoutAccessHash(t *testing.T) {
	// No chats object, no storage.
	if _, ok := inputPeerFromSearchPeer(&tg.PeerChannel{ChannelID: 55}, nil, nil, nil); ok {
		t.Fatal("channel without response/storage must be omitted")
	}
	// *tg.Channel present but AccessHash==0, no storage.
	chats := map[int64]tg.ChatClass{
		55: &tg.Channel{ID: 55, AccessHash: 0, Title: "nohash"},
		66: &tg.Channel{ID: 66, AccessHash: 0, Title: "mega", Megagroup: true},
	}
	if _, ok := inputPeerFromSearchPeer(&tg.PeerChannel{ChannelID: 55}, nil, chats, nil); ok {
		t.Fatal("channel with zero AccessHash must be omitted")
	}
	if _, ok := inputPeerFromSearchPeer(&tg.PeerChannel{ChannelID: 66}, nil, chats, nil); ok {
		t.Fatal("megagroup with zero AccessHash must be omitted")
	}
	// Planning must also skip such messages.
	m := partRich(9, &tg.PeerChannel{ChannelID: 55}, "x")
	res := &tg.MessagesMessages{
		Messages: []tg.MessageClass{m},
		Chats:    []tg.ChatClass{chats[55].(*tg.Channel)},
	}
	maps := collectMessages(res, 0, timeZero())
	plans := planGlobalRichFetches(res, maps, nil)
	if len(plans) != 0 {
		t.Fatalf("plans = %+v, want none for zero-hash channel", plans)
	}
}

func TestInputPeerChannelStorageFallback(t *testing.T) {
	wantHash := int64(999)
	storage := func(tdlibID int64) tg.InputPeerClass {
		if tdlibID == resolve.TDLibPeerID(&tg.PeerChannel{ChannelID: 55}) {
			return &tg.InputPeerChannel{ChannelID: 55, AccessHash: wantHash}
		}
		return nil
	}
	// No response channel object — storage supplies hash.
	ip, ok := inputPeerFromSearchPeer(&tg.PeerChannel{ChannelID: 55}, nil, nil, storage)
	if !ok {
		t.Fatal("channel storage fallback must resolve")
	}
	ich, ok := ip.(*tg.InputPeerChannel)
	if !ok || ich.ChannelID != 55 || ich.AccessHash != wantHash {
		t.Fatalf("storage channel = %#v", ip)
	}
	// Zero-hash response channel should still fall through to storage.
	chats := map[int64]tg.ChatClass{55: &tg.Channel{ID: 55, AccessHash: 0}}
	ip, ok = inputPeerFromSearchPeer(&tg.PeerChannel{ChannelID: 55}, nil, chats, storage)
	if !ok {
		t.Fatal("zero-hash response must fall through to storage")
	}
	ich, ok = ip.(*tg.InputPeerChannel)
	if !ok || ich.AccessHash != wantHash {
		t.Fatalf("storage after zero-hash = %#v", ip)
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
	if len(plans) != 1 || plans[0].Key.MsgID != 1 {
		t.Fatalf("plans = %+v, want only msg 1", plans)
	}
}

// An early message dropped by since must not steal the Part row's map index.
// Positional pairing would bind the remaining Part plan to the wrong map (or
// leave it mis-keyed); key-attached indexing keeps the Part on its own row.
func TestIndexMapsSafeWhenSinceFiltersEarlyMessage(t *testing.T) {
	u := &tg.User{ID: 7, AccessHash: 70}
	// Old plain message (filtered by since) then a newer Part rich message.
	old := &tg.Message{
		ID:      1,
		Message: "old",
		Date:    int(time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC).Unix()),
		PeerID:  &tg.PeerUser{UserID: 7},
	}
	part := partRich(2, &tg.PeerUser{UserID: 7}, "keep-plain")
	part.Date = int(time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC).Unix())
	res := &tg.MessagesMessages{
		Messages: []tg.MessageClass{old, part},
		Users:    []tg.UserClass{u},
	}
	since := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	maps := collectMessages(res, 0, since)
	if len(maps) != 1 {
		t.Fatalf("maps len = %d, want 1 after since filter", len(maps))
	}
	if maps[0]["id"] != 2 {
		t.Fatalf("surviving map id = %v, want 2", maps[0]["id"])
	}

	want := globalRichKey{Kind: peerKindUser, PeerID: 7, MsgID: 2}
	byKey := indexMapsByGlobalKey(maps)
	if byKey[want] == nil || byKey[want]["id"] != 2 {
		t.Fatalf("Part key %+v must address its own map row, got %v (index=%v)", want, byKey[want], byKey)
	}
	// Same underlying map: mutate via key and observe on maps[0].
	byKey[want]["__probe"] = true
	if maps[0]["__probe"] != true {
		t.Fatal("index must point at the surviving collectMessages row, not a copy/mispair")
	}
	delete(maps[0], "__probe")
	// Filtered-out id=1 must not appear under any key.
	for k := range byKey {
		if k.MsgID == 1 {
			t.Fatalf("filtered message must not be indexed: %+v", k)
		}
	}

	plans := planGlobalRichFetches(res, maps, nil)
	if len(plans) != 1 || plans[0].Key != want {
		t.Fatalf("plans = %+v, want single %+v", plans, want)
	}
	// Simulate full-fetch apply: only the surviving map row is rewritten.
	full := tg.RichMessage{Blocks: fullRichBlocksText("full-body")}
	applyFetchedRichRender(byKey[want], "keep-plain", full, nil)
	if maps[0]["text"] != "keep-plain\n\nfull-body" {
		t.Fatalf("applied text = %q, want keep-plain\\n\\nfull-body", maps[0]["text"])
	}
}

// Public message maps must not expose the internal global rich key after strip.
func TestStripRichMapKeysRemovesInternalMeta(t *testing.T) {
	u := &tg.User{ID: 3, AccessHash: 1}
	m := partRich(5, &tg.PeerUser{UserID: 3}, "x")
	res := &tg.MessagesMessages{Messages: []tg.MessageClass{m}, Users: []tg.UserClass{u}}
	maps := collectMessages(res, 0, timeZero())
	if _, ok := maps[0][richMapKeyField]; !ok {
		t.Fatalf("collectMessages must attach internal key for indexing")
	}
	stripRichMapKeys(maps)
	if _, ok := maps[0][richMapKeyField]; ok {
		t.Fatalf("stripRichMapKeys must remove %q from public maps", richMapKeyField)
	}
	// Public contract fields remain.
	if maps[0]["id"] != 5 || maps[0]["chat_id"] != int64(3) {
		t.Fatalf("public fields damaged: %+v", maps[0])
	}
}

// Service messages must not shift plan/map alignment (key-matched, not positional-only).
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
	want := globalRichKey{Kind: peerKindUser, PeerID: 5, MsgID: 11}
	if len(plans) != 1 || plans[0].Key != want {
		t.Fatalf("plans = %+v, want single %+v", plans, want)
	}
	byKey := indexMapsByGlobalKey(maps)
	mp := byKey[plans[0].Key]
	if mp == nil || mp["id"] != 11 {
		t.Fatalf("map alignment failed: %v", mp)
	}
}

// Budget/truncation semantics remain the pure applyFetchedRichRender seam
// (already covered in messages_rich_test); verify peer-kind keys keep maps
// distinct when public chat_id collides.
func TestGlobalKeyIndexAndApplyPreservesBudgetSemantics(t *testing.T) {
	kA := globalRichKey{Kind: peerKindUser, PeerID: 10, MsgID: 1}
	kB := globalRichKey{Kind: peerKindUser, PeerID: 20, MsgID: 1}
	mpA := map[string]any{"id": 1, "chat_id": int64(10), "text": "plain\n\npartial", "rich": true, "rich_truncated": true, richMapKeyField: kA}
	mpB := map[string]any{"id": 1, "chat_id": int64(20), "text": "plain\n\npartial", "rich": true, "rich_truncated": true, richMapKeyField: kB}
	byKey := indexMapsByGlobalKey([]map[string]any{mpA, mpB})
	if len(byKey) != 2 {
		t.Fatalf("index size = %d, want 2", len(byKey))
	}
	full := tg.RichMessage{Blocks: fullRichBlocks()}
	// Apply only to peer 10; peer 20 stays truncated (simulates budget exhaustion path).
	applyFetchedRichRender(byKey[kA], "plain", full, nil)
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

// full-fetch matching is peer-aware: a same-ID rich body from the wrong peer
// must not be applied (retain inline + rich_truncated).
func TestFullRichMessageForKeyRejectsWrongPeer(t *testing.T) {
	wrong := &tg.Message{ID: 7, Message: "other-plain", PeerID: &tg.PeerChannel{ChannelID: 99}}
	wrong.SetRichMessage(tg.RichMessage{Blocks: fullRichBlocksText("WRONG")})
	right := &tg.Message{ID: 7, Message: "mine-plain", PeerID: &tg.PeerUser{UserID: 1}}
	right.SetRichMessage(tg.RichMessage{Blocks: fullRichBlocksText("RIGHT")})

	key := globalRichKey{Kind: peerKindUser, PeerID: 1, MsgID: 7}

	// Wrong peer only (and first): must not match.
	onlyWrong := &tg.MessagesMessages{Messages: []tg.MessageClass{wrong}}
	if rm := fullRichMessageForKey(onlyWrong, key); rm != nil {
		t.Fatal("must not accept wrong-peer same-ID rich body")
	}
	if plain := messagePlainForKey(onlyWrong, key); plain != "" {
		t.Fatalf("plain for wrong peer = %q, want empty", plain)
	}

	// Wrong peer first, then correct: must select the correct body.
	both := &tg.MessagesMessages{Messages: []tg.MessageClass{wrong, right}}
	rm := fullRichMessageForKey(both, key)
	if rm == nil {
		t.Fatal("expected match for correct peer")
	}
	md, _ := markup.RenderRichMessage(*rm, nil)
	if md != "RIGHT" {
		t.Fatalf("matched body = %q, want RIGHT (not wrong-peer WRONG)", md)
	}
	if plain := messagePlainForKey(both, key); plain != "mine-plain" {
		t.Fatalf("plain = %q, want mine-plain", plain)
	}
}

// timeZero is a named zero time for readability in global-search tests.
func timeZero() time.Time { return time.Time{} }
