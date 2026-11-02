import { describe, expect, it } from "vitest";

import { metaKeySequence } from "./paneKeys.ts";

const key = (key: string, mods: Partial<Record<"metaKey" | "altKey" | "ctrlKey" | "shiftKey", boolean>> = {}) => ({
  key,
  metaKey: false,
  altKey: false,
  ctrlKey: false,
  shiftKey: false,
  ...mods,
});

describe("metaKeySequence", () => {
  it("⌘← and ⌘→ send Home and End, in the terminal's cursor-key mode", () => {
    expect(metaKeySequence(key("ArrowLeft", { metaKey: true }), false)).toBe("\x1b[H");
    expect(metaKeySequence(key("ArrowRight", { metaKey: true }), false)).toBe("\x1b[F");
    expect(metaKeySequence(key("ArrowLeft", { metaKey: true }), true)).toBe("\x1bOH");
    expect(metaKeySequence(key("ArrowRight", { metaKey: true }), true)).toBe("\x1bOF");
  });

  it("⌘⌫ deletes the whole line: End, then Ctrl+U back to its start", () => {
    expect(metaKeySequence(key("Backspace", { metaKey: true }), false)).toBe("\x1b[F\x15");
    expect(metaKeySequence(key("Backspace", { metaKey: true }), true)).toBe("\x1bOF\x15");
    expect(metaKeySequence(key("Backspace"), false)).toBeNull();
    expect(metaKeySequence(key("Backspace", { metaKey: true, shiftKey: true }), false)).toBeNull();
    expect(metaKeySequence(key("Backspace", { metaKey: true, altKey: true }), false)).toBeNull();
    expect(metaKeySequence(key("Backspace", { metaKey: true, ctrlKey: true }), false)).toBeNull();
  });

  it("leaves every other key to xterm", () => {
    expect(metaKeySequence(key("ArrowLeft"), false)).toBeNull();
    expect(metaKeySequence(key("ArrowLeft", { altKey: true }), false)).toBeNull();
    expect(metaKeySequence(key("ArrowLeft", { ctrlKey: true }), false)).toBeNull();
    expect(metaKeySequence(key("ArrowLeft", { metaKey: true, shiftKey: true }), false)).toBeNull();
    expect(metaKeySequence(key("ArrowLeft", { metaKey: true, altKey: true }), false)).toBeNull();
    expect(metaKeySequence(key("ArrowUp", { metaKey: true }), false)).toBeNull();
    expect(metaKeySequence(key("ArrowDown", { metaKey: true }), false)).toBeNull();
    expect(metaKeySequence(key("a", { metaKey: true }), false)).toBeNull();
    expect(metaKeySequence(key("Home"), false)).toBeNull();
  });
});
