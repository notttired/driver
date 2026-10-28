// ext/driver.ts — the pi extension every driver job loads with `-e`. It tells
// the daemon what pi is doing by writing <DRIVER_JOB_DIR>/status.json, which
// the tick loop reads (docs/DESIGN.md "State derivation"). Under driver's
// pane (`pi -p` once per prompt, the default launch) it covers the run —
// working, the tool in flight — and the pane writes the states between runs;
// under pi's interactive UI it covers everything. A no-op unless
// DRIVER_JOB_DIR is set, so a pi started by hand with this file loaded does
// nothing. Nothing long-lived here: no timers, no sockets — one file write per
// lifecycle event, and never a throw into pi.
import { mkdirSync, renameSync, writeFileSync } from "node:fs";
import { join } from "node:path";
import type { ExtensionAPI, ExtensionContext } from "@earendil-works/pi-coding-agent";

type State = "starting" | "ready" | "working" | "idle" | "dialog" | "aborted" | "exited";

export default function (pi: ExtensionAPI) {
  const dir = process.env.DRIVER_JOB_DIR;
  if (!dir) return;

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

  // Print mode (what the pane runs) and json mode have no UI: a session_start
  // there is always followed by one run, and pi exits once it is done. The
  // pane writes ready, idle, aborted and exited around the run, so here the
  // run reads working from its first moment and session_shutdown leaves
  // status.json alone.
  const headless = (ctx: ExtensionContext): boolean => ctx.mode === "print" || ctx.mode === "json";

  // session_start → working in print mode; otherwise ready for a session
  // nobody has spoken in yet (a job just opened; the board keeps it in DRAFT
  // until the first message starts an agent run), idle for a resumed one (pi
  // waits on its operator: blocked).
  pi.on("session_start", (_e, ctx) => {
    state = headless(ctx) ? "working" : briefed(ctx) ? "idle" : "ready";
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
  // that follows the latter rewrites idle within milliseconds. In print mode
  // every run ends here, and the pane — not pi — says what comes next.
  pi.on("session_shutdown", (_e, ctx) => {
    if (headless(ctx)) return;
    state = "exited";
    write(ctx);
  });
}
