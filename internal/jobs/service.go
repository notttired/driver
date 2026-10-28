package jobs

import (
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"driver/internal/store"
	"driver/internal/util"
)

// Service is the engine's handle: the store, the resolved config and the
// tmux wrapper. The API layer calls its methods; the Daemon ticks on it.
type Service struct {
	Store *store.Store
	Cfg   Config
	Tmux  *Tmux
}

// NewService wires a Service over an open store.
func NewService(st *store.Store, cfg Config) *Service {
	return &Service{Store: st, Cfg: cfg, Tmux: NewTmux()}
}

// Lookup resolves a board slug: an active job by slug first, else an
// archived job by its archive name ("<slug>-<id>", which is what archived
// cards carry as their slug). store.ErrNotFound when neither matches.
func (s *Service) Lookup(slug string) (*store.Job, error) {
	j, err := s.Store.GetJobBySlug(slug)
	if err == nil {
		return j, nil
	}
	if !errors.Is(err, store.ErrNotFound) {
		return nil, err
	}
	return s.Store.GetArchivedJobByName(slug)
}

// LookupArchivedFirst is Lookup with the preference flipped — for the two
// verbs that act on archived jobs (unarchive, delete), since an active job
// may legitimately hold a slug equal to an archive name.
func (s *Service) LookupArchivedFirst(slug string) (*store.Job, error) {
	j, err := s.Store.GetArchivedJobByName(slug)
	if err == nil {
		return j, nil
	}
	if !errors.Is(err, store.ErrNotFound) {
		return nil, err
	}
	return s.Store.GetJobBySlug(slug)
}

// Create makes a not-started draft: slug, the name the operator typed (what
// the board calls the job; "" = the slug), the directory pi will open in
// (the default dir when empty; it must exist) and the model ("" = pi's
// default). Submit opens it.
func (s *Service) Create(slug, name, dir, model string) (*store.Job, error) {
	if !ValidSlug(slug) {
		return nil, Refuse("invalid slug %q (want ^[a-z0-9][a-z0-9-]{0,63}$)", util.OneLine(slug, 40))
	}
	name, err := CleanName(name)
	if err != nil {
		return nil, err
	}
	dir = util.ExpandTilde(strings.TrimSpace(dir))
	if dir == "" {
		dir = s.Cfg.DefaultDir
	}
	if !util.IsDir(dir) {
		return nil, Refuse("dir %q is not an existing directory", dir)
	}
	j := &store.Job{Slug: slug, Name: name, Dir: dir, Model: strings.TrimSpace(model)}
	if _, err := s.Store.CreateJob(j); err != nil {
		if errors.Is(err, store.ErrSlugTaken) {
			return nil, Refuse("slug %q is taken", slug)
		}
		return nil, err
	}
	return j, nil
}

// Editable reports whether dir/model may be edited: not-started drafts only
// (once pi is open they are the session's).
func Editable(j *store.Job) bool { return j.State == "" && !j.Archived() }

// SaveForm replaces dir and model on a not-started draft. mtime 0 skips the
// optimistic lock; otherwise it must equal the job's updated_at. Returns the
// new updated_at.
func (s *Service) SaveForm(j *store.Job, dir, model string, mtime int64) (int64, error) {
	if !Editable(j) {
		return 0, Refuse("job %s is not editable (%s)", j.Slug, stage(j))
	}
	if mtime != 0 && mtime != j.UpdatedAt {
		return 0, Refuse("edited elsewhere — reload")
	}
	dir = util.ExpandTilde(strings.TrimSpace(dir))
	if _, err := s.Store.UpdateJobFields(j.ID, j.Rev, map[string]any{"dir": dir, "model": strings.TrimSpace(model)}); err != nil {
		return 0, mapStoreErr(err)
	}
	fresh, err := s.Store.GetJobByID(j.ID)
	if err != nil {
		return 0, err
	}
	return fresh.UpdatedAt, nil
}

// mapStoreErr turns the store's concurrency sentinels into refusals.
func mapStoreErr(err error) error {
	switch {
	case errors.Is(err, store.ErrStaleRev):
		return Refuse("edited elsewhere — reload")
	case errors.Is(err, store.ErrSlugTaken):
		return Refuse("slug is taken")
	}
	return err
}

// resolveTools finds pi and tmux; both missing read as refusals.
func (s *Service) resolveTools() (piBin string, err error) {
	if !s.Tmux.Available() {
		return "", Refuse("tmux not found on PATH")
	}
	piBin, err = exec.LookPath(s.Cfg.PiBin)
	if err != nil {
		return "", Refuse("pi not found (%s): set DRIVER_PI or PATH in %s", s.Cfg.PiBin, util.EnvFilePath())
	}
	return piBin, nil
}

// stage names a job's stored state for a refusal: the column, except that
// the two kinds of draft are told apart.
func stage(j *store.Job) string {
	switch j.State {
	case "":
		return "not started"
	case "ready":
		return "open, waiting for its first message"
	}
	return j.State
}

// Submit opens a not-started draft (docs/DESIGN.md "Lifecycle": Submit): a
// tmux session running pi in the job's dir, with no prompt — the job is
// briefed in its pane. The job becomes ready; its first agent run makes it
// working.
func (s *Service) Submit(j *store.Job) error {
	if j.Archived() {
		return Refuse("job %s is archived", j.Slug)
	}
	if j.State == "ready" {
		return Refuse("job %s is already open — brief it in its pane", j.Slug)
	}
	if j.State != "" {
		return Refuse("job %s is not a draft (%s)", j.Slug, stage(j))
	}
	if j.Dir == "" || !util.IsDir(j.Dir) {
		return Refuse("dir %q is not an existing directory", j.Dir)
	}
	piBin, err := s.resolveTools()
	if err != nil {
		return err
	}
	sess := FreshSessionName(j)
	if err := s.clearStaleSession(sess); err != nil {
		return Refuse("%v", err)
	}
	jobDir := s.Cfg.FreshJobDir(j)
	if err := os.MkdirAll(jobDir, 0o755); err != nil {
		return fmt.Errorf("mkdir %s: %w", jobDir, err)
	}
	sessionFile := filepath.Join(jobDir, "session.jsonl")
	if _, err := os.Stat(sessionFile); err == nil {
		// a killed run's transcript: keep it, under a dated name
		_ = os.Rename(sessionFile, filepath.Join(jobDir, fmt.Sprintf("session-%d.jsonl", time.Now().Unix())))
	}
	_ = os.Remove(filepath.Join(jobDir, "prompt.md")) // earlier builds passed a spec this way
	_ = os.Remove(filepath.Join(jobDir, "status.json"))
	cmd := s.Cfg.LaunchCommand(j, piBin, jobDir)
	if err := s.Tmux.NewSession(sess, j.Dir, j.Slug, j.Slug, cmd); err != nil {
		_ = s.Tmux.KillSession(sess) // a half-made session must not linger
		return Refuse("%v", err)
	}
	// a fresh transcript: forget the old scan offset and pending PR calls
	_ = s.Store.DeleteKV(scanOffsetKey(j.ID))
	_ = s.Store.DeleteKV(scanPendingKey(j.ID))
	_, err = s.Store.UpdateJobFields(j.ID, j.Rev, map[string]any{
		"tmux": sess, "pi_session": sessionFile, "started_at": time.Now().Unix(),
		"state": "ready", "outcome": "", "finished_at": int64(0),
	})
	if err != nil {
		return fmt.Errorf("session %s is up but the state write failed: %w", sess, mapStoreErr(err))
	}
	return nil
}

// Reactivate relaunches pi on a done job's existing session file.
func (s *Service) Reactivate(j *store.Job) error {
	if j.Archived() {
		return Refuse("job %s is archived — unarchive it first", j.ArchivedName)
	}
	if j.State != "done" {
		return Refuse("job %s is not done (%s)", j.Slug, stage(j))
	}
	if j.Dir == "" || !util.IsDir(j.Dir) {
		return Refuse("dir %q is gone", j.Dir)
	}
	piBin, err := s.resolveTools()
	if err != nil {
		return err
	}
	sess := FreshSessionName(j)
	if err := s.clearStaleSession(sess); err != nil {
		return Refuse("%v", err)
	}
	jobDir := s.Cfg.JobDir(j)
	if err := os.MkdirAll(jobDir, 0o755); err != nil {
		return fmt.Errorf("mkdir %s: %w", jobDir, err)
	}
	_ = os.Remove(filepath.Join(jobDir, "status.json"))
	cmd := s.Cfg.LaunchCommand(j, piBin, jobDir)
	if err := s.Tmux.NewSession(sess, j.Dir, j.Slug, j.Slug, cmd); err != nil {
		_ = s.Tmux.KillSession(sess) // a half-made session must not linger
		return Refuse("%v", err)
	}
	_, err = s.Store.UpdateJobFields(j.ID, j.Rev, map[string]any{
		"tmux": sess, "pi_session": filepath.Join(jobDir, "session.jsonl"), "started_at": time.Now().Unix(),
		"state": "working", "outcome": "", "finished_at": int64(0),
	})
	if err != nil {
		return fmt.Errorf("session %s is up but the state write failed: %w", sess, mapStoreErr(err))
	}
	return nil
}

// Stop sends Escape to a working/blocked job's pane; the next tick reads pi
// idle and writes blocked. A ready job has nothing to interrupt.
func (s *Service) Stop(j *store.Job) error {
	if j.Archived() || (j.State != "working" && j.State != "blocked") {
		return Refuse("job %s is not working or blocked", j.Slug)
	}
	sess := SessionName(j)
	if !s.Tmux.HasSession(sess) || s.Tmux.PaneDead(sess) {
		return Refuse("job %s has no live session", j.Slug)
	}
	if err := s.Tmux.SendEscape(sess); err != nil {
		return Refuse("%v", err)
	}
	return nil
}

// Done kills the session and marks the job done/accepted (from ready,
// working or blocked).
func (s *Service) Done(j *store.Job) error {
	if j.Archived() {
		return Refuse("job %s is archived", j.ArchivedName)
	}
	if j.State == "" || j.State == "done" {
		return Refuse("job %s is %s", j.Slug, stage(j))
	}
	if err := s.killIfPresent(j); err != nil {
		return err
	}
	_, err := s.Store.UpdateJobFields(j.ID, j.Rev, map[string]any{
		"state": "done", "outcome": "accepted", "finished_at": time.Now().Unix(),
	})
	return mapStoreErr(err)
}

// Kill cancels an opened job back to not started; the session file stays on
// disk.
func (s *Service) Kill(j *store.Job) error {
	if j.Archived() {
		return Refuse("job %s is archived", j.ArchivedName)
	}
	if j.State == "" {
		return Refuse("job %s is not started", j.Slug)
	}
	if err := s.killIfPresent(j); err != nil {
		return err
	}
	_, err := s.Store.UpdateJobFields(j.ID, j.Rev, map[string]any{
		"state": "", "tmux": "", "started_at": int64(0), "finished_at": int64(0), "outcome": "",
	})
	return mapStoreErr(err)
}

// clearStaleSession kills a session left under a name the job's stored state
// says should be free: a launch that failed half-way or a kill that did not
// stick. Only driver creates drv-* names, so nothing else lives there.
func (s *Service) clearStaleSession(sess string) error {
	if !s.Tmux.HasSession(sess) {
		return nil
	}
	log.Printf("%s: killing stale tmux session", sess)
	return s.Tmux.KillSession(sess)
}

func (s *Service) killIfPresent(j *store.Job) error {
	sess := SessionName(j)
	if s.Tmux.Available() && s.Tmux.HasSession(sess) {
		if err := s.Tmux.KillSession(sess); err != nil {
			return Refuse("%v", err)
		}
	}
	return nil
}

// Archive moves a done job into the drawer (its slug is freed).
func (s *Service) Archive(j *store.Job) error {
	if j.Archived() {
		return Refuse("job %s is already archived", j.ArchivedName)
	}
	if j.State != "done" {
		return Refuse("job %s is not done (%s)", j.Slug, stage(j))
	}
	sess := SessionName(j)
	if s.Tmux.Available() && s.Tmux.HasSession(sess) {
		if !s.Tmux.PaneDead(sess) {
			return Refuse("job %s still has a live tmux session", j.Slug)
		}
		_ = s.Tmux.KillSession(sess)
	}
	_, err := s.Store.ArchiveJob(j.ID)
	return err
}

// Unarchive restores an archived job; refuses when an active job holds the slug.
func (s *Service) Unarchive(j *store.Job) error {
	if !j.Archived() {
		return Refuse("job %s is not archived", j.Slug)
	}
	if err := s.Store.UnarchiveJob(j.ID); err != nil {
		if errors.Is(err, store.ErrSlugTaken) {
			return Refuse("an active job already holds the slug %q", j.Slug)
		}
		return err
	}
	return nil
}

// Delete removes an archived job for good, job dir included.
func (s *Service) Delete(j *store.Job) error {
	if !j.Archived() {
		return Refuse("job %s is not archived", j.Slug)
	}
	sess := SessionName(j)
	if s.Tmux.Available() && s.Tmux.HasSession(sess) && !s.Tmux.PaneDead(sess) {
		return Refuse("job %s still has a live tmux session", j.ArchivedName)
	}
	if err := s.Store.DeleteJob(j.ID); err != nil {
		return err
	}
	for _, d := range []string{s.Cfg.JobDir(j), s.Cfg.FreshJobDir(j)} {
		// only ever below <home>/jobs/, and never the jobs dir itself
		if j.ID != 0 && strings.HasPrefix(d, s.Cfg.JobsDir()+string(filepath.Separator)) {
			_ = os.RemoveAll(d)
		}
	}
	return nil
}

// Rename changes what a job is called: its name (what the board shows; "" =
// the slug) and its slug (and its tmux session's name when the session
// exists). Returns the slug the job now has.
func (s *Service) Rename(j *store.Job, to, name string) (string, error) {
	to = strings.TrimSpace(to)
	if !ValidSlug(to) {
		return "", Refuse("invalid slug %q (want ^[a-z0-9][a-z0-9-]{0,63}$)", util.OneLine(to, 40))
	}
	name, err := CleanName(name)
	if err != nil {
		return "", err
	}
	if j.Archived() {
		return "", Refuse("job %s is archived", j.ArchivedName)
	}
	fields := map[string]any{}
	if name != j.Name {
		fields["name"] = name
	}
	if to == j.Slug {
		// the name alone changes: no session to rename, no slug to free
		if len(fields) == 0 {
			return to, nil
		}
		if _, err := s.Store.UpdateJobFields(j.ID, j.Rev, fields); err != nil {
			return "", mapStoreErr(err)
		}
		return to, nil
	}
	if _, err := s.Store.GetJobBySlug(to); err == nil {
		return "", Refuse("slug %q is taken", to)
	} else if !errors.Is(err, store.ErrNotFound) {
		return "", err
	}
	fields["slug"] = to
	renamed := &store.Job{ID: j.ID, Slug: to}
	oldSess, newSess := SessionName(j), FreshSessionName(renamed)
	sessionExists := j.Tmux != "" && s.Tmux.Available() && s.Tmux.HasSession(oldSess)
	if sessionExists {
		if err := s.Tmux.RenameSession(oldSess, newSess, to); err != nil {
			return "", Refuse("%v", err)
		}
	}
	if j.Tmux != "" {
		fields["tmux"] = newSess
	}
	if _, err := s.Store.UpdateJobFields(j.ID, j.Rev, fields); err != nil {
		if sessionExists {
			_ = s.Tmux.RenameSession(newSess, oldSess, j.Slug)
		}
		if errors.Is(err, store.ErrSlugTaken) {
			return "", Refuse("slug %q is taken", to)
		}
		return "", mapStoreErr(err)
	}
	return to, nil
}

// SetColor sets or clears ("") the card colour.
func (s *Service) SetColor(j *store.Job, color string) error {
	if color != "" && !ValidColor(color) {
		return fmt.Errorf("unknown color %s: %w", util.OneLine(color, 24), ErrBadInput)
	}
	if j.Archived() {
		return Refuse("job %s is archived", j.ArchivedName)
	}
	_, err := s.Store.UpdateJobFields(j.ID, j.Rev, map[string]any{"color": color})
	return mapStoreErr(err)
}

// MaxNotesRunes caps the operator's notes.
const MaxNotesRunes = 64000

// SetNotes replaces the operator's private notes.
func (s *Service) SetNotes(j *store.Job, text string) error {
	if utf8.RuneCountInString(text) > MaxNotesRunes {
		return Refuse("notes exceed %d characters", MaxNotesRunes)
	}
	if j.Archived() {
		return Refuse("job %s is archived", j.ArchivedName)
	}
	_, err := s.Store.UpdateJobFields(j.ID, j.Rev, map[string]any{"notes": text})
	return mapStoreErr(err)
}

// AddLink attaches a URL with its detected kind; a duplicate is a refusal.
func (s *Service) AddLink(j *store.Job, rawURL string) (*store.Link, error) {
	rawURL = strings.TrimSpace(rawURL)
	if !ValidLinkURL(rawURL) {
		return nil, Refuse("bad URL (must be http or https)")
	}
	l, created, err := s.Store.AddLink(j.ID, LinkKind(rawURL), rawURL)
	if err != nil {
		return nil, err
	}
	if !created {
		return nil, Refuse("that URL is already linked")
	}
	return l, nil
}

// AttachPR idempotently adds a github_pr link; attached is false when the
// URL was already there.
func (s *Service) AttachPR(j *store.Job, rawURL string) (attached bool, err error) {
	rawURL = strings.TrimSpace(rawURL)
	if LinkKind(rawURL) != "github_pr" {
		return false, Refuse("not a GitHub pull request URL")
	}
	_, created, err := s.Store.AddLink(j.ID, "github_pr", rawURL)
	return created, err
}

// RemoveLink deletes one of the job's links.
func (s *Service) RemoveLink(j *store.Job, id int64) error { return s.Store.RemoveLink(j.ID, id) }

// Reorder saves a column's full intended order; slugs whose display state no
// longer matches the column are dropped silently.
func (s *Service) Reorder(column string, slugs []string) error {
	if !IsColumn(column) {
		return Refuse("unknown column %q", util.OneLine(column, 24))
	}
	jobs, err := s.Store.ListJobs()
	if err != nil {
		return err
	}
	bySlug := make(map[string]*store.Job, len(jobs))
	for _, j := range jobs {
		bySlug[j.Slug] = j
	}
	ids := make([]int64, 0, len(slugs))
	for _, slug := range slugs {
		if j := bySlug[slug]; j != nil && DisplayState(j) == column {
			ids = append(ids, j.ID)
		}
	}
	return s.Store.SetBoardColumn(column, ids)
}
