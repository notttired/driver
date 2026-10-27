import type { WebJob } from "./types.ts";

/**
 * The board's identity for a job. Archived jobs ride the same `jobs` array as
 * live ones and may REUSE a live job's slug (API.md: slugs are unique among
 * non-archived jobs only), so a bare slug is not a key. Every React key, every
 * dnd-kit id and every "which job is open" lookup goes through this.
 */
export const ARCHIVE_KEY_PREFIX = "archive:";

export function jobKey(j: Pick<WebJob, "slug" | "archived">): string {
  return j.archived ? ARCHIVE_KEY_PREFIX + j.slug : j.slug;
}

/** The API slug behind a board key (an archived job's key carries a prefix;
 *  its slug is the archive name the routes take). */
export function slugOfKey(key: string): string {
  return key.startsWith(ARCHIVE_KEY_PREFIX) ? key.slice(ARCHIVE_KEY_PREFIX.length) : key;
}
