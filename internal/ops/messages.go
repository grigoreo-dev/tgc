package ops

import (
	"crypto/rand"
	"encoding/binary"
	"encoding/json"
	"strings"
	"time"

	"github.com/gotd/td/tg"

	"github.com/grigoreo-dev/tgc/internal/client"
	"github.com/grigoreo-dev/tgc/internal/config"
	"github.com/grigoreo-dev/tgc/internal/markup"
	"github.com/grigoreo-dev/tgc/internal/output"
	"github.com/grigoreo-dev/tgc/internal/resolve"
)

const richFetchBudget = 10

// ParseDateArg parses a date argument in YYYY-MM-DD or RFC3339 form.
func ParseDateArg(s string) (time.Time, error) {
	for _, layout := range []string{"2006-01-02", time.RFC3339} {
		if t, err := time.Parse(layout, s); err == nil {
			return t, nil
		}
	}
	return time.Time{}, output.Errf("bad_args", "cannot parse date %q (want YYYY-MM-DD or RFC3339)", s)
}

// ReadOpts controls Read: paging, id/date windows, sender and search filters.
type ReadOpts struct {
	Limit    int
	BeforeID int
	AfterID  int
	Since    string
	Until    string
	From     string
	Search   string
}

// Read returns chat messages newest-first. Search or From routes through
// messages.search; otherwise messages.getHistory with id/date windows.
func Read(conn *client.Conn, selector string, o ReadOpts) ([]map[string]any, error) {
	peer, err := resolve.Resolve(conn, selector)
	if err != nil {
		return nil, err
	}
	ip, err := resolve.InputPeer(conn, peer)
	if err != nil {
		return nil, err
	}
	if o.Limit == 0 {
		o.Limit = 20
	}

	var sinceT time.Time
	if o.Since != "" {
		if sinceT, err = ParseDateArg(o.Since); err != nil {
			return nil, err
		}
	}

	switch {
	case o.Search != "" || o.From != "":
		req := &tg.MessagesSearchRequest{
			Peer: ip, Q: o.Search, Filter: &tg.InputMessagesFilterEmpty{}, Limit: o.Limit,
		}
		if o.From != "" {
			fromPeer, err := resolve.Resolve(conn, o.From)
			if err != nil {
				return nil, err
			}
			fip, err := resolve.InputPeer(conn, fromPeer)
			if err != nil {
				return nil, err
			}
			req.SetFromID(fip)
		}
		res, err := conn.Ctx.Raw.MessagesSearch(conn.Ctx, req)
		if err != nil {
			return nil, client.WrapErr(err)
		}
		maps := collectMessages(res, peer.ID, sinceT)
		autofetchRichParts(conn, ip, res, maps)
		stripRichMapKeys(maps)
		return maps, nil
	default:
		req := &tg.MessagesGetHistoryRequest{Peer: ip, Limit: o.Limit}
		if o.BeforeID > 0 {
			req.OffsetID = o.BeforeID
			req.MaxID = o.BeforeID
		}
		if o.AfterID > 0 {
			req.MinID = o.AfterID
		}
		if o.Until != "" {
			t, err := ParseDateArg(o.Until)
			if err != nil {
				return nil, err
			}
			req.OffsetDate = int(t.Unix())
		}
		res, err := conn.Ctx.Raw.MessagesGetHistory(conn.Ctx, req)
		if err != nil {
			return nil, client.WrapErr(err)
		}
		maps := collectMessages(res, peer.ID, sinceT)
		autofetchRichParts(conn, ip, res, maps)
		stripRichMapKeys(maps)
		return maps, nil
	}
}

// Context returns a message plus radius messages on each side.
func Context(conn *client.Conn, selector string, msgID, radius int) ([]map[string]any, error) {
	if radius <= 0 {
		radius = 10
	}
	peer, err := resolve.Resolve(conn, selector)
	if err != nil {
		return nil, err
	}
	ip, err := resolve.InputPeer(conn, peer)
	if err != nil {
		return nil, err
	}
	res, err := conn.Ctx.Raw.MessagesGetHistory(conn.Ctx, &tg.MessagesGetHistoryRequest{
		Peer: ip, OffsetID: msgID + radius + 1, Limit: radius*2 + 1,
	})
	if err != nil {
		return nil, client.WrapErr(err)
	}
	maps := collectMessages(res, peer.ID, time.Time{})
	autofetchRichParts(conn, ip, res, maps)
	stripRichMapKeys(maps)
	return maps, nil
}

// richPartIDs returns the ids of messages whose RichMessage is truncated (Part).
func richPartIDs(res tg.MessagesMessagesClass) []int {
	var ids []int
	for _, mc := range messagesFromResponse(res) {
		m, ok := mc.(*tg.Message)
		if !ok {
			continue
		}
		if rm, ok := m.GetRichMessage(); ok && rm.Part {
			ids = append(ids, m.ID)
		}
	}
	return ids
}

// peerKind discriminates wire peer types that can share a numeric id
// (PeerUser(1) vs PeerChat(1) vs PeerChannel(1)).
type peerKind uint8

const (
	peerKindUser peerKind = iota + 1
	peerKindChat
	peerKindChannel
)

// globalRichKey identifies a message in multi-peer results (global search).
// It encodes peer kind + numeric peer id + message id so kinds cannot collide.
// This is independent of the public historical chat_id field (plain peerID).
type globalRichKey struct {
	Kind   peerKind
	PeerID int64
	MsgID  int
}

// globalRichPlan is one planned getRichMessage for a Part message in global search.
type globalRichPlan struct {
	Key  globalRichKey
	Peer tg.InputPeerClass
}

// peerStorageLookup returns a non-empty InputPeer from local peer storage by
// TDLib-form id, or nil. Used only as a no-network fallback when the global
// search response lacks the peer's access hash.
type peerStorageLookup func(tdlibID int64) tg.InputPeerClass

// globalRichKeyFromPeer builds an internal multi-peer key from a wire Peer and
// message id. Zero PeerID or unknown peer yields a zero key (Kind==0).
func globalRichKeyFromPeer(peer tg.PeerClass, msgID int) globalRichKey {
	switch p := peer.(type) {
	case *tg.PeerUser:
		return globalRichKey{Kind: peerKindUser, PeerID: p.UserID, MsgID: msgID}
	case *tg.PeerChat:
		return globalRichKey{Kind: peerKindChat, PeerID: p.ChatID, MsgID: msgID}
	case *tg.PeerChannel:
		return globalRichKey{Kind: peerKindChannel, PeerID: p.ChannelID, MsgID: msgID}
	default:
		return globalRichKey{}
	}
}

// peerMatchesGlobalKey reports whether peer is the exact kind+id in key.
func peerMatchesGlobalKey(peer tg.PeerClass, key globalRichKey) bool {
	if key.Kind == 0 {
		return false
	}
	return globalRichKeyFromPeer(peer, key.MsgID) == key
}

// inputPeerFromSearchPeer builds an InputPeer for a wire Peer using response
// users/chats (preferred) or peer storage (fallback). No network resolves.
// Channels/megagroups without a non-zero access hash (response or storage) are
// omitted so we never plan a zero-hash InputPeerChannel RPC.
func inputPeerFromSearchPeer(
	peer tg.PeerClass,
	users map[int64]*tg.User,
	chats map[int64]tg.ChatClass,
	storage peerStorageLookup,
) (tg.InputPeerClass, bool) {
	if peer == nil {
		return nil, false
	}
	switch p := peer.(type) {
	case *tg.PeerUser:
		if u, ok := users[p.UserID]; ok {
			return &tg.InputPeerUser{UserID: p.UserID, AccessHash: u.AccessHash}, true
		}
		if storage != nil {
			if ip := storage(p.UserID); ip != nil {
				return ip, true
			}
		}
	case *tg.PeerChat:
		// Basic groups need no access hash.
		if _, ok := chats[p.ChatID]; ok {
			return &tg.InputPeerChat{ChatID: p.ChatID}, true
		}
		// Response may omit the chat object; InputPeerChat still works with plain id.
		if storage != nil {
			if ip := storage(resolve.TDLibPeerID(p)); ip != nil {
				return ip, true
			}
		}
		return &tg.InputPeerChat{ChatID: p.ChatID}, true
	case *tg.PeerChannel:
		if c, ok := chats[p.ChannelID]; ok {
			if ch, ok := c.(*tg.Channel); ok && ch.AccessHash != 0 {
				return &tg.InputPeerChannel{ChannelID: p.ChannelID, AccessHash: ch.AccessHash}, true
			}
		}
		if storage != nil {
			if ip := storage(resolve.TDLibPeerID(p)); ip != nil {
				if ich, ok := ip.(*tg.InputPeerChannel); ok && ich.AccessHash != 0 {
					return ip, true
				}
				// Non-channel storage entry is unusable for a channel peer.
			}
		}
	}
	return nil, false
}

// richMapKeyField is an unexported map key carrying globalRichKey for
// filter-safe indexing. It is stripped before any public JSON output.
const richMapKeyField = "_rich_global_key"

// indexMapsByGlobalKey indexes maps by the type-safe key attached at collection
// time. It does not walk the API response positionally, so since-filters and
// service skips cannot mis-pair a plan with the wrong map row.
func indexMapsByGlobalKey(maps []map[string]any) map[globalRichKey]map[string]any {
	out := make(map[globalRichKey]map[string]any, len(maps))
	for _, mp := range maps {
		if mp == nil {
			continue
		}
		key, ok := mp[richMapKeyField].(globalRichKey)
		if !ok || key.Kind == 0 {
			continue
		}
		out[key] = mp
	}
	return out
}

// stripRichMapKeys removes internal rich-index metadata so public message maps
// keep the historical field contract (no internal keys in JSON).
func stripRichMapKeys(maps []map[string]any) {
	for _, mp := range maps {
		if mp != nil {
			delete(mp, richMapKeyField)
		}
	}
}

// planGlobalRichFetches lists Part rich messages present in maps, each with a
// derived InputPeer. Keys encode peer kind + peer id + msg id so PeerUser(1),
// PeerChat(1), and PeerChannel(1) stay distinct. Messages without a derivable
// peer (e.g. channel missing access hash) or absent from maps (e.g. since
// filtered) are omitted and stay truncated.
func planGlobalRichFetches(res tg.MessagesMessagesClass, maps []map[string]any, storage peerStorageLookup) []globalRichPlan {
	byKey := indexMapsByGlobalKey(maps)
	if len(byKey) == 0 {
		return nil
	}
	users := messagesResponseUsers(res)
	chats := messagesResponseChats(res)
	var plans []globalRichPlan
	for _, mc := range messagesFromResponse(res) {
		m, ok := mc.(*tg.Message)
		if !ok {
			continue
		}
		rm, ok := m.GetRichMessage()
		if !ok || !rm.Part {
			continue
		}
		key := globalRichKeyFromPeer(m.PeerID, m.ID)
		if key.Kind == 0 || byKey[key] == nil {
			// Unknown peer or filtered out of maps (e.g. since) — do not plan.
			continue
		}
		ip, ok := inputPeerFromSearchPeer(m.PeerID, users, chats, storage)
		if !ok {
			continue
		}
		plans = append(plans, globalRichPlan{Key: key, Peer: ip})
	}
	return plans
}

// autofetchGlobalRichParts is the multi-peer counterpart of autofetchRichParts:
// each Part message is fetched with its own InputPeer. One shared richFetchBudget
// applies to the whole invocation; first FLOOD_WAIT stops further fetches.
// RPC failures are best-effort (inline partial + rich_truncated + warning).
// Full-fetch bodies are matched by peer-aware key, never by message id alone.
// Map rows are located via keys attached at collect time (not positional).
func autofetchGlobalRichParts(conn *client.Conn, res tg.MessagesMessagesClass, maps []map[string]any) {
	var storage peerStorageLookup
	if conn != nil && conn.Ctx != nil && conn.Ctx.PeerStorage != nil {
		storage = func(tdlibID int64) tg.InputPeerClass {
			ip := conn.Ctx.PeerStorage.GetInputPeerById(tdlibID)
			if ip == nil {
				return nil
			}
			if _, empty := ip.(*tg.InputPeerEmpty); empty {
				return nil
			}
			return ip
		}
	}
	plans := planGlobalRichFetches(res, maps, storage)
	if len(plans) == 0 {
		return
	}
	byKey := indexMapsByGlobalKey(maps)
	origResolve := richResolveMap(messagesResponseUsers(res))
	spent := 0
	stopped := false
	for _, plan := range plans {
		mp := byKey[plan.Key]
		if mp == nil {
			continue
		}
		if stopped || spent >= richFetchBudget {
			mp["rich_truncated"] = true
			continue
		}
		spent++
		full, err := conn.Ctx.Raw.MessagesGetRichMessage(conn.Ctx, &tg.MessagesGetRichMessageRequest{
			Peer: plan.Peer,
			ID:   plan.Key.MsgID,
		})
		if err != nil {
			mp["rich_truncated"] = true
			output.Warnf("rich_fetch_failed", "peer kind %d id %d msg %d: %v",
				plan.Key.Kind, plan.Key.PeerID, plan.Key.MsgID, err)
			if isFloodWait(err) {
				stopped = true
			}
			continue
		}
		if rm := fullRichMessageForKey(full, plan.Key); rm != nil {
			// Prefer plain from the full message; fall back to the list response
			// with the same peer-aware key.
			plain := messagePlainForKey(full, plan.Key)
			if plain == "" {
				plain = messagePlainForKey(res, plan.Key)
			}
			resolve := mergeRichResolve(origResolve, richResolveMap(messagesResponseUsers(full)))
			applyFetchedRichRender(mp, plain, *rm, resolve)
		} else {
			// Unexpected/wrong peer in fetch response — keep inline truncated body.
			mp["rich_truncated"] = true
		}
	}
}

// fullRichMessageForKey finds the message matching key (peer kind + peer id +
// msg id) in a getRichMessage response and returns its RichMessage. Same-ID
// bodies from a different peer are rejected.
func fullRichMessageForKey(res tg.MessagesMessagesClass, key globalRichKey) *tg.RichMessage {
	for _, mc := range messagesFromResponse(res) {
		m, ok := mc.(*tg.Message)
		if !ok || m.ID != key.MsgID {
			continue
		}
		if !peerMatchesGlobalKey(m.PeerID, key) {
			continue
		}
		if rm, ok := m.GetRichMessage(); ok {
			return &rm
		}
	}
	return nil
}

// messagePlainForKey returns m.Message for the *tg.Message matching key.
func messagePlainForKey(res tg.MessagesMessagesClass, key globalRichKey) string {
	for _, mc := range messagesFromResponse(res) {
		m, ok := mc.(*tg.Message)
		if !ok || m.ID != key.MsgID {
			continue
		}
		if peerMatchesGlobalKey(m.PeerID, key) {
			return m.Message
		}
	}
	return ""
}

// messagesResponseChats indexes Chat/Channel objects from a messages response
// by plain wire id (Chat.ID / Channel.ID).
func messagesResponseChats(res tg.MessagesMessagesClass) map[int64]tg.ChatClass {
	var chats []tg.ChatClass
	switch r := res.(type) {
	case *tg.MessagesMessages:
		chats = r.Chats
	case *tg.MessagesMessagesSlice:
		chats = r.Chats
	case *tg.MessagesChannelMessages:
		chats = r.Chats
	default:
		return nil
	}
	return indexChats(chats)
}

// autofetchRichParts re-renders truncated (Part) rich messages by fetching the
// full version via messages.getRichMessage, bounded by richFetchBudget and a
// stop-on-FLOOD_WAIT rule. It mutates maps in place (matched by "id").
//
// On success, text is rebuilt with the same plain-prefix semantics as
// messageToMap, and mentions resolve from the fetch response's users with
// fallback to the original list response's users (never drop a known resolve).
func autofetchRichParts(conn *client.Conn, ip tg.InputPeerClass, res tg.MessagesMessagesClass, maps []map[string]any) {
	partIDs := richPartIDs(res)
	if len(partIDs) == 0 {
		return
	}
	byID := map[int]map[string]any{}
	for _, mp := range maps {
		if id, ok := mp["id"].(int); ok {
			byID[id] = mp
		}
	}
	// Original list/history users for mention fallback when a fetch omits them.
	origResolve := richResolveMap(messagesResponseUsers(res))
	spent := 0
	stopped := false
	for _, id := range partIDs {
		mp := byID[id]
		if mp == nil {
			// since-filtered or otherwise absent from maps — do not spend budget
			continue
		}
		if stopped || spent >= richFetchBudget {
			mp["rich_truncated"] = true
			continue
		}
		spent++
		full, err := conn.Ctx.Raw.MessagesGetRichMessage(conn.Ctx, &tg.MessagesGetRichMessageRequest{Peer: ip, ID: id})
		if err != nil {
			mp["rich_truncated"] = true
			output.Warnf("rich_fetch_failed", "id %d: %v", id, err)
			if isFloodWait(err) {
				stopped = true
			}
			continue
		}
		if rm := fullRichMessage(full, id); rm != nil {
			// Prefer plain from the full message; fall back to the list response
			// message (id-keyed, not positional — service/since gaps are safe).
			plain := messagePlainByID(full, id)
			if plain == "" {
				plain = messagePlainByID(res, id)
			}
			resolve := mergeRichResolve(origResolve, richResolveMap(messagesResponseUsers(full)))
			applyFetchedRichRender(mp, plain, *rm, resolve)
		}
	}
}

// composeRichMapText applies messageToMap ordinary-prefix semantics: if plain
// is non-empty and rich Markdown is non-empty, join with "\n\n"; otherwise
// return whichever side is present.
func composeRichMapText(plain, richMD string) string {
	switch {
	case plain == "":
		return richMD
	case richMD == "":
		return plain
	default:
		return plain + "\n\n" + richMD
	}
}

// mergeRichResolve returns a mention resolve map where overlay entries win and
// base fills gaps. Empty inputs yield nil (renderer treats nil as no resolve).
func mergeRichResolve(base, overlay map[int64]string) map[int64]string {
	if len(base) == 0 && len(overlay) == 0 {
		return nil
	}
	out := make(map[int64]string, len(base)+len(overlay))
	for id, name := range base {
		out[id] = name
	}
	for id, name := range overlay {
		out[id] = name
	}
	return out
}

// applyFetchedRichRender rewrites mp text/flags after a successful full
// RichMessage fetch. No network; pure post-processing seam for tests.
func applyFetchedRichRender(mp map[string]any, plain string, rm tg.RichMessage, resolve map[int64]string) {
	if mp == nil {
		return
	}
	md, truncated := markup.RenderRichMessage(rm, resolve)
	if md == "" {
		return
	}
	mp["text"] = composeRichMapText(plain, md)
	mp["rich"] = true
	if truncated {
		mp["rich_truncated"] = true
	} else {
		delete(mp, "rich_truncated")
	}
}

// messagesResponseUsers extracts the Users slice from a MessagesMessagesClass.
func messagesResponseUsers(res tg.MessagesMessagesClass) map[int64]*tg.User {
	var users []tg.UserClass
	switch r := res.(type) {
	case *tg.MessagesMessages:
		users = r.Users
	case *tg.MessagesMessagesSlice:
		users = r.Users
	case *tg.MessagesChannelMessages:
		users = r.Users
	default:
		return nil
	}
	return indexUsers(users)
}

// messagesFromResponse returns the Messages slice for a MessagesMessagesClass.
func messagesFromResponse(res tg.MessagesMessagesClass) []tg.MessageClass {
	switch r := res.(type) {
	case *tg.MessagesMessages:
		return r.Messages
	case *tg.MessagesMessagesSlice:
		return r.Messages
	case *tg.MessagesChannelMessages:
		return r.Messages
	default:
		return nil
	}
}

// messagePlainByID returns m.Message for the *tg.Message with the given id, or "".
// Lookup is by message id, never by list position (service/since filters safe).
func messagePlainByID(res tg.MessagesMessagesClass, id int) string {
	for _, mc := range messagesFromResponse(res) {
		if m, ok := mc.(*tg.Message); ok && m.ID == id {
			return m.Message
		}
	}
	return ""
}

// fullRichMessage finds the message with id in a getRichMessage response and
// returns its RichMessage.
func fullRichMessage(res tg.MessagesMessagesClass, id int) *tg.RichMessage {
	for _, mc := range messagesFromResponse(res) {
		if m, ok := mc.(*tg.Message); ok && m.ID == id {
			if rm, ok := m.GetRichMessage(); ok {
				return &rm
			}
		}
	}
	return nil
}

// isFloodWait reports whether err is a Telegram FLOOD_WAIT.
func isFloodWait(err error) bool {
	return strings.Contains(strings.ToUpper(err.Error()), "FLOOD_WAIT")
}

// collectMessages extracts messages plus their attached users/chats from an
// API response, converts each to a map, and drops messages older than since
// (when set). A chatID of 0 makes each message derive its own chat_id.
func collectMessages(res tg.MessagesMessagesClass, chatID int64, since time.Time) []map[string]any {
	var (
		msgs  []tg.MessageClass
		users []tg.UserClass
		chats []tg.ChatClass
	)
	switch r := res.(type) {
	case *tg.MessagesMessages:
		msgs, users, chats = r.Messages, r.Users, r.Chats
	case *tg.MessagesMessagesSlice:
		msgs, users, chats = r.Messages, r.Users, r.Chats
	case *tg.MessagesChannelMessages:
		msgs, users, chats = r.Messages, r.Users, r.Chats
	default:
		return nil
	}
	userIdx := indexUsers(users)
	chatIdx := indexChats(chats)
	out := make([]map[string]any, 0, len(msgs))
	for _, mc := range msgs {
		m, ok := mc.(*tg.Message)
		if !ok {
			continue
		}
		if !since.IsZero() && time.Unix(int64(m.Date), 0).UTC().Before(since) {
			continue
		}
		cid := chatID
		if cid == 0 {
			cid = peerID(m.PeerID)
		}
		mp := messageToMap(m, userIdx, chatIdx, cid)
		// Attach type-safe multi-peer key for filter-safe rich indexing.
		// Stripped before public return (SearchMessages/Read/Context).
		if key := globalRichKeyFromPeer(m.PeerID, m.ID); key.Kind != 0 {
			mp[richMapKeyField] = key
		}
		out = append(out, mp)
	}
	return out
}

// messageToMap renders a message per the tgc field contract. Senders are read
// only from the users/chats attached to the API response (no extra resolves).
func messageToMap(m *tg.Message, users map[int64]*tg.User, chats map[int64]tg.ChatClass, chatID int64) map[string]any {
	out := map[string]any{
		"id":              m.ID,
		"chat_id":         chatID,
		"date":            time.Unix(int64(m.Date), 0).UTC().Format(time.RFC3339),
		"text":            m.Message,
		"reply_to":        nil,
		"media":           nil,
		"edited":          false,
		"fwd_from":        nil,
		"sender_id":       nil,
		"sender_name":     nil,
		"sender_username": nil,
	}

	if m.FromID != nil {
		if uid := peerUserID(m.FromID); uid != 0 {
			out["sender_id"] = uid
			if u, ok := users[uid]; ok {
				if name := userName(u); name != "" {
					out["sender_name"] = name
				}
				if u.Username != "" {
					out["sender_username"] = u.Username
				}
			}
		}
	}

	if m.ReplyTo != nil {
		if h, ok := m.ReplyTo.(*tg.MessageReplyHeader); ok && h.ReplyToMsgID != 0 {
			out["reply_to"] = h.ReplyToMsgID
		}
	}

	if m.EditDate != 0 {
		out["edited"] = true
	}

	if m.FwdFrom.FromID != nil || m.FwdFrom.FromName != "" {
		if name := fwdFromName(m.FwdFrom, users); name != "" {
			out["fwd_from"] = name
		}
	}

	if m.Media != nil {
		if mm := mediaToMap(m.Media); mm != nil {
			out["media"] = mm
		}
	}

	// rich:true means text contains a rich render. Photos/Documents on RichMessage
	// are a media reference pool for blocks, not standalone content — renderable
	// rich content always has Blocks.
	if rm, ok := m.GetRichMessage(); ok && !rm.Zero() && len(rm.Blocks) > 0 {
		md, truncated := markup.RenderRichMessage(rm, richResolveMap(users))
		if md != "" {
			if m.Message == "" {
				out["text"] = md
			} else {
				out["text"] = m.Message + "\n\n" + md
			}
			out["rich"] = true
			// rm.Part means the inline copy is itself truncated (the full body
			// needs a getRichMessage fetch). Flag it here so no-fetch paths
			// (await live handler, global search) never silently emit a partial
			// rich body; autofetchRichParts clears the flag on a successful
			// full re-render.
			if truncated || rm.Part {
				out["rich_truncated"] = true
			}
		}
	}

	// grouped_id ties together the members of an album / media group. Emit it
	// only when the message is actually grouped so ordinary messages stay
	// uncluttered; callers reassemble albums via jq group_by(.grouped_id).
	if gid, ok := m.GetGroupedID(); ok {
		out["grouped_id"] = gid
	}

	return out
}

// richResolveMap builds a user_id -> display-name map for rich mention rendering
// from the response-attached users (no extra resolves).
func richResolveMap(users map[int64]*tg.User) map[int64]string {
	if len(users) == 0 {
		return nil
	}
	m := make(map[int64]string, len(users))
	for id, u := range users {
		if name := userName(u); name != "" {
			m[id] = name
		} else if u.Username != "" {
			m[id] = "@" + u.Username
		}
	}
	return m
}

func mediaToMap(media tg.MessageMediaClass) map[string]any {
	switch mv := media.(type) {
	case *tg.MessageMediaPhoto:
		return map[string]any{"type": "photo"}
	case *tg.MessageMediaDocument:
		doc, ok := mv.Document.(*tg.Document)
		if !ok {
			return map[string]any{"type": "document"}
		}
		out := map[string]any{
			"type": "document",
			"size": doc.Size,
			"mime": doc.MimeType,
		}
		for _, attr := range doc.Attributes {
			switch a := attr.(type) {
			case *tg.DocumentAttributeVideo:
				out["type"] = "video"
			case *tg.DocumentAttributeAudio:
				if a.Voice {
					out["type"] = "voice"
				} else {
					out["type"] = "audio"
				}
			case *tg.DocumentAttributeFilename:
				out["file_name"] = a.FileName
			}
		}
		return out
	}
	return nil
}

func fwdFromName(fwd tg.MessageFwdHeader, users map[int64]*tg.User) string {
	if fwd.FromName != "" {
		return fwd.FromName
	}
	if fwd.FromID != nil {
		if uid := peerUserID(fwd.FromID); uid != 0 {
			if u, ok := users[uid]; ok {
				if name := userName(u); name != "" {
					return name
				}
			}
		}
	}
	return ""
}

func userName(u *tg.User) string {
	name := u.FirstName
	if u.LastName != "" {
		if name != "" {
			name += " "
		}
		name += u.LastName
	}
	return name
}

func indexChats(chats []tg.ChatClass) map[int64]tg.ChatClass {
	m := make(map[int64]tg.ChatClass, len(chats))
	for _, c := range chats {
		m[c.GetID()] = c
	}
	return m
}

func peerID(peer tg.PeerClass) int64 {
	switch p := peer.(type) {
	case *tg.PeerUser:
		return p.UserID
	case *tg.PeerChat:
		return p.ChatID
	case *tg.PeerChannel:
		return p.ChannelID
	}
	return 0
}

// SendOpts controls SendText: reply target, Markdown vs plain parsing, and an
// optional expert --rich payload (JSON) sent via sendMessage.rich_message.
type SendOpts struct {
	ReplyTo  int
	Plain    bool
	RichJSON string
}

// parseText converts message text to body + entities, honoring the plain flag.
func parseText(text string, plain bool) (string, []tg.MessageEntityClass, error) {
	if plain {
		return markup.ParsePlain(text)
	}
	return markup.Parse(text)
}

// randomID returns a non-zero cryptographically random int64 for message
// deduplication (Telegram's random_id).
func randomID() int64 {
	var b [8]byte
	for {
		if _, err := rand.Read(b[:]); err != nil {
			// crypto/rand should not fail; fall back to a time-derived seed.
			return time.Now().UnixNano() | 1
		}
		id := int64(binary.LittleEndian.Uint64(b[:]))
		if id != 0 {
			return id
		}
	}
}

// sentResult extracts message_id + date from a send/edit/forward update,
// pairing them with the given chat id in the tgc result shape.
func sentResult(upd tg.UpdatesClass, chatID int64) map[string]any {
	res := map[string]any{
		"message_id": nil,
		"chat_id":    chatID,
		"date":       nil,
	}
	switch u := upd.(type) {
	case *tg.UpdateShortSentMessage:
		res["message_id"] = u.ID
		res["date"] = time.Unix(int64(u.Date), 0).UTC().Format(time.RFC3339)
	case *tg.Updates:
		for _, up := range u.Updates {
			var mc tg.MessageClass
			switch nu := up.(type) {
			case *tg.UpdateNewMessage:
				mc = nu.Message
			case *tg.UpdateNewChannelMessage:
				mc = nu.Message
			case *tg.UpdateEditMessage:
				mc = nu.Message
			case *tg.UpdateEditChannelMessage:
				mc = nu.Message
			default:
				continue
			}
			if m, ok := mc.(*tg.Message); ok {
				res["message_id"] = m.ID
				res["date"] = time.Unix(int64(m.Date), 0).UTC().Format(time.RFC3339)
				break
			}
		}
	}
	return res
}

// useRichPath reports whether a send should attempt the rich_message path.
//
// It is used for the default (non-plain) path only, and bots must skip it:
// sendMessage.rich_message (InputRichMessageMarkdown) is a user/Premium-only
// feature. When a bot sends it, Telegram accepts the RPC WITHOUT error but
// silently drops the payload, delivering an empty-text message. Routing bots to
// the entities path instead carries their text correctly.
func useRichPath(plain bool, p *config.Profile) bool {
	if plain {
		return false
	}
	return p == nil || p.Type != "bot"
}

// SendText sends a text message to selector, applying Markdown unless plain.
func SendText(conn *client.Conn, selector, text string, o SendOpts) (map[string]any, error) {
	peer, err := resolve.Resolve(conn, selector)
	if err != nil {
		return nil, err
	}
	ip, err := resolve.InputPeer(conn, peer)
	if err != nil {
		return nil, err
	}
	body, entities, err := parseText(text, o.Plain)
	if err != nil {
		return nil, err
	}

	// setReplyTo applies the reply target to a request when requested.
	setReplyTo := func(req *tg.MessagesSendMessageRequest) {
		if o.ReplyTo > 0 {
			req.SetReplyTo(&tg.InputReplyToMessage{ReplyToMsgID: o.ReplyTo})
		}
	}

	// Path 1: explicit --rich payload. This is an expert request, so an RPC
	// failure surfaces to the caller — no silent entities fallback.
	if o.RichJSON != "" {
		rm, err := markup.ParseRichJSON(json.RawMessage(o.RichJSON))
		if err != nil {
			return nil, err
		}
		req := &tg.MessagesSendMessageRequest{
			Peer:     ip,
			Message:  body,
			RandomID: randomID(),
		}
		req.SetRichMessage(rm)
		setReplyTo(req)
		upd, err := conn.Ctx.Raw.MessagesSendMessage(conn.Ctx, req)
		if err != nil {
			return nil, client.WrapErr(err)
		}
		return sentResult(upd, peer.ID), nil
	}

	// Path 2: default (non-plain) — attempt rich_message, then transparently
	// fall back to the Task 6 entities path once if the user-layer rejects it.
	// Bots skip rich entirely (see useRichPath).
	if useRichPath(o.Plain, conn.Profile) {
		req := &tg.MessagesSendMessageRequest{
			Peer:     ip,
			Message:  body,
			RandomID: randomID(),
		}
		// InputRichMessageMarkdown expects the RAW Markdown source; parseText's
		// body has already been rendered/stripped, so use the original text.
		req.SetRichMessage(markup.TryRichMarkdown(text))
		setReplyTo(req)
		if upd, err := conn.Ctx.Raw.MessagesSendMessage(conn.Ctx, req); err == nil {
			return sentResult(upd, peer.ID), nil
		}
		// Rich rejected: retry once with the plain entities path, no error surfaced.
	}

	// Path 3 (and rich fallback): entities path — today's behavior.
	req := &tg.MessagesSendMessageRequest{
		Peer:     ip,
		Message:  body,
		RandomID: randomID(),
	}
	if len(entities) > 0 {
		req.SetEntities(entities)
	}
	setReplyTo(req)
	upd, err := conn.Ctx.Raw.MessagesSendMessage(conn.Ctx, req)
	if err != nil {
		return nil, client.WrapErr(err)
	}
	return sentResult(upd, peer.ID), nil
}

// EditText replaces the text of an existing message.
func EditText(conn *client.Conn, selector string, msgID int, text string, plain bool) (map[string]any, error) {
	peer, err := resolve.Resolve(conn, selector)
	if err != nil {
		return nil, err
	}
	ip, err := resolve.InputPeer(conn, peer)
	if err != nil {
		return nil, err
	}
	body, entities, err := parseText(text, plain)
	if err != nil {
		return nil, err
	}
	req := &tg.MessagesEditMessageRequest{
		Peer:    ip,
		ID:      msgID,
		Message: body,
	}
	if len(entities) > 0 {
		req.SetEntities(entities)
	}
	upd, err := conn.Ctx.Raw.MessagesEditMessage(conn.Ctx, req)
	if err != nil {
		return nil, client.WrapErr(err)
	}
	return sentResult(upd, peer.ID), nil
}

// Delete removes messages. For channels it uses channels.deleteMessages (no
// per-user option); elsewhere messages.deleteMessages with revoke = !forMe.
func Delete(conn *client.Conn, selector string, ids []int, forMe bool) (map[string]any, error) {
	peer, err := resolve.Resolve(conn, selector)
	if err != nil {
		return nil, err
	}
	if peer.Type == "channel" {
		if forMe {
			return nil, output.Errf("bad_args", "channels cannot delete messages only for you")
		}
		ch, err := resolve.InputChannel(peer)
		if err != nil {
			return nil, err
		}
		if _, err := conn.Ctx.Raw.ChannelsDeleteMessages(conn.Ctx, &tg.ChannelsDeleteMessagesRequest{
			Channel: ch,
			ID:      ids,
		}); err != nil {
			return nil, client.WrapErr(err)
		}
		return map[string]any{"status": "ok", "deleted": len(ids)}, nil
	}
	if _, err := conn.Ctx.Raw.MessagesDeleteMessages(conn.Ctx, &tg.MessagesDeleteMessagesRequest{
		Revoke: !forMe,
		ID:     ids,
	}); err != nil {
		return nil, client.WrapErr(err)
	}
	return map[string]any{"status": "ok", "deleted": len(ids)}, nil
}

// Forward copies a single message from fromSel into toSel.
func Forward(conn *client.Conn, fromSel string, msgID int, toSel string) (map[string]any, error) {
	fromPeer, err := resolve.Resolve(conn, fromSel)
	if err != nil {
		return nil, err
	}
	fromIP, err := resolve.InputPeer(conn, fromPeer)
	if err != nil {
		return nil, err
	}
	toPeer, err := resolve.Resolve(conn, toSel)
	if err != nil {
		return nil, err
	}
	toIP, err := resolve.InputPeer(conn, toPeer)
	if err != nil {
		return nil, err
	}
	upd, err := conn.Ctx.Raw.MessagesForwardMessages(conn.Ctx, &tg.MessagesForwardMessagesRequest{
		FromPeer: fromIP,
		ToPeer:   toIP,
		ID:       []int{msgID},
		RandomID: []int64{randomID()},
	})
	if err != nil {
		return nil, client.WrapErr(err)
	}
	return sentResult(upd, toPeer.ID), nil
}
