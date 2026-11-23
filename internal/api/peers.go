package api

import (
	"log"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"

	"driver/internal/jobs"
	"driver/internal/pi"
	"driver/internal/store"
)

// threadSelf resolves ?job_dir= (or a body's) to the job and the id of the
// pi asking: 0 for the job's own, else the thread's.
func (a *API) threadSelf(w http.ResponseWriter, dir string) (*store.Job, int64, bool) {
	dir = filepath.Clean(strings.TrimSpace(dir))
	j, t, err := a.Svc.LookupThreadDir(dir)
	if err != nil {
		writeFailure(w, "threads", err)
		return nil, 0, false
	}
	if t != nil {
		return j, t.ID, true
	}
	return j, 0, true
}

// jobThreads is GET /api/job/threads?job_dir=: the job's pis as the threads
// read each other, and which of them is asking.
func (a *API) jobThreads(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "GET only")
		return
	}
	j, self, ok := a.threadSelf(w, r.URL.Query().Get("job_dir"))
	if !ok {
		return
	}
	list, err := a.Svc.ThreadList(j)
	if err != nil {
		writeFailure(w, "threads", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"self": self, "threads": list})
}

// jobThreadsUpdate is POST /api/job/threads/update {"job_dir", "seen"}: what
// the asking pi has not heard from the job's other threads.
func (a *API) jobThreadsUpdate(w http.ResponseWriter, r *http.Request) {
	var req struct {
		JobDir string   `json:"job_dir"`
		Seen   []string `json:"seen"`
	}
	if !decodePost(w, r, &req) {
		return
	}
	j, self, ok := a.threadSelf(w, req.JobDir)
	if !ok {
		return
	}
	seen := make(map[string]bool, len(req.Seen))
	for _, id := range req.Seen {
		seen[id] = true
	}
	up, err := a.Svc.PeerUpdate(j, self, seen)
	if err != nil {
		writeFailure(w, "threads update", err)
		return
	}
	carried := up.Carried
	if carried == nil {
		carried = []pi.Carried{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"text": up.Text, "carried": carried, "peers": up.Peers})
}

// jobThreadsHistory is GET /api/job/threads/history?job_dir=&thread=: a
// thread's whole conversation (0 = the job's own pi).
func (a *API) jobThreadsHistory(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "GET only")
		return
	}
	j, _, ok := a.threadSelf(w, r.URL.Query().Get("job_dir"))
	if !ok {
		return
	}
	id, err := strconv.ParseInt(strings.TrimSpace(r.URL.Query().Get("thread")), 10, 64)
	if err != nil || id < 0 {
		writeError(w, http.StatusBadRequest, "thread must be a thread id (0 for the job's own pi)")
		return
	}
	text, err := a.Svc.ThreadHistory(j, id)
	if err != nil {
		if isNotFound(err) {
			writeError(w, http.StatusNotFound, "no such thread")
			return
		}
		writeFailure(w, "threads history", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"text": text})
}

func (a *API) jobThreadsRename(w http.ResponseWriter, r *http.Request) {
	var req struct {
		JobDir string `json:"job_dir"`
		Name   string `json:"name"`
	}
	if !decodePost(w, r, &req) {
		return
	}
	j, self, ok := a.threadSelf(w, req.JobDir)
	if !ok {
		return
	}
	if self == 0 {
		writeFailure(w, "threads rename", jobs.Refuse("the job's first tab keeps its name, %q", jobs.OwnName(j)))
		return
	}
	t, err := a.Svc.LookupThread(j, self)
	if err != nil {
		writeFailure(w, "threads rename", err)
		return
	}
	if err := a.Svc.RenameThread(j, t, req.Name); err != nil {
		writeFailure(w, "threads rename", err)
		return
	}
	if t, err = a.Svc.LookupThread(j, self); err != nil {
		writeFailure(w, "threads rename", err)
		return
	}
	log.Printf("thread rename %s %q (by its pi)", j.Slug, t.Name)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "name": t.Name, "pi_name": jobs.ThreadPiName(j, t)})
}
