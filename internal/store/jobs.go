package store

import (
	"database/sql"
	"errors"
	"fmt"
	"sort"
)

// Job is one row of the jobs table. State is the STORED state (” = draft,
// working, blocked, done); the display state is the jobs package's concern.
// ArchivedAt == 0 means not archived. All times are unix seconds.
type Job struct {
	ID           int64
	Slug         string
	Dir          string
	Model        string
	Color        string
	Spec         string
	Notes        string
	State        string
	Outcome      string
	PiSession    string
	Tmux         string
	StartedAt    int64
	FinishedAt   int64
	CreatedAt    int64
	UpdatedAt    int64
	ArchivedAt   int64
	ArchivedName string
	Rev          int64
}

// Archived reports whether the job sits in the archive drawer.
func (j *Job) Archived() bool { return j.ArchivedAt != 0 }

const jobColumns = `id, slug, dir, model, color, spec, notes, state, outcome, pi_session, tmux,
	started_at, finished_at, created_at, updated_at, archived_at, archived_name, rev`

func scanJob(row interface{ Scan(...any) error }) (*Job, error) {
	var j Job
	var archivedAt sql.NullInt64
	var archivedName sql.NullString
	err := row.Scan(&j.ID, &j.Slug, &j.Dir, &j.Model, &j.Color, &j.Spec, &j.Notes, &j.State, &j.Outcome,
		&j.PiSession, &j.Tmux, &j.StartedAt, &j.FinishedAt, &j.CreatedAt, &j.UpdatedAt,
		&archivedAt, &archivedName, &j.Rev)
	if err != nil {
		return nil, err
	}
	j.ArchivedAt = archivedAt.Int64
	j.ArchivedName = archivedName.String
	return &j, nil
}

// CreateJob inserts j (a draft, normally) and fills in its ID, timestamps and
// rev. A slug held by an active job is ErrSlugTaken.
func (s *Store) CreateJob(j *Job) (int64, error) {
	t := now()
	if j.CreatedAt == 0 {
		j.CreatedAt = t
	}
	if j.UpdatedAt == 0 {
		j.UpdatedAt = t
	}
	if j.Rev == 0 {
		j.Rev = 1
	}
	res, err := s.db.Exec(`INSERT INTO jobs (slug, dir, model, color, spec, notes, state, outcome, pi_session, tmux,
		started_at, finished_at, created_at, updated_at, rev)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		j.Slug, j.Dir, j.Model, j.Color, j.Spec, j.Notes, j.State, j.Outcome, j.PiSession, j.Tmux,
		j.StartedAt, j.FinishedAt, j.CreatedAt, j.UpdatedAt, j.Rev)
	if err != nil {
		if isUniqueErr(err) {
			return 0, fmt.Errorf("create job %s: %w", j.Slug, ErrSlugTaken)
		}
		return 0, fmt.Errorf("create job %s: %w", j.Slug, err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("create job %s: %w", j.Slug, err)
	}
	j.ID = id
	return id, nil
}

// GetJobBySlug returns the ACTIVE job with slug, or ErrNotFound.
func (s *Store) GetJobBySlug(slug string) (*Job, error) {
	j, err := scanJob(s.db.QueryRow(`SELECT `+jobColumns+` FROM jobs WHERE slug = ? AND archived_at IS NULL`, slug))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get job %s: %w", slug, err)
	}
	return j, nil
}

// GetArchivedJobByName returns the archived job whose archive name is name.
func (s *Store) GetArchivedJobByName(name string) (*Job, error) {
	j, err := scanJob(s.db.QueryRow(`SELECT `+jobColumns+` FROM jobs WHERE archived_name = ? AND archived_at IS NOT NULL`, name))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get archived job %s: %w", name, err)
	}
	return j, nil
}

// GetJobByID returns the job (active or archived) with id, or ErrNotFound.
func (s *Store) GetJobByID(id int64) (*Job, error) {
	j, err := scanJob(s.db.QueryRow(`SELECT `+jobColumns+` FROM jobs WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get job %d: %w", id, err)
	}
	return j, nil
}

// ListJobs returns every active job, by slug.
func (s *Store) ListJobs() ([]*Job, error) {
	return s.queryJobs(`SELECT ` + jobColumns + ` FROM jobs WHERE archived_at IS NULL ORDER BY slug`)
}

// ListArchivedJobs returns every archived job, newest archive first.
func (s *Store) ListArchivedJobs() ([]*Job, error) {
	return s.queryJobs(`SELECT ` + jobColumns + ` FROM jobs WHERE archived_at IS NOT NULL ORDER BY archived_at DESC, id DESC`)
}

func (s *Store) queryJobs(query string, args ...any) ([]*Job, error) {
	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("list jobs: %w", err)
	}
	defer rows.Close()
	var out []*Job
	for rows.Next() {
		j, err := scanJob(rows)
		if err != nil {
			return nil, fmt.Errorf("list jobs: %w", err)
		}
		out = append(out, j)
	}
	return out, rows.Err()
}

// jobFieldColumns is the allowlist UpdateJobFields accepts.
var jobFieldColumns = map[string]bool{
	"slug": true, "dir": true, "model": true, "color": true, "spec": true, "notes": true,
	"state": true, "outcome": true, "pi_session": true, "tmux": true,
	"started_at": true, "finished_at": true,
}

// UpdateJobFields applies a partial update to the active job id, gated on
// rev matching exactly (optimistic concurrency). On success it bumps rev,
// stamps updated_at and returns the new rev. A zero-row update is
// disambiguated: ErrNotFound when the row is gone or archived, ErrStaleRev
// when it exists and its rev simply moved. A slug collision is ErrSlugTaken.
func (s *Store) UpdateJobFields(id, rev int64, fields map[string]any) (int64, error) {
	if len(fields) == 0 {
		return 0, fmt.Errorf("update job %d: no fields", id)
	}
	keys := make([]string, 0, len(fields))
	for k := range fields {
		if !jobFieldColumns[k] {
			return 0, fmt.Errorf("update job %d: field %q is not updatable", id, k)
		}
		keys = append(keys, k)
	}
	sort.Strings(keys)
	set := ""
	args := make([]any, 0, len(keys)+3)
	for _, k := range keys {
		set += k + " = ?, "
		args = append(args, fields[k])
	}
	set += "rev = rev + 1, updated_at = ?"
	args = append(args, now(), id, rev)
	res, err := s.db.Exec(`UPDATE jobs SET `+set+` WHERE id = ? AND rev = ? AND archived_at IS NULL`, args...)
	if err != nil {
		if isUniqueErr(err) {
			return 0, fmt.Errorf("update job %d: %w", id, ErrSlugTaken)
		}
		return 0, fmt.Errorf("update job %d: %w", id, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("update job %d: %w", id, err)
	}
	if n == 0 {
		var archivedAt sql.NullInt64
		lookup := s.db.QueryRow(`SELECT archived_at FROM jobs WHERE id = ?`, id).Scan(&archivedAt)
		switch {
		case errors.Is(lookup, sql.ErrNoRows):
			return 0, fmt.Errorf("update job %d: %w", id, ErrNotFound)
		case lookup != nil:
			return 0, fmt.Errorf("update job %d: %w", id, lookup)
		case archivedAt.Valid:
			return 0, fmt.Errorf("update job %d: archived job is immutable: %w", id, ErrNotFound)
		default:
			return 0, fmt.Errorf("update job %d at rev %d: %w", id, rev, ErrStaleRev)
		}
	}
	return rev + 1, nil
}

// ArchiveJob moves the active job id into the archive under the name
// "<slug>-<id>" (unique by construction), freeing its slug. Returns the name.
func (s *Store) ArchiveJob(id int64) (string, error) {
	var slug string
	err := s.db.QueryRow(`SELECT slug FROM jobs WHERE id = ? AND archived_at IS NULL`, id).Scan(&slug)
	if errors.Is(err, sql.ErrNoRows) {
		return "", fmt.Errorf("archive job %d: %w", id, ErrNotFound)
	}
	if err != nil {
		return "", fmt.Errorf("archive job %d: %w", id, err)
	}
	name := fmt.Sprintf("%s-%d", slug, id)
	t := now()
	if _, err := s.db.Exec(`UPDATE jobs SET archived_at = ?, archived_name = ?, rev = rev + 1, updated_at = ? WHERE id = ?`,
		t, name, t, id); err != nil {
		return "", fmt.Errorf("archive job %d: %w", id, err)
	}
	return name, nil
}

// UnarchiveJob restores the archived job id to the active set under its
// original slug; ErrSlugTaken when an active job now holds that slug.
func (s *Store) UnarchiveJob(id int64) error {
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("unarchive job %d: %w", id, err)
	}
	defer tx.Rollback()
	var slug string
	err = tx.QueryRow(`SELECT slug FROM jobs WHERE id = ? AND archived_at IS NOT NULL`, id).Scan(&slug)
	if errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("unarchive job %d: %w", id, ErrNotFound)
	}
	if err != nil {
		return fmt.Errorf("unarchive job %d: %w", id, err)
	}
	var taken int
	err = tx.QueryRow(`SELECT 1 FROM jobs WHERE slug = ? AND archived_at IS NULL`, slug).Scan(&taken)
	if err == nil {
		return fmt.Errorf("unarchive job %d: an active job holds slug %q: %w", id, slug, ErrSlugTaken)
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("unarchive job %d: %w", id, err)
	}
	if _, err := tx.Exec(`UPDATE jobs SET archived_at = NULL, archived_name = NULL, rev = rev + 1, updated_at = ? WHERE id = ?`,
		now(), id); err != nil {
		return fmt.Errorf("unarchive job %d: %w", id, err)
	}
	return tx.Commit()
}

// DeleteJob removes job id for good; its links cascade and its board_order
// rows are pruned.
func (s *Store) DeleteJob(id int64) error {
	res, err := s.db.Exec(`DELETE FROM jobs WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete job %d: %w", id, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("delete job %d: %w", id, err)
	}
	if n == 0 {
		return fmt.Errorf("delete job %d: %w", id, ErrNotFound)
	}
	_, _ = s.db.Exec(`DELETE FROM board_order WHERE job_id = ?`, id)
	return nil
}
