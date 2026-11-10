import { describe, expect, it } from "vitest";
import type { Active, ClientRect, DroppableContainer } from "@dnd-kit/core";

import { boardCollisionDetection, columnDropId, columnSortId, shouldStartDrag } from "./useDragReorder.ts";

describe("shouldStartDrag", () => {
  it("lets a press on a control marked data-drag start one, and on any other control not", () => {
    document.body.innerHTML = `
      <div class="col-head"><button class="lane-name" data-drag>later</button><span class="n">1</span>
        <button class="lane-remove">✕</button><input class="lane-input" /></div>`;
    expect(shouldStartDrag(document.querySelector(".lane-name"))).toBe(true);
    expect(shouldStartDrag(document.querySelector(".n"))).toBe(true);
    expect(shouldStartDrag(document.querySelector(".lane-remove"))).toBe(false);
    expect(shouldStartDrag(document.querySelector(".lane-input"))).toBe(false);
    document.body.innerHTML = "";
  });
});

const rect = (left: number, width: number, height: number): ClientRect => ({
  left,
  right: left + width,
  width,
  top: 88,
  bottom: 88 + height,
  height,
});

// four columns 200px wide at a 12px gap; BLOCKED and DONE are tall, the rest short
const COLUMN_RECTS = {
  draft: rect(16, 200, 120),
  working: rect(228, 200, 120),
  blocked: rect(440, 200, 1400),
  done: rect(652, 200, 1600),
  "lane:3": rect(864, 200, 120),
} as const;

function args(activeId: string, pointerX: number, collisionRect: ClientRect) {
  const containers: DroppableContainer[] = [];
  const rects = new Map<string, ClientRect>();
  for (const [col, r] of Object.entries(COLUMN_RECTS)) {
    for (const id of [columnSortId(col as keyof typeof COLUMN_RECTS), columnDropId(col as keyof typeof COLUMN_RECTS)]) {
      containers.push({ id, disabled: false, data: { current: {} }, node: { current: null }, rect: { current: r } } as unknown as DroppableContainer);
      rects.set(id, r);
    }
  }
  containers.push({ id: "some-card", disabled: false, data: { current: {} }, node: { current: null }, rect: { current: rect(448, 184, 60) } } as unknown as DroppableContainer);
  rects.set("some-card", rect(448, 184, 60));
  return {
    active: { id: activeId, data: { current: {} }, rect: { current: { initial: null, translated: null } } } as unknown as Active,
    collisionRect,
    droppableRects: rects,
    droppableContainers: containers,
    pointerCoordinates: { x: pointerX, y: 100 },
  };
}

describe("boardCollisionDetection — a column in the air", () => {
  it("lands by horizontal position alone, so tall neighbours are no further than short ones", () => {
    // the short lane dragged to sit between the two tall columns: the pointer is just past DONE's left edge
    const hits = boardCollisionDetection(args(columnSortId("lane:3"), 700, rect(600, 200, 120)));
    expect(hits[0]?.id).toBe(columnSortId("done"));
    expect(hits.map((h) => h.id)).not.toContain(columnDropId("done"));
    expect(hits.map((h) => h.id)).not.toContain("some-card");
    // nearer BLOCKED's centre than DONE's: BLOCKED
    expect(boardCollisionDetection(args(columnSortId("lane:3"), 600, rect(500, 200, 120)))[0]?.id).toBe(columnSortId("blocked"));
    // without a pointer (keyboard), the dragged column's own centre decides
    const noPointer = { ...args(columnSortId("lane:3"), 0, rect(500, 200, 120)), pointerCoordinates: null };
    expect(boardCollisionDetection(noPointer)[0]?.id).toBe(columnSortId("blocked"));
  });

  it("never offers a column's sort slot to a card", () => {
    const hits = boardCollisionDetection(args("some-card", 740, rect(640, 184, 60)));
    expect(hits.map((h) => h.id)).toContain(columnDropId("done"));
    expect(hits.some((h) => String(h.id).startsWith("colsort:"))).toBe(false);
  });
});
