package store

import (
	"database/sql"
	"errors"
	"fmt"
)

// Link is one row of the links table: a free-form URL attached to a job with
// a detected kind and, for GitHub PRs, a polled status.
type Link struct {
	ID        int64
	JobID     int64
	Kind      string // github_pr | github | slack | linear | url
	URL       string
	Label     string
	Status    string // github_pr: '' | open | draft | merged | closed
	StatusAt  int64
	CreatedAt int64
}

const linkColumns = `id, job_id, kind, url, label, status, status_at, created_at`

func scanLink(row interface{ Scan(...any) error }) (*Link, error) {
	var l Link
	if err := row.Scan(&l.ID, &l.JobID, &l.Kind, &l.URL, &l.Label, &l.Status, &l.StatusAt, &l.CreatedAt); err != nil {
		return nil, err
	}
	return &l, nil
}

// AddLink attaches url to job jobID. It is idempotent on (job_id, url): an
// existing row is returned with created == false and left untouched.
func (s *Store) AddLink(jobID int64, kind, url, label string) (link *Link, created bool, err error) {
	res, err := s.db.Exec(`INSERT INTO links (job_id, kind, url, label, created_at) VALUES (?, ?, ?, ?, ?)`,
		jobID, kind, url, label, now())
	if err != nil {
		if isUniqueErr(err) {
			l, gerr := s.linkByURL(jobID, url)
			if gerr != nil {
				return nil, false, gerr
			}
			return l, false, nil
		}
		return nil, false, fmt.Errorf("add link: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return nil, false, fmt.Errorf("add link: %w", err)
	}
	l, err := s.GetLink(jobID, id)
	if err != nil {
		return nil, false, err
	}
	return l, true, nil
}

func (s *Store) linkByURL(jobID int64, url string) (*Link, error) {
	l, err := scanLink(s.db.QueryRow(`SELECT `+linkColumns+` FROM links WHERE job_id = ? AND url = ?`, jobID, url))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get link: %w", err)
	}
	return l, nil
}

// GetLink returns link id, which must belong to job jobID.
func (s *Store) GetLink(jobID, id int64) (*Link, error) {
	l, err := scanLink(s.db.QueryRow(`SELECT `+linkColumns+` FROM links WHERE id = ? AND job_id = ?`, id, jobID))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get link %d: %w", id, err)
	}
	return l, nil
}

// ListLinks returns a job's links in created order.
func (s *Store) ListLinks(jobID int64) ([]*Link, error) {
	return s.queryLinks(`SELECT `+linkColumns+` FROM links WHERE job_id = ? ORDER BY created_at, id`, jobID)
}

// ListAllLinks returns every link keyed by job id, in created order — one
// query for the whole board.
func (s *Store) ListAllLinks() (map[int64][]*Link, error) {
	ls, err := s.queryLinks(`SELECT ` + linkColumns + ` FROM links ORDER BY created_at, id`)
	if err != nil {
		return nil, err
	}
	out := map[int64][]*Link{}
	for _, l := range ls {
		out[l.JobID] = append(out[l.JobID], l)
	}
	return out, nil
}

// StalePRLinks returns up to limit github_pr links whose status_at is older
// than before (unix seconds), oldest first — the PR refresh's work list.
func (s *Store) StalePRLinks(before int64, limit int) ([]*Link, error) {
	return s.queryLinks(`SELECT `+linkColumns+` FROM links WHERE kind = 'github_pr' AND status_at < ? ORDER BY status_at, id LIMIT ?`, before, limit)
}

func (s *Store) queryLinks(query string, args ...any) ([]*Link, error) {
	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("list links: %w", err)
	}
	defer rows.Close()
	var out []*Link
	for rows.Next() {
		l, err := scanLink(rows)
		if err != nil {
			return nil, fmt.Errorf("list links: %w", err)
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

// RemoveLink deletes link id from job jobID; ErrNotFound when no such row.
func (s *Store) RemoveLink(jobID, id int64) error {
	res, err := s.db.Exec(`DELETE FROM links WHERE id = ? AND job_id = ?`, id, jobID)
	if err != nil {
		return fmt.Errorf("remove link %d: %w", id, err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("remove link %d: %w", id, ErrNotFound)
	}
	return nil
}

// RelabelLink sets link id's user label.
func (s *Store) RelabelLink(jobID, id int64, label string) error {
	res, err := s.db.Exec(`UPDATE links SET label = ? WHERE id = ? AND job_id = ?`, label, id, jobID)
	if err != nil {
		return fmt.Errorf("relabel link %d: %w", id, err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("relabel link %d: %w", id, ErrNotFound)
	}
	return nil
}

// SetLinkStatus records a PR poll result: status and the poll time.
func (s *Store) SetLinkStatus(id int64, status string, at int64) error {
	if _, err := s.db.Exec(`UPDATE links SET status = ?, status_at = ? WHERE id = ?`, status, at, id); err != nil {
		return fmt.Errorf("set link %d status: %w", id, err)
	}
	return nil
}
