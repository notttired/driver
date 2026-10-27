package api

import (
	"net/http"
	"strings"

	"driver/internal/jobs"
	"driver/internal/term"
)

// term is GET /api/term?slug=…: 404/409 as plain HTTP when there is nothing
// to attach to, else the websocket ⇄ pty bridge. A finished job whose pane is
// dead (remain-on-exit) still attaches, so its last screen can be read.
func (a *API) term(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "GET only")
		return
	}
	j, err := a.Svc.Lookup(strings.TrimSpace(r.URL.Query().Get("slug")))
	if err != nil {
		writeFailure(w, "term", err)
		return
	}
	sess := jobs.SessionName(j)
	if !a.Svc.Tmux.Available() || !a.Svc.Tmux.HasSession(sess) {
		writeError(w, http.StatusConflict, "no live session")
		return
	}
	term.Serve(w, r, a.Svc.Tmux.Bin(), sess)
}
