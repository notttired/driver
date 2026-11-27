package jobs

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"driver/internal/pi"
	"driver/internal/store"
	"driver/internal/util"
)

// ThreadSessionName is the tmux session a thread runs in: the job's with
// -t<thread id> appended.
func ThreadSessionName(j *store.Job, t *store.Thread) string {
	return fmt.Sprintf("%s-t%d", FreshSessionName(j), t.ID)
}

// ThreadDir is where a thread's session.jsonl / status.json live: the
// directory of its recorded session file, else <job dir>/t<id>.
func (c Config) ThreadDir(j *store.Job, t *store.Thread) string {
	if t.PiSession != "" {
		return filepath.Dir(t.PiSession)
	}
	return filepath.Join(c.JobDir(j), fmt.Sprintf("t%d", t.ID))
}

// MaxThreadNameLen bounds a thread's name (in runes): a tab.
const MaxThreadNameLen = 60

// CleanThreadName normalises a thread's name as CleanName does; empty or
// longer than MaxThreadNameLen is a refusal.
func CleanThreadName(s string) (string, error) {
	s = strings.Join(strings.Fields(s), " ")
	if s == "" {
		return "", Refuse("a thread needs a name")
	}
	if utf8.RuneCountInString(s) > MaxThreadNameLen {
		return "", fmt.Errorf("thread name is too long (max %d characters): %w", MaxThreadNameLen, ErrBadInput)
	}
	return s, nil
}

// LookupThread finds thread id on j; a thread of another job is not found.
func (s *Service) LookupThread(j *store.Job, id int64) (*store.Thread, error) {
	t, err := s.Store.GetThread(id)
	if err != nil {
		return nil, err
	}
	if t.JobID != j.ID {
		return nil, store.ErrNotFound
	}
	return t, nil
}

// OpenThreads lists j's threads whose pi has not been closed.
func (s *Service) OpenThreads(j *store.Job) ([]*store.Thread, error) {
	all, err := s.Store.ListThreads(j.ID)
	if err != nil {
		return nil, err
	}
	out := all[:0]
	for _, t := range all {
		if !t.Closed() {
			out = append(out, t)
		}
	}
	return out, nil
}

// OpenThread adds a thread to an open job: a copy of the conversation of
// thread `from` (0 = the job's own) up to its finished work — a turn still
// under way stays behind (pi.ForkSessionFinished) — with its own pi resumed
// on it in a tmux session of its own. The thread keeps the job's column
// until its pi reports.
func (s *Service) OpenThread(j *store.Job, from int64, name string) (*store.Thread, error) {
	name, err := CleanThreadName(name)
	if err != nil {
		return nil, err
	}
	if j.Archived() {
		return nil, Refuse("job %s is archived", j.ArchivedName)
	}
	if j.State == "" || j.State == "done" {
		return nil, Refuse("job %s is not open (%s) — a thread joins a running job", j.Slug, stage(j))
	}
	if j.Dir == "" || !util.IsDir(j.Dir) {
		return nil, Refuse("dir %q is gone", j.Dir)
	}
	piBin, err := s.resolveTools()
	if err != nil {
		return nil, err
	}
	src := resumeFile(j, s.Cfg.OwnDir(j))
	if from != 0 {
		ft, err := s.LookupThread(j, from)
		if err != nil {
			return nil, err
		}
		src = ft.PiSession
	}
	t := &store.Thread{JobID: j.ID, Name: name}
	if _, err := s.Store.CreateThread(t); err != nil {
		return nil, err
	}
	tdir := s.Cfg.ThreadDir(j, t)
	sess := ""
	undo := func(err error) (*store.Thread, error) {
		if sess != "" {
			_ = s.Tmux.KillSession(sess)
		}
		_ = os.RemoveAll(tdir)
		_ = s.Store.DeleteKV(scanOffsetKey(threadKey(j, t)))
		if derr := s.Store.DeleteThread(t.ID); derr != nil {
			log.Printf("thread %s/%s: %v; and the half-made thread could not be removed: %v", j.Slug, name, err, derr)
		}
		return nil, err
	}
	if err := os.MkdirAll(tdir, 0o755); err != nil {
		return undo(fmt.Errorf("mkdir %s: %w", tdir, err))
	}
	sessionFile := filepath.Join(tdir, "session.jsonl")
	if fi, err := os.Stat(src); err == nil && fi.Mode().IsRegular() {
		if err := pi.ForkSessionFinished(src, sessionFile, time.Now()); err != nil {
			return undo(err)
		}
		// the copied conversation's links are the job's already: the
		// thread's scanner starts at its end
		if fi, err := os.Stat(sessionFile); err == nil {
			_ = s.Store.SetKV(scanOffsetKey(threadKey(j, t)), strconv.FormatInt(fi.Size(), 10))
		}
	}
	sess = ThreadSessionName(j, t)
	if err := s.clearStaleSession(sess); err != nil {
		sess = ""
		return undo(Refuse("%v", err))
	}
	cmd := s.Cfg.ThreadLaunchCommand(j, t, piBin, sessionFile)
	if err := s.Tmux.NewSession(sess, j.Dir, fmt.Sprintf("%s-t%d", j.Slug, t.ID), j.Slug, cmd); err != nil {
		return undo(Refuse("%v", err))
	}
	if err := s.Store.UpdateThreadFields(t.ID, map[string]any{"pi_session": sessionFile, "tmux": sess, "started_at": time.Now().Unix()}); err != nil {
		return undo(err)
	}
	return s.Store.GetThread(t.ID)
}

// CloseThread ends a thread's pi (its tmux session is killed); the thread
// and its conversation stay with the job.
func (s *Service) CloseThread(j *store.Job, t *store.Thread) error {
	if cur, err := s.Store.GetThread(t.ID); err != nil {
		return err
	} else if cur.Closed() {
		return Refuse("thread %q is already closed", t.Name)
	}
	if err := s.Tmux.KillSession(ThreadSessionName(j, t)); err != nil {
		return Refuse("%v", err)
	}
	at, err := s.closeStamp(j)
	if err != nil {
		return err
	}
	return s.Store.UpdateThreadFields(t.ID, map[string]any{"closed_at": at})
}

func (s *Service) closeStamp(j *store.Job) (int64, error) {
	all, err := s.Store.ListThreads(j.ID)
	if err != nil {
		return 0, err
	}
	at := time.Now().Unix()
	for _, t := range all {
		if t.ClosedAt >= at {
			at = t.ClosedAt + 1
		}
	}
	return at, nil
}

func (s *Service) RestoreThread(j *store.Job) (*store.Thread, error) {
	if j.Archived() {
		return nil, Refuse("job %s is archived", j.ArchivedName)
	}
	if j.State == "" || j.State == "done" {
		return nil, Refuse("job %s is not open (%s) — a thread joins a running job", j.Slug, stage(j))
	}
	all, err := s.Store.ListThreads(j.ID)
	if err != nil {
		return nil, err
	}
	var last *store.Thread
	for _, t := range all {
		if !t.Closed() {
			continue
		}
		if last == nil || t.ClosedAt > last.ClosedAt || (t.ClosedAt == last.ClosedAt && t.ID > last.ID) {
			last = t
		}
	}
	if last == nil {
		return nil, Refuse("job %s has no closed tab to bring back", j.Slug)
	}
	if err := s.restartThread(j, last, false); err != nil {
		return nil, err
	}
	if err := s.Store.UpdateThreadFields(last.ID, map[string]any{"closed_at": int64(0), "attention_at": int64(0)}); err != nil {
		return nil, err
	}
	return s.Store.GetThread(last.ID)
}

func tabOrderKey(j *store.Job) string {
	return "tab-order:" + strconv.FormatInt(j.ID, 10)
}

func (s *Service) SetTabOrder(j *store.Job, ids []int64) error {
	all, err := s.Store.ListThreads(j.ID)
	if err != nil {
		return err
	}
	known := map[int64]bool{0: true}
	for _, t := range all {
		known[t.ID] = true
	}
	order := []int64{}
	for _, id := range ids {
		if known[id] {
			known[id] = false
			order = append(order, id)
		}
	}
	raw, err := json.Marshal(order)
	if err != nil {
		return err
	}
	return s.Store.SetKV(tabOrderKey(j), string(raw))
}

func (s *Service) TabOrder(j *store.Job, tabs []int64) []int64 {
	open := make(map[int64]bool, len(tabs))
	for _, id := range tabs {
		open[id] = true
	}
	out := make([]int64, 0, len(tabs))
	for _, id := range s.placedTabs(j, tabs) {
		if open[id] {
			out = append(out, id)
		}
	}
	return out
}

func (s *Service) placedTabs(j *store.Job, tabs []int64) []int64 {
	var saved []int64
	if raw, err := s.Store.GetKV(tabOrderKey(j)); err == nil {
		_ = json.Unmarshal([]byte(raw), &saved)
	}
	seen := make(map[int64]bool, len(saved)+len(tabs))
	out := make([]int64, 0, len(saved)+len(tabs))
	for _, id := range append(saved, tabs...) {
		if !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	return out
}

// CloseOwnThread closes the job's own pi — the first tab — and hands its
// place to the next open thread: that thread's session file, tmux session
// and start time become the job's (its files stay where they are; its name
// becomes the tab's), its row goes, and the old own pi stays as a closed
// thread, so what it said remains part of the job. The last tab cannot be
// closed.
func (s *Service) CloseOwnThread(j *store.Job) error {
	if j.Archived() || j.State == "" || j.State == "done" {
		return Refuse("job %s is not open (%s)", j.Slug, stage(j))
	}
	open, err := s.OpenThreads(j)
	if err != nil {
		return err
	}
	if len(open) == 0 {
		return Refuse("job %s has one thread left — it stays", j.Slug)
	}
	next := open[0]
	tabs := []int64{0}
	for _, t := range open {
		tabs = append(tabs, t.ID)
	}
	order := s.placedTabs(j, tabs)
	if err := s.killIfPresent(j); err != nil {
		return err
	}
	at, err := s.closeStamp(j)
	if err != nil {
		return err
	}
	old := &store.Thread{JobID: j.ID, Name: OwnName(j), PiSession: j.PiSession, Tmux: j.Tmux, StartedAt: j.StartedAt, ClosedAt: at}
	if _, err := s.Store.CreateThread(old); err != nil {
		return err
	}
	for i, id := range order {
		switch id {
		case 0:
			order[i] = old.ID
		case next.ID:
			order[i] = 0
		}
	}
	// the scanner's place in each session follows the session
	s.moveScanKeys(jobKey(j), threadKey(j, old))
	s.moveScanKeys(threadKey(j, next), jobKey(j))
	if _, err := s.Store.UpdateJobFields(j.ID, j.Rev, map[string]any{
		"pi_session": next.PiSession, "tmux": next.Tmux, "started_at": next.StartedAt, "thread_name": next.Name, "own_attention_at": next.AttentionAt,
	}); err != nil {
		return mapStoreErr(err)
	}
	if err := s.Store.DeleteThread(next.ID); err != nil {
		return err
	}
	return s.SetTabOrder(j, order)
}

// moveScanKeys carries a session's scan offset and pending PR calls from
// one scan key to another.
func (s *Service) moveScanKeys(from, to string) {
	for _, key := range []func(string) string{scanOffsetKey, scanPendingKey} {
		if v, err := s.Store.GetKV(key(from)); err == nil {
			_ = s.Store.SetKV(key(to), v)
		} else {
			_ = s.Store.DeleteKV(key(to))
		}
		_ = s.Store.DeleteKV(key(from))
	}
}

// StopThread sends Escape to the thread's pane.
func (s *Service) StopThread(j *store.Job, t *store.Thread) error {
	sess := ThreadSessionName(j, t)
	if t.Closed() || !s.Tmux.HasSession(sess) || s.Tmux.PaneDead(sess) {
		return Refuse("thread %q has no live session", t.Name)
	}
	if err := s.Tmux.SendEscape(sess); err != nil {
		return Refuse("%v", err)
	}
	return nil
}

// RenameThread gives the thread a new name (its tab).
func (s *Service) RenameThread(j *store.Job, t *store.Thread, name string) error {
	name, err := CleanThreadName(name)
	if err != nil {
		return err
	}
	return s.Store.UpdateThreadFields(t.ID, map[string]any{"name": name})
}

func (s *Service) RenameOwnThread(j *store.Job, name string) error {
	name, err := CleanThreadName(name)
	if err != nil {
		return err
	}
	if _, err := s.Store.UpdateJobFields(j.ID, j.Rev, map[string]any{"thread_name": name}); err != nil {
		return mapStoreErr(err)
	}
	return nil
}

// RetryThread is Retry for a thread whose pi gave up or died.
func (s *Service) RetryThread(j *store.Job, t *store.Thread) error {
	if t.Closed() {
		return Refuse("thread %q is closed", t.Name)
	}
	sess := ThreadSessionName(j, t)
	live := s.Tmux.HasSession(sess) && !s.Tmux.PaneDead(sess)
	st := s.Cfg.ReadThreadStatus(j, t)
	conv, err := pi.ReadConversation(t.PiSession)
	if err != nil {
		return err
	}
	if ObserveThread(j.State, t.StartedAt, live, st, time.Now().Unix()).State != "blocked" {
		return Refuse("thread %q is not waiting", t.Name)
	}
	switch planRetry(live, st, conv) {
	case RetryContinue:
		if err := s.Tmux.SendLine(sess, "continue"); err != nil {
			return Refuse("%v", err)
		}
		return nil
	case RetryRestart:
		return s.restartThread(j, t, false)
	case RetryResume:
		return s.restartThread(j, t, true)
	}
	return Refuse("thread %q has nothing to retry: its pi is up and its last turn did not fail", t.Name)
}

func (s *Service) restartThread(j *store.Job, t *store.Thread, resume bool) error {
	if j.Dir == "" || !util.IsDir(j.Dir) {
		return Refuse("dir %q is gone", j.Dir)
	}
	piBin, err := s.resolveTools()
	if err != nil {
		return err
	}
	sess := ThreadSessionName(j, t)
	if s.Tmux.HasSession(sess) {
		if err := s.Tmux.KillSession(sess); err != nil {
			return Refuse("%v", err)
		}
	}
	tdir := s.Cfg.ThreadDir(j, t)
	if err := os.MkdirAll(tdir, 0o755); err != nil {
		return fmt.Errorf("mkdir %s: %w", tdir, err)
	}
	_ = os.Remove(filepath.Join(tdir, "status.json"))
	sessionFile := filepath.Join(tdir, "session.jsonl")
	if fi, err := os.Stat(t.PiSession); t.PiSession != "" && err == nil && fi.Mode().IsRegular() {
		sessionFile = t.PiSession
	}
	var prompt []string
	if resume {
		prompt = []string{"continue"}
	}
	cmd := s.Cfg.ThreadLaunchCommand(j, t, piBin, sessionFile, prompt...)
	if err := s.Tmux.NewSession(sess, j.Dir, fmt.Sprintf("%s-t%d", j.Slug, t.ID), j.Slug, cmd); err != nil {
		_ = s.Tmux.KillSession(sess)
		return Refuse("%v", err)
	}
	return s.Store.UpdateThreadFields(t.ID, map[string]any{"tmux": sess, "pi_session": sessionFile, "started_at": time.Now().Unix()})
}

// closeThreads ends every open thread of j (DONE); their rows stay.
func (s *Service) closeThreads(j *store.Job) error {
	threads, err := s.OpenThreads(j)
	if err != nil {
		return err
	}
	for _, t := range threads {
		if err := s.CloseThread(j, t); err != nil {
			return err
		}
	}
	return nil
}

// dropThreads ends and forgets every thread of j (cancel): the job starts
// over, and so does its thread list. Their dirs stay with the job dir.
func (s *Service) dropThreads(j *store.Job) error {
	threads, err := s.Store.ListThreads(j.ID)
	if err != nil {
		return err
	}
	for _, t := range threads {
		if !t.Closed() {
			if err := s.Tmux.KillSession(ThreadSessionName(j, t)); err != nil {
				return Refuse("%v", err)
			}
		}
		_ = s.Store.DeleteKV(scanOffsetKey(threadKey(j, t)))
		_ = s.Store.DeleteKV(scanPendingKey(threadKey(j, t)))
	}
	_ = s.Store.DeleteKV(tabOrderKey(j))
	return s.Store.DeleteThreads(j.ID)
}

// renameThreadSessions moves every live thread session to the job's new
// slug (Rename).
func (s *Service) renameThreadSessions(old, renamed *store.Job) error {
	threads, err := s.OpenThreads(renamed)
	if err != nil {
		return err
	}
	for _, t := range threads {
		from, to := ThreadSessionName(old, t), ThreadSessionName(renamed, t)
		if from == to || !s.Tmux.HasSession(from) {
			continue
		}
		if err := s.Tmux.RenameSession(from, to, renamed.Slug); err != nil {
			return err
		}
		if err := s.Store.UpdateThreadFields(t.ID, map[string]any{"tmux": to}); err != nil {
			return err
		}
	}
	return nil
}

// LookupThreadDir resolves a DRIVER_JOB_DIR to its job and, when it is a
// thread's dir, the thread (nil for the job's own pi).
func (s *Service) LookupThreadDir(dir string) (*store.Job, *store.Thread, error) {
	all, err := s.Store.ListJobs()
	if err != nil {
		return nil, nil, err
	}
	dir = filepath.Clean(dir)
	for _, j := range all {
		if s.Cfg.OwnDir(j) == dir {
			return j, nil, nil
		}
	}
	for _, j := range all {
		// a thread's dir is under the job's root — or is the root, for the
		// job's first pi once another took its place
		if root := s.Cfg.JobDir(j); dir != root && filepath.Dir(dir) != root {
			continue
		}
		threads, err := s.Store.ListThreads(j.ID)
		if err != nil {
			return nil, nil, err
		}
		for _, t := range threads {
			if s.Cfg.ThreadDir(j, t) == dir {
				return j, t, nil
			}
		}
	}
	return nil, nil, store.ErrNotFound
}
