import { isHttpUrl, KIND_GLYPH, linkLabel, prStatusClass } from "../../../lib/links.ts";
import type { WebLink } from "../../../lib/types.ts";

/** One chip per link on a card: a PR reads `#N` in its status colour, every
 *  other kind its host behind a kind glyph. Anchors stop the click so the
 *  card underneath does not open. */
export function LinkChips({ links }: { links: WebLink[] | undefined }) {
  if (!links?.length) return null;
  return (
    <div className="prs">
      {links.map((l) => (
        <LinkChip key={l.id} link={l} />
      ))}
    </div>
  );
}

export function LinkChip({ link }: { link: WebLink }) {
  const label = linkLabel(link);
  const glyph = KIND_GLYPH[link.kind] ?? KIND_GLYPH.url;
  const cls =
    "pr-chip " + (link.kind === "github_pr" ? prStatusClass(link.status) : `kind-${link.kind}`);
  const title = link.kind === "github_pr" && link.status ? `${link.status} · ${link.url}` : link.url;
  const body = (
    <>
      {glyph ? <span className="kind-glyph">{glyph}</span> : null}
      {label}
    </>
  );
  if (!isHttpUrl(link.url)) {
    return (
      <span className={cls} title={title}>
        {body}
      </span>
    );
  }
  return (
    <a
      className={cls}
      href={link.url}
      target="_blank"
      rel="noreferrer"
      title={title}
      data-no-drag
      onClick={(e) => e.stopPropagation()}
    >
      {body}
    </a>
  );
}
