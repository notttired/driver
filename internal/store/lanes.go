package store

import (
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// Lane is a column of the operator's own beside the four state columns; the
// jobs parked in it keep their state and stay put while it changes.
type Lane struct {
	ID       int64
	Name     string
	Position int64
}

const laneColumnPrefix = "lane:"

// WatchingColumn is the board_order key of WATCHING, the built-in column a
// job is parked in while its pi watches over the shipped work.
const WatchingColumn = "watching"

// ParkedColumn is the column a job is parked in, away from the one its state
// names: WATCHING, its lane, or "" for neither.
func ParkedColumn(j *Job) string {
	if j.WatchingAt != 0 {
		return WatchingColumn
	}
	if j.LaneID != 0 {
		return LaneColumn(j.LaneID)
	}
	return ""
}

// LaneColumn is the board_order key for a lane.
func LaneColumn(id int64) string { return laneColumnPrefix + strconv.FormatInt(id, 10) }

// ParseLaneColumn reads a lane's id out of its board_order key.
func ParseLaneColumn(column string) (int64, bool) {
	rest, ok := strings.CutPrefix(column, laneColumnPrefix)
	if !ok {
		return 0, false
	}
	id, err := strconv.ParseInt(rest, 10, 64)
	if err != nil || id <= 0 {
		return 0, false
	}
	return id, true
}

func (s *Store) ListLanes() ([]*Lane, error) {
	rows, err := s.db.Query(`SELECT id, name, position FROM lanes ORDER BY position, id`)
	if err != nil {
		return nil, fmt.Errorf("list lanes: %w", err)
	}
	defer rows.Close()
	out := []*Lane{}
	for rows.Next() {
		var l Lane
		if err := rows.Scan(&l.ID, &l.Name, &l.Position); err != nil {
			return nil, fmt.Errorf("list lanes: %w", err)
		}
		out = append(out, &l)
	}
	return out, rows.Err()
}

func (s *Store) GetLane(id int64) (*Lane, error) {
	var l Lane
	err := s.db.QueryRow(`SELECT id, name, position FROM lanes WHERE id = ?`, id).Scan(&l.ID, &l.Name, &l.Position)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("lane %d: %w", id, ErrNotFound)
	}
	if err != nil {
		return nil, fmt.Errorf("get lane %d: %w", id, err)
	}
	return &l, nil
}

// AddLane appends a lane after the existing ones.
func (s *Store) AddLane(name string) (*Lane, error) {
	res, err := s.db.Exec(`INSERT INTO lanes (name, position, created_at)
		SELECT ?, COALESCE(MAX(position), -1) + 1, ? FROM lanes`, name, now())
	if err != nil {
		return nil, fmt.Errorf("add lane %s: %w", name, err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf("add lane %s: %w", name, err)
	}
	return s.GetLane(id)
}

func (s *Store) RenameLane(id int64, name string) error {
	res, err := s.db.Exec(`UPDATE lanes SET name = ? WHERE id = ?`, name, id)
	if err != nil {
		return fmt.Errorf("rename lane %d: %w", id, err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("lane %d: %w", id, ErrNotFound)
	}
	return nil
}

// DeleteLane removes the lane, takes every job out of it and forgets its
// order; it returns the ids of the jobs it held.
func (s *Store) DeleteLane(id int64) ([]int64, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return nil, fmt.Errorf("delete lane %d: %w", id, err)
	}
	defer tx.Rollback()
	res, err := tx.Exec(`DELETE FROM lanes WHERE id = ?`, id)
	if err != nil {
		return nil, fmt.Errorf("delete lane %d: %w", id, err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return nil, fmt.Errorf("lane %d: %w", id, ErrNotFound)
	}
	rows, err := tx.Query(`SELECT id FROM jobs WHERE lane_id = ? ORDER BY id`, id)
	if err != nil {
		return nil, fmt.Errorf("delete lane %d: %w", id, err)
	}
	var held []int64
	for rows.Next() {
		var jobID int64
		if err := rows.Scan(&jobID); err != nil {
			rows.Close()
			return nil, fmt.Errorf("delete lane %d: %w", id, err)
		}
		held = append(held, jobID)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("delete lane %d: %w", id, err)
	}
	if _, err := tx.Exec(`UPDATE jobs SET lane_id = 0 WHERE lane_id = ?`, id); err != nil {
		return nil, fmt.Errorf("delete lane %d: %w", id, err)
	}
	if _, err := tx.Exec(`DELETE FROM board_order WHERE "column" = ?`, LaneColumn(id)); err != nil {
		return nil, fmt.Errorf("delete lane %d: %w", id, err)
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("delete lane %d: %w", id, err)
	}
	return held, nil
}
