// Package jobs is driver's engine: the job model and its display state, every
// board action (submit, stop, done, kill, reactivate, archive, rename, …),
// the tmux and pi launch plumbing, the 2-second tick loop that derives states
// from tmux liveness plus pi's status.json, notifications, and the PR scan.
package jobs

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"driver/internal/util"
)

// Config is every environment knob the engine reads, resolved once at start
// (see docs/DESIGN.md "Names, paths, knobs").
type Config struct {
	Home       string   // DRIVER_HOME, default ~/.driver
	DBPath     string   // DRIVER_DB_PATH, default <home>/driver.db
	Port       int      // -p / DRIVER_PORT
	PiBin      string   // DRIVER_PI, default "pi" (resolved on PATH at launch)
	PiFlags    []string // DRIVER_PI_FLAGS split on spaces, default ["-a"]; set empty to disable
	PiMode     string   // DRIVER_PI_MODE: "print" (default: the pane runs `pi -p` once per prompt, so only final answers show) or "tui" (pi's interactive UI)
	SelfExe    string   // this binary; a job's pane runs `<SelfExe> pane` (os.Executable, else "driver" on PATH)
	Operator   string   // DRIVER_OPERATOR, else $USER
	DefaultDir string   // DRIVER_DEFAULT_DIR, else $HOME
	Notify     bool     // false when DRIVER_NOTIFY=off
	Browser    string   // DRIVER_BROWSER, default DefaultBrowser; the browser /api/tabs asks for open tabs
	Editor     string   // DRIVER_EDITOR: command (split on spaces) that ⌘-click runs with the job dir appended; empty = VS Code
	UserHome   string   // $HOME, for ~ expansion and pi's settings
}

// LoadConfig reads the environment (after the env file has been applied).
func LoadConfig(port int) Config {
	home := util.Home()
	cfg := Config{
		Home:     home,
		DBPath:   filepath.Join(home, "driver.db"),
		Port:     port,
		PiBin:    "pi",
		PiFlags:  []string{"-a"},
		PiMode:   "print",
		SelfExe:  "driver",
		Operator: os.Getenv("DRIVER_OPERATOR"),
		Notify:   !strings.EqualFold(os.Getenv("DRIVER_NOTIFY"), "off"),
		UserHome: util.UserHome(),
	}
	if v := os.Getenv("DRIVER_DB_PATH"); v != "" {
		cfg.DBPath = util.ExpandTilde(v)
	}
	if v := os.Getenv("DRIVER_PI"); v != "" {
		cfg.PiBin = util.ExpandTilde(v)
	}
	if v, ok := os.LookupEnv("DRIVER_PI_FLAGS"); ok {
		cfg.PiFlags = strings.Fields(v)
	}
	if strings.EqualFold(strings.TrimSpace(os.Getenv("DRIVER_PI_MODE")), "tui") {
		cfg.PiMode = "tui"
	}
	if exe, err := os.Executable(); err == nil {
		cfg.SelfExe = exe
	}
	if cfg.Operator == "" {
		cfg.Operator = os.Getenv("USER")
	}
	if cfg.Operator == "" {
		cfg.Operator = "operator"
	}
	cfg.Editor = strings.TrimSpace(os.Getenv("DRIVER_EDITOR"))
	cfg.Browser = strings.TrimSpace(os.Getenv("DRIVER_BROWSER"))
	if cfg.Browser == "" {
		cfg.Browser = DefaultBrowser
	}
	cfg.DefaultDir = util.ExpandTilde(os.Getenv("DRIVER_DEFAULT_DIR"))
	if cfg.DefaultDir == "" {
		cfg.DefaultDir = cfg.UserHome
	}
	return cfg
}

// ServeURL is the base URL jobs are told to reach the daemon at.
func (c Config) ServeURL() string { return fmt.Sprintf("http://127.0.0.1:%d", c.Port) }

// ExtPath is where `driver serve` writes the embedded pi extension.
func (c Config) ExtPath() string { return filepath.Join(c.Home, "ext", "driver.ts") }

// JobsDir holds every job dir.
func (c Config) JobsDir() string { return filepath.Join(c.Home, "jobs") }

// DaemonFile is the heartbeat file path.
func (c Config) DaemonFile() string { return filepath.Join(c.Home, "daemon.json") }
