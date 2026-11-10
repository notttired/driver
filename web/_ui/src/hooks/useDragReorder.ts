// dnd-kit wiring for the board: sensors, collision detection, the drop
// handler that reads the drag table, and the TWO pieces of client-side
// state the pessimistic board otherwise never holds — an optimistic
// same-column reorder (the one write the user sees instantly) and the
// in-flight submit (a draft drawn in WORKING while the server spawns it).

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import {
  KeyboardSensor,
  MouseSensor,
  TouchSensor,
  pointerWithin,
  rectIntersection,
  useSensor,
  useSensors,
} from "@dnd-kit/core";
import type {
  Collision,
  CollisionDetection,
  DragEndEvent,
  DragOverEvent,
  DragStartEvent,
  UniqueIdentifier,
} from "@dnd-kit/core";
import { arrayMove, sortableKeyboardCoordinates } from "@dnd-kit/sortable";

import { reorderColumn, reorderColumns } from "../api/client.ts";
import type { Column } from "../lib/columns.ts";
import { columnFor } from "../lib/format.ts";
import { jobKey } from "../lib/jobKey.ts";
import { ARCHIVED, SUBMIT_ENDPOINT, resolveMove, type DropColumn, type MoveContext } from "../lib/moves.ts";
import { columnSlugs, predictReorderedJobs } from "../lib/predictReorder.ts";
import { isOpened } from "../lib/status.ts";
import type { WebBoard, WebJob } from "../lib/types.ts";
import type { BoardModel } from "./useBoard.ts";

export const COLUMN_DROP_PREFIX = "col:";

export const columnDropId = (column: DropColumn): string => COLUMN_DROP_PREFIX + column;

export function parseColumnDropId(id: UniqueIdentifier | undefined): DropColumn | null {
  if (id === undefined) return null;
  const s = String(id);
  if (!s.startsWith(COLUMN_DROP_PREFIX)) return null;
  return s.slice(COLUMN_DROP_PREFIX.length) as DropColumn;
}

/** A column's own sortable id: dragging the head moves the column. */
export const COLUMN_SORT_PREFIX = "colsort:";

export const columnSortId = (column: Column): string => COLUMN_SORT_PREFIX + column;

export function parseColumnSortId(id: UniqueIdentifier | undefined): Column | null {
  if (id === undefined) return null;
  const s = String(id);
  if (!s.startsWith(COLUMN_SORT_PREFIX)) return null;
  return s.slice(COLUMN_SORT_PREFIX.length) as Column;
}

export const MOUSE_ACTIVATION = { distance: 4 } as const;
export const TOUCH_ACTIVATION = { delay: 250, tolerance: 8 } as const;

/** Surfaces inside a card that a press must NOT start a drag from. */
export const NO_DRAG_SELECTOR = [
  "[data-no-drag]",
  "a",
  "button",
  "input",
  "textarea",
  "select",
  "[contenteditable]:not([contenteditable='false'])",
].join(",");

/** …unless the control says it is a handle too (a plain click still reaches it). */
export const DRAG_HANDLE_ATTR = "data-drag";

export function shouldStartDrag(target: EventTarget | null): boolean {
  if (!(target instanceof Element)) return true;
  const control = target.closest(NO_DRAG_SELECTOR);
  return control === null || control.hasAttribute(DRAG_HANDLE_ATTR);
}

/** what the drag table needs to know about the card in the air */
export function moveContext(job: WebJob | undefined): MoveContext {
  return job === undefined ? {} : { opened: isOpened(job), state: columnFor(job.state) };
}

export function useBoardSensors() {
  return useSensors(
    useSensor(MouseSensor, { activationConstraint: MOUSE_ACTIVATION }),
    useSensor(TouchSensor, { activationConstraint: TOUCH_ACTIVATION }),
    useSensor(KeyboardSensor, { coordinateGetter: sortableKeyboardCoordinates }),
  );
}

/** A column in the air only meets other columns' sort slots, by horizontal
 *  distance alone — columns differ wildly in height, so their centres say
 *  nothing about where the pointer is. A card never meets those slots: for
 *  cards, pointer-within first (the column under the cursor), rect overlap
 *  as the fallback for keyboard drags, which have no pointer. */
export const boardCollisionDetection: CollisionDetection = (args) => {
  const columnDrag = parseColumnSortId(args.active.id) !== null;
  const droppableContainers = args.droppableContainers.filter(
    (c) => (parseColumnSortId(c.id) !== null) === columnDrag,
  );
  const scoped = { ...args, droppableContainers };
  if (columnDrag) return closestColumnByX(scoped);
  const within = pointerWithin(scoped);
  return within.length > 0 ? within : rectIntersection(scoped);
};

const closestColumnByX: CollisionDetection = ({ collisionRect, droppableContainers, droppableRects, pointerCoordinates }) => {
  const x = pointerCoordinates?.x ?? collisionRect.left + collisionRect.width / 2;
  const collisions: Collision[] = [];
  for (const container of droppableContainers) {
    const rect = droppableRects.get(container.id);
    if (!rect) continue;
    const distance = Math.abs(rect.left + rect.width / 2 - x);
    collisions.push({ id: container.id, data: { droppableContainer: container, value: distance } });
  }
  return collisions.sort((a, b) => (a.data?.value ?? 0) - (b.data?.value ?? 0));
};

interface Override {
  column: Column;
  slugs: string[];
  /** the POST has answered; the next payload decides whether to drop it */
  settled: boolean;
  /** the payload in hand when it settled — a newer one is a verdict */
  at: WebBoard | undefined;
  /** payloads that disagreed since settling; one is tolerated (a poll that
   *  was in flight when the POST landed), the second wins */
  disagreements: number;
}

interface Submitting {
  key: string;
  job: WebJob;
}

export interface DragReorderOptions {
  board: BoardModel;
  /** blocked → working: open the job overlay instead of writing anything */
  onOpenJob: (slug: string) => void;
  onToast?: (message: string) => void;
  /** keys visible under an active search — a drop on the column body lands
   *  after the last VISIBLE card, not after a hidden one */
  visibleKeys?: ReadonlySet<string>;
}

export interface DragReorderModel {
  activeKey: string | null;
  activeJob: WebJob | null;
  /** the column in the air, when the drag is a column's */
  activeColumn: Column | null;
  /** every column in display order, an in-flight column move applied */
  columns: Column[];
  from: Column | null;
  over: DropColumn | null;
  sensors: ReturnType<typeof useBoardSensors>;
  collisionDetection: CollisionDetection;
  onDragStart: (e: DragStartEvent) => void;
  onDragOver: (e: DragOverEvent) => void;
  onDragEnd: (e: DragEndEvent) => void;
  onDragCancel: () => void;
  /** a column's jobs with the optimistic override and in-flight submits applied */
  orderJobs: (column: Column, jobs: WebJob[]) => WebJob[];
  isTarget: (to: DropColumn) => boolean;
  submittingKeys: ReadonlySet<string>;
}

export function useDragReorder({
  board,
  onOpenJob,
  onToast,
  visibleKeys,
}: DragReorderOptions): DragReorderModel {
  const [activeKey, setActiveKey] = useState<string | null>(null);
  const [activeColumn, setActiveColumn] = useState<Column | null>(null);
  const [columnOverride, setColumnOverride] = useState<Column[] | null>(null);
  const [from, setFrom] = useState<Column | null>(null);
  const [over, setOver] = useState<DropColumn | null>(null);
  const [override, setOverride] = useState<Override | null>(null);
  const [submitting, setSubmitting] = useState<readonly Submitting[]>([]);
  const submittingKeys = useMemo(() => new Set(submitting.map((s) => s.key)), [submitting]);

  const boardRef = useRef<WebBoard | undefined>(board.board);
  boardRef.current = board.board;
  const activeRef = useRef<{ key: string; from: Column } | null>(null);

  const { byKey, byColumn, archived, setDragging, refetch, mutate } = board;

  const columns = columnOverride ?? board.columns;
  const columnsRef = useRef(columns);
  columnsRef.current = columns;
  useEffect(() => {
    if (columnOverride === null) return;
    const incoming = board.columns;
    if (incoming.length === columnOverride.length && incoming.every((c, i) => c === columnOverride[i])) {
      setColumnOverride(null);
    }
  }, [board.columns, columnOverride]);

  const columnOf = useMemo(() => {
    const m = new Map<string, DropColumn>();
    for (const [c, jobs] of Object.entries(byColumn) as [Column, WebJob[]][]) {
      for (const j of jobs) m.set(jobKey(j), c);
    }
    for (const j of archived) m.set(jobKey(j), ARCHIVED);
    return m;
  }, [byColumn, archived]);

  const resolveOver = useCallback(
    (id: UniqueIdentifier | undefined): DropColumn | null => {
      const asColumn = parseColumnDropId(id);
      if (asColumn !== null) return asColumn;
      if (id === undefined) return null;
      return columnOf.get(String(id)) ?? null;
    },
    [columnOf],
  );

  const orderJobs = useCallback(
    (column: Column, jobs: WebJob[]): WebJob[] => {
      let out = jobs;
      if (override && override.column === column) {
        const bySlug = new Map(jobs.map((j) => [j.slug, j] as const));
        const ordered: WebJob[] = [];
        for (const s of override.slugs) {
          const j = bySlug.get(s);
          if (j) {
            ordered.push(j);
            bySlug.delete(s);
          }
        }
        for (const j of jobs) if (bySlug.has(j.slug)) ordered.push(j);
        out = ordered;
      }
      if (submitting.length > 0) {
        if (column === "draft") {
          out = out.filter((j) => !submittingKeys.has(jobKey(j)));
        } else if (column === "working") {
          // de-dupe: the server may already list it as working
          const add = submitting.filter((s) => !out.some((j) => jobKey(j) === s.key));
          if (add.length > 0) out = [...add.map((s) => s.job).reverse(), ...out];
        }
      }
      return out;
    },
    [override, submitting, submittingKeys],
  );

  // settle the optimistic override against the payloads that follow it
  useEffect(() => {
    if (!override || !override.settled) return;
    const incoming = board.board;
    if (incoming === override.at) return;
    const order = columnSlugs(incoming?.jobs ?? [], override.column);
    const agrees =
      order.length === override.slugs.length && order.every((s, i) => s === override.slugs[i]);
    if (agrees) {
      setOverride(null);
      return;
    }
    setOverride((o) =>
      o === null
        ? null
        : o.disagreements >= 1
          ? null
          : { ...o, disagreements: o.disagreements + 1, at: incoming },
    );
  }, [board.board, override]);

  const finishDrag = useCallback(() => {
    activeRef.current = null;
    setActiveKey(null);
    setActiveColumn(null);
    setFrom(null);
    setOver(null);
    setDragging(false);
  }, [setDragging]);

  const commitColumnMove = useCallback(
    async (column: Column, overId: UniqueIdentifier | undefined) => {
      const target = parseColumnSortId(overId);
      const current = columnsRef.current;
      const oldIdx = current.indexOf(column);
      const newIdx = target === null ? -1 : current.indexOf(target);
      if (oldIdx < 0 || newIdx < 0 || oldIdx === newIdx) return;
      const next = arrayMove(current, oldIdx, newIdx);
      setColumnOverride(next);
      const err = await mutate(() => reorderColumns(next));
      if (err) {
        setColumnOverride(null);
        onToast?.(`move column failed: ${err.message} — reverted`);
      }
    },
    [mutate, onToast],
  );

  const onDragStart = useCallback(
    (e: DragStartEvent) => {
      const column = parseColumnSortId(e.active.id);
      if (column !== null) {
        setActiveColumn(column);
        setDragging(true);
        return;
      }
      const key = String(e.active.id);
      const col = columnOf.get(key);
      if (col === undefined || col === ARCHIVED) return;
      activeRef.current = { key, from: col };
      setActiveKey(key);
      setFrom(col);
      setOver(col);
      setDragging(true);
    },
    [columnOf, setDragging],
  );

  const onDragOver = useCallback(
    (e: DragOverEvent) => {
      if (parseColumnSortId(e.active.id) !== null) return;
      setOver(resolveOver(e.over?.id));
    },
    [resolveOver],
  );

  const lastVisibleIndex = useCallback(
    (ordered: WebJob[]): number => {
      if (!visibleKeys) return ordered.length - 1;
      for (let i = ordered.length - 1; i >= 0; i--) {
        if (visibleKeys.has(jobKey(ordered[i]!))) return i;
      }
      return ordered.length - 1;
    },
    [visibleKeys],
  );

  const commitReorder = useCallback(
    async (column: Column, key: string, overId: UniqueIdentifier | undefined) => {
      const ordered = orderJobs(column, byColumn[column] ?? []);
      const oldIdx = ordered.findIndex((j) => jobKey(j) === key);
      const newIdx =
        parseColumnDropId(overId) !== null
          ? Math.max(oldIdx, lastVisibleIndex(ordered))
          : ordered.findIndex((j) => jobKey(j) === String(overId));
      if (oldIdx < 0 || newIdx < 0) {
        await refetch();
        return;
      }
      if (oldIdx === newIdx) return;

      const slugs = arrayMove(ordered, oldIdx, newIdx).map((j) => j.slug);
      const predicted = predictReorderedJobs(boardRef.current?.jobs ?? [], column, slugs);
      if (predicted === null) {
        // the client's picture disagrees with itself — go pessimistic
        await mutate(() => reorderColumn(column, slugs));
        return;
      }

      setOverride({ column, slugs, settled: false, at: undefined, disagreements: 0 });
      try {
        await reorderColumn(column, slugs);
        setOverride((o) =>
          o !== null && o.column === column && !o.settled
            ? { ...o, settled: true, at: boardRef.current }
            : o,
        );
      } catch (e) {
        const msg = e instanceof Error ? e.message : String(e);
        setOverride(null);
        onToast?.(`reorder failed: ${msg} — reverted`);
        await refetch();
      }
    },
    [byColumn, lastVisibleIndex, mutate, onToast, orderJobs, refetch],
  );

  const onDragEnd = useCallback(
    (e: DragEndEvent) => {
      const column = parseColumnSortId(e.active.id);
      const active = activeRef.current;
      finishDrag();
      if (column !== null) {
        void commitColumnMove(column, e.over?.id);
        return;
      }
      if (active === null) return;
      const to = resolveOver(e.over?.id);
      const job = byKey.get(active.key);
      if (to === null || job === undefined) {
        void refetch();
        return;
      }
      const move = resolveMove(active.from, to, moveContext(job));
      if (move === null) {
        void refetch();
        return;
      }
      if (move.kind === "reorder") {
        void commitReorder(active.from, active.key, e.over?.id);
        return;
      }
      if (move.kind === "open") {
        onOpenJob(job.slug);
        void refetch();
        return;
      }
      if (move.endpoint === SUBMIT_ENDPOINT) {
        if (submittingKeys.has(active.key)) {
          onToast?.(`${job.slug} is already starting`);
          void refetch();
          return;
        }
        setSubmitting((prev) => [...prev, { key: active.key, job }]);
        void mutate(() => move.run(job.slug))
          .then((err) => {
            // a 409 carries the reason; the card has returned to DRAFT
            if (err) onToast?.(`${job.slug}: ${err.message}`);
          })
          .finally(() => setSubmitting((prev) => prev.filter((s) => s.key !== active.key)));
        return;
      }
      void mutate(() => move.run(job.slug)).then((err) => {
        if (err) onToast?.(`${job.slug}: ${err.message}`);
      });
    },
    [byKey, commitColumnMove, commitReorder, finishDrag, mutate, onOpenJob, onToast, refetch, resolveOver, submittingKeys],
  );

  const onDragCancel = useCallback(() => {
    finishDrag();
    void refetch();
  }, [finishDrag, refetch]);

  const isTarget = useCallback(
    (to: DropColumn): boolean => {
      if (from === null || from === to) return false;
      return resolveMove(from, to, moveContext(activeKey === null ? undefined : byKey.get(activeKey))) !== null;
    },
    [activeKey, byKey, from],
  );

  const sensors = useBoardSensors();

  return {
    activeKey,
    activeJob: activeKey === null ? null : (byKey.get(activeKey) ?? null),
    activeColumn,
    columns,
    from,
    over,
    sensors,
    collisionDetection: boardCollisionDetection,
    onDragStart,
    onDragOver,
    onDragEnd,
    onDragCancel,
    orderJobs,
    isTarget,
    submittingKeys,
  };
}
