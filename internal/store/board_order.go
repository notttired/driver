package store

import (
	"fmt"
	"sort"
)

// GetBoardOrder returns every column's job-id order, keyed by column name and
// sorted by position within each column.
func (s *Store) GetBoardOrder() (map[string][]int64, error) {
	rows, err := s.db.Query(`SELECT "column", job_id FROM board_order ORDER BY "column", position`)
	if err != nil {
		return nil, fmt.Errorf("get board order: %w", err)
	}
	defer rows.Close()
	out := map[string][]int64{}
	for rows.Next() {
		var col string
		var id int64
		if err := rows.Scan(&col, &id); err != nil {
			return nil, fmt.Errorf("get board order: %w", err)
		}
		out[col] = append(out[col], id)
	}
	return out, rows.Err()
}

// SetBoardColumn atomically replaces one column's job-id order (the reorder
// primitive drag-and-drop drives). ids are deduplicated (first wins, zero
// skipped) and each surviving id is removed from every other column in the
// same transaction, so a job always lives in exactly one column.
func (s *Store) SetBoardColumn(column string, ids []int64) error {
	seen := make(map[int64]bool, len(ids))
	deduped := make([]int64, 0, len(ids))
	for _, id := range ids {
		if id == 0 || seen[id] {
			continue
		}
		seen[id] = true
		deduped = append(deduped, id)
	}
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("set board column %s: %w", column, err)
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`DELETE FROM board_order WHERE "column" = ?`, column); err != nil {
		return fmt.Errorf("set board column %s: %w", column, err)
	}
	for i, id := range deduped {
		if _, err := tx.Exec(`DELETE FROM board_order WHERE "column" != ? AND job_id = ?`, column, id); err != nil {
			return fmt.Errorf("set board column %s: %w", column, err)
		}
		if _, err := tx.Exec(`INSERT INTO board_order ("column", position, job_id) VALUES (?, ?, ?)`, column, i, id); err != nil {
			return fmt.Errorf("set board column %s: %w", column, err)
		}
	}
	return tx.Commit()
}

// ApplyColumnOrder orders members (the ids of the jobs whose display state
// is this column) by the stored order: ids the stored order does not list
// come first, in the order given (callers pass them alphabetically by slug),
// then the listed ones in stored order. Stored ids not in members are
// ignored — stale rows never add phantom cards.
func ApplyColumnOrder(members []int64, stored []int64) []int64 {
	position := make(map[int64]int, len(stored))
	for i, id := range stored {
		if _, ok := position[id]; !ok {
			position[id] = i
		}
	}
	var unlisted, listed []int64
	for _, m := range members {
		if _, ok := position[m]; ok {
			listed = append(listed, m)
		} else {
			unlisted = append(unlisted, m)
		}
	}
	sort.SliceStable(listed, func(i, k int) bool { return position[listed[i]] < position[listed[k]] })
	out := make([]int64, 0, len(members))
	out = append(out, unlisted...)
	return append(out, listed...)
}
