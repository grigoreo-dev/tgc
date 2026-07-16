// Package client bootstraps the gotgproto Telegram client for a tgc profile.
package client

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/celestix/gotgproto"
	gotgerrors "github.com/celestix/gotgproto/errors"
	"github.com/celestix/gotgproto/ext"
	"github.com/celestix/gotgproto/sessionMaker"
	"github.com/glebarez/sqlite"
	"github.com/gotd/contrib/middleware/floodwait"
	"github.com/gotd/td/telegram"
	"github.com/gotd/td/tgerr"
	"golang.org/x/term"

	"github.com/grigoreo-dev/tgc/internal/config"
	"github.com/grigoreo-dev/tgc/internal/output"
)

// maxAutoFloodWait: FLOOD_WAIT up to this duration is retried transparently.
const maxAutoFloodWait = 30 * time.Second

// Conn is an established Telegram connection bound to a tgc profile.
type Conn struct {
	Client  *gotgproto.Client
	Ctx     *ext.Context
	Profile *config.Profile
}

// Close stops the underlying client and releases its resources.
func (c *Conn) Close() { c.Client.Stop() }

// sessionFor picks the session backend for a profile: an imported string
// session (<profile>/session.txt) if present, otherwise a SQLite database at
// the profile's session path (pure-Go driver, no CGO).
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

// build creates the gotgproto client. gotgproto's client type interface is
// unexported (ClientTypePhone/ClientTypeBot return an unnameable type), so
// build takes (bot, secret) and constructs it internally.
func build(p *config.Profile, bot bool, secret string, opts *gotgproto.ClientOpts) (*Conn, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, err
	}
	apiID, apiHash, err := config.APICredentials(cfg)
	if err != nil {
		return nil, err
	}
	ctype := gotgproto.ClientTypePhone(secret)
	if bot {
		ctype = gotgproto.ClientTypeBot(secret)
	}
	c, err := gotgproto.NewClient(apiID, apiHash, ctype, opts)
	if err != nil {
		return nil, WrapErr(err)
	}
	return &Conn{Client: c, Ctx: c.CreateContext(), Profile: p}, nil
}

// isNotAuthenticated reports whether err means the stored session is
// missing, invalid, or revoked (as opposed to e.g. a network failure).
func isNotAuthenticated(err error) bool {
	if errors.Is(err, gotgerrors.ErrSessionUnauthorized) {
		return true
	}
	return tgerr.Is(err,
		"AUTH_KEY_UNREGISTERED", "AUTH_KEY_INVALID", "AUTH_KEY_PERM_EMPTY",
		"SESSION_REVOKED", "SESSION_EXPIRED", "USER_DEACTIVATED",
		"ACCESS_TOKEN_INVALID", "ACCESS_TOKEN_EXPIRED",
	)
}

// Connect opens a connection using the existing profile session.
// It never prompts: an invalid/absent session yields a structured
// "not_authenticated" error.
func Connect(profileName string) (*Conn, error) {
	p, err := config.ResolveProfile(profileName)
	if err != nil {
		return nil, err
	}
	// The bot token / phone lives inside the stored session; the empty
	// secret is only consulted when the session is invalid, in which case
	// the resulting error is mapped to not_authenticated below.
	conn, err := build(p, p.Type == "bot", "", &gotgproto.ClientOpts{
		Session:          sessionFor(p),
		NoUpdates:        true,
		NoAutoAuth:       true,
		DisableCopyright: true,
		Middlewares:      middlewares(),
	})
	if err != nil {
		var oe *output.Error
		if output.AsError(err, &oe) {
			return nil, err // already structured (config, flood_wait, ...)
		}
		if isNotAuthenticated(err) {
			return nil, output.Errf("not_authenticated",
				"profile %q has no valid session (%v); run `tgc auth login` or `tgc auth import`", p.Name, err)
		}
		return nil, err
	}
	return conn, nil
}

// isAuthRestart reports whether err is Telegram's AUTH_RESTART, the protocol
// signal to discard the current auth state and start the flow over.
func isAuthRestart(err error) bool {
	return tgerr.Is(err, "AUTH_RESTART")
}

// resetSession removes a profile's persisted session state so the next auth
// flow starts clean: the sqlite database (plus its WAL/SHM sidecars) and any
// imported string session. Missing files are not an error.
func resetSession(p *config.Profile) error {
	paths := []string{
		p.SessionPath,
		p.SessionPath + "-wal",
		p.SessionPath + "-shm",
		filepath.Join(p.Dir, "session.txt"),
	}
	for _, path := range paths {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}

// ConnectForLogin runs the auth flow: interactive terminal prompts for user
// accounts, non-interactive for bot tokens. Persists profile type on success.
//
// If Telegram answers AUTH_RESTART (stale/partial auth state from a previous
// interrupted attempt), the leftover session is cleared and the flow is
// retried once transparently, so callers never have to delete session files
// by hand.
func ConnectForLogin(profileName, phone, botToken string) (*Conn, error) {
	p, err := config.ResolveProfile(profileName)
	if err != nil {
		return nil, err
	}
	bot := botToken != ""
	secret, ptype := phone, "user"
	if bot {
		secret, ptype = botToken, "bot"
	}

	attempt := func() (*Conn, error) {
		return build(p, bot, secret, &gotgproto.ClientOpts{
			Session:          sessionFor(p),
			NoUpdates:        true,
			DisableCopyright: true,
			Middlewares:      middlewares(),
			AuthConversator:  newTerminalConversator(),
		})
	}

	conn, err := attempt()
	if err != nil && isAuthRestart(err) {
		// Telegram wants a clean slate: drop the partial session and retry once.
		fmt.Fprintln(os.Stderr, "auth: AUTH_RESTART — clearing partial session and retrying")
		if rerr := resetSession(p); rerr != nil {
			return nil, rerr
		}
		conn, err = attempt()
	}
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
	if tgerr.Is(err, "RICH_MESSAGE_UNSUPPORTED") {
		return output.Errf("rich_unsupported",
			"rich messages are not supported for this account; omit --rich or send default Markdown")
	}
	return err
}

// terminalConversator prompts on stderr and reads answers from stdin.
type terminalConversator struct {
	in     *bufio.Reader
	status gotgproto.AuthStatus
}

func newTerminalConversator() *terminalConversator {
	return &terminalConversator{in: bufio.NewReader(os.Stdin)}
}

func (t *terminalConversator) prompt(label string) (string, error) {
	return t.readAnswer(label, false, term.IsTerminal(int(os.Stdin.Fd())))
}

// promptSecret reads a sensitive answer (e.g. a 2FA password) without echoing
// it to the terminal.
func (t *terminalConversator) promptSecret(label string) (string, error) {
	return t.readAnswer(label, true, term.IsTerminal(int(os.Stdin.Fd())))
}

// readAnswer prints label to stderr and reads one answer from stdin. When
// secret is set and stdin is a real terminal, input is read with echo
// disabled (via term.ReadPassword) so passwords never appear on screen or in
// the scrollback. When stdin is not a TTY (piped input, automation, tests),
// it falls back to a normal buffered read so those flows still work.
func (t *terminalConversator) readAnswer(label string, secret, isTTY bool) (string, error) {
	if _, err := fmt.Fprint(os.Stderr, label); err != nil {
		return "", err
	}
	if secret && isTTY {
		b, err := term.ReadPassword(int(os.Stdin.Fd()))
		// ReadPassword consumes the newline but does not echo it; emit one so
		// the next prompt/output starts on a fresh line.
		fmt.Fprintln(os.Stderr)
		if err != nil {
			return "", err
		}
		return strings.TrimSpace(string(b)), nil
	}
	s, err := t.in.ReadString('\n')
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(s), nil
}

func (t *terminalConversator) retryNote(event gotgproto.AuthStatusEvent, what string) {
	if t.status.Event == event {
		fmt.Fprintf(os.Stderr, "%s was not accepted, %d attempt(s) left; try again.\n",
			what, t.status.AttemptsLeft)
	}
}

func (t *terminalConversator) AskPhoneNumber() (string, error) {
	t.retryNote(gotgproto.AuthStatusPhoneRetrial, "phone number")
	return t.prompt("Phone number (intl format): ")
}

func (t *terminalConversator) AskCode() (string, error) {
	t.retryNote(gotgproto.AuthStatusPhoneCodeRetrial, "code")
	return t.prompt("Code from Telegram: ")
}

func (t *terminalConversator) AskPassword() (string, error) {
	t.retryNote(gotgproto.AuthStatusPasswordRetrial, "2FA password")
	return t.promptSecret("2FA password: ")
}

func (t *terminalConversator) AuthStatus(s gotgproto.AuthStatus) {
	t.status = s
	fmt.Fprintf(os.Stderr, "auth: %s\n", s.Event)
}
