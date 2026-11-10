package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"driver/internal/jobs"
	"driver/internal/store"
)

func TestLanes(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "d.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	work := t.TempDir()
	svc := &jobs.Service{Store: st, Cfg: jobs.Config{Home: t.TempDir()}, Tmux: jobs.NewTmux()}
	a := New(svc, jobs.NewDaemon(svc), "test")
	mux := http.NewServeMux()
	a.Register(mux)

	post := func(path, body string) (int, map[string]any) {
		t.Helper()
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, path, strings.NewReader(body)))
		var out map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
			t.Fatalf("%s: %s: %v", path, rec.Body.String(), err)
		}
		return rec.Code, out
	}
	board := func() webBoard {
		t.Helper()
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/board", nil))
		var b webBoard
		if err := json.Unmarshal(rec.Body.Bytes(), &b); err != nil {
			t.Fatalf("%s: %v", rec.Body.String(), err)
		}
		return b
	}
	put := func(slug, state string) {
		t.Helper()
		j, err := svc.Create(slug, "", work, "")
		if err != nil {
			t.Fatal(err)
		}
		if state != "" {
			if _, err := st.UpdateJobFields(j.ID, j.Rev, map[string]any{"state": state}); err != nil {
				t.Fatal(err)
			}
		}
	}
	put("alpha", "working")
	put("beta", "blocked")
	put("gamma", "")

	if b := board(); b.Lanes == nil || len(b.Lanes) != 0 {
		t.Fatalf("lanes on a fresh board: %#v", b.Lanes)
	}

	code, out := post("/api/lanes", `{"op":"add","name":"  later   on "}`)
	if code != http.StatusOK {
		t.Fatalf("add lane: %d %v", code, out)
	}
	lane := out["lane"].(map[string]any)
	id := int64(lane["id"].(float64))
	if id == 0 || lane["name"] != "later on" {
		t.Fatalf("add lane: %v", out)
	}
	if code, out := post("/api/lanes", `{"op":"add","name":"   "}`); code != http.StatusBadRequest {
		t.Fatalf("add a nameless lane: %d %v", code, out)
	}
	if code, out := post("/api/lanes", `{"op":"add","name":"`+strings.Repeat("x", 41)+`"}`); code != http.StatusBadRequest {
		t.Fatalf("add a lane with a long name: %d %v", code, out)
	}
	code, out = post("/api/lanes", `{"op":"add","name":"review"}`)
	review := int64(out["lane"].(map[string]any)["id"].(float64))
	if code != http.StatusOK || review == id {
		t.Fatalf("add second lane: %d %v", code, out)
	}

	b := board()
	if len(b.Lanes) != 2 || b.Lanes[0].ID != id || b.Lanes[0].Name != "later on" || b.Lanes[1].Name != "review" {
		t.Fatalf("lanes: %+v", b.Lanes)
	}

	laneArg := func(n int64) string { return `"lane":` + jsonInt(n) }
	for _, slug := range []string{"beta", "alpha"} {
		if code, out := post("/api/job/lane", `{"slug":"`+slug+`",`+laneArg(id)+`}`); code != http.StatusOK {
			t.Fatalf("park %s: %d %v", slug, code, out)
		}
	}
	if code, out := post("/api/job/lane", `{"slug":"gamma",`+laneArg(99)+`}`); code != http.StatusNotFound {
		t.Fatalf("park in a missing lane: %d %v", code, out)
	}
	if code, out := post("/api/job/lane", `{"slug":"nope",`+laneArg(id)+`}`); code != http.StatusNotFound {
		t.Fatalf("park a missing job: %d %v", code, out)
	}

	slugsOf := func(b webBoard, lane int64) []string {
		var out []string
		for _, j := range b.Jobs {
			if j.Lane == lane {
				out = append(out, j.Slug)
			}
		}
		return out
	}
	b = board()
	if got := strings.Join(slugsOf(b, id), " "); got != "beta alpha" {
		t.Fatalf("parked, in arrival order: %q", got)
	}
	for _, j := range b.Jobs {
		want := map[string]string{"alpha": "working", "beta": "blocked", "gamma": "draft"}[j.Slug]
		if j.State != want {
			t.Fatalf("%s: state %s (a lane does not change the state)", j.Slug, j.State)
		}
	}

	if code, out := post("/api/reorder", `{"column":"lane:`+jsonInt(id)+`","slugs":["alpha","beta","gamma"]}`); code != http.StatusOK {
		t.Fatalf("reorder lane: %d %v", code, out)
	}
	if got := strings.Join(slugsOf(board(), id), " "); got != "alpha beta" {
		t.Fatalf("reordered (a job elsewhere is dropped): %q", got)
	}
	if code, out := post("/api/reorder", `{"column":"lane:99","slugs":[]}`); code != http.StatusConflict {
		t.Fatalf("reorder a missing lane: %d %v", code, out)
	}

	if code, out := post("/api/job/lane", `{"slug":"beta",`+laneArg(review)+`}`); code != http.StatusOK {
		t.Fatalf("move between lanes: %d %v", code, out)
	}
	if code, out := post("/api/job/lane", `{"slug":"alpha",`+laneArg(0)+`}`); code != http.StatusOK {
		t.Fatalf("unpark: %d %v", code, out)
	}
	b = board()
	if got := strings.Join(slugsOf(b, review), " "); got != "beta" {
		t.Fatalf("review lane: %q", got)
	}
	if got := strings.Join(slugsOf(b, id), " "); got != "" {
		t.Fatalf("first lane: %q", got)
	}
	if got := strings.Join(slugsOf(b, 0), " "); got != "gamma alpha" {
		t.Fatalf("state columns: %q", got)
	}

	if code, out := post("/api/lanes", `{"op":"rename","id":`+jsonInt(review)+`,"name":"in review"}`); code != http.StatusOK {
		t.Fatalf("rename lane: %d %v", code, out)
	}
	if code, out := post("/api/lanes", `{"op":"rename","id":99,"name":"x"}`); code != http.StatusNotFound {
		t.Fatalf("rename a missing lane: %d %v", code, out)
	}
	if code, out := post("/api/lanes", `{"op":"remove","id":`+jsonInt(review)+`}`); code != http.StatusOK {
		t.Fatalf("remove lane: %d %v", code, out)
	}
	if code, out := post("/api/lanes", `{"op":"remove","id":`+jsonInt(review)+`}`); code != http.StatusNotFound {
		t.Fatalf("remove a lane twice: %d %v", code, out)
	}
	if code, out := post("/api/lanes", `{"op":"drop","id":1}`); code != http.StatusBadRequest {
		t.Fatalf("unknown op: %d %v", code, out)
	}
	b = board()
	if len(b.Lanes) != 1 || b.Lanes[0].Name != "later on" {
		t.Fatalf("lanes after remove: %+v", b.Lanes)
	}
	if got := strings.Join(slugsOf(b, 0), " "); got != "gamma alpha beta" {
		t.Fatalf("a removed lane's jobs go back to the end of their state columns: %q", got)
	}
	for _, j := range b.Jobs {
		if j.Slug == "beta" && j.State != "blocked" {
			t.Fatalf("beta: %+v", j)
		}
	}
}

func jsonInt(n int64) string { return strconv.FormatInt(n, 10) }

func TestColumns(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "d.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	svc := &jobs.Service{Store: st, Cfg: jobs.Config{Home: t.TempDir()}, Tmux: jobs.NewTmux()}
	a := New(svc, jobs.NewDaemon(svc), "test")
	mux := http.NewServeMux()
	a.Register(mux)

	post := func(path, body string) (int, map[string]any) {
		t.Helper()
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, path, strings.NewReader(body)))
		var out map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
			t.Fatalf("%s: %s: %v", path, rec.Body.String(), err)
		}
		return rec.Code, out
	}
	columns := func() string {
		t.Helper()
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/board", nil))
		var b webBoard
		if err := json.Unmarshal(rec.Body.Bytes(), &b); err != nil {
			t.Fatalf("%s: %v", rec.Body.String(), err)
		}
		return strings.Join(b.Columns, " ")
	}

	if got := columns(); got != "draft working blocked done watching" {
		t.Fatalf("fresh board: %q", got)
	}
	_, out := post("/api/lanes", `{"op":"add","name":"later"}`)
	later := jsonInt(int64(out["lane"].(map[string]any)["id"].(float64)))
	_, out = post("/api/lanes", `{"op":"add","name":"review"}`)
	review := jsonInt(int64(out["lane"].(map[string]any)["id"].(float64)))
	if got := columns(); got != "draft working blocked done watching lane:"+later+" lane:"+review {
		t.Fatalf("new lanes go last: %q", got)
	}

	if code, out := post("/api/columns", `{"columns":["lane:`+later+`","watching","draft","done","nope","lane:99","working"]}`); code != http.StatusOK {
		t.Fatalf("reorder columns: %d %v", code, out)
	}
	if got := columns(); got != "lane:"+later+" watching draft done working blocked lane:"+review {
		t.Fatalf("reordered (unknown dropped, missing appended): %q", got)
	}
	_, out = post("/api/lanes", `{"op":"add","name":"someday"}`)
	someday := jsonInt(int64(out["lane"].(map[string]any)["id"].(float64)))
	if got := columns(); got != "lane:"+later+" watching draft done working blocked lane:"+review+" lane:"+someday {
		t.Fatalf("a lane added after a reorder goes last: %q", got)
	}
	if code, out := post("/api/lanes", `{"op":"remove","id":`+later+`}`); code != http.StatusOK {
		t.Fatalf("remove: %d %v", code, out)
	}
	if got := columns(); got != "watching draft done working blocked lane:"+review+" lane:"+someday {
		t.Fatalf("after remove: %q", got)
	}
	if code, out := post("/api/reorder", `{"column":"watching","slugs":[]}`); code != http.StatusOK {
		t.Fatalf("reorder WATCHING: %d %v", code, out)
	}
	if code, out := post("/api/columns", `{"columns":"draft"}`); code != http.StatusBadRequest {
		t.Fatalf("bad body: %d %v", code, out)
	}
}

func TestDoneLeavesTheLane(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "d.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	svc := &jobs.Service{Store: st, Cfg: jobs.Config{Home: t.TempDir()}, Tmux: jobs.NewTmux()}
	a := New(svc, jobs.NewDaemon(svc), "test")
	mux := http.NewServeMux()
	a.Register(mux)
	post := func(path, body string) int {
		t.Helper()
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, path, strings.NewReader(body)))
		return rec.Code
	}
	j, err := svc.Create("parked", "", t.TempDir(), "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.UpdateJobFields(j.ID, j.Rev, map[string]any{"state": "blocked"}); err != nil {
		t.Fatal(err)
	}
	lane, err := svc.AddLane("later")
	if err != nil {
		t.Fatal(err)
	}
	if code := post("/api/job/lane", `{"slug":"parked","lane":`+jsonInt(lane.ID)+`}`); code != http.StatusOK {
		t.Fatalf("park: %d", code)
	}
	if code := post("/api/done", `{"slug":"parked"}`); code != http.StatusOK {
		t.Fatalf("done: %d", code)
	}
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/board", nil))
	var b webBoard
	if err := json.Unmarshal(rec.Body.Bytes(), &b); err != nil {
		t.Fatal(err)
	}
	if len(b.Jobs) != 1 || b.Jobs[0].State != "done" || b.Jobs[0].Lane != 0 {
		t.Fatalf("after DONE: %+v", b.Jobs)
	}
	order, _ := st.GetBoardOrder()
	if got := order["done"]; len(got) != 1 || got[0] != j.ID {
		t.Fatalf("done order %v", got)
	}
}

func TestWatchPrompt(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "d.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	svc := &jobs.Service{Store: st, Cfg: jobs.Config{Home: t.TempDir()}, Tmux: jobs.NewTmux()}
	a := New(svc, jobs.NewDaemon(svc), "test")
	mux := http.NewServeMux()
	a.Register(mux)
	post := func(path, body string) int {
		t.Helper()
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, path, strings.NewReader(body)))
		return rec.Code
	}
	prompt := func() string {
		t.Helper()
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/board", nil))
		var b webBoard
		if err := json.Unmarshal(rec.Body.Bytes(), &b); err != nil {
			t.Fatalf("%s: %v", rec.Body.String(), err)
		}
		return b.WatchPrompt
	}
	if got := prompt(); got != jobs.DefaultWatchPrompt || !strings.Contains(got, "30 minutes") {
		t.Fatalf("default prompt: %q", got)
	}
	if code := post("/api/watch-prompt", `{"text":"  Keep an eye on it.\n\nThen stop.  "}`); code != http.StatusOK {
		t.Fatalf("set: %d", code)
	}
	if got := prompt(); got != "Keep an eye on it.\n\nThen stop." {
		t.Fatalf("set prompt: %q", got)
	}
	if code := post("/api/watch-prompt", `{"text":"`+strings.Repeat("x", 8001)+`"}`); code != http.StatusBadRequest {
		t.Fatalf("too long: %d", code)
	}
	if code := post("/api/watch-prompt", `{"text":""}`); code != http.StatusOK {
		t.Fatalf("reset: %d", code)
	}
	if got := prompt(); got != jobs.DefaultWatchPrompt {
		t.Fatalf("after reset: %q", got)
	}
	if _, err := svc.Create("draft", "", t.TempDir(), ""); err != nil {
		t.Fatal(err)
	}
	if code := post("/api/watch", `{"slug":"draft"}`); code != http.StatusConflict {
		t.Fatalf("watch a not-started draft: %d", code)
	}
	if code := post("/api/watch", `{"slug":"nope"}`); code != http.StatusNotFound {
		t.Fatalf("watch a missing job: %d", code)
	}
}
