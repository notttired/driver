// Command driver is the job board's daemon: `driver serve [-p N]` runs the
// tick loop and the web board on 127.0.0.1:<port>; `driver pane … -- <pi> -p …`
// is the prompt loop a job's tmux pane runs around pi's print mode
// (internal/pane); `driver version` prints the build version.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"syscall"

	"driver/ext"
	"driver/internal/api"
	"driver/internal/jobs"
	"driver/internal/pane"
	"driver/internal/store"
	"driver/internal/util"
	"driver/web"
)

// version is stamped by the Makefile: -ldflags "-X main.version=…".
var version = "dev"

const defaultPort = 7781

func main() {
	log.SetFlags(log.Ldate | log.Ltime)
	log.SetOutput(os.Stderr)
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	switch os.Args[1] {
	case "serve":
		if err := serve(os.Args[2:]); err != nil {
			log.Fatalf("driver: %v", err)
		}
	case "pane":
		if err := pane.Main(os.Args[2:]); err != nil {
			fmt.Fprintf(os.Stderr, "driver pane: %v\n", err)
			os.Exit(1)
		}
	case "version", "-v", "--version":
		fmt.Println(version)
	case "help", "-h", "--help":
		usage()
	default:
		fmt.Fprintf(os.Stderr, "driver: unknown command %q\n", os.Args[1])
		usage()
		os.Exit(2)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: driver serve [-p PORT]   (also DRIVER_PORT; default 7781)\n"+
		"       driver pane --title NAME --session FILE [--model MODEL] -- <pi> -p <flags>   (what a job's tmux pane runs)\n"+
		"       driver version")
}

func serve(args []string) error {
	// the env file first: it may set DRIVER_PORT, DRIVER_HOME, PATH, …
	if applied := util.ApplyEnvFile(util.EnvFilePath()); len(applied) > 0 {
		log.Printf("env file %s: set %v", util.EnvFilePath(), applied)
	}
	port := defaultPort
	if v := os.Getenv("DRIVER_PORT"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n <= 0 || n > 65535 {
			return fmt.Errorf("DRIVER_PORT=%q is not a port", v)
		}
		port = n
	}
	fs := flag.NewFlagSet("serve", flag.ContinueOnError)
	fs.IntVar(&port, "p", port, "port to listen on (127.0.0.1 only)")
	fs.IntVar(&port, "port", port, "port to listen on (127.0.0.1 only)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	cfg := jobs.LoadConfig(port)

	// bind first: an occupied port must fail before anything else starts
	ln, err := web.Listen(port)
	if err != nil {
		return err
	}
	for _, d := range []string{cfg.Home, cfg.JobsDir(), filepath.Dir(cfg.ExtPath())} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return fmt.Errorf("mkdir %s: %w", d, err)
		}
	}
	// the pi extension on disk always matches the running binary
	if err := os.WriteFile(cfg.ExtPath(), ext.DriverTS, 0o644); err != nil {
		return fmt.Errorf("write %s: %w", cfg.ExtPath(), err)
	}
	st, err := store.Open(cfg.DBPath)
	if err != nil {
		return err
	}
	defer st.Close()

	svc := jobs.NewService(st, cfg)
	if !svc.Tmux.Available() {
		log.Printf("warning: tmux not found on PATH — jobs cannot start (set PATH in %s)", util.EnvFilePath())
	}
	daemon := jobs.NewDaemon(svc)
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	go daemon.Run(ctx)

	a := api.New(svc, daemon, version)
	h, err := web.Handler(a)
	if err != nil {
		return err
	}
	log.Printf("driver %s serving http://127.0.0.1:%d (home %s, db %s, pi %s in %s mode)", version, port, cfg.Home, cfg.DBPath, cfg.PiBin, cfg.PiMode)
	errc := make(chan error, 1)
	go func() { errc <- web.Serve(ln, h) }()
	select {
	case err := <-errc:
		return err
	case <-ctx.Done():
		log.Printf("driver: stopping")
		ln.Close()
		return nil
	}
}
