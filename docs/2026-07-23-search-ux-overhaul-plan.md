# Search UX Overhaul Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use beads-superpowers:subagent-driven-development (recommended) or beads-superpowers:executing-plans to implement this plan task-by-task. Each Task becomes a bead (`bd create -t task --parent <epic-id>`). Steps within tasks use checkbox (`- [ ]`) syntax for human readability.

**Goal:** Replace the three confusing search surfaces with one unified `tgc search` command (peers + messages, optional `--type`, in-chat via `--chat`), removing `read --search` and `search --messages`.

**Architecture:** New `ops.Search` orchestrator fans out to two section engines — `searchPeers` (contacts.Search + local fuzzy) and `SearchMessages` (messages.SearchGlobal with kind/date flags) — or to a new in-chat engine `searchInChat` (messages.Search, extracted from `ops.Read`'s search branch). Every JSONL row carries a `result: "chat"|"message"` discriminator. CLI layer does flag validation and bot preflight.

**Tech Stack:** Go, cobra, gotd/td (`tg.*` raw RPC), existing `internal/{ops,cli,resolve,output,client}` layering.

**Spec:** `docs/2026-07-23-search-ux-overhaul-design.md`

## Global Constraints

- Breaking change is intended: `read --search` and `search --messages` are deleted with NO aliases or deprecation shims.
- Error contract: user-facing errors go through `output.Errf(code, ...)`; codes used here: `bad_args`, `bot_unsupported`.
- `--type` vocabulary is exactly: `chats|messages|user|group|channel`.
- Peer rows in search output MUST NOT include `AccessHash` (emit projected maps, not `resolve.Peer` structs).
- Default `--limit` is 20 and applies **per section**.
- Section order in default mode: chat rows first, then message rows.
- Partial failure in default (two-RPC) mode: emit surviving section + `output.Warnf`; explicit `--type`/`--chat` modes fail hard.
- All commits reference bead ids of the epic/task being executed.
- Verify with: `go build ./... && go vet ./... && go test ./...` (must pass at the end of every task).

---

### Task 1: `ops.SearchOpts` + validation

**Files:**
- Create: `internal/ops/search.go`
- Test: `internal/ops/search_test.go`

**Interfaces:**
- Produces: `type SearchOpts struct { Type, Chat, From, Since, Until string; Limit int }`, `func ValidateSearchOpts(o SearchOpts) error`, and `searchTypeValid(t string) bool`. Later tasks rely on these exact names.

**Acceptance Criteria:**
- `ValidateSearchOpts` rejects: `--type` with `--chat`; `--from` without `--chat`; unknown `--type` values. Accepts: empty opts, each valid type, chat+from.
- All errors are `output.Errf("bad_args", ...)` with actionable messages.

- [ ] **Step 1: Write the failing test**

```go
// internal/ops/search_test.go
package ops

import "testing"

func TestValidateSearchOpts(t *testing.T) {
	cases := []struct {
		name    string
		o       SearchOpts
		wantErr bool
	}{
		{"empty ok", SearchOpts{}, false},
		{"type chats ok", SearchOpts{Type: "chats"}, false},
		{"type messages ok", SearchOpts{Type: "messages"}, false},
		{"type user ok", SearchOpts{Type: "user"}, false},
		{"type group ok", SearchOpts{Type: "group"}, false},
		{"type channel ok", SearchOpts{Type: "channel"}, false},
		{"chat ok", SearchOpts{Chat: "@devops"}, false},
		{"chat+from ok", SearchOpts{Chat: "@devops", From: "@vasya"}, false},
		{"type with chat rejected", SearchOpts{Type: "messages", Chat: "@devops"}, true},
		{"from without chat rejected", SearchOpts{From: "@vasya"}, true},
		{"unknown type rejected", SearchOpts{Type: "posts"}, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := ValidateSearchOpts(c.o)
			if (err != nil) != c.wantErr {
				t.Fatalf("ValidateSearchOpts(%+v) err=%v, wantErr=%v", c.o, err, c.wantErr)
			}
		})
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/ops/ -run TestValidateSearchOpts -v`
Expected: FAIL (compile error: `SearchOpts` undefined)

- [ ] **Step 3: Write minimal implementation**

```go
// internal/ops/search.go
// Package ops: unified search (peers + messages) per
// docs/2026-07-23-search-ux-overhaul-design.md.
package ops

import (
	"github.com/grigoreo-dev/tgc/internal/output"
)

// SearchOpts controls the unified search command.
type SearchOpts struct {
	Type  string // "", "chats", "messages", "user", "group", "channel"
	Chat  string // in-chat peer selector; switches to messages.search mode
	From  string // sender selector; requires Chat
	Since string // YYYY-MM-DD or RFC3339
	Until string
	Limit int // per-section cap; 0 => 20
}

func searchTypeValid(t string) bool {
	switch t {
	case "", "chats", "messages", "user", "group", "channel":
		return true
	}
	return false
}

// ValidateSearchOpts enforces the flag compatibility matrix.
func ValidateSearchOpts(o SearchOpts) error {
	if !searchTypeValid(o.Type) {
		return output.Errf("bad_args",
			"invalid --type %q; valid values: chats|messages|user|group|channel", o.Type)
	}
	if o.Type != "" && o.Chat != "" {
		return output.Errf("bad_args",
			"--type applies to global search only; drop --type when using --chat")
	}
	if o.From != "" && o.Chat == "" {
		return output.Errf("bad_args",
			"--from requires --chat; global search cannot filter by sender")
	}
	return nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/ops/ -run TestValidateSearchOpts -v`
Expected: PASS (all subtests)

- [ ] **Step 5: Commit**

```bash
git add internal/ops/search.go internal/ops/search_test.go
git commit -m "feat(search): SearchOpts + flag validation matrix"
```

---

### Task 2: peers section engine — kind filter + projected rows

**Files:**
- Modify: `internal/ops/chats.go` (function `SearchChats`, currently at `:222`)
- Create: append to `internal/ops/search.go`
- Test: append to `internal/ops/search_test.go`

**Interfaces:**
- Consumes: existing `SearchChats(conn, query, limit) ([]resolve.Peer, error)` — signature CHANGES here.
- Produces: `SearchChats(conn *client.Conn, query, kind string, limit int) ([]resolve.Peer, error)` (kind: ""|"user"|"group"|"channel") and `func peerRow(p resolve.Peer) map[string]any` returning `{"result":"chat","id","type","title"[, "username"]}` — no AccessHash.

**Acceptance Criteria:**
- `peerRow` output contains `result:"chat"`, id/type/title, username only when non-empty, and never AccessHash.
- `SearchChats` with kind filter drops peers of other kinds (unit-tested via a pure helper `filterPeersByKind`).

- [ ] **Step 1: Write the failing test**

```go
// append to internal/ops/search_test.go
import "github.com/grigoreo-dev/tgc/internal/resolve" // add to imports

func TestPeerRow(t *testing.T) {
	p := resolve.Peer{ID: 42, AccessHash: 999, Type: "user", Title: "Anna", Username: "anna"}
	row := peerRow(p)
	if row["result"] != "chat" || row["id"] != int64(42) || row["type"] != "user" || row["title"] != "Anna" || row["username"] != "anna" {
		t.Fatalf("unexpected row: %+v", row)
	}
	if _, leaked := row["access_hash"]; leaked {
		t.Fatal("AccessHash leaked into search output")
	}
	if _, ok := peerRow(resolve.Peer{ID: 1, Type: "group", Title: "G"})["username"]; ok {
		t.Fatal("empty username must be omitted")
	}
}

func TestFilterPeersByKind(t *testing.T) {
	in := []resolve.Peer{{ID: 1, Type: "user"}, {ID: 2, Type: "group"}, {ID: 3, Type: "channel"}}
	got := filterPeersByKind(in, "group")
	if len(got) != 1 || got[0].ID != 2 {
		t.Fatalf("want only group peer, got %+v", got)
	}
	if len(filterPeersByKind(in, "")) != 3 {
		t.Fatal("empty kind must keep all")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/ops/ -run 'TestPeerRow|TestFilterPeersByKind' -v`
Expected: FAIL (undefined `peerRow`, `filterPeersByKind`)

- [ ] **Step 3: Implement**

Append to `internal/ops/search.go`:

```go
import "github.com/grigoreo-dev/tgc/internal/resolve" // add to imports

// peerRow projects a Peer into a tagged search-output row (no AccessHash).
func peerRow(p resolve.Peer) map[string]any {
	row := map[string]any{
		"result": "chat",
		"id":     p.ID,
		"type":   p.Type,
		"title":  p.Title,
	}
	if p.Username != "" {
		row["username"] = p.Username
	}
	return row
}

// filterPeersByKind keeps peers of the given kind; empty kind keeps all.
func filterPeersByKind(peers []resolve.Peer, kind string) []resolve.Peer {
	if kind == "" {
		return peers
	}
	out := make([]resolve.Peer, 0, len(peers))
	for _, p := range peers {
		if p.Type == kind {
			out = append(out, p)
		}
	}
	return out
}
```

In `internal/ops/chats.go`, change `SearchChats` signature and apply the filter **before** the limit cap:

```go
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
```

The old call site in `internal/cli/chats.go:99` (`ops.SearchChats(conn, args[0], searchLimit)`) will not compile — that is expected; it is rewritten in Task 5. To keep this task green, update it minimally now: `ops.SearchChats(conn, args[0], "", searchLimit)`.

- [ ] **Step 4: Verify**

Run: `go build ./... && go test ./internal/ops/ -run 'TestPeerRow|TestFilterPeersByKind' -v`
Expected: build OK, tests PASS

- [ ] **Step 5: Commit**

```bash
git add internal/ops/search.go internal/ops/search_test.go internal/ops/chats.go internal/cli/chats.go
git commit -m "feat(search): peer section — kind filter + AccessHash-free rows"
```

---

### Task 3: messages section engine — kind/date flags on SearchGlobal

**Files:**
- Modify: `internal/ops/chats.go` (function `SearchMessages`, currently at `:250`)
- Test: append to `internal/ops/search_test.go`

**Interfaces:**
- Consumes: `ParseDateArg` (exists in `internal/ops/messages.go`), `collectMessages`, `autofetchGlobalRichParts`, `stripRichMapKeys`.
- Produces: `SearchMessages(conn *client.Conn, query string, o SearchOpts) ([]map[string]any, error)`; each row gains `"result":"message"`. Helper `applyGlobalKind(req *tg.MessagesSearchGlobalRequest, kind string)`.

**Acceptance Criteria:**
- kind "user"→`UsersOnly`, "group"→`GroupsOnly`, "channel"→`BroadcastsOnly`; "", "chats", "messages" set nothing (unit-tested).
- `Since`/`Until` map to `MinDate`/`MaxDate` (unix); parse errors surface as existing date errors.
- Every returned row has `result:"message"`.

- [ ] **Step 1: Write the failing test**

```go
// append to internal/ops/search_test.go
import "github.com/gotd/td/tg" // add to imports

func TestApplyGlobalKind(t *testing.T) {
	cases := []struct {
		kind                    string
		users, groups, channels bool
	}{
		{"", false, false, false},
		{"chats", false, false, false},
		{"messages", false, false, false},
		{"user", true, false, false},
		{"group", false, true, false},
		{"channel", false, false, true},
	}
	for _, c := range cases {
		req := &tg.MessagesSearchGlobalRequest{}
		applyGlobalKind(req, c.kind)
		if req.UsersOnly != c.users || req.GroupsOnly != c.groups || req.BroadcastsOnly != c.channels {
			t.Fatalf("kind %q: got users=%v groups=%v broadcasts=%v",
				c.kind, req.UsersOnly, req.GroupsOnly, req.BroadcastsOnly)
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/ops/ -run TestApplyGlobalKind -v`
Expected: FAIL (undefined `applyGlobalKind`)

- [ ] **Step 3: Implement**

Append to `internal/ops/search.go`:

```go
import "github.com/gotd/td/tg" // add to imports

// applyGlobalKind maps a --type chat-kind onto SearchGlobal only-flags.
func applyGlobalKind(req *tg.MessagesSearchGlobalRequest, kind string) {
	switch kind {
	case "user":
		req.UsersOnly = true
	case "group":
		req.GroupsOnly = true
	case "channel":
		req.BroadcastsOnly = true
	}
}
```

Rewrite `SearchMessages` in `internal/ops/chats.go`:

```go
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
```

- [ ] **Step 4: Verify**

Run: `go build ./... && go test ./internal/ops/ -run TestApplyGlobalKind -v`
Expected: build fails ONLY at `internal/cli/chats.go` call site `ops.SearchMessages(conn, args[0], searchLimit)` — patch it minimally: `ops.SearchMessages(conn, args[0], ops.SearchOpts{Limit: searchLimit})`. Then build OK, test PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/ops/search.go internal/ops/search_test.go internal/ops/chats.go internal/cli/chats.go
git commit -m "feat(search): messages section — kind + date flags on SearchGlobal"
```

---

### Task 4: in-chat engine + `ops.Search` orchestrator

**Files:**
- Modify: `internal/ops/search.go`
- Test: append to `internal/ops/search_test.go`

**Interfaces:**
- Consumes: `resolve.Resolve`, `resolve.InputPeer`, `collectMessages`, `autofetchRichParts`, `stripRichMapKeys`, `ParseDateArg`, `SearchChats` (Task 2), `SearchMessages` (Task 3), `ValidateSearchOpts` (Task 1), `peerRow` (Task 2).
- Produces: `Search(conn *client.Conn, query string, o SearchOpts) ([]map[string]any, error)` — the single entry point the CLI calls. Internal `searchInChat(conn, query string, o SearchOpts) ([]map[string]any, error)`.

**Acceptance Criteria:**
- `--chat` mode routes to `messages.Search` with optional `FromID`, `MinDate`/`MaxDate`; rows tagged `result:"message"`.
- Default mode returns chat rows then message rows; if exactly one section RPC fails, the other is returned plus `output.Warnf("search_partial", ...)`; if both fail, error out.
- `Type: "chats"` → peers only; `Type: "messages"` → messages only; kind types → both sections filtered.

- [ ] **Step 1: Write the failing test**

Pure-logic seam: extract section planning into `searchSections(o SearchOpts) (wantChats, wantMessages bool)` and test it (network paths are covered by e2e in Task 7).

```go
// append to internal/ops/search_test.go
func TestSearchSections(t *testing.T) {
	cases := []struct {
		typ            string
		chats, message bool
	}{
		{"", true, true},
		{"chats", true, false},
		{"messages", false, true},
		{"user", true, true},
		{"group", true, true},
		{"channel", true, true},
	}
	for _, c := range cases {
		gc, gm := searchSections(SearchOpts{Type: c.typ})
		if gc != c.chats || gm != c.message {
			t.Fatalf("type %q: got (%v,%v), want (%v,%v)", c.typ, gc, gm, c.chats, c.message)
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/ops/ -run TestSearchSections -v`
Expected: FAIL (undefined `searchSections`)

- [ ] **Step 3: Implement**

Append to `internal/ops/search.go` (add imports `time`, `github.com/grigoreo-dev/tgc/internal/client`):

```go
// searchSections reports which sections a --type selects.
func searchSections(o SearchOpts) (wantChats, wantMessages bool) {
	switch o.Type {
	case "chats":
		return true, false
	case "messages":
		return false, true
	default: // "", user, group, channel
		return true, true
	}
}

// peerKindFilter returns the chat-kind filter for the peers section.
func peerKindFilter(o SearchOpts) string {
	switch o.Type {
	case "user", "group", "channel":
		return o.Type
	}
	return ""
}

// searchInChat searches messages inside one chat via messages.search.
func searchInChat(conn *client.Conn, query string, o SearchOpts) ([]map[string]any, error) {
	peer, err := resolve.Resolve(conn, o.Chat)
	if err != nil {
		return nil, err
	}
	ip, err := resolve.InputPeer(conn, peer)
	if err != nil {
		return nil, err
	}
	limit := o.Limit
	if limit == 0 {
		limit = 20
	}
	req := &tg.MessagesSearchRequest{
		Peer: ip, Q: query, Filter: &tg.InputMessagesFilterEmpty{}, Limit: limit,
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
	res, err := conn.Ctx.Raw.MessagesSearch(conn.Ctx, req)
	if err != nil {
		return nil, client.WrapErr(err)
	}
	maps := collectMessages(res, peer.ID, time.Time{})
	autofetchRichParts(conn, ip, res, maps)
	stripRichMapKeys(maps)
	for _, m := range maps {
		m["result"] = "message"
	}
	return maps, nil
}

// Search is the unified entry point: peers + messages by default, one
// section via --type, or in-chat via --chat.
func Search(conn *client.Conn, query string, o SearchOpts) ([]map[string]any, error) {
	if err := ValidateSearchOpts(o); err != nil {
		return nil, err
	}
	if o.Chat != "" {
		return searchInChat(conn, query, o)
	}
	wantChats, wantMessages := searchSections(o)
	var rows []map[string]any
	var chatErr, msgErr error
	if wantChats {
		peers, err := SearchChats(conn, query, peerKindFilter(o), o.Limit)
		if err != nil {
			chatErr = err
		} else {
			for _, p := range peers {
				rows = append(rows, peerRow(p))
			}
		}
	}
	if wantMessages {
		maps, err := SearchMessages(conn, query, o)
		if err != nil {
			msgErr = err
		} else {
			rows = append(rows, maps...)
		}
	}
	// Single-section modes fail hard; dual mode degrades with a warning.
	if wantChats && !wantMessages && chatErr != nil {
		return nil, chatErr
	}
	if wantMessages && !wantChats && msgErr != nil {
		return nil, msgErr
	}
	if chatErr != nil && msgErr != nil {
		return nil, msgErr
	}
	if chatErr != nil {
		output.Warnf("search_partial", "chat section failed: %v", chatErr)
	}
	if msgErr != nil {
		output.Warnf("search_partial", "message section failed: %v", msgErr)
	}
	return rows, nil
}
```

- [ ] **Step 4: Verify**

Run: `go build ./... && go vet ./... && go test ./internal/ops/ -v -run 'TestValidateSearchOpts|TestPeerRow|TestFilterPeersByKind|TestApplyGlobalKind|TestSearchSections'`
Expected: all PASS

- [ ] **Step 5: Commit**

```bash
git add internal/ops/search.go internal/ops/search_test.go
git commit -m "feat(search): in-chat engine + unified Search orchestrator"
```

---

### Task 5: CLI rewrite — new `search`, bot preflight

**Files:**
- Modify: `internal/cli/chats.go` (`searchCmd` at `:79-108`, flag wiring in `init()` at `:110-118`)

**Interfaces:**
- Consumes: `ops.Search`, `ops.SearchOpts` (Task 4); `conn.Profile.Type` (`"bot"` marks bot profiles, `internal/config/config.go:24`).
- Produces: final CLI surface. `searchMsgs` variable and `--messages` flag deleted.

**Acceptance Criteria:**
- `tgc search <q>` with flags `--type/--chat/--from/--since/--until/--limit`; `--messages` gone.
- Bot profile → `bot_unsupported` error before any RPC.
- Help text: `Search chats and messages; --type to narrow, --chat to search inside one chat`.

- [ ] **Step 1: Rewrite searchCmd**

Replace the `searchMsgs`/`searchLimit` var block and `searchCmd` in `internal/cli/chats.go`:

```go
var searchOpts ops.SearchOpts

var searchCmd = &cobra.Command{
	Use:   "search <query>",
	Short: "Search chats and messages; --type to narrow, --chat to search inside one chat",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := ops.ValidateSearchOpts(searchOpts); err != nil {
			return err // bad_args before connecting
		}
		conn, err := client.Connect(ProfileName())
		if err != nil {
			return err
		}
		defer conn.Close()
		if conn.Profile.Type == "bot" {
			return output.Errf("bot_unsupported",
				"search is not available for bot accounts")
		}
		rows, err := ops.Search(conn, args[0], searchOpts)
		if err != nil {
			return err
		}
		for _, r := range rows {
			output.Emit(r)
		}
		return nil
	},
}
```

In `init()` replace the two old search flag lines with:

```go
searchCmd.Flags().StringVar(&searchOpts.Type, "type", "", "narrow results: chats|messages|user|group|channel")
searchCmd.Flags().StringVar(&searchOpts.Chat, "chat", "", "search inside one chat (peer selector)")
searchCmd.Flags().StringVar(&searchOpts.From, "from", "", "only from this sender (requires --chat)")
searchCmd.Flags().StringVar(&searchOpts.Since, "since", "", "start date (YYYY-MM-DD or RFC3339)")
searchCmd.Flags().StringVar(&searchOpts.Until, "until", "", "end date (YYYY-MM-DD or RFC3339)")
searchCmd.Flags().IntVar(&searchOpts.Limit, "limit", 20, "max results per section")
```

- [ ] **Step 2: Verify**

Run: `go build ./... && go run ./cmd/tgc search --help`
Expected: new flags listed, no `--messages`.

Run: `go run ./cmd/tgc search x --from y 2>&1 | head -2`
Expected: `bad_args` error mentioning `--from requires --chat` (validation fires before connect).

- [ ] **Step 3: Commit**

```bash
git add internal/cli/chats.go
git commit -m "feat(search)!: unified search command; drop --messages"
```

---

### Task 6: remove `read --search`

**Files:**
- Modify: `internal/cli/read.go:68` (delete flag), `internal/ops/messages.go` (`ReadOpts` `:31-40`, `Read` search branch `:65`)

**Interfaces:**
- Consumes: nothing new.
- Produces: `ReadOpts` without `Search` field. `Read`'s `messages.search` branch KEPT for `From != ""` (with empty `Q`) — `read --from` still works.

**Acceptance Criteria:**
- `tgc read x --search y` → cobra `unknown flag` error.
- `read --from` behavior unchanged (routes through `messages.Search` with `Q: ""`).
- `go test ./...` passes; no `ReadOpts.Search` references remain (`rg -n 'ReadOpts' -A2` shows no Search).

- [ ] **Step 1: Delete the flag**

In `internal/cli/read.go` remove line:

```go
readCmd.Flags().StringVar(&readOpts.Search, "search", "", "server-side search within chat")
```

- [ ] **Step 2: Update ReadOpts and Read**

In `internal/ops/messages.go`: delete the `Search string` field from `ReadOpts`; in `Read` change the branch condition and request:

```go
	case o.From != "":
		req := &tg.MessagesSearchRequest{
			Peer: ip, Q: "", Filter: &tg.InputMessagesFilterEmpty{}, Limit: o.Limit,
		}
```

(the rest of the branch is unchanged). Update the `Read` doc comment: `// Read returns chat messages newest-first. From routes through messages.search; otherwise messages.getHistory with id/date windows.`

- [ ] **Step 3: Verify**

Run: `go build ./... && go test ./... && rg -n 'o\.Search|ReadOpts' internal/ | rg -v '_test'`
Expected: build+tests green; no `Search` field references.

- [ ] **Step 4: Commit**

```bash
git add internal/cli/read.go internal/ops/messages.go
git commit -m "feat(read)!: remove --search; search lives in tgc search --chat"
```

---

### Task 7: e2e smoke + docs

**Files:**
- Modify: `README.md` (command table `:122`, bot limitations `:201`), `README.ru.md` (mirror rows), `docs/integration-checklist.md` (search entries)

**Interfaces:**
- Consumes: final CLI from Tasks 5–6.

**Acceptance Criteria:**
- README command tables describe: `search` (unified, `--type`, `--chat`); no `read --search`, no `--messages` anywhere (`rg -n '\-\-messages|read --search' README* docs/integration-checklist.md` → empty).
- Live smoke (own profile; skip if no session available, report as blocked): default search returns both `"result":"chat"` and `"result":"message"` rows; `--type chats` returns only chat rows; `--chat` works; literal query `chats` searches the word.

- [ ] **Step 1: e2e smoke (uses per-project `./.tgc` profile)**

```bash
go build -o /tmp/tgc-e2e ./cmd/tgc
/tmp/tgc-e2e search "test" --limit 3            # expect chat+message rows, tagged
/tmp/tgc-e2e search "test" --type chats --limit 3
/tmp/tgc-e2e search "test" --type messages --limit 3
/tmp/tgc-e2e search "chats" --limit 3            # literal word, no reserved-word trap
/tmp/tgc-e2e search "test" --chat "Saved Messages" --limit 3
/tmp/tgc-e2e search x --type posts; echo "exit=$?"   # bad_args
```

Expected: JSONL rows with `result` field; last command prints `bad_args` error.

- [ ] **Step 2: Update docs**

`README.md` command table — replace the `search` row with:

```markdown
| `search`   | Search chats and messages (both by default); `--type chats|messages|user|group|channel` to narrow; `--chat <peer>` to search inside one chat (`--from`, `--since`, `--until`). |
```

Remove any `read` row mention of `--search`. Update the bot section (`:201`) to state `search` is unavailable for bots. Mirror both edits in `README.ru.md`. Update `docs/integration-checklist.md` search entries to the new surface.

- [ ] **Step 3: Verify**

Run: `rg -n '\-\-messages|read --search' README.md README.ru.md docs/integration-checklist.md`
Expected: no matches. `go build ./... && go vet ./... && go test ./...` green.

- [ ] **Step 4: Commit**

```bash
git add README.md README.ru.md docs/integration-checklist.md
git commit -m "docs(search): unified search surface; drop old search docs"
```

---

## Task dependency order

Task 1 → Task 2 → Task 3 → Task 4 → Task 5 → Task 6 → Task 7 (strictly sequential; Tasks 2–4 share `search.go`/`chats.go`, 5–6 depend on 4, 7 documents the final surface).
