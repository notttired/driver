package web

import (
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"strings"
	"time"

	"driver/internal/api"
)

// Listen binds 127.0.0.1:<port>; an occupied port is a clear error up front,
// before the daemon starts anything.
func Listen(port int) (net.Listener, error) {
	addr := fmt.Sprintf("127.0.0.1:%d", port)
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("port %d is already in use (another driver serve?): %w", port, err)
	}
	return ln, nil
}

// Handler builds the mux: the SPA at / (hashed /assets/* immutable-cached,
// everything else no-cache so a binary upgrade is never pinned to a stale
// index.html) plus the API's routes.
func Handler(a *api.API) (http.Handler, error) {
	static, err := fs.Sub(assets, "_ui/dist")
	if err != nil {
		return nil, fmt.Errorf("embedded assets: %w", err)
	}
	files := http.FileServer(http.FS(static))
	mux := http.NewServeMux()
	mux.Handle("/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/assets/") {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		} else {
			w.Header().Set("Cache-Control", "no-cache")
		}
		files.ServeHTTP(w, r)
	}))
	a.Register(mux)
	return mux, nil
}

// Serve runs the HTTP server on ln until it fails. No write timeout: the
// terminal websocket is long-lived.
func Serve(ln net.Listener, h http.Handler) error {
	srv := &http.Server{Handler: h, ReadHeaderTimeout: 5 * time.Second}
	if err := srv.Serve(ln); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("http server: %w", err)
	}
	return nil
}
