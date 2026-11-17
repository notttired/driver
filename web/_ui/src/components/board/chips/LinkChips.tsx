import { isHttpUrl, KIND_GLYPH, linkLabel, prStatusClass } from "../../../lib/links.ts";
import type { WebLink } from "../../../lib/types.ts";

/** How many links a card shows: as many as the overlay's links panel has
 *  rows; the rest are a count. */
export const MAX_CHIPS = 5;

/** One chip per link on a card — the first MAX_CHIPS in display order
 *  (pinned first, then newest), then "+N" for the rest: a PR reads `#N` in
 *  its status colour, every other kind its host behind a kind glyph.
 *  Anchors stop the click so the card underneath does not open. */
export function LinkChips({ links, onOpen }: { links: WebLink[] | undefined; onOpen?: (url: string) => void }) {
  if (!links?.length) return null;
  const rest = links.length - MAX_CHIPS;
  return (
    <div className="prs">
      {links.slice(0, MAX_CHIPS).map((l) => (
        <LinkChip key={l.id} link={l} onOpen={onOpen} />
      ))}
      {rest > 0 ? (
        <span className="pr-chip pr-more" title={`${rest} more ${rest === 1 ? "link" : "links"} — open the job to see them all`}>
          +{rest}
        </span>
      ) : null}
    </div>
  );
}

export function LinkChip({ link, onOpen }: { link: WebLink; onOpen?: (url: string) => void }) {
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
      onClick={(e) => {
        e.stopPropagation();
        onOpen?.(link.url);
      }}
    >
      {body}
    </a>
  );
}
