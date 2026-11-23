import { useCallback, useEffect, useMemo, useRef, useState } from "react";

import { addPlan, applyPlanOps, fetchPlan, fetchPlans, removePlan, renamePlan } from "../../api/client.ts";
import { readLS, writeLS } from "../../hooks/useLocalStorage.ts";
import { usePoll } from "../../hooks/usePoll.ts";
import { layoutPlan, stepKey, type PlanNode } from "../../lib/planLayout.ts";
import type { PlanOp, WebPlan, WebPlanStep, WebPlanSummary } from "../../lib/types.ts";
import { showToast } from "../overlays/Toast.tsx";
import { PlanGraph, STATE_GLYPH, type PlanFilter } from "./PlanGraph.tsx";
import { StepPanel, type StepText } from "./StepPanel.tsx";

export const PLANS_POLL_MS = 3000;

const PLAN_KEY = "driver.plan";
const FOLDED_PREFIX = "driver.plan-folded:";
/** the id of the box being named, before the daemon gives it one */
const DRAFT_ID = -1;
/** how much of the canvas the step panel covers, right edge included */
const PANEL_COVERS = 392;

const message = (e: unknown) => (e instanceof Error ? e.message : String(e));

function readFolded(plan: number): Set<string> {
  try {
    const v = JSON.parse(readLS(FOLDED_PREFIX + plan) ?? "[]") as unknown;
    return new Set(Array.isArray(v) ? v.filter((t): t is string => typeof t === "string") : []);
  } catch {
    return new Set();
  }
}

/** The PLANS view: a tab per plan, and the plan in view as a graph you can
 *  edit. Plans are the daemon's, apart from the board; the view polls, so
 *  what any pi changes shows up here as it happens. */
export function PlansView() {
  const list = usePoll((signal) => fetchPlans(signal), PLANS_POLL_MS);
  const plans = list.data?.plans;
  const [chosen, setChosen] = useState<number | null>(() => {
    const v = Number(readLS(PLAN_KEY));
    return Number.isInteger(v) && v > 0 ? v : null;
  });
  const current = plans?.find((p) => p.id === chosen) ?? plans?.[0];
  const currentId = current?.id;
  const choose = useCallback((id: number) => {
    setChosen(id);
    writeLS(PLAN_KEY, String(id));
  }, []);

  const polled = usePoll(
    (signal) => (currentId === undefined ? Promise.resolve(undefined) : fetchPlan(currentId, signal)),
    PLANS_POLL_MS,
    { enabled: currentId !== undefined },
  );
  const refetchPlan = polled.refetch;
  const refetchList = list.refetch;
  useEffect(() => {
    if (currentId !== undefined) void refetchPlan();
  }, [currentId, refetchPlan]);
  // the answer to your own last edit, until a poll is as new
  const [answered, setAnswered] = useState<WebPlan | undefined>();
  const newest = [polled.data, answered].filter((p): p is WebPlan => p?.id === currentId).sort((a, b) => b.rev - a.rev)[0];

  const [filter, setFilter] = useState<PlanFilter>("all");
  // + STEP: each press asks the plan in view for a new box
  const [addStep, setAddStep] = useState(0);

  const ops = useCallback(
    async (planId: number, batch: PlanOp[]) => {
      try {
        const res = await applyPlanOps(planId, batch);
        setAnswered(res.plan);
        void refetchList();
        return res.plan;
      } catch (e) {
        showToast(message(e));
        void refetchPlan();
        return undefined;
      }
    },
    [refetchList, refetchPlan],
  );

  return (
    <section className="plans" id="plans">
      <div className="plans-bar">
        <PlanTabs plans={plans ?? []} currentId={currentId} onChoose={choose} onChanged={refetchList} />
        <button type="button" className="plan-add-step" disabled={!newest} onClick={() => setAddStep((n) => n + 1)}>
          + STEP
        </button>
        {newest ? <Filters plan={newest} filter={filter} setFilter={setFilter} /> : null}
      </div>
      {plans && plans.length === 0 ? (
        <div className="plans-empty">No plans yet. Press + to make one, or ask any pi on the board — it has the driver_plans tool.</div>
      ) : newest ? (
        <PlanPane key={newest.id} plan={newest} filter={filter} addStep={addStep} onOps={(batch) => ops(newest.id, batch)} />
      ) : null}
    </section>
  );
}

function PlanTabs({ plans, currentId, onChoose, onChanged }: { plans: WebPlanSummary[]; currentId?: number; onChoose: (id: number) => void; onChanged: () => Promise<boolean> }) {
  const [naming, setNaming] = useState<{ id: number | null; value: string } | null>(null);
  const commit = async () => {
    if (!naming) return;
    const name = naming.value.trim();
    const id = naming.id;
    setNaming(null);
    if (!name) return;
    try {
      if (id === null) {
        const res = await addPlan(name);
        await onChanged();
        onChoose(res.plan.id);
      } else {
        await renamePlan(id, name);
        await onChanged();
      }
    } catch (e) {
      showToast(message(e));
    }
  };
  const nameBox = (
    <input
      className="plan-name"
      aria-label="plan name"
      autoFocus
      placeholder="name the plan"
      value={naming?.value ?? ""}
      onChange={(e) => setNaming((n) => (n ? { ...n, value: e.target.value } : n))}
      onKeyDown={(e) => {
        if (e.key === "Enter") void commit();
        if (e.key === "Escape") {
          e.stopPropagation();
          setNaming(null);
        }
      }}
      onBlur={() => void commit()}
    />
  );
  const remove = async (p: WebPlanSummary) => {
    if (!window.confirm(`Remove the plan “${p.name}” and its ${p.steps} step${p.steps === 1 ? "" : "s"}?`)) return;
    try {
      await removePlan(p.id);
      await onChanged();
    } catch (e) {
      showToast(message(e));
    }
  };
  return (
    <div className="plan-tabs">
      {plans.map((p) =>
        naming?.id === p.id ? (
          <span key={p.id}>{nameBox}</span>
        ) : (
          <span key={p.id} className="plan-tab-wrap">
            <button
              type="button"
              className={"plan-tab" + (p.id === currentId ? " on" : "")}
              title="double-click to rename"
              onClick={() => onChoose(p.id)}
              onDoubleClick={() => setNaming({ id: p.id, value: p.name })}
            >
              {p.name}
              <small>
                {p.done}/{p.steps}
              </small>
            </button>
            {p.id === currentId ? (
              <button type="button" className="plan-remove" aria-label="remove plan" title="remove this plan" onClick={() => void remove(p)}>
                ✕
              </button>
            ) : null}
          </span>
        ),
      )}
      {naming?.id === null ? (
        nameBox
      ) : (
        <button type="button" className="plan-new" aria-label="new plan" title="make a plan" onClick={() => setNaming({ id: null, value: "" })}>
          +
        </button>
      )}
    </div>
  );
}

function Filters({ plan, filter, setFilter }: { plan: WebPlan; filter: PlanFilter; setFilter: (f: PlanFilter) => void }) {
  const n = (f: PlanFilter) => plan.steps.filter((s) => (f === "ready" ? s.ready : s.state === f && !s.ready)).length;
  const button = (f: PlanFilter, label: string, glyph?: string) => (
    <button type="button" className={"plan-filter f-" + f + (filter === f ? " on" : "")} aria-pressed={filter === f} onClick={() => setFilter(f)}>
      {glyph ? <i aria-hidden="true">{glyph}</i> : null}
      {label}
      {f !== "all" ? <b>{` ${n(f)}`}</b> : null}
    </button>
  );
  return (
    <div className="plan-filters" role="group" aria-label="filter steps">
      {button("all", "All")}
      {button("ready", "Ready", STATE_GLYPH.ready)}
      {button("wait", "Waiting", STATE_GLYPH.wait)}
      {button("blocked", "Blocked", STATE_GLYPH.blocked)}
    </div>
  );
}

function PlanPane({ plan, filter, addStep, onOps }: { plan: WebPlan; filter: PlanFilter; addStep: number; onOps: (ops: PlanOp[]) => Promise<WebPlan | undefined> }) {
  const [folded, setFolded] = useState(() => readFolded(plan.id));
  const toggleTopic = useCallback(
    (topic: string) =>
      setFolded((was) => {
        const next = new Set(was);
        if (!next.delete(topic)) next.add(topic);
        writeLS(FOLDED_PREFIX + plan.id, next.size ? JSON.stringify([...next]) : null);
        return next;
      }),
    [plan.id],
  );

  const [selected, setSelected] = useState<number | null>(null);
  const [typing, setTyping] = useState<{ id: number; fields: Partial<StepText> } | null>(null);
  // a box being named: after a step (+ on its box), or on its own (+ STEP)
  const [draft, setDraft] = useState<{ after: WebPlanStep | null } | null>(null);
  const [draftTitle, setDraftTitle] = useState("");

  const asked = useRef(addStep);
  useEffect(() => {
    if (addStep === asked.current) return;
    asked.current = addStep;
    setDraftTitle("");
    setDraft({ after: null });
  }, [addStep]);

  const steps = useMemo(() => {
    let out = plan.steps;
    if (typing) out = out.map((s) => (s.id === typing.id ? { ...s, ...typing.fields } : s));
    if (draft) {
      out = [...out, { id: DRAFT_ID, title: "", note: "", state: "todo", ready: false, kind: "", topic: draft.after?.topic ?? "", waits_on: draft.after ? [draft.after.id] : [] }];
    }
    return out;
  }, [plan.steps, typing, draft]);
  const layout = useMemo(() => layoutPlan(steps, folded), [steps, folded]);
  const step = selected === null ? undefined : plan.steps.find((s) => s.id === selected);

  const close = useCallback(() => {
    setSelected(null);
    setTyping(null);
  }, []);

  const del = useCallback(
    async (id: number) => {
      setSelected(null);
      setTyping(null);
      await onOps([{ op: "delete", step: id }]);
    },
    [onOps],
  );

  // ⌫ deletes the selected step, unless you are typing somewhere
  const selectedRef = useRef(selected);
  selectedRef.current = selected;
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.key !== "Backspace" && e.key !== "Delete") return;
      const t = e.target as HTMLElement | null;
      if (t && (t.tagName === "INPUT" || t.tagName === "TEXTAREA" || t.isContentEditable)) return;
      if (selectedRef.current === null) return;
      e.preventDefault();
      void del(selectedRef.current);
    };
    document.addEventListener("keydown", onKey);
    return () => document.removeEventListener("keydown", onKey);
  }, [del]);

  const commitDraft = async () => {
    if (!draft) return;
    const title = draftTitle.trim();
    const after = draft.after;
    setDraft(null);
    setDraftTitle("");
    if (!title) return;
    const op: PlanOp = { op: "add", title };
    if (after) op.waits_on = [after.id];
    if (after?.topic) op.topic = after.topic;
    const before = new Set(plan.steps.map((s) => s.id));
    const next = await onOps([op]);
    const made = next?.steps.find((s) => !before.has(s.id));
    if (made && !after) setSelected(made.id);
  };

  const renderBody = (n: PlanNode) =>
    n.step?.id === DRAFT_ID ? (
      <>
        <div className="ps-title">
          <i aria-hidden="true">{STATE_GLYPH.todo}</i>
          <input
            className="ps-name"
            aria-label="new step"
            autoFocus
            placeholder="name the step"
            value={draftTitle}
            onChange={(e) => setDraftTitle(e.target.value)}
            onClick={(e) => e.stopPropagation()}
            onKeyDown={(e) => {
              if (e.key === "Enter") void commitDraft();
              if (e.key === "Escape") {
                e.stopPropagation();
                setDraft(null);
              }
            }}
            onBlur={() => void commitDraft()}
          />
        </div>
        <div className="ps-sub">enter to save</div>
      </>
    ) : null;

  return (
    <div className="plan-pane">
      {plan.steps.length || draft ? (
        <PlanGraph
          layout={layout}
          filter={filter}
          onToggleTopic={toggleTopic}
          selected={selected === null ? null : stepKey(selected)}
          coveredRight={step ? PANEL_COVERS : 0}
          onSelectStep={(n) => {
            if (n.step && n.step.id !== DRAFT_ID) {
              setTyping(null);
              setSelected(n.step.id);
            }
          }}
          onAddAfter={(n) => {
            if (!n.step || n.step.id === DRAFT_ID) return;
            setDraftTitle("");
            setDraft({ after: n.step });
          }}
          renderBody={renderBody}
          onBackground={close}
        />
      ) : (
        <div className="plans-empty">This plan has no steps yet. + STEP adds one.</div>
      )}
      {step ? (
        <StepPanel
          key={step.id}
          plan={plan}
          step={step}
          onTyping={(fields) => setTyping({ id: step.id, fields })}
          onOps={onOps}
          onDelete={() => void del(step.id)}
          onClose={close}
        />
      ) : null}
    </div>
  );
}
