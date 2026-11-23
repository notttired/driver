import { act, cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { App } from "../../App.tsx";
import type { WebPlan, WebPlanStep } from "../../lib/types.ts";
import { makeBoard } from "../../test/fixtures.ts";

const json = (body: unknown, status = 200) =>
  new Response(JSON.stringify(body), { status, headers: { "content-type": "application/json" } });

type Op = { op: string; ref?: string; step?: number | string; title?: string; note?: string; state?: string; kind?: string; topic?: string; waits_on?: (number | string)[]; unblocks?: (number | string)[] };

/** A small stand-in for the daemon's plans: enough to answer what the view
 *  posts, and to show the result back. */
function fakeDaemon() {
  let nextPlan = 3;
  let nextStep = 100;
  const plans: WebPlan[] = [
    {
      id: 2,
      name: "Field removal",
      rev: 1,
      steps: [
        { id: 1, title: "No calls to the field", note: "", state: "done", ready: false, kind: "", topic: "", waits_on: [] },
        { id: 2, title: "World PR", note: "", state: "todo", ready: true, kind: "PR", topic: "", waits_on: [1] },
        { id: 3, title: "Land the World PR", note: "", state: "todo", ready: false, kind: "", topic: "", waits_on: [2] },
        { id: 4, title: "No errors for a day", note: "", state: "todo", ready: false, kind: "", topic: "", waits_on: [3] },
        { id: 5, title: "Close #2389", note: "", state: "todo", ready: false, kind: "", topic: "", waits_on: [4] },
      ],
    },
  ];
  const posts: { path: string; body: Record<string, unknown> }[] = [];
  const refresh = (p: WebPlan) => {
    const byId = new Map(p.steps.map((s) => [s.id, s]));
    for (const s of p.steps) s.ready = s.state === "todo" && s.waits_on.every((w) => byId.get(w)?.state === "done");
    p.rev++;
  };
  const apply = (p: WebPlan, ops: Op[]) => {
    const refs: Record<string, number> = {};
    const id = (r: number | string | undefined) => (typeof r === "number" ? r : /^\d+$/.test(String(r)) ? Number(r) : refs[String(r)]!);
    const find = (r: number | string | undefined) => p.steps.find((s) => s.id === id(r))!;
    for (const op of ops) {
      if (op.op === "add") {
        const s: WebPlanStep = { id: nextStep++, title: op.title ?? "", note: op.note ?? "", state: (op.state as WebPlanStep["state"]) ?? "todo", ready: false, kind: op.kind ?? "", topic: op.topic ?? "", waits_on: (op.waits_on ?? []).map(id) };
        p.steps.push(s);
        if (op.ref) refs[op.ref] = s.id;
        for (const u of op.unblocks ?? []) find(u).waits_on.push(s.id);
      } else if (op.op === "update") {
        Object.assign(find(op.step), Object.fromEntries(Object.entries(op).filter(([k]) => ["title", "note", "state", "kind", "topic"].includes(k))));
      } else if (op.op === "link") {
        find(op.step).waits_on.push(...(op.waits_on ?? []).map(id));
      } else if (op.op === "unlink") {
        const s = find(op.step);
        const drop = new Set((op.waits_on ?? []).map(id));
        s.waits_on = s.waits_on.filter((w) => !drop.has(w));
      } else if (op.op === "delete") {
        const gone = find(op.step);
        p.steps = p.steps.filter((s) => s !== gone);
        for (const s of p.steps) if (s.waits_on.includes(gone.id)) s.waits_on = [...s.waits_on.filter((w) => w !== gone.id), ...gone.waits_on];
      }
    }
    refresh(p);
    return refs;
  };
  const fetchMock = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
    const url = typeof input === "string" ? input : input instanceof URL ? input.href : input.url;
    const u = new URL(url, "http://localhost");
    const body = init?.body ? (JSON.parse(String(init.body)) as Record<string, unknown>) : undefined;
    if (body) posts.push({ path: u.pathname, body });
    const plan = (ref: unknown) => plans.find((p) => String(p.id) === String(ref) || p.name === ref);
    switch (u.pathname) {
      case "/api/board":
        return json(makeBoard());
      case "/api/plans":
        if (!body) {
          return json({ plans: plans.map((p) => ({ id: p.id, name: p.name, rev: p.rev, steps: p.steps.length, done: 0, ready: 0, waiting: 0, blocked: 0 })) });
        }
        if (body.op === "add") {
          const p: WebPlan = { id: nextPlan++, name: String(body.name), rev: 1, steps: [] };
          plans.push(p);
          return json({ ok: true, plan: { id: p.id, name: p.name, rev: 1, steps: 0, done: 0, ready: 0, waiting: 0, blocked: 0 } });
        }
        if (body.op === "rename") {
          plan(body.plan)!.name = String(body.name);
          return json({ ok: true });
        }
        if (body.op === "remove") {
          plans.splice(plans.indexOf(plan(body.plan)!), 1);
          return json({ ok: true });
        }
        return json({ error: "op must be add, rename or remove" }, 400);
      case "/api/plan": {
        const p = plan(u.searchParams.get("plan"));
        return p ? json(p) : json({ error: "no such plan" }, 404);
      }
      case "/api/plan/ops": {
        const p = plan(body!.plan)!;
        const added = apply(p, body!.ops as Op[]);
        return json({ ok: true, plan: p, added });
      }
      default:
        return json({ ok: true });
    }
  });
  vi.stubGlobal("fetch", fetchMock);
  return { plans, posts };
}

const box = (title: string) => {
  const el = within(document.querySelector<HTMLElement>(".plan-canvas")!).getByText(title).closest<HTMLElement>(".plan-step");
  if (!el) throw new Error(`no box for ${title}`);
  return el;
};
const panel = () => screen.getByRole("complementary", { name: "step" });
const opsPosted = (posts: { path: string; body: Record<string, unknown> }[]) => posts.filter((p) => p.path === "/api/plan/ops").map((p) => p.body.ops);

async function openPlans() {
  render(<App />);
  fireEvent.click(await screen.findByRole("button", { name: "PLANS" }));
  await screen.findByText("World PR");
}

describe("editing a plan", () => {
  let daemon: ReturnType<typeof fakeDaemon>;
  beforeEach(() => {
    daemon = fakeDaemon();
  });
  afterEach(() => {
    cleanup();
    vi.unstubAllGlobals();
    vi.restoreAllMocks();
    localStorage.clear();
  });

  it("changes a step's state, name and note from its panel", async () => {
    await openPlans();
    fireEvent.click(box("World PR"));
    expect(within(panel()).getByRole("textbox", { name: "title" })).toHaveValue("World PR");
    expect(within(panel()).getByText(/Ready now/)).toBeInTheDocument();

    fireEvent.click(within(panel()).getByRole("button", { name: /Done/ }));
    await waitFor(() => expect(opsPosted(daemon.posts)).toContainEqual([{ op: "update", step: 2, state: "done" }]));
    await waitFor(() => expect(box("World PR").dataset.state).toBe("done"));
    await waitFor(() => expect(box("Land the World PR").dataset.state).toBe("ready"));

    fireEvent.change(within(panel()).getByRole("textbox", { name: "title" }), { target: { value: "World PR: remove the field" } });
    expect(box("World PR: remove the field")).toBeInTheDocument();
    fireEvent.change(within(panel()).getByRole("textbox", { name: "note" }), { target: { value: "regen schema" } });
    await waitFor(() =>
      expect(opsPosted(daemon.posts)).toContainEqual([{ op: "update", step: 2, title: "World PR: remove the field", note: "regen schema" }]),
    );
    expect(within(box("World PR: remove the field")).getByText("regen schema")).toBeInTheDocument();

    fireEvent.keyDown(document, { key: "Escape" });
    await waitFor(() => expect(screen.queryByRole("complementary", { name: "step" })).toBeNull());
  });

  it("adds the step that comes next with the + on a box", async () => {
    await openPlans();
    fireEvent.mouseEnter(box("Close #2389"));
    fireEvent.click(within(box("Close #2389")).getByRole("button", { name: "add the next step" }));
    const input = await screen.findByRole("textbox", { name: "new step" });
    await waitFor(() => expect(input).toHaveFocus());
    fireEvent.change(input, { target: { value: "Tell Logan it's gone" } });
    fireEvent.keyDown(input, { key: "Enter" });
    await waitFor(() => expect(opsPosted(daemon.posts)).toContainEqual([{ op: "add", title: "Tell Logan it's gone", waits_on: [5] }]));
    expect(await screen.findByText("Tell Logan it's gone")).toBeInTheDocument();
    expect(screen.queryByRole("textbox", { name: "new step" })).toBeNull();
  });

  it("adds a loose step, then links it to the step it holds up", async () => {
    await openPlans();
    fireEvent.click(screen.getByRole("button", { name: "+ STEP" }));
    const input = await screen.findByRole("textbox", { name: "new step" });
    fireEvent.change(input, { target: { value: "Fix the entityCountryCodes note" } });
    fireEvent.keyDown(input, { key: "Enter" });
    await waitFor(() => expect(opsPosted(daemon.posts)).toContainEqual([{ op: "add", title: "Fix the entityCountryCodes note" }]));
    await waitFor(() => expect(within(panel()).getByRole("textbox", { name: "title" })).toHaveValue("Fix the entityCountryCodes note"));

    const unblocks = within(panel()).getByRole("group", { name: "unblocks" });
    fireEvent.click(within(unblocks).getByRole("button", { name: "+ link a step" }));
    fireEvent.click(within(unblocks).getByRole("button", { name: "World PR" }));
    await waitFor(() => expect(opsPosted(daemon.posts)).toContainEqual([{ op: "link", step: 2, waits_on: [100] }]));
    await waitFor(() => expect(box("World PR").dataset.state).toBe("todo"));
    expect(within(unblocks).getByText("World PR")).toBeInTheDocument();
  });

  it("drops an arrow from the panel", async () => {
    await openPlans();
    fireEvent.click(box("Land the World PR"));
    const waits = within(panel()).getByRole("group", { name: "waits on" });
    fireEvent.click(within(waits).getByRole("button", { name: "stop waiting on World PR" }));
    await waitFor(() => expect(opsPosted(daemon.posts)).toContainEqual([{ op: "unlink", step: 3, waits_on: [2] }]));
    await waitFor(() => expect(box("Land the World PR").dataset.state).toBe("ready"));
  });

  it("deletes a step with its button, or ⌫ on the selected box", async () => {
    await openPlans();
    fireEvent.click(box("No errors for a day"));
    fireEvent.click(within(panel()).getByRole("button", { name: "DELETE STEP" }));
    await waitFor(() => expect(opsPosted(daemon.posts)).toContainEqual([{ op: "delete", step: 4 }]));
    await waitFor(() => expect(document.querySelector(".plan-canvas")).not.toHaveTextContent("No errors for a day"));
    expect(screen.queryByRole("complementary", { name: "step" })).toBeNull();

    fireEvent.click(box("Close #2389"));
    fireEvent.keyDown(document.body, { key: "Backspace" });
    await waitFor(() => expect(opsPosted(daemon.posts)).toContainEqual([{ op: "delete", step: 5 }]));
    await waitFor(() => expect(screen.queryByText("Close #2389")).toBeNull());
  });

  it("makes, renames and removes plans", async () => {
    vi.spyOn(window, "confirm").mockReturnValue(true);
    await openPlans();
    fireEvent.click(screen.getByRole("button", { name: "new plan" }));
    const name = screen.getByRole("textbox", { name: "plan name" });
    fireEvent.change(name, { target: { value: "Rollout" } });
    fireEvent.keyDown(name, { key: "Enter" });
    await waitFor(() => expect(daemon.posts).toContainEqual({ path: "/api/plans", body: { op: "add", name: "Rollout" } }));
    const tab = await screen.findByRole("button", { name: /Rollout/ });
    await waitFor(() => expect(tab).toHaveClass("on"));
    expect(await screen.findByText(/no steps yet/)).toBeInTheDocument();

    fireEvent.doubleClick(tab);
    const rename = screen.getByRole("textbox", { name: "plan name" });
    fireEvent.change(rename, { target: { value: "Rollout v2" } });
    fireEvent.keyDown(rename, { key: "Enter" });
    await waitFor(() => expect(daemon.posts).toContainEqual({ path: "/api/plans", body: { op: "rename", plan: 3, name: "Rollout v2" } }));
    expect(await screen.findByRole("button", { name: /Rollout v2/ })).toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: "remove plan" }));
    await waitFor(() => expect(daemon.posts).toContainEqual({ path: "/api/plans", body: { op: "remove", plan: 3 } }));
    await waitFor(() => expect(screen.queryByRole("button", { name: /Rollout v2/ })).toBeNull());
    await act(async () => {});
    expect(window.confirm).toHaveBeenCalled();
  });
});
