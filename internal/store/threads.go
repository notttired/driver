package store

import (
	"database/sql"
	"errors"
	"fmt"
	"sort"
)

// Thread is one more pi on a job beside the job's own (the job row carries
// the first thread: pi_session, tmux, started_at). Each has its own session
// file, tmux session and status, in a dir of its own under the job dir.
// ClosedAt set = its pi was closed; the row and its session file stay, so
// what it said remains part of the job.
type Thread struct {
	ID        int64
	JobID     int64
	Name      string
	PiSession string
	Tmux      string
	StartedAt int64
	CreatedAt int64
	ClosedAt  int64
}

// Closed reports whether the thread's pi was closed.
func (t *Thread) Closed() bool { return t.ClosedAt != 0 }

const threadColumns = `id, job_id, name, pi_session, tmux, started_at, created_at, closed_at`

func scanThread(row interface{ Scan(...any) error }) (*Thread, error) {
	var t Thread
	if err := row.Scan(&t.ID, &t.JobID, &t.Name, &t.PiSession, &t.Tmux, &t.StartedAt, &t.CreatedAt, &t.ClosedAt); err != nil {
		return nil, err
	}
	return &t, nil
}

// CreateThread inserts t and fills in its ID and CreatedAt.
func (s *Store) CreateThread(t *Thread) (int64, error) {
	if t.CreatedAt == 0 {
		t.CreatedAt = now()
	}
	res, err := s.db.Exec(`INSERT INTO threads (job_id, name, pi_session, tmux, started_at, created_at, closed_at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		t.JobID, t.Name, t.PiSession, t.Tmux, t.StartedAt, t.CreatedAt, t.ClosedAt)
	if err != nil {
		return 0, fmt.Errorf("create thread on job %d: %w", t.JobID, err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("create thread on job %d: %w", t.JobID, err)
	}
	t.ID = id
	return id, nil
}

// GetThread returns the thread with id, or ErrNotFound.
func (s *Store) GetThread(id int64) (*Thread, error) {
	t, err := scanThread(s.db.QueryRow(`SELECT `+threadColumns+` FROM threads WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get thread %d: %w", id, err)
	}
	return t, nil
}

// ListThreads returns a job's threads, open and closed, oldest first.
func (s *Store) ListThreads(jobID int64) ([]*Thread, error) {
	rows, err := s.db.Query(`SELECT `+threadColumns+` FROM threads WHERE job_id = ? ORDER BY id`, jobID)
	if err != nil {
		return nil, fmt.Errorf("list threads of job %d: %w", jobID, err)
	}
	defer rows.Close()
	out := []*Thread{}
	for rows.Next() {
		t, err := scanThread(rows)
		if err != nil {
			return nil, fmt.Errorf("list threads of job %d: %w", jobID, err)
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

var threadFieldColumns = map[string]bool{"name": true, "pi_session": true, "tmux": true, "started_at": true, "closed_at": true}

// UpdateThreadFields applies a partial update to thread id.
func (s *Store) UpdateThreadFields(id int64, fields map[string]any) error {
	if len(fields) == 0 {
		return fmt.Errorf("update thread %d: no fields", id)
	}
	keys := make([]string, 0, len(fields))
	for k := range fields {
		if !threadFieldColumns[k] {
			return fmt.Errorf("update thread %d: field %q is not updatable", id, k)
		}
		keys = append(keys, k)
	}
	sort.Strings(keys)
	set := ""
	args := make([]any, 0, len(keys)+1)
	for i, k := range keys {
		if i > 0 {
			set += ", "
		}
		set += k + " = ?"
		args = append(args, fields[k])
	}
	args = append(args, id)
	res, err := s.db.Exec(`UPDATE threads SET `+set+` WHERE id = ?`, args...)
	if err != nil {
		return fmt.Errorf("update thread %d: %w", id, err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("update thread %d: %w", id, ErrNotFound)
	}
	return nil
}

// DeleteThread removes one thread.
func (s *Store) DeleteThread(id int64) error {
	if _, err := s.db.Exec(`DELETE FROM threads WHERE id = ?`, id); err != nil {
		return fmt.Errorf("delete thread %d: %w", id, err)
	}
	return nil
}

// DeleteThreads removes every thread of a job.
func (s *Store) DeleteThreads(jobID int64) error {
	if _, err := s.db.Exec(`DELETE FROM threads WHERE job_id = ?`, jobID); err != nil {
		return fmt.Errorf("delete threads of job %d: %w", jobID, err)
	}
	return nil
}
