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

/** What a link reads as: a PR's `#N`, else its label, else its host. */
export function linkLabel(link: Pick<WebLink, "kind" | "url" | "label">): string {
  if (link.kind === "github_pr") {
    return prNumber(link.url) ?? link.label ?? "PR";
  }
  return link.label || linkHost(link.url);
}

export function isHttpUrl(url: string): boolean {
  return /^https?:\/\//i.test(url);
}
