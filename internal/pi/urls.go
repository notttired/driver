package pi

import (
	"net/url"
	"regexp"
	"strings"
)

// urlRe matches an http(s) URL up to the first character that cannot be part
// of one in running text: whitespace, angle brackets, quotes.
var urlRe = regexp.MustCompile("(?i)https?://[^\\s<>\"'`]+")

// DetectURLs returns every distinct http(s) URL in text, in first-seen order.
// The punctuation prose hangs on a URL's end is stripped; a closing bracket
// stays only when the URL opened it (wiki-style `…/Foo_(bar)` keeps its
// parenthesis, a Markdown `(https://…)` loses the one that is not its own).
// Anything without a host (`https://` alone) is dropped.
func DetectURLs(text string) []string {
	seen := map[string]bool{}
	var out []string
	for _, m := range urlRe.FindAllString(text, -1) {
		m = trimURL(m)
		if seen[m] {
			continue
		}
		if u, err := url.Parse(m); err != nil || u.Host == "" {
			continue
		}
		seen[m] = true
		out = append(out, m)
	}
	return out
}

var brackets = [...][2]string{{"(", ")"}, {"[", "]"}, {"{", "}"}}

// trimURL peels trailing sentence punctuation and unbalanced closing
// brackets, repeating until nothing more comes off (`…/a).` loses both).
func trimURL(u string) string {
	for {
		t := strings.TrimRight(u, ".,;:!?*")
		for _, b := range brackets {
			for strings.HasSuffix(t, b[1]) && strings.Count(t, b[0]) < strings.Count(t, b[1]) {
				t = t[:len(t)-1]
			}
		}
		if t == u {
			return u
		}
		u = t
	}
}
