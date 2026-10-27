package api

import (
	"net/http"
	"time"

	"driver/internal/jobs"
)

// browserTimeout bounds one round trip to the browser. It is generous because
// the first call makes macOS ask the operator for permission and waits for
// the answer; a hung browser must still
// not hang the links panel.
const browserTimeout = 20 * time.Second

// tabsHandler → GET /api/tabs: the configured browser's open http(s) tabs,
// front window first, for the links panel's picker. A browser with no
// process answers running:false rather than being launched.
func (a *API) tabsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "GET only")
		return
	}
	browser := a.Svc.Cfg.Browser
	if !jobs.BrowserRunning(browser) {
		writeJSON(w, http.StatusOK, map[string]any{"browser": browser, "running": false, "tabs": []jobs.Tab{}})
		return
	}
	tabs, err := jobs.ListTabs(browser, browserTimeout)
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"browser": browser, "running": true, "tabs": tabs})
}
