package api

import (
	"errors"
	"net/http"
	"strings"

	"driver/internal/jobs"
	"driver/internal/store"
)

// jobForm is GET /api/job/form and the answer to /api/job/create.
type jobForm struct {
	Slug     string    `json:"slug"`
	Name     string    `json:"name"`
	Dir      string    `json:"dir"`
	Model    string    `json:"model"`
	Color    string    `json:"color"`
	Notes    string    `json:"notes"`
	State    string    `json:"state"`
	Editable bool      `json:"editable"`
	Archived bool      `json:"archived"`
	MTime    int64     `json:"mtime"`
	Links    []webLink `json:"links"`
}

func isNotFound(err error) bool { return errors.Is(err, store.ErrNotFound) }

func (a *API) formFor(j *store.Job) (*jobForm, error) {
	links, err := a.Svc.Store.ListLinks(j.ID)
	if err != nil {
		return nil, err
	}
	wl := webLinksFrom(links)
	if wl == nil {
		wl = []webLink{}
	}
	slug := j.Slug
	if j.Archived() {
		slug = j.ArchivedName
	}
	return &jobForm{
		Slug: slug, Name: j.Name, Dir: j.Dir, Model: j.Model, Color: j.Color, Notes: j.Notes,
		State: jobs.DisplayState(j), Editable: jobs.Editable(j), Archived: j.Archived(),
		MTime: j.UpdatedAt, Links: wl,
	}, nil
}

func (a *API) jobForm(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "GET only")
		return
	}
	j, err := a.Svc.Lookup(strings.TrimSpace(r.URL.Query().Get("slug")))
	if err != nil {
		writeFailure(w, "form", err)
		return
	}
	f, err := a.formFor(j)
	if err != nil {
		writeFailure(w, "form", err)
		return
	}
	writeJSON(w, http.StatusOK, f)
}

func (a *API) jobSaveForm(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Slug  string `json:"slug"`
		Dir   string `json:"dir"`
		Model string `json:"model"`
		MTime int64  `json:"mtime"`
	}
	if !decodePost(w, r, &req) {
		return
	}
	j, err := a.Svc.Lookup(strings.TrimSpace(req.Slug))
	if err != nil {
		writeFailure(w, "saveform", err)
		return
	}
	mtime, err := a.Svc.SaveForm(j, req.Dir, req.Model, req.MTime)
	if err != nil {
		writeFailure(w, "saveform", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "mtime": mtime})
}

func (a *API) jobCreate(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Slug  string `json:"slug"`
		Name  string `json:"name"`
		Dir   string `json:"dir"`
		Model string `json:"model"`
	}
	if !decodePost(w, r, &req) {
		return
	}
	j, err := a.Svc.Create(strings.TrimSpace(req.Slug), req.Name, req.Dir, req.Model)
	if err != nil {
		writeFailure(w, "create", err)
		return
	}
	f, err := a.formFor(j)
	if err != nil {
		writeFailure(w, "create", err)
		return
	}
	writeJSON(w, http.StatusOK, f)
}
