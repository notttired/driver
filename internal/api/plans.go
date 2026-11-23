package api

import (
	"bytes"
	"encoding/json"
	"log"
	"net/http"
	"strings"

	"driver/internal/jobs"
	"driver/internal/store"
)

// webPlanSummary is a plan in the list: its name and how its steps stand.
type webPlanSummary struct {
	ID      int64  `json:"id"`
	Name    string `json:"name"`
	Rev     int64  `json:"rev"`
	Steps   int    `json:"steps"`
	Done    int    `json:"done"`
	Ready   int    `json:"ready"`
	Waiting int    `json:"waiting"`
	Blocked int    `json:"blocked"`
}

type webPlanStep struct {
	ID      int64   `json:"id"`
	Title   string  `json:"title"`
	Note    string  `json:"note"`
	State   string  `json:"state"`
	Ready   bool    `json:"ready"`
	Kind    string  `json:"kind"`
	Topic   string  `json:"topic"`
	WaitsOn []int64 `json:"waits_on"`
}

type webPlan struct {
	ID    int64         `json:"id"`
	Name  string        `json:"name"`
	Rev   int64         `json:"rev"`
	Steps []webPlanStep `json:"steps"`
}

// ref is a plan or a step as the wire names it: a number (its id) or a
// string (an id, a plan's name, or a step's ref within a batch).
type ref string

func (r *ref) UnmarshalJSON(b []byte) error {
	b = bytes.TrimSpace(b)
	if len(b) > 0 && b[0] == '"' {
		var s string
		if err := json.Unmarshal(b, &s); err != nil {
			return err
		}
		*r = ref(s)
		return nil
	}
	var n json.Number
	if err := json.Unmarshal(b, &n); err != nil {
		return err
	}
	*r = ref(n.String())
	return nil
}

func refs(rs []ref) []string {
	out := make([]string, len(rs))
	for i, r := range rs {
		out[i] = string(r)
	}
	return out
}

type webPlanOp struct {
	Op       string  `json:"op"`
	Ref      string  `json:"ref"`
	Step     ref     `json:"step"`
	Title    *string `json:"title"`
	Note     *string `json:"note"`
	State    *string `json:"state"`
	Kind     *string `json:"kind"`
	Topic    *string `json:"topic"`
	WaitsOn  []ref   `json:"waits_on"`
	Unblocks []ref   `json:"unblocks"`
}

func (a *API) planPayload(p *store.Plan) (webPlan, error) {
	steps, err := a.Svc.Store.PlanSteps(p.ID)
	if err != nil {
		return webPlan{}, err
	}
	byID := make(map[int64]*store.PlanStep, len(steps))
	for _, st := range steps {
		byID[st.ID] = st
	}
	out := webPlan{ID: p.ID, Name: p.Name, Rev: p.Rev, Steps: make([]webPlanStep, 0, len(steps))}
	for _, st := range steps {
		out.Steps = append(out.Steps, webPlanStep{
			ID: st.ID, Title: st.Title, Note: st.Note, State: st.State, Ready: jobs.StepReady(st, byID),
			Kind: st.Kind, Topic: st.Topic, WaitsOn: st.WaitsOn,
		})
	}
	return out, nil
}

func summarize(p webPlan) webPlanSummary {
	s := webPlanSummary{ID: p.ID, Name: p.Name, Rev: p.Rev, Steps: len(p.Steps)}
	for _, st := range p.Steps {
		switch {
		case st.Ready:
			s.Ready++
		case st.State == "done":
			s.Done++
		case st.State == "wait":
			s.Waiting++
		case st.State == "blocked":
			s.Blocked++
		}
	}
	return s
}

// writePlanFailure is writeFailure with the plan's own words for a miss: a
// plan that is not there, or a step a batch names that is not.
func writePlanFailure(w http.ResponseWriter, what string, err error) {
	if isNotFound(err) {
		msg := "no such plan"
		if strings.HasPrefix(err.Error(), "op ") {
			msg = strings.TrimSuffix(err.Error(), ": "+store.ErrNotFound.Error())
		}
		writeError(w, http.StatusNotFound, msg)
		return
	}
	writeFailure(w, what, err)
}

// plans is GET: every plan with its counts; POST: add, rename or remove one.
func (a *API) plans(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		ps, err := a.Svc.Store.ListPlans()
		if err != nil {
			writeFailure(w, "plans", err)
			return
		}
		out := make([]webPlanSummary, 0, len(ps))
		for _, p := range ps {
			full, err := a.planPayload(p)
			if err != nil {
				writeFailure(w, "plans", err)
				return
			}
			out = append(out, summarize(full))
		}
		writeJSON(w, http.StatusOK, map[string]any{"plans": out})
		return
	}
	var req struct {
		Op   string `json:"op"`
		Plan ref    `json:"plan"`
		Name string `json:"name"`
	}
	if !decodePost(w, r, &req) {
		return
	}
	if req.Op == "add" {
		p, err := a.Svc.AddPlan(req.Name)
		if err != nil {
			writePlanFailure(w, "plans", err)
			return
		}
		log.Printf("plan %d: added as %q", p.ID, p.Name)
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "plan": webPlanSummary{ID: p.ID, Name: p.Name, Rev: p.Rev}})
		return
	}
	if req.Op != "rename" && req.Op != "remove" {
		writeError(w, http.StatusBadRequest, "op must be add, rename or remove")
		return
	}
	p, err := a.Svc.LookupPlan(string(req.Plan))
	if err != nil {
		writePlanFailure(w, "plans", err)
		return
	}
	if req.Op == "rename" {
		err = a.Svc.RenamePlan(p, req.Name)
	} else {
		err = a.Svc.DeletePlan(p)
		if err == nil {
			log.Printf("plan %d: removed (%q)", p.ID, p.Name)
		}
	}
	if err != nil {
		writePlanFailure(w, "plans", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// plan is GET ?plan=<id or name>: the plan with every step.
func (a *API) plan(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "GET only")
		return
	}
	p, err := a.Svc.LookupPlan(r.URL.Query().Get("plan"))
	if err != nil {
		writePlanFailure(w, "plan", err)
		return
	}
	out, err := a.planPayload(p)
	if err != nil {
		writeFailure(w, "plan", err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

// planOps applies a batch of step changes, all or none, and answers with
// the plan as it now stands and the ids of the steps the batch added.
func (a *API) planOps(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Plan ref         `json:"plan"`
		Ops  []webPlanOp `json:"ops"`
	}
	if !decodePost(w, r, &req) {
		return
	}
	p, err := a.Svc.LookupPlan(string(req.Plan))
	if err != nil {
		writePlanFailure(w, "plan ops", err)
		return
	}
	ops := make([]store.PlanOp, len(req.Ops))
	for i, o := range req.Ops {
		ops[i] = store.PlanOp{
			Op: o.Op, Ref: o.Ref, Step: string(o.Step), Title: o.Title, Note: o.Note, State: o.State,
			Kind: o.Kind, Topic: o.Topic, WaitsOn: refs(o.WaitsOn), Unblocks: refs(o.Unblocks),
		}
	}
	added, err := a.Svc.ApplyPlanOps(p, ops)
	if err != nil {
		writePlanFailure(w, "plan ops", err)
		return
	}
	p, err = a.Svc.Store.GetPlan(p.ID)
	if err == nil {
		var out webPlan
		if out, err = a.planPayload(p); err == nil {
			if added == nil {
				added = map[string]int64{}
			}
			writeJSON(w, http.StatusOK, map[string]any{"ok": true, "plan": out, "added": added})
			return
		}
	}
	writeFailure(w, "plan ops", err)
}
