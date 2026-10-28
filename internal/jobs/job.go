package jobs

import (
	"errors"
	"fmt"
	"net/url"
	"path/filepath"
	"regexp"
	"strings"
	"unicode/utf8"

	"driver/internal/store"
)

// Refusal is a validation refusal: the API answers 409 with its text.
type Refusal struct{ Msg string }

func (r *Refusal) Error() string { return r.Msg }

// Refuse builds a Refusal.
func Refuse(format string, a ...any) error { return &Refusal{Msg: fmt.Sprintf(format, a...)} }

// IsRefusal reports whether err is (or wraps) a Refusal.
func IsRefusal(err error) bool {
	var r *Refusal
	return errors.As(err, &r)
}

// ErrBadInput marks a malformed request value (the API answers 400).
var ErrBadInput = errors.New("bad input")

// Columns are the four board columns; also the display states.
var Columns = []string{"draft", "working", "blocked", "done"}

// IsColumn reports whether c is a board column name.
func IsColumn(c string) bool {
	for _, x := range Columns {
		if x == c {
			return true
		}
	}
	return false
}

// DisplayState maps a stored state onto the board column: ” (not started),
// ready (pi open, nothing sent) and anything unknown read as draft; working,
// blocked and done read as themselves.
func DisplayState(j *store.Job) string {
	switch j.State {
	case "working", "blocked", "done":
		return j.State
	}
	return "draft"
}

// Opened reports whether the job has (or had) a pi session: every stored
// state but ” — the overlay shows the pane rather than the launch form.
func Opened(j *store.Job) bool { return j.State != "" }

// Palette is the eight card colours (kraken's names; the UI holds the hexes).
var Palette = []string{"red", "blue", "green", "yellow", "purple", "orange", "pink", "cyan"}

// ValidColor reports whether c is a palette name ("" is the clear and is not
// a colour).
func ValidColor(c string) bool {
	for _, p := range Palette {
		if p == c {
			return true
		}
	}
	return false
}

var slugRe = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,63}$`)

// ValidSlug reports whether s satisfies ^[a-z0-9][a-z0-9-]{0,63}$.
func ValidSlug(s string) bool { return slugRe.MatchString(s) }

// MaxNameLen bounds a job's name (in runes): one line on a card.
const MaxNameLen = 200

// CleanName normalises the name the operator typed: whitespace runs collapse
// to one space and the ends are trimmed. Longer than MaxNameLen is
// ErrBadInput; "" is fine (the job then shows its slug).
func CleanName(s string) (string, error) {
	s = strings.Join(strings.Fields(s), " ")
	if utf8.RuneCountInString(s) > MaxNameLen {
		return "", fmt.Errorf("name is too long (max %d characters): %w", MaxNameLen, ErrBadInput)
	}
	return s, nil
}

// FreshSessionName is the tmux session a job is launched in: drv-<slug>-<id>.
func FreshSessionName(j *store.Job) string { return fmt.Sprintf("drv-%s-%d", j.Slug, j.ID) }

// SessionName is the job's recorded tmux session, or the fresh name when
// none was recorded yet.
func SessionName(j *store.Job) string {
	if j.Tmux != "" {
		return j.Tmux
	}
	return FreshSessionName(j)
}

// FreshJobDir is <home>/jobs/<slug>-<id>, the dir a new launch uses.
func (c Config) FreshJobDir(j *store.Job) string {
	return filepath.Join(c.JobsDir(), fmt.Sprintf("%s-%d", j.Slug, j.ID))
}

// JobDir is where a job's session.jsonl / status.json live: the
// directory of its recorded session file (a rename does not move the dir),
// else the fresh dir.
func (c Config) JobDir(j *store.Job) string {
	if j.PiSession != "" {
		return filepath.Dir(j.PiSession)
	}
	return c.FreshJobDir(j)
}

var prPathRe = regexp.MustCompile(`^/[^/]+/[^/]+/pull/\d+/?$`)

// LinkKind derives a link's kind from its URL (docs/DESIGN.md "Job model").
func LinkKind(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return "url"
	}
	host := strings.ToLower(u.Hostname())
	switch {
	case host == "github.com" && prPathRe.MatchString(u.Path):
		return "github_pr"
	case host == "github.com" || strings.HasSuffix(host, ".github.com"):
		return "github"
	case host == "slack.com" || strings.HasSuffix(host, ".slack.com"):
		return "slack"
	case host == "linear.app" || strings.HasSuffix(host, ".linear.app"):
		return "linear"
	}
	return "url"
}

// ValidLinkURL reports whether raw is an absolute http(s) URL with a host.
func ValidLinkURL(raw string) bool {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return false
	}
	return (u.Scheme == "http" || u.Scheme == "https") && u.Host != ""
}
