package pi

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
)

// Status is the status.json the driver extension writes next to a job's
// session: pi's own view of what it is doing.
type Status struct {
	State   string `json:"state"` // ready | idle | working | dialog | aborted | exited
	Tool    string `json:"tool,omitempty"`
	PID     int    `json:"pid"`
	Session string `json:"session"`
	TS      int64  `json:"ts"` // unix seconds (normalised: values > 1e12 are read as ms)
}

// ReadStatus reads path. found is false when the file does not exist yet; a
// malformed file is an error so the caller can treat it as "no status".
func ReadStatus(path string) (st Status, found bool, err error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Status{}, false, nil
		}
		return Status{}, false, fmt.Errorf("read %s: %w", path, err)
	}
	var raw struct {
		State   string      `json:"state"`
		Tool    string      `json:"tool"`
		PID     int         `json:"pid"`
		Session string      `json:"session"`
		TS      json.Number `json:"ts"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return Status{}, false, fmt.Errorf("parse %s: %w", path, err)
	}
	st = Status{State: raw.State, Tool: raw.Tool, PID: raw.PID, Session: raw.Session}
	if f, err := raw.TS.Float64(); err == nil {
		st.TS = NormalizeTS(int64(f))
	}
	return st, true, nil
}

// NormalizeTS maps a timestamp that may be unix seconds or unix milliseconds
// onto seconds: anything above 1e12 can only be milliseconds (1e12 seconds is
// the year 33658).
func NormalizeTS(ts int64) int64 {
	if ts > 1e12 {
		return ts / 1000
	}
	return ts
}
