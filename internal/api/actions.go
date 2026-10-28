package api

import (
	"log"
	"net/http"
	"strings"

	"driver/internal/store"
)

// jobReq is the decoded {"slug"} body plus the job it resolved to.
type jobReq struct {
	Slug string `json:"slug"`
	job  *store.Job
}

// action wraps a {"slug"} → {"ok":true} verb: POST only, 400 on bad JSON,
// 404 on an unknown slug, 409 on a refusal.
func (a *API) action(name string, f func(jobReq) error) http.HandlerFunc {
	return a.actionWith(name, a.Svc.Lookup, f)
}

// archivedAction resolves the slug archive-first (unarchive, delete).
func (a *API) archivedAction(name string, f func(jobReq) error) http.HandlerFunc {
	return a.actionWith(name, a.Svc.LookupArchivedFirst, f)
}

func (a *API) actionWith(name string, lookup func(string) (*store.Job, error), f func(jobReq) error) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req jobReq
		if !decodePost(w, r, &req) {
			return
		}
		req.Slug = strings.TrimSpace(req.Slug)
		j, err := lookup(req.Slug)
		if err != nil {
			writeFailure(w, name, err)
			return
		}
		req.job = j
		if err := f(req); err != nil {
			log.Printf("%s %s: refused: %v", name, req.Slug, err)
			writeFailure(w, name, err)
			return
		}
		log.Printf("%s %s", name, req.Slug)
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	}
}

func (a *API) reorder(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Column string   `json:"column"`
		Slugs  []string `json:"slugs"`
	}
	if !decodePost(w, r, &req) {
		return
	}
	if err := a.Svc.Reorder(req.Column, req.Slugs); err != nil {
		writeFailure(w, "reorder", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (a *API) jobColor(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Slug  string `json:"slug"`
		Color string `json:"color"`
	}
	if !decodePost(w, r, &req) {
		return
	}
	j, err := a.Svc.Lookup(req.Slug)
	if err != nil {
		writeFailure(w, "color", err)
		return
	}
	if err := a.Svc.SetColor(j, strings.TrimSpace(req.Color)); err != nil {
		writeFailure(w, "color", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (a *API) jobRename(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Slug string `json:"slug"`
		To   string `json:"to"`
		Name string `json:"name"`
	}
	if !decodePost(w, r, &req) {
		return
	}
	j, err := a.Svc.Lookup(req.Slug)
	if err != nil {
		writeFailure(w, "rename", err)
		return
	}
	to, err := a.Svc.Rename(j, req.To, req.Name)
	if err != nil {
		writeFailure(w, "rename", err)
		return
	}
	log.Printf("rename %s → %s", req.Slug, to)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "slug": to})
}

// jobFork is POST /api/job/fork {slug, to, name}: a copy of the job under a
// new slug and name, in the same column; answers with the copy's form.
func (a *API) jobFork(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Slug string `json:"slug"`
		To   string `json:"to"`
		Name string `json:"name"`
	}
	if !decodePost(w, r, &req) {
		return
	}
	j, err := a.Svc.Lookup(req.Slug)
	if err != nil {
		writeFailure(w, "fork", err)
		return
	}
	fork, err := a.Svc.Fork(j, req.To, req.Name)
	if err != nil {
		log.Printf("fork %s: refused: %v", req.Slug, err)
		writeFailure(w, "fork", err)
		return
	}
	f, err := a.formFor(fork)
	if err != nil {
		writeFailure(w, "fork", err)
		return
	}
	log.Printf("fork %s → %s", req.Slug, fork.Slug)
	writeJSON(w, http.StatusOK, f)
}

func (a *API) jobNotes(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Slug string `json:"slug"`
		Text string `json:"text"`
	}
	if !decodePost(w, r, &req) {
		return
	}
	j, err := a.Svc.Lookup(req.Slug)
	if err != nil {
		writeFailure(w, "notes", err)
		return
	}
	if err := a.Svc.SetNotes(j, req.Text); err != nil {
		writeFailure(w, "notes", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (a *API) jobLinks(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Slug string `json:"slug"`
		Op   string `json:"op"`
		URL  string `json:"url"`
		ID   int64  `json:"id"`
	}
	if !decodePost(w, r, &req) {
		return
	}
	j, err := a.Svc.Lookup(req.Slug)
	if err != nil {
		writeFailure(w, "links", err)
		return
	}
	switch req.Op {
	case "add":
		l, err := a.Svc.AddLink(j, req.URL)
		if err != nil {
			writeFailure(w, "links", err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "link": webLinksFrom([]*store.Link{l})[0]})
	case "remove":
		if err := a.Svc.RemoveLink(j, req.ID); err != nil {
			writeLinkFailure(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	default:
		writeError(w, http.StatusBadRequest, "op must be add or remove")
	}
}

// writeLinkFailure is writeFailure with "no such link" for the not-found case.
func writeLinkFailure(w http.ResponseWriter, err error) {
	if isNotFound(err) {
		writeError(w, http.StatusNotFound, "no such link")
		return
	}
	writeFailure(w, "links", err)
}

func (a *API) prAttach(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Slug string `json:"slug"`
		URL  string `json:"url"`
	}
	if !decodePost(w, r, &req) {
		return
	}
	j, err := a.Svc.Lookup(req.Slug)
	if err != nil {
		writeFailure(w, "pr-attach", err)
		return
	}
	attached, err := a.Svc.AttachPR(j, req.URL)
	if err != nil {
		writeFailure(w, "pr-attach", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "attached": attached})
}
