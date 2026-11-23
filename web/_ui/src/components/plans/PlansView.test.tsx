import { act, cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { App } from "../../App.tsx";
import type { WebPlan, WebPlanStep, WebPlanSummary } from "../../lib/types.ts";
import { makeBoard, makeJob } from "../../test/fixtures.ts";

const json = (body: unknown, status = 200) =>
  new Response(JSON.stringify(body), { status, headers: { "content-type": "application/json" } });

const step = (over: Partial<WebPlanStep> & Pick<WebPlanStep, "id" | "title">): WebPlanStep => ({
  note: "",
  state: "todo",
  ready: false,
  kind: "",
  topic: "",
  waits_on: [],
  ...over,
});

function fieldRemoval(): WebPlan {
  return {
    id: 2,
    name: "Field removal",
    rev: 4,
    steps: [
      step({ id: 1, title: "Abacus #7711", state: "done", note: "merged" }),
      step({ id: 2, title: "Abacus #7712", state: "done", note: "merged" }),
      step({ id: 3, title: "No calls to the field", state: "done", waits_on: [1, 2] }),
      step({ id: 4, title: "World PR", ready: true, waits_on: [3], topic: "World" }),
      step({ id: 5, title: "BFCM freeze", state: "wait", note: "until 1 Dec" }),
      step({ id: 6, title: "Land the World PR", waits_on: [4, 5], topic: "World" }),
      step({ id: 7, title: "Refresh the schema", waits_on: [6] }),
      step({ id: 8, title: "Flag paused", state: "blocked" }),
    ],
  };
}

const monorail: WebPlan = {
  id: 1,
  name: "Monorail sunset",
  rev: 1,
  steps: [step({ id: 11, title: "Move to Observe", state: "done", topic: "H" })],
};

const summary = (p: WebPlan): WebPlanSummary => ({
  id: p.id,
  name: p.name,
  rev: p.rev,
  steps: p.steps.length,
  done: p.steps.filter((s) => s.state === "done").length,
  ready: p.steps.filter((s) => s.ready).length,
  waiting: p.steps.filter((s) => s.state === "wait").length,
  blocked: p.steps.filter((s) => s.state === "blocked").length,
});

let plans: WebPlan[] = [];
const requests: string[] = [];

function installFetch() {
  const fetchMock = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
    const url = typeof input === "string" ? input : input instanceof URL ? input.href : input.url;
    const u = new URL(url, "http://localhost");
    requests.push(`${init?.method ?? "GET"} ${u.pathname}${u.search}`);
    switch (u.pathname) {
      case "/api/board":
        return json(makeBoard({ jobs: [makeJob({ slug: "er-verifier", state: "working" })] }));
      case "/api/plans":
        return json({ plans: plans.map(summary) });
      case "/api/plan": {
        const ref = u.searchParams.get("plan");
        const p = plans.find((x) => String(x.id) === ref || x.name === ref);
        return p ? json(p) : json({ error: "no such plan" }, 404);
      }
      default:
        return json({ ok: true });
    }
  });
  vi.stubGlobal("fetch", fetchMock);
}

const box = (title: string) => {
  const el = screen.getByText(title).closest<HTMLElement>(".plan-step");
  if (!el) throw new Error(`no box for ${title}`);
  return el;
};
const at = (el: HTMLElement) => {
  const m = /translate\(([-\d.]+)px, ([-\d.]+)px\)/.exec(el.style.transform);
  if (!m) throw new Error(`no position on ${el.textContent}: ${el.style.transform}`);
  return { x: Number(m[1]), y: Number(m[2]) };
};
const settle = () => act(() => new Promise((r) => setTimeout(r, 800)));

async function openPlans() {
  render(<App />);
  fireEvent.click(await screen.findByRole("button", { name: "PLANS" }));
}

describe("PLANS", () => {
  beforeEach(() => {
    requests.length = 0;
    plans = [monorail, fieldRemoval()];
    installFetch();
  });
  afterEach(() => {
    cleanup();
    vi.unstubAllGlobals();
    localStorage.clear();
  });

  it("is a view of its own beside the board, and the board shows no plans", async () => {
    render(<App />);
    expect(await screen.findByText("er-verifier")).toBeInTheDocument();
    expect(screen.queryByText("Monorail sunset")).toBeNull();
    expect(requests.some((r) => r.includes("/api/plan"))).toBe(false);

    fireEvent.click(screen.getByRole("button", { name: "PLANS" }));
    expect(await screen.findByRole("button", { name: /Monorail sunset/ })).toBeInTheDocument();
    expect(screen.queryByText("er-verifier")).toBeNull();
    expect(await screen.findByText("Move to Observe")).toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: "BOARD" }));
    expect(await screen.findByText("er-verifier")).toBeInTheDocument();
  });

  it("remembers the view and the plan it was left on", async () => {
    await openPlans();
    fireEvent.click(await screen.findByRole("button", { name: /Field removal/ }));
    expect(await screen.findByText("World PR")).toBeInTheDocument();
    cleanup();
    render(<App />);
    expect(await screen.findByText("World PR")).toBeInTheDocument();
    expect(requests).toContain("GET /api/plan?plan=2");
  });

  it("lays steps out left to right by what they wait on", async () => {
    localStorage.setItem("driver.plan", "2");
    await openPlans();
    await screen.findByText("Refresh the schema");
    await settle();
    const [a, b, calls, pr, freeze, land, refresh] = [
      "Abacus #7711",
      "Abacus #7712",
      "No calls to the field",
      "World PR",
      "BFCM freeze",
      "Land the World PR",
      "Refresh the schema",
    ].map((t) => at(box(t)));
    expect(a!.x).toBe(b!.x);
    expect(calls!.x).toBeGreaterThan(a!.x);
    expect(pr!.x).toBeGreaterThan(calls!.x);
    expect(land!.x).toBeGreaterThan(pr!.x);
    expect(freeze!.x).toBe(pr!.x);
    expect(refresh!.x).toBeGreaterThan(land!.x);
    expect(pr!.y).toBe(land!.y);
    expect(pr!.y).toBeLessThan(a!.y);

    const states = Object.fromEntries(
      ["Abacus #7711", "World PR", "BFCM freeze", "Land the World PR", "Flag paused"].map((t) => [t, box(t).dataset.state]),
    );
    expect(states).toEqual({ "Abacus #7711": "done", "World PR": "ready", "BFCM freeze": "wait", "Land the World PR": "todo", "Flag paused": "blocked" });
    expect(within(box("BFCM freeze")).getByText("until 1 Dec")).toBeInTheDocument();
    expect(document.querySelectorAll(".plan-edge")).toHaveLength(6);
  });

  it("folds a topic into one box and back", async () => {
    localStorage.setItem("driver.plan", "2");
    await openPlans();
    fireEvent.click(await screen.findByRole("button", { name: /^World/ }));
    await waitFor(() => expect(screen.queryByText("World PR")).toBeNull());
    const folded = screen.getByText("World").closest<HTMLElement>(".plan-step")!;
    expect(folded).toHaveClass("folded");
    expect(within(folded).getByText("0/2 done · 1 ready")).toBeInTheDocument();
    fireEvent.click(folded);
    expect(await screen.findByText("World PR")).toBeInTheDocument();
  });

  it("filters to what is ready, waiting or blocked, and traces a step's chain", async () => {
    localStorage.setItem("driver.plan", "2");
    await openPlans();
    await screen.findByText("World PR");
    const filters = screen.getByRole("group", { name: "filter steps" });
    fireEvent.click(within(filters).getByRole("button", { name: /Ready 1/ }));
    expect(box("World PR")).not.toHaveClass("dim");
    expect(box("BFCM freeze")).toHaveClass("dim");
    fireEvent.click(within(filters).getByRole("button", { name: /Waiting 1/ }));
    expect(box("BFCM freeze")).not.toHaveClass("dim");
    expect(within(filters).getByRole("button", { name: /Blocked 1/ })).toBeInTheDocument();
    fireEvent.click(within(filters).getByRole("button", { name: "All" }));
    expect(box("World PR")).not.toHaveClass("dim");

    fireEvent.mouseEnter(box("Land the World PR"));
    expect(box("World PR")).toHaveClass("up");
    expect(box("Abacus #7711")).toHaveClass("up");
    expect(box("Refresh the schema")).toHaveClass("down");
    expect(box("Flag paused")).toHaveClass("dim");
    fireEvent.mouseLeave(box("Land the World PR"));
    expect(box("Flag paused")).not.toHaveClass("dim");
  });

  it("shows what changes on the daemon — a pi's edits — without a reload", async () => {
    localStorage.setItem("driver.plan", "2");
    await openPlans();
    await screen.findByText("World PR");
    const p = plans[1]!;
    plans[1] = { ...p, rev: p.rev + 1, steps: [...p.steps, step({ id: 9, title: "Tell Logan it's gone", waits_on: [7] })] };
    expect(await screen.findByText("Tell Logan it's gone", undefined, { timeout: 4500 })).toBeInTheDocument();
  }, 10_000);

  it("says how to start when there are no plans", async () => {
    plans = [];
    await openPlans();
    expect(await screen.findByText(/No plans yet/)).toBeInTheDocument();
  });
});
