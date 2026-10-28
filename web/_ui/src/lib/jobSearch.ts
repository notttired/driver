import { dirBase } from "./format.ts";
import type { WebJob } from "./types.ts";

export function parseTerms(query: string): readonly string[] {
  return query.trim().toLowerCase().split(/\s+/).filter(Boolean);
}

/** Subsequence match: every character of `term` appears in `haystack`, in order. */
export function fuzzyMatch(haystack: string, term: string): boolean {
  let i = 0;
  for (let h = 0; h < haystack.length && i < term.length; h++) {
    if (haystack[h] === term[i]) i++;
  }
  return i === term.length;
}

/** The searchable surface of a job: its slug (and original slug when
 *  archived), the name you gave it, plus its directory. Notes are
 *  deliberately not searched. */
export function searchHaystack(job: Pick<WebJob, "slug" | "display_slug" | "name" | "dir">): string {
  const parts = [job.slug];
  if (job.display_slug && job.display_slug !== job.slug) parts.push(job.display_slug);
  if (job.name) parts.push(job.name);
  if (job.dir) {
    parts.push(job.dir);
    parts.push(dirBase(job.dir));
  }
  return parts.join(" ").toLowerCase();
}

export function matchJob(job: WebJob, terms: readonly string[]): boolean {
  if (terms.length === 0) return true;
  const hay = searchHaystack(job);
  return terms.every((t) => fuzzyMatch(hay, t));
}
