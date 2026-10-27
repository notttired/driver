import { describe, expect, it } from "vitest";

import { isOpened, statusToken } from "./status.ts";

describe("statusToken", () => {
  it("draft", () => {
    expect(statusToken({ state: "draft" })).toMatchObject({ text: "draft", variant: "draft" });
  });

  it("working: starting vs working, with the tool as detail", () => {
    expect(statusToken({ state: "working", pi: { state: "starting", ts: 1 } })).toMatchObject({
      text: "○ starting",
      variant: "starting",
    });
    expect(statusToken({ state: "working" })).toMatchObject({ text: "● working", variant: "working" });
    const t = statusToken({ state: "working", pi: { state: "working", tool: "bash", ts: 1 } });
    expect(t).toMatchObject({ text: "● working", variant: "working" });
    expect(t.detail).toBe("⚙ bash");
  });

  it("blocked: the pi state picks the word", () => {
    expect(statusToken({ state: "blocked", pi: { state: "dialog", ts: 1 } })).toMatchObject({
      text: "? dialog",
      variant: "dialog",
    });
    expect(statusToken({ state: "blocked", pi: { state: "exited", ts: 1 } })).toMatchObject({
      text: "✕ exited",
      variant: "exited",
    });
    expect(statusToken({ state: "blocked", pi: { state: "silent", ts: 1 } })).toMatchObject({
      text: "… silent",
      variant: "silent",
    });
    expect(statusToken({ state: "blocked", pi: { state: "aborted", ts: 1 } })).toMatchObject({
      text: "■ aborted",
      variant: "aborted",
    });
    expect(statusToken({ state: "blocked", pi: { state: "idle", ts: 1 } })).toMatchObject({
      text: "⚑ needs you",
      variant: "needs-you",
    });
    expect(statusToken({ state: "blocked" })).toMatchObject({ text: "⚑ needs you", variant: "needs-you" });
  });

  it("done", () => {
    expect(statusToken({ state: "done" })).toMatchObject({ text: "✓ done", variant: "done" });
  });
});

describe("an opened draft", () => {
  it("shows pi at its prompt, or booting", () => {
    expect(statusToken({ state: "draft", pi: { state: "ready", ts: 1 } })).toMatchObject({ text: "○ pi open", variant: "ready" });
    expect(statusToken({ state: "draft", pi: { state: "starting", ts: 1 } })).toMatchObject({ text: "○ starting", variant: "starting" });
  });
  it("isOpened: any pi reading means a pane exists; a not-started draft has none", () => {
    expect(isOpened({ state: "draft" })).toBe(false);
    expect(isOpened({ state: "draft", pi: { state: "ready", ts: 1 } })).toBe(true);
    expect(isOpened({ state: "working" })).toBe(true);
    expect(isOpened({ state: "done" })).toBe(true);
  });
});
