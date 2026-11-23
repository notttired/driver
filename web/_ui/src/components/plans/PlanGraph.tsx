import { useEffect, useLayoutEffect, useMemo, useRef, useState, type ReactNode } from "react";

import { NODE_H, NODE_W, chain, type NodeState, type PlanLayout, type PlanNode } from "../../lib/planLayout.ts";

export type PlanFilter = "all" | "ready" | "wait" | "blocked";

export const STATE_GLYPH: Record<NodeState, string> = { done: "✓", ready: "●", wait: "◷", blocked: "✕", todo: "○" };

const TWEEN_MS = 600;

type Point = { x: number; y: number };

const raf = (cb: (t: number) => void) =>
  typeof window.requestAnimationFrame === "function" ? window.requestAnimationFrame(cb) : window.setTimeout(() => cb(performance.now()), 16);
const cancelRaf = (id: number) =>
  typeof window.cancelAnimationFrame === "function" ? window.cancelAnimationFrame(id) : window.clearTimeout(id);

/** The boxes' positions, gliding from where they were to where the layout
 *  puts them now; a box that was not there before starts where it lands. */
function useGlide(target: Map<string, Point>): Map<string, Point> {
  const [now, setNow] = useState(target);
  const nowRef = useRef(now);
  nowRef.current = now;
  useEffect(() => {
    const from = nowRef.current;
    const t0 = performance.now();
    let id = 0;
    const step = (t: number) => {
      const k = Math.min(1, (t - t0) / TWEEN_MS);
      const e = k < 0.5 ? 4 * k * k * k : 1 - Math.pow(-2 * k + 2, 3) / 2;
      const next = new Map<string, Point>();
      for (const [key, p] of target) {
        const f = from.get(key) ?? p;
        next.set(key, { x: f.x + (p.x - f.x) * e, y: f.y + (p.y - f.y) * e });
      }
      setNow(next);
      if (k < 1) id = raf(step);
    };
    id = raf(step);
    return () => cancelRaf(id);
  }, [target]);
  return now;
}

const matches = (n: PlanNode, filter: PlanFilter) =>
  filter === "all" || (n.members ? n.members.some((m) => (m.ready ? "ready" : m.state) === filter) : n.state === filter);

export interface PlanGraphProps {
  layout: PlanLayout;
  filter: PlanFilter;
  onToggleTopic: (topic: string) => void;
  /** a step's box was clicked (a folded topic's box unfolds it instead) */
  onSelectStep?: (node: PlanNode) => void;
  selected?: string | null;
  /** width the side panel covers; the camera keeps the selected box clear of it */
  coveredRight?: number;
  /** the box's content in place of its title and note (a step being named) */
  renderBody?: (node: PlanNode) => ReactNode | null;
  /** + on a step's box: add the step that comes after it */
  onAddAfter?: (node: PlanNode) => void;
  onBackground?: () => void;
}

/** The plan as boxes and arrows, fitted to the space it has. */
export function PlanGraph({ layout, filter, onToggleTopic, onSelectStep, selected, coveredRight = 0, renderBody, onAddAfter, onBackground }: PlanGraphProps) {
  const target = useMemo(() => new Map(layout.nodes.map((n) => [n.key, { x: n.x, y: n.y }])), [layout]);
  const at = useGlide(target);
  const [hover, setHover] = useState<string | null>(null);
  const traced = useMemo(() => (hover && layout.nodes.some((n) => n.key === hover) ? chain(layout, hover) : null), [hover, layout]);

  const canvas = useRef<HTMLDivElement>(null);
  const [size, setSize] = useState({ w: 0, h: 0 });
  useLayoutEffect(() => {
    const el = canvas.current;
    if (!el) return;
    const measure = () => setSize({ w: el.clientWidth, h: el.clientHeight });
    measure();
    const ro = new ResizeObserver(measure);
    ro.observe(el);
    return () => ro.disconnect();
  }, []);

  const camera = useMemo(() => {
    if (!size.w || !size.h || !layout.width) return { x: 0, y: 0, s: 1 };
    const s = Math.min(size.w / layout.width, size.h / layout.height, 1.2);
    let x = (size.w - layout.width * s) / 2;
    const y = Math.max(0, (size.h - layout.height * s) / 2);
    const sel = selected ? layout.nodes.find((n) => n.key === selected) : undefined;
    if (sel && coveredRight) {
      const right = size.w - coveredRight - 16;
      const edge = x + (sel.x + NODE_W) * s;
      if (edge > right) x -= edge - right;
    }
    return { x, y, s };
  }, [size, layout, selected, coveredRight]);

  const bandBoxes = layout.bands.map((b) => {
    const ps = layout.nodes.filter((n) => n.step?.topic === b.topic).map((n) => at.get(n.key)).filter((p): p is Point => !!p);
    const x = ps.length ? Math.min(...ps.map((p) => p.x)) : b.x;
    const top = ps.length ? Math.min(...ps.map((p) => p.y)) : b.y + 22;
    const bottom = ps.length ? Math.max(...ps.map((p) => p.y)) + NODE_H : b.y + b.height;
    return { ...b, x, top, bottom };
  });

  return (
    <div
      className="plan-canvas"
      ref={canvas}
      onClick={(e) => {
        if (e.target === e.currentTarget) onBackground?.();
      }}
    >
      <div
        className="plan-world"
        style={{ transform: `translate(${camera.x}px, ${camera.y}px) scale(${camera.s})`, width: layout.width, height: layout.height }}
        onClick={(e) => {
          if (e.target === e.currentTarget) onBackground?.();
        }}
      >
        {bandBoxes.map((b) => (
          <div key={b.topic} className={"plan-band" + (traced || filter !== "all" ? " quiet" : "")} style={{ transform: `translate(${b.x - 14}px, ${b.top - 22}px)`, height: b.bottom - b.top + 22 }}>
            <button className="plan-topic" type="button" title="fold this topic into one box" onClick={() => onToggleTopic(b.topic)}>
              {b.topic}
              <small>
                {b.done}/{b.total} ▾
              </small>
            </button>
          </div>
        ))}
        <svg className="plan-edges" width={layout.width} height={layout.height} aria-hidden="true">
          <defs>
            <marker id="plan-arrow" viewBox="0 0 8 8" refX="7" refY="4" markerWidth="6" markerHeight="6" orient="auto">
              <path d="M0 0 L8 4 L0 8 z" />
            </marker>
          </defs>
          {layout.edges.map((e) => {
            const a = at.get(e.from);
            const b = at.get(e.to);
            if (!a || !b) return null;
            const x1 = a.x + NODE_W;
            const y1 = a.y + NODE_H / 2;
            const x2 = b.x - 3;
            const y2 = b.y + NODE_H / 2;
            const dx = Math.max(30, (x2 - x1) / 2);
            const fromNode = layout.nodes.find((n) => n.key === e.from);
            const toNode = layout.nodes.find((n) => n.key === e.to);
            const lit = traced && ((traced.up.has(e.from) && (traced.up.has(e.to) || e.to === hover)) || (traced.down.has(e.to) && (traced.down.has(e.from) || e.from === hover)));
            const cls =
              "plan-edge" +
              (toNode?.state === "blocked" ? " e-blocked" : fromNode?.state === "done" ? " e-done" : "") +
              (lit ? " lit" : traced || filter !== "all" ? " quiet" : "");
            return <path key={`${e.from}>${e.to}`} className={cls} d={`M${x1} ${y1} C${x1 + dx} ${y1} ${x2 - dx} ${y2} ${x2} ${y2}`} markerEnd="url(#plan-arrow)" />;
          })}
        </svg>
        {layout.nodes.map((n) => {
          const p = at.get(n.key) ?? { x: n.x, y: n.y };
          const inChain = !traced || n.key === hover || traced.up.has(n.key) || traced.down.has(n.key);
          const cls =
            `plan-step s-${n.state}` +
            (n.members ? " folded" : "") +
            (!matches(n, filter) || !inChain ? " dim" : "") +
            (traced?.up.has(n.key) ? " up" : "") +
            (traced?.down.has(n.key) ? " down" : "") +
            (n.key === hover ? " me" : "") +
            (n.key === selected ? " sel" : "");
          return (
            <div
              key={n.key}
              className={cls}
              data-state={n.state}
              data-key={n.key}
              style={{ transform: `translate(${p.x}px, ${p.y}px)` }}
              onMouseEnter={() => setHover(n.key)}
              onMouseLeave={() => setHover((h) => (h === n.key ? null : h))}
              onClick={() => (n.topic !== undefined ? onToggleTopic(n.topic) : onSelectStep?.(n))}
            >
              {renderBody?.(n) ?? (
                <>
                  <div className="ps-title">
                    <i aria-hidden="true">{STATE_GLYPH[n.state]}</i>
                    <span>{n.title}</span>
                  </div>
                  <div className="ps-sub">{n.sub || "\u00a0"}</div>
                  {onAddAfter && n.step ? (
                    <button
                      type="button"
                      className="ps-add"
                      aria-label="add the next step"
                      title="add the step that comes after this one"
                      onClick={(e) => {
                        e.stopPropagation();
                        onAddAfter(n);
                      }}
                    >
                      +
                    </button>
                  ) : null}
                </>
              )}
            </div>
          );
        })}
      </div>
    </div>
  );
}
