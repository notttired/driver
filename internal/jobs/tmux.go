package jobs

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// Liveness is one snapshot of tmux: which sessions exist and which of them
// carry a dead pane (remain-on-exit kept the window of an exited pi).
type Liveness struct {
	Sessions map[string]bool
	Dead     map[string]bool
}

// Exists reports whether the session is present (live or with a dead pane).
func (l Liveness) Exists(sess string) bool { return l.Sessions[sess] }

// Live reports whether the session exists and its pane is not dead.
func (l Liveness) Live(sess string) bool { return l.Sessions[sess] && !l.Dead[sess] }

// Tmux runs tmux commands and caches a liveness snapshot for a second, so a
// board request and the tick share one pair of tmux calls.
type Tmux struct {
	bin string

	mu   sync.Mutex
	snap Liveness
	at   time.Time
}

// snapshotTTL is how long a liveness snapshot is reused.
const snapshotTTL = time.Second

// NewTmux resolves the tmux binary; bin is "" when it is not on PATH (every
// command then fails with a clear error, and Submit refuses).
func NewTmux() *Tmux {
	bin, _ := exec.LookPath("tmux")
	return &Tmux{bin: bin}
}

// Available reports whether tmux was found on PATH.
func (t *Tmux) Available() bool { return t.bin != "" }

func (t *Tmux) run(args ...string) (string, error) {
	if t.bin == "" {
		return "", fmt.Errorf("tmux not found on PATH")
	}
	cmd := exec.Command(t.bin, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return stdout.String(), fmt.Errorf("tmux %s: %s", args[0], msg)
	}
	return stdout.String(), nil
}

// Snapshot returns the cached liveness, refreshing it when older than a
// second: one `list-sessions` plus one `list-panes -a`. A tmux server that
// is not running reads as no sessions.
func (t *Tmux) Snapshot() Liveness {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.snap.Sessions != nil && time.Since(t.at) < snapshotTTL {
		return t.snap
	}
	snap := Liveness{Sessions: map[string]bool{}, Dead: map[string]bool{}}
	if out, err := t.run("list-sessions", "-F", "#{session_name}"); err == nil {
		for _, name := range strings.Split(out, "\n") {
			if name = strings.TrimSpace(name); name != "" {
				snap.Sessions[name] = true
			}
		}
	}
	if len(snap.Sessions) > 0 {
		if out, err := t.run("list-panes", "-a", "-F", "#{session_name} #{pane_dead}"); err == nil {
			for _, line := range strings.Split(out, "\n") {
				f := strings.Fields(line)
				if len(f) == 2 && f[1] == "1" {
					snap.Dead[f[0]] = true
				}
			}
		}
	}
	t.snap, t.at = snap, time.Now()
	return snap
}

// Invalidate drops the cached snapshot so the next Snapshot is fresh — called
// after anything that creates or kills a session.
func (t *Tmux) Invalidate() {
	t.mu.Lock()
	t.snap = Liveness{}
	t.mu.Unlock()
}

// HasSession asks tmux directly whether sess exists.
func (t *Tmux) HasSession(sess string) bool {
	_, err := t.run("has-session", "-t", "="+sess)
	return err == nil
}

// PaneDead asks tmux directly whether sess's pane 0 is dead.
func (t *Tmux) PaneDead(sess string) bool {
	out, err := t.run("list-panes", "-t", "="+sess, "-F", "#{pane_dead}")
	if err != nil {
		return true
	}
	return strings.Contains(out, "1")
}

// NewSession starts a detached 220x50 session running cmd (a sh string) in
// dir, with the window named window, the @driver-job option set to slug and
// remain-on-exit on — all in one tmux invocation, so a pi that exits
// immediately still leaves its dead pane behind for the terminal to show.
// The session's tmux status bar is off: the embedded terminal is the whole
// job and pi draws its own footer, so the bar would only cost a row.
// The window option targets the session, not ":0": a base-index of 1 in the
// operator's tmux.conf would make ":0" "no such window".
func (t *Tmux) NewSession(sess, dir, window, slug, cmd string) error {
	_, err := t.run("new-session", "-d", "-x", "220", "-y", "50", "-s", sess, "-c", dir, "-n", window, cmd,
		";", "set-option", "-t", sess, "@driver-job", slug,
		";", "set-window-option", "-t", sess, "remain-on-exit", "on",
		";", "set-option", "-t", sess, "status", "off")
	t.Invalidate()
	return err
}

// KillSession kills sess; a session that is already gone is not an error.
func (t *Tmux) KillSession(sess string) error {
	_, err := t.run("kill-session", "-t", "="+sess)
	t.Invalidate()
	if err != nil && !t.HasSession(sess) {
		return nil
	}
	return err
}

// SendEscape sends one Escape keystroke to sess's active pane. The target is
// "=sess:" — the exact-match "=" is only honoured on the session part of a
// window/pane target, so a bare "=sess" would be "can't find pane".
func (t *Tmux) SendEscape(sess string) error {
	_, err := t.run("send-keys", "-t", "="+sess+":", "Escape")
	return err
}

// RenameSession renames a session and keeps its @driver-job marker current.
func (t *Tmux) RenameSession(from, to, slug string) error {
	if _, err := t.run("rename-session", "-t", "="+from, to); err != nil {
		return err
	}
	_, _ = t.run("set-option", "-t", "="+to, "@driver-job", slug)
	t.Invalidate()
	return nil
}

// Bin is the resolved tmux binary path ("" when unavailable).
func (t *Tmux) Bin() string { return t.bin }
