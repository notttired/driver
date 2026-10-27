package pi

import (
	"regexp"
	"strings"
)

var prURLRe = regexp.MustCompile(`https://github\.com/[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+/pull/\d+`)

// prURLExact matches a whole URL (optionally with a trailing slash) — the
// form the links API and pr-attach accept.
var prURLExact = regexp.MustCompile(`^https://github\.com/[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+/pull/\d+/?$`)

// DetectPRURLs returns every distinct https://github.com/<owner>/<repo>/pull/<n>
// in text, in first-seen order, with trailing punctuation stripped (the regex
// stops at the number, so "…/pull/12." already yields "…/pull/12").
func DetectPRURLs(text string) []string {
	seen := map[string]bool{}
	var out []string
	for _, m := range prURLRe.FindAllString(text, -1) {
		m = strings.TrimRight(m, ".,;:!?)]}'\"")
		if seen[m] {
			continue
		}
		seen[m] = true
		out = append(out, m)
	}
	return out
}

// IsPRURL reports whether url is exactly a GitHub pull-request URL.
func IsPRURL(url string) bool { return prURLExact.MatchString(strings.TrimSpace(url)) }

// IsPRCreateCommand reports whether a bash command creates a PR with gh —
// `gh pr create` anywhere in it (chained commands included).
func IsPRCreateCommand(cmd string) bool {
	return strings.Contains(strings.Join(strings.Fields(cmd), " "), "gh pr create")
}
