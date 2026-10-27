// Package api serves the JSON routes in docs/API.md over the jobs engine:
// the board payload, the action verbs, the job editor, directory and model
// autocomplete, and the terminal websocket.
package api

import (
	"net/http"
	"time"

	"driver/internal/jobs"
)

// Contract is the wire contract version the SPA checks.
const Contract = 1

// API holds the handlers' dependencies; nothing here is global.
type API struct {
	Svc     *jobs.Service
	Daemon  *jobs.Daemon
	Version string
	models  *modelCache
}

// New wires an API over the engine.
func New(svc *jobs.Service, d *jobs.Daemon, version string) *API {
	return &API{Svc: svc, Daemon: d, Version: version, models: newModelCache(svc.Cfg, 10*time.Minute)}
}

// Register mounts every route on mux.
func (a *API) Register(mux *http.ServeMux) {
	mux.HandleFunc("/api/health", a.health)
	mux.HandleFunc("/api/board", a.board)
	mux.HandleFunc("/api/queue", a.action("submit", func(j jobReq) error { return a.Svc.Submit(j.job) }))
	mux.HandleFunc("/api/stop", a.action("stop", func(j jobReq) error { return a.Svc.Stop(j.job) }))
	mux.HandleFunc("/api/done", a.action("done", func(j jobReq) error { return a.Svc.Done(j.job) }))
	mux.HandleFunc("/api/kill", a.action("kill", func(j jobReq) error { return a.Svc.Kill(j.job) }))
	mux.HandleFunc("/api/reactivate", a.action("reactivate", func(j jobReq) error { return a.Svc.Reactivate(j.job) }))
	mux.HandleFunc("/api/archive", a.action("archive", func(j jobReq) error { return a.Svc.Archive(j.job) }))
	mux.HandleFunc("/api/unarchive", a.archivedAction("unarchive", func(j jobReq) error { return a.Svc.Unarchive(j.job) }))
	mux.HandleFunc("/api/delete", a.archivedAction("delete", func(j jobReq) error { return a.Svc.Delete(j.job) }))
	mux.HandleFunc("/api/reorder", a.reorder)
	mux.HandleFunc("/api/job/color", a.jobColor)
	mux.HandleFunc("/api/job/rename", a.jobRename)
	mux.HandleFunc("/api/job/notes", a.jobNotes)
	mux.HandleFunc("/api/job/links", a.jobLinks)
	mux.HandleFunc("/api/job/open", a.action("open", func(j jobReq) error { return a.Svc.OpenEditor(j.job) }))
	mux.HandleFunc("/api/pr-attach", a.prAttach)
	mux.HandleFunc("/api/job/form", a.jobForm)
	mux.HandleFunc("/api/job/saveform", a.jobSaveForm)
	mux.HandleFunc("/api/job/create", a.jobCreate)
	mux.HandleFunc("/api/dirs", a.dirs)
	mux.HandleFunc("/api/models", a.modelsHandler)
	mux.HandleFunc("/api/tabs", a.tabsHandler)
	mux.HandleFunc("/api/term", a.term)
}

func (a *API) health(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "GET only")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "pid": a.Daemon.Status().PID, "version": a.Version, "contract": Contract})
}
