import { useCallback, useEffect, useMemo, useState } from "react";

import { fetchPlan, fetchPlans } from "../../api/client.ts";
import { readLS, writeLS } from "../../hooks/useLocalStorage.ts";
import { usePoll } from "../../hooks/usePoll.ts";
import { layoutPlan } from "../../lib/planLayout.ts";
import type { WebPlan } from "../../lib/types.ts";
import { PlanGraph, type PlanFilter } from "./PlanGraph.tsx";

export const PLANS_POLL_MS = 3000;

const PLAN_KEY = "driver.plan";
const FOLDED_PREFIX = "driver.plan-folded:";

function readFolded(plan: number): Set<string> {
  try {
    const v = JSON.parse(readLS(FOLDED_PREFIX + plan) ?? "[]") as unknown;
    return new Set(Array.isArray(v) ? v.filter((t): t is string => typeof t === "string") : []);
  } catch {
    return new Set();
  }
}

/** The PLANS view: a tab per plan, and the plan in view as a graph. Plans
 *  are the daemon's, apart from the board; it is polled, so what any pi
 *  changes shows up here as it happens. */
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

  const plan = usePoll(
    (signal) => (currentId === undefined ? Promise.resolve(undefined) : fetchPlan(currentId, signal)),
    PLANS_POLL_MS,
    { enabled: currentId !== undefined },
  );
  const { refetch } = plan;
  useEffect(() => {
    if (currentId !== undefined) void refetch();
  }, [currentId, refetch]);
  const shown: WebPlan | undefined = plan.data?.id === currentId ? plan.data : undefined;
  const [filter, setFilter] = useState<PlanFilter>("all");

  return (
    <section className="plans" id="plans">
      <div className="plans-bar">
        <div className="plan-tabs">
          {(plans ?? []).map((p) => (
            <button key={p.id} type="button" className={"plan-tab" + (p.id === currentId ? " on" : "")} onClick={() => choose(p.id)}>
              {p.name}
              <small>
                {p.done}/{p.steps}
              </small>
            </button>
          ))}
        </div>
        {shown ? <Filters plan={shown} filter={filter} setFilter={setFilter} /> : null}
      </div>
      {plans && plans.length === 0 ? (
        <div className="plans-empty">No plans yet. Ask any pi on the board to make one (it has the driver_plans tool).</div>
      ) : shown ? (
        <PlanPane key={shown.id} plan={shown} filter={filter} />
      ) : null}
    </section>
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
      {button("ready", "Ready", "●")}
      {button("wait", "Waiting", "◷")}
      {button("blocked", "Blocked", "✕")}
    </div>
  );
}

function PlanPane({ plan, filter }: { plan: WebPlan; filter: PlanFilter }) {
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
  const layout = useMemo(() => layoutPlan(plan.steps, folded), [plan.steps, folded]);
  if (!plan.steps.length) return <div className="plans-empty">This plan has no steps yet.</div>;
  return <PlanGraph layout={layout} filter={filter} onToggleTopic={toggleTopic} />;
}
