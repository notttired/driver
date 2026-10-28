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
	if !live {
		ts := j.StartedAt
		if st != nil && st.TS > ts {
			ts = st.TS
		}
		return Observation{State: "blocked", Pi: &PiInfo{State: "exited", Reason: "exited", TS: ts}}
	}
	if st == nil {
		if now-j.StartedAt < startingGrace {
			// a just-opened job keeps its column (ready stays in DRAFT, a
			// blocked job's fork in BLOCKED) while pi boots and the extension
			// writes its first status
			state := "working"
			if j.State == "ready" || j.State == "blocked" {
				state = j.State
			}
			return Observation{State: state, Pi: &PiInfo{State: "starting", TS: j.StartedAt}}
		}
		return Observation{State: "blocked", Pi: &PiInfo{State: "silent", Reason: "silent", TS: j.StartedAt}}
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
func (c Config) ReadStatus(j *store.Job) *pi.Status {
	st, found, err := pi.ReadStatus(filepath.Join(c.JobDir(j), "status.json"))
	if err != nil || !found {
		return nil
	}
	return &st
}
