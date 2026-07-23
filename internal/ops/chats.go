// Package ops implements tgc core operations, reusable by future server modes.
package ops

import (
	"time"

	"github.com/gotd/td/tg"

	"github.com/grigoreo-dev/tgc/internal/client"
	"github.com/grigoreo-dev/tgc/internal/output"
	"github.com/grigoreo-dev/tgc/internal/resolve"
)

// Chats lists dialogs, optionally filtered by peer type (user|group|channel).
func Chats(conn *client.Conn, fresh bool, limit int, typeFilter string) ([]resolve.Peer, error) {
	peers, err := resolve.Dialogs(conn, fresh, 0)
	if err != nil {
		return nil, err
	}
	out := make([]resolve.Peer, 0, len(peers))
	for _, p := range peers {
		if typeFilter != "" && p.Type != typeFilter {
			continue
		}
		out = append(out, p)
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out, nil
}

// Info returns a card for a chat/user: id, type, title, and any of
// username, phone, about, members_count, bot that apply.
func Info(conn *client.Conn, selector string) (map[string]any, error) {
	p, err := resolve.Resolve(conn, selector)
	if err != nil {
		return nil, err
	}
	card := map[string]any{"id": p.ID, "type": p.Type, "title": p.Title}
	if p.Username != "" {
		card["username"] = p.Username
	}
	switch p.Type {
	case "user":
		in, err := resolve.InputUser(p)
		if err != nil {
			return nil, err
		}
		full, err := conn.Ctx.Raw.UsersGetFullUser(conn.Ctx, in)
		if err != nil {
			return nil, client.WrapErr(err)
		}
		if about, ok := full.FullUser.GetAbout(); ok && about != "" {
			card["about"] = about
		}
		for _, u := range full.Users {
			user, ok := u.(*tg.User)
			if !ok || user.ID != p.ID {
				continue
			}
			applyUserInfoCard(card, user)
		}
	case "channel":
		if err := fillChannelInfo(conn, p, card); err != nil {
			return nil, err
		}
	case "group":
		if p.AccessHash != 0 {
			if err := fillChannelInfo(conn, p, card); err != nil {
				return nil, err
			}
		} else if err := fillChatInfo(conn, p, card); err != nil {
			return nil, err
		}
	}
	return card, nil
}

// applyUserInfoCard projects user flags onto an info card. Pure: no network.
// Emits bot/premium only when true; phone only when non-empty.
func applyUserInfoCard(card map[string]any, user *tg.User) {
	if user == nil || card == nil {
		return
	}
	if user.Bot {
		card["bot"] = true
	}
	if user.Premium {
		card["premium"] = true
	}
	if phone, ok := user.GetPhone(); ok && phone != "" {
		card["phone"] = phone
	}
}

func fillChannelInfo(conn *client.Conn, p *resolve.Peer, card map[string]any) error {
	in, err := resolve.InputChannel(p)
	if err != nil {
		return err
	}
	full, err := conn.Ctx.Raw.ChannelsGetFullChannel(conn.Ctx, in)
	if err != nil {
		return client.WrapErr(err)
	}
	if cf, ok := full.FullChat.(*tg.ChannelFull); ok {
		if cf.About != "" {
			card["about"] = cf.About
		}
		if n, ok := cf.GetParticipantsCount(); ok {
			card["members_count"] = n
		}
	}
	return nil
}

func fillChatInfo(conn *client.Conn, p *resolve.Peer, card map[string]any) error {
	full, err := conn.Ctx.Raw.MessagesGetFullChat(conn.Ctx, resolve.PlainChatID(p.ID))
	if err != nil {
		return client.WrapErr(err)
	}
	if cf, ok := full.FullChat.(*tg.ChatFull); ok {
		if cf.About != "" {
			card["about"] = cf.About
		}
		if parts, ok := cf.GetParticipants().(*tg.ChatParticipants); ok {
			card["members_count"] = len(parts.Participants)
		}
	}
	return nil
}

// Members lists members of a group/channel: id, name, username, status
// (creator|admin|member|banned|left). A user selector is a bad_args error.
func Members(conn *client.Conn, selector string, limit int) ([]map[string]any, error) {
	p, err := resolve.Resolve(conn, selector)
	if err != nil {
		return nil, err
	}
	if p.Type == "user" {
		return nil, output.Errf("bad_args", "%q is a user, not a group", selector)
	}
	if p.Type == "group" && p.AccessHash == 0 {
		return legacyChatMembers(conn, p, limit)
	}
	in, err := resolve.InputChannel(p)
	if err != nil {
		return nil, err
	}
	var out []map[string]any
	offset := 0
	const page = 100
	for {
		want := page
		if limit > 0 && limit-len(out) < want {
			want = limit - len(out)
		}
		res, err := conn.Ctx.Raw.ChannelsGetParticipants(conn.Ctx, &tg.ChannelsGetParticipantsRequest{
			Channel: in,
			Filter:  &tg.ChannelParticipantsRecent{},
			Offset:  offset,
			Limit:   want,
		})
		if err != nil {
			return nil, client.WrapErr(err)
		}
		cp, ok := res.(*tg.ChannelsChannelParticipants)
		if !ok {
			break
		}
		users := indexUsers(cp.Users)
		for _, part := range cp.Participants {
			out = append(out, memberFromParticipant(part, users))
		}
		offset += len(cp.Participants)
		if len(cp.Participants) == 0 || offset >= cp.Count {
			break
		}
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func legacyChatMembers(conn *client.Conn, p *resolve.Peer, limit int) ([]map[string]any, error) {
	full, err := conn.Ctx.Raw.MessagesGetFullChat(conn.Ctx, resolve.PlainChatID(p.ID))
	if err != nil {
		return nil, client.WrapErr(err)
	}
	users := indexUsers(full.Users)
	cf, ok := full.FullChat.(*tg.ChatFull)
	if !ok {
		return nil, nil
	}
	parts, ok := cf.GetParticipants().(*tg.ChatParticipants)
	if !ok {
		return nil, nil
	}
	var out []map[string]any
	for _, cp := range parts.Participants {
		status := "member"
		switch cp.(type) {
		case *tg.ChatParticipantCreator:
			status = "creator"
		case *tg.ChatParticipantAdmin:
			status = "admin"
		}
		out = append(out, memberMap(cp.GetUserID(), status, users))
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out, nil
}

// SearchChats searches local dialogs (fuzzy) and the contacts.Search API,
// deduplicated by peer id. kind ("user"|"group"|"channel") filters results;
// empty kind keeps all.
func SearchChats(conn *client.Conn, query, kind string, limit int) ([]resolve.Peer, error) {
	local, err := resolve.Dialogs(conn, false, 0)
	if err != nil {
		return nil, err
	}
	found := resolve.FuzzyMatch(local, query)
	seen := map[int64]bool{}
	for _, p := range found {
		seen[p.ID] = true
	}
	res, err := conn.Ctx.Raw.ContactsSearch(conn.Ctx, &tg.ContactsSearchRequest{Q: query, Limit: 20})
	if err == nil { // API search is best-effort on top of local results
		for _, p := range resolve.PeersFromUsersChats(res.Users, res.Chats) {
			if !seen[p.ID] {
				found = append(found, p)
				seen[p.ID] = true
			}
		}
	}
	found = filterPeersByKind(found, kind)
	if limit > 0 && len(found) > limit {
		found = found[:limit]
	}
	return found, nil
}

// SearchMessages searches messages across the user's chats
// (messages.searchGlobal — "global" in the API means all chats known to the
// user, not all of Telegram). Each result derives its own chat_id from the
// message peer; Part rich messages are auto-fetched per peer. Rows are tagged
// result:"message".
func SearchMessages(conn *client.Conn, query string, o SearchOpts) ([]map[string]any, error) {
	limit := o.Limit
	if limit == 0 {
		limit = 20
	}
	req := &tg.MessagesSearchGlobalRequest{
		Q:          query,
		Limit:      limit,
		OffsetPeer: &tg.InputPeerEmpty{},
		Filter:     &tg.InputMessagesFilterEmpty{},
	}
	applyGlobalKind(req, o.Type)
	if o.Since != "" {
		t, err := ParseDateArg(o.Since)
		if err != nil {
			return nil, err
		}
		req.MinDate = int(t.Unix())
	}
	if o.Until != "" {
		t, err := ParseDateArg(o.Until)
		if err != nil {
			return nil, err
		}
		req.MaxDate = int(t.Unix())
	}
	res, err := conn.Ctx.Raw.MessagesSearchGlobal(conn.Ctx, req)
	if err != nil {
		return nil, client.WrapErr(err)
	}
	maps := collectMessages(res, 0, time.Time{})
	autofetchGlobalRichParts(conn, res, maps)
	stripRichMapKeys(maps)
	for _, m := range maps {
		m["result"] = "message"
	}
	return maps, nil
}

func indexUsers(users []tg.UserClass) map[int64]*tg.User {
	m := make(map[int64]*tg.User, len(users))
	for _, u := range users {
		if user, ok := u.(*tg.User); ok {
			m[user.ID] = user
		}
	}
	return m
}

func memberFromParticipant(part tg.ChannelParticipantClass, users map[int64]*tg.User) map[string]any {
	var uid int64
	status := "member"
	switch p := part.(type) {
	case *tg.ChannelParticipantCreator:
		uid, status = p.UserID, "creator"
	case *tg.ChannelParticipantAdmin:
		uid, status = p.UserID, "admin"
	case *tg.ChannelParticipantSelf:
		uid = p.UserID
	case *tg.ChannelParticipant:
		uid = p.UserID
	case *tg.ChannelParticipantBanned:
		uid, status = peerUserID(p.Peer), "banned"
	case *tg.ChannelParticipantLeft:
		uid, status = peerUserID(p.Peer), "left"
	}
	return memberMap(uid, status, users)
}

func memberMap(uid int64, status string, users map[int64]*tg.User) map[string]any {
	m := map[string]any{"id": uid, "status": status}
	if u, ok := users[uid]; ok {
		name := u.FirstName
		if u.LastName != "" {
			name = name + " " + u.LastName
		}
		if name != "" {
			m["name"] = name
		}
		if u.Username != "" {
			m["username"] = u.Username
		}
	}
	return m
}

func peerUserID(peer tg.PeerClass) int64 {
	if pu, ok := peer.(*tg.PeerUser); ok {
		return pu.UserID
	}
	return 0
}
