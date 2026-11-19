package jobs

import (
	"path/filepath"

	"driver/internal/pi"
	"driver/internal/store"
)

// startingGrace is how long a freshly launched job may go without a
// status.json before it reads as blocked ("silent").
const startingGrace = 120

// PiInfo is the board's `pi` object: pi's state as the daemon last read it,
// and — when the derivation calls the job blocked — the reason, in the same
// vocabulary (idle | dialog | aborted | exited | silent). "ready" is pi open
// at its prompt with nothing sent yet (a draft that has its pane).
type PiInfo struct {
	State  string `json:"state"` // starting|ready|working|idle|dialog|aborted|exited|silent
	Tool   string `json:"tool,omitempty"`
	Reason string `json:"reason,omitempty"`
	Error  string `json:"error,omitempty"`
	TS     int64  `json:"ts"`
}

// Observation is one derivation: the state the job SHOULD be in now and
// pi's reading behind it (nil for not-started drafts and done jobs).
type Observation struct {
	State string
	Pi    *PiInfo
}

// Observe derives a job's state (docs/DESIGN.md "State derivation") from its
// stored state, tmux liveness, pi's status.json (nil = not written yet) and
// the clock. It is pure, so the tick and the board share it.
func Observe(j *store.Job, live bool, st *pi.Status, now int64) Observation {
	switch j.State {
	case "":
		return Observation{State: "draft"}
	case "done":
		return Observation{State: "done"}
	}
	return ObserveThread(j.State, j.StartedAt, live, st, now)
}

// ObserveThread derives one pi's state — the job's own or a thread's — from
// the job's stored state (ready, working or blocked: the column a pi that
// has not reported yet keeps), the pi's tmux liveness, its status.json and
// the clock.
func ObserveThread(stored string, startedAt int64, live bool, st *pi.Status, now int64) Observation {
	if !live {
		ts := startedAt
		if st != nil && st.TS > ts {
			ts = st.TS
		}
		return Observation{State: "blocked", Pi: &PiInfo{State: "exited", Reason: "exited", TS: ts}}
	}
	if st == nil {
		if now-startedAt < startingGrace {
			// a just-opened pi keeps its column (ready stays in DRAFT, a
			// blocked job's fork or thread in BLOCKED) while it boots and the
			// extension writes its first status
			state := "working"
			if stored == "ready" || stored == "blocked" {
				state = stored
			}
			return Observation{State: state, Pi: &PiInfo{State: "starting", TS: startedAt}}
		}
		return Observation{State: "blocked", Pi: &PiInfo{State: "silent", Reason: "silent", TS: startedAt}}
	}
	info := &PiInfo{State: st.State, Tool: st.Tool, TS: st.TS}
	switch st.State {
	case "ready":
		return Observation{State: "ready", Pi: info}
	case "working":
		return Observation{State: "working", Pi: info}
	}
	// idle, dialog, aborted, exited (pi shutting down inside a live pane) and
	// anything unknown all need the operator
	info.Reason = st.State
	return Observation{State: "blocked", Pi: info}
}

// Combine folds the observations of a job's threads into the job's: working
// while any thread works, else blocked when any thread waits, else ready.
// The first thread in that state speaks for the job (its pi on the card).
func Combine(obs []Observation) Observation {
	for _, want := range []string{"working", "blocked", "ready"} {
		for _, o := range obs {
			if o.State == want {
				return o
			}
		}
	}
	return obs[0]
}

// holdBlocked confirms a working job's blocked reading over two consecutive
// ticks before the daemon acts on it (kraken's one-tick hold: it hides any
// one-tick misread, and the idle a resumed session reports at session_start
// before its operator has typed). It returns the state to act on and the tick to
// remember as the hold's start (0 = no hold in progress). The hold demands
// ADJACENCY: a stale entry from an earlier, interrupted hold never confirms
// a fresh reading. Only the working → blocked edge is held; every other
// transition passes through.
func holdBlocked(persisted, derived string, firstTick, tickN int) (string, int) {
	if derived != "blocked" || persisted != "working" {
		return derived, 0
	}
	if firstTick == 0 || tickN-firstTick != 1 {
		return "working", tickN
	}
	return "blocked", firstTick
}

// ReadStatus reads a job's status.json; nil when absent or unreadable.
func (c Config) ReadStatus(j *store.Job) *pi.Status { return readStatus(c.OwnDir(j)) }

// ReadThreadStatus reads a thread's status.json.
func (c Config) ReadThreadStatus(j *store.Job, t *store.Thread) *pi.Status {
	return readStatus(c.ThreadDir(j, t))
}

func readStatus(dir string) *pi.Status {
	st, found, err := pi.ReadStatus(filepath.Join(dir, "status.json"))
	if err != nil || !found {
		return nil
	}
	return &st
}

type RetryPlan string

const (
	RetryNone     RetryPlan = ""
	RetryContinue RetryPlan = "continue"
	RetryRestart  RetryPlan = "restart"
	RetryResume   RetryPlan = "resume"
)

func PlanRetry(j *store.Job, live bool, st *pi.Status, conv pi.Conversation) RetryPlan {
	if j.Archived() || j.State != "blocked" {
		return RetryNone
	}
	return planRetry(live, st, conv)
}

// PlanThreadRetry reports whether RETRY has something to do for a thread
// known to be waiting.
func PlanThreadRetry(live bool, st *pi.Status, conv pi.Conversation) bool {
	return planRetry(live, st, conv) != RetryNone
}

// planRetry is PlanRetry for a pi known to be waiting: what RETRY would do
// for it.
func planRetry(live bool, st *pi.Status, conv pi.Conversation) RetryPlan {
	_, failed := conv.Failed()
	if !live {
		if failed || conv.Pending {
			return RetryResume
		}
		return RetryRestart
	}
	if st != nil && st.State == "idle" && failed {
		return RetryContinue
	}
	return RetryNone
}
