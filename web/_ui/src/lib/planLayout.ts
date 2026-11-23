// The PLANS graph's geometry: where each step's box goes, from the links
// alone. A step's column is how many steps it waits on in a row (its longest
// chain back); a step that waits on nothing but holds others up sits right
// before the first of them. Steps of one topic share a band, the bands in the
// order their topics first appear and the steps without a topic last; a
// folded topic is one box in that last band. Within a column, a step goes
// near the steps it waits on, and one whose next step is columns away goes
// last, so its long arrow runs clear of the rest.

import type { WebPlanStep } from "./types.ts";

export const NODE_W = 176;
export const NODE_H = 44;
const COL_GAP = 44;
const ROW_GAP = 12;
const BAND_GAP = 30;
const LABEL_H = 22;
const PAD = 24;

/** What a box shows: the step's own state, ready when it can start now. */
export type NodeState = "done" | "ready" | "wait" | "blocked" | "todo";

export interface PlanNode {
  /** "s<id>" for a step, "t:<topic>" for a folded topic */
  key: string;
  x: number;
  y: number;
  state: NodeState;
  title: string;
  sub: string;
  /** the step, when the box is one */
  step?: WebPlanStep;
  /** the folded topic, when the box is one */
  topic?: string;
  /** a folded topic's steps */
  members?: WebPlanStep[];
}

/** an arrow: `to` waits on `from` */
export interface PlanEdge {
  from: string;
  to: string;
}

/** a topic's band, drawn with its label (the fold control) above it */
export interface PlanBand {
  topic: string;
  x: number;
  y: number;
  height: number;
  done: number;
  total: number;
}

export interface PlanLayout {
  nodes: PlanNode[];
  edges: PlanEdge[];
  bands: PlanBand[];
  width: number;
  height: number;
}

export const stepState = (s: WebPlanStep): NodeState => (s.ready ? "ready" : s.state);

export const stepKey = (id: number) => `s${id}`;

const topicKey = (topic: string) => `t:${topic}`;

function foldedNode(topic: string, members: WebPlanStep[]): Omit<PlanNode, "x" | "y"> {
  const n = (st: NodeState) => members.filter((m) => stepState(m) === st).length;
  const state: NodeState =
    n("done") === members.length ? "done" : n("blocked") ? "blocked" : n("ready") ? "ready" : n("wait") ? "wait" : "todo";
  const sub = [`${n("done")}/${members.length} done`, n("ready") && `${n("ready")} ready`, n("wait") && `${n("wait")} waiting`, n("blocked") && `${n("blocked")} blocked`]
    .filter(Boolean)
    .join(" · ");
  return { key: topicKey(topic), state, title: topic, sub, topic, members };
}

export function layoutPlan(steps: readonly WebPlanStep[], folded: ReadonlySet<string>): PlanLayout {
  const byId = new Map(steps.map((s) => [s.id, s]));
  const keyOf = (s: WebPlanStep) => (s.topic && folded.has(s.topic) ? topicKey(s.topic) : stepKey(s.id));

  // the boxes, in the order the steps were added, and the band each sits in
  const boxes: Omit<PlanNode, "x" | "y">[] = [];
  const band = new Map<string, string>();
  const seen = new Set<string>();
  const topics: string[] = [];
  for (const s of steps) {
    if (s.topic && !topics.includes(s.topic)) topics.push(s.topic);
    const key = keyOf(s);
    if (seen.has(key)) continue;
    seen.add(key);
    if (key.startsWith("t:")) {
      boxes.push(foldedNode(s.topic, steps.filter((m) => m.topic === s.topic)));
      band.set(key, "");
    } else {
      boxes.push({ key, state: stepState(s), title: s.title, sub: s.note.split(/\r?\n/)[0] ?? "", step: s });
      band.set(key, s.topic);
    }
  }

  // the arrows between boxes, a folded topic's own dropped
  const edgeSet = new Map<string, PlanEdge>();
  for (const s of steps) {
    for (const on of s.waits_on) {
      const w = byId.get(on);
      if (!w) continue;
      const from = keyOf(w);
      const to = keyOf(s);
      if (from !== to) edgeSet.set(`${from}>${to}`, { from, to });
    }
  }
  const edges = [...edgeSet.values()];
  const preds = new Map(boxes.map((b) => [b.key, [] as string[]]));
  const succs = new Map(boxes.map((b) => [b.key, [] as string[]]));
  for (const e of edges) {
    preds.get(e.to)!.push(e.from);
    succs.get(e.from)!.push(e.to);
  }

  // columns: the longest chain back; a starting point that holds others up
  // moves up to right before the first of them
  const rank = new Map<string, number>();
  const visiting = new Set<string>();
  const rankOf = (key: string): number => {
    const known = rank.get(key);
    if (known !== undefined) return known;
    if (visiting.has(key)) return 0;
    visiting.add(key);
    const ps = preds.get(key)!;
    const r = ps.length ? Math.max(...ps.map(rankOf)) + 1 : 0;
    visiting.delete(key);
    rank.set(key, r);
    return r;
  };
  for (const b of boxes) rankOf(b.key);
  for (const b of boxes) {
    const ss = succs.get(b.key)!;
    if (!preds.get(b.key)!.length && ss.length) rank.set(b.key, Math.max(0, Math.min(...ss.map((s) => rank.get(s)!)) - 1));
  }

  const bandIds = [...topics.filter((t) => !folded.has(t)), ...(boxes.some((b) => band.get(b.key) === "") ? [""] : [])];
  const cols = new Map(bandIds.map((b) => [b, new Map<number, string[]>()]));
  for (const b of boxes) {
    const c = cols.get(band.get(b.key)!)!;
    const r = rank.get(b.key)!;
    c.set(r, [...(c.get(r) ?? []), b.key]);
  }

  let pos = new Map<string, { x: number; y: number }>();
  let bands: PlanBand[] = [];
  const place = (): number => {
    pos = new Map();
    bands = [];
    let top = PAD;
    for (const id of bandIds) {
      const c = cols.get(id)!;
      const tallest = Math.max(...[...c.values()].map((ks) => ks.length));
      const h = tallest * (NODE_H + ROW_GAP) - ROW_GAP;
      const label = id === "" ? 0 : LABEL_H;
      top += label;
      for (const [r, ks] of c) {
        const ch = ks.length * (NODE_H + ROW_GAP) - ROW_GAP;
        ks.forEach((k, i) => pos.set(k, { x: PAD + r * (NODE_W + COL_GAP), y: top + (h - ch) / 2 + i * (NODE_H + ROW_GAP) }));
      }
      if (id !== "") {
        const members = steps.filter((s) => s.topic === id);
        const x = Math.min(...[...c.keys()].map((r) => PAD + r * (NODE_W + COL_GAP)));
        bands.push({ topic: id, x, y: top - label, height: h + label, done: members.filter((s) => s.state === "done").length, total: members.length });
      }
      top += h + BAND_GAP;
    }
    return top - BAND_GAP + PAD;
  };
  place();
  const span = (key: string) => {
    const ss = succs.get(key)!;
    return ss.length ? Math.min(...ss.map((s) => rank.get(s)!)) - rank.get(key)! : 1;
  };
  for (let pass = 0; pass < 3; pass++) {
    for (const c of cols.values()) {
      for (const ks of c.values()) {
        const weight = new Map(
          ks.map((k) => {
            const ps = preds.get(k)!.filter((p) => pos.has(p));
            const centre = ps.length ? ps.reduce((sum, p) => sum + pos.get(p)!.y, 0) / ps.length : pos.get(k)!.y;
            return [k, centre + (span(k) > 1 ? 1e6 : 0)];
          }),
        );
        ks.sort((a, b) => weight.get(a)! - weight.get(b)!);
      }
    }
    place();
  }
  const height = boxes.length ? place() : 0;
  const columns = boxes.length ? Math.max(...rank.values()) + 1 : 0;
  const width = columns ? PAD * 2 + columns * (NODE_W + COL_GAP) - COL_GAP : 0;
  return { nodes: boxes.map((b) => ({ ...b, ...pos.get(b.key)! })), edges, bands, width, height };
}

/** Every box upstream of key (what it waits on, all the way back) and
 *  downstream (what waits on it). */
export function chain(layout: PlanLayout, key: string): { up: Set<string>; down: Set<string> } {
  const walk = (next: (k: string) => string[]) => {
    const out = new Set<string>();
    const stack = [key];
    while (stack.length) {
      for (const k of next(stack.pop()!)) {
        if (!out.has(k)) {
          out.add(k);
          stack.push(k);
        }
      }
    }
    return out;
  };
  return {
    up: walk((k) => layout.edges.filter((e) => e.to === k).map((e) => e.from)),
    down: walk((k) => layout.edges.filter((e) => e.from === k).map((e) => e.to)),
  };
}
