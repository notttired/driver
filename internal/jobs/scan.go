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

// A scan key names one session file the scanner follows: a job's own
// ("<job id>") or a thread's ("<job id>/t<thread id>").
func jobKey(j *store.Job) string                     { return strconv.FormatInt(j.ID, 10) }
func threadKey(j *store.Job, t *store.Thread) string { return fmt.Sprintf("%d/t%d", j.ID, t.ID) }

func scanOffsetKey(key string) string   { return "scan:" + key }
func scanPendingKey(key string) string  { return "scan-pending:" + key }
func scanMentionsKey(key string) string { return "scan-mentions:" + key }

// scanGeneration names what the scanner looks for. Bump it when that grows:
// a scanner meeting a different value in kv forgets every saved offset, so
// each session is read again from its start and nothing already on disk is
// missed (attaching is idempotent). 1: PR results · 2: + every URL in your
// messages and pi's replies.
const scanGeneration = "2"

const scanGenerationKey = "scanner-generation"

// scanStore is the subset of the store the scanner needs.
type scanStore interface {
	GetKV(string) (string, error)
	SetKV(string, string) error
	DeleteKV(string) error
	DeleteKVPrefix(string) error
	AddLink(jobID int64, kind, url string) (*store.Link, bool, error)
	LinkByURL(jobID int64, url string) (*store.Link, error)
	AddMention(linkID int64, entryID, role string) error
	LinkSeenAt(linkID, at int64) error
}

// sessionScanner tails each job's session JSONL and attaches links from the
// conversation: every URL in your messages and in pi's replies (what you
// hand the agent and what it hands back is what the job is about), plus the
// PR URLs printed by the result of a `gh pr create` bash tool call — and
// counts each message that mentions one, by whom. Other tool output is not
// scanned — a curl of a page would litter the card. The byte offset lives
// in kv (scan:<id>); the ids of PR calls still awaiting a result ride beside
// it as JSON (scan-pending:<id>) so a restart loses nothing.
type sessionScanner struct {
	st       scanStore
	pending  map[string]map[string]bool // scan key → tool call ids awaiting a result
	backfill map[string]map[string]bool
	checked  bool // scanGeneration compared with kv once
}

func newSessionScanner(st scanStore) *sessionScanner {
	return &sessionScanner{st: st, pending: map[string]map[string]bool{}, backfill: map[string]map[string]bool{}}
}

func (p *sessionScanner) offset(key string) int64 {
	v, err := p.st.GetKV(scanOffsetKey(key))
	if err != nil {
		return 0
	}
	n, _ := strconv.ParseInt(v, 10, 64)
	return n
}

// pendingFor returns the job's pending set, loading it from kv on first use.
func (p *sessionScanner) pendingFor(key string) map[string]bool {
	if m, ok := p.pending[key]; ok {
		return m
	}
	m := map[string]bool{}
	if v, err := p.st.GetKV(scanPendingKey(key)); err == nil {
		var ids []string
		if json.Unmarshal([]byte(v), &ids) == nil {
			for _, id := range ids {
				m[id] = true
			}
		}
	}
	p.pending[key] = m
	return m
}

func (p *sessionScanner) savePending(key string, m map[string]bool) error {
	if len(m) == 0 {
		return p.st.DeleteKV(scanPendingKey(key))
	}
	ids := make([]string, 0, len(m))
	for id := range m {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	data, _ := json.Marshal(ids)
	return p.st.SetKV(scanPendingKey(key), string(data))
}

// checkGeneration, once per process, forgets every saved offset when the
// scanner on disk was of an older generation (see scanGeneration).
func (p *sessionScanner) checkGeneration() error {
	if p.checked {
		return nil
	}
	if v, err := p.st.GetKV(scanGenerationKey); err == nil && v == scanGeneration {
		p.checked = true
		return nil
	}
	for _, prefix := range []string{"scan:", "scan-pending:"} {
		if err := p.st.DeleteKVPrefix(prefix); err != nil {
			return err
		}
	}
	p.pending = map[string]map[string]bool{}
	if err := p.st.SetKV(scanGenerationKey, scanGeneration); err != nil {
		return err
	}
	log.Printf("scanner: generation %s — rereading every session for links", scanGeneration)
	p.checked = true
	return nil
}

// scan reads what pi appended to j's own session since the saved offset.
func (p *sessionScanner) scan(j *store.Job) error {
	if err := p.checkGeneration(); err != nil {
		return err
	}
	if err := p.countMentions(j); err != nil {
		return err
	}
	return p.scanFile(j, jobKey(j), j.PiSession)
}

// scanThread reads what a thread's pi appended to its session.
func (p *sessionScanner) scanThread(j *store.Job, t *store.Thread) error {
	if err := p.checkGeneration(); err != nil {
		return err
	}
	return p.scanFile(j, threadKey(j, t), t.PiSession)
}

func (p *sessionScanner) scanFile(j *store.Job, key, path string) error {
	offset := p.offset(key)
	lines, next, err := pi.ReadNewLines(path, offset)
	if err != nil {
		return fmt.Errorf("%s: scan: %w", j.Slug, err)
	}
	if next == offset && len(lines) == 0 {
		return nil
	}
	pending := p.pendingFor(key)
	if next < offset {
		// the file was replaced or truncated: start over
		for k := range pending {
			delete(pending, k)
		}
	}
	for _, line := range lines {
		for _, s := range sightingsIn(line, pending) {
			if err := p.attach(j, s); err != nil {
				return err
			}
		}
	}
	if err := p.savePending(key, pending); err != nil {
		return err
	}
	return p.st.SetKV(scanOffsetKey(key), strconv.FormatInt(next, 10))
}

type sighting struct {
	kind, url, entry, role, what string
	at                           int64
}

func sightingsIn(line []byte, pending map[string]bool) []sighting {
	e, ok := pi.ParseEntry(line)
	if !ok || e.Kind != "message" {
		return nil
	}
	var out []sighting
	switch e.Role {
	case "user", "assistant":
		for _, u := range pi.DetectURLs(e.Text) {
			if ValidLinkURL(u) {
				u = LinkURL(u)
				out = append(out, sighting{LinkKind(u), u, e.ID, e.Role, "link", e.Time})
			}
		}
		for _, tc := range e.ToolCalls {
			if tc.Name == "bash" && pi.IsPRCreateCommand(tc.Command) {
				pending[tc.ID] = true
			}
		}
	case "toolResult":
		if e.Result == nil || !pending[e.Result.ToolCallID] {
			return nil
		}
		delete(pending, e.Result.ToolCallID)
		for _, u := range pi.DetectPRURLs(e.Result.Text) {
			out = append(out, sighting{"github_pr", u, e.ID, e.Role, "PR", e.Time})
		}
	}
	return out
}

func (p *sessionScanner) countMentions(j *store.Job) error {
	key := jobKey(j)
	v, err := p.st.GetKV(scanMentionsKey(key))
	if err != nil {
		return nil
	}
	from, _ := strconv.ParseInt(v, 10, 64)
	upTo := p.offset(key)
	next := from
	if from < upTo {
		var lines [][]byte
		if lines, next, err = pi.ReadNewLines(j.PiSession, from); err != nil {
			return fmt.Errorf("%s: count mentions: %w", j.Slug, err)
		}
		if p.backfill[key] == nil {
			p.backfill[key] = map[string]bool{}
		}
		for _, line := range lines {
			for _, s := range sightingsIn(line, p.backfill[key]) {
				l, err := p.st.LinkByURL(j.ID, s.url)
				if errors.Is(err, store.ErrNotFound) {
					continue
				}
				if err != nil {
					return fmt.Errorf("%s: count mentions: %w", j.Slug, err)
				}
				if err := p.st.AddMention(l.ID, s.entry, s.role); err != nil {
					return err
				}
				if err := p.st.LinkSeenAt(l.ID, s.at); err != nil {
					return err
				}
			}
		}
	}
	if next > from && next < upTo {
		return p.st.SetKV(scanMentionsKey(key), strconv.FormatInt(next, 10))
	}
	delete(p.backfill, key)
	return p.st.DeleteKV(scanMentionsKey(key))
}

// forget drops a session's saved offset and pending PR calls: the next scan
// reads it from its start — for a pi that moved to another file
// (daemon.followSession).
func (p *sessionScanner) forget(key string) error {
	delete(p.pending, key)
	if err := p.st.DeleteKV(scanOffsetKey(key)); err != nil {
		return err
	}
	return p.st.DeleteKV(scanPendingKey(key))
}

// attach adds the sighted URL to j's links — a URL already there is left
// alone — and counts the sighting as a mention of it, the link arriving no
// later than the message that carries it. what names the find in the log
// ("link", "PR").
func (p *sessionScanner) attach(j *store.Job, s sighting) error {
	l, created, err := p.st.AddLink(j.ID, s.kind, s.url)
	if err != nil {
		if errors.Is(err, store.ErrDuplicate) {
			return nil
		}
		return fmt.Errorf("%s: attach %s: %w", j.Slug, s.url, err)
	}
	if created {
		log.Printf("%s: attached %s %s", j.Slug, s.what, s.url)
	}
	if err := p.st.AddMention(l.ID, s.entry, s.role); err != nil {
		return err
	}
	return p.st.LinkSeenAt(l.ID, s.at)
}
