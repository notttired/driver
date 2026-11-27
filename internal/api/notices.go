package api

import (
	"context"
	"net/http"
	"time"
)

const noticesWait = 25 * time.Second

func (a *API) notices(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "GET only")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), noticesWait)
	defer cancel()
	writeJSON(w, http.StatusOK, map[string]any{"notices": a.Daemon.Listen(ctx)})
}
