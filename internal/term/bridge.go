// Package term bridges a websocket to a pty running `tmux attach-session`
// (docs/API.md "Terminal"): binary frames carry terminal bytes both ways,
// text frames are JSON control messages — {"type":"resize"} from the client,
// {"type":"exit","code":N} from the server right before it closes.
package term

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/creack/pty"
	"github.com/gorilla/websocket"
)

// Default pty size when the client never sends a resize.
const (
	defaultCols = 120
	defaultRows = 40
)

// writeTimeout bounds every websocket write so a stuck client cannot wedge
// the pty pump.
const writeTimeout = 10 * time.Second

// firstResizeWait is how long Serve waits for the client's opening resize
// before starting the attach at the default size.
const firstResizeWait = 2 * time.Second

var upgrader = websocket.Upgrader{
	ReadBufferSize:  32 << 10,
	WriteBufferSize: 32 << 10,
	// The server binds loopback only; any page running on this Mac (the
	// WKWebView shell, the vite dev server on :5173, a browser tab) may attach.
	CheckOrigin: func(r *http.Request) bool { return isLoopback(r.RemoteAddr) },
}

func isLoopback(remote string) bool {
	host, _, err := net.SplitHostPort(remote)
	if err != nil {
		host = remote
	}
	ip := net.ParseIP(strings.Trim(host, "[]"))
	return ip != nil && ip.IsLoopback()
}

type controlMsg struct {
	Type string `json:"type"`
	Cols int    `json:"cols,omitempty"`
	Rows int    `json:"rows,omitempty"`
	Code int    `json:"code"`
}

// Serve upgrades w/r and pumps bytes between the socket and a fresh
// `tmux attach-session -t <session>` until either side ends. Closing the
// socket kills the attach process; the tmux session lives on.
func Serve(w http.ResponseWriter, r *http.Request, tmuxBin, session string) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return // Upgrade already answered
	}
	defer conn.Close()
	conn.SetReadLimit(1 << 20)

	size := &pty.Winsize{Cols: defaultCols, Rows: defaultRows}
	// the first frame is normally the opening resize; honour it before the
	// attach starts so tmux never draws at the wrong size
	var pendingInput []byte
	_ = conn.SetReadDeadline(time.Now().Add(firstResizeWait))
	if mt, data, err := conn.ReadMessage(); err == nil {
		if mt == websocket.TextMessage {
			if c, ok := parseControl(data); ok && c.Type == "resize" {
				applySize(size, c)
			}
		} else if mt == websocket.BinaryMessage {
			pendingInput = data
		}
	} else if !isTimeout(err) {
		return
	}
	_ = conn.SetReadDeadline(time.Time{})

	cmd := exec.Command(tmuxBin, "attach-session", "-t", "="+session)
	cmd.Env = append(os.Environ(), "TERM=xterm-256color", "COLORTERM=truecolor")
	ptmx, err := pty.StartWithSize(cmd, size)
	if err != nil {
		_ = writeText(conn, controlMsg{Type: "exit", Code: -1})
		log.Printf("term %s: start attach: %v", session, err)
		return
	}
	var writeMu sync.Mutex
	done := make(chan struct{})
	var once sync.Once
	finish := func() { once.Do(func() { close(done) }) }

	// pty → socket
	go func() {
		defer finish()
		buf := make([]byte, 32<<10)
		for {
			n, err := ptmx.Read(buf)
			if n > 0 {
				writeMu.Lock()
				_ = conn.SetWriteDeadline(time.Now().Add(writeTimeout))
				werr := conn.WriteMessage(websocket.BinaryMessage, buf[:n])
				writeMu.Unlock()
				if werr != nil {
					return
				}
			}
			if err != nil {
				return // EIO when the attach exits
			}
		}
	}()
	// socket → pty
	go func() {
		defer finish()
		if len(pendingInput) > 0 {
			_, _ = ptmx.Write(pendingInput)
		}
		for {
			mt, data, err := conn.ReadMessage()
			if err != nil {
				return
			}
			switch mt {
			case websocket.BinaryMessage:
				if _, err := ptmx.Write(data); err != nil {
					return
				}
			case websocket.TextMessage:
				if c, ok := parseControl(data); ok && c.Type == "resize" {
					applySize(size, c)
					_ = pty.Setsize(ptmx, size)
				}
			}
		}
	}()

	<-done
	// whichever side ended first, end the attach: a closed socket must not
	// leave a stray attach client on the session
	_ = cmd.Process.Kill()
	code := 0
	if err := cmd.Wait(); err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			code = ee.ExitCode()
		} else {
			code = -1
		}
	}
	_ = ptmx.Close()
	writeMu.Lock()
	_ = writeText(conn, controlMsg{Type: "exit", Code: code})
	_ = conn.SetWriteDeadline(time.Now().Add(writeTimeout))
	_ = conn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, fmt.Sprintf("exit %d", code)))
	writeMu.Unlock()
}

func parseControl(data []byte) (controlMsg, bool) {
	var c controlMsg
	if err := json.Unmarshal(data, &c); err != nil || c.Type == "" {
		return controlMsg{}, false
	}
	return c, true
}

func applySize(size *pty.Winsize, c controlMsg) {
	if c.Cols > 0 && c.Cols < 1000 {
		size.Cols = uint16(c.Cols)
	}
	if c.Rows > 0 && c.Rows < 1000 {
		size.Rows = uint16(c.Rows)
	}
}

func writeText(conn *websocket.Conn, c controlMsg) error {
	data, _ := json.Marshal(c)
	_ = conn.SetWriteDeadline(time.Now().Add(writeTimeout))
	return conn.WriteMessage(websocket.TextMessage, data)
}

func isTimeout(err error) bool {
	var ne net.Error
	if errors.As(err, &ne) && ne.Timeout() {
		return true
	}
	return errors.Is(err, os.ErrDeadlineExceeded)
}
