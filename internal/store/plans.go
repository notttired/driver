package store

import (
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// Plans are graphs of steps for long pieces of work: each step may wait on
// others. They stand apart from the board — no job, thread or tmux session
// is named here, and nothing on a job touches them.

// ErrLoop is returned when a link would make a step wait on itself.
var ErrLoop = errors.New("loop")

// ErrBadPlanOp is returned for a batch op that cannot be read: an unknown
// op, a ref used before (or without) the add that names it, a ref named
// twice.
var ErrBadPlanOp = errors.New("bad plan op")

// Plan is one plan; Rev moves with every change to it or its steps.
type Plan struct {
	ID        int64
	Name      string
	Rev       int64
	CreatedAt int64
	UpdatedAt int64
}

// PlanStep is one step of a plan, with the ids of the steps it waits on.
type PlanStep struct {
	ID      int64
	PlanID  int64
	Title   string
	Note    string
	State   string // todo | wait | blocked | done
	Kind    string
	Topic   string
	WaitsOn []int64
}

// PlanOp is one change in a batch ApplyPlanOps makes whole or not at all.
// Steps are named by id ("12") or by the Ref an earlier add in the same
// batch gave its step. Field pointers left nil are left alone.
type PlanOp struct {
	Op       string // add | update | link | unlink | delete
	Ref      string
	Step     string
	Title    *string
	Note     *string
	State    *string
	Kind     *string
	Topic    *string
	WaitsOn  []string
	Unblocks []string
}

const planCols = `id, name, rev, created_at, updated_at`

func scanPlan(row interface{ Scan(...any) error }) (*Plan, error) {
	var p Plan
	if err := row.Scan(&p.ID, &p.Name, &p.Rev, &p.CreatedAt, &p.UpdatedAt); err != nil {
		return nil, err
	}
	return &p, nil
}

func (s *Store) ListPlans() ([]*Plan, error) {
	rows, err := s.db.Query(`SELECT ` + planCols + ` FROM plans ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("list plans: %w", err)
	}
	defer rows.Close()
	out := []*Plan{}
	for rows.Next() {
		p, err := scanPlan(rows)
		if err != nil {
			return nil, fmt.Errorf("list plans: %w", err)
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (s *Store) GetPlan(id int64) (*Plan, error) {
	p, err := scanPlan(s.db.QueryRow(`SELECT `+planCols+` FROM plans WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("plan %d: %w", id, ErrNotFound)
	}
	if err != nil {
		return nil, fmt.Errorf("get plan %d: %w", id, err)
	}
	return p, nil
}

// FindPlan is the plan under name, matched without regard to case.
func (s *Store) FindPlan(name string) (*Plan, error) {
	p, err := scanPlan(s.db.QueryRow(`SELECT `+planCols+` FROM plans WHERE name = ? COLLATE NOCASE`, name))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("plan %q: %w", name, ErrNotFound)
	}
	if err != nil {
		return nil, fmt.Errorf("find plan %q: %w", name, err)
	}
	return p, nil
}

// AddPlan makes an empty plan; a name another plan holds is ErrDuplicate.
func (s *Store) AddPlan(name string) (*Plan, error) {
	t := now()
	res, err := s.db.Exec(`INSERT INTO plans (name, created_at, updated_at) VALUES (?, ?, ?)`, name, t, t)
	if isUniqueErr(err) {
		return nil, fmt.Errorf("plan %q: %w", name, ErrDuplicate)
	}
	if err != nil {
		return nil, fmt.Errorf("add plan %q: %w", name, err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf("add plan %q: %w", name, err)
	}
	return s.GetPlan(id)
}

func (s *Store) RenamePlan(id int64, name string) error {
	res, err := s.db.Exec(`UPDATE plans SET name = ?, rev = rev + 1, updated_at = ? WHERE id = ?`, name, now(), id)
	if isUniqueErr(err) {
		return fmt.Errorf("plan %q: %w", name, ErrDuplicate)
	}
	if err != nil {
		return fmt.Errorf("rename plan %d: %w", id, err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("plan %d: %w", id, ErrNotFound)
	}
	return nil
}

// DeletePlan removes the plan with its steps and their links.
func (s *Store) DeletePlan(id int64) error {
	res, err := s.db.Exec(`DELETE FROM plans WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete plan %d: %w", id, err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("plan %d: %w", id, ErrNotFound)
	}
	return nil
}

// PlanSteps is every step of the plan in the order they were added, each
// with the ids of the steps it waits on.
func (s *Store) PlanSteps(planID int64) ([]*PlanStep, error) {
	return planSteps(s.db, planID)
}

type querier interface {
	Query(query string, args ...any) (*sql.Rows, error)
}

func planSteps(q querier, planID int64) ([]*PlanStep, error) {
	rows, err := q.Query(`SELECT id, plan_id, title, note, state, kind, topic FROM plan_steps WHERE plan_id = ? ORDER BY id`, planID)
	if err != nil {
		return nil, fmt.Errorf("plan %d steps: %w", planID, err)
	}
	out := []*PlanStep{}
	byID := map[int64]*PlanStep{}
	for rows.Next() {
		st := &PlanStep{WaitsOn: []int64{}}
		if err := rows.Scan(&st.ID, &st.PlanID, &st.Title, &st.Note, &st.State, &st.Kind, &st.Topic); err != nil {
			rows.Close()
			return nil, fmt.Errorf("plan %d steps: %w", planID, err)
		}
		out = append(out, st)
		byID[st.ID] = st
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("plan %d steps: %w", planID, err)
	}
	links, err := q.Query(`SELECT l.step_id, l.waits_on FROM plan_links l JOIN plan_steps s ON s.id = l.step_id
		WHERE s.plan_id = ? ORDER BY l.rowid`, planID)
	if err != nil {
		return nil, fmt.Errorf("plan %d links: %w", planID, err)
	}
	defer links.Close()
	for links.Next() {
		var step, on int64
		if err := links.Scan(&step, &on); err != nil {
			return nil, fmt.Errorf("plan %d links: %w", planID, err)
		}
		if st := byID[step]; st != nil {
			st.WaitsOn = append(st.WaitsOn, on)
		}
	}
	return out, links.Err()
}

// ApplyPlanOps makes every change of the batch in one transaction, or none:
// the first op that cannot be made rolls the whole batch back, and the error
// names it ("op 3: …"). It returns the ids of the steps the batch added, by
// their refs. Deleting a step hands its links on: each step that waited on
// it now waits on what it waited on, unless it already waits on that
// through another step.
func (s *Store) ApplyPlanOps(planID int64, ops []PlanOp) (map[string]int64, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return nil, fmt.Errorf("plan %d ops: %w", planID, err)
	}
	defer tx.Rollback()
	var exists int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM plans WHERE id = ?`, planID).Scan(&exists); err != nil {
		return nil, fmt.Errorf("plan %d ops: %w", planID, err)
	}
	if exists == 0 {
		return nil, fmt.Errorf("plan %d: %w", planID, ErrNotFound)
	}
	steps, err := planSteps(tx, planID)
	if err != nil {
		return nil, err
	}
	g := &planGraph{tx: tx, plan: planID, waits: map[int64][]int64{}, refs: map[string]int64{}}
	for _, st := range steps {
		g.waits[st.ID] = append([]int64{}, st.WaitsOn...)
	}
	t := now()
	for i, op := range ops {
		if err := g.apply(op, t); err != nil {
			return nil, fmt.Errorf("op %d: %w", i+1, err)
		}
	}
	if _, err := tx.Exec(`UPDATE plans SET rev = rev + 1, updated_at = ? WHERE id = ?`, t, planID); err != nil {
		return nil, fmt.Errorf("plan %d ops: %w", planID, err)
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("plan %d ops: %w", planID, err)
	}
	return g.refs, nil
}

// planGraph is the plan's links as the batch goes, kept in step with the
// rows it writes so every check sees the ops before it.
type planGraph struct {
	tx    *sql.Tx
	plan  int64
	waits map[int64][]int64 // step → the steps it waits on
	refs  map[string]int64
}

func (g *planGraph) resolve(ref string) (int64, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return 0, fmt.Errorf("a step is missing: %w", ErrBadPlanOp)
	}
	if id, err := strconv.ParseInt(ref, 10, 64); err == nil {
		if _, ok := g.waits[id]; !ok {
			return 0, fmt.Errorf("no step %d in this plan: %w", id, ErrNotFound)
		}
		return id, nil
	}
	id, ok := g.refs[ref]
	if !ok {
		return 0, fmt.Errorf("no step added as %q before this: %w", ref, ErrBadPlanOp)
	}
	if _, live := g.waits[id]; !live {
		return 0, fmt.Errorf("step %q was deleted: %w", ref, ErrNotFound)
	}
	return id, nil
}

func (g *planGraph) resolveAll(refs []string) ([]int64, error) {
	out := make([]int64, 0, len(refs))
	for _, r := range refs {
		id, err := g.resolve(r)
		if err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, nil
}

// reaches reports whether from waits on to, directly or through others.
func (g *planGraph) reaches(from, to int64) bool {
	seen := map[int64]bool{}
	stack := []int64{from}
	for len(stack) > 0 {
		n := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		for _, w := range g.waits[n] {
			if w == to {
				return true
			}
			if !seen[w] {
				seen[w] = true
				stack = append(stack, w)
			}
		}
	}
	return false
}

func (g *planGraph) waitsOn(step, on int64) bool {
	for _, w := range g.waits[step] {
		if w == on {
			return true
		}
	}
	return false
}

func (g *planGraph) link(step, on int64) error {
	if step == on || g.reaches(on, step) {
		return fmt.Errorf("step %d waiting on %d would make a loop: %w", step, on, ErrLoop)
	}
	if g.waitsOn(step, on) {
		return nil
	}
	if _, err := g.tx.Exec(`INSERT INTO plan_links (step_id, waits_on) VALUES (?, ?)`, step, on); err != nil {
		return fmt.Errorf("link %d to %d: %w", step, on, err)
	}
	g.waits[step] = append(g.waits[step], on)
	return nil
}

func (g *planGraph) unlink(step, on int64) error {
	if _, err := g.tx.Exec(`DELETE FROM plan_links WHERE step_id = ? AND waits_on = ?`, step, on); err != nil {
		return fmt.Errorf("unlink %d from %d: %w", step, on, err)
	}
	kept := g.waits[step][:0]
	for _, w := range g.waits[step] {
		if w != on {
			kept = append(kept, w)
		}
	}
	g.waits[step] = kept
	return nil
}

func (g *planGraph) apply(op PlanOp, t int64) error {
	switch op.Op {
	case "add":
		return g.add(op, t)
	case "update":
		id, err := g.resolve(op.Step)
		if err != nil {
			return err
		}
		sets, args := []string{}, []any{}
		for _, f := range []struct {
			col string
			v   *string
		}{{"title", op.Title}, {"note", op.Note}, {"state", op.State}, {"kind", op.Kind}, {"topic", op.Topic}} {
			if f.v != nil {
				sets = append(sets, f.col+" = ?")
				args = append(args, *f.v)
			}
		}
		if len(sets) == 0 {
			return nil
		}
		args = append(args, t, id)
		if _, err := g.tx.Exec(`UPDATE plan_steps SET `+strings.Join(sets, ", ")+`, updated_at = ? WHERE id = ?`, args...); err != nil {
			return fmt.Errorf("update step %d: %w", id, err)
		}
		return nil
	case "link", "unlink":
		id, err := g.resolve(op.Step)
		if err != nil {
			return err
		}
		ons, err := g.resolveAll(op.WaitsOn)
		if err != nil {
			return err
		}
		if len(ons) == 0 {
			return fmt.Errorf("%s names no step to wait on: %w", op.Op, ErrBadPlanOp)
		}
		for _, on := range ons {
			if op.Op == "link" {
				err = g.link(id, on)
			} else {
				err = g.unlink(id, on)
			}
			if err != nil {
				return err
			}
		}
		return nil
	case "delete":
		id, err := g.resolve(op.Step)
		if err != nil {
			return err
		}
		return g.delete(id)
	}
	return fmt.Errorf("unknown op %q (add, update, link, unlink or delete): %w", op.Op, ErrBadPlanOp)
}

func (g *planGraph) add(op PlanOp, t int64) error {
	if op.Ref != "" {
		if _, err := strconv.ParseInt(op.Ref, 10, 64); err == nil {
			return fmt.Errorf("ref %q looks like a step id; name it with letters: %w", op.Ref, ErrBadPlanOp)
		}
		if _, taken := g.refs[op.Ref]; taken {
			return fmt.Errorf("ref %q names two steps: %w", op.Ref, ErrBadPlanOp)
		}
	}
	ons, err := g.resolveAll(op.WaitsOn)
	if err != nil {
		return err
	}
	afters, err := g.resolveAll(op.Unblocks)
	if err != nil {
		return err
	}
	val := func(p *string, def string) string {
		if p == nil {
			return def
		}
		return *p
	}
	res, err := g.tx.Exec(`INSERT INTO plan_steps (plan_id, title, note, state, kind, topic, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`, g.plan, val(op.Title, ""), val(op.Note, ""), val(op.State, "todo"), val(op.Kind, ""), val(op.Topic, ""), t, t)
	if err != nil {
		return fmt.Errorf("add step: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return fmt.Errorf("add step: %w", err)
	}
	g.waits[id] = []int64{}
	if op.Ref != "" {
		g.refs[op.Ref] = id
	}
	for _, on := range ons {
		if err := g.link(id, on); err != nil {
			return err
		}
	}
	for _, after := range afters {
		if err := g.link(after, id); err != nil {
			return err
		}
	}
	return nil
}

func (g *planGraph) delete(id int64) error {
	inherited := g.waits[id]
	var after []int64
	for step, ons := range g.waits {
		for _, on := range ons {
			if on == id {
				after = append(after, step)
				break
			}
		}
	}
	sort.Slice(after, func(i, j int) bool { return after[i] < after[j] })
	if _, err := g.tx.Exec(`DELETE FROM plan_steps WHERE id = ?`, id); err != nil {
		return fmt.Errorf("delete step %d: %w", id, err)
	}
	if _, err := g.tx.Exec(`DELETE FROM plan_links WHERE step_id = ? OR waits_on = ?`, id, id); err != nil {
		return fmt.Errorf("delete step %d: %w", id, err)
	}
	delete(g.waits, id)
	for _, step := range after {
		kept := g.waits[step][:0]
		for _, on := range g.waits[step] {
			if on != id {
				kept = append(kept, on)
			}
		}
		g.waits[step] = kept
	}
	for _, step := range after {
		for _, on := range inherited {
			if g.reaches(step, on) {
				continue
			}
			if _, err := g.tx.Exec(`INSERT INTO plan_links (step_id, waits_on) VALUES (?, ?)`, step, on); err != nil {
				return fmt.Errorf("hand on step %d's links: %w", id, err)
			}
			g.waits[step] = append(g.waits[step], on)
		}
	}
	return nil
}
