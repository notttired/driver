import { describe, expect, it } from "vitest";

import { DEFAULT_SPLIT, clampSplit, splitFromPointer } from "./split.ts";

describe("split", () => {
  it("clamps to the 25–75 band and rounds", () => {
    expect(clampSplit(50.4)).toBe(50);
    expect(clampSplit(10)).toBe(25);
    expect(clampSplit(99)).toBe(75);
    expect(clampSplit(Number.NaN)).toBe(DEFAULT_SPLIT);
  });
  it("maps a pointer across the body to a share", () => {
    expect(splitFromPointer(500, 0, 1000)).toBe(50);
    expect(splitFromPointer(100, 0, 1000)).toBe(25);
    expect(splitFromPointer(1700, 200, 1000)).toBe(75);
    expect(splitFromPointer(5, 0, 0)).toBe(DEFAULT_SPLIT);
  });
});
