import type { Column } from "./columns.ts";
import { bucketJobs } from "./format.ts";
import type { WebJob } from "./types.ts";

/** A column's member slugs in server order (archived entries never count). */
export function columnSlugs(jobs: readonly WebJob[], column: Column): string[] {
  return bucketJobs(jobs).byColumn[column].map((j) => j.slug);
}

/**
 * What the board payload will look like once the server has applied a
 * same-column reorder: the column's members in the posted order, every other
 * position untouched, the same job OBJECTS. Null when the posted order and the
 * client's picture of the column disagree in any way — the caller then falls
 * back to a plain refetch rather than painting a guess.
 */
export function predictReorderedJobs(
  jobs: readonly WebJob[],
  column: Column,
  slugs: readonly string[],
): WebJob[] | null {
  const members = bucketJobs(jobs).byColumn[column];

  const bySlug = new Map<string, WebJob>();
  for (const j of members) bySlug.set(j.slug, j);

  const reordered: WebJob[] = [];
  for (const s of slugs) {
    const j = bySlug.get(s);
    if (j) reordered.push(j);
  }
  if (reordered.length !== slugs.length) return null;
  if (members.length !== reordered.length) return null;

  const memberSet = new Set<WebJob>(members);
  let i = 0;
  const out: WebJob[] = [];
  for (const j of jobs) {
    if (memberSet.has(j)) out.push(reordered[i++] as WebJob);
    else out.push(j);
  }
  return out;
}
