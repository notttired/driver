import assert from "node:assert/strict";
import { mkdtempSync } from "node:fs";
import { createServer, type IncomingMessage, type Server } from "node:http";
import type { AddressInfo } from "node:net";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { after, before, beforeEach, describe, it } from "node:test";

import driver from "./driver.ts";

type Seen = { method: string; url: string; body: unknown };
type Tool = {
  name: string;
  execute: (id: string, params: Record<string, unknown>, signal?: AbortSignal) => Promise<{ content: { type: string; text: string }[] }>;
};

const load = (env: Record<string, string | undefined>): Map<string, Tool> => {
  const saved = { ...process.env };
  Object.assign(process.env, env);
  for (const [k, v] of Object.entries(env)) if (v === undefined) delete process.env[k];
  const tools = new Map<string, Tool>();
  try {
    driver({ on() {}, registerTool: (t: Tool) => tools.set(t.name, t) } as never);
  } finally {
    process.env = saved;
  }
  return tools;
};

const text = (r: { content: { type: string; text: string }[] }) => r.content.map((c) => c.text).join("");

describe("driver_notes", () => {
  const jobDir = join(mkdtempSync(join(tmpdir(), "driver-ext-")), "my-job-7");
  let server: Server;
  let serve: string;
  let seen: Seen[] = [];
  let reply: { status: number; body: unknown } = { status: 200, body: { ok: true } };

  before(async () => {
    server = createServer(async (req: IncomingMessage, res) => {
      let raw = "";
      for await (const chunk of req) raw += chunk;
      seen.push({ method: req.method ?? "", url: req.url ?? "", body: raw ? JSON.parse(raw) : undefined });
      res.writeHead(reply.status, { "Content-Type": "application/json" });
      res.end(JSON.stringify(reply.body));
    });
    await new Promise<void>((r) => server.listen(0, "127.0.0.1", r));
    serve = `http://127.0.0.1:${(server.address() as AddressInfo).port}`;
  });
  after(() => server.close());
  beforeEach(() => {
    seen = [];
    reply = { status: 200, body: { ok: true } };
  });

  const tool = () => {
    const t = load({ DRIVER_JOB_DIR: jobDir, DRIVER_SERVE: serve }).get("driver_notes");
    assert.ok(t, "driver_notes is registered in a driver job");
    return t;
  };

  it("reads the notes of the job pi runs in", async () => {
    reply = { status: 200, body: { text: "check staging after the deploy" } };
    const out = await tool().execute("c1", { action: "read" });
    assert.equal(text(out), "check staging after the deploy");
    assert.deepEqual(seen, [{ method: "GET", url: `/api/job/notes?job_dir=${encodeURIComponent(jobDir)}`, body: undefined }]);
  });

  it("says so when there are none", async () => {
    reply = { status: 200, body: { text: "" } };
    assert.equal(text(await tool().execute("c1", { action: "read" })), "The notes are empty.");
  });

  it("adds to the end of the notes", async () => {
    const out = await tool().execute("c1", { action: "append", text: "PR: https://github.com/o/r/pull/7" });
    assert.equal(text(out), "Added to the notes.");
    assert.deepEqual(seen, [
      { method: "POST", url: "/api/job/notes", body: { job_dir: jobDir, op: "append", text: "PR: https://github.com/o/r/pull/7" } },
    ]);
  });

  it("rewrites the notes whole", async () => {
    const out = await tool().execute("c1", { action: "replace", text: "only this" });
    assert.equal(text(out), "The notes now read as given.");
    assert.deepEqual(seen, [{ method: "POST", url: "/api/job/notes", body: { job_dir: jobDir, op: "replace", text: "only this" } }]);
  });

  it("fails with the daemon's reason", async () => {
    reply = { status: 409, body: { error: "notes exceed 64000 characters" } };
    await assert.rejects(tool().execute("c1", { action: "append", text: "x" }), { message: "notes exceed 64000 characters" });
  });

  it("wants text to add or write", async () => {
    await assert.rejects(tool().execute("c1", { action: "append" }), { message: "append needs text" });
    await assert.rejects(tool().execute("c1", { action: "replace" }), { message: "replace needs text" });
    assert.deepEqual(seen, []);
  });

  it("is not there outside a driver job", () => {
    assert.equal(load({ DRIVER_JOB_DIR: undefined, DRIVER_SERVE: serve }).has("driver_notes"), false);
    assert.equal(load({ DRIVER_JOB_DIR: jobDir, DRIVER_SERVE: undefined }).has("driver_notes"), false);
  });
});

type Handler = (event: Record<string, unknown>, ctx: Record<string, unknown>) => unknown;

/** Loads the extension capturing its tools AND its event handlers. */
const loadAll = (env: Record<string, string | undefined>) => {
  const saved = { ...process.env };
  Object.assign(process.env, env);
  for (const [k, v] of Object.entries(env)) if (v === undefined) delete process.env[k];
  const tools = new Map<string, Tool>();
  const handlers = new Map<string, Handler[]>();
  const sessionNames: string[] = [];
  try {
    driver({
      on: (name: string, h: Handler) => handlers.set(name, [...(handlers.get(name) ?? []), h]),
      registerTool: (t: Tool) => tools.set(t.name, t),
      setSessionName: (name: string) => sessionNames.push(name),
    } as never);
  } finally {
    process.env = saved;
  }
  const fire = async (name: string, event: Record<string, unknown>, ctx: Record<string, unknown>) => {
    let out: unknown;
    for (const h of handlers.get(name) ?? []) out = (await h(event, ctx)) ?? out;
    return out as Record<string, unknown> | undefined;
  };
  return { tools, fire, sessionNames };
};

describe("threads: shared context", () => {
  const jobDir = join(mkdtempSync(join(tmpdir(), "driver-ext-")), "job-3", "t2");
  let server: Server;
  let serve: string;
  let seen: Seen[] = [];
  let reply: (url: string) => { status: number; body: unknown } = () => ({ status: 200, body: {} });

  before(async () => {
    server = createServer(async (req: IncomingMessage, res) => {
      let raw = "";
      for await (const chunk of req) raw += chunk;
      seen.push({ method: req.method ?? "", url: req.url ?? "", body: raw ? JSON.parse(raw) : undefined });
      const r = reply(req.url ?? "");
      res.writeHead(r.status, { "Content-Type": "application/json" });
      res.end(JSON.stringify(r.body));
    });
    await new Promise<void>((r) => server.listen(0, "127.0.0.1", r));
    serve = `http://127.0.0.1:${(server.address() as AddressInfo).port}`;
  });
  after(() => server.close());
  beforeEach(() => {
    seen = [];
    reply = () => ({ status: 200, body: {} });
  });

  const branch = [
    { type: "session", id: "h" },
    { type: "message", id: "u1", message: { role: "user", content: "Go." } },
    { type: "message", id: "a1", message: { role: "assistant", content: [] } },
    { type: "custom_message", id: "x1", customType: "driver-peers", content: "earlier", details: { carried: [{ thread: 0, entry: "u9" }, { thread: 0, entry: "a9" }] } },
    { type: "custom_message", id: "x2", customType: "other", content: "noise", details: { carried: [{ thread: 0, entry: "zz" }] } },
  ];
  const ctx = (extra: Record<string, unknown> = {}) => ({
    sessionManager: { getBranch: () => branch, getSessionFile: () => join(jobDir, "session.jsonl") },
    isIdle: () => true,
    ...extra,
  });
  const update = { text: "Context from the other threads…", carried: [{ thread: 0, entry: "u10" }], peers: 1 };

  it("asks the daemon what the other threads said, with what it already holds, and adds it to the prompt", async () => {
    reply = () => ({ status: 200, body: update });
    const { fire } = loadAll({ DRIVER_JOB_DIR: jobDir, DRIVER_SERVE: serve });
    const sections: Record<string, string> = {};
    const out = await fire("before_agent_start", { messageType: "user", prompt: "hi", systemPromptOptions: { sections } }, ctx());
    assert.deepEqual(seen.map((s) => [s.method, s.url]), [["POST", "/api/job/threads/update"]]);
    const body = seen[0]!.body as { job_dir: string; seen: string[] };
    assert.equal(body.job_dir, jobDir);
    assert.deepEqual([...body.seen].sort(), ["a1", "a9", "h", "u1", "u9", "x1", "x2"].sort(), "every id on the branch and every id the driver-peers messages carried");
    assert.deepEqual(out?.message, { customType: "driver-peers", content: update.text, display: true, details: { carried: update.carried } });
    assert.match(sections.driver_threads ?? "", /one of several threads/);
    assert.match(sections.driver_threads ?? "", /git worktree/);
    assert.match(sections.driver_threads ?? "", /stop and tell the user/);
    assert.match(sections.driver_threads ?? "", /When the user tells you what this thread is for, name your tab/);
    assert.doesNotMatch(sections.driver_threads ?? "", /first tab keeps/);
  });

  it("adds nothing when there is nothing new, and no rules when the job has one pi", async () => {
    reply = () => ({ status: 200, body: { text: "", carried: [], peers: 0 } });
    const { fire } = loadAll({ DRIVER_JOB_DIR: jobDir, DRIVER_SERVE: serve });
    const sections: Record<string, string> = {};
    const out = await fire("before_agent_start", { messageType: "user", prompt: "hi", systemPromptOptions: { sections } }, ctx());
    assert.equal(out?.message, undefined);
    assert.equal(sections.driver_threads, undefined);
  });

  it("keeps the rules once the job has threads, even with nothing new", async () => {
    reply = () => ({ status: 200, body: { text: "", carried: [], peers: 2 } });
    const { fire } = loadAll({ DRIVER_JOB_DIR: jobDir, DRIVER_SERVE: serve });
    const sections: Record<string, string> = {};
    await fire("before_agent_start", { messageType: "user", prompt: "hi", systemPromptOptions: { sections } }, ctx());
    assert.match(sections.driver_threads ?? "", /one of several threads/);
  });

  it("lets a run start when the daemon is unreachable", async () => {
    const { fire } = loadAll({ DRIVER_JOB_DIR: jobDir, DRIVER_SERVE: "http://127.0.0.1:1" });
    const out = await fire("before_agent_start", { messageType: "user", prompt: "hi", systemPromptOptions: { sections: {} } }, ctx());
    assert.equal(out?.message, undefined);
  });

  it("leaves runs started by other extensions' messages alone", async () => {
    reply = () => ({ status: 200, body: update });
    const { fire } = loadAll({ DRIVER_JOB_DIR: jobDir, DRIVER_SERVE: serve });
    const out = await fire("before_agent_start", { messageType: "custom", message: {}, systemPromptOptions: { sections: {} } }, ctx());
    assert.equal(out, undefined);
    assert.deepEqual(seen, []);
  });

  it("puts the update before a message of yours that was waiting for the turn to end", async () => {
    reply = () => ({ status: 200, body: update });
    const { fire } = loadAll({ DRIVER_JOB_DIR: jobDir, DRIVER_SERVE: serve });
    const prior = [{ type: "custom", customType: "x", data: 1 }];
    const out = await fire(
      "turn_end",
      { entries: prior, context: { pendingMessages: [{ role: "user", content: "and this" }] }, outcome: "completed" },
      ctx(),
    );
    assert.deepEqual(out?.entries, [...prior, { type: "custom_message", customType: "driver-peers", content: update.text, display: true, details: { carried: update.carried } }]);
  });

  it("does nothing at a turn's end when no message of yours is waiting", async () => {
    reply = () => ({ status: 200, body: update });
    const { fire } = loadAll({ DRIVER_JOB_DIR: jobDir, DRIVER_SERVE: serve });
    const out = await fire("turn_end", { entries: [], context: { pendingMessages: [] }, outcome: "completed" }, ctx());
    assert.equal(out, undefined);
    assert.deepEqual(seen, []);
  });

  it("gives pi the driver_threads tool: the list and any thread's history", async () => {
    reply = (url) =>
      url.startsWith("/api/job/threads/history")
        ? { status: 200, body: { text: "[10:00] user: Go.\n[10:01] assistant: Done." } }
        : { status: 200, body: { self: 2, threads: [{ id: 0, name: "Multi", state: "waiting" }, { id: 2, name: "CI", state: "working" }] } };
    const { tools } = loadAll({ DRIVER_JOB_DIR: jobDir, DRIVER_SERVE: serve });
    const tool = tools.get("driver_threads");
    assert.ok(tool, "driver_threads is registered in a driver job");
    assert.equal(text(await tool.execute("c1", { action: "list" })), "Threads of this job:\n- 0: Multi (waiting)\n- 2: CI (working) — you");
    assert.equal(text(await tool.execute("c2", { action: "history", thread: 0 })), "[10:00] user: Go.\n[10:01] assistant: Done.");
    assert.equal(seen[1]!.url, `/api/job/threads/history?job_dir=${encodeURIComponent(jobDir)}&thread=0`);
    await assert.rejects(tool.execute("c3", { action: "history" }), { message: "history needs a thread id" });
    reply = () => ({ status: 404, body: { error: "no such thread" } });
    await assert.rejects(tool.execute("c4", { action: "history", thread: 9 }), { message: "no such thread" });
  });

  it("lets a thread name its own tab, and pi's session with it", async () => {
    reply = () => ({ status: 200, body: { ok: true, name: "Topic E", pi_name: "Multi · Topic E" } });
    const { tools, sessionNames } = loadAll({ DRIVER_JOB_DIR: jobDir, DRIVER_SERVE: serve });
    const tool = tools.get("driver_threads")!;
    assert.equal(text(await tool.execute("c1", { action: "rename", name: "Topic E" })), 'Your tab is now named "Topic E".');
    assert.deepEqual(seen, [{ method: "POST", url: "/api/job/threads/rename", body: { job_dir: jobDir, name: "Topic E" } }]);
    assert.deepEqual(sessionNames, ["Multi · Topic E"]);
  });

  it("renames nothing without a name, or when the daemon refuses", async () => {
    const { tools, sessionNames } = loadAll({ DRIVER_JOB_DIR: jobDir, DRIVER_SERVE: serve });
    const tool = tools.get("driver_threads")!;
    await assert.rejects(tool.execute("c1", { action: "rename" }), { message: "rename needs a name" });
    assert.deepEqual(seen, []);
    reply = () => ({ status: 409, body: { error: "a thread needs a name" } });
    await assert.rejects(tool.execute("c2", { action: "rename", name: " " }), { message: "a thread needs a name" });
    assert.deepEqual(sessionNames, []);
  });
});

describe("driver_plans", () => {
  const jobDir = join(mkdtempSync(join(tmpdir(), "driver-ext-")), "job-9");
  let server: Server;
  let serve: string;
  let seen: Seen[] = [];
  let reply: (url: string) => { status: number; body: unknown } = () => ({ status: 200, body: { ok: true } });

  before(async () => {
    server = createServer(async (req: IncomingMessage, res) => {
      let raw = "";
      for await (const chunk of req) raw += chunk;
      seen.push({ method: req.method ?? "", url: req.url ?? "", body: raw ? JSON.parse(raw) : undefined });
      const r = reply(req.url ?? "");
      res.writeHead(r.status, { "Content-Type": "application/json" });
      res.end(JSON.stringify(r.body));
    });
    await new Promise<void>((r) => server.listen(0, "127.0.0.1", r));
    serve = `http://127.0.0.1:${(server.address() as AddressInfo).port}`;
  });
  after(() => server.close());
  beforeEach(() => {
    seen = [];
    reply = () => ({ status: 200, body: { ok: true } });
  });

  const tool = () => {
    const t = load({ DRIVER_JOB_DIR: jobDir, DRIVER_SERVE: serve }).get("driver_plans");
    assert.ok(t, "driver_plans is registered in a driver job");
    return t;
  };

  const plan = {
    id: 2,
    name: "Field removal",
    rev: 7,
    steps: [
      { id: 1, title: "Abacus #7711", note: "merged", state: "done", ready: false, kind: "PR", topic: "", waits_on: [] },
      { id: 2, title: "No calls to the field", note: "", state: "done", ready: false, kind: "", topic: "", waits_on: [1] },
      { id: 3, title: "World PR", note: "regen schema\nspec it's gone", state: "todo", ready: true, kind: "PR", topic: "World", waits_on: [2] },
      { id: 4, title: "BFCM freeze", note: "until 1 Dec", state: "wait", ready: false, kind: "date", topic: "", waits_on: [] },
      { id: 5, title: "Land the World PR", note: "", state: "todo", ready: false, kind: "", topic: "World", waits_on: [3, 4] },
      { id: 6, title: "Flag paused", note: "", state: "blocked", ready: false, kind: "", topic: "", waits_on: [] },
    ],
  };
  const planText = [
    'Plan "Field removal" (id 2): 6 steps — 2 done, 1 ready, 1 waiting, 1 blocked.',
    "",
    "Topic World:",
    "- 3 · ready · World PR [PR] — regen schema / spec it's gone · waits on 2",
    "- 5 · later · Land the World PR · waits on 3, 4",
    "",
    "No topic:",
    "- 1 · done · Abacus #7711 [PR] — merged",
    "- 2 · done · No calls to the field · waits on 1",
    "- 4 · waiting · BFCM freeze [date] — until 1 Dec",
    "- 6 · blocked · Flag paused",
  ].join("\n");

  it("lists every plan with how its steps stand", async () => {
    reply = () => ({
      status: 200,
      body: { plans: [{ id: 1, name: "Monorail sunset", rev: 3, steps: 38, done: 10, ready: 6, waiting: 6, blocked: 3 }, { id: 2, name: "Field removal", rev: 1, steps: 0, done: 0, ready: 0, waiting: 0, blocked: 0 }] },
    });
    const out = await tool().execute("c1", { action: "list" });
    assert.equal(
      text(out),
      "Plans:\n- 1: Monorail sunset — 38 steps: 10 done, 6 ready, 6 waiting, 3 blocked\n- 2: Field removal — no steps yet",
    );
    assert.deepEqual(seen, [{ method: "GET", url: "/api/plans", body: undefined }]);
  });

  it("says so when there are no plans", async () => {
    reply = () => ({ status: 200, body: { plans: [] } });
    assert.equal(text(await tool().execute("c1", { action: "list" })), "There are no plans yet.");
  });

  it("reads any plan, by name or id, step by step under its topics", async () => {
    reply = () => ({ status: 200, body: plan });
    assert.equal(text(await tool().execute("c1", { action: "get", plan: "Field removal" })), planText);
    await tool().execute("c2", { action: "get", plan: 2 });
    assert.deepEqual(seen.map((s) => s.url), [`/api/plan?plan=${encodeURIComponent("Field removal")}`, "/api/plan?plan=2"]);
  });

  it("makes, renames and removes plans", async () => {
    reply = () => ({ status: 200, body: { ok: true, plan: { id: 4, name: "Rollout", rev: 1 } } });
    assert.equal(text(await tool().execute("c1", { action: "create", name: "Rollout" })), 'Made plan "Rollout" (id 4).');
    reply = () => ({ status: 200, body: { ok: true } });
    assert.equal(text(await tool().execute("c2", { action: "rename", plan: 4, name: "Rollout v2" })), 'Renamed the plan to "Rollout v2".');
    assert.equal(text(await tool().execute("c3", { action: "delete", plan: "Rollout v2" })), "Removed the plan and its steps.");
    assert.deepEqual(seen, [
      { method: "POST", url: "/api/plans", body: { op: "add", name: "Rollout" } },
      { method: "POST", url: "/api/plans", body: { op: "rename", plan: 4, name: "Rollout v2" } },
      { method: "POST", url: "/api/plans", body: { op: "remove", plan: "Rollout v2" } },
    ]);
  });

  it("edits a plan in one batch and shows it as it now stands", async () => {
    reply = () => ({ status: 200, body: { ok: true, plan, added: { tell: 6 } } });
    const ops = [
      { op: "add", ref: "tell", title: "Tell Logan", waits_on: [2] },
      { op: "update", step: 3, state: "todo" },
    ];
    const out = await tool().execute("c1", { action: "edit", plan: "Field removal", ops });
    assert.equal(text(out), `Made 2 changes. New steps: tell = 6.\n\n${planText}`);
    assert.deepEqual(seen, [{ method: "POST", url: "/api/plan/ops", body: { plan: "Field removal", ops } }]);
  });

  it("fails with the daemon's reason, having changed nothing", async () => {
    reply = () => ({ status: 409, body: { error: "op 2: step 1 waiting on 5 would make a loop" } });
    await assert.rejects(tool().execute("c1", { action: "edit", plan: 2, ops: [{ op: "link", step: 1, waits_on: [5] }] }), {
      message: "op 2: step 1 waiting on 5 would make a loop",
    });
  });

  it("wants what each action needs", async () => {
    await assert.rejects(tool().execute("c1", { action: "get" }), { message: "get needs a plan (its name or id)" });
    await assert.rejects(tool().execute("c1", { action: "create" }), { message: "create needs a name" });
    await assert.rejects(tool().execute("c1", { action: "rename", plan: 1 }), { message: "rename needs a name" });
    await assert.rejects(tool().execute("c1", { action: "edit", plan: 1 }), { message: "edit needs ops" });
    await assert.rejects(tool().execute("c1", { action: "edit", plan: 1, ops: [] }), { message: "edit needs ops" });
    assert.deepEqual(seen, []);
  });

  it("is for every pi on the board, and only when the user asks", () => {
    const t = load({ DRIVER_JOB_DIR: jobDir, DRIVER_SERVE: serve }).get("driver_plans") as unknown as { promptGuidelines: string[]; description: string };
    assert.match(t.promptGuidelines.join("\n"), /only when the user asks/);
    assert.match(t.promptGuidelines.join("\n"), /any plan/);
    assert.match(t.description, /not tied to this job/);
    assert.equal(load({ DRIVER_JOB_DIR: undefined, DRIVER_SERVE: serve }).has("driver_plans"), false);
    assert.equal(load({ DRIVER_JOB_DIR: jobDir, DRIVER_SERVE: undefined }).has("driver_plans"), false);
  });
});
