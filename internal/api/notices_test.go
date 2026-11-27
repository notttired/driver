package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"driver/internal/jobs"
	"driver/internal/store"
)

func TestNotices(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "d.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	svc := &jobs.Service{Store: st, Cfg: jobs.Config{Home: t.TempDir(), Notify: true}, Tmux: jobs.NewTmux()}
	d := jobs.NewDaemon(svc)
	banners := 0
	d.Banner = func(jobs.Notice) { banners++ }
	a := New(svc, d, "test")
	mux := http.NewServeMux()
	a.Register(mux)
	listen := func(ctx context.Context) chan *httptest.ResponseRecorder {
		t.Helper()
		got := make(chan *httptest.ResponseRecorder, 1)
		go func() {
			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/notices", nil).WithContext(ctx))
			got <- rec
		}()
		for deadline := time.Now().Add(5 * time.Second); !d.Listening(); time.Sleep(5 * time.Millisecond) {
			if time.Now().After(deadline) {
				t.Fatal("the request never started listening")
			}
		}
		return got
	}
	answer := func(got chan *httptest.ResponseRecorder) *httptest.ResponseRecorder {
		t.Helper()
		select {
		case rec := <-got:
			return rec
		case <-time.After(5 * time.Second):
			t.Fatal("the request never answered")
			return nil
		}
	}

	one := jobs.Notice{Slug: "multi", Thread: 7, Title: "Multi · CI", Body: "idle"}
	got := listen(context.Background())
	d.Notify(one)
	rec := answer(got)
	var out struct {
		Notices []jobs.Notice `json:"notices"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("%s: %v", rec.Body.String(), err)
	}
	if rec.Code != http.StatusOK || len(out.Notices) != 1 || out.Notices[0] != one || banners != 0 {
		t.Fatalf("the app hears the notice, and the daemon posts no banner of its own: %d %s (banners %d)", rec.Code, rec.Body.String(), banners)
	}
	if body := rec.Body.String(); !strings.Contains(body, `"slug":"multi"`) || !strings.Contains(body, `"thread":7`) || !strings.Contains(body, `"title":"Multi · CI"`) || !strings.Contains(body, `"body":"idle"`) {
		t.Fatalf("the notice on the wire: %s", body)
	}

	ctx, cancel := context.WithCancel(context.Background())
	got = listen(ctx)
	cancel()
	if rec := answer(got); rec.Code != http.StatusOK || strings.TrimSpace(rec.Body.String()) != `{"notices":[]}` {
		t.Fatalf("a wait that ends with no news: %d %s", rec.Code, rec.Body.String())
	}

	post := httptest.NewRecorder()
	mux.ServeHTTP(post, httptest.NewRequest(http.MethodPost, "/api/notices", strings.NewReader("{}")))
	if post.Code != http.StatusMethodNotAllowed {
		t.Fatalf("POST notices: %d", post.Code)
	}
}
