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

// BoardColumn is where the board shows j: the lane it is parked in, else
// the column its display state names.
func BoardColumn(j *store.Job) string {
	if parked := store.ParkedColumn(j); parked != "" {
		return parked
	}
	return DisplayState(j)
}

// WatchPrompt is what WATCH says to pi.
const WatchPrompt = `The work on this job is done. Get it merged, then watch over it until it is live and completely stable. Do not stop before it is merged.

1. Merging: keep at it until the pull request is merged, and clear whatever stands in the way yourself:
   - a failing check or a bug: find the cause, fix it and push. Re-run a check only when the failure is not from this change.
   - a review comment: make the change and reply with what you changed, or reply with why it should stay as it is.
   - a conflict, or a branch that has fallen behind: bring it up to date and resolve it.
   - a merge that fails or is dropped from the queue: fix the reason and merge again.
   When it is approved and the checks pass, merge it, through the merge queue where there is one. While there is nothing to do but wait (checks running, a reviewer yet to look), wait and look again every few minutes.
2. Deployment: follow the deploy until the change is in production.
3. Stable: for 30 minutes after it is live, check every few minutes for errors, failing requests, alerts or rollbacks related to this change.

Decide the small things yourself. Ask me only about a critical design decision: one that changes what the work does or how it is built beyond what we agreed. The one other reason to stop before the merge is something only I can clear (an expired login, a missing permission): say exactly what you need. Do not get it merged by going around the rules: no skipped or disabled checks, no merge without the approvals the repository requires, no force-push over someone else's commits.

After the merge, do not change code unless I ask: if anything looks wrong, tell me what you see. When the 30 minutes are up and all is well, say so in one line.`

const ReviewPrompt = `Get an adversarial review of your work on this job from a second agent, then tell me what it found.

1. What to review: the pull request for this work if there is one, else the diff on your branch or worktree against its base.
2. Spawn the ae-deep-reviewer subagent on it; this message is my go-ahead to delegate. Give it the directory to work in, the pull request or the base to diff against, and what the change is meant to do. Ask it to look for what is wrong, not to confirm that the work is right.
3. Report its findings grouped as it gave them, with your own view on each: whether you agree, and why not when you don't.

Do not change any code until I say which findings to fix. If ae-deep-reviewer cannot be spawned, say so and stop; do not review the work yourself in its place.`

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

// MaxLaneNameLen bounds a lane's name (in runes): a column head.
const MaxLaneNameLen = 40

// CleanLaneName normalises a lane's name as CleanName does; empty or longer
// than MaxLaneNameLen is ErrBadInput.
func CleanLaneName(s string) (string, error) {
	s = strings.Join(strings.Fields(s), " ")
	if s == "" {
		return "", fmt.Errorf("a column needs a name: %w", ErrBadInput)
	}
	if utf8.RuneCountInString(s) > MaxLaneNameLen {
		return "", fmt.Errorf("column name is too long (max %d characters): %w", MaxLaneNameLen, ErrBadInput)
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

// OwnDir is where the job's own pi keeps its session.jsonl / status.json:
// the directory of its recorded session file (a rename does not move the
// dir; a thread promoted to the job's own pi keeps its t<id> dir), else the
// fresh dir.
func (c Config) OwnDir(j *store.Job) string {
	if j.PiSession != "" {
		return filepath.Dir(j.PiSession)
	}
	return c.FreshJobDir(j)
}

var threadDirRe = regexp.MustCompile(`^t\d+$`)

// JobDir is the job's root dir, which the threads' t<id> dirs sit under:
// the own pi's dir, or its parent when the own pi lives in a thread's dir.
func (c Config) JobDir(j *store.Job) string {
	own := c.OwnDir(j)
	if threadDirRe.MatchString(filepath.Base(own)) {
		return filepath.Dir(own)
	}
	return own
}

// OwnName is what the tab of the job's own pi is called: the name it had
// as a thread, when it was one, else the job's.
func OwnName(j *store.Job) string {
	if j.ThreadName != "" {
		return j.ThreadName
	}
	return j.DisplayName()
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

var prPageRe = regexp.MustCompile(`(?i)^(https?://(?:github\.com|delta\.shopify\.io)/[^/?#]+/[^/?#]+/pull/\d+)(?:[/?#]|$)`)

func LinkURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if m := prPageRe.FindStringSubmatch(raw); m != nil {
		return m[1]
	}
	page, fragment, _ := strings.Cut(raw, "#")
	if strings.HasPrefix(fragment, "/") || strings.HasPrefix(fragment, "!") {
		return raw
	}
	path, query, hasQuery := strings.Cut(page, "?")
	path = strings.TrimRight(path, "/")
	if hasQuery {
		return path + "?" + query
	}
	return path
}

// ValidLinkURL reports whether raw is an absolute http(s) URL with a host.
func ValidLinkURL(raw string) bool {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return false
	}
	return (u.Scheme == "http" || u.Scheme == "https") && u.Host != ""
}
