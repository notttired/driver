package store

import (
	"database/sql"
	"errors"
	"fmt"
)

// Link is one row of the links table: a URL attached to a job with a detected
// kind and, for GitHub PRs, a polled status. A link has no label: it reads as
// its URL (the UI shows a PR as #N, anything else as its host).
type Link struct {
	ID        int64
	JobID     int64
	Kind      string // github_pr | github | slack | linear | url
	URL       string
	Status    string // github_pr: '' | open | draft | merged | closed
	StatusAt  int64
	CreatedAt int64
	PinnedAt  int64
	Mentions  int
	Yours     bool
}

const linkSelect = `SELECT l.id, l.job_id, l.kind, l.url, l.status, l.status_at, l.created_at, l.pinned_at,
	COUNT(m.link_id) AS mentions, COALESCE(MAX(m.role = 'user'), 0) AS yours
	FROM links l LEFT JOIN link_mentions m ON m.link_id = l.id`

const linkOrder = ` GROUP BY l.id ORDER BY l.pinned_at DESC, l.created_at DESC, l.id DESC`

func scanLink(row interface{ Scan(...any) error }) (*Link, error) {
	var l Link
	var yours int64
	if err := row.Scan(&l.ID, &l.JobID, &l.Kind, &l.URL, &l.Status, &l.StatusAt, &l.CreatedAt, &l.PinnedAt, &l.Mentions, &yours); err != nil {
		return nil, err
	}
	l.Yours = yours != 0
	return &l, nil
}

// AddLink attaches url to job jobID. It is idempotent on (job_id, url): an
// existing row is returned with created == false and left untouched.
func (s *Store) AddLink(jobID int64, kind, url string) (link *Link, created bool, err error) {
	res, err := s.db.Exec(`INSERT INTO links (job_id, kind, url, created_at) VALUES (?, ?, ?, ?)`,
		jobID, kind, url, now())
	if err != nil {
		if isUniqueErr(err) {
			l, gerr := s.LinkByURL(jobID, url)
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

// CopyLinks gives job to every link of job from that it does not have yet —
// kind, url, status, pin, mentions and timestamps carried over, so a fork's
// links read exactly as the original's. It returns how many were added.
func (s *Store) CopyLinks(from, to int64) (int, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return 0, fmt.Errorf("copy links %d → %d: %w", from, to, err)
	}
	defer tx.Rollback()
	res, err := tx.Exec(`INSERT OR IGNORE INTO links (job_id, kind, url, status, status_at, created_at, pinned_at)
		SELECT ?, kind, url, status, status_at, created_at, pinned_at FROM links WHERE job_id = ? ORDER BY created_at, id`, to, from)
	if err != nil {
		return 0, fmt.Errorf("copy links %d → %d: %w", from, to, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("copy links %d → %d: %w", from, to, err)
	}
	if _, err := tx.Exec(`INSERT OR IGNORE INTO link_mentions (link_id, entry_id, role)
		SELECT n.id, m.entry_id, m.role FROM link_mentions m
		JOIN links o ON o.id = m.link_id
		JOIN links n ON n.job_id = ? AND n.url = o.url
		WHERE o.job_id = ?`, to, from); err != nil {
		return 0, fmt.Errorf("copy link mentions %d → %d: %w", from, to, err)
	}
	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("copy links %d → %d: %w", from, to, err)
	}
	return int(n), nil
}

func (s *Store) LinkByURL(jobID int64, url string) (*Link, error) {
	l, err := scanLink(s.db.QueryRow(linkSelect+` WHERE l.job_id = ? AND l.url = ? GROUP BY l.id`, jobID, url))
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
	l, err := scanLink(s.db.QueryRow(linkSelect+` WHERE l.id = ? AND l.job_id = ? GROUP BY l.id`, id, jobID))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get link %d: %w", id, err)
	}
	return l, nil
}

// ListLinks returns a job's links in display order: the pinned ones, the
// last pinned first, then the rest, newest first.
func (s *Store) ListLinks(jobID int64) ([]*Link, error) {
	return s.queryLinks(linkSelect+` WHERE l.job_id = ?`+linkOrder, jobID)
}

// ListAllLinks returns every link keyed by job id, in display order — one
// query for the whole board.
func (s *Store) ListAllLinks() (map[int64][]*Link, error) {
	ls, err := s.queryLinks(linkSelect + linkOrder)
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
	return s.queryLinks(linkSelect+` WHERE l.kind = 'github_pr' AND l.status_at < ? GROUP BY l.id ORDER BY l.status_at, l.id LIMIT ?`, before, limit)
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

// SetLinkStatus records a PR poll result: status and the poll time.
func (s *Store) SetLinkStatus(id int64, status string, at int64) error {
	if _, err := s.db.Exec(`UPDATE links SET status = ?, status_at = ? WHERE id = ?`, status, at, id); err != nil {
		return fmt.Errorf("set link %d status: %w", id, err)
	}
	return nil
}

func (s *Store) AddMention(linkID int64, entryID, role string) error {
	if _, err := s.db.Exec(`INSERT OR IGNORE INTO link_mentions (link_id, entry_id, role) VALUES (?, ?, ?)`, linkID, entryID, role); err != nil {
		return fmt.Errorf("add mention of link %d: %w", linkID, err)
	}
	return nil
}

func (s *Store) PinLink(jobID int64, url string, at int64) (bool, error) {
	res, err := s.db.Exec(`UPDATE links SET pinned_at = ? WHERE job_id = ? AND url = ?`, at, jobID, url)
	if err != nil {
		return false, fmt.Errorf("pin link %s: %w", url, err)
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

func (s *Store) UnpinLink(jobID, id int64) error {
	res, err := s.db.Exec(`UPDATE links SET pinned_at = 0 WHERE id = ? AND job_id = ?`, id, jobID)
	if err != nil {
		return fmt.Errorf("unpin link %d: %w", id, err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("unpin link %d: %w", id, ErrNotFound)
	}
	return nil
}
