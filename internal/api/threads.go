package api

import (
	"cmp"
	"errors"
	"log"
	"net/http"
	"strconv"
	"strings"

	"driver/internal/jobs"
	"driver/internal/store"
)

// webThread is one tab of a job: its own pi (id 0, named as the job is)
// or a thread's (docs/API.md "Threads").
type webThread struct {
	ID        int64        `json:"id"`
	Name      string       `json:"name"`
	Live      bool         `json:"live"`
	Pi        *jobs.PiInfo `json:"pi,omitempty"`
	Retryable bool         `json:"retryable,omitempty"`
	StartedAt int64        `json:"started_at,omitempty"`
	Attention bool         `json:"attention,omitempty"`
}

// threadsOf is the job's tabs — its own pi (own, as observed) first, then
// every open thread as the daemon reads it — and the observations behind
// them, for Combine.
func (a *API) threadsOf(j *store.Job, own webThread, ownObs jobs.Observation, now int64) ([]webThread, []jobs.Observation, error) {
	tabs, all := []webThread{own}, []jobs.Observation{ownObs}
	threads, err := a.Svc.OpenThreads(j)
	if err != nil {
		return nil, nil, err
	}
	snap := a.Svc.Tmux.Snapshot()
	for _, t := range threads {
		live := snap.Live(jobs.ThreadSessionName(j, t))
		status := a.Svc.Cfg.ReadThreadStatus(j, t)
		obs := jobs.ObserveThread(j.State, t.StartedAt, live, status, now)
		wt := webThread{ID: t.ID, Name: t.Name, Live: live, Pi: obs.Pi, StartedAt: t.StartedAt, Attention: t.AttentionAt != 0}
		if obs.State == "blocked" {
			if conv, err := a.conversations.recordOf(threadKey(j, t), t.PiSession); err == nil {
				wt.Retryable = jobs.PlanThreadRetry(live, status, conv)
				if msg, failed := conv.Failed(); failed {
					wt.Pi.Error = cmp.Or(msg, "error")
				}
			}
		}
		tabs, all = append(tabs, wt), append(all, obs)
	}
	return tabs, all, nil
}

func threadKey(j *store.Job, t *store.Thread) string {
	return strconv.FormatInt(j.ID, 10) + "/t" + strconv.FormatInt(t.ID, 10)
}

// threadParam reads ?thread=<id> (0 or absent: the job's own pi).
func (a *API) threadParam(j *store.Job, r *http.Request) (*store.Thread, error) {
	raw := strings.TrimSpace(r.URL.Query().Get("thread"))
	if raw == "" || raw == "0" {
		return nil, nil
	}
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || id <= 0 {
		return nil, errors.New("thread must be a thread id")
	}
	return a.Svc.LookupThread(j, id)
}

// jobThread is POST /api/job/thread: {"slug", "op": open | close | stop |
// retry | rename, "id"?, "from"?, "name"?}.
func (a *API) jobThread(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Slug string `json:"slug"`
		Op   string `json:"op"`
		ID   int64  `json:"id"`
		From int64  `json:"from"`
		Name string `json:"name"`
	}
	if !decodePost(w, r, &req) {
		return
	}
	j, err := a.Svc.Lookup(strings.TrimSpace(req.Slug))
	if err != nil {
		writeFailure(w, "thread", err)
		return
	}
	if req.Op == "open" {
		t, err := a.Svc.OpenThread(j, req.From, req.Name)
		if err != nil {
			writeThreadFailure(w, err)
			return
		}
		log.Printf("thread %s: opened %q (%d)", j.Slug, t.Name, t.ID)
		live := a.Svc.Tmux.HasSession(jobs.ThreadSessionName(j, t))
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "thread": webThread{ID: t.ID, Name: t.Name, Live: live, StartedAt: t.StartedAt}})
		return
	}
	if req.ID == 0 && req.Op == "close" {
		if err := a.Svc.CloseOwnThread(j); err != nil {
			log.Printf("thread close %s (own): refused: %v", j.Slug, err)
			writeThreadFailure(w, err)
			return
		}
		log.Printf("thread close %s (own): the next thread is the job's pi now", j.Slug)
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
		return
	}
	t, err := a.Svc.LookupThread(j, req.ID)
	if err != nil {
		writeThreadFailure(w, err)
		return
	}
	switch req.Op {
	case "close":
		err = a.Svc.CloseThread(j, t)
	case "stop":
		err = a.Svc.StopThread(j, t)
	case "retry":
		err = a.Svc.RetryThread(j, t)
	case "rename":
		err = a.Svc.RenameThread(j, t, req.Name)
	default:
		writeError(w, http.StatusBadRequest, "op must be open, close, stop, retry or rename")
		return
	}
	if err != nil {
		log.Printf("thread %s %s %q: refused: %v", req.Op, j.Slug, t.Name, err)
		writeThreadFailure(w, err)
		return
	}
	log.Printf("thread %s %s %q", req.Op, j.Slug, t.Name)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (a *API) tabAction(name string, f func(*store.Job, *store.Thread) error) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Slug   string `json:"slug"`
			Thread int64  `json:"thread"`
		}
		if !decodePost(w, r, &req) {
			return
		}
		j, err := a.Svc.Lookup(strings.TrimSpace(req.Slug))
		if err != nil {
			writeFailure(w, name, err)
			return
		}
		var t *store.Thread
		if req.Thread != 0 {
			if t, err = a.Svc.LookupThread(j, req.Thread); err != nil {
				writeThreadFailure(w, err)
				return
			}
		}
		if err := f(j, t); err != nil {
			log.Printf("%s %s (tab %d): refused: %v", name, j.Slug, req.Thread, err)
			writeFailure(w, name, err)
			return
		}
		log.Printf("%s %s (tab %d)", name, j.Slug, req.Thread)
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	}
}

func writeThreadFailure(w http.ResponseWriter, err error) {
	if isNotFound(err) {
		writeError(w, http.StatusNotFound, "no such thread")
		return
	}
	writeFailure(w, "thread", err)
}
