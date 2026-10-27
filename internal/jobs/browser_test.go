package jobs

import "testing"

func TestParseTabs(t *testing.T) {
	out := "1\x1f1\x1ffalse\x1fhttps://github.com/o/r/pull/7\x1fFix login · PR #7\x1e" +
		"1\x1f2\x1ftrue\x1fhttps://shopify.slack.com/archives/C1/p2\x1fthread\twith\ttabs\nand a newline\x1e" +
		"1\x1f3\x1ffalse\x1fchrome://newtab/\x1fNew Tab\x1e" +
		"2\x1f1\x1ftrue\x1f\x1f\x1e" + // blank tab: missing URL and title
		"\n" // osascript's trailing newline
	tabs := parseTabs(out)
	if len(tabs) != 2 {
		t.Fatalf("want 2 http tabs, got %d: %+v", len(tabs), tabs)
	}
	if tabs[0].URL != "https://github.com/o/r/pull/7" || tabs[0].Title != "Fix login · PR #7" || tabs[0].Window != 1 || tabs[0].Index != 1 || tabs[0].Active {
		t.Errorf("tab 0: %+v", tabs[0])
	}
	if !tabs[1].Active || tabs[1].Index != 2 || tabs[1].Title != "thread\twith\ttabs\nand a newline" {
		t.Errorf("tab 1: %+v", tabs[1])
	}
	if got := parseTabs(""); len(got) != 0 || got == nil {
		t.Errorf("empty output should give an empty, non-nil slice: %#v", got)
	}
}
