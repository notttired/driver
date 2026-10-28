import { describe, expect, it } from "vitest";

import { metaArrowSequence } from "./paneKeys.ts";

const key = (key: string, mods: Partial<Record<"metaKey" | "altKey" | "ctrlKey" | "shiftKey", boolean>> = {}) => ({
  key,
  metaKey: false,
  altKey: false,
  ctrlKey: false,
  shiftKey: false,
  ...mods,
});

describe("metaArrowSequence", () => {
  it("⌘← and ⌘→ send Home and End, in the terminal's cursor-key mode", () => {
    expect(metaArrowSequence(key("ArrowLeft", { metaKey: true }), false)).toBe("\x1b[H");
    expect(metaArrowSequence(key("ArrowRight", { metaKey: true }), false)).toBe("\x1b[F");
    expect(metaArrowSequence(key("ArrowLeft", { metaKey: true }), true)).toBe("\x1bOH");
    expect(metaArrowSequence(key("ArrowRight", { metaKey: true }), true)).toBe("\x1bOF");
  });

  it("leaves every other key to xterm", () => {
    expect(metaArrowSequence(key("ArrowLeft"), false)).toBeNull();
    expect(metaArrowSequence(key("ArrowLeft", { altKey: true }), false)).toBeNull();
    expect(metaArrowSequence(key("ArrowLeft", { ctrlKey: true }), false)).toBeNull();
    expect(metaArrowSequence(key("ArrowLeft", { metaKey: true, shiftKey: true }), false)).toBeNull();
    expect(metaArrowSequence(key("ArrowLeft", { metaKey: true, altKey: true }), false)).toBeNull();
    expect(metaArrowSequence(key("ArrowUp", { metaKey: true }), false)).toBeNull();
    expect(metaArrowSequence(key("ArrowDown", { metaKey: true }), false)).toBeNull();
    expect(metaArrowSequence(key("a", { metaKey: true }), false)).toBeNull();
    expect(metaArrowSequence(key("Home"), false)).toBeNull();
  });
});
