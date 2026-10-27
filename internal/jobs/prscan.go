package jobs

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"sort"
	"strconv"

	"driver/internal/pi"
	"driver/internal/store"
)

func scanOffsetKey(jobID int64) string  { return fmt.Sprintf("scan:%d", jobID) }
func scanPendingKey(jobID int64) string { return fmt.Sprintf("scan-pending:%d", jobID) }

// scanStore is the subset of the store the scanner needs.
type scanStore interface {
	GetKV(string) (string, error)
	SetKV(string, string) error
	DeleteKV(string) error
	AddLink(jobID int64, kind, url, label string) (*store.Link, bool, error)
}

// prScanner tails each job's session JSONL for `gh pr create` bash tool calls
// and attaches the PR URLs their tool results print. The byte offset lives in
// kv (scan:<id>); the ids of calls still awaiting a result ride beside it as
// JSON (scan-pending:<id>) so a restart loses nothing.
type prScanner struct {
	st      scanStore
	pending map[int64]map[string]bool // job id → tool call ids awaiting a result
}

func newPRScanner(st scanStore) *prScanner {
	return &prScanner{st: st, pending: map[int64]map[string]bool{}}
}

func (p *prScanner) offset(jobID int64) int64 {
	v, err := p.st.GetKV(scanOffsetKey(jobID))
	if err != nil {
		return 0
	}
	n, _ := strconv.ParseInt(v, 10, 64)
	return n
}

// pendingFor returns the job's pending set, loading it from kv on first use.
func (p *prScanner) pendingFor(jobID int64) map[string]bool {
	if m, ok := p.pending[jobID]; ok {
		return m
	}
	m := map[string]bool{}
	if v, err := p.st.GetKV(scanPendingKey(jobID)); err == nil {
		var ids []string
		if json.Unmarshal([]byte(v), &ids) == nil {
			for _, id := range ids {
				m[id] = true
			}
		}
	}
	p.pending[jobID] = m
	return m
}

func (p *prScanner) savePending(jobID int64, m map[string]bool) error {
	if len(m) == 0 {
		return p.st.DeleteKV(scanPendingKey(jobID))
	}
	ids := make([]string, 0, len(m))
	for id := range m {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	data, _ := json.Marshal(ids)
	return p.st.SetKV(scanPendingKey(jobID), string(data))
}

// scan reads what pi appended to j's session since the saved offset.
func (p *prScanner) scan(j *store.Job) error {
	offset := p.offset(j.ID)
	lines, next, err := pi.ReadNewLines(j.PiSession, offset)
	if err != nil {
		return fmt.Errorf("%s: scan: %w", j.Slug, err)
	}
	if next == offset && len(lines) == 0 {
		return nil
	}
	pending := p.pendingFor(j.ID)
	if next < offset {
		// the file was replaced or truncated: start over
		for k := range pending {
			delete(pending, k)
		}
	}
	for _, line := range lines {
		e, ok := pi.ParseEntry(line)
		if !ok || e.Kind != "message" {
			continue
		}
		switch e.Role {
		case "assistant":
			for _, tc := range e.ToolCalls {
				if tc.Name == "bash" && pi.IsPRCreateCommand(tc.Command) {
					pending[tc.ID] = true
				}
			}
		case "toolResult":
			if e.Result == nil || !pending[e.Result.ToolCallID] {
				continue
			}
			delete(pending, e.Result.ToolCallID)
			for _, u := range pi.DetectPRURLs(e.Result.Text) {
				if _, created, err := p.st.AddLink(j.ID, "github_pr", u, ""); err != nil {
					if !errors.Is(err, store.ErrDuplicate) {
						return fmt.Errorf("%s: attach %s: %w", j.Slug, u, err)
					}
				} else if created {
					log.Printf("%s: attached PR %s", j.Slug, u)
				}
			}
		}
	}
	if err := p.savePending(j.ID, pending); err != nil {
		return err
	}
	return p.st.SetKV(scanOffsetKey(j.ID), strconv.FormatInt(next, 10))
}
