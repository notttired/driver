package jobs

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBrowserContextText(t *testing.T) {
	ctx := BrowserContext{URL: "https://example.com/doc", Title: "A doc", Text: "  the body  \n", Selected: false}
	got := ctx.Prompt()
	want := "Context from my browser — A doc\nhttps://example.com/doc\n\nthe body"
	if got != want {
		t.Fatalf("prompt:\n%q\nwant\n%q", got, want)
	}
	sel := BrowserContext{URL: "https://example.com/doc", Title: "A doc", Text: "just this", Selected: true}
	if got := sel.Prompt(); !strings.HasPrefix(got, "Context from my browser (selected text) — A doc\n") || !strings.HasSuffix(got, "\n\njust this") {
		t.Fatalf("selected: %q", got)
	}
	bare := BrowserContext{URL: "https://example.com/doc", Title: "A doc"}
	if got := bare.Prompt(); got != "Context from my browser — A doc\nhttps://example.com/doc" {
		t.Fatalf("no text: %q", got)
	}
	long := BrowserContext{URL: "u", Title: "t", Text: strings.Repeat("x", MaxContextChars+500)}
	if got := long.Prompt(); !strings.HasSuffix(got, "\n\n(cut at "+itoa(MaxContextChars)+" characters)") || len(got) > MaxContextChars+200 {
		t.Fatalf("cut: %d chars, ends %q", len(got), got[len(got)-60:])
	}
}

func TestParseBrowserContext(t *testing.T) {
	out := "https://x.io/p\x1fPage title\x1ftrue\x1fselected words\x1e"
	ctx, ok := parseBrowserContext(out)
	if !ok || ctx.URL != "https://x.io/p" || ctx.Title != "Page title" || !ctx.Selected || ctx.Text != "selected words" {
		t.Fatalf("parsed: %+v %v", ctx, ok)
	}
	// JavaScript off in the browser: the script reports the page without text
	ctx, ok = parseBrowserContext("https://x.io/p\x1fPage title\x1f\x1f\x1e")
	if !ok || ctx.Text != "" || ctx.Selected {
		t.Fatalf("no text: %+v %v", ctx, ok)
	}
	if _, ok := parseBrowserContext("chrome://newtab\x1f\x1f\x1f\x1e"); ok {
		t.Fatal("a non-web page is no context")
	}
}

func TestPaste(t *testing.T) {
	svc, st, work := threadsFixture(t)
	j, err := svc.Create("p", "", work, "")
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.Paste(j, nil, "hello"); !IsRefusal(err) {
		t.Fatalf("no pane yet: %v", err)
	}
	if err := svc.Submit(j); err != nil {
		t.Fatal(err)
	}
	j, _ = st.GetJobBySlug("p")
	jobDir := filepath.Dir(j.PiSession)
	waitFor(t, "pi to start", func() bool { return readFile(filepath.Join(jobDir, "argv")) != "" })
	if err := svc.Paste(j, nil, "  "); !IsRefusal(err) {
		t.Fatalf("nothing to paste: %v", err)
	}
	if err := svc.Paste(j, nil, "two\nlines"); err != nil {
		t.Fatal(err)
	}
	// the fake pi reads its tty line by line: a paste with no Enter leaves
	// the last line unsent, so only the first arrives — proof nothing was
	// submitted
	waitFor(t, "the first line to land", func() bool { return strings.Contains(readFile(filepath.Join(jobDir, "typed")), "two\n") })
	if typed := readFile(filepath.Join(jobDir, "typed")); strings.Contains(typed, "lines\n") {
		t.Fatalf("the paste was sent: %q", typed)
	}
	th, err := svc.OpenThread(j, 0, "side")
	if err != nil {
		t.Fatal(err)
	}
	waitFor(t, "the thread's pi to start", func() bool { return readFile(filepath.Join(filepath.Dir(th.PiSession), "argv")) != "" })
	if err := svc.Paste(j, th, "to the thread\n"); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "the thread to get it", func() bool {
		return strings.Contains(readFile(filepath.Join(filepath.Dir(th.PiSession), "typed")), "to the thread")
	})
	svc.Tmux.KillSession(th.Tmux)
	if err := svc.Paste(j, th, "x"); !IsRefusal(err) {
		t.Fatalf("a dead pane: %v", err)
	}
}

func TestProbeFolders(t *testing.T) {
	home := t.TempDir()
	os.MkdirAll(filepath.Join(home, "Desktop"), 0o755)
	os.MkdirAll(filepath.Join(home, "Downloads"), 0o000)
	t.Cleanup(func() { os.Chmod(filepath.Join(home, "Downloads"), 0o755) })
	got := ProbeFolders(home)
	if len(got) != 3 || got[0].Name != "Desktop" || got[0].State != "ok" || got[1].Name != "Documents" || got[1].State != "missing" || got[2].Name != "Downloads" || got[2].State != "denied" {
		t.Fatalf("probe: %+v", got)
	}
}
