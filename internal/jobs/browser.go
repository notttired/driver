package jobs

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// DefaultBrowser is what DRIVER_BROWSER falls back to. Any Chromium-family
// browser that ships Chrome's AppleScript dictionary works in its place
// (Brave Browser, Microsoft Edge, Chromium, Vivaldi); Safari's dictionary
// differs and is not supported.
const DefaultBrowser = "Google Chrome"

// Tab is one open browser tab, as the browser reports it over Apple events.
type Tab struct {
	URL    string `json:"url"`
	Title  string `json:"title"`
	Window int    `json:"window"` // 1 = the front window
	Index  int    `json:"index"`  // 1-based position within its window
	Active bool   `json:"active"` // the window's selected tab
}

// BrowserRunning reports whether a process with exactly that name exists.
// Asking a browser that is not running would launch it, so callers check
// this first.
func BrowserRunning(name string) bool {
	return exec.Command("pgrep", "-x", name).Run() == nil
}

// tabsScript walks every window front to back and emits one record per tab:
// window, index, active, URL, title — joined by the unit separator and
// terminated by the record separator, so a title with a tab or newline in it
// cannot break the framing. Integers are coerced explicitly: AppleScript's
// `&` with a number on the left builds a list rather than a string.
const tabsScript = `tell application %s
	set us to character id 31
	set rs to character id 30
	set out to ""
	set wi to 0
	repeat with w in windows
		set wi to wi + 1
		set ai to active tab index of w
		set ti to 0
		repeat with t in tabs of w
			set ti to ti + 1
			set u to URL of t
			if u is missing value then set u to ""
			set n to title of t
			if n is missing value then set n to ""
			set out to out & (wi as string) & us & (ti as string) & us & ((ti = ai) as string) & us & u & us & n & rs
		end repeat
	end repeat
	return out
end tell`

// ListTabs asks the browser for its open http(s) tabs via osascript, front
// window first. The first call from a new responsible process makes macOS
// ask the operator to allow it; a refusal comes back as a sentence that
// names where to change that.
func ListTabs(browser string, timeout time.Duration) ([]Tab, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	var stdout, stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, "osascript", "-e", fmt.Sprintf(tabsScript, appleScriptString(browser)))
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		switch {
		case strings.Contains(msg, "-1743") || strings.Contains(msg, "Not authorized"):
			return nil, fmt.Errorf("macOS has not let driver read %s's tabs — approve the automation prompt, or allow it under System Settings › Privacy & Security › Automation", browser)
		case ctx.Err() != nil:
			return nil, fmt.Errorf("%s did not answer within %s", browser, timeout)
		case msg == "":
			return nil, fmt.Errorf("osascript: %w", err)
		default:
			return nil, fmt.Errorf("osascript: %s", strings.SplitN(msg, "\n", 2)[0])
		}
	}
	return parseTabs(stdout.String()), nil
}

// parseTabs decodes tabsScript's output, dropping anything that is not an
// http(s) page (new-tab pages, chrome:// settings, blank tabs).
func parseTabs(out string) []Tab {
	tabs := []Tab{}
	for _, rec := range strings.Split(out, "\x1e") {
		f := strings.SplitN(rec, "\x1f", 5)
		if len(f) < 5 {
			continue
		}
		url := strings.TrimSpace(f[3])
		if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
			continue
		}
		wi, _ := strconv.Atoi(strings.TrimSpace(f[0]))
		ti, _ := strconv.Atoi(strings.TrimSpace(f[1]))
		tabs = append(tabs, Tab{
			URL:    url,
			Title:  strings.TrimSpace(f[4]),
			Window: wi,
			Index:  ti,
			Active: strings.TrimSpace(f[2]) == "true",
		})
	}
	return tabs
}
