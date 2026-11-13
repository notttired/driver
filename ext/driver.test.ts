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
