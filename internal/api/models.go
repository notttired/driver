package api

import (
	"net/http"
	"sync"
	"time"

	"driver/internal/jobs"
	"driver/internal/pi"
)

// modelCache memoises `pi --list-models` for a while; ?refresh=1 bypasses it.
type modelCache struct {
	cfg jobs.Config
	ttl time.Duration

	mu     sync.Mutex
	models []pi.Model
	at     time.Time
}

func newModelCache(cfg jobs.Config, ttl time.Duration) *modelCache {
	return &modelCache{cfg: cfg, ttl: ttl}
}

// piListTimeout bounds one `pi --list-models` run.
const piListTimeout = 45 * time.Second

func (c *modelCache) get(refresh bool) ([]pi.Model, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !refresh && c.models != nil && time.Since(c.at) < c.ttl {
		return c.models, nil
	}
	models, err := pi.ListModels(c.cfg.PiBin, piListTimeout)
	if err != nil {
		if c.models != nil {
			return c.models, nil // stale beats nothing
		}
		return nil, err
	}
	if models == nil {
		models = []pi.Model{}
	}
	c.models, c.at = models, time.Now()
	return models, nil
}

func (a *API) modelsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "GET only")
		return
	}
	models, err := a.models.get(r.URL.Query().Get("refresh") == "1")
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"models": models, "default": pi.DefaultModel(a.Svc.Cfg.UserHome)})
}
