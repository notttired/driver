import type { LinkKind, WebLink } from "./types.ts";

const PR_NUMBER_RE = /\/pull\/(\d+)(?:[/?#]|$)/;

/** `#N` for a GitHub PR URL, null for anything else. */
export function prNumber(url: string): string | null {
  const m = PR_NUMBER_RE.exec(url);
  return m ? `#${m[1]}` : null;
}

/** The chip class carrying a PR's state — colour is the whole distinction. */
export function prStatusClass(status: WebLink["status"]): string {
  switch (status) {
    case "open":
      return "pr-open";
    case "draft":
      return "pr-draft";
    case "merged":
      return "pr-merged";
    case "closed":
      return "pr-closed";
    default:
      return "pr-unknown";
  }
}

export const KIND_GLYPH: Readonly<Record<LinkKind, string>> = {
  github_pr: "",
  github: "⑂",
  slack: "#",
  linear: "◆",
  url: "↗",
};

export function linkHost(url: string): string {
  try {
    return new URL(url).host.replace(/^www\./, "");
  } catch {
    return url;
  }
}

/** What a link reads as: a PR's `#N`, anything else its host. Links carry no labels. */
export function linkLabel(link: Pick<WebLink, "kind" | "url">): string {
  if (link.kind === "github_pr") {
    return prNumber(link.url) ?? "PR";
  }
  return linkHost(link.url);
}

const URL_RE = /https?:\/\/[^\s<>"'`]+/gi;
const OPENER: Readonly<Record<string, string>> = { ")": "(", "]": "[", "}": "{" };

const count = (s: string, ch: string) => s.split(ch).length - 1;

/** Peel the punctuation prose hangs on a URL's end — sentence marks and
 *  closing brackets the URL did not open — the way the daemon's scanner does
 *  (`…/a).` loses both; a wiki-style `…/Foo_(bar)` keeps its parenthesis). */
export function trimUrl(url: string): string {
  for (;;) {
    let t = url.replace(/[.,;:!?*]+$/, "");
    for (;;) {
      const last = t.at(-1) ?? "";
      const opener = OPENER[last];
      if (!opener || count(t, opener) >= count(t, last)) break;
      t = t.slice(0, -1);
    }
    if (t === url) return url;
    url = t;
  }
}

export interface TextPart {
  text: string;
  /** set when this part is a URL */
  url?: string;
}

/** Split running text into its plain runs and the http(s) URLs in it, in order. */
export function splitUrls(text: string): TextPart[] {
  const parts: TextPart[] = [];
  let at = 0;
  for (const m of text.matchAll(URL_RE)) {
    const start = m.index ?? 0;
    const url = trimUrl(m[0] ?? "");
    if (!url) continue;
    if (start > at) parts.push({ text: text.slice(at, start) });
    parts.push({ text: url, url });
    at = start + url.length;
  }
  if (at < text.length) parts.push({ text: text.slice(at) });
  return parts;
}

export function isHttpUrl(url: string): boolean {
  return /^https?:\/\//i.test(url);
}

/**
 * Open a URL outside the board — a new tab in a browser; in Driver.app the
 * shell routes `window.open` to the default browser. The URL must ride in
 * the call itself: xterm's link addons by default open an EMPTY window and
 * then set its location, which reaches the shell as about:blank and ends in
 * a LaunchServices "-50" alert. Only http(s) goes out.
 */
export function openExternal(url: string): boolean {
  if (!isHttpUrl(url)) return false;
  window.open(url, "_blank", "noopener,noreferrer");
  return true;
}

/** Terminal convention: a URL in the pane opens on ⌘-click (ctrl elsewhere),
 *  so a click that only focuses the pane never launches a browser. */
export function isOpenClick(e: Pick<MouseEvent, "metaKey" | "ctrlKey">): boolean {
  return e.metaKey || e.ctrlKey;
}
