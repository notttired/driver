// Package pane is the program a job's tmux pane runs when pi is in print
// mode (DRIVER_PI_MODE=print, the default): a prompt loop around `pi -p`.
//
// pi's interactive TUI has no setting that hides the text a model emits
// between tool calls ("let me check X…"); its print mode prints only the
// text of the last assistant message — the final answer — but takes one
// prompt and exits. The loop joins the two: every line typed at its prompt
// runs pi once more in print mode on the job's session file (pi resumes the
// conversation from it), with the prompt on stdin; what pi prints is the
// answer, and the prompt comes back for the next one.
//
// The pane owns status.json between runs — ready while a fresh session waits
// for its first prompt, idle after an answer, aborted after Esc, exited when
// it closes — and ext/driver.ts reports working and the tool in flight for
// as long as pi runs (docs/DESIGN.md "State derivation"). Esc (the board's
// stop: `tmux send-keys Escape`) or ^C while pi runs sends it SIGTERM, which
// pi's print mode answers by killing its tool processes and closing the
// session cleanly; ^D at the prompt closes the pane.
package pane

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"golang.org/x/sys/unix"
	"golang.org/x/term"

	"driver/internal/pi"
)

// Options is one pane's configuration; Main fills it from the command line.
type Options struct {
	Title     string   // the job's name, for the header
	Model     string   // the model pi was given, for the header ("" = pi's default)
	Session   string   // pi's session file: a user message in it means the job is briefed (idle, not ready)
	StatusDir string   // where status.json goes (DRIVER_JOB_DIR); "" = do not write one
	Pi        []string // pi's argv, binary first, without a prompt: run once per prompt, the prompt on stdin
	In        *os.File // the terminal (a pipe in tests)
	Out       *os.File // where the pane and pi write
}

const (
	prompt = "› "
	esc    = 0x1b
	ctrlC  = 0x03
	ctrlK  = 0x0b
	ctrlU  = 0x15
)

// Main is `driver pane`: flags, then the pi command after "--".
func Main(args []string) error {
	fs := flag.NewFlagSet("pane", flag.ContinueOnError)
	title := fs.String("title", "", "the job's name, shown in the header")
	session := fs.String("session", "", "pi's session file")
	model := fs.String("model", "", "the model pi was given, shown in the header")
	fs.Usage = func() {
		fmt.Fprintln(fs.Output(), "usage: driver pane --title NAME --session FILE [--model MODEL] -- <pi> -p <pi flags>")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return err
	}
	if len(fs.Args()) == 0 {
		fs.Usage()
		return errors.New("no pi command after --")
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGHUP, syscall.SIGINT)
	defer stop()
	return Run(ctx, Options{
		Title:     *title,
		Model:     *model,
		Session:   *session,
		StatusDir: os.Getenv("DRIVER_JOB_DIR"),
		Pi:        fs.Args(),
		In:        os.Stdin,
		Out:       os.Stdout,
	})
}

// Run is the loop. It returns when the operator closes the pane (^D), the
// input ends, or ctx is cancelled (tmux killing the session sends SIGHUP).
func Run(ctx context.Context, o Options) error {
	if len(o.Pi) == 0 {
		return errors.New("pane: no pi command")
	}
	p := &pane{o: o, in: newInput(ctx, o.In)}
	fd := int(o.In.Fd())
	if term.IsTerminal(fd) {
		orig, err := unix.IoctlGetTermios(fd, getTermios)
		if err != nil {
			return fmt.Errorf("pane: tty: %w", err)
		}
		p.fd, p.orig = fd, orig
		defer p.restore()
		p.term = term.NewTerminal(struct {
			io.Reader
			io.Writer
		}{p.in, o.Out}, prompt)
	}
	p.header()
	if briefed(o.Session) {
		p.status("idle")
	} else {
		p.status("ready")
	}
	for {
		text, err := p.readPrompt()
		if err != nil {
			p.status("exited")
			if errors.Is(err, io.EOF) || ctx.Err() != nil {
				p.printf("\n%sclosed%s\n", p.dim(), p.reset())
				return nil
			}
			return err
		}
		if strings.TrimSpace(text) == "" {
			continue
		}
		p.status("working")
		p.printf("  %s⋯ pi is working — Esc stops it%s\n", p.dim(), p.reset())
		res := p.runPi(text)
		switch {
		case res.aborted:
			p.printf("  %s■ stopped%s\n", p.dim(), p.reset())
			p.status("aborted")
		case res.err != nil:
			p.printf("  %s✗ %v%s\n", p.dim(), res.err, p.reset())
			p.status("idle")
		case res.code != 0:
			p.printf("  %s✗ pi exited with status %d%s\n", p.dim(), res.code, p.reset())
			p.status("idle")
		default:
			p.status("idle")
		}
		p.printf("\n")
		if ctx.Err() != nil {
			p.status("exited")
			return nil
		}
	}
}

type pane struct {
	o    Options
	in   *input
	fd   int
	orig *unix.Termios // the tty's state at start; nil when In is not a tty
	term *term.Terminal
}

func (p *pane) tty() bool { return p.orig != nil }

func (p *pane) dim() string {
	if p.tty() {
		return "\x1b[2m"
	}
	return ""
}

func (p *pane) bold() string {
	if p.tty() {
		return "\x1b[1m"
	}
	return ""
}

func (p *pane) reset() string {
	if p.tty() {
		return "\x1b[0m"
	}
	return ""
}

func (p *pane) printf(format string, a ...any) { fmt.Fprintf(p.o.Out, format, a...) }

// restore puts the tty back the way the pane found it (cooked).
func (p *pane) restore() {
	if p.tty() {
		_ = unix.IoctlSetTermios(p.fd, setTermios, p.orig)
	}
}

// cbreak is the mode pi runs under: keys arrive at once and unechoed (so Esc
// and ^C reach watch, and typing is not echoed over pi's output) while output
// processing stays on (pi's newlines need it). ^C is a byte here, not SIGINT:
// pi is stopped with SIGTERM, which its print mode handles.
func (p *pane) cbreak() error {
	t := *p.orig
	t.Lflag &^= unix.ICANON | unix.ECHO | unix.ECHONL | unix.ISIG | unix.IEXTEN
	t.Cc[unix.VMIN] = 1
	t.Cc[unix.VTIME] = 0
	return unix.IoctlSetTermios(p.fd, setTermios, &t)
}

func (p *pane) header() {
	dir, _ := os.Getwd()
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		if dir == home {
			dir = "~"
		} else if strings.HasPrefix(dir, home+"/") {
			dir = "~" + strings.TrimPrefix(dir, home)
		}
	}
	model := p.o.Model
	if model == "" {
		model = "pi's default model"
	}
	p.printf("%s%s%s  %s%s · %s%s\n", p.bold(), p.o.Title, p.reset(), p.dim(), dir, model, p.reset())
	p.printf("%spi -p — final answers only · Enter sends · Esc stops pi · ^D closes%s\n\n", p.dim(), p.reset())
}

// status writes status.json the way ext/driver.ts does, so the daemon reads
// one file whoever wrote it last.
func (p *pane) status(state string) {
	if p.o.StatusDir == "" {
		return
	}
	_ = pi.WriteStatus(filepath.Join(p.o.StatusDir, "status.json"), pi.Status{
		State:   state,
		PID:     os.Getpid(),
		Session: p.o.Session,
		TS:      time.Now().UnixMilli(),
	})
}

// readPrompt reads one prompt: a line edited at the terminal (history on
// ↑/↓; a bracketed paste of several lines stays one prompt), or a line from
// a pipe. io.EOF is ^D on an empty line or the input closing.
func (p *pane) readPrompt() (string, error) {
	if !p.tty() {
		return p.in.readLine()
	}
	if w, h, err := term.GetSize(p.fd); err == nil {
		_ = p.term.SetSize(w, h) // x/term holds its lock through ReadLine, so the size is taken here, not on SIGWINCH
	}
	if _, err := term.MakeRaw(p.fd); err != nil {
		return "", fmt.Errorf("raw mode: %w", err)
	}
	defer p.restore()
	p.term.SetBracketedPasteMode(true)
	defer p.term.SetBracketedPasteMode(false)
	var parts []string
	for {
		line, err := p.term.ReadLine()
		parts = append(parts, line)
		if errors.Is(err, term.ErrPasteIndicator) {
			continue // a pasted line with more of the paste to come
		}
		if err != nil {
			return "", err
		}
		return strings.Join(parts, "\n"), nil
	}
}

type result struct {
	aborted bool  // stopped by Esc/^C or the pane shutting down
	code    int   // pi's exit status (-1: killed)
	err     error // pi could not be run at all
}

// runPi runs the pi command once with text as its prompt and waits for it,
// watching the input for the stop keys meanwhile.
func (p *pane) runPi(text string) result {
	if p.tty() {
		if err := p.cbreak(); err != nil {
			return result{err: fmt.Errorf("tty: %w", err)}
		}
		defer p.restore()
	}
	cmd := exec.Command(p.o.Pi[0], p.o.Pi[1:]...)
	cmd.Stdin = strings.NewReader(text + "\n")
	cmd.Stdout = p.o.Out
	cmd.Stderr = p.o.Out
	cmd.WaitDelay = 3 * time.Second
	if err := cmd.Start(); err != nil {
		return result{err: err}
	}
	done := make(chan struct{})
	var waitErr error
	go func() {
		waitErr = cmd.Wait()
		close(done)
	}()
	res := result{}
	if p.in.watch(done) != finished {
		res.aborted = true
		_ = cmd.Process.Signal(syscall.SIGTERM)
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			_ = cmd.Process.Kill()
			<-done
		}
	}
	<-done
	if cmd.ProcessState != nil {
		res.code = cmd.ProcessState.ExitCode()
	}
	var exit *exec.ExitError
	if waitErr != nil && !errors.As(waitErr, &exit) {
		res.err = waitErr
	}
	return res
}

// briefed reports whether the session file holds a user message: the job
// has been spoken to, so the pane opens idle rather than ready. A file that
// exists but cannot be read counts as briefed, the louder state (ext/driver.ts
// reads an unknown the same way).
func briefed(session string) bool {
	f, err := os.Open(session)
	if err != nil {
		return !errors.Is(err, os.ErrNotExist)
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 64<<20)
	for sc.Scan() {
		if e, ok := pi.ParseEntry(sc.Bytes()); ok && e.Kind == "message" && e.Role == "user" {
			return true
		}
	}
	return sc.Err() != nil
}

// input is the pane's one reader of its stdin. x/term reads prompts through
// Read; readLine serves a pipe; watch, while pi runs, looks at what arrives
// for the stop keys and drops the rest (typing during a run goes nowhere).
type input struct {
	ctx     context.Context
	ch      chan []byte
	pending []byte // read from ch but not yet consumed
}

func newInput(ctx context.Context, r io.Reader) *input {
	in := &input{ctx: ctx, ch: make(chan []byte, 64)}
	go func() {
		defer close(in.ch)
		for {
			buf := make([]byte, 4096)
			n, err := r.Read(buf)
			if n > 0 {
				in.ch <- buf[:n]
			}
			if err != nil {
				return
			}
		}
	}()
	return in
}

// next is the next chunk: pending bytes first. io.EOF once the input has
// closed, the context's error once it is cancelled.
func (in *input) next() ([]byte, error) {
	if len(in.pending) > 0 {
		b := in.pending
		in.pending = nil
		return b, nil
	}
	select {
	case <-in.ctx.Done():
		return nil, in.ctx.Err()
	case b, ok := <-in.ch:
		if !ok {
			return nil, io.EOF
		}
		return b, nil
	}
}

// Read serves x/term. ^C becomes ^U ^K — clear the line — rather than the
// end of input x/term would make of it.
func (in *input) Read(p []byte) (int, error) {
	b, err := in.next()
	if err != nil {
		return 0, err
	}
	b = bytes.ReplaceAll(b, []byte{ctrlC}, []byte{ctrlU, ctrlK})
	n := copy(p, b)
	if n < len(b) {
		in.pending = append(b[n:len(b):len(b)], in.pending...)
	}
	return n, nil
}

// readLine is the pipe prompt: bytes up to a newline.
func (in *input) readLine() (string, error) {
	var line []byte
	for {
		b, err := in.next()
		if err != nil {
			if errors.Is(err, io.EOF) && len(line) > 0 {
				return string(line), nil // a last line without its newline
			}
			return "", err
		}
		if i := bytes.IndexByte(b, '\n'); i >= 0 {
			line = append(line, b[:i]...)
			in.pending = append([]byte{}, b[i+1:]...)
			return strings.TrimRight(string(line), "\r"), nil
		}
		line = append(line, b...)
	}
}

type stopReason int

const (
	finished stopReason = iota // pi exited on its own
	stopKey                    // Esc or ^C arrived
	stopCtx                    // the pane is shutting down
)

// watch waits for pi (done) while reading the input for a stop key; bytes
// already pending stay for the next prompt.
func (in *input) watch(done <-chan struct{}) stopReason {
	ch := in.ch
	for {
		select {
		case <-done:
			return finished
		case <-in.ctx.Done():
			return stopCtx
		case b, ok := <-ch:
			if !ok {
				ch = nil // input gone: nothing more can stop pi; wait for it
				continue
			}
			if isStop(b) {
				return stopKey
			}
		}
	}
}

// isStop: a chunk that is nothing but Escape bytes (the key, or tmux's
// `send-keys Escape`), or one carrying ^C.
func isStop(b []byte) bool {
	if len(b) == 0 {
		return false
	}
	if bytes.IndexByte(b, ctrlC) >= 0 {
		return true
	}
	return bytes.Count(b, []byte{esc}) == len(b)
}
