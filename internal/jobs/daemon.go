package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strings"
	"sync/atomic"
	"time"

	"driver/internal/util"
)

// TickInterval is the daemon's cadence.
const TickInterval = 2 * time.Second

// Status is the heartbeat (<home>/daemon.json and the board's `daemon`).
type Status struct {
	PID       int    `json:"pid"`
	StartedAt int64  `json:"started_at"`
	LastTick  int64  `json:"last_tick"`
	Live      int    `json:"live"` // jobs with a live tmux session
	LastError string `json:"last_error,omitempty"`
}

// Daemon is the tick loop: state derivation (for every opened job: ready,
// working, blocked) with the one-tick blocked hold, notifications, the PR
// scan and the PR status refresh.
type Daemon struct {
	svc    *Service
	status atomic.Pointer[Status]

	tickN        int
	blockedSince map[int64]int    // job id → tick the current hold began
	lastReason   map[int64]string // job id → last blocked reason seen
	scanner      *prScanner
	prBackoff    map[string]time.Time // PR url → last failed gh call
	ghMissing    bool
	prBusy       atomic.Bool // a refreshPRs run is in flight
}

// NewDaemon builds a Daemon over svc.
func NewDaemon(svc *Service) *Daemon {
	d := &Daemon{
		svc:          svc,
		blockedSince: map[int64]int{},
		lastReason:   map[int64]string{},
		scanner:      newPRScanner(svc.Store),
		prBackoff:    map[string]time.Time{},
	}
	st := Status{PID: os.Getpid(), StartedAt: time.Now().Unix()}
	d.status.Store(&st)
	return d
}

// Status returns the latest heartbeat.
func (d *Daemon) Status() Status { return *d.status.Load() }

// Run ticks every TickInterval until ctx is cancelled. The heartbeat file is
// written before the first tick so a board loaded immediately sees a pid.
func (d *Daemon) Run(ctx context.Context) {
	d.writeHeartbeat()
	t := time.NewTicker(TickInterval)
	defer t.Stop()
	for {
		d.runTick()
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

func (d *Daemon) runTick() {
	live, err := d.tick()
	st := *d.status.Load()
	st.LastTick, st.Live, st.LastError = time.Now().Unix(), live, ""
	if err != nil {
		st.LastError = err.Error()
		log.Printf("tick error: %v", err)
	}
	d.status.Store(&st)
	d.writeHeartbeat()
}

func (d *Daemon) writeHeartbeat() {
	data, err := json.Marshal(d.Status())
	if err != nil {
		return
	}
	if err := os.MkdirAll(d.svc.Cfg.Home, 0o755); err == nil {
		_ = util.AtomicWrite(d.svc.Cfg.DaemonFile(), data, 0o644)
	}
}

// tick is one pass; each concern is independent and the first error is
// reported through the heartbeat without stopping the others.
func (d *Daemon) tick() (live int, firstErr error) {
	d.tickN++
	note := func(err error) {
		if err != nil && firstErr == nil {
			firstErr = err
		}
	}
	jobs, err := d.svc.Store.ListJobs()
	if err != nil {
		return 0, err
	}
	snap := d.svc.Tmux.Snapshot()
	now := time.Now().Unix()
	seen := map[int64]bool{}
	for _, j := range jobs {
		if j.State != "ready" && j.State != "working" && j.State != "blocked" {
			continue
		}
		seen[j.ID] = true
		sess := SessionName(j)
		isLive := snap.Live(sess)
		if isLive {
			live++
		}
		obs := Observe(j, isLive, d.svc.Cfg.ReadStatus(j), now)
		state, hold := holdBlocked(j.State, obs.State, d.blockedSince[j.ID], d.tickN)
		if hold == 0 {
			delete(d.blockedSince, j.ID)
		} else {
			d.blockedSince[j.ID] = hold
		}
		reason := ""
		if obs.Pi != nil {
			reason = obs.Pi.Reason
		}
		if state != j.State {
			if _, err := d.svc.Store.UpdateJobFields(j.ID, j.Rev, map[string]any{"state": state}); err != nil {
				note(fmt.Errorf("%s: write state %s: %w", j.Slug, state, err))
				continue
			}
			log.Printf("%s: %s → %s%s", j.Slug, j.State, state, reasonSuffix(reason))
			if state == "blocked" {
				d.notify(fmt.Sprintf("driver ⚑ %s — %s", j.Slug, reason))
			}
		} else if j.State == "blocked" && reason == "exited" && d.lastReason[j.ID] != "" && d.lastReason[j.ID] != "exited" {
			// already blocked (idle) and now pi is gone: the exit itself is
			// worth a notification (design: "or its pi exits")
			d.notify(fmt.Sprintf("driver ⚑ %s — exited", j.Slug))
		}
		if state == "blocked" {
			d.lastReason[j.ID] = reason
		} else {
			delete(d.lastReason, j.ID)
		}
	}
	for id := range d.blockedSince {
		if !seen[id] {
			delete(d.blockedSince, id)
		}
	}
	for id := range d.lastReason {
		if !seen[id] {
			delete(d.lastReason, id)
		}
	}
	for _, j := range jobs {
		if j.PiSession != "" {
			note(d.scanner.scan(j))
		}
	}
	// gh round trips (up to 3 × 10s) must not stall the heartbeat: refresh in
	// the background, never more than one run at a time
	if d.prBusy.CompareAndSwap(false, true) {
		go func() {
			defer d.prBusy.Store(false)
			if err := d.refreshPRs(now); err != nil {
				log.Printf("pr refresh: %v", err)
			}
		}()
	}
	return live, firstErr
}

func reasonSuffix(reason string) string {
	if reason == "" {
		return ""
	}
	return " (" + reason + ")"
}

// notify posts a macOS notification through osascript unless DRIVER_NOTIFY=off.
// Fire-and-forget: a failure is logged, never fatal — the board is the
// durable signal.
func (d *Daemon) notify(msg string) {
	if !d.svc.Cfg.Notify {
		return
	}
	script := fmt.Sprintf("display notification %s with title %s", appleScriptString(util.OneLine(msg, 120)), appleScriptString("driver"))
	go func() {
		if out, err := exec.Command("osascript", "-e", script).CombinedOutput(); err != nil {
			log.Printf("osascript notify failed: %v: %s", err, strings.TrimSpace(string(out)))
		}
	}()
}

// appleScriptString quotes s as an AppleScript string literal.
func appleScriptString(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	return `"` + s + `"`
}

// prFreshness is how old a PR status may be before it is re-polled.
const prFreshness = 60

// prPerTick caps gh calls per tick.
const prPerTick = 3

// refreshPRs re-polls up to three stale github_pr links with
// `gh pr view <url> --json state,isDraft`. A failing gh leaves the status
// untouched and backs that URL off for a minute.
func (d *Daemon) refreshPRs(now int64) error {
	if d.ghMissing {
		return nil
	}
	links, err := d.svc.Store.StalePRLinks(now-prFreshness, prPerTick*4)
	if err != nil {
		return err
	}
	if len(links) == 0 {
		return nil
	}
	gh, err := exec.LookPath("gh")
	if err != nil {
		d.ghMissing = true
		log.Printf("gh not found on PATH: PR statuses will not refresh")
		return nil
	}
	polled := 0
	for _, l := range links {
		if polled >= prPerTick {
			break
		}
		if t, ok := d.prBackoff[l.URL]; ok && time.Since(t) < prFreshness*time.Second {
			continue
		}
		polled++
		status, err := ghPRStatus(gh, l.URL)
		if err != nil {
			d.prBackoff[l.URL] = time.Now()
			log.Printf("pr %s: %v", l.URL, err)
			continue
		}
		delete(d.prBackoff, l.URL)
		if err := d.svc.Store.SetLinkStatus(l.ID, status, now); err != nil {
			return err
		}
	}
	return nil
}

// ghPRStatus maps gh's {state, isDraft} onto open|draft|merged|closed.
func ghPRStatus(gh, url string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, gh, "pr", "view", url, "--json", "state,isDraft").Output()
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) && len(ee.Stderr) > 0 {
			return "", fmt.Errorf("gh pr view: %s", util.OneLine(string(ee.Stderr), 200))
		}
		return "", fmt.Errorf("gh pr view: %w", err)
	}
	var v struct {
		State   string `json:"state"`
		IsDraft bool   `json:"isDraft"`
	}
	if err := json.Unmarshal(out, &v); err != nil {
		return "", fmt.Errorf("gh pr view: parse: %w", err)
	}
	return mapPRState(v.State, v.IsDraft), nil
}

func mapPRState(state string, draft bool) string {
	switch strings.ToUpper(state) {
	case "MERGED":
		return "merged"
	case "CLOSED":
		return "closed"
	case "OPEN":
		if draft {
			return "draft"
		}
		return "open"
	}
	return ""
}
