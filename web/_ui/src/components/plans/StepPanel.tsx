import { useEffect, useRef, useState } from "react";

import { useDebouncedCallback } from "../../hooks/useDebouncedDraft.ts";
import { ESC_PRIORITY, useEscapeLayer } from "../../hooks/useEscapeStack.ts";
import { stepState } from "../../lib/planLayout.ts";
import type { PlanOp, StepState, WebPlan, WebPlanStep } from "../../lib/types.ts";
import { STATE_GLYPH } from "./PlanGraph.tsx";

export const STEP_AUTOSAVE_MS = 600;

/** The text fields a step's panel edits as you type. */
export type StepText = Pick<WebPlanStep, "title" | "note" | "kind" | "topic">;
const TEXT_FIELDS = ["title", "note", "kind", "topic"] as const;

export interface StepPanelProps {
  plan: WebPlan;
  step: WebPlanStep;
  /** what you have typed and not saved yet, so the box shows it too */
  onTyping: (fields: Partial<StepText>) => void;
  onOps: (ops: PlanOp[]) => Promise<unknown>;
  onDelete: () => void;
  onClose: () => void;
}

const STATES: [StepState, string][] = [
  ["todo", "Later"],
  ["wait", "Waiting"],
  ["blocked", "Blocked"],
  ["done", "Done"],
];

function ancestors(plan: WebPlan, id: number): Set<number> {
  const byId = new Map(plan.steps.map((s) => [s.id, s]));
  const out = new Set<number>();
  const stack = [id];
  while (stack.length) {
    for (const w of byId.get(stack.pop()!)?.waits_on ?? []) {
      if (!out.has(w)) {
        out.add(w);
        stack.push(w);
      }
    }
  }
  return out;
}

function descendants(plan: WebPlan, id: number): Set<number> {
  const out = new Set<number>();
  const stack = [id];
  while (stack.length) {
    const at = stack.pop()!;
    for (const s of plan.steps) {
      if (s.waits_on.includes(at) && !out.has(s.id)) {
        out.add(s.id);
        stack.push(s.id);
      }
    }
  }
  return out;
}

/** A step's panel: its state, name, note, kind and topic, what it waits on
 *  and what waits on it, and DELETE. Text saves 600ms after you stop
 *  typing; what arrives from the daemon meanwhile (a pi's edit) shows in a
 *  field you are not typing in. */
export function StepPanel({ plan, step, onTyping, onOps, onDelete, onClose }: StepPanelProps) {
  useEscapeLayer(true, ESC_PRIORITY.plan, onClose);
  const [text, setText] = useState<StepText>(() => ({ title: step.title, note: step.note, kind: step.kind, topic: step.topic }));
  const held = useRef<StepText>({ title: step.title, note: step.note, kind: step.kind, topic: step.topic });
  const pending = useRef<Partial<StepText>>({});

  // the daemon's values land in every field you have not changed since
  useEffect(() => {
    const next = { ...text };
    let changed = false;
    for (const f of TEXT_FIELDS) {
      if (step[f] !== held.current[f] && pending.current[f] === undefined) {
        next[f] = step[f];
        changed = true;
      }
      if (pending.current[f] === undefined) held.current[f] = step[f];
    }
    if (changed) setText(next);
  }, [step.title, step.note, step.kind, step.topic]);

  const save = useDebouncedCallback(() => {
    const fields = pending.current;
    pending.current = {};
    const changed = TEXT_FIELDS.filter((f) => fields[f] !== undefined && fields[f] !== held.current[f]);
    if (!changed.length) return;
    if (fields.title !== undefined && !fields.title.trim()) return;
    const op: PlanOp = { op: "update", step: step.id };
    for (const f of changed) {
      op[f] = fields[f];
      held.current[f] = fields[f]!;
    }
    void onOps([op]).then(() => {
      if (!Object.keys(pending.current).length) onTyping({});
    });
  }, STEP_AUTOSAVE_MS);

  const type = (f: keyof StepText, v: string) => {
    setText((t) => ({ ...t, [f]: v }));
    pending.current = { ...pending.current, [f]: v };
    onTyping({ ...pending.current });
    save.call();
  };

  const [picking, setPicking] = useState<"waits" | "unblocks" | null>(null);
  const byId = new Map(plan.steps.map((s) => [s.id, s]));
  const after = plan.steps.filter((s) => s.waits_on.includes(step.id));
  const up = ancestors(plan, step.id);
  const down = descendants(plan, step.id);
  const waitCandidates = plan.steps.filter((s) => s.id !== step.id && !step.waits_on.includes(s.id) && !down.has(s.id));
  const unblockCandidates = plan.steps.filter((s) => s.id !== step.id && !s.waits_on.includes(step.id) && !up.has(s.id));
  const topics = [...new Set(plan.steps.map((s) => s.topic).filter(Boolean))];

  const row = (s: WebPlanStep, remove: string, onRemove: () => void) => (
    <div key={s.id} className={`sp-dep s-${stepState(s)}`}>
      <i aria-hidden="true">{STATE_GLYPH[stepState(s)]}</i>
      <span>{s.title}</span>
      <button type="button" className="sp-rm" aria-label={remove} title="remove this arrow" onClick={onRemove}>
        ✕
      </button>
    </div>
  );
  const picker = (which: "waits" | "unblocks", list: WebPlanStep[], pick: (s: WebPlanStep) => void) =>
    picking === which ? (
      <div className="sp-pick">
        {list.length ? (
          list.map((s) => (
            <button
              key={s.id}
              type="button"
              className={`s-${stepState(s)}`}
              onClick={() => {
                setPicking(null);
                pick(s);
              }}
            >
              <i aria-hidden="true">{STATE_GLYPH[stepState(s)]}</i>
              {s.title}
            </button>
          ))
        ) : (
          <div className="sp-none">no step to link without a loop</div>
        )}
      </div>
    ) : null;

  return (
    <aside className="step-panel" role="complementary" aria-label="step">
      <div className="sp-head">
        <input className="sp-title" aria-label="title" value={text.title} spellCheck={false} onChange={(e) => type("title", e.target.value)} />
        <button type="button" className="sp-close" aria-label="close" title="close (Esc)" onClick={onClose}>
          ✕
        </button>
      </div>
      <div className="sp-body">
        <section>
          <h4>STATE</h4>
          <div className="sp-states" role="group" aria-label="state">
            {STATES.map(([st, label]) => (
              <button
                key={st}
                type="button"
                className={`s-${st}` + (step.state === st ? " on" : "")}
                aria-pressed={step.state === st}
                onClick={() => {
                  if (step.state !== st) void onOps([{ op: "update", step: step.id, state: st }]);
                }}
              >
                {STATE_GLYPH[st]} {label}
              </button>
            ))}
          </div>
          <div className="sp-ready">{step.ready ? "● Ready now: everything it waits on is done" : ""}</div>
        </section>
        <section>
          <h4>NOTE</h4>
          <input className="sp-input" aria-label="note" placeholder="a short note" value={text.note} spellCheck={false} onChange={(e) => type("note", e.target.value)} />
        </section>
        <section className="sp-pair">
          <label>
            <h4>KIND</h4>
            <input className="sp-input" aria-label="kind" placeholder="PR, ask, flag…" value={text.kind} onChange={(e) => type("kind", e.target.value)} />
          </label>
          <label>
            <h4>TOPIC</h4>
            <input className="sp-input" aria-label="topic" list="plan-topics" placeholder="none" value={text.topic} onChange={(e) => type("topic", e.target.value)} />
            <datalist id="plan-topics">
              {topics.map((t) => (
                <option key={t} value={t} />
              ))}
            </datalist>
          </label>
        </section>
        <section role="group" aria-label="waits on">
          <h4>WAITS ON</h4>
          {step.waits_on.map((id) => byId.get(id)).filter((s): s is WebPlanStep => !!s).map((s) =>
            row(s, `stop waiting on ${s.title}`, () => void onOps([{ op: "unlink", step: step.id, waits_on: [s.id] }])),
          )}
          <button type="button" className="sp-link" onClick={() => setPicking(picking === "waits" ? null : "waits")}>
            + link a step
          </button>
          {picker("waits", waitCandidates, (s) => void onOps([{ op: "link", step: step.id, waits_on: [s.id] }]))}
        </section>
        <section role="group" aria-label="unblocks">
          <h4>UNBLOCKS</h4>
          {after.map((s) => row(s, `stop ${s.title} waiting on this`, () => void onOps([{ op: "unlink", step: s.id, waits_on: [step.id] }])))}
          <button type="button" className="sp-link" onClick={() => setPicking(picking === "unblocks" ? null : "unblocks")}>
            + link a step
          </button>
          {picker("unblocks", unblockCandidates, (s) => void onOps([{ op: "link", step: s.id, waits_on: [step.id] }]))}
        </section>
        <button type="button" className="sp-delete" onClick={onDelete} title="delete this step (⌫); the steps after it then wait on what it waited on">
          DELETE STEP
        </button>
      </div>
    </aside>
  );
}
