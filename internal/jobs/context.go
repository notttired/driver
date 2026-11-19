package jobs

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"driver/internal/store"
	"driver/internal/util"
)

// MaxContextChars bounds the text a browser context carries into a pane.
const MaxContextChars = 20000

// MaxPasteChars bounds what /api/job/paste puts into a pane.
const MaxPasteChars = 64000

// BrowserContext is the front tab of the operator's browser as a paste for
// pi: its URL and title, and the text selected on it — or, with nothing
// selected, the page's text (empty when the browser does not let Apple
// events run JavaScript).
type BrowserContext struct {
	URL      string `json:"url"`
	Title    string `json:"title"`
	Text     string `json:"text"`
	Selected bool   `json:"selected"`
}

// Prompt is the context as it reads in pi's editor: where it came from,
// then the text, cut at MaxContextChars.
func (c BrowserContext) Prompt() string {
	head := "Context from my browser"
	if c.Selected {
		head += " (selected text)"
	}
	head += " — " + strings.TrimSpace(c.Title) + "\n" + c.URL
	text := strings.TrimSpace(c.Text)
	if text == "" {
		return head
	}
	if len(text) > MaxContextChars {
		text = text[:MaxContextChars] + "\n\n(cut at " + itoa(MaxContextChars) + " characters)"
	}
	return head + "\n\n" + text
}

func itoa(n int) string { return strconv.Itoa(n) }

// contextScript reads the front window's active tab: URL, title, whether
// text is selected, and the selected text or the page's. The JavaScript
// step needs the browser's "Allow JavaScript from Apple Events"; without it
// the tab is reported with empty text rather than failing.
const contextScript = `tell application %s
	set us to character id 31
	set rs to character id 30
	set t to active tab of front window
	set u to URL of t
	if u is missing value then set u to ""
	set n to title of t
	if n is missing value then set n to ""
	set sel to ""
	set body to ""
	try
		set sel to execute t javascript "window.getSelection().toString()"
		if sel is missing value then set sel to ""
		if sel is "" then
			set body to execute t javascript "document.body.innerText"
			if body is missing value then set body to ""
		end if
	end try
	if sel is not "" then
		return u & us & n & us & "true" & us & sel & rs
	end if
	return u & us & n & us & "false" & us & body & rs
end tell`

// FrontTabContext asks the browser for its front tab as a BrowserContext.
func FrontTabContext(browser string, timeout time.Duration) (BrowserContext, error) {
	if !BrowserRunning(browser) {
		return BrowserContext{}, Refuse("%s is not running", browser)
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	var stdout, stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, "osascript", "-e", fmt.Sprintf(contextScript, appleScriptString(browser)))
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		return BrowserContext{}, browserError(browser, stderr.String(), ctx, err, timeout)
	}
	out, ok := parseBrowserContext(stdout.String())
	if !ok {
		return BrowserContext{}, Refuse("%s's front tab is not a web page", browser)
	}
	return out, nil
}

func browserError(browser, stderr string, ctx context.Context, err error, timeout time.Duration) error {
	msg := strings.TrimSpace(stderr)
	switch {
	case strings.Contains(msg, "-1743") || strings.Contains(msg, "Not authorized"):
		return Refuse("macOS has not let driver read %s — allow it under System Settings › Privacy & Security › Automation (the ⚙ settings can open it)", browser)
	case ctx.Err() != nil:
		return Refuse("%s did not answer within %s", browser, timeout)
	case strings.Contains(msg, "-1728") || strings.Contains(msg, "Can’t get"):
		return Refuse("%s has no front window", browser)
	case msg == "":
		return fmt.Errorf("osascript: %w", err)
	}
	return Refuse("osascript: %s", strings.SplitN(msg, "\n", 2)[0])
}

func parseBrowserContext(out string) (BrowserContext, bool) {
	rec, _, _ := strings.Cut(out, "\x1e")
	f := strings.SplitN(rec, "\x1f", 4)
	if len(f) < 4 {
		return BrowserContext{}, false
	}
	url := strings.TrimSpace(f[0])
	if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
		return BrowserContext{}, false
	}
	return BrowserContext{URL: url, Title: strings.TrimSpace(f[1]), Selected: strings.TrimSpace(f[2]) == "true", Text: f[3]}, true
}

// Paste puts text into a pane's pi editor — the job's own (t nil) or a
// thread's — as one bracketed paste, nothing sent: the operator adds their
// question and presses Enter.
func (s *Service) Paste(j *store.Job, t *store.Thread, text string) error {
	if strings.TrimSpace(text) == "" {
		return Refuse("nothing to paste")
	}
	if len(text) > MaxPasteChars {
		return fmt.Errorf("paste is too long (max %d characters): %w", MaxPasteChars, ErrBadInput)
	}
	sess := SessionName(j)
	who := "job " + j.Slug
	if t != nil {
		sess = ThreadSessionName(j, t)
		who = "thread " + strconv.Quote(t.Name)
	}
	if !s.Tmux.Available() || !s.Tmux.HasSession(sess) || s.Tmux.PaneDead(sess) {
		return Refuse("%s has no live pane", who)
	}
	if err := s.Tmux.PasteOnly(sess, text); err != nil {
		return Refuse("%v", err)
	}
	return nil
}

// FolderProbe is one protected folder as the daemon finds it.
type FolderProbe struct {
	Name  string `json:"name"`
	State string `json:"state"` // ok | denied | missing
}

// protectedFolders are the home folders macOS guards: a read of one from a
// process without a grant puts up a prompt in the responsible process's
// name — driver's, for every pi a job runs.
var protectedFolders = []string{"Desktop", "Documents", "Downloads"}

// ProbeFolders reads each protected folder of home once, from the daemon
// itself, so the grant macOS asks for lands on driver — the one process
// every pane answers to — rather than on a pane mid-task. A denied read
// is "denied": the prompt was refused once, or is up right now.
func ProbeFolders(home string) []FolderProbe {
	out := make([]FolderProbe, 0, len(protectedFolders))
	for _, name := range protectedFolders {
		state := "ok"
		if _, err := os.ReadDir(filepath.Join(home, name)); err != nil {
			if errors.Is(err, os.ErrNotExist) {
				state = "missing"
			} else {
				state = "denied"
			}
		}
		out = append(out, FolderProbe{Name: name, State: state})
	}
	return out
}

// OpenSystemPane opens System Settings on a Privacy & Security pane:
// "full-disk-access", "automation" or "files"; "binary" shows the installed
// driver binary in the Finder, for dragging into a list.
func OpenSystemPane(pane string) error {
	switch pane {
	case "full-disk-access":
		return exec.Command("open", "x-apple.systempreferences:com.apple.preference.security?Privacy_AllFiles").Run()
	case "files":
		return exec.Command("open", "x-apple.systempreferences:com.apple.preference.security?Privacy_FilesAndFolders").Run()
	case "automation":
		return exec.Command("open", "x-apple.systempreferences:com.apple.preference.security?Privacy_Automation").Run()
	case "binary":
		bin, err := os.Executable()
		if err != nil {
			return err
		}
		if real, err := filepath.EvalSymlinks(bin); err == nil {
			bin = real
		}
		return exec.Command("open", "-R", bin).Run()
	}
	return fmt.Errorf("unknown pane %q: %w", util.OneLine(pane, 30), ErrBadInput)
}
