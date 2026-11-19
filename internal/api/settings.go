package api

import (
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"driver/internal/jobs"
	"driver/internal/store"
)

// jobPaste is POST /api/job/paste {"slug", "thread"?, "text"}: text into
// that pane's pi editor, nothing sent.
func (a *API) jobPaste(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Slug   string `json:"slug"`
		Thread int64  `json:"thread"`
		Text   string `json:"text"`
	}
	if !decodePost(w, r, &req) {
		return
	}
	j, err := a.Svc.Lookup(strings.TrimSpace(req.Slug))
	if err != nil {
		writeFailure(w, "paste", err)
		return
	}
	var t *store.Thread
	if req.Thread != 0 {
		if t, err = a.Svc.LookupThread(j, req.Thread); err != nil {
			writeThreadFailure(w, err)
			return
		}
	}
	if err := a.Svc.Paste(j, t, req.Text); err != nil {
		writeFailure(w, "paste", err)
		return
	}
	log.Printf("paste %s: %d characters", j.Slug, len(req.Text))
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// browserContext is GET /api/context/browser: the browser's front tab as a
// BrowserContext plus the prompt it reads as in a pane.
func (a *API) browserContext(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "GET only")
		return
	}
	ctx, err := jobs.FrontTabContext(a.Svc.Cfg.Browser, browserTimeout)
	if err != nil {
		writeFailure(w, "browser context", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"url": ctx.URL, "title": ctx.Title, "text": ctx.Text, "selected": ctx.Selected, "prompt": ctx.Prompt()})
}

// settings is GET /api/settings: what the ⚙ panel shows about this install.
func (a *API) settings(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "GET only")
		return
	}
	bin, _ := os.Executable()
	if real, err := filepath.EvalSymlinks(bin); err == nil {
		bin = real
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"version": a.Version, "binary": bin, "home": a.Svc.Cfg.Home, "browser": a.Svc.Cfg.Browser, "serve": a.Svc.Cfg.ServeURL(),
	})
}

// settingsProbe is POST /api/settings/probe {"what": "folders" | "browser"}:
// try the thing from the daemon, so any permission macOS asks for is asked
// of driver here and now rather than of a pane mid-task.
func (a *API) settingsProbe(w http.ResponseWriter, r *http.Request) {
	var req struct {
		What string `json:"what"`
	}
	if !decodePost(w, r, &req) {
		return
	}
	switch req.What {
	case "folders":
		writeJSON(w, http.StatusOK, map[string]any{"folders": jobs.ProbeFolders(a.Svc.Cfg.UserHome)})
	case "browser":
		ctx, err := jobs.FrontTabContext(a.Svc.Cfg.Browser, browserTimeout)
		if err != nil {
			writeJSON(w, http.StatusOK, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "title": ctx.Title, "url": ctx.URL, "text": ctx.Text != "", "selected": ctx.Selected})
	default:
		writeError(w, http.StatusBadRequest, "what must be folders or browser")
	}
}

// settingsOpen is POST /api/settings/open {"pane"}: System Settings on a
// Privacy & Security pane, or the binary in the Finder.
func (a *API) settingsOpen(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Pane string `json:"pane"`
	}
	if !decodePost(w, r, &req) {
		return
	}
	if err := jobs.OpenSystemPane(req.Pane); err != nil {
		writeFailure(w, "settings open", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}
