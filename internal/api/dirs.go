package api

import (
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"driver/internal/util"
)

// maxDirs caps one autocomplete answer.
const maxDirs = 50

// CompleteDirs implements GET /api/dirs: expand a leading ~, list the
// subdirectories of the typed path's parent whose names start with the last
// path component — absolute, sorted, at most 50, hidden only when the typed
// component starts with "." — and for an empty q, defaultDir's subdirectories.
// A relative q resolves against defaultDir.
func CompleteDirs(q, defaultDir string) []string {
	q = strings.TrimSpace(q)
	var parent, prefix string
	switch {
	case q == "":
		parent, prefix = defaultDir, ""
	default:
		q = util.ExpandTilde(q)
		if !filepath.IsAbs(q) {
			q = filepath.Join(defaultDir, q)
		}
		if strings.HasSuffix(q, "/") {
			parent, prefix = filepath.Clean(q), ""
		} else {
			parent, prefix = filepath.Dir(q), filepath.Base(q)
		}
	}
	entries, err := os.ReadDir(parent)
	if err != nil {
		return []string{}
	}
	showHidden := strings.HasPrefix(prefix, ".")
	out := []string{}
	for _, e := range entries {
		name := e.Name()
		if !strings.HasPrefix(name, prefix) {
			continue
		}
		if !showHidden && strings.HasPrefix(name, ".") {
			continue
		}
		if !e.IsDir() {
			// a symlink to a directory counts as one
			if e.Type()&os.ModeSymlink == 0 {
				continue
			}
			if st, err := os.Stat(filepath.Join(parent, name)); err != nil || !st.IsDir() {
				continue
			}
		}
		out = append(out, filepath.Join(parent, name))
	}
	sort.Strings(out)
	if len(out) > maxDirs {
		out = out[:maxDirs]
	}
	return out
}

func (a *API) dirs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "GET only")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"dirs": CompleteDirs(r.URL.Query().Get("q"), a.Svc.Cfg.DefaultDir)})
}
