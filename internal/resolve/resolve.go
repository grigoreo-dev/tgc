// Package resolve turns user-facing chat selectors (@username, numeric ID,
// phone, fuzzy display name) into Telegram peers, using local caches first.
package resolve

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/celestix/gotgproto/types"
	"github.com/gotd/td/tg"

	"github.com/grigoreo-dev/tgc/internal/client"
	"github.com/grigoreo-dev/tgc/internal/output"
)

const dialogCacheTTLSeconds = 300

// Peer is a resolved chat/user suitable for CLI output and further API calls.
type Peer struct {
	ID         int64  `json:"id"`
	AccessHash int64  `json:"-"`
	Type       string `json:"type"` // user | group | channel
	Title      string `json:"title"`
	Username   string `json:"username,omitempty"`
}

var phoneRe = regexp.MustCompile(`^\+[0-9]{7,15}$`)

// Classify determines the selector kind: self, id, username, phone or name.
func Classify(s string) (kind, value string) {
	s = strings.TrimSpace(s)
	switch strings.ToLower(s) {
	case "me", "self", "saved", "saved messages":
		return "self", ""
	}
	if strings.HasPrefix(s, "@") {
		return "username", strings.TrimPrefix(s, "@")
	}
	// Phone before numeric id: strconv.ParseInt accepts a leading '+'.
	if phoneRe.MatchString(s) {
		return "phone", strings.TrimPrefix(s, "+")
	}
	if _, err := strconv.ParseInt(s, 10, 64); err == nil {
		return "id", s
	}
	return "name", s
}

// FuzzyMatch returns peers whose title or username contains query
// (case-insensitive). Exact title match narrows to that peer alone.
func FuzzyMatch(peers []Peer, query string) []Peer {
	q := strings.ToLower(strings.TrimSpace(query))
	var out []Peer
	for _, p := range peers {
		if strings.ToLower(p.Title) == q {
			return []Peer{p}
		}
		if strings.Contains(strings.ToLower(p.Title), q) ||
			strings.Contains(strings.ToLower(p.Username), q) {
			out = append(out, p)
		}
	}
	return out
}

// Resolve maps a selector to a Peer. Cheap paths first: numeric ID and
// username go through gotgproto's peer storage before hitting the API.
func Resolve(conn *client.Conn, selector string) (*Peer, error) {
	kind, val := Classify(selector)
	switch kind {
	case "self":
		p := peerFromUser(conn.Client.Self)
		if p == nil {
			return nil, output.Errf("not_authenticated", "cannot resolve %q: not logged in", selector)
		}
		return p, nil
	case "id":
		id, _ := strconv.ParseInt(val, 10, 64)
		return resolveByID(conn, id)
	case "username":
		chat, err := conn.Ctx.ResolveUsername(val)
		if err != nil {
			return nil, output.Errf("not_found", "cannot resolve @%s: %v", val, client.WrapErr(err))
		}
		p := peerFromEffectiveChat(chat)
		if p == nil {
			return nil, output.Errf("not_found", "cannot resolve @%s", val)
		}
		return p, nil
	case "phone":
		return resolveByPhone(conn, val)
	default:
		peers, err := Dialogs(conn, false, 0)
		if err != nil {
			return nil, err
		}
		matches := FuzzyMatch(peers, val)
		switch len(matches) {
		case 1:
			return &matches[0], nil
		case 0:
			return nil, output.Errf("not_found", "no chat matches %q; try `tgc search`", val)
		default:
			cands := make([]map[string]any, 0, len(matches))
			for _, m := range matches {
				cands = append(cands, map[string]any{"id": m.ID, "title": m.Title, "username": m.Username, "type": m.Type})
			}
			return nil, output.ErrfX("ambiguous", map[string]any{"candidates": cands},
				"%d chats match %q; use @username or id", len(matches), val)
		}
	}
}

// InputUser builds a Telegram InputUser for a resolved user peer.
func InputUser(p *Peer) (tg.InputUserClass, error) {
	if p == nil || p.Type != "user" {
		return nil, output.Errf("bad_args", "not a user peer")
	}
	return &tg.InputUser{UserID: p.ID, AccessHash: p.AccessHash}, nil
}

// InputChannel builds a Telegram InputChannel for a resolved channel or
// megagroup peer. Legacy basic groups have no channel id and yield an error.
func InputChannel(p *Peer) (tg.InputChannelClass, error) {
	if p == nil {
		return nil, output.Errf("bad_args", "peer is nil")
	}
	switch p.Type {
	case "channel":
		return &tg.InputChannel{ChannelID: plainChannelID(p.ID), AccessHash: p.AccessHash}, nil
	case "group":
		if p.AccessHash != 0 {
			return &tg.InputChannel{ChannelID: plainChannelID(p.ID), AccessHash: p.AccessHash}, nil
		}
		return nil, output.Errf("bad_args", "legacy basic group has no channel id")
	default:
		return nil, output.Errf("bad_args", "not a channel/group peer")
	}
}

// PlainChatID exposes the legacy basic-group id (positive) for a group peer.
func PlainChatID(id int64) int64 { return plainChatID(id) }

// InputPeer builds a Telegram InputPeer for the resolved peer.
func InputPeer(conn *client.Conn, p *Peer) (tg.InputPeerClass, error) {
	if p == nil {
		return nil, output.Errf("bad_args", "peer is nil")
	}
	if ip := conn.Ctx.PeerStorage.GetInputPeerById(p.ID); ip != nil {
		if _, empty := ip.(*tg.InputPeerEmpty); !empty {
			return ip, nil
		}
	}
	switch p.Type {
	case "user":
		return &tg.InputPeerUser{UserID: p.ID, AccessHash: p.AccessHash}, nil
	case "group":
		// Legacy basic groups use plain chat id; megagroups are stored as channel input.
		if p.AccessHash != 0 {
			return &tg.InputPeerChannel{ChannelID: plainChannelID(p.ID), AccessHash: p.AccessHash}, nil
		}
		return &tg.InputPeerChat{ChatID: plainChatID(p.ID)}, nil
	case "channel":
		return &tg.InputPeerChannel{ChannelID: plainChannelID(p.ID), AccessHash: p.AccessHash}, nil
	default:
		return nil, output.Errf("bad_args", "unknown peer type %q", p.Type)
	}
}

// Dialogs returns the dialog list, from the profile cache when fresh enough.
func Dialogs(conn *client.Conn, fresh bool, limit int) ([]Peer, error) {
	if !fresh {
		if peers, ok := loadDialogCache(conn.Profile.Dir, dialogCacheTTLSeconds); ok {
			return capPeers(peers, limit), nil
		}
	}
	raw, err := conn.Ctx.Raw.MessagesGetDialogs(conn.Ctx, &tg.MessagesGetDialogsRequest{
		Limit:      500,
		OffsetPeer: &tg.InputPeerEmpty{},
	})
	if err != nil {
		return nil, client.WrapErr(err)
	}
	peers := peersFromDialogs(raw)
	_ = saveDialogCache(conn.Profile.Dir, peers)
	return capPeers(peers, limit), nil
}

// PeersFromUsersChats converts Telegram users/chats lists into Peers.
func PeersFromUsersChats(users []tg.UserClass, chats []tg.ChatClass) []Peer {
	var out []Peer
	for _, u := range users {
		if p := peerFromUserClass(u); p != nil {
			out = append(out, *p)
		}
	}
	for _, c := range chats {
		if p := peerFromChatClass(c); p != nil {
			out = append(out, *p)
		}
	}
	return out
}

func capPeers(peers []Peer, limit int) []Peer {
	if limit > 0 && len(peers) > limit {
		return peers[:limit]
	}
	return peers
}

func resolveByID(conn *client.Conn, id int64) (*Peer, error) {
	ip := conn.Ctx.PeerStorage.GetInputPeerById(id)
	if _, isEmpty := ip.(*tg.InputPeerEmpty); isEmpty {
		peers, err := Dialogs(conn, false, 0)
		if err != nil {
			return nil, err
		}
		for _, p := range peers {
			if p.ID == id {
				return &p, nil
			}
		}
		return nil, output.Errf("not_found", "peer id %d is unknown to this session", id)
	}
	return peerFromInputPeer(id, ip), nil
}

func resolveByPhone(conn *client.Conn, phone string) (*Peer, error) {
	res, err := conn.Ctx.Raw.ContactsResolvePhone(conn.Ctx, phone)
	if err != nil {
		return nil, output.Errf("not_found", "cannot resolve phone +%s: %v", phone, client.WrapErr(err))
	}
	for _, u := range res.Users {
		if p := peerFromUserClass(u); p != nil {
			return p, nil
		}
	}
	return nil, output.Errf("not_found", "phone +%s did not resolve to a user", phone)
}

func peersFromDialogs(raw tg.MessagesDialogsClass) []Peer {
	switch d := raw.(type) {
	case *tg.MessagesDialogs:
		return PeersFromUsersChats(d.Users, d.Chats)
	case *tg.MessagesDialogsSlice:
		return PeersFromUsersChats(d.Users, d.Chats)
	case *tg.MessagesDialogsNotModified:
		return nil
	default:
		return nil
	}
}

func peerFromEffectiveChat(chat types.EffectiveChat) *Peer {
	if chat == nil {
		return nil
	}
	switch {
	case chat.IsAUser():
		if u, ok := chat.(*types.User); ok {
			raw := u.Raw()
			return peerFromUser(raw)
		}
	case chat.IsAChat():
		if c, ok := chat.(*types.Chat); ok {
			raw := c.Raw()
			return &Peer{
				ID:    chat.GetID(),
				Type:  "group",
				Title: raw.Title,
			}
		}
	case chat.IsAChannel():
		if c, ok := chat.(*types.Channel); ok {
			raw := c.Raw()
			typ := "channel"
			if raw.Megagroup {
				typ = "group"
			}
			return &Peer{
				ID:         chat.GetID(),
				AccessHash: raw.AccessHash,
				Type:       typ,
				Title:      raw.Title,
				Username:   raw.Username,
			}
		}
	}
	return nil
}

func peerFromInputPeer(id int64, ip tg.InputPeerClass) *Peer {
	switch p := ip.(type) {
	case *tg.InputPeerUser:
		return &Peer{ID: id, AccessHash: p.AccessHash, Type: "user", Title: ""}
	case *tg.InputPeerChat:
		return &Peer{ID: id, Type: "group", Title: ""}
	case *tg.InputPeerChannel:
		return &Peer{ID: id, AccessHash: p.AccessHash, Type: "channel", Title: ""}
	default:
		return &Peer{ID: id, Type: "user"}
	}
}

func peerFromUserClass(u tg.UserClass) *Peer {
	user, ok := u.(*tg.User)
	if !ok {
		return nil
	}
	return peerFromUser(user)
}

func peerFromUser(user *tg.User) *Peer {
	if user == nil {
		return nil
	}
	title := strings.TrimSpace(user.FirstName + " " + user.LastName)
	return &Peer{
		ID:         user.ID,
		AccessHash: user.AccessHash,
		Type:       "user",
		Title:      title,
		Username:   user.Username,
	}
}

func peerFromChatClass(c tg.ChatClass) *Peer {
	switch ch := c.(type) {
	case *tg.Chat:
		return &Peer{
			ID:    chatTDLibID(ch.ID),
			Type:  "group",
			Title: ch.Title,
		}
	case *tg.Channel:
		typ := "channel"
		if ch.Megagroup {
			typ = "group"
		}
		return &Peer{
			ID:         channelTDLibID(ch.ID),
			AccessHash: ch.AccessHash,
			Type:       typ,
			Title:      ch.Title,
			Username:   ch.Username,
		}
	default:
		return nil
	}
}

func chatTDLibID(plain int64) int64 {
	// TDLib marks basic chats as -plainID.
	if plain < 0 {
		return plain
	}
	return -plain
}

func channelTDLibID(plain int64) int64 {
	// TDLib channel/supergroup id: -100xxxxxxxxxx
	if plain < 0 {
		return plain
	}
	return -(1_000_000_000_000 + plain)
}

func plainChatID(id int64) int64 {
	if id < 0 {
		return -id
	}
	return id
}

func plainChannelID(id int64) int64 {
	if id >= 0 {
		return id
	}
	// -100xxxxxxxxxx → plain channel id
	const prefix = int64(1_000_000_000_000)
	n := -id
	if n > prefix {
		return n - prefix
	}
	return n
}
