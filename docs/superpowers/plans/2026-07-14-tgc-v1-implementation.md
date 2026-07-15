# tgc v1 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Go CLI `tgc` — agent-first Telegram-клиент (user-bot + bot режимы): auth/профили, чаты, сообщения, файлы.

**Architecture:** Один бинарник на cobra. Ядро — gotgproto (сессии, peer cache, хелперы) поверх gotd/td; raw `client.API()` для того, чего нет в обёртках. Каждая команда — one-shot: поднимает клиент с `NoUpdates: true`, выполняет вызов, печатает компактный JSONL в stdout. Профили — директории с SQLite-файлом сессии.

**Tech Stack:** Go 1.25+, github.com/celestix/gotgproto (beta), github.com/gotd/td, github.com/gotd/contrib (floodwait), cobra, glebarez/sqlite (pure-Go GORM driver, без CGO), BurntSushi/toml.

**Spec:** `docs/superpowers/specs/2026-07-13-tgc-telegram-cli-design.md` (в этом репозитории).

## Global Constraints

- Репозиторий: `grigoreo-dev/tgc`, подключается как submodule `projects/tgc` meta-репо.
- Имя бинарника: `tgc`. Module path: `github.com/grigoreo-dev/tgc`.
- **Контракт вывода:** stdout — только результат, компактный JSON/JSONL (`json.Marshal`, без отступов); списки — JSONL. Всё остальное (ошибки, логи, прогресс) — stderr. Ошибки: JSON `{"error":"<code>","message":"..."}` в stderr + exit code 1. `--pretty` — человекочитаемо; цвета гасятся при non-TTY и `NO_COLOR`.
- API-креденшелы: только пользовательские, env `TGC_API_ID`/`TGC_API_HASH` приоритетнее конфига. Ничего не вшивать.
- Профили: `~/.config/tgc/` (уважать `XDG_CONFIG_HOME`), выбор через `--profile` / env `TGC_PROFILE`.
- Никакого CGO: sqlite — только `github.com/glebarez/sqlite`.
- Все list-команды имеют `--limit`.
- Селектор чата единый во всех командах: `@username` | ID | телефон | fuzzy-имя.
- FLOOD_WAIT ≤ 30с — ждать прозрачно (floodwait.Waiter); больше — ошибка `{"error":"flood_wait","retry_after":X}`.
- **gotd pin:** stay on whatever gotgproto pulls (currently `gotd/td v0.153.0`, Layer 227). Before Task 8, one optional bump of **gotd only** to latest; gate = `go build ./... && go test ./...`; on failure roll back to 0.153. Do **not** bump gotgproto unless blocked. RichMessage types already exist in 0.153 — no upgrade required for Task 11.
- **bot_unsupported preflight (profile type `bot`):** fail before RPC with `{"error":"bot_unsupported",...}` for: `chats` (dialogs), `search` without `--messages` (contacts/dialogs), phone resolve, fuzzy-name resolve. Allowed: auth*, send/edit/delete/forward/download, read/context, info by `@username`|id, members (channel peers), `search --messages`, resolve `@username`|id. Reactive `WrapErr(BOT_METHOD_INVALID)` remains a safety net (do **not** map broad `*_INVALID`).
- **Session security:** export/import write files `0600`; never log session strings; README warns sessions = account credentials; no encryption-at-rest in v1.
- **Media defaults (Task 10):** images (jpg/png/webp/gif) send as **photo** by default; `--as-document` forces document. Album max 10 → `bad_args` if more; mixed types → pass API error through. Download default path: `$TGC_DOWNLOAD_DIR` if set, else `~/.tgc/downloads/<file_id>/<original_filename>` (photo id for photos); `-o` overrides (file or dir); conflicts → `uniquePath`; `--stdout` = raw bytes.
- **read pagination:** `--after N` → `getHistory.MinID=N`; `--before N` → `MaxID=N` (+ `OffsetID=N` when used as cursor). One RPC = one page. No AddOffset hack.
- **Task 13 live integration is required** to close v1: use `.env` (`TGC_API_ID`/`TGC_API_HASH`/`TGC_BOT_TOKEN`) + interactive user-bot login when needed. Never commit or log secrets.
- Каждый коммит — работающее состояние: `go build ./... && go vet ./... && go test ./...` зелёные.
- **Язык репозитория tgc — строго английский**: код, комментарии, тесты, коммиты (conventional commits), README, docs/, тексты ошибок и help CLI. Единственное исключение — `README.ru.md` (русская версия README, добавляется в Task 13). Никакого русского в идентификаторах, строках, фикстурах тестов.
- **Git workflow: main защищён, все изменения — только через PR.** Каждая задача выполняется в ветке `task/<N>-<slug>`; в конце: push → `gh pr create --fill` → `gh pr merge --squash --auto --delete-branch` (мержится автоматически после зелёного CI) → `git checkout main && git pull`. Прямой пуш в main запрещён (единственное исключение — бутстрап-коммит Task 1 до включения защиты ветки).
- Если PR не мержится из-за красного CI — чини в той же ветке, не обходи защиту.

## File Structure (итоговая)

```
projects/tgc/
├── cmd/tgc/main.go              # entrypoint → internal/cli.Execute()
├── internal/
│   ├── cli/                     # cobra-команды (тонкие: флаги → вызов ядра → вывод)
│   │   ├── root.go              # rootCmd, глобальные флаги --profile/--pretty
│   │   ├── auth.go              # auth login/export/import/list/logout
│   │   ├── chats.go             # chats, info, members, search
│   │   ├── read.go              # read, context
│   │   ├── send.go              # send, edit, delete, forward
│   │   └── download.go          # download
│   ├── config/                  # config.toml, профили, env
│   │   ├── config.go
│   │   └── config_test.go
│   ├── client/                  # bootstrap gotgproto + floodwait
│   │   └── client.go
│   ├── output/                  # JSONL/pretty-принтеры, контракт ошибок
│   │   ├── output.go
│   │   └── output_test.go
│   ├── resolve/                 # универсальный селектор + кэш диалогов
│   │   ├── resolve.go
│   │   ├── cache.go
│   │   └── resolve_test.go
│   ├── markup/                  # Markdown → entities / RichMessage + фолбэк
│   │   ├── markdown.go
│   │   ├── rich.go
│   │   └── markdown_test.go
│   └── ops/                     # операции ядра (переиспользуются будущими серверными режимами)
│       ├── messages.go          # read/send/edit/delete/forward/context
│       ├── media.go             # download/upload/albums
│       ├── chats.go             # dialogs/info/members/search
│       └── messages_test.go
├── go.mod
├── README.md
└── .github/workflows/ci.yml
```

---

### Task 1: Репозиторий, скелет, output-пакет (контракт вывода)

**Files:**
- Create: `projects/tgc/go.mod`, `projects/tgc/cmd/tgc/main.go`, `projects/tgc/internal/cli/root.go`, `projects/tgc/internal/output/output.go`, `projects/tgc/internal/output/output_test.go`, `projects/tgc/.github/workflows/ci.yml`, `projects/tgc/.gitignore`, `projects/tgc/README.md`

**Interfaces:**
- Produces: `output.Emit(v any)` — одна JSON-строка в stdout; `output.EmitAll(items []any)` — JSONL; `output.Fail(code, message string, extra map[string]any)` — JSON в stderr + `os.Exit(1)`; `output.Errf(code, format string, args ...any) error` — типизированная ошибка `*output.Error` с полями `Code`, `Message`, `Extra map[string]any`; `cli.Execute()`; глобальные флаги `--profile`, `--pretty` доступны через `cli.ProfileName()`, `cli.Pretty()`.

- [ ] **Step 1: Создать репозиторий и submodule**

```bash
cd /root/workspace
gh repo create grigoreo-dev/tgc --private --description "Agent-first Telegram CLI (Go, MTProto)" --clone=false
git submodule add https://github.com/grigoreo-dev/tgc.git projects/tgc
cd projects/tgc
git checkout -b main 2>/dev/null || true
```

Expected: пустой репозиторий создан, submodule добавлен.

- [ ] **Step 2: go.mod + main.go + root.go**

`projects/tgc/go.mod` — начать с:

```bash
cd /root/workspace/projects/tgc
go mod init github.com/grigoreo-dev/tgc
```

`projects/tgc/cmd/tgc/main.go`:

```go
package main

import "github.com/grigoreo-dev/tgc/internal/cli"

func main() {
	cli.Execute()
}
```

`projects/tgc/internal/cli/root.go`:

```go
package cli

import (
	"os"

	"github.com/grigoreo-dev/tgc/internal/output"
	"github.com/spf13/cobra"
)

var (
	flagProfile string
	flagPretty  bool
)

var rootCmd = &cobra.Command{
	Use:           "tgc",
	Short:         "Agent-first Telegram CLI",
	Long:          "tgc is a Telegram client for terminals and AI agents.\nDefault output is compact JSONL; use --pretty for humans.",
	SilenceUsage:  true,
	SilenceErrors: true,
}

// ProfileName returns the selected profile: --profile flag, TGC_PROFILE env, or "".
func ProfileName() string {
	if flagProfile != "" {
		return flagProfile
	}
	return os.Getenv("TGC_PROFILE")
}

// Pretty reports whether human-readable output was requested.
func Pretty() bool { return flagPretty }

func Execute() {
	rootCmd.PersistentFlags().StringVar(&flagProfile, "profile", "", "profile name (default from config or TGC_PROFILE)")
	rootCmd.PersistentFlags().BoolVar(&flagPretty, "pretty", false, "human-readable output")
	if err := rootCmd.Execute(); err != nil {
		output.FailErr(err)
	}
}
```

- [ ] **Step 3: Написать падающие тесты output-пакета**

`projects/tgc/internal/output/output_test.go`:

```go
package output

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestEmitCompactJSON(t *testing.T) {
	var buf bytes.Buffer
	stdout = &buf
	defer func() { stdout = defaultStdout }()

	Emit(map[string]any{"id": 1, "text": "hi"})

	got := buf.String()
	if strings.Contains(got, "  ") || strings.Count(got, "\n") != 1 {
		t.Fatalf("want single compact line, got %q", got)
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(got), &m); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
}

func TestEmitAllJSONL(t *testing.T) {
	var buf bytes.Buffer
	stdout = &buf
	defer func() { stdout = defaultStdout }()

	EmitAll([]any{map[string]any{"a": 1}, map[string]any{"a": 2}})

	lines := strings.Split(strings.TrimRight(buf.String(), "\n"), "\n")
	if len(lines) != 2 {
		t.Fatalf("want 2 JSONL lines, got %d: %q", len(lines), buf.String())
	}
	for _, ln := range lines {
		var m map[string]any
		if err := json.Unmarshal([]byte(ln), &m); err != nil {
			t.Fatalf("line %q not JSON: %v", ln, err)
		}
	}
}

func TestErrfProducesStructuredError(t *testing.T) {
	err := Errf("flood_wait", "wait %d seconds", 42)
	var e *Error
	if !AsError(err, &e) {
		t.Fatal("Errf must return *output.Error")
	}
	if e.Code != "flood_wait" || e.Message != "wait 42 seconds" {
		t.Fatalf("unexpected: %+v", e)
	}
}

func TestErrorJSONShape(t *testing.T) {
	e := &Error{Code: "not_found", Message: "no such chat", Extra: map[string]any{"query": "vasya"}}
	b, _ := json.Marshal(e.jsonBody())
	var m map[string]any
	_ = json.Unmarshal(b, &m)
	if m["error"] != "not_found" || m["message"] != "no such chat" || m["query"] != "vasya" {
		t.Fatalf("bad shape: %s", b)
	}
}
```

- [ ] **Step 4: Запустить тесты — убедиться, что падают**

Run: `cd /root/workspace/projects/tgc && go test ./internal/output/`
Expected: FAIL (пакет не существует / функции не определены).

- [ ] **Step 5: Реализовать output-пакет**

`projects/tgc/internal/output/output.go`:

```go
// Package output implements the tgc output contract:
// stdout carries only results (compact JSON / JSONL), everything else
// goes to stderr; errors are structured JSON on stderr + exit code 1.
package output

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
)

var (
	defaultStdout io.Writer = os.Stdout
	stdout        io.Writer = os.Stdout
	stderr        io.Writer = os.Stderr
)

// Error is a structured tgc error. Code is a stable machine-readable
// identifier (e.g. "flood_wait", "not_found", "ambiguous", "bot_unsupported").
type Error struct {
	Code    string
	Message string
	Extra   map[string]any
}

func (e *Error) Error() string { return e.Code + ": " + e.Message }

func (e *Error) jsonBody() map[string]any {
	m := map[string]any{"error": e.Code, "message": e.Message}
	for k, v := range e.Extra {
		m[k] = v
	}
	return m
}

// Errf creates a structured error with a code and printf-style message.
func Errf(code, format string, args ...any) error {
	return &Error{Code: code, Message: fmt.Sprintf(format, args...)}
}

// ErrfX is Errf with extra JSON fields.
func ErrfX(code string, extra map[string]any, format string, args ...any) error {
	return &Error{Code: code, Message: fmt.Sprintf(format, args...), Extra: extra}
}

// AsError unwraps err into *Error.
func AsError(err error, target **Error) bool { return errors.As(err, target) }

// Emit writes one compact JSON line to stdout.
func Emit(v any) {
	b, err := json.Marshal(v)
	if err != nil {
		FailErr(fmt.Errorf("marshal output: %w", err))
	}
	fmt.Fprintln(stdout, string(b))
}

// EmitAll writes items as JSONL (one compact JSON object per line).
func EmitAll(items []any) {
	for _, it := range items {
		Emit(it)
	}
}

// FailErr prints a structured JSON error to stderr and exits 1.
// Unknown errors get code "internal".
func FailErr(err error) {
	var e *Error
	if !errors.As(err, &e) {
		e = &Error{Code: "internal", Message: err.Error()}
	}
	b, _ := json.Marshal(e.jsonBody())
	fmt.Fprintln(stderr, string(b))
	os.Exit(1)
}

// Fail is a convenience wrapper: structured error to stderr, exit 1.
func Fail(code, message string, extra map[string]any) {
	FailErr(&Error{Code: code, Message: message, Extra: extra})
}
```

- [ ] **Step 6: Прогнать тесты**

Run: `cd /root/workspace/projects/tgc && go get github.com/spf13/cobra@latest && go mod tidy && go build ./... && go vet ./... && go test ./...`
Expected: PASS.

- [ ] **Step 7: CI, .gitignore, README-заглушка**

`projects/tgc/.github/workflows/ci.yml`:

```yaml
name: CI
on:
  push:
    branches: [main]
  pull_request:
concurrency:
  group: ${{ github.workflow }}-${{ github.ref }}
  cancel-in-progress: true
jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: '1.25'
      - run: go build ./...
      - run: go vet ./...
      - run: go test ./...
```

`projects/tgc/.gitignore`:

```
tgc
/dist/
*.db
```

`projects/tgc/README.md`:

```markdown
# tgc

Agent-first Telegram CLI (Go, MTProto via gotgproto/gotd).

Compact JSONL on stdout, structured JSON errors on stderr, `--pretty` for humans.

Status: v1 in development.

- Design: `docs/superpowers/specs/2026-07-13-tgc-telegram-cli-design.md`
- Plan: `docs/superpowers/plans/2026-07-14-tgc-v1-implementation.md`
```

- [ ] **Step 8: Bootstrap commit (единственный прямой пуш в main)**

```bash
cd /root/workspace/projects/tgc
git add -A
git commit -m "feat: project scaffold, output contract package, CI"
git push -u origin main
```

- [ ] **Step 9: Включить защиту main (PR-only + зелёный CI)**

```bash
gh api -X PUT repos/grigoreo-dev/tgc/branches/main/protection \
  -H "Accept: application/vnd.github+json" \
  --input - << 'JSON'
{
  "required_status_checks": {"strict": true, "checks": [{"context": "test"}]},
  "enforce_admins": false,
  "required_pull_request_reviews": {"required_approving_review_count": 0},
  "restrictions": null,
  "allow_force_pushes": false,
  "allow_deletions": false
}
JSON
gh api repos/grigoreo-dev/tgc/branches/main/protection --jq '.required_status_checks.checks'
```

Expected: защита включена, check `test` обязателен. С этого момента все изменения — только через PR.

---

### Task 2: Config и профили

**Files:**
- Create: `projects/tgc/internal/config/config.go`, `projects/tgc/internal/config/config_test.go`

**Interfaces:**
- Consumes: `output.Errf`.
- Produces:
  - `config.Dir() string` — корень конфига (`$XDG_CONFIG_HOME/tgc` или `~/.config/tgc`), переопределяется env `TGC_CONFIG_DIR` (для тестов).
  - `type config.Config struct { DefaultProfile string \`toml:"default_profile"\`; APIID int \`toml:"api_id"\`; APIHash string \`toml:"api_hash"\` }`
  - `config.Load() (*Config, error)` — читает `config.toml`, отсутствие файла — не ошибка (пустой Config).
  - `config.Save(c *Config) error`
  - `type config.Profile struct { Name string; Dir string; SessionPath string; Type string }` (`Type`: `"user"|"bot"|""`)
  - `config.ResolveProfile(explicit string) (*Profile, error)` — explicit → `TGC_PROFILE` (обрабатывается вызывающим) → `DefaultProfile` из конфига → `"default"`. Создаёт директорию профиля. `SessionPath = <dir>/profiles/<name>/session.db`. `Type` читается из маркер-файла `<dir>/profiles/<name>/type`.
  - `config.SetProfileType(p *Profile, t string) error` — пишет маркер-файл.
  - `config.ListProfiles() ([]Profile, error)`
  - `config.APICredentials(c *Config) (int, string, error)` — env `TGC_API_ID`/`TGC_API_HASH` → конфиг; если нет ни там ни там — `output.Errf("no_api_credentials", ...)` с подсказкой про my.telegram.org.
  - `config.DeleteProfile(name string) error` — удаляет директорию профиля.

- [ ] **Step 1: Написать падающие тесты**

`projects/tgc/internal/config/config_test.go`:

```go
package config

import (
	"os"
	"path/filepath"
	"testing"
)

func withTempConfig(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("TGC_CONFIG_DIR", dir)
	return dir
}

func TestDirRespectsEnvOverride(t *testing.T) {
	dir := withTempConfig(t)
	if Dir() != dir {
		t.Fatalf("want %s, got %s", dir, Dir())
	}
}

func TestLoadMissingConfigIsEmpty(t *testing.T) {
	withTempConfig(t)
	c, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if c.DefaultProfile != "" || c.APIID != 0 {
		t.Fatalf("want zero config, got %+v", c)
	}
}

func TestSaveLoadRoundtrip(t *testing.T) {
	withTempConfig(t)
	if err := Save(&Config{DefaultProfile: "personal", APIID: 42, APIHash: "abc"}); err != nil {
		t.Fatal(err)
	}
	c, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if c.DefaultProfile != "personal" || c.APIID != 42 || c.APIHash != "abc" {
		t.Fatalf("roundtrip mismatch: %+v", c)
	}
}

func TestResolveProfileDefaultChain(t *testing.T) {
	dir := withTempConfig(t)

	p, err := ResolveProfile("")
	if err != nil {
		t.Fatal(err)
	}
	if p.Name != "default" {
		t.Fatalf(`want "default", got %q`, p.Name)
	}
	want := filepath.Join(dir, "profiles", "default", "session.db")
	if p.SessionPath != want {
		t.Fatalf("want %s, got %s", want, p.SessionPath)
	}
	if _, err := os.Stat(filepath.Dir(p.SessionPath)); err != nil {
		t.Fatalf("profile dir must be created: %v", err)
	}

	_ = Save(&Config{DefaultProfile: "work"})
	p, _ = ResolveProfile("")
	if p.Name != "work" {
		t.Fatalf(`config default: want "work", got %q`, p.Name)
	}

	p, _ = ResolveProfile("explicit")
	if p.Name != "explicit" {
		t.Fatalf(`explicit wins: got %q`, p.Name)
	}
}

func TestProfileTypeMarker(t *testing.T) {
	withTempConfig(t)
	p, _ := ResolveProfile("mybot")
	if p.Type != "" {
		t.Fatalf("new profile type must be empty, got %q", p.Type)
	}
	if err := SetProfileType(p, "bot"); err != nil {
		t.Fatal(err)
	}
	p2, _ := ResolveProfile("mybot")
	if p2.Type != "bot" {
		t.Fatalf(`want "bot", got %q`, p2.Type)
	}
}

func TestListProfiles(t *testing.T) {
	withTempConfig(t)
	_, _ = ResolveProfile("a")
	_, _ = ResolveProfile("b")
	list, err := ListProfiles()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 {
		t.Fatalf("want 2, got %d", len(list))
	}
}

func TestAPICredentialsEnvWinsOverConfig(t *testing.T) {
	withTempConfig(t)
	_ = Save(&Config{APIID: 1, APIHash: "cfg"})
	t.Setenv("TGC_API_ID", "99")
	t.Setenv("TGC_API_HASH", "env")
	c, _ := Load()
	id, hash, err := APICredentials(c)
	if err != nil {
		t.Fatal(err)
	}
	if id != 99 || hash != "env" {
		t.Fatalf("env must win: %d %s", id, hash)
	}
}

func TestAPICredentialsMissing(t *testing.T) {
	withTempConfig(t)
	c, _ := Load()
	_, _, err := APICredentials(c)
	if err == nil {
		t.Fatal("want error when no credentials anywhere")
	}
}
```

- [ ] **Step 2: Запустить — убедиться, что падают**

Run: `cd /root/workspace/projects/tgc && go test ./internal/config/`
Expected: FAIL (пакет не существует).

- [ ] **Step 3: Реализовать config.go**

```go
// Package config manages tgc configuration and named profiles.
package config

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/grigoreo-dev/tgc/internal/output"
)

type Config struct {
	DefaultProfile string `toml:"default_profile"`
	APIID          int    `toml:"api_id"`
	APIHash        string `toml:"api_hash"`
}

type Profile struct {
	Name        string
	Dir         string
	SessionPath string
	Type        string // "user", "bot", or "" (not logged in yet)
}

// Dir returns the tgc config root, honoring TGC_CONFIG_DIR and XDG_CONFIG_HOME.
func Dir() string {
	if d := os.Getenv("TGC_CONFIG_DIR"); d != "" {
		return d
	}
	if x := os.Getenv("XDG_CONFIG_HOME"); x != "" {
		return filepath.Join(x, "tgc")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "tgc")
}

func configPath() string { return filepath.Join(Dir(), "config.toml") }

func Load() (*Config, error) {
	var c Config
	b, err := os.ReadFile(configPath())
	if err != nil {
		if os.IsNotExist(err) {
			return &c, nil
		}
		return nil, err
	}
	if err := toml.Unmarshal(b, &c); err != nil {
		return nil, output.Errf("bad_config", "cannot parse %s: %v", configPath(), err)
	}
	return &c, nil
}

func Save(c *Config) error {
	if err := os.MkdirAll(Dir(), 0o700); err != nil {
		return err
	}
	f, err := os.OpenFile(configPath(), os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	return toml.NewEncoder(f).Encode(c)
}

// ResolveProfile picks a profile: explicit name, then config default_profile,
// then "default". It creates the profile directory.
func ResolveProfile(explicit string) (*Profile, error) {
	name := explicit
	if name == "" {
		c, err := Load()
		if err != nil {
			return nil, err
		}
		name = c.DefaultProfile
	}
	if name == "" {
		name = "default"
	}
	dir := filepath.Join(Dir(), "profiles", name)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	p := &Profile{Name: name, Dir: dir, SessionPath: filepath.Join(dir, "session.db")}
	if b, err := os.ReadFile(filepath.Join(dir, "type")); err == nil {
		p.Type = strings.TrimSpace(string(b))
	}
	return p, nil
}

func SetProfileType(p *Profile, t string) error {
	return os.WriteFile(filepath.Join(p.Dir, "type"), []byte(t), 0o600)
}

func ListProfiles() ([]Profile, error) {
	root := filepath.Join(Dir(), "profiles")
	entries, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out []Profile
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		p, err := ResolveProfile(e.Name())
		if err != nil {
			return nil, err
		}
		out = append(out, *p)
	}
	return out, nil
}

func DeleteProfile(name string) error {
	if name == "" {
		return output.Errf("bad_args", "profile name required")
	}
	return os.RemoveAll(filepath.Join(Dir(), "profiles", name))
}

// APICredentials returns api_id/api_hash: env first, then config.
func APICredentials(c *Config) (int, string, error) {
	id, hash := c.APIID, c.APIHash
	if v := os.Getenv("TGC_API_ID"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			return 0, "", output.Errf("bad_config", "TGC_API_ID is not a number: %q", v)
		}
		id = n
	}
	if v := os.Getenv("TGC_API_HASH"); v != "" {
		hash = v
	}
	if id == 0 || hash == "" {
		return 0, "", output.Errf("no_api_credentials",
			"api_id/api_hash not set; get them at https://my.telegram.org and set TGC_API_ID/TGC_API_HASH or run `tgc auth login`")
	}
	return id, hash, nil
}
```

- [ ] **Step 4: Прогнать тесты**

Run: `cd /root/workspace/projects/tgc && go get github.com/BurntSushi/toml@latest && go mod tidy && go test ./...`
Expected: PASS.

- [ ] **Step 5: Commit + PR**

```bash
cd /root/workspace/projects/tgc
git checkout -b task/2-config 2>/dev/null || git checkout task/2-config
git add -A && git commit -m "feat: config and named profiles (XDG, env overrides, type markers)"
git push -u origin task/2-config
gh pr create --fill
gh pr merge --squash --auto --delete-branch
git checkout main && git pull
```

Если PR не смержился сразу — дождись CI (`gh pr checks --watch`); красный CI чини в этой же ветке.

---

### Task 3: Client bootstrap (gotgproto + floodwait + маппинг ошибок)

Это **компиляционный спайк**: первая задача, линкующая реальные зависимости. Если сигнатуры gotgproto/beta отличаются от приведённых — адаптируй код, сохранив produced-интерфейс (`Conn`, `Connect`, `ConnectForLogin`, `WrapErr`) неизменным.

**Files:**
- Create: `projects/tgc/internal/client/client.go`

**Interfaces:**
- Consumes: `config.ResolveProfile`, `config.APICredentials`, `output.Errf/ErrfX`.
- Produces:
  - `type client.Conn struct { Client *gotgproto.Client; Ctx *ext.Context; Profile *config.Profile }`
  - `client.Connect(profileName string) (*Conn, error)` — коннект по существующей сессии, никогда не спрашивает креденшелы (невалидная сессия → ошибка `not_authenticated`). `NoUpdates: true`.
  - `client.ConnectForLogin(profileName, phone, botToken string) (*Conn, error)` — интерактивный логин (терминальные промпты в stderr) или бот-логин по токену.
  - `conn.Close()`
  - `client.WrapErr(err error) error` — превращает RPC-ошибки в структурированные: FLOOD_WAIT → `{"error":"flood_wait","retry_after":X}`, `*_INVALID`/`BOT_METHOD_INVALID` → `bot_unsupported`, прочие — как есть.
  - Сессия профиля: если есть `<profile>/session.txt` (импортированная строка) — `sessionMaker.StringSession`, иначе `sessionMaker.SqlSession(sqlite.Open(session.db))` (glebarez, без CGO).

- [ ] **Step 1: Реализовать client.go**

```go
// Package client bootstraps the gotgproto Telegram client for a tgc profile.
package client

import (
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/celestix/gotgproto"
	"github.com/celestix/gotgproto/ext"
	"github.com/celestix/gotgproto/sessionMaker"
	"github.com/glebarez/sqlite"
	"github.com/gotd/contrib/middleware/floodwait"
	"github.com/gotd/td/telegram"
	"github.com/gotd/td/tgerr"

	"github.com/grigoreo-dev/tgc/internal/config"
	"github.com/grigoreo-dev/tgc/internal/output"
)

// maxAutoFloodWait: FLOOD_WAIT up to this duration is retried transparently.
const maxAutoFloodWait = 30 * time.Second

type Conn struct {
	Client  *gotgproto.Client
	Ctx     *ext.Context
	Profile *config.Profile
}

func (c *Conn) Close() { c.Client.Stop() }

func sessionFor(p *config.Profile) sessionMaker.SessionConstructor {
	if b, err := os.ReadFile(filepath.Join(p.Dir, "session.txt")); err == nil {
		return sessionMaker.StringSession(strings.TrimSpace(string(b)))
	}
	return sessionMaker.SqlSession(sqlite.Open(p.SessionPath))
}

func middlewares() []telegram.Middleware {
	return []telegram.Middleware{
		floodwait.NewSimpleWaiter().WithMaxRetries(3).WithMaxWait(maxAutoFloodWait),
	}
}

func build(p *config.Profile, ctype gotgproto.ClientType, opts *gotgproto.ClientOpts) (*Conn, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, err
	}
	apiID, apiHash, err := config.APICredentials(cfg)
	if err != nil {
		return nil, err
	}
	c, err := gotgproto.NewClient(apiID, apiHash, ctype, opts)
	if err != nil {
		return nil, WrapErr(err)
	}
	return &Conn{Client: c, Ctx: c.CreateContext(), Profile: p}, nil
}

// Connect opens a connection using the existing profile session.
// It never prompts: an invalid/absent session yields a structured error.
func Connect(profileName string) (*Conn, error) {
	p, err := config.ResolveProfile(profileName)
	if err != nil {
		return nil, err
	}
	ctype := gotgproto.ClientTypePhone("")
	if p.Type == "bot" {
		// Token is inside the stored session; empty token relies on it.
		ctype = gotgproto.ClientTypeBot("")
	}
	conn, err := build(p, ctype, &gotgproto.ClientOpts{
		Session:          sessionFor(p),
		NoUpdates:        true,
		NoAutoAuth:       true,
		DisableCopyright: true,
		Middlewares:      middlewares(),
	})
	if err != nil {
		return nil, output.Errf("not_authenticated",
			"profile %q has no valid session (%v); run `tgc auth login` or `tgc auth import`", p.Name, err)
	}
	return conn, nil
}

// ConnectForLogin runs the auth flow: interactive terminal prompts for user
// accounts, non-interactive for bot tokens. Persists profile type on success.
func ConnectForLogin(profileName, phone, botToken string) (*Conn, error) {
	p, err := config.ResolveProfile(profileName)
	if err != nil {
		return nil, err
	}
	var ctype gotgproto.ClientType
	ptype := "user"
	if botToken != "" {
		ctype = gotgproto.ClientTypeBot(botToken)
		ptype = "bot"
	} else {
		ctype = gotgproto.ClientTypePhone(phone)
	}
	conn, err := build(p, ctype, &gotgproto.ClientOpts{
		Session:          sessionFor(p),
		NoUpdates:        true,
		DisableCopyright: true,
		Middlewares:      middlewares(),
		AuthConversator:  &terminalConversator{},
	})
	if err != nil {
		return nil, err
	}
	if err := config.SetProfileType(p, ptype); err != nil {
		conn.Close()
		return nil, err
	}
	return conn, nil
}

// WrapErr maps Telegram RPC errors onto the tgc structured error contract.
func WrapErr(err error) error {
	if err == nil {
		return nil
	}
	if wait, ok := tgerr.AsFloodWait(err); ok {
		return output.ErrfX("flood_wait",
			map[string]any{"retry_after": int(wait.Seconds())},
			"telegram flood wait: retry after %d seconds", int(wait.Seconds()))
	}
	if tgerr.Is(err, "BOT_METHOD_INVALID") {
		return output.Errf("bot_unsupported", "this command is not available for bot accounts")
	}
	return err
}
```

- [ ] **Step 2: Терминальный конверсатор (в том же файле)**

```go
// terminalConversator prompts on stderr and reads answers from stdin.
type terminalConversator struct{}

func prompt(label string) (string, error) {
	if _, err := os.Stderr.WriteString(label); err != nil {
		return "", err
	}
	var s string
	if _, err := fmt.Fscanln(os.Stdin, &s); err != nil {
		return "", err
	}
	return strings.TrimSpace(s), nil
}

func (t *terminalConversator) AskPhoneNumber() (string, error) { return prompt("Phone number (intl format): ") }
func (t *terminalConversator) AskCode() (string, error)        { return prompt("Code from Telegram: ") }
func (t *terminalConversator) AskPassword() (string, error)    { return prompt("2FA password: ") }
func (t *terminalConversator) AuthStatus(s gotgproto.AuthStatus) {
	fmt.Fprintf(os.Stderr, "auth: %v\n", s.Event)
}
```

Добавь `"fmt"` в импорты. Если интерфейс `AuthConversator` в текущей beta-версии имеет другие методы (например, `RetryPassword`) — реализуй их аналогично, промптами в stderr.

- [ ] **Step 3: Собрать с реальными зависимостями**

Run:
```bash
cd /root/workspace/projects/tgc
go get github.com/celestix/gotgproto@beta github.com/gotd/td@latest github.com/gotd/contrib@latest github.com/glebarez/sqlite@latest
go mod tidy && go build ./... && go vet ./...
```
Expected: сборка зелёная. Если API отличается (имена `ClientTypePhone`/`CreateContext`/`Stop`/поля ClientOpts) — поправь под фактические сигнатуры, интерфейс пакета не меняй.

- [ ] **Step 4: Commit + PR**

```bash
cd /root/workspace/projects/tgc
git checkout -b task/3-client 2>/dev/null || git checkout task/3-client
git add -A && git commit -m "feat: gotgproto client bootstrap with floodwait and error mapping"
git push -u origin task/3-client
gh pr create --fill
gh pr merge --squash --auto --delete-branch
git checkout main && git pull
```

Если PR не смержился сразу — дождись CI (`gh pr checks --watch`); красный CI чини в этой же ветке.

---

### Task 4: Команды auth (login / export / import / list / logout)

**Files:**
- Create: `projects/tgc/internal/cli/auth.go`

**Interfaces:**
- Consumes: `client.Connect`, `client.ConnectForLogin`, `config.*`, `output.*`.
- Produces: CLI-команды `tgc auth login|export|import|list|logout`. JSON-ответы: login → `{"status":"ok","profile":"...","type":"user|bot","user_id":...}`; export → `{"session":"<base64>"}` или запись в файл; import → `{"status":"ok","profile":"..."}`; list → JSONL `{"name":...,"type":...,"default":bool}`; logout → `{"status":"ok"}`.

- [ ] **Step 1: Реализовать auth.go**

```go
package cli

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/grigoreo-dev/tgc/internal/client"
	"github.com/grigoreo-dev/tgc/internal/config"
	"github.com/grigoreo-dev/tgc/internal/output"
	"github.com/spf13/cobra"
)

var authCmd = &cobra.Command{Use: "auth", Short: "Manage Telegram sessions"}

var (
	loginPhone    string
	loginBotToken string
	loginAPIID    int
	loginAPIHash  string
	exportOut     string
)

var authLoginCmd = &cobra.Command{
	Use:   "login",
	Short: "Log in interactively (user) or with --bot-token (bot)",
	RunE: func(cmd *cobra.Command, args []string) error {
		if loginAPIID != 0 || loginAPIHash != "" {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			if loginAPIID != 0 {
				cfg.APIID = loginAPIID
			}
			if loginAPIHash != "" {
				cfg.APIHash = loginAPIHash
			}
			if err := config.Save(cfg); err != nil {
				return err
			}
		}
		conn, err := client.ConnectForLogin(ProfileName(), loginPhone, loginBotToken)
		if err != nil {
			return err
		}
		defer conn.Close()
		self := conn.Client.Self
		ptype := "user"
		if loginBotToken != "" {
			ptype = "bot"
		}
		output.Emit(map[string]any{
			"status": "ok", "profile": conn.Profile.Name, "type": ptype,
			"user_id": self.ID, "username": self.Username,
		})
		return nil
	},
}

var authExportCmd = &cobra.Command{
	Use:   "export",
	Short: "Export session as a portable string",
	RunE: func(cmd *cobra.Command, args []string) error {
		conn, err := client.Connect(ProfileName())
		if err != nil {
			return err
		}
		defer conn.Close()
		s, err := conn.Client.ExportStringSession()
		if err != nil {
			return err
		}
		if exportOut != "" {
			if err := os.WriteFile(exportOut, []byte(s), 0o600); err != nil {
				return err
			}
			output.Emit(map[string]any{"status": "ok", "path": exportOut})
			return nil
		}
		output.Emit(map[string]any{"session": s})
		return nil
	},
}

var authImportCmd = &cobra.Command{
	Use:   "import [file]",
	Short: "Import a session string (arg=file, stdin, or TGC_SESSION env)",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		var raw []byte
		var err error
		switch {
		case len(args) == 1:
			raw, err = os.ReadFile(args[0])
		case os.Getenv("TGC_SESSION") != "":
			raw = []byte(os.Getenv("TGC_SESSION"))
		default:
			raw, err = os.ReadFile("/dev/stdin")
		}
		if err != nil {
			return err
		}
		s := strings.TrimSpace(string(raw))
		if s == "" {
			return output.Errf("bad_args", "empty session string")
		}
		p, err := config.ResolveProfile(ProfileName())
		if err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(p.Dir, "session.txt"), []byte(s), 0o600); err != nil {
			return err
		}
		// Validate by connecting once.
		conn, err := client.Connect(p.Name)
		if err != nil {
			_ = os.Remove(filepath.Join(p.Dir, "session.txt"))
			return err
		}
		defer conn.Close()
		ptype := "user"
		if conn.Client.Self.Bot {
			ptype = "bot"
		}
		if err := config.SetProfileType(p, ptype); err != nil {
			return err
		}
		output.Emit(map[string]any{"status": "ok", "profile": p.Name, "type": ptype})
		return nil
	},
}

var authListCmd = &cobra.Command{
	Use:   "list",
	Short: "List profiles",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load()
		if err != nil {
			return err
		}
		def := cfg.DefaultProfile
		if def == "" {
			def = "default"
		}
		profiles, err := config.ListProfiles()
		if err != nil {
			return err
		}
		for _, p := range profiles {
			output.Emit(map[string]any{"name": p.Name, "type": p.Type, "default": p.Name == def})
		}
		return nil
	},
}

var authLogoutCmd = &cobra.Command{
	Use:   "logout [profile]",
	Short: "Delete a profile and its session",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := ProfileName()
		if len(args) == 1 {
			name = args[0]
		}
		if name == "" {
			name = "default"
		}
		if err := config.DeleteProfile(name); err != nil {
			return err
		}
		output.Emit(map[string]any{"status": "ok", "profile": name})
		return nil
	},
}

func init() {
	authLoginCmd.Flags().StringVar(&loginPhone, "phone", "", "phone number (international format)")
	authLoginCmd.Flags().StringVar(&loginBotToken, "bot-token", "", "bot token from @BotFather")
	authLoginCmd.Flags().IntVar(&loginAPIID, "api-id", 0, "Telegram api_id (saved to config)")
	authLoginCmd.Flags().StringVar(&loginAPIHash, "api-hash", "", "Telegram api_hash (saved to config)")
	authExportCmd.Flags().StringVarP(&exportOut, "out", "o", "", "write session to file instead of stdout")
	authCmd.AddCommand(authLoginCmd, authExportCmd, authImportCmd, authListCmd, authLogoutCmd)
	rootCmd.AddCommand(authCmd)
}
```

Примечание: если у `conn.Client.Self` другое имя поля/типа в текущей beta — адаптируй (нужны ID, Username, Bot).

- [ ] **Step 2: Сборка и smoke-тест без сети**

Run:
```bash
cd /root/workspace/projects/tgc && go build ./... && go vet ./... && go test ./...
TGC_CONFIG_DIR=$(mktemp -d) go run ./cmd/tgc auth list
TGC_CONFIG_DIR=$(mktemp -d) go run ./cmd/tgc auth export 2>&1; echo "exit=$?"
```
Expected: `auth list` — пустой вывод, exit 0. `auth export` — JSON-ошибка `no_api_credentials` или `not_authenticated` в stderr, exit=1.

- [ ] **Step 3: Commit + PR**

```bash
cd /root/workspace/projects/tgc
git checkout -b task/4-auth 2>/dev/null || git checkout task/4-auth
git add -A && git commit -m "feat: auth commands (login, export, import, list, logout)"
git push -u origin task/4-auth
gh pr create --fill
gh pr merge --squash --auto --delete-branch
git checkout main && git pull
```

Если PR не смержился сразу — дождись CI (`gh pr checks --watch`); красный CI чини в этой же ветке.

---

### Task 5: Resolve — универсальный селектор + кэш диалогов

**Files:**
- Create: `projects/tgc/internal/resolve/resolve.go`, `projects/tgc/internal/resolve/cache.go`, `projects/tgc/internal/resolve/resolve_test.go`

**Interfaces:**
- Consumes: `client.Conn` (Ctx.PeerStorage, Ctx.ResolveUsername, Ctx.Raw), `output.ErrfX`.
- Produces:
  - `type resolve.Peer struct { ID int64 \`json:"id"\`; AccessHash int64 \`json:"-"\`; Type string \`json:"type"\`; Title string \`json:"title"\`; Username string \`json:"username,omitempty"\` }` (`Type`: `"user"|"group"|"channel"`)
  - `resolve.Resolve(conn *client.Conn, selector string) (*Peer, error)` — порядок: числовой ID → `@username` → телефон (`+...`) → fuzzy по кэшу диалогов. Неоднозначный fuzzy → ошибка `ambiguous` с `candidates` в Extra. Не найдено → `not_found`.
  - `resolve.InputPeer(conn *client.Conn, p *Peer) (tg.InputPeerClass, error)`
  - `resolve.Dialogs(conn *client.Conn, fresh bool, limit int) ([]Peer, error)` — список диалогов; кэш в `<profile>/dialogs.json` с TTL 5 минут; `fresh` — форсировать API.
  - Чисто-функциональное ядро для тестов: `resolve.Classify(selector string) (kind, value string)` (kind: `"id"|"username"|"phone"|"name"`) и `resolve.FuzzyMatch(peers []Peer, query string) []Peer`.

- [ ] **Step 1: Написать падающие тесты (чистые функции)**

`projects/tgc/internal/resolve/resolve_test.go`:

```go
package resolve

import "testing"

func TestClassify(t *testing.T) {
	cases := []struct{ in, kind, val string }{
		{"@durov", "username", "durov"},
		{"durov", "name", "durov"},        // bare word without @ is a fuzzy name
		{"123456789", "id", "123456789"},
		{"-1001234567890", "id", "-1001234567890"},
		{"+79991234567", "phone", "79991234567"},
		{"Alice Smith", "name", "Alice Smith"},
	}
	for _, c := range cases {
		kind, val := Classify(c.in)
		if kind != c.kind || val != c.val {
			t.Errorf("Classify(%q) = %q,%q want %q,%q", c.in, kind, val, c.kind, c.val)
		}
	}
}

func TestFuzzyMatch(t *testing.T) {
	peers := []Peer{
		{ID: 1, Title: "Alice Smith", Username: "alice"},
		{ID: 2, Title: "Alice Jones"},
		{ID: 3, Title: "Work Chat"},
	}
	// exact-ish single match
	got := FuzzyMatch(peers, "work")
	if len(got) != 1 || got[0].ID != 3 {
		t.Fatalf("want peer 3, got %+v", got)
	}
	// ambiguous
	got = FuzzyMatch(peers, "alice")
	if len(got) != 2 {
		t.Fatalf("want 2 candidates, got %d", len(got))
	}
	// case-insensitive over username too
	got = FuzzyMatch(peers, "ALICE")
	if len(got) == 0 {
		t.Fatal("username match must be case-insensitive")
	}
	// non-ASCII (Cyrillic) titles still match case-insensitively
	cyr := []Peer{{ID: 4, Title: "\u0412\u0430\u0441\u044f"}} // Cyrillic name via escapes; fixture stays ASCII
	if len(FuzzyMatch(cyr, "\u0432\u0430\u0441\u044f")) != 1 {
		t.Fatal("cyrillic titles must match case-insensitively")
	}
}

func TestDialogCacheTTL(t *testing.T) {
	dir := t.TempDir()
	peers := []Peer{{ID: 1, Title: "A", Type: "user"}}
	if err := saveDialogCache(dir, peers); err != nil {
		t.Fatal(err)
	}
	got, ok := loadDialogCache(dir, 300)
	if !ok || len(got) != 1 || got[0].ID != 1 {
		t.Fatalf("fresh cache must load: ok=%v got=%+v", ok, got)
	}
	if _, ok := loadDialogCache(dir, 0); ok {
		t.Fatal("expired cache (ttl=0) must miss")
	}
}
```

- [ ] **Step 2: Запустить — падают**

Run: `cd /root/workspace/projects/tgc && go test ./internal/resolve/`
Expected: FAIL.

- [ ] **Step 3: Реализовать cache.go**

```go
package resolve

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

type dialogCacheFile struct {
	SavedAt time.Time `json:"saved_at"`
	Peers   []Peer    `json:"peers"`
}

func cachePath(profileDir string) string { return filepath.Join(profileDir, "dialogs.json") }

func saveDialogCache(profileDir string, peers []Peer) error {
	b, err := json.Marshal(dialogCacheFile{SavedAt: time.Now(), Peers: peers})
	if err != nil {
		return err
	}
	return os.WriteFile(cachePath(profileDir), b, 0o600)
}

// loadDialogCache returns cached peers if the cache is younger than ttlSeconds.
func loadDialogCache(profileDir string, ttlSeconds int) ([]Peer, bool) {
	b, err := os.ReadFile(cachePath(profileDir))
	if err != nil {
		return nil, false
	}
	var f dialogCacheFile
	if json.Unmarshal(b, &f) != nil {
		return nil, false
	}
	if time.Since(f.SavedAt) > time.Duration(ttlSeconds)*time.Second {
		return nil, false
	}
	return f.Peers, true
}
```

- [ ] **Step 4: Реализовать resolve.go**

```go
// Package resolve turns user-facing chat selectors (@username, numeric ID,
// phone, fuzzy display name) into Telegram peers, using local caches first.
package resolve

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/gotd/td/tg"

	"github.com/grigoreo-dev/tgc/internal/client"
	"github.com/grigoreo-dev/tgc/internal/output"
)

const dialogCacheTTLSeconds = 300

type Peer struct {
	ID         int64  `json:"id"`
	AccessHash int64  `json:"-"`
	Type       string `json:"type"` // user | group | channel
	Title      string `json:"title"`
	Username   string `json:"username,omitempty"`
}

var phoneRe = regexp.MustCompile(`^\+[0-9]{7,15}$`)

// Classify determines the selector kind: id, username, phone or name.
func Classify(s string) (kind, value string) {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "@") {
		return "username", strings.TrimPrefix(s, "@")
	}
	if _, err := strconv.ParseInt(s, 10, 64); err == nil {
		return "id", s
	}
	if phoneRe.MatchString(s) {
		return "phone", strings.TrimPrefix(s, "+")
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
	case "id":
		id, _ := strconv.ParseInt(val, 10, 64)
		return resolveByID(conn, id)
	case "username":
		chat, err := conn.Ctx.ResolveUsername(val)
		if err != nil {
			return nil, output.Errf("not_found", "cannot resolve @%s: %v", val, client.WrapErr(err))
		}
		return peerFromEffectiveChat(chat), nil
	case "phone":
		return resolveByPhone(conn, val)
	default: // name → fuzzy over dialogs
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
```

Далее в том же файле — интеграционные хелперы (компилируемые, но проверяемые только вживую):

```go
func resolveByID(conn *client.Conn, id int64) (*Peer, error) {
	// gotgproto peer storage knows id+access_hash for everything seen before.
	ip := conn.Ctx.PeerStorage.GetInputPeerById(id)
	if _, isEmpty := ip.(*tg.InputPeerEmpty); isEmpty {
		// Fall back to dialogs cache/API once.
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
	return peerFromInputPeer(conn, id, ip), nil
}

func resolveByPhone(conn *client.Conn, phone string) (*Peer, error) {
	res, err := conn.Ctx.Raw.ContactsResolvePhone(conn.Ctx, phone)
	if err != nil {
		return nil, output.Errf("not_found", "cannot resolve phone +%s: %v", phone, client.WrapErr(err))
	}
	for _, u := range res.Users {
		if user, ok := u.(*tg.User); ok {
			return &Peer{ID: user.ID, AccessHash: user.AccessHash, Type: "user",
				Title: strings.TrimSpace(user.FirstName + " " + user.LastName), Username: user.Username}, nil
		}
	}
	return nil, output.Errf("not_found", "phone +%s did not resolve to a user", phone)
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
	_ = saveDialogCache(conn.Profile.Dir, peers) // cache write is best-effort
	return capPeers(peers, limit), nil
}

func capPeers(peers []Peer, limit int) []Peer {
	if limit > 0 && len(peers) > limit {
		return peers[:limit]
	}
	return peers
}
```

Допиши недостающие конвертеры (`peerFromEffectiveChat`, `peerFromInputPeer`, `peersFromDialogs`) по фактическим типам gotgproto/gotd: users → `Type:"user"`, `tg.Chat` → `"group"`, `tg.Channel` → `"channel"` (megagroup-каналы тоже помечай `"group"`), заголовок — имя+фамилия или Title. `InputPeer(conn, p)` — собери `tg.InputPeerUser/Chat/Channel` из ID+AccessHash, либо верни из PeerStorage.

- [ ] **Step 5: Прогнать тесты и сборку**

Run: `cd /root/workspace/projects/tgc && go build ./... && go vet ./... && go test ./...`
Expected: PASS (юнит-тесты Classify/FuzzyMatch/cache зелёные, остальное компилируется).

- [ ] **Step 6: Commit + PR**

```bash
cd /root/workspace/projects/tgc
git checkout -b task/5-resolve 2>/dev/null || git checkout task/5-resolve
git add -A && git commit -m "feat: universal chat selector with dialog cache (TTL 5m)"
git push -u origin task/5-resolve
gh pr create --fill
gh pr merge --squash --auto --delete-branch
git checkout main && git pull
```

Если PR не смержился сразу — дождись CI (`gh pr checks --watch`); красный CI чини в этой же ветке.

---

### Task 6: Markdown → entities (markup, базовый уровень)

RichMessage — в Task 11; здесь плоские entities, которые работают везде.

**Files:**
- Create: `projects/tgc/internal/markup/markdown.go`, `projects/tgc/internal/markup/markdown_test.go`

**Interfaces:**
- Consumes: `github.com/gotd/td/telegram/message/styling`, `github.com/gotd/td/telegram/message/entity`.
- Produces:
  - `markup.Parse(md string) (text string, entities []tg.MessageEntityClass, err error)` — конвертирует Markdown-подмножество в плоский текст + entities: `**bold**`, `*italic*` / `_italic_`, `` `code` ``, ```` ```pre``` ```` (с языком), `[label](url)`, `~~strike~~`. Блочные элементы деградируют: `# заголовок` → bold-строка, `- item` → `• item`, `> quote` → entity blockquote, таблицы → pre.
  - `markup.ParsePlain(s string) (string, []tg.MessageEntityClass, error)` — без парсинга (пустые entities).

Реализация — своя, на основе построчного прохода + inline-токенизатора. Не тяни тяжёлые markdown-библиотеки: подмножество маленькое, а маппинг офсетов в UTF-16 (требование Telegram) всё равно ручной. Для UTF-16-длины используй `entity.ComputeLength` из gotd, если доступна, иначе:

```go
func utf16len(s string) int {
	n := 0
	for _, r := range s {
		if r > 0xFFFF {
			n += 2
		} else {
			n++
		}
	}
	return n
}
```

- [ ] **Step 1: Написать падающие тесты**

`projects/tgc/internal/markup/markdown_test.go`:

```go
package markup

import (
	"testing"

	"github.com/gotd/td/tg"
)

func TestParseBold(t *testing.T) {
	text, ents, err := Parse("hello **world**")
	if err != nil {
		t.Fatal(err)
	}
	if text != "hello world" {
		t.Fatalf("text: %q", text)
	}
	if len(ents) != 1 {
		t.Fatalf("want 1 entity, got %d", len(ents))
	}
	b, ok := ents[0].(*tg.MessageEntityBold)
	if !ok {
		t.Fatalf("want Bold, got %T", ents[0])
	}
	if b.Offset != 6 || b.Length != 5 {
		t.Fatalf("offset/length: %d/%d", b.Offset, b.Length)
	}
}

func TestParseInlineCodeAndLink(t *testing.T) {
	text, ents, err := Parse("run `ls` or [docs](https://example.com)")
	if err != nil {
		t.Fatal(err)
	}
	if text != "run ls or docs" {
		t.Fatalf("text: %q", text)
	}
	var haveCode, haveURL bool
	for _, e := range ents {
		switch e.(type) {
		case *tg.MessageEntityCode:
			haveCode = true
		case *tg.MessageEntityTextURL:
			haveURL = true
		}
	}
	if !haveCode || !haveURL {
		t.Fatalf("code=%v url=%v ents=%+v", haveCode, haveURL, ents)
	}
}

func TestParseCodeBlockWithLang(t *testing.T) {
	text, ents, err := Parse("```go\nfmt.Println(1)\n```")
	if err != nil {
		t.Fatal(err)
	}
	if text != "fmt.Println(1)" {
		t.Fatalf("text: %q", text)
	}
	pre, ok := ents[0].(*tg.MessageEntityPre)
	if !ok || pre.Language != "go" {
		t.Fatalf("want pre(go), got %+v", ents[0])
	}
}

func TestHeadingDegradesToBoldLine(t *testing.T) {
	text, ents, err := Parse("# Title\nbody")
	if err != nil {
		t.Fatal(err)
	}
	if text != "Title\nbody" {
		t.Fatalf("text: %q", text)
	}
	b, ok := ents[0].(*tg.MessageEntityBold)
	if !ok || b.Offset != 0 || b.Length != 5 {
		t.Fatalf("heading must become bold line: %+v", ents[0])
	}
}

func TestListBullets(t *testing.T) {
	text, _, err := Parse("- one\n- two")
	if err != nil {
		t.Fatal(err)
	}
	if text != "• one\n• two" {
		t.Fatalf("text: %q", text)
	}
}

func TestUTF16OffsetsWithEmoji(t *testing.T) {
	// 🔥 is 2 UTF-16 units; bold must start at offset 3 (f=1,i=1? no: "🔥 " = 2+1=3)
	text, ents, err := Parse("🔥 **hot**")
	if err != nil {
		t.Fatal(err)
	}
	if text != "🔥 hot" {
		t.Fatalf("text: %q", text)
	}
	b := ents[0].(*tg.MessageEntityBold)
	if b.Offset != 3 || b.Length != 3 {
		t.Fatalf("UTF-16 offsets wrong: %d/%d", b.Offset, b.Length)
	}
}

func TestPlainNoParsing(t *testing.T) {
	text, ents, err := ParsePlain("**not bold**")
	if err != nil || text != "**not bold**" || len(ents) != 0 {
		t.Fatalf("plain must not parse: %q %v %v", text, ents, err)
	}
}
```

- [ ] **Step 2: Запустить — падают**

Run: `cd /root/workspace/projects/tgc && go test ./internal/markup/`
Expected: FAIL.

- [ ] **Step 3: Реализовать markdown.go**

Структура реализации (полный файл пишет исполнитель, скелет обязательных частей):

```go
// Package markup converts agent-friendly Markdown into Telegram message
// text + entities (UTF-16 offsets), degrading block elements gracefully.
package markup

import (
	"strings"

	"github.com/gotd/td/tg"
)

func ParsePlain(s string) (string, []tg.MessageEntityClass, error) {
	return s, nil, nil
}

// Parse converts a Markdown subset to text + entities.
// Supported inline: **bold**, *italic*, _italic_, `code`, ~~strike~~, [t](url).
// Block level: ``` fences → Pre, # headings → bold line, - lists → bullets,
// > quotes → Blockquote, everything else passes through.
func Parse(md string) (string, []tg.MessageEntityClass, error) {
	var out strings.Builder
	var ents []tg.MessageEntityClass
	lines := strings.Split(md, "\n")
	i := 0
	for i < len(lines) {
		line := lines[i]
		switch {
		case strings.HasPrefix(line, "```"):
			lang := strings.TrimPrefix(line, "```")
			var code []string
			i++
			for i < len(lines) && !strings.HasPrefix(lines[i], "```") {
				code = append(code, lines[i])
				i++
			}
			i++ // skip closing fence
			body := strings.Join(code, "\n")
			start := utf16len(out.String())
			out.WriteString(body)
			ents = append(ents, &tg.MessageEntityPre{Offset: start, Length: utf16len(body), Language: lang})
		case strings.HasPrefix(line, "#"):
			title := strings.TrimSpace(strings.TrimLeft(line, "#"))
			start := utf16len(out.String())
			out.WriteString(title)
			ents = append(ents, &tg.MessageEntityBold{Offset: start, Length: utf16len(title)})
			i++
		case strings.HasPrefix(line, "- ") || strings.HasPrefix(line, "* "):
			item := line[2:]
			text, sub := parseInline(item, utf16len(out.String())+utf16len("• "))
			out.WriteString("• " + text)
			ents = append(ents, sub...)
			i++
		case strings.HasPrefix(line, "> "):
			quoted := strings.TrimPrefix(line, "> ")
			start := utf16len(out.String())
			text, sub := parseInline(quoted, start)
			out.WriteString(text)
			ents = append(ents, &tg.MessageEntityBlockquote{Offset: start, Length: utf16len(text)})
			ents = append(ents, sub...)
			i++
		default:
			text, sub := parseInline(line, utf16len(out.String()))
			out.WriteString(text)
			ents = append(ents, sub...)
			i++
		}
		if i <= len(lines)-1 {
			out.WriteString("\n")
		}
	}
	return out.String(), ents, nil
}
```

`parseInline(s string, base int) (string, []tg.MessageEntityClass)` — однопроходный токенизатор: сканируй руны, при встрече маркера (`**`, `*`, `_`, `` ` ``, `~~`, `[`) ищи закрывающий; без пары — маркер остаётся литералом. Оффсеты — `base + utf16len(накопленного текста)`. Таблицы (строки, начинающиеся с `|`) собирай в блок и оформляй как `MessageEntityPre` без языка.

- [ ] **Step 4: Прогнать тесты**

Run: `cd /root/workspace/projects/tgc && go test ./internal/markup/ -v`
Expected: PASS все кейсы, включая эмодзи-оффсеты.

- [ ] **Step 5: Commit + PR**

```bash
cd /root/workspace/projects/tgc
git checkout -b task/6-markup 2>/dev/null || git checkout task/6-markup
git add -A && git commit -m "feat: markdown to Telegram entities converter (UTF-16 offsets, block degradation)"
git push -u origin task/6-markup
gh pr create --fill
gh pr merge --squash --auto --delete-branch
git checkout main && git pull
```

Если PR не смержился сразу — дождись CI (`gh pr checks --watch`); красный CI чини в этой же ветке.

---

### Task 7: ops/chats + CLI-команды chats, info, members, search

**Files:**
- Create: `projects/tgc/internal/ops/chats.go`, `projects/tgc/internal/cli/chats.go`

**Interfaces:**
- Consumes: `client.Conn`, `resolve.*`, `output.*`.
- Produces (пакет `ops`):
  - `ops.Chats(conn *client.Conn, fresh bool, limit int, typeFilter string) ([]resolve.Peer, error)` — обёртка над `resolve.Dialogs` с фильтром по `Type`.
  - `ops.Info(conn *client.Conn, selector string) (map[string]any, error)` — карточка: `{id, type, title, username, phone?, members_count?, about?, bot?}` (users — через `users.getFullUser`, каналы/группы — `channels.getFullChannel`/`messages.getFullChat`).
  - `ops.Members(conn *client.Conn, selector string, limit int) ([]map[string]any, error)` — `{id, name, username, status}` где status ∈ `creator|admin|member|banned|left`. Для user-селектора — ошибка `bad_args`.
  - `ops.SearchChats(conn *client.Conn, query string, limit int) ([]resolve.Peer, error)` — локальный кэш диалогов (FuzzyMatch) + `contacts.Search` API, дедуп по ID.
  - `ops.SearchMessages(conn *client.Conn, query string, limit int) ([]map[string]any, error)` — глобальный `messages.searchGlobal`, элементы в формате сообщений Task 8.
- Produces (CLI): `tgc chats [--limit N] [--type user|group|channel] [--fresh]`, `tgc info <sel>`, `tgc members <sel> [--limit N]`, `tgc search <q> [--messages] [--limit N]`.

- [ ] **Step 1: Реализовать ops/chats.go**

Ключевые вызовы (адаптируй к фактическим сигнатурам):

```go
// Package ops implements tgc core operations, reusable by future server modes.
package ops

import (
	"github.com/gotd/td/tg"

	"github.com/grigoreo-dev/tgc/internal/client"
	"github.com/grigoreo-dev/tgc/internal/output"
	"github.com/grigoreo-dev/tgc/internal/resolve"
)

func Chats(conn *client.Conn, fresh bool, limit int, typeFilter string) ([]resolve.Peer, error) {
	peers, err := resolve.Dialogs(conn, fresh, 0)
	if err != nil {
		return nil, err
	}
	var out []resolve.Peer
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

func SearchChats(conn *client.Conn, query string, limit int) ([]resolve.Peer, error) {
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
		for _, p := range peersFromContactsSearch(res) {
			if !seen[p.ID] {
				found = append(found, p)
				seen[p.ID] = true
			}
		}
	}
	if limit > 0 && len(found) > limit {
		found = found[:limit]
	}
	return found, nil
}
```

`Info` — свитч по `peer.Type`: `user` → `UsersGetFullUser` (нужен `tg.InputUser{UserID, AccessHash}`), `channel`/`group`-мегагруппа → `ChannelsGetFullChannel`, легаси-группа → `MessagesGetFullChat`. Собери map с непустыми полями. `Members` — для `channel/group`: `ChannelsGetParticipants` c `tg.ChannelParticipantsRecent{}` (пагинация offset/limit до `limit`), для легаси-групп — из `MessagesGetFullChat`. Статусы маппи из типов участников (`ChannelParticipantCreator` → `creator` и т.д.). `SearchMessages` — `MessagesSearchGlobal` с `tg.InputMessagesFilterEmpty{}`. Конверсия сообщений (`messageToMap`) реализуется в Task 8: здесь объяви `SearchMessages` с телом-заглушкой `return nil, output.Errf("not_implemented", "message search lands with the read command")`, а в Task 8 замени тело на реальный вызов + `collectMessages`. (В Task 8 это отражено шагом Step 3a.)

Важно: `peersFromContactsSearch(res *tg.ContactsFound) []resolve.Peer` — переиспользуй конвертеры peers из resolve (экспортируй из resolve хелпер `resolve.PeersFromUsersChats(users []tg.UserClass, chats []tg.ChatClass) []resolve.Peer` и используй его и в resolve.Dialogs, и здесь).

- [ ] **Step 2: Реализовать cli/chats.go**

```go
package cli

import (
	"github.com/grigoreo-dev/tgc/internal/client"
	"github.com/grigoreo-dev/tgc/internal/ops"
	"github.com/grigoreo-dev/tgc/internal/output"
	"github.com/spf13/cobra"
)

var (
	chatsLimit  int
	chatsType   string
	chatsFresh  bool
	membersLimit int
	searchMsgs   bool
	searchLimit  int
)

var chatsCmd = &cobra.Command{
	Use:   "chats",
	Short: "List dialogs (cached 5m; --fresh to refresh)",
	RunE: func(cmd *cobra.Command, args []string) error {
		conn, err := client.Connect(ProfileName())
		if err != nil {
			return err
		}
		defer conn.Close()
		peers, err := ops.Chats(conn, chatsFresh, chatsLimit, chatsType)
		if err != nil {
			return err
		}
		for _, p := range peers {
			output.Emit(p)
		}
		return nil
	},
}

var infoCmd = &cobra.Command{
	Use:   "info <chat>",
	Short: "Show chat/user card",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		conn, err := client.Connect(ProfileName())
		if err != nil {
			return err
		}
		defer conn.Close()
		card, err := ops.Info(conn, args[0])
		if err != nil {
			return err
		}
		output.Emit(card)
		return nil
	},
}

var membersCmd = &cobra.Command{
	Use:   "members <group>",
	Short: "List group members",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		conn, err := client.Connect(ProfileName())
		if err != nil {
			return err
		}
		defer conn.Close()
		items, err := ops.Members(conn, args[0], membersLimit)
		if err != nil {
			return err
		}
		for _, m := range items {
			output.Emit(m)
		}
		return nil
	},
}

var searchCmd = &cobra.Command{
	Use:   "search <query>",
	Short: "Search chats/contacts; --messages for global message search",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		conn, err := client.Connect(ProfileName())
		if err != nil {
			return err
		}
		defer conn.Close()
		if searchMsgs {
			items, err := ops.SearchMessages(conn, args[0], searchLimit)
			if err != nil {
				return err
			}
			for _, m := range items {
				output.Emit(m)
			}
			return nil
		}
		peers, err := ops.SearchChats(conn, args[0], searchLimit)
		if err != nil {
			return err
		}
		for _, p := range peers {
			output.Emit(p)
		}
		return nil
	},
}

func init() {
	chatsCmd.Flags().IntVar(&chatsLimit, "limit", 50, "max dialogs")
	chatsCmd.Flags().StringVar(&chatsType, "type", "", "filter: user|group|channel")
	chatsCmd.Flags().BoolVar(&chatsFresh, "fresh", false, "bypass dialog cache")
	membersCmd.Flags().IntVar(&membersLimit, "limit", 200, "max members")
	searchCmd.Flags().BoolVar(&searchMsgs, "messages", false, "global message search")
	searchCmd.Flags().IntVar(&searchLimit, "limit", 20, "max results")
	rootCmd.AddCommand(chatsCmd, infoCmd, membersCmd, searchCmd)
}
```

- [ ] **Step 3: Сборка + тесты + smoke**

Run:
```bash
cd /root/workspace/projects/tgc && go build ./... && go vet ./... && go test ./...
TGC_CONFIG_DIR=$(mktemp -d) go run ./cmd/tgc chats 2>&1; echo "exit=$?"
```
Expected: сборка/тесты зелёные; `chats` без сессии — JSON-ошибка (`no_api_credentials`/`not_authenticated`) в stderr, exit=1.

- [ ] **Step 4: Commit + PR**

```bash
cd /root/workspace/projects/tgc
git checkout -b task/7-chats 2>/dev/null || git checkout task/7-chats
git add -A && git commit -m "feat: chats, info, members, search commands"
git push -u origin task/7-chats
gh pr create --fill
gh pr merge --squash --auto --delete-branch
git checkout main && git pull
```

Если PR не смержился сразу — дождись CI (`gh pr checks --watch`); красный CI чини в этой же ветке.

---

### Task 8: ops/messages — read и context + CLI

**Files:**
- Create: `projects/tgc/internal/ops/messages.go`, `projects/tgc/internal/ops/messages_test.go`, `projects/tgc/internal/cli/read.go`

**Interfaces:**
- Consumes: `client.Conn`, `resolve.Resolve/InputPeer`, `output.*`.
- Produces:
  - `type ops.ReadOpts struct { Limit int; BeforeID, AfterID int; Since, Until string; From string; Search string }` (Since/Until — `YYYY-MM-DD` или RFC3339)
  - `ops.Read(conn *client.Conn, selector string, o ReadOpts) ([]map[string]any, error)` — сообщения новые-сверху. Пути: `Search != ""` → `messages.search` с `InputMessagesFilterEmpty` + Q; `From != ""` → `messages.search` с `FromID`; иначе `messages.getHistory` (`MaxID`/`OffsetID` for `--before`, `MinID` for `--after`, OffsetDate from Until; client-side Since filter only).
  - `ops.Context(conn *client.Conn, selector string, msgID, radius int) ([]map[string]any, error)` — `getHistory` с `OffsetID: msgID+radius+1, Limit: radius*2+1`.
  - `ops.messageToMap(m *tg.Message, users map[int64]*tg.User, chats map[int64]tg.ChatClass, chatID int64) map[string]any` — контракт полей: `id, chat_id, sender_id, sender_name, sender_username, date` (RFC3339), `text, reply_to` (id|null), `media` (`{type,file_name,size,mime}`|null), `edited` (bool), `fwd_from` (string|null). Отправители — только из users/chats, приложенных к ответу API, без дорезолвов.
  - `ops.ParseDateArg(s string) (time.Time, error)` — экспортируемая (тестируется напрямую).

- [ ] **Step 1: Написать падающие тесты чистых функций**

`projects/tgc/internal/ops/messages_test.go`:

```go
package ops

import (
	"testing"
	"time"

	"github.com/gotd/td/tg"
)

func TestParseDateArg(t *testing.T) {
	d, err := ParseDateArg("2026-07-01")
	if err != nil {
		t.Fatal(err)
	}
	if d.Year() != 2026 || d.Month() != 7 || d.Day() != 1 {
		t.Fatalf("got %v", d)
	}
	d2, err := ParseDateArg("2026-07-01T15:04:05Z")
	if err != nil {
		t.Fatal(err)
	}
	if d2.Hour() != 15 {
		t.Fatalf("got %v", d2)
	}
	if _, err := ParseDateArg("yesterday"); err == nil {
		t.Fatal("want error for garbage")
	}
}

func TestMessageToMapBasics(t *testing.T) {
	msg := &tg.Message{
		ID:      42,
		Message: "hi",
		Date:    int(time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC).Unix()),
		FromID:  &tg.PeerUser{UserID: 7},
	}
	msg.SetReplyTo(&tg.MessageReplyHeader{ReplyToMsgID: 41})
	users := map[int64]*tg.User{7: {ID: 7, FirstName: "Vasya", Username: "vasya"}}

	m := messageToMap(msg, users, nil, 100)

	if m["id"] != 42 || m["chat_id"] != int64(100) || m["text"] != "hi" {
		t.Fatalf("basics: %+v", m)
	}
	if m["sender_id"] != int64(7) || m["sender_name"] != "Vasya" || m["sender_username"] != "vasya" {
		t.Fatalf("sender: %+v", m)
	}
	if m["reply_to"] != 41 {
		t.Fatalf("reply_to: %v", m["reply_to"])
	}
	if m["date"] != "2026-07-01T12:00:00Z" {
		t.Fatalf("date: %v", m["date"])
	}
}

func TestMessageToMapDocumentMedia(t *testing.T) {
	doc := &tg.Document{ID: 1, MimeType: "application/pdf", Size: 1234}
	doc.Attributes = []tg.DocumentAttributeClass{&tg.DocumentAttributeFilename{FileName: "report.pdf"}}
	msg := &tg.Message{ID: 1, Date: 0}
	msg.SetMedia(&tg.MessageMediaDocument{Document: doc})

	m := messageToMap(msg, nil, nil, 5)
	media, ok := m["media"].(map[string]any)
	if !ok {
		t.Fatalf("media missing: %+v", m)
	}
	if media["type"] != "document" || media["file_name"] != "report.pdf" ||
		media["mime"] != "application/pdf" || media["size"] != int64(1234) {
		t.Fatalf("media: %+v", media)
	}
}

func TestMessageToMapPhotoMedia(t *testing.T) {
	msg := &tg.Message{ID: 2}
	msg.SetMedia(&tg.MessageMediaPhoto{Photo: &tg.Photo{ID: 9}})
	m := messageToMap(msg, nil, nil, 5)
	media := m["media"].(map[string]any)
	if media["type"] != "photo" {
		t.Fatalf("media: %+v", media)
	}
}
```

Примечание: если сеттеры (`SetMedia`, `SetReplyTo`) в tg-пакете называются иначе / поля flags-опциональные конструируются по-другому — адаптируй конструирование в тестах, ассерты не меняй.

- [ ] **Step 2: Запустить — падают**

Run: `cd /root/workspace/projects/tgc && go test ./internal/ops/`
Expected: FAIL.

- [ ] **Step 3: Реализовать messages.go (read/context/messageToMap/ParseDateArg)**

Скелет ключевых частей:

```go
package ops

import (
	"time"

	"github.com/gotd/td/tg"

	"github.com/grigoreo-dev/tgc/internal/client"
	"github.com/grigoreo-dev/tgc/internal/output"
	"github.com/grigoreo-dev/tgc/internal/resolve"
)

func ParseDateArg(s string) (time.Time, error) {
	for _, layout := range []string{"2006-01-02", time.RFC3339} {
		if t, err := time.Parse(layout, s); err == nil {
			return t, nil
		}
	}
	return time.Time{}, output.Errf("bad_args", "cannot parse date %q (want YYYY-MM-DD or RFC3339)", s)
}

type ReadOpts struct {
	Limit    int
	BeforeID int
	AfterID  int
	Since    string
	Until    string
	From     string
	Search   string
}

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
		return collectMessages(res, peer.ID, sinceT), nil
	default:
		// Stress-test: use native MinID/MaxID (not AddOffset hack). One RPC = one page.
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
		return collectMessages(res, peer.ID, sinceT), nil
	}
}

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
	return collectMessages(res, peer.ID, time.Time{}), nil
}
```

`collectMessages(res tg.MessagesMessagesClass, chatID int64, since time.Time) []map[string]any` — достань `[]tg.MessageClass` + `Users` + `Chats` из конкретного типа ответа (`MessagesMessages`, `MessagesMessagesSlice`, `MessagesChannelMessages`), построй индексы users/chats по ID, прогони `messageToMap`, отбрось сообщения старше `since` (если задано). `messageToMap` реализуй строго по контракту тестов; `fwd_from` — имя из `FwdFrom().FromName` или резолв из users-индекса; `edited` — `EditDate != 0`; media-типы: photo → `"photo"`, document с video-атрибутом → `"video"`, с audio → `"audio"`/`"voice"`, иначе `"document"`.

- [ ] **Step 3a: Заменить заглушку SearchMessages (из Task 7)**

В `ops/chats.go` замени тело `SearchMessages` на реальную реализацию: `MessagesSearchGlobal{Q: query, Limit: limit, OffsetPeer: &tg.InputPeerEmpty{}, Filter: &tg.InputMessagesFilterEmpty{}}` → `collectMessages(res, 0, time.Time{})` (chat_id бери из peer каждого сообщения внутри collectMessages, когда он там есть).

- [ ] **Step 4: Реализовать cli/read.go**

```go
package cli

import (
	"strconv"

	"github.com/grigoreo-dev/tgc/internal/client"
	"github.com/grigoreo-dev/tgc/internal/ops"
	"github.com/grigoreo-dev/tgc/internal/output"
	"github.com/spf13/cobra"
)

var readOpts ops.ReadOpts
var contextRadius int

var readCmd = &cobra.Command{
	Use:   "read <chat>",
	Short: "Read chat history (newest first)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		conn, err := client.Connect(ProfileName())
		if err != nil {
			return err
		}
		defer conn.Close()
		msgs, err := ops.Read(conn, args[0], readOpts)
		if err != nil {
			return err
		}
		for _, m := range msgs {
			output.Emit(m)
		}
		return nil
	},
}

var contextCmd = &cobra.Command{
	Use:   "context <chat> <message_id>",
	Short: "Show a message with surrounding context",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		msgID, err := strconv.Atoi(args[1])
		if err != nil {
			return output.Errf("bad_args", "message_id must be a number, got %q", args[1])
		}
		conn, err := client.Connect(ProfileName())
		if err != nil {
			return err
		}
		defer conn.Close()
		msgs, err := ops.Context(conn, args[0], msgID, contextRadius)
		if err != nil {
			return err
		}
		for _, m := range msgs {
			output.Emit(m)
		}
		return nil
	},
}

func init() {
	readCmd.Flags().IntVar(&readOpts.Limit, "limit", 20, "max messages")
	readCmd.Flags().IntVar(&readOpts.BeforeID, "before", 0, "messages older than this id")
	readCmd.Flags().IntVar(&readOpts.AfterID, "after", 0, "messages newer than this id")
	readCmd.Flags().StringVar(&readOpts.Since, "since", "", "start date (YYYY-MM-DD or RFC3339)")
	readCmd.Flags().StringVar(&readOpts.Until, "until", "", "end date (YYYY-MM-DD or RFC3339)")
	readCmd.Flags().StringVar(&readOpts.From, "from", "", "only from this sender (selector)")
	readCmd.Flags().StringVar(&readOpts.Search, "search", "", "server-side search within chat")
	contextCmd.Flags().IntVar(&contextRadius, "radius", 10, "messages around the target")
	rootCmd.AddCommand(readCmd, contextCmd)
}
```

- [ ] **Step 5: Прогнать тесты и сборку**

Run: `cd /root/workspace/projects/tgc && go build ./... && go vet ./... && go test ./...`
Expected: PASS.

- [ ] **Step 6: Commit + PR**

```bash
cd /root/workspace/projects/tgc
git checkout -b task/8-read 2>/dev/null || git checkout task/8-read
git add -A && git commit -m "feat: read and context commands with date/sender/search filters"
git push -u origin task/8-read
gh pr create --fill
gh pr merge --squash --auto --delete-branch
git checkout main && git pull
```

Если PR не смержился сразу — дождись CI (`gh pr checks --watch`); красный CI чини в этой же ветке.

---

### Task 9: ops/messages — send, edit, delete, forward + CLI (текст)

Файлы/альбомы — Task 10. Здесь только текстовые операции.

**Files:**
- Modify: `projects/tgc/internal/ops/messages.go` (добавить функции)
- Create: `projects/tgc/internal/cli/send.go`

**Interfaces:**
- Consumes: `markup.Parse/ParsePlain`, `resolve.*`, `client.WrapErr`.
- Produces:
  - `type ops.SendOpts struct { ReplyTo int; Plain bool }`
  - `ops.SendText(conn *client.Conn, selector, text string, o SendOpts) (map[string]any, error)` → `{message_id, chat_id, date}`
  - `ops.EditText(conn *client.Conn, selector string, msgID int, text string, plain bool) (map[string]any, error)` → то же
  - `ops.Delete(conn *client.Conn, selector string, ids []int, forMe bool) (map[string]any, error)` → `{status:"ok", deleted:N}`; дефолт — у всех (revoke), `forMe` — только у себя
  - `ops.Forward(conn *client.Conn, fromSel string, msgID int, toSel string) (map[string]any, error)` → `{message_id, chat_id, date}`

- [ ] **Step 1: Реализовать операции в ops/messages.go**

```go
type SendOpts struct {
	ReplyTo int
	Plain   bool
}

func parseText(text string, plain bool) (string, []tg.MessageEntityClass, error) {
	if plain {
		return markup.ParsePlain(text)
	}
	return markup.Parse(text)
}

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
	req := &tg.MessagesSendMessageRequest{
		Peer:     ip,
		Message:  body,
		RandomID: randomID(),
	}
	if len(entities) > 0 {
		req.SetEntities(entities)
	}
	if o.ReplyTo > 0 {
		req.SetReplyTo(&tg.InputReplyToMessage{ReplyToMsgID: o.ReplyTo})
	}
	upd, err := conn.Ctx.Raw.MessagesSendMessage(conn.Ctx, req)
	if err != nil {
		return nil, client.WrapErr(err)
	}
	return sentResult(upd, peer.ID), nil
}
```

`randomID()` — `crypto/rand`-заполненный int64. `sentResult(upd tg.UpdatesClass, chatID int64) map[string]any` — вытащи `message_id` и `date`: для `*tg.UpdateShortSentMessage` — напрямую, для `*tg.Updates` — найди `UpdateNewMessage`/`UpdateNewChannelMessage`. Верни `{"message_id": id, "chat_id": chatID, "date": rfc3339}`.

`EditText` — `MessagesEditMessageRequest{Peer, ID, Message, Entities}`. `Delete` — для channel-peer: `ChannelsDeleteMessages` (у каналов нет опции «у себя»; `forMe` для канала → ошибка `bad_args`); для остальных: `MessagesDeleteMessages{Revoke: !forMe, ID: ids}`. `Forward` — `MessagesForwardMessages{FromPeer, ToPeer, ID: []int{msgID}, RandomID: []int64{randomID()}}`.

- [ ] **Step 2: Реализовать cli/send.go (текстовая часть)**

```go
package cli

import (
	"io"
	"os"
	"strconv"

	"github.com/grigoreo-dev/tgc/internal/client"
	"github.com/grigoreo-dev/tgc/internal/ops"
	"github.com/grigoreo-dev/tgc/internal/output"
	"github.com/spf13/cobra"
)

var (
	sendReply      int
	sendPlain      bool
	sendFiles      []string
	sendCaption    string
	sendAsDocument bool
	editPlain      bool
	deleteForMe    bool
)

func textArg(args []string, idx int) (string, error) {
	if len(args) > idx && args[idx] != "-" {
		return args[idx], nil
	}
	b, err := io.ReadAll(os.Stdin)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

var sendCmd = &cobra.Command{
	Use:   "send <chat> [text|-]",
	Short: "Send a message (Markdown by default; --file for media)",
	Args:  cobra.RangeArgs(1, 2),
	RunE: func(cmd *cobra.Command, args []string) error {
		conn, err := client.Connect(ProfileName())
		if err != nil {
			return err
		}
		defer conn.Close()

		if len(sendFiles) > 0 {
			results, err := ops.SendFiles(conn, args[0], sendFiles, ops.FileOpts{
				Caption: sendCaption, AsDocument: sendAsDocument, ReplyTo: sendReply, Plain: sendPlain,
			})
			if err != nil {
				return err
			}
			for _, r := range results {
				output.Emit(r)
			}
			return nil
		}

		text, err := textArg(args, 1)
		if err != nil {
			return err
		}
		if text == "" {
			return output.Errf("bad_args", "empty message: pass text, '-' for stdin, or --file")
		}
		res, err := ops.SendText(conn, args[0], text, ops.SendOpts{ReplyTo: sendReply, Plain: sendPlain})
		if err != nil {
			return err
		}
		output.Emit(res)
		return nil
	},
}

var editCmd = &cobra.Command{
	Use:   "edit <chat> <message_id> <text>",
	Short: "Edit a message",
	Args:  cobra.ExactArgs(3),
	RunE: func(cmd *cobra.Command, args []string) error {
		id, err := strconv.Atoi(args[1])
		if err != nil {
			return output.Errf("bad_args", "message_id must be a number")
		}
		conn, err := client.Connect(ProfileName())
		if err != nil {
			return err
		}
		defer conn.Close()
		res, err := ops.EditText(conn, args[0], id, args[2], editPlain)
		if err != nil {
			return err
		}
		output.Emit(res)
		return nil
	},
}

var deleteCmd = &cobra.Command{
	Use:   "delete <chat> <message_id>...",
	Short: "Delete messages (for everyone by default; --for-me to keep for others)",
	Args:  cobra.MinimumNArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		var ids []int
		for _, a := range args[1:] {
			id, err := strconv.Atoi(a)
			if err != nil {
				return output.Errf("bad_args", "message_id must be a number, got %q", a)
			}
			ids = append(ids, id)
		}
		conn, err := client.Connect(ProfileName())
		if err != nil {
			return err
		}
		defer conn.Close()
		res, err := ops.Delete(conn, args[0], ids, deleteForMe)
		if err != nil {
			return err
		}
		output.Emit(res)
		return nil
	},
}

var forwardCmd = &cobra.Command{
	Use:   "forward <from_chat> <message_id> <to_chat>",
	Short: "Forward a message",
	Args:  cobra.ExactArgs(3),
	RunE: func(cmd *cobra.Command, args []string) error {
		id, err := strconv.Atoi(args[1])
		if err != nil {
			return output.Errf("bad_args", "message_id must be a number")
		}
		conn, err := client.Connect(ProfileName())
		if err != nil {
			return err
		}
		defer conn.Close()
		res, err := ops.Forward(conn, args[0], id, args[2])
		if err != nil {
			return err
		}
		output.Emit(res)
		return nil
	},
}

func init() {
	sendCmd.Flags().IntVar(&sendReply, "reply", 0, "reply to message id")
	sendCmd.Flags().BoolVar(&sendPlain, "plain", false, "disable Markdown parsing")
	sendCmd.Flags().StringArrayVar(&sendFiles, "file", nil, "file to send (repeat for album, max 10)")
	sendCmd.Flags().StringVar(&sendCaption, "caption", "", "caption for file/album")
	sendCmd.Flags().BoolVar(&sendAsDocument, "as-document", false, "send image as document (default: photo for image/*)")
	editCmd.Flags().BoolVar(&editPlain, "plain", false, "disable Markdown parsing")
	deleteCmd.Flags().BoolVar(&deleteForMe, "for-me", false, "delete only for me")
	rootCmd.AddCommand(sendCmd, editCmd, deleteCmd, forwardCmd)
}
```

Примечание: `ops.SendFiles`/`ops.FileOpts` появятся в Task 10 — чтобы Task 9 компилировался самостоятельно, добавь в `ops/media.go` заглушку с честной ошибкой:

```go
// Package-level stub until media sending lands (Task 10).
type FileOpts struct {
	Caption    string
	AsDocument bool // force image/* as document; default for images is photo
	ReplyTo    int
	Plain      bool
}

func SendFiles(conn *client.Conn, selector string, files []string, o FileOpts) ([]map[string]any, error) {
	return nil, output.Errf("not_implemented", "file sending is not implemented yet")
}
```

- [ ] **Step 3: Сборка и тесты**

Run: `cd /root/workspace/projects/tgc && go build ./... && go vet ./... && go test ./...`
Expected: PASS.

- [ ] **Step 4: Commit + PR**

```bash
cd /root/workspace/projects/tgc
git checkout -b task/9-send 2>/dev/null || git checkout task/9-send
git add -A && git commit -m "feat: send, edit, delete, forward commands (text)"
git push -u origin task/9-send
gh pr create --fill
gh pr merge --squash --auto --delete-branch
git checkout main && git pull
```

Если PR не смержился сразу — дождись CI (`gh pr checks --watch`); красный CI чини в этой же ветке.

---

### Task 10: ops/media — upload/albums/download + CLI download

**Files:**
- Modify: `projects/tgc/internal/ops/media.go` (заменить заглушку), `projects/tgc/internal/cli/send.go` (ничего — уже вызывает SendFiles)
- Create: `projects/tgc/internal/cli/download.go`, `projects/tgc/internal/ops/media_test.go`

**Interfaces:**
- Consumes: `github.com/gotd/td/telegram/uploader`, `github.com/gotd/td/telegram/downloader`, `markup.Parse`, `resolve.*`.
- Produces:
  - `ops.SendFiles(conn, selector string, files []string, o FileOpts) ([]map[string]any, error)` — 1 файл → `messages.sendMedia`; 2–10 → `messages.sendMultiMedia` (album); >10 → ошибка `bad_args` «split into batches of 10». Caption (Markdown) — на первом элементе. Результат — по map на сообщение: `{message_id, chat_id, date, grouped_id?}`. Images default to photo; `FileOpts.AsDocument` forces document.
  - `ops.Download(conn *client.Conn, selector string, msgID int, outPath string, toStdout bool) (map[string]any, error)` → `{path, size, mime, file_name}`; `toStdout` — байты в stdout, JSON не печатается (это делает CLI-слой). Default path when `outPath==""`: `filepath.Join(downloadRoot(), fileID, originalName)` where `downloadRoot()` = `TGC_DOWNLOAD_DIR` or `~/.tgc/downloads`.
  - `ops.classifyUpload(path string, asDocument bool) (kind string, mime string)` — kind: `photo|video|audio|document`; image/* → `photo` unless `asDocument`.
  - `ops.uniquePath(path string) string` — если файл существует, добавляет ` (1)`, ` (2)`...
  - `ops.downloadRoot() string` — `TGC_DOWNLOAD_DIR` if set, else `filepath.Join(home, ".tgc", "downloads")`.

- [ ] **Step 1: Написать падающие тесты чистых функций**

`projects/tgc/internal/ops/media_test.go`:

```go
package ops

import (
	"os"
	"path/filepath"
	"testing"
)

func TestClassifyUpload(t *testing.T) {
	cases := []struct {
		path       string
		asDocument bool
		kind       string
	}{
		{"pic.jpg", false, "photo"},    // images are photos by default
		{"pic.jpg", true, "document"},  // --as-document forces document
		{"clip.mp4", false, "video"},
		{"song.mp3", false, "audio"},
		{"report.pdf", false, "document"},
		{"data.bin", true, "document"}, // as-document has no effect on non-images
	}
	for _, c := range cases {
		kind, _ := classifyUpload(c.path, c.asDocument)
		if kind != c.kind {
			t.Errorf("classifyUpload(%q, %v) = %q, want %q", c.path, c.asDocument, kind, c.kind)
		}
	}
}

func TestUniquePath(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "file.txt")
	if uniquePath(p) != p {
		t.Fatal("free path must be returned as-is")
	}
	_ = os.WriteFile(p, []byte("x"), 0o600)
	got := uniquePath(p)
	want := filepath.Join(dir, "file (1).txt")
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestSendFilesTooMany(t *testing.T) {
	files := make([]string, 11)
	for i := range files {
		files[i] = "f.jpg"
	}
	_, err := SendFiles(nil, "x", files, FileOpts{})
	if err == nil {
		t.Fatal("11 files must be rejected")
	}
}
```

- [ ] **Step 2: Запустить — падают**

Run: `cd /root/workspace/projects/tgc && go test ./internal/ops/ -run 'Classify|Unique|TooMany'`
Expected: FAIL по Classify/Unique (функций нет). TooMany с заглушкой формально пройдёт (заглушка всегда ошибается) — он станет осмысленным после реализации.

- [ ] **Step 3: Реализовать media.go**

Структура:

```go
package ops

import (
	"context"
	"mime"
	"os"
	"path/filepath"
	"strings"

	"github.com/gotd/td/telegram/downloader"
	"github.com/gotd/td/telegram/uploader"
	"github.com/gotd/td/tg"

	"github.com/grigoreo-dev/tgc/internal/client"
	"github.com/grigoreo-dev/tgc/internal/markup"
	"github.com/grigoreo-dev/tgc/internal/output"
	"github.com/grigoreo-dev/tgc/internal/resolve"
)

func classifyUpload(path string, asDocument bool) (string, string) {
	m := mime.TypeByExtension(strings.ToLower(filepath.Ext(path)))
	switch {
	case strings.HasPrefix(m, "image/") && !asDocument:
		return "photo", m
	case strings.HasPrefix(m, "video/"):
		return "video", m
	case strings.HasPrefix(m, "audio/"):
		return "audio", m
	default:
		if m == "" {
			m = "application/octet-stream"
		}
		return "document", m
	}
}

func downloadRoot() string {
	if d := os.Getenv("TGC_DOWNLOAD_DIR"); d != "" {
		return d
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".tgc", "downloads")
}

func uniquePath(p string) string {
	if _, err := os.Stat(p); os.IsNotExist(err) {
		return p
	}
	ext := filepath.Ext(p)
	base := strings.TrimSuffix(p, ext)
	for i := 1; ; i++ {
		cand := fmt.Sprintf("%s (%d)%s", base, i, ext)
		if _, err := os.Stat(cand); os.IsNotExist(err) {
			return cand
		}
	}
}
```

`SendFiles`: валидация count (≤10, иначе `output.Errf("bad_args", "too many files: %d (max 10 per album); split into batches", n)`) — **до** использования conn (тест зовёт с nil). Один файл: `uploader.NewUploader(conn.Ctx.Raw).FromPath(ctx, path)` → собери `tg.InputMediaUploadedPhoto` (kind photo) или `tg.InputMediaUploadedDocument` (+ `DocumentAttributeFilename`, MimeType; для video добавь `DocumentAttributeVideo{SupportsStreaming: true}`) → `MessagesSendMedia` с caption через `markup.Parse`. Альбом: для каждого файла после аплоада вызови `MessagesUploadMedia` (превращает uploaded в готовый `MessageMediaPhoto/Document` — обязательный шаг для `sendMultiMedia`), собери `[]tg.InputSingleMedia` (caption+entities только у первой), отправь `MessagesSendMultiMedia`. Результаты — из Updates (переиспользуй/обобщи `sentResult`; для мультимедиа собери все `UpdateNewMessage` и добавь `grouped_id`).

`Download`: найди сообщение — `ChannelsGetMessages`/`MessagesGetMessages` по ID (для channel-peer — первый, иначе второй). Из media построй location: photo → `tg.InputPhotoFileLocation` (максимальный size-тип), document → `tg.InputDocumentFileLocation`. Скачивание: `downloader.NewDownloader().Download(conn.Ctx.Raw, loc).ToPath(ctx, path)` / `.Stream(ctx, os.Stdout)` для stdout. Default when `-o` empty: `~/.tgc/downloads/<file_id>/<original_filename>` (`file_id` = document/photo id; original name from attributes, photos → `photo_<msgID>.jpg` if unnamed). Override root with `TGC_DOWNLOAD_DIR`. `-o` file or directory overrides fully; mkdir 0700 as needed; name conflicts → `uniquePath`. Нет media → `output.Errf("no_media", "message %d has no downloadable media", msgID)`.

- [ ] **Step 4: Реализовать cli/download.go**

```go
package cli

import (
	"strconv"

	"github.com/grigoreo-dev/tgc/internal/client"
	"github.com/grigoreo-dev/tgc/internal/ops"
	"github.com/grigoreo-dev/tgc/internal/output"
	"github.com/spf13/cobra"
)

var (
	downloadOut    string
	downloadStdout bool
)

var downloadCmd = &cobra.Command{
	Use:   "download <chat> <message_id>",
	Short: "Download media from a message",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		id, err := strconv.Atoi(args[1])
		if err != nil {
			return output.Errf("bad_args", "message_id must be a number")
		}
		conn, err := client.Connect(ProfileName())
		if err != nil {
			return err
		}
		defer conn.Close()
		res, err := ops.Download(conn, args[0], id, downloadOut, downloadStdout)
		if err != nil {
			return err
		}
		if !downloadStdout {
			output.Emit(res)
		}
		return nil
	},
}

func init() {
	downloadCmd.Flags().StringVarP(&downloadOut, "out", "o", "", "output file or directory (default: ~/.tgc/downloads/<file_id>/<name>)")
	downloadCmd.Flags().BoolVar(&downloadStdout, "stdout", false, "write raw bytes to stdout")
	rootCmd.AddCommand(downloadCmd)
}
```

- [ ] **Step 5: Прогнать тесты и сборку**

Run: `cd /root/workspace/projects/tgc && go build ./... && go vet ./... && go test ./...`
Expected: PASS.

- [ ] **Step 6: Commit + PR**

```bash
cd /root/workspace/projects/tgc
git checkout -b task/10-media 2>/dev/null || git checkout task/10-media
git add -A && git commit -m "feat: file upload (single + albums), media download"
git push -u origin task/10-media
gh pr create --fill
gh pr merge --squash --auto --delete-branch
git checkout main && git pull
```

Если PR не смержился сразу — дождись CI (`gh pr checks --watch`); красный CI чини в этой же ветке.

---

### Task 11: RichMessage — InputRichMessageMarkdown + fallback + `--rich`

**Stress-test decision:** gotd v0.153 (Layer 227) already has `InputRichMessageMarkdown`, `InputRichMessageHTML`, `InputRichMessage` (PageBlock), and `messages.sendMessage.rich_message`. Do **not** build a custom PageBlock tree in v1. Prefer server-side Markdown constructor; fall back to Task 6 entities if the server rejects rich.

**Files:**
- Create: `projects/tgc/internal/markup/rich.go`, `projects/tgc/internal/markup/rich_test.go`, `projects/tgc/docs/rich-spike.md`
- Modify: `projects/tgc/internal/ops/messages.go` (SendText rich path), `projects/tgc/internal/cli/send.go` (`--rich`)

**Interfaces:**
- Produces:
  - `markup.TryRichMarkdown(md string) tg.InputRichMessageClass` — returns `&tg.InputRichMessageMarkdown{Markdown: md}` (or HTML path if we choose it); no custom AST required in v1.
  - `ops.SendOpts` gains `RichJSON string` (expert `--rich` payload) and uses rich when Markdown has block structure **or** when sending default Markdown if a prior probe/session flag says rich works.
  - SendText path: (1) if `RichJSON != ""` → decode JSON into `InputRichMessageClass` (markdown/html/blocks forms); (2) else if not plain → try `req.SetRichMessage(InputRichMessageMarkdown{Markdown: text})` with empty `Message` or message+rich per TL; on RPC failure indicating unsupported → retry with entities fallback from Task 6; (3) plain → entities-free string.
  - User and bot both attempt rich; if user-layer rejects, transparent entities fallback (no error). Document outcome in `docs/rich-spike.md` after first live probe in Task 13 (spike notes from module inspection go there in this task).

- [ ] **Step 1: Spike notes from module (already verified in stress-test)**

```bash
cd /root/workspace/projects/tgc
grep -n 'InputRichMessageMarkdown\|SetRichMessage' $(go env GOMODCACHE)/github.com/gotd/td@*/tg/tl_input_rich_message_gen.go $(go env GOMODCACHE)/github.com/gotd/td@*/tg/tl_messages_send_message_gen.go | head -20
# Layer constant:
grep -n 'const Layer' $(go env GOMODCACHE)/github.com/gotd/td@*/tg/tl_registry_gen.go | head -3
```

Write `projects/tgc/docs/rich-spike.md`: types present (`InputRichMessageMarkdown/HTML`, `rich_message` flag on send/edit), Layer 227, decision = use Markdown constructor + entities fallback; no custom PageBlock translator in v1.

- [ ] **Step 2: Unit tests for rich helpers**

`projects/tgc/internal/markup/rich_test.go`:

```go
package markup

import (
	"encoding/json"
	"testing"

	"github.com/gotd/td/tg"
)

func TestHasBlockContent(t *testing.T) {
	if HasBlockContent("just plain text") {
		t.Fatal("plain text is not block content")
	}
	if !HasBlockContent("# heading\ntext") {
		t.Fatal("heading is block content")
	}
	if !HasBlockContent("| a |\n|---|\n| 1 |") {
		t.Fatal("table is block content")
	}
}

func TestTryRichMarkdown(t *testing.T) {
	rm := TryRichMarkdown("# Hi\n\nbody")
	md, ok := rm.(*tg.InputRichMessageMarkdown)
	if !ok || md.Markdown != "# Hi\n\nbody" {
		t.Fatalf("got %#v", rm)
	}
}

func TestParseRichJSONMarkdown(t *testing.T) {
	raw := `{"type":"markdown","markdown":"# x"}`
	rm, err := ParseRichJSON(json.RawMessage(raw))
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := rm.(*tg.InputRichMessageMarkdown); !ok {
		t.Fatalf("want markdown constructor, got %T", rm)
	}
}
```

- [ ] **Step 3: Run — fail**

Run: `cd /root/workspace/projects/tgc && go test ./internal/markup/ -run 'HasBlock|TryRich|ParseRichJSON'`
Expected: FAIL.

- [ ] **Step 4: Implement rich.go (no PageBlock AST)**

```go
package markup

import (
	"encoding/json"
	"strings"

	"github.com/gotd/td/tg"

	"github.com/grigoreo-dev/tgc/internal/output"
)

// HasBlockContent reports whether md has block-level markup (headings/tables/lists/fences/quotes).
func HasBlockContent(md string) bool {
	for _, line := range strings.Split(md, "\n") {
		t := strings.TrimSpace(line)
		if strings.HasPrefix(t, "#") || strings.HasPrefix(t, "|") ||
			strings.HasPrefix(t, "- ") || strings.HasPrefix(t, "* ") ||
			strings.HasPrefix(t, "```") || strings.HasPrefix(t, "> ") {
			return true
		}
	}
	return false
}

// TryRichMarkdown wraps Markdown in InputRichMessageMarkdown for sendMessage.rich_message.
func TryRichMarkdown(md string) tg.InputRichMessageClass {
	return &tg.InputRichMessageMarkdown{Markdown: md}
}

// ParseRichJSON decodes expert --rich payload:
//   {"type":"markdown","markdown":"..."} | {"type":"html","html":"..."} | {"type":"blocks",...}
func ParseRichJSON(raw json.RawMessage) (tg.InputRichMessageClass, error) {
	var head struct {
		Type     string `json:"type"`
		Markdown string `json:"markdown"`
		HTML     string `json:"html"`
	}
	if err := json.Unmarshal(raw, &head); err != nil {
		return nil, output.Errf("bad_args", "invalid --rich JSON: %v", err)
	}
	switch strings.ToLower(head.Type) {
	case "markdown", "md", "":
		if head.Markdown == "" {
			return nil, output.Errf("bad_args", "--rich markdown payload requires markdown field")
		}
		return &tg.InputRichMessageMarkdown{Markdown: head.Markdown}, nil
	case "html":
		if head.HTML == "" {
			return nil, output.Errf("bad_args", "--rich html payload requires html field")
		}
		return &tg.InputRichMessageHTML{HTML: head.HTML}, nil
	default:
		return nil, output.Errf("bad_args", "unsupported --rich type %q (markdown|html)", head.Type)
	}
}
```

- [ ] **Step 5: Tests pass**

Run: `cd /root/workspace/projects/tgc && go test ./internal/markup/ -run 'HasBlock|TryRich|ParseRichJSON' -v`
Expected: PASS.

- [ ] **Step 6: Integrate into SendText**

In `ops.SendText`: if `o.RichJSON != ""` → `ParseRichJSON` + `req.SetRichMessage(...)`; else if `!o.Plain` → try `SetRichMessage(TryRichMarkdown(text))` (especially when `HasBlockContent(text)`); on RPC failure that indicates unsupported rich → retry with Task 6 entities. CLI: `--rich` string flag. Document live user-account result later in Task 13 (update `docs/rich-spike.md`).

- [ ] **Step 7: Build + all tests**

Run: `cd /root/workspace/projects/tgc && go build ./... && go vet ./... && go test ./...`
Expected: PASS.

- [ ] **Step 8: Commit + PR**

```bash
cd /root/workspace/projects/tgc
git checkout -b task/11-rich 2>/dev/null || git checkout task/11-rich
git add -A && git commit -m "feat: RichMessage via InputRichMessageMarkdown with entities fallback"
git push -u origin task/11-rich
gh pr create --fill
gh pr merge --squash --auto --delete-branch
git checkout main && git pull
```

Если PR не смержился сразу — дождись CI (`gh pr checks --watch`); красный CI чини в этой же ветке.

---

### Task 12: --pretty вывод

**Files:**
- Modify: `projects/tgc/internal/output/output.go`, `projects/tgc/internal/output/output_test.go`

**Interfaces:**
- Produces:
  - `output.SetPretty(on bool)` — вызывается из `cli.Execute` до выполнения команды (`cobra.OnInitialize` или PersistentPreRun).
  - В pretty-режиме `Emit`/`EmitAll` рендерят ключевые типы таблично/строчно; неизвестные map — выровненный `key: value`. Цвета — только если stdout TTY и `NO_COLOR` не установлен.
  - `output.IsTTY(f *os.File) bool`.

- [ ] **Step 1: Написать падающие тесты**

Добавить в `output_test.go`:

```go
func TestPrettyMapRendering(t *testing.T) {
	var buf bytes.Buffer
	stdout = &buf
	defer func() { stdout = defaultStdout; SetPretty(false) }()
	SetPretty(true)

	Emit(map[string]any{"id": 42, "title": "Chat"})

	got := buf.String()
	if strings.Contains(got, "{") {
		t.Fatalf("pretty must not print raw JSON: %q", got)
	}
	if !strings.Contains(got, "42") || !strings.Contains(got, "Chat") {
		t.Fatalf("values missing: %q", got)
	}
}

func TestPrettyNoColorWhenNotTTY(t *testing.T) {
	var buf bytes.Buffer
	stdout = &buf
	defer func() { stdout = defaultStdout; SetPretty(false) }()
	SetPretty(true)

	Emit(map[string]any{"a": 1})
	if strings.Contains(buf.String(), "\x1b[") {
		t.Fatalf("ANSI codes in non-TTY output: %q", buf.String())
	}
}
```

- [ ] **Step 2: Запустить — падают**

Run: `cd /root/workspace/projects/tgc && go test ./internal/output/ -run Pretty`
Expected: FAIL.

- [ ] **Step 3: Реализовать pretty-рендер**

В `output.go`: глобальный `prettyMode bool` + `SetPretty`. В `Emit`: если pretty — `renderPretty(v)`: map → отсортированные `key: value` строки + пустая строка-разделитель; срезы — рекурсивно. Цвет (ключи — dim): только `IsTTY(os.Stdout) && os.Getenv("NO_COLOR") == ""` — проверка по `os.Stdout`, не по подменённому writer (тест выше это гарантирует: buf не TTY... но writer подменён; поэтому проверяй `stdout == defaultStdout && IsTTY(os.Stdout)`). `IsTTY`: `(f.Fd())` через `golang.org/x/term.IsTerminal(int(f.Fd()))` — добавь зависимость `golang.org/x/term`. В `cli/root.go` добавь `cobra.OnInitialize(func() { output.SetPretty(flagPretty) })` в `Execute`.

- [ ] **Step 4: Прогнать тесты, ручная проверка**

Run:
```bash
cd /root/workspace/projects/tgc && go test ./... && go build ./...
TGC_CONFIG_DIR=$(mktemp -d) go run ./cmd/tgc auth list --pretty
```
Expected: тесты PASS; pretty-вывод без JSON-скобок.

- [ ] **Step 5: Commit + PR**

```bash
cd /root/workspace/projects/tgc
git checkout -b task/12-pretty 2>/dev/null || git checkout task/12-pretty
git add -A && git commit -m "feat: --pretty human-readable output with TTY/NO_COLOR detection"
git push -u origin task/12-pretty
gh pr create --fill
gh pr merge --squash --auto --delete-branch
git checkout main && git pull
```

Если PR не смержился сразу — дождись CI (`gh pr checks --watch`); красный CI чини в этой же ветке.

---

### Task 13: Живой интеграционный прогон, README, документация meta-репо

**Files:**
- Create: `projects/tgc/docs/integration-checklist.md`
- Modify: `projects/tgc/README.md`
- Create (meta-repo): `docs/knowledge-base/technical/tgc.md`
- Modify (meta-repo): `docs/backlog/current.md`

**Interfaces:**
- Consumes: всё выше.
- Produces: задокументированный чек-лист живой проверки + запись в базе знаний meta-репо + зафиксированный указатель submodule.

- [ ] **Step 1: Чек-лист живой проверки**

`projects/tgc/docs/integration-checklist.md` — исполняется человеком/агентом с реальными креденшелами (нужны `TGC_API_ID`/`TGC_API_HASH` и тестовый аккаунт + тестовый бот):

```markdown
# tgc live integration checklist

Prereqs: TGC_API_ID/TGC_API_HASH set; a test user account and a test bot token.
Use a throwaway profile dir: `export TGC_CONFIG_DIR=$(mktemp -d)`.

## Auth
- [ ] `tgc auth login` — interactive user login completes; JSON `{"status":"ok",...}`
- [ ] `tgc auth login --profile bot --bot-token $TOKEN` — bot login ok
- [ ] `tgc auth list` — two profiles, correct types
- [ ] `tgc auth export -o /tmp/s.txt` + fresh TGC_CONFIG_DIR + `tgc auth import /tmp/s.txt` — session survives
- [ ] `tgc auth logout bot` — profile gone

## Chats / resolve
- [ ] `tgc chats --limit 5` — JSONL dialogs; second run instant (cache)
- [ ] `tgc chats --fresh` — refreshes
- [ ] `tgc info @<known_user>` / `tgc info <chat_id>` — cards ok
- [ ] `tgc search <partial_name>` — candidates; ambiguous fuzzy send errors with candidates
- [ ] `tgc members <test_group>` — members with statuses

## Messages
- [ ] `tgc send @<self_or_test> "plain hi"` — arrives; JSON has message_id
- [ ] `tgc send ... "**bold** and \`code\`"` — formatting renders in the app
- [ ] `tgc send ... "# Title\n- a\n- b"` — bot profile: RichMessage (if supported), user: degraded entities
- [ ] `tgc send ... --reply <id>` — reply linked
- [ ] `echo "from stdin" | tgc send <chat> -` — works
- [ ] `tgc read <chat> --limit 5` — newest first, fields per contract
- [ ] `tgc read <chat> --search <word>` / `--from @user` / `--since` — filters work
- [ ] `tgc context <chat> <id> --radius 3` — window around message
- [ ] `tgc edit <chat> <id> "edited"` — edited flag visible on re-read
- [ ] `tgc forward <chat> <id> <other_chat>` — forwarded
- [ ] `tgc delete <chat> <id>` — gone for everyone

## Files
- [ ] `tgc send <chat> --file photo.jpg --caption "**cap**"` — photo by default with caption
- [ ] `tgc send <chat> --file photo.jpg --as-document` — image as document
- [ ] `tgc send <chat> --file a.jpg --file b.jpg` — album (grouped)
- [ ] `tgc send <chat> --file doc.pdf` — document
- [ ] `tgc download <chat> <id>` — file under `~/.tgc/downloads/<file_id>/<name>`, JSON `{path,size,mime,file_name}`
- [ ] `tgc download <chat> <id> --stdout > /tmp/out` — bytes match
- [ ] 11 files → error `bad_args`

## Bot mode restrictions
- [ ] `tgc --profile bot chats` — preflight `bot_unsupported` (no dialogs RPC)
- [ ] `tgc --profile bot search foo` (no --messages) — preflight `bot_unsupported`
- [ ] `tgc --profile bot send <chat_id> "hi from bot"` — works for a chat the bot is in

## Contract
- [ ] any error: stdout empty, stderr one JSON line, exit 1
- [ ] `--pretty` renders humanly; piping without it gives pure JSONL
```

- [ ] **Step 2: Прогнать чек-лист**

Выполни чек-лист с реальными креденшелами (запросить у пользователя, если не заданы). Найденные баги — чинить в этой же задаче, каждая правка — обычный TDD-цикл, где воспроизводимо юнит-тестом.

- [ ] **Step 3: Финальный README (EN) + README.ru.md (RU)**

Обнови `projects/tgc/README.md` (английский): краткое описание, install (`go install github.com/grigoreo-dev/tgc/cmd/tgc@latest`), quick start (login → chats → send → read), таблица команд, переменные окружения (`TGC_PROFILE`, `TGC_API_ID`, `TGC_API_HASH`, `TGC_SESSION`, `TGC_CONFIG_DIR`, `NO_COLOR`), контракт вывода, ограничения bot-режима, статус RichMessage (по итогу спайка). В шапку добавь ссылку `[Русская версия](README.ru.md)`.

Создай `projects/tgc/README.ru.md` — полный перевод README на русский, со ссылкой `[English](README.md)` в шапке. Это **единственный** файл на русском в репозитории tgc.

- [ ] **Step 4: Commit + PR (submodule)**

```bash
cd /root/workspace/projects/tgc
git checkout -b task/13-integration 2>/dev/null || git checkout task/13-integration
git add -A && git commit -m "docs: integration checklist, full README"
git push -u origin task/13-integration
gh pr create --fill
gh pr merge --squash --auto --delete-branch
git checkout main && git pull
```

- [ ] **Step 5: Документация meta-репо + указатель submodule**

`docs/knowledge-base/technical/tgc.md` (meta-репо): что такое tgc, где живёт, как устроены профили, контракт вывода, ссылки на спек/план, статус v1 и что отложено (группы-управление, webhook/MCP/Bot API-шлюз). Обнови `docs/backlog/current.md` (запись о tgc v1 → Сделано). Зафиксируй submodule:

```bash
cd /root/workspace
git add projects/tgc .gitmodules docs/knowledge-base/technical/tgc.md docs/backlog/current.md
git commit -m "feat: add tgc submodule (v1) + knowledge base entry

- projects/tgc: agent-first Telegram CLI (Go)
- docs/knowledge-base/technical/tgc.md
- docs/backlog/current.md updated"
git push
```

---

## Порядок и зависимости

```
Task 1 (scaffold+CI+branch protection)
  → Task 2 (config) → Task 3 (client) → Task 4 (auth)
  → Task 5 (resolve) → Task 6 (markup) — независимы друг от друга, оба после 3
  → Task 7 (chats CLI) — после 5
  → Task 8 (read) — после 5
  → Task 9 (send text) — после 5+6
  → Task 10 (media) — после 9
  → Task 11 (rich) — после 9
  → Task 12 (pretty) — независима, после 1
  → Task 13 (integration+docs) — последняя
```

## Verification (весь план)

- Каждая задача: `go build ./... && go vet ./... && go test ./...` зелёные, PR смержен зелёным CI.
- Финально: живой чек-лист Task 13 пройден (credentials from `.env` + user-bot login), submodule-указатель зафиксирован в meta-репо, документация meta-репо обновлена.

---

## Stress Test Results: tgc v1 remaining plan (Tasks 8–13)

### Resolved Decisions
- **RichMessage path:** Use gotd `InputRichMessageMarkdown` / HTML / optional JSON blocks via `--rich`; no custom PageBlock AST in v1; entities fallback on server reject.
- **gotd version:** Stay on current pin (0.153 / Layer 227); optional gotd-only bump before Task 8 with build+test gate; no forced gotgproto upgrade.
- **read --after/--before:** Native `MinID`/`MaxID` on `messages.getHistory`; one RPC = one page; drop AddOffset hack.
- **Session security:** Keep export/import; files `0600`; never log sessions; README warns sessions = passwords; no encryption-at-rest in v1.
- **bot_unsupported:** Preflight deny-list for bot profile (chats, search without `--messages`, phone/fuzzy resolve) + reactive `BOT_METHOD_INVALID` safety net (not broad `*_INVALID`).
- **Task order:** Serial 8 → 9 → 10 → 11 → 12 → 13.
- **Task 13 live:** Required to close v1; use `.env` API/bot tokens + interactive user-bot login; never commit/log secrets.
- **Media send:** Images default to photo; `--as-document` forces document (replaces plan's `--as-photo` default-document model).
- **Download path:** `TGC_DOWNLOAD_DIR` or `~/.tgc/downloads/<file_id>/<original_filename>`; `-o` override; `uniquePath` on conflict; `--stdout` raw.

### Changes Made
- Global Constraints updated with gotd pin, bot preflight, session security, media/download, pagination, live-gate.
- Task 8 sample code: MinID/MaxID instead of AddOffset.
- Task 9/10: `--as-document` / `AsDocument`; download default under `~/.tgc/downloads/...`.
- Task 11 rewritten around InputRichMessageMarkdown (no PageBlock tree).
- Task 13 checklist aligned with media/bot decisions.

### Deferred / Parking Lot
- Custom PageBlock tree / `ParseRich` AST — only if server rejects Markdown constructor in live probe.
- gotgproto upgrade — only if blocked by API gap.
- Multi-RPC history paginator — out of v1.
- Session encryption-at-rest — out of v1.

### Confidence Assessment
- Overall: **High** for Tasks 8–10, 12; **Medium** for Task 11 (server-side rich acceptance for user accounts needs live probe); **High** process for Task 13 given credentials available.
- Areas of concern: user-account RichMessage acceptance; bot preflight false positives if Telegram expands bot methods — mitigate with narrow deny-list + reactive net.
