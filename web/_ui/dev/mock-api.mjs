// A stand-in for `driver serve` on 127.0.0.1:7781 while the Go daemon is not
// running: six jobs across the four columns plus one archived, the job-editor
// routes, and `{"ok":true}` for every action. `/api/term` answers 409 before
// any upgrade, which the UI shows as the "session ended" banner.
//
//   npm run mock      (then `npm run dev` in another shell)

import http from "node:http";

const PORT = Number(process.env.PORT || 7781);
const now = () => Math.floor(Date.now() / 1000);
const DEFAULT_DIR = "/Users/me/src";

let nextLinkId = 10;
const jobs = [
  { slug: "stripe-webhooks", dir: `${DEFAULT_DIR}/payments`, model: "", color: "blue", state: "draft", live: false, has_notes: true, mtime: now() - 400 },
  { slug: "flaky-ci", dir: `${DEFAULT_DIR}/platform`, model: "anthropic/claude-sonnet-4-5", color: "", state: "draft", live: false, has_notes: false, mtime: now() - 9000 },
  { slug: "search-reindex", dir: `${DEFAULT_DIR}/search`, model: "", color: "orange", state: "working", live: true, has_notes: false, mtime: now() - 60, started_at: now() - 600, pi: { state: "working", tool: "bash", ts: now() - 5 },
    links: [{ id: 1, kind: "github_pr", url: "https://github.com/acme/search/pull/482", status: "open", status_at: now() - 120 }] },
  { slug: "onboarding-copy", name: "Onboarding copy pass", dir: `${DEFAULT_DIR}/web`, model: "", color: "pink", state: "blocked", live: true, has_notes: true, mtime: now() - 1800, started_at: now() - 7200, pi: { state: "idle", ts: now() - 1700 },
    links: [{ id: 2, kind: "linear", url: "https://linear.app/acme/issue/WEB-231" }] },
  { slug: "rate-limits", dir: `${DEFAULT_DIR}/gateway`, model: "", color: "green", state: "done", live: false, has_notes: false, mtime: now() - 86400, started_at: now() - 90000, finished_at: now() - 86400, outcome: "accepted", reactivatable: true,
    links: [{ id: 3, kind: "github_pr", url: "https://github.com/acme/gateway/pull/77", status: "merged", status_at: now() - 80000 }] },
  { slug: "legacy-export", dir: `${DEFAULT_DIR}/exports`, model: "", color: "", state: "done", live: false, has_notes: false, mtime: now() - 172800, finished_at: now() - 172800, outcome: "accepted", reactivatable: false },
  { slug: "spike-graphql-12", display_slug: "spike-graphql", dir: `${DEFAULT_DIR}/api`, model: "", color: "purple", state: "done", live: false, has_notes: true, mtime: now() - 400000, finished_at: now() - 400000, archived: true },
];
const forms = new Map();
for (const j of jobs) {
  forms.set(j.slug, {
    slug: j.slug, name: j.name ?? "", dir: j.dir, model: j.model, color: j.color, spec: j.state === "draft" && j.slug === "flaky-ci" ? "" : `Investigate ${j.slug.replace(/-/g, " ")} and open a PR.\n\n- read the failing tests first\n- keep the diff small`,
    notes: j.has_notes ? `Started from the #${j.slug} thread. Check the staging dashboard after.` : "",
    state: j.state, editable: j.state === "draft" && !j.archived, archived: !!j.archived, mtime: j.mtime, links: j.links ?? [],
  });
}
const order = { draft: [], working: [], blocked: [], done: [] };

const find = (slug) => jobs.find((j) => j.slug === slug);
const touch = (j, patch) => Object.assign(j, patch, { mtime: now() });
const syncForm = (j) => { const f = forms.get(j.slug); if (f) Object.assign(f, { state: j.state, color: j.color, editable: j.state === "draft" && !j.archived, archived: !!j.archived, mtime: j.mtime, links: j.links ?? [] }); };

function board() {
  const sorted = [...jobs].sort((a, b) => {
    const ca = a.state, cb = b.state;
    if (ca !== cb) return 0;
    const oa = order[ca]?.indexOf(a.slug) ?? -1, ob = order[cb]?.indexOf(b.slug) ?? -1;
    return (oa < 0 ? 1e9 : oa) - (ob < 0 ? 1e9 : ob);
  });
  return { daemon: { pid: process.pid, started_at: now() - 3600, last_tick: now(), live: jobs.filter((j) => j.live).length }, daemon_ok: true, jobs: sorted, contract: 1, operator: process.env.USER || "me", default_dir: DEFAULT_DIR };
}

const send = (res, status, body) => {
  res.writeHead(status, { "content-type": "application/json", "cache-control": "no-cache" });
  res.end(JSON.stringify(body));
};
const err = (res, status, error) => send(res, status, { error });
const readBody = (req) => new Promise((resolve) => { let s = ""; req.on("data", (c) => (s += c)); req.on("end", () => { try { resolve(s ? JSON.parse(s) : {}); } catch { resolve({}); } }); });
const sleep = (ms) => new Promise((r) => setTimeout(r, ms));

const server = http.createServer(async (req, res) => {
  const url = new URL(req.url, `http://127.0.0.1:${PORT}`);
  const p = url.pathname;
  console.log(req.method, p + url.search);
  if (req.method === "GET") {
    if (p === "/api/health") return send(res, 200, { ok: true, pid: process.pid, version: "mock", started_at: now() - 3600 });
    if (p === "/api/board") return send(res, 200, board());
    if (p === "/api/job/form") { const f = forms.get(url.searchParams.get("slug")); return f ? send(res, 200, f) : err(res, 404, "no such job"); }
    if (p === "/api/dirs") { const q = url.searchParams.get("q") || DEFAULT_DIR + "/"; const base = q.endsWith("/") ? q : q.slice(0, q.lastIndexOf("/") + 1); const last = q.slice(base.length); return send(res, 200, { dirs: ["payments", "platform", "search", "web", "gateway", "exports", "api", "pi-ext"].filter((d) => d.startsWith(last)).map((d) => base + d) }); }
    if (p === "/api/models") return send(res, 200, { models: [{ provider: "anthropic", id: "claude-sonnet-4-5" }, { provider: "anthropic", id: "claude-opus-4-1" }, { provider: "openai", id: "gpt-5" }], default: "anthropic/claude-sonnet-4-5" });
    if (p === "/api/term") return err(res, find(url.searchParams.get("slug")) ? 409 : 404, find(url.searchParams.get("slug")) ? "no live session" : "no such job");
    return err(res, 404, "not found");
  }
  if (req.method !== "POST") return err(res, 405, "method");
  const b = await readBody(req);
  const j = b.slug ? find(b.slug) : undefined;
  if (p === "/api/job/create") {
    if (!b.slug || find(b.slug)) return err(res, 409, "slug taken");
    const nj = { slug: b.slug, name: (b.name ?? "").trim(), dir: DEFAULT_DIR, model: "", color: "", state: "draft", live: false, has_notes: false, mtime: now() };
    jobs.push(nj);
    const f = { slug: nj.slug, name: nj.name, dir: nj.dir, model: "", color: "", spec: "", notes: "", state: "draft", editable: true, archived: false, mtime: nj.mtime, links: [] };
    forms.set(nj.slug, f);
    return send(res, 200, f);
  }
  if (p === "/api/reorder") { if (!order[b.column]) return err(res, 409, "unknown column"); order[b.column] = b.slugs ?? []; return send(res, 200, { ok: true }); }
  if (!j) return err(res, 404, "no such job");
  switch (p) {
    case "/api/queue": {
      if (j.state !== "draft") return err(res, 409, "not a draft");
      if (!forms.get(j.slug)?.spec.trim()) return err(res, 409, "empty spec");
      await sleep(1500);
      touch(j, { state: "working", live: true, started_at: now(), pi: { state: "starting", ts: now() } });
      setTimeout(() => { if (j.state === "working") j.pi = { state: "working", tool: "read", ts: now() }; }, 4000);
      break;
    }
    case "/api/stop": if (!j.live) return err(res, 409, "no live session"); setTimeout(() => touch(j, { state: "blocked", pi: { state: "aborted", ts: now() } }), 800); break;
    case "/api/done": if (j.state === "draft" || j.state === "done") return err(res, 409, "draft or done"); touch(j, { state: "done", live: false, finished_at: now(), outcome: "accepted", reactivatable: true, pi: undefined }); break;
    case "/api/kill": if (j.state === "draft") return err(res, 409, "already a draft"); touch(j, { state: "draft", live: false, started_at: undefined, pi: undefined }); break;
    case "/api/reactivate": if (j.state !== "done") return err(res, 409, "not done"); if (j.archived) return err(res, 409, "archived"); if (j.reactivatable === false) return err(res, 409, "dir gone"); await sleep(800); touch(j, { state: "working", live: true, pi: { state: "starting", ts: now() } }); break;
    case "/api/archive": if (j.state !== "done") return err(res, 409, "not done"); if (j.live) return err(res, 409, "live session"); touch(j, { archived: true, display_slug: j.slug, slug: `${j.slug}-${Math.floor(Math.random() * 90 + 10)}` }); forms.set(j.slug, forms.get(b.slug)); forms.delete(b.slug); break;
    case "/api/unarchive": if (!j.archived) return err(res, 409, "not archived"); if (find(j.display_slug)) return err(res, 409, "slug taken"); forms.set(j.display_slug, forms.get(j.slug)); forms.delete(j.slug); touch(j, { archived: false, slug: j.display_slug, display_slug: undefined }); break;
    case "/api/delete": if (!j.archived) return err(res, 409, "not archived"); jobs.splice(jobs.indexOf(j), 1); forms.delete(j.slug); break;
    case "/api/job/color": if (b.color && !["red", "blue", "green", "yellow", "purple", "orange", "pink", "cyan"].includes(b.color)) return err(res, 400, "unknown colour"); touch(j, { color: b.color }); break;
    case "/api/job/rename": { if (!/^[a-z0-9][a-z0-9-]{0,63}$/.test(b.to || "")) return err(res, 409, "invalid"); if (b.to !== j.slug && find(b.to)) return err(res, 409, "taken"); const name = (b.name ?? "").trim(); const f = forms.get(j.slug); forms.delete(j.slug); touch(j, { slug: b.to, name: name || undefined }); if (f) { f.slug = b.to; f.name = name; forms.set(b.to, f); } syncForm(j); return send(res, 200, { ok: true, slug: b.to }); }
    case "/api/job/notes": { const f = forms.get(j.slug); if (f) f.notes = b.text ?? ""; touch(j, { has_notes: !!(b.text ?? "").trim() }); break; }
    case "/api/job/links": {
      j.links = j.links ?? [];
      if (b.op === "add") { if (!/^https?:\/\//.test(b.url || "")) return err(res, 409, "bad URL"); const pr = /github\.com\/[^/]+\/[^/]+\/pull\/\d+/.test(b.url); const link = { id: nextLinkId++, kind: pr ? "github_pr" : /github\.com/.test(b.url) ? "github" : /slack\.com/.test(b.url) ? "slack" : /linear\.app/.test(b.url) ? "linear" : "url", url: b.url, ...(pr ? { status: "open", status_at: now() } : {}) }; j.links.push(link); touch(j, {}); syncForm(j); return send(res, 200, { ok: true, link }); }
      if (b.op === "remove") { j.links = j.links.filter((l) => l.id !== b.id); touch(j, {}); break; }
      return err(res, 400, "bad op");
    }
    case "/api/job/saveform": { const f = forms.get(j.slug); if (!f) return err(res, 404, "no such job"); if (!f.editable) return err(res, 409, "not editable"); if (b.mtime && b.mtime !== f.mtime) return err(res, 409, "edited elsewhere — reload"); Object.assign(f, { dir: b.dir ?? f.dir, model: b.model ?? f.model, spec: b.spec ?? f.spec }); touch(j, { dir: f.dir, model: f.model }); f.mtime = j.mtime; return send(res, 200, { ok: true, mtime: f.mtime }); }
    default: return err(res, 404, "not found");
  }
  syncForm(j);
  return send(res, 200, { ok: true });
});

server.listen(PORT, "127.0.0.1", () => console.log(`mock driver api on http://127.0.0.1:${PORT}`));
