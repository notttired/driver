package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"driver/internal/jobs"
	"driver/internal/store"
)

type planFixture struct {
	t   *testing.T
	mux *http.ServeMux
}

func newPlanFixture(t *testing.T) *planFixture {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "d.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	svc := &jobs.Service{Store: st, Cfg: jobs.Config{Home: t.TempDir()}, Tmux: jobs.NewTmux()}
	a := New(svc, jobs.NewDaemon(svc), "test")
	mux := http.NewServeMux()
	a.Register(mux)
	return &planFixture{t: t, mux: mux}
}

func (f *planFixture) do(method, path, body string, into any) int {
	f.t.Helper()
	rec := httptest.NewRecorder()
	f.mux.ServeHTTP(rec, httptest.NewRequest(method, path, strings.NewReader(body)))
	if into != nil {
		if err := json.Unmarshal(rec.Body.Bytes(), into); err != nil {
			f.t.Fatalf("%s %s: %s: %v", method, path, rec.Body.String(), err)
		}
	}
	return rec.Code
}

func (f *planFixture) post(path, body string) (int, map[string]any) {
	f.t.Helper()
	var out map[string]any
	code := f.do(http.MethodPost, path, body, &out)
	return code, out
}

func (f *planFixture) plan(ref string) webPlan {
	f.t.Helper()
	var p webPlan
	if code := f.do(http.MethodGet, "/api/plan?plan="+url.QueryEscape(ref), "", &p); code != http.StatusOK {
		f.t.Fatalf("get plan %s: %d", ref, code)
	}
	return p
}

func (f *planFixture) list() []webPlanSummary {
	f.t.Helper()
	var out struct {
		Plans []webPlanSummary `json:"plans"`
	}
	if code := f.do(http.MethodGet, "/api/plans", "", &out); code != http.StatusOK {
		f.t.Fatalf("list plans: %d", code)
	}
	return out.Plans
}

func (f *planFixture) addPlan(name string) int64 {
	f.t.Helper()
	code, out := f.post("/api/plans", `{"op":"add","name":`+jsonString(name)+`}`)
	if code != http.StatusOK {
		f.t.Fatalf("add plan %s: %d %v", name, code, out)
	}
	return int64(out["plan"].(map[string]any)["id"].(float64))
}

// ops posts a batch and returns the status, the plan after it and the ids
// of the steps it added, by their refs.
func (f *planFixture) ops(plan, ops string) (int, webPlan, map[string]int64, string) {
	f.t.Helper()
	var out struct {
		Plan  webPlan          `json:"plan"`
		Added map[string]int64 `json:"added"`
		Error string           `json:"error"`
	}
	code := f.do(http.MethodPost, "/api/plan/ops", `{"plan":`+plan+`,"ops":`+ops+`}`, &out)
	return code, out.Plan, out.Added, out.Error
}

func jsonString(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

func stepByTitle(t *testing.T, p webPlan, title string) webPlanStep {
	t.Helper()
	for _, s := range p.Steps {
		if s.Title == title {
			return s
		}
	}
	t.Fatalf("no step %q in %+v", title, p.Steps)
	return webPlanStep{}
}

func TestPlans(t *testing.T) {
	f := newPlanFixture(t)

	if got := f.list(); got == nil || len(got) != 0 {
		t.Fatalf("plans on a fresh daemon: %#v", got)
	}
	id := f.addPlan("  Monorail   sunset ")
	if got := f.list(); len(got) != 1 || got[0].ID != id || got[0].Name != "Monorail sunset" || got[0].Steps != 0 {
		t.Fatalf("one plan: %+v", got)
	}
	if code, out := f.post("/api/plans", `{"op":"add","name":"monorail SUNSET"}`); code != http.StatusConflict {
		t.Fatalf("a second plan under the same name: %d %v", code, out)
	}
	if code, out := f.post("/api/plans", `{"op":"add","name":"   "}`); code != http.StatusBadRequest {
		t.Fatalf("a nameless plan: %d %v", code, out)
	}
	if code, out := f.post("/api/plans", `{"op":"add","name":"`+strings.Repeat("x", 81)+`"}`); code != http.StatusBadRequest {
		t.Fatalf("a name too long: %d %v", code, out)
	}

	other := f.addPlan("entitiesByCountry removal")
	if code, out := f.post("/api/plans", `{"op":"rename","plan":"entitiesbycountry removal","name":"Field removal"}`); code != http.StatusOK {
		t.Fatalf("rename by name: %d %v", code, out)
	}
	if p := f.plan("Field removal"); p.ID != other || p.Name != "Field removal" {
		t.Fatalf("renamed plan: %+v", p)
	}
	if code, out := f.post("/api/plans", `{"op":"rename","plan":`+jsonInt(other)+`,"name":"Monorail sunset"}`); code != http.StatusConflict {
		t.Fatalf("rename onto another plan's name: %d %v", code, out)
	}
	if got := f.list(); len(got) != 2 || got[0].ID != id || got[1].ID != other {
		t.Fatalf("plans in the order they were made: %+v", got)
	}

	if code, _, _, msg := f.ops(jsonInt(other), `[{"op":"add","title":"World PR"}]`); code != http.StatusOK {
		t.Fatalf("a step on the plan: %d %s", code, msg)
	}
	if code, out := f.post("/api/plans", `{"op":"remove","plan":`+jsonInt(other)+`}`); code != http.StatusOK {
		t.Fatalf("remove: %d %v", code, out)
	}
	if code := f.do(http.MethodGet, "/api/plan?plan="+jsonInt(other), "", nil); code != http.StatusNotFound {
		t.Fatalf("a removed plan: %d", code)
	}
	if got := f.list(); len(got) != 1 || got[0].ID != id {
		t.Fatalf("after remove: %+v", got)
	}
	for _, body := range []string{`{"op":"remove","plan":99}`, `{"op":"rename","plan":"nope","name":"x"}`} {
		if code, out := f.post("/api/plans", body); code != http.StatusNotFound || out["error"] != "no such plan" {
			t.Fatalf("%s: %d %v", body, code, out)
		}
	}
	if code, _ := f.post("/api/plans", `{"op":"merge"}`); code != http.StatusBadRequest {
		t.Fatalf("unknown op: %d", code)
	}
	if code := f.do(http.MethodGet, "/api/plan?plan=nope", "", nil); code != http.StatusNotFound {
		t.Fatalf("unknown plan: %d", code)
	}
}

func TestPlanSteps(t *testing.T) {
	f := newPlanFixture(t)
	id := f.addPlan("Field removal")
	plan := jsonInt(id)

	code, p, added, msg := f.ops(plan, `[
		{"op":"add","ref":"a","title":"Abacus #7711","state":"done","kind":"PR","note":"merged"},
		{"op":"add","ref":"zero","title":"No calls to the field","state":"done","waits_on":["a"]},
		{"op":"add","ref":"freeze","title":"BFCM freeze","state":"wait","kind":"date","note":"until 1 Dec"},
		{"op":"add","ref":"pr","title":"World PR","waits_on":["zero"],"topic":"World"},
		{"op":"add","ref":"land","title":"Land the World PR","waits_on":["pr","freeze"],"topic":"World"}
	]`)
	if code != http.StatusOK {
		t.Fatalf("add steps: %d %s", code, msg)
	}
	if len(added) != 5 || added["a"] == 0 || added["land"] == 0 {
		t.Fatalf("added: %v", added)
	}
	if len(p.Steps) != 5 || p.ID != id {
		t.Fatalf("plan after adds: %+v", p)
	}
	zero := stepByTitle(t, p, "No calls to the field")
	if zero.State != "done" || len(zero.WaitsOn) != 1 || zero.WaitsOn[0] != added["a"] {
		t.Fatalf("waits on a step added earlier in the batch: %+v", zero)
	}
	if s := stepByTitle(t, p, "Abacus #7711"); s.Kind != "PR" || s.Note != "merged" || s.Ready {
		t.Fatalf("fields, and done is not ready: %+v", s)
	}
	if s := stepByTitle(t, p, "World PR"); !s.Ready || s.State != "todo" || s.Topic != "World" {
		t.Fatalf("todo with everything before it done is ready: %+v", s)
	}
	if s := stepByTitle(t, p, "Land the World PR"); s.Ready {
		t.Fatalf("waiting on a step not done is not ready: %+v", s)
	}
	rev := p.Rev
	if got := f.plan("field removal"); got.Rev != rev || len(got.Steps) != 5 {
		t.Fatalf("by name: %+v", got)
	}
	if got := f.list(); len(got) != 1 || got[0].Steps != 5 || got[0].Done != 2 || got[0].Ready != 1 || got[0].Waiting != 1 || got[0].Blocked != 0 {
		t.Fatalf("counts: %+v", got)
	}

	// update takes only what it names; link and unlink by id
	code, p, _, msg = f.ops(`"Field removal"`, `[
		{"op":"update","step":`+jsonInt(added["pr"])+`,"title":"World PR: remove the field","state":"blocked"},
		{"op":"add","ref":"tell","title":"Tell Logan about #7664","waits_on":[`+jsonInt(added["zero"])+`],"unblocks":[`+jsonInt(added["land"])+`]},
		{"op":"unlink","step":`+jsonInt(added["land"])+`,"waits_on":[`+jsonInt(added["freeze"])+`]}
	]`)
	if code != http.StatusOK {
		t.Fatalf("update, add with unblocks, unlink: %d %s", code, msg)
	}
	if p.Rev <= rev {
		t.Fatalf("rev moves with every change: %d → %d", rev, p.Rev)
	}
	if s := stepByTitle(t, p, "World PR: remove the field"); s.State != "blocked" || s.Topic != "World" || s.Ready {
		t.Fatalf("update keeps what it does not name: %+v", s)
	}
	land := stepByTitle(t, p, "Land the World PR")
	if !sameIDs(land.WaitsOn, added["pr"], stepByTitle(t, p, "Tell Logan about #7664").ID) {
		t.Fatalf("unblocks links the new step in; unlink drops the freeze: %+v", land)
	}

	// a batch is all or nothing
	before := f.plan(plan)
	code, _, _, msg = f.ops(plan, `[
		{"op":"update","step":`+jsonInt(added["zero"])+`,"title":"renamed"},
		{"op":"link","step":`+jsonInt(added["a"])+`,"waits_on":[`+jsonInt(added["land"])+`]}
	]`)
	if code != http.StatusConflict || !strings.Contains(msg, "loop") {
		t.Fatalf("a link that closes a loop: %d %s", code, msg)
	}
	if got := f.plan(plan); got.Rev != before.Rev || stepByTitle(t, got, "No calls to the field").Title == "renamed" {
		t.Fatalf("nothing of a refused batch is kept: %+v", got)
	}
	for _, c := range []struct {
		ops  string
		code int
	}{
		{`[{"op":"link","step":` + jsonInt(added["a"]) + `,"waits_on":[` + jsonInt(added["a"]) + `]}]`, http.StatusConflict},
		{`[{"op":"update","step":999,"title":"x"}]`, http.StatusNotFound},
		{`[{"op":"link","step":` + jsonInt(added["a"]) + `,"waits_on":["later"]}]`, http.StatusBadRequest},
		{`[{"op":"add","title":"  "}]`, http.StatusBadRequest},
		{`[{"op":"add","title":"x","state":"finished"}]`, http.StatusBadRequest},
		{`[{"op":"update","step":` + jsonInt(added["a"]) + `,"title":""}]`, http.StatusBadRequest},
		{`[{"op":"add","ref":"x","title":"x"},{"op":"add","ref":"x","title":"y"}]`, http.StatusBadRequest},
		{`[{"op":"rewrite"}]`, http.StatusBadRequest},
		{`[]`, http.StatusBadRequest},
	} {
		if code, _, _, msg := f.ops(plan, c.ops); code != c.code || msg == "" {
			t.Fatalf("%s: %d %q, want %d", c.ops, code, msg, c.code)
		}
	}
	if code, _, _, msg := f.ops(`"nope"`, `[{"op":"add","title":"x"}]`); code != http.StatusNotFound || msg != "no such plan" {
		t.Fatalf("ops on an unknown plan: %d %s", code, msg)
	}
	other := f.addPlan("Other")
	if code, _, _, msg := f.ops(jsonInt(other), `[{"op":"update","step":`+jsonInt(added["a"])+`,"title":"x"}]`); code != http.StatusNotFound {
		t.Fatalf("a step of another plan: %d %s", code, msg)
	}
}

func TestPlanDeleteReconnects(t *testing.T) {
	f := newPlanFixture(t)
	plan := jsonInt(f.addPlan("Chain"))
	code, _, added, msg := f.ops(plan, `[
		{"op":"add","ref":"a","title":"a"},
		{"op":"add","ref":"b","title":"b","waits_on":["a"]},
		{"op":"add","ref":"c","title":"c","waits_on":["b"]},
		{"op":"add","ref":"d","title":"d","waits_on":["b","a"]},
		{"op":"add","ref":"x","title":"x"},
		{"op":"add","ref":"e","title":"e","waits_on":["b","x"]}
	]`)
	if code != http.StatusOK {
		t.Fatalf("chain: %d %s", code, msg)
	}
	code, p, _, msg := f.ops(plan, `[{"op":"delete","step":`+jsonInt(added["b"])+`}]`)
	if code != http.StatusOK {
		t.Fatalf("delete: %d %s", code, msg)
	}
	if len(p.Steps) != 5 {
		t.Fatalf("one step fewer: %+v", p.Steps)
	}
	if s := stepByTitle(t, p, "c"); !sameIDs(s.WaitsOn, added["a"]) {
		t.Fatalf("the step after waits on what the deleted one waited on: %+v", s)
	}
	if s := stepByTitle(t, p, "d"); !sameIDs(s.WaitsOn, added["a"]) {
		t.Fatalf("no second link to what it already waited on: %+v", s)
	}
	if s := stepByTitle(t, p, "e"); !sameIDs(s.WaitsOn, added["x"], added["a"]) {
		t.Fatalf("its other links stay: %+v", s)
	}
	code, p, _, msg = f.ops(plan, `[{"op":"add","ref":"y","title":"y","waits_on":[`+jsonInt(added["x"])+`]},{"op":"add","title":"z","waits_on":["y",`+jsonInt(added["x"])+`]},{"op":"delete","step":"y"}]`)
	if code != http.StatusOK {
		t.Fatalf("delete a step added in the same batch: %d %s", code, msg)
	}
	if s := stepByTitle(t, p, "z"); !sameIDs(s.WaitsOn, added["x"]) {
		t.Fatalf("z: %+v", s)
	}
	if code, _, _, msg := f.ops(plan, `[{"op":"add","title":"w","waits_on":["x"]}]`); code != http.StatusBadRequest {
		t.Fatalf("a ref lives only in its own batch: %d %s", code, msg)
	}
}

func sameIDs(got []int64, want ...int64) bool {
	if len(got) != len(want) {
		return false
	}
	seen := map[int64]bool{}
	for _, id := range got {
		seen[id] = true
	}
	for _, id := range want {
		if !seen[id] {
			return false
		}
	}
	return true
}
