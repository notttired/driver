import { laneId } from "./columns.ts";
import { columnOf, dirBase, jobTitle } from "./format.ts";
import { jobKey } from "./jobKey.ts";
import { fuzzyMatch, parseTerms } from "./jobSearch.ts";
import { statusToken, threadStatusToken, type StatusToken } from "./status.ts";
import type { WebJob, WebLane, WebThread } from "./types.ts";

/** One row of the command palette: a job, or one of its threads. */
export interface PaletteItem {
  /** jobKey(), with `#t<id>` for a thread */
  key: string;
  jobKey: string;
  slug: string;
  /** 0: the job itself; else the thread to open it on */
  thread: number;
  /** the job's name (or slug); "<job> · <thread>" for a thread */
  title: string;
  /** the slug when the title is a name, and the directory's last piece */
  subtitle: string;
  token: StatusToken;
  /** the column the board shows it in: a state, a lane's name, ARCHIVED */
  column: string;
  /** where a match outside the title came from (a link, a dir, a model…) */
  why?: string;
  score: number;
}

/** The field weights: what a word is worth when found in each. */
const W = {
  titleExact: 150,
  titlePrefix: 120,
  title: 100,
  slug: 90,
  threadName: 90,
  dirBase: 60,
  column: 50,
  status: 50,
  dir: 40,
  model: 40,
  link: 35,
  tool: 30,
  fuzzy: 20,
} as const;

const ARCHIVED_TOKEN: StatusToken = { text: "■ archived", variant: "done" };

const MAX_ITEMS = 60;

/**
 * Every job and thread on the board as the palette lists them — in board
 * order (archived last) when `query` is empty, else those every word of the
 * query is found in, best first. A word counts where it is found: the
 * title or slug most (a prefix more, the whole more still), then thread
 * names, the directory, the column or lane, the status words ("needs you",
 * "working", "done", "archived"…), the model, a link, the tool pi is in; a
 * title the word only abbreviates (its initials, or letters in order within
 * one of its words) counts least.
 */
export function searchPalette(jobs: readonly WebJob[], lanes: readonly WebLane[], query: string, limit = MAX_ITEMS): PaletteItem[] {
  const terms = parseTerms(query);
  const laneName = new Map(lanes.map((l) => [l.id, l.name]));
  const ordered = [...jobs.filter((j) => !j.archived), ...jobs.filter((j) => j.archived)];
  const items: PaletteItem[] = [];
  for (const j of ordered) {
    const col = columnLabel(j, laneName);
    const own = itemOf(j, null, col, terms);
    if (own) items.push(own);
    for (const t of j.threads ?? []) {
      if (t.id === 0) continue;
      const row = itemOf(j, t, col, terms);
      if (row) items.push(row);
    }
  }
  if (terms.length > 0) items.sort((a, b) => b.score - a.score);
  return items.slice(0, limit);
}

function columnLabel(j: WebJob, laneName: Map<number, string>): string {
  if (j.archived) return "ARCHIVED";
  const col = columnOf(j);
  const lane = laneId(col);
  if (lane !== null) return laneName.get(lane) ?? "LANE";
  return col.toUpperCase();
}

function itemOf(j: WebJob, t: WebThread | null, column: string, terms: readonly string[]): PaletteItem | null {
  const name = jobTitle(j);
  const title = t ? `${name} · ${t.name}` : name;
  const token = j.archived ? ARCHIVED_TOKEN : t ? threadStatusToken(t) : statusToken(j);
  const fields = fieldsOf(j, t, column, token);
  let score = 0;
  let why: string | undefined;
  for (const term of terms) {
    const hit = bestField(fields, title, j, term);
    if (!hit) return null;
    score += hit.score;
    if (hit.why && !why) why = hit.why;
  }
  return {
    key: t ? `${jobKey(j)}#t${t.id}` : jobKey(j),
    jobKey: jobKey(j),
    slug: j.slug,
    thread: t?.id ?? 0,
    title,
    subtitle: [name === j.slug || j.archived ? "" : j.display_slug || j.slug, dirBase(j.dir)].filter(Boolean).join(" · "),
    token,
    column,
    why,
    score,
  };
}

interface Field {
  text: string;
  weight: number;
  /** shown as the reason when this field made the match */
  why?: string;
}

function fieldsOf(j: WebJob, t: WebThread | null, column: string, token: StatusToken): Field[] {
  const out: Field[] = [{ text: j.slug, weight: W.slug }];
  if (j.display_slug) out.push({ text: j.display_slug, weight: W.slug });
  if (t) out.push({ text: t.name, weight: W.threadName });
  if (j.dir) {
    out.push({ text: dirBase(j.dir), weight: W.dirBase, why: j.dir });
    out.push({ text: j.dir, weight: W.dir, why: j.dir });
  }
  out.push({ text: column, weight: W.column });
  out.push({ text: j.state, weight: W.status });
  out.push({ text: token.text.replace(/^\S+\s/, ""), weight: W.status }); // the words, not the glyph
  if (token.detail) out.push({ text: token.detail.replace(/^\S+\s/, ""), weight: W.tool, why: token.detail });
  if (j.pi?.error) out.push({ text: j.pi.error, weight: W.tool, why: j.pi.error });
  if (j.model) out.push({ text: j.model, weight: W.model, why: j.model });
  for (const l of j.links ?? []) {
    const shown = l.url.replace(/^https?:\/\//, "");
    out.push({ text: shown, weight: W.link, why: shown });
  }
  return out;
}

/** The field the term is worth most in, or null when it is nowhere. */
function bestField(fields: readonly Field[], title: string, j: WebJob, term: string): { score: number; why?: string } | null {
  const lowTitle = title.toLowerCase();
  if (lowTitle === term) return { score: W.titleExact };
  if (lowTitle.startsWith(term)) return { score: W.titlePrefix };
  let best: { score: number; why?: string } | null = lowTitle.includes(term) ? { score: W.title } : null;
  for (const f of fields) {
    if (f.weight <= (best?.score ?? 0)) continue;
    if (f.text.toLowerCase().includes(term)) best = { score: f.weight, why: f.why };
  }
  if (best) return best;
  if (fuzzyTitle(lowTitle, term) || fuzzyTitle(j.slug, term)) return { score: W.fuzzy };
  return null;
}

/** A loose match that still means something: the term is the title's word
 *  initials ("swr" → "Stripe webhooks retry") or its letters in order within
 *  one word ("rlmt" → "rate-limits"), never letters scattered across words. */
function fuzzyTitle(title: string, term: string): boolean {
  const words = title.split(/[\s\-_·/.]+/).filter(Boolean);
  const initials = words.map((w) => w[0]).join("");
  return initials.startsWith(term) || words.some((w) => fuzzyMatch(w, term));
}
