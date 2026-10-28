package api

import (
	"net/http"
	"sort"
	"time"

	"driver/internal/jobs"
	"driver/internal/store"
)

// webBoard is GET /api/board (docs/API.md "Board").
type webBoard struct {
	Daemon     jobs.Status `json:"daemon"`
	DaemonOK   bool        `json:"daemon_ok"`
	Jobs       []webJob    `json:"jobs"`
	Contract   int         `json:"contract"`
	Operator   string      `json:"operator"`
	DefaultDir string      `json:"default_dir"`
}

type webJob struct {
	Slug          string       `json:"slug"`
	DisplaySlug   string       `json:"display_slug,omitempty"`
	Name          string       `json:"name,omitempty"`
	Dir           string       `json:"dir"`
	Model         string       `json:"model,omitempty"`
	Color         string       `json:"color,omitempty"`
	State         string       `json:"state"`
	Outcome       string       `json:"outcome,omitempty"`
	Live          bool         `json:"live"`
	Pi            *jobs.PiInfo `json:"pi,omitempty"`
	HasNotes      bool         `json:"has_notes"`
	Links         []webLink    `json:"links,omitempty"`
	StartedAt     int64        `json:"started_at,omitempty"`
	FinishedAt    int64        `json:"finished_at,omitempty"`
	MTime         int64        `json:"mtime"`
	Archived      bool         `json:"archived,omitempty"`
	Reactivatable bool         `json:"reactivatable,omitempty"`
}

type webLink struct {
	ID       int64  `json:"id"`
	Kind     string `json:"kind"`
	URL      string `json:"url"`
	Status   string `json:"status,omitempty"`
	StatusAt int64  `json:"status_at,omitempty"`
}

func webLinksFrom(ls []*store.Link) []webLink {
	if len(ls) == 0 {
		return nil
	}
	out := make([]webLink, 0, len(ls))
	for _, l := range ls {
		out = append(out, webLink{ID: l.ID, Kind: l.Kind, URL: l.URL, Status: l.Status, StatusAt: l.StatusAt})
	}
	return out
}

// daemonOKWindow is how stale last_tick may be before the chrome shows the
// daemon as down.
const daemonOKWindow = 10

func (a *API) board(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "GET only")
		return
	}
	b, err := a.buildBoard()
	if err != nil {
		writeFailure(w, "board", err)
		return
	}
	writeJSON(w, http.StatusOK, b)
}

func (a *API) buildBoard() (*webBoard, error) {
	st := a.Svc.Store
	active, err := st.ListJobs()
	if err != nil {
		return nil, err
	}
	archived, err := st.ListArchivedJobs()
	if err != nil {
		return nil, err
	}
	order, err := st.GetBoardOrder()
	if err != nil {
		return nil, err
	}
	links, err := st.ListAllLinks()
	if err != nil {
		return nil, err
	}
	snap := a.Svc.Tmux.Snapshot()
	now := time.Now().Unix()

	// per column: members alphabetically (ListJobs orders by slug), then the
	// stored order applied — unlisted first, listed after
	byID := make(map[int64]*store.Job, len(active))
	members := map[string][]int64{}
	for _, j := range active {
		byID[j.ID] = j
		col := jobs.DisplayState(j)
		members[col] = append(members[col], j.ID)
	}
	out := make([]webJob, 0, len(active)+len(archived))
	for _, col := range jobs.Columns {
		for _, id := range store.ApplyColumnOrder(members[col], order[col]) {
			j := byID[id]
			sess := jobs.SessionName(j)
			live := snap.Live(sess)
			obs := jobs.Observe(j, live, a.Svc.Cfg.ReadStatus(j), now)
			out = append(out, webJob{
				Slug: j.Slug, Name: j.Name, Dir: j.Dir, Model: j.Model, Color: j.Color,
				State: col, Outcome: j.Outcome, Live: live, Pi: obs.Pi,
				HasNotes: j.Notes != "", Links: webLinksFrom(links[j.ID]),
				StartedAt: j.StartedAt, FinishedAt: j.FinishedAt, MTime: j.UpdatedAt,
				Reactivatable: j.State == "done",
			})
		}
	}
	sort.SliceStable(archived, func(i, k int) bool { return archived[i].ArchivedAt > archived[k].ArchivedAt })
	for _, j := range archived {
		out = append(out, webJob{
			Slug: j.ArchivedName, DisplaySlug: j.Slug, Name: j.Name, Dir: j.Dir, Model: j.Model, Color: j.Color,
			State: jobs.DisplayState(j), Outcome: j.Outcome, HasNotes: j.Notes != "",
			Links: webLinksFrom(links[j.ID]), StartedAt: j.StartedAt, FinishedAt: j.FinishedAt,
			MTime: j.UpdatedAt, Archived: true,
		})
	}
	ds := a.Daemon.Status()
	return &webBoard{
		Daemon:     ds,
		DaemonOK:   ds.LastTick != 0 && now-ds.LastTick <= daemonOKWindow,
		Jobs:       out,
		Contract:   Contract,
		Operator:   a.Svc.Cfg.Operator,
		DefaultDir: a.Svc.Cfg.DefaultDir,
	}, nil
}
