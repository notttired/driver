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

func TestJobLinksPins(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "d.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	svc := &jobs.Service{Store: st, Cfg: jobs.Config{Home: t.TempDir()}, Tmux: jobs.NewTmux()}
	a := New(svc, jobs.NewDaemon(svc), "test")
	j, err := svc.Create("pins", "", t.TempDir(), "")
	if err != nil {
		t.Fatal(err)
	}
	post := func(body string) (int, map[string]any) {
		t.Helper()
		rec := httptest.NewRecorder()
		a.jobLinks(rec, httptest.NewRequest(http.MethodPost, "/api/job/links", strings.NewReader(body)))
		var out map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
			t.Fatalf("%s: %v", rec.Body.String(), err)
		}
		return rec.Code, out
	}
	form := func() []webLink {
		t.Helper()
		rec := httptest.NewRecorder()
		a.jobForm(rec, httptest.NewRequest(http.MethodGet, "/api/job/form?slug=pins", nil))
		var f jobForm
		if err := json.Unmarshal(rec.Body.Bytes(), &f); err != nil {
			t.Fatalf("%s: %v", rec.Body.String(), err)
		}
		return f.Links
	}

	code, out := post(`{"slug":"pins","op":"add","url":"https://you.io/x"}`)
	added, _ := out["link"].(map[string]any)
	if code != http.StatusOK || added["mentions"] != float64(1) || added["yours"] != true {
		t.Fatalf("a link added by hand is yours: %d %v", code, out)
	}

	l, _, _ := st.AddLink(j.ID, "url", "https://pi.io/y")
	st.AddMention(l.ID, "a1", "assistant")
	l2, _, _ := st.AddLink(j.ID, "url", "https://pi.io/z")
	st.AddMention(l2.ID, "a1", "assistant")
	st.AddMention(l2.ID, "a2", "assistant")

	if code, out := post(`{"slug":"pins","op":"pin","url":"https://pi.io/y"}`); code != http.StatusOK || out["pinned"] != true {
		t.Fatalf("pin: %d %v", code, out)
	}
	if code, out := post(`{"slug":"pins","op":"pin","url":"https://not.linked/x"}`); code != http.StatusOK || out["pinned"] != false {
		t.Fatalf("pin of a URL the job does not have: %d %v", code, out)
	}
	links := form()
	if len(links) != 3 || links[0].URL != "https://you.io/x" || !links[0].Yours ||
		links[1].URL != "https://pi.io/y" || links[1].PinnedAt == 0 || links[1].Mentions != 1 ||
		links[2].URL != "https://pi.io/z" || links[2].Mentions != 2 || links[2].PinnedAt != 0 || links[2].Yours {
		t.Fatalf("form links: %+v", links)
	}

	if code, out := post(`{"slug":"pins","op":"unpin","id":` + strconv.FormatInt(l.ID, 10) + `}`); code != http.StatusOK || out["ok"] != true {
		t.Fatalf("unpin: %d %v", code, out)
	}
	if links := form(); links[1].URL != "https://pi.io/z" || links[2].URL != "https://pi.io/y" || links[2].PinnedAt != 0 {
		t.Fatalf("after unpin: %+v", links)
	}
	if code, out := post(`{"slug":"pins","op":"unpin","id":999}`); code != http.StatusNotFound || out["error"] != "no such link" {
		t.Fatalf("unpin of nothing: %d %v", code, out)
	}
	if code, _ := post(`{"slug":"pins","op":"bogus"}`); code != http.StatusBadRequest {
		t.Fatalf("bogus op: %d", code)
	}
}
