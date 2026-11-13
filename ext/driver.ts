// ext/driver.ts — the pi extension every driver job loads with `-e`. It tells
// the daemon what pi is doing by writing <DRIVER_JOB_DIR>/status.json, which
// the tick loop reads (docs/DESIGN.md "State derivation"), and gives pi the
// driver_notes tool over the operator's notes. A no-op unless DRIVER_JOB_DIR
// is set, so a pi started by hand with this file loaded does nothing. Nothing
// long-lived here: no timers, no sockets — one file write per lifecycle
// event, and never a throw into pi outside the tool's own result.
import { mkdirSync, renameSync, writeFileSync } from "node:fs";
import { join } from "node:path";
import type { ExtensionAPI, ExtensionContext } from "@earendil-works/pi-coding-agent";

type State = "starting" | "ready" | "working" | "idle" | "dialog" | "aborted" | "exited";

type NotesParams = { action: "read" | "append" | "replace"; text?: string };

const registerNotesTool = (pi: ExtensionAPI, jobDir: string, serve: string) => {
  const call = async (path: string, init: RequestInit, signal?: AbortSignal) => {
    const res = await fetch(serve + path, { ...init, signal });
    const body = (await res.json().catch(() => ({}))) as { text?: string; error?: string };
    if (!res.ok) throw new Error(body.error || `driver answered ${res.status}`);
    return body;
  };
  const save = (op: "append" | "replace", text: string, signal?: AbortSignal) =>
    call("/api/job/notes", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ job_dir: jobDir, op, text }),
    }, signal);
  const reply = (text: string) => ({ content: [{ type: "text" as const, text }], details: undefined });

  pi.registerTool({
    name: "driver_notes",
    label: "Driver notes",
    description:
      "Read or change the user's notes on this job in Driver (the NOTES panel beside this session). " +
      "action read returns them; append adds text at the end after a blank line; replace rewrites them whole with text. " +
      "Only when the user asks.",
    promptSnippet: "Read, add to or rewrite the user's Driver notes for this job, only when they ask",
    promptGuidelines: [
      "Use driver_notes only when the user explicitly asks you to read, save to or change their notes; never read or write them on your own initiative.",
      "Use driver_notes with action append to save something to the user's notes, and with action replace only when the user asks to rewrite or tidy them (read them first, then pass the whole new text).",
    ],
    parameters: {
      type: "object",
      properties: {
        action: { type: "string", enum: ["read", "append", "replace"] },
        text: { type: "string", description: "what to add (append) or the whole new notes (replace)" },
      },
      required: ["action"],
      additionalProperties: false,
    },
    async execute(_toolCallId: string, params: NotesParams, signal?: AbortSignal) {
      if (params.action === "read") {
        const { text } = await call(`/api/job/notes?job_dir=${encodeURIComponent(jobDir)}`, {}, signal);
        return reply(text || "The notes are empty.");
      }
      if (typeof params.text !== "string") throw new Error(`${params.action} needs text`);
      await save(params.action, params.text, signal);
      return reply(params.action === "append" ? "Added to the notes." : "The notes now read as given.");
    },
  });
};

export default function (pi: ExtensionAPI) {
  const dir = process.env.DRIVER_JOB_DIR;
  if (!dir) return;
  const serve = process.env.DRIVER_SERVE;
  if (serve) registerNotesTool(pi, dir, serve);

  let state: State = "starting"; // in-memory only; session_start writes the first file
  let running = false; // an agent run is in flight (agent_start .. agent_settled)
  let dialogs = 0; // nesting depth of blocking ui prompts
  const tools = new Map<string, string>(); // toolCallId → name, for parallel tool calls

  // Atomic: the daemon may read status.json at any moment, so the JSON goes to
  // a sibling tmp file first and rename() swaps it in whole. Every fs call is
  // wrapped — a full disk or a vanished job dir must never surface in pi.
  const write = (ctx: ExtensionContext) => {
    try {
      let session = "";
      try {
        session = ctx.sessionManager.getSessionFile() ?? "";
      } catch {}
      const tool = [...tools.values()].pop(); // the most recently started tool still running
      const status = {
        state,
        ...(tool ? { tool } : {}),
        pid: process.pid,
        session,
        ts: Date.now(), // unix MILLISECONDS
      };
      mkdirSync(dir, { recursive: true });
      const tmp = join(dir, "status.json.tmp");
      writeFileSync(tmp, JSON.stringify(status));
      renameSync(tmp, join(dir, "status.json"));
    } catch {}
  };

  // Whether the operator has said anything in this session: a user message
  // on the current branch. Unknown reads as yes — the louder state.
  const briefed = (ctx: ExtensionContext): boolean => {
    try {
      return ctx.sessionManager
        .getBranch()
        .some((e) => e.type === "message" && e.message.role === "user");
    } catch {
      return true;
    }
  };

  // session_start → ready for a session nobody has spoken in yet (a job just
  // opened; the board keeps it in DRAFT until the first message starts an
  // agent run), idle for a resumed one (pi waits on its operator: blocked).
  pi.on("session_start", (_e, ctx) => {
    state = briefed(ctx) ? "idle" : "ready";
    running = false;
    dialogs = 0;
    tools.clear();
    write(ctx);
  });

  pi.on("agent_start", (_e, ctx) => {
    running = true;
    state = "working";
    write(ctx);
  });

  pi.on("tool_execution_start", (e, ctx) => {
    tools.set(e.toolCallId, e.toolName);
    write(ctx);
  });

  pi.on("tool_execution_end", (e, ctx) => {
    tools.delete(e.toolCallId);
    write(ctx);
  });

  // A blocking extension prompt (select/confirm/input/editor) is the one place
  // pi waits on the operator mid-run: the board calls that "dialog" (blocked).
  pi.on("ui_prompt_start", (_e, ctx) => {
    dialogs++;
    state = "dialog";
    write(ctx);
  });

  pi.on("ui_prompt_end", (_e, ctx) => {
    dialogs = Math.max(0, dialogs - 1);
    if (dialogs === 0) state = running ? "working" : "idle";
    write(ctx);
  });

  // agent_settled fires once no retry, compaction or queued continuation is
  // left: aborted (Escape) → aborted; otherwise idle when pi says it is idle,
  // else leave the state alone (a dialog still open keeps "dialog").
  pi.on("agent_settled", (e, ctx) => {
    running = false;
    if (dialogs > 0) {
      state = "dialog";
    } else if (e.aborted) {
      state = "aborted";
    } else if (ctx.isIdle()) {
      state = "idle";
    }
    write(ctx);
  });

  // Fires for quit AND for reload / new / resume / fork; the session_start
  // that follows the latter rewrites idle within milliseconds.
  pi.on("session_shutdown", (_e, ctx) => {
    state = "exited";
    write(ctx);
  });
}
