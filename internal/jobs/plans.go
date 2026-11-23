package jobs

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"

	"driver/internal/store"
)

// Plans: graphs of steps for long pieces of work (docs/DESIGN.md "Plans").
// They name no job and no session; the board and any pi on it edit them
// through the same calls.

// Bounds on what a plan holds, in runes.
const (
	MaxPlanNameLen  = 80
	MaxStepTitleLen = 200
	MaxStepNoteLen  = 4000
	MaxStepKindLen  = 24
	MaxStepTopicLen = 80
	MaxPlanOps      = 500
)

// StepStates are the states a step is stored in; "ready" is worked out (a
// todo step whose every prerequisite is done), never stored.
var StepStates = []string{"todo", "wait", "blocked", "done"}

func cleanLine(s string) string { return strings.Join(strings.Fields(s), " ") }

// CleanPlanName collapses whitespace; empty or longer than MaxPlanNameLen is
// ErrBadInput.
func CleanPlanName(s string) (string, error) {
	s = cleanLine(s)
	if s == "" {
		return "", fmt.Errorf("a plan needs a name: %w", ErrBadInput)
	}
	if utf8.RuneCountInString(s) > MaxPlanNameLen {
		return "", fmt.Errorf("plan name is too long (max %d characters): %w", MaxPlanNameLen, ErrBadInput)
	}
	return s, nil
}

// LookupPlan finds a plan by id ("12") or by name, without regard to case.
func (s *Service) LookupPlan(ref string) (*store.Plan, error) {
	ref = strings.TrimSpace(ref)
	if id, err := strconv.ParseInt(ref, 10, 64); err == nil {
		return s.Store.GetPlan(id)
	}
	if ref == "" {
		return nil, fmt.Errorf("no plan named: %w", store.ErrNotFound)
	}
	return s.Store.FindPlan(cleanLine(ref))
}

func mapPlanErr(name string, err error) error {
	if errors.Is(err, store.ErrDuplicate) {
		return Refuse("a plan named %q already exists", name)
	}
	return err
}

func (s *Service) AddPlan(name string) (*store.Plan, error) {
	name, err := CleanPlanName(name)
	if err != nil {
		return nil, err
	}
	p, err := s.Store.AddPlan(name)
	return p, mapPlanErr(name, err)
}

func (s *Service) RenamePlan(p *store.Plan, name string) error {
	name, err := CleanPlanName(name)
	if err != nil {
		return err
	}
	if name == p.Name {
		return nil
	}
	return mapPlanErr(name, s.Store.RenamePlan(p.ID, name))
}

func (s *Service) DeletePlan(p *store.Plan) error { return s.Store.DeletePlan(p.ID) }

// ApplyPlanOps cleans every op of the batch, then has the store make them
// all or none. Titles, notes, kinds and topics are trimmed (a note keeps its
// line breaks); a step must keep a title.
func (s *Service) ApplyPlanOps(p *store.Plan, ops []store.PlanOp) (map[string]int64, error) {
	if len(ops) == 0 {
		return nil, fmt.Errorf("no ops: %w", ErrBadInput)
	}
	if len(ops) > MaxPlanOps {
		return nil, fmt.Errorf("too many ops in one batch (max %d): %w", MaxPlanOps, ErrBadInput)
	}
	for i := range ops {
		if err := cleanPlanOp(&ops[i]); err != nil {
			return nil, fmt.Errorf("op %d: %w", i+1, err)
		}
	}
	added, err := s.Store.ApplyPlanOps(p.ID, ops)
	switch {
	case errors.Is(err, store.ErrLoop):
		return nil, Refuse("%s", strings.TrimSuffix(err.Error(), ": "+store.ErrLoop.Error()))
	case errors.Is(err, store.ErrBadPlanOp):
		return nil, fmt.Errorf("%s: %w", strings.TrimSuffix(err.Error(), ": "+store.ErrBadPlanOp.Error()), ErrBadInput)
	}
	return added, err
}

func cleanPlanOp(op *store.PlanOp) error {
	op.Ref = strings.TrimSpace(op.Ref)
	field := func(p **string, what string, max int, line bool, required bool) error {
		if *p == nil {
			if required {
				return fmt.Errorf("a step needs a title: %w", ErrBadInput)
			}
			return nil
		}
		v := strings.TrimSpace(**p)
		if line {
			v = cleanLine(v)
		}
		if required && v == "" {
			return fmt.Errorf("a step needs a title: %w", ErrBadInput)
		}
		if utf8.RuneCountInString(v) > max {
			return fmt.Errorf("%s is too long (max %d characters): %w", what, max, ErrBadInput)
		}
		*p = &v
		return nil
	}
	isAdd := op.Op == "add"
	if err := field(&op.Title, "title", MaxStepTitleLen, true, isAdd || op.Title != nil); err != nil {
		return err
	}
	if err := field(&op.Note, "note", MaxStepNoteLen, false, false); err != nil {
		return err
	}
	if err := field(&op.Kind, "kind", MaxStepKindLen, true, false); err != nil {
		return err
	}
	if err := field(&op.Topic, "topic", MaxStepTopicLen, true, false); err != nil {
		return err
	}
	if op.State != nil {
		st := strings.ToLower(strings.TrimSpace(*op.State))
		if st == "waiting" {
			st = "wait"
		}
		ok := false
		for _, s := range StepStates {
			ok = ok || s == st
		}
		if !ok {
			return fmt.Errorf("unknown state %q (todo, wait, blocked or done): %w", *op.State, ErrBadInput)
		}
		op.State = &st
	}
	return nil
}

// StepReady reports whether a step can be started now: it is todo and every
// step it waits on is done.
func StepReady(st *store.PlanStep, byID map[int64]*store.PlanStep) bool {
	if st.State != "todo" {
		return false
	}
	for _, on := range st.WaitsOn {
		if w := byID[on]; w == nil || w.State != "done" {
			return false
		}
	}
	return true
}
