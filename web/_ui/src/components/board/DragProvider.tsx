import { createContext, useContext, useMemo } from "react";
import type { ReactNode } from "react";
import { DndContext, DragOverlay, useDroppable } from "@dnd-kit/core";
import {
  SortableContext,
  horizontalListSortingStrategy,
  useSortable,
  verticalListSortingStrategy,
} from "@dnd-kit/sortable";
import { CSS } from "@dnd-kit/utilities";

import type { BoardModel } from "../../hooks/useBoard.ts";
import { columnDropId, columnSortId, shouldStartDrag, useDragReorder } from "../../hooks/useDragReorder.ts";
import type { Column } from "../../lib/columns.ts";
import { jobKey } from "../../lib/jobKey.ts";
import type { DropColumn } from "../../lib/moves.ts";
import type { WebJob } from "../../lib/types.ts";

interface DragContextValue {
  activeKey: string | null;
  /** every column in display order, an in-flight column move applied */
  columns: Column[];
  orderJobs: (column: Column, jobs: WebJob[]) => WebJob[];
  isTarget: (to: DropColumn) => boolean;
  submittingKeys: ReadonlySet<string>;
}

const IDENTITY: DragContextValue = {
  activeKey: null,
  columns: [],
  orderJobs: (_column, jobs) => jobs,
  isTarget: () => false,
  submittingKeys: new Set<string>(),
};

export const DragCtx = createContext<DragContextValue>(IDENTITY);

export interface DragProviderProps {
  board: BoardModel;
  onOpenJob: (slug: string) => void;
  onToast?: (message: string) => void;
  /** the card under the pointer while dragging; a bare slug box by default */
  renderOverlay?: (job: WebJob | null, key: string) => ReactNode;
  visibleKeys?: ReadonlySet<string>;
  children: ReactNode;
}

export function DragProvider({
  board,
  onOpenJob,
  onToast,
  renderOverlay,
  visibleKeys,
  children,
}: DragProviderProps) {
  const drag = useDragReorder({ board, onOpenJob, onToast, visibleKeys });

  const value = useMemo<DragContextValue>(
    () => ({
      activeKey: drag.activeKey,
      columns: drag.columns,
      orderJobs: drag.orderJobs,
      isTarget: drag.isTarget,
      submittingKeys: drag.submittingKeys,
    }),
    [drag.activeKey, drag.columns, drag.orderJobs, drag.isTarget, drag.submittingKeys],
  );

  return (
    <DndContext
      sensors={drag.sensors}
      collisionDetection={drag.collisionDetection}
      onDragStart={drag.onDragStart}
      onDragOver={drag.onDragOver}
      onDragEnd={drag.onDragEnd}
      onDragCancel={drag.onDragCancel}
    >
      <DragCtx.Provider value={value}>{children}</DragCtx.Provider>
      <DragOverlay dropAnimation={null}>
        {drag.activeKey === null
          ? null
          : (renderOverlay?.(drag.activeJob, drag.activeKey) ?? (
              <div className="card dragging-overlay">
                <div className="card-top">
                  <span className="slug">{drag.activeJob?.slug ?? drag.activeKey}</span>
                </div>
              </div>
            ))}
      </DragOverlay>
    </DndContext>
  );
}

export function useDragState(): DragContextValue {
  return useContext(DragCtx);
}

export function useColumnJobs(column: Column, jobs: WebJob[]): WebJob[] {
  return useContext(DragCtx).orderJobs(column, jobs);
}

/** every column in display order, an in-flight column move applied */
export function useColumns(): Column[] {
  return useContext(DragCtx).columns;
}

/** The columns' own sortable row: drag a head sideways to move its column. */
export function SortableColumns({ columns, children }: { columns: Column[]; children: ReactNode }) {
  const items = useMemo(() => columns.map(columnSortId), [columns]);
  return (
    <SortableContext items={items} strategy={horizontalListSortingStrategy}>
      {children}
    </SortableContext>
  );
}

export interface SortableColumn {
  setNodeRef: (element: HTMLElement | null) => void;
  /** for the head: presses on its buttons and inputs never start a drag */
  handle: Record<string, (event: never) => void>;
  /** for a collapsed column's strip, which is the whole handle */
  stripHandle: Record<string, (event: never) => void>;
  style: { transform?: string; transition?: string };
  isDragging: boolean;
}

export function useSortableColumn(column: Column): SortableColumn {
  const { listeners, setNodeRef, transform, transition, isDragging } = useSortable({ id: columnSortId(column) });
  const stripHandle = (listeners ?? {}) as Record<string, (event: never) => void>;
  const handle = useMemo(() => guardListeners(listeners ?? {}), [listeners]);
  return {
    setNodeRef,
    handle,
    stripHandle,
    style: {
      transform: CSS.Translate.toString(transform) ?? undefined,
      transition: transition ?? undefined,
    },
    isDragging,
  };
}

function guardListeners(listeners: Record<string, unknown>): Record<string, (event: never) => void> {
  const out: Record<string, (event: never) => void> = {};
  for (const [name, fn] of Object.entries(listeners)) {
    out[name] = (event: never) => {
      const e = event as unknown as { target?: EventTarget | null };
      if (!shouldStartDrag(e?.target ?? null)) return;
      (fn as (ev: unknown) => void)(event);
    };
  }
  return out;
}

export function useCardSubmitting(key: string): boolean {
  return useContext(DragCtx).submittingKeys.has(key);
}

export interface ColumnDrop {
  setNodeRef: (element: HTMLElement | null) => void;
  isOver: boolean;
  isTarget: boolean;
  /** "droptarget" / "droptarget drophover" / "" */
  className: string;
}

export function useColumnDrop(column: DropColumn): ColumnDrop {
  const { isTarget } = useContext(DragCtx);
  const { setNodeRef, isOver } = useDroppable({ id: columnDropId(column) });
  const target = isTarget(column);
  const className = [target ? "droptarget" : "", target && isOver ? "drophover" : ""]
    .filter(Boolean)
    .join(" ");
  return { setNodeRef, isOver, isTarget: target, className };
}

export interface SortableColumnProps {
  column: Column;
  jobs: WebJob[];
  children: ReactNode;
}

export function SortableColumn({ column, jobs, children }: SortableColumnProps) {
  const ordered = useColumnJobs(column, jobs);
  const items = useMemo(() => ordered.map((j) => jobKey(j)), [ordered]);
  return (
    <SortableContext items={items} strategy={verticalListSortingStrategy}>
      {children}
    </SortableContext>
  );
}

export interface SortableCard {
  setNodeRef: (element: HTMLElement | null) => void;
  attributes: Record<string, unknown>;
  listeners: Record<string, (event: never) => void>;
  style: { transform?: string; transition?: string };
  isDragging: boolean;
  className: string;
}

/** A card's sortable wiring; presses on links, buttons and inputs inside the
 *  card never start a drag. */
export function useSortableCard(key: string): SortableCard {
  const submitting = useCardSubmitting(key);
  const { attributes, listeners, setNodeRef, transform, transition, isDragging } = useSortable({
    id: key,
    disabled: submitting,
  });

  const guarded = useMemo(() => (submitting ? {} : guardListeners(listeners ?? {})), [listeners, submitting]);

  return {
    setNodeRef,
    attributes: attributes as unknown as Record<string, unknown>,
    listeners: guarded,
    style: {
      transform: CSS.Transform.toString(transform) ?? undefined,
      transition: transition ?? undefined,
    },
    isDragging,
    className: isDragging ? "dragging" : "",
  };
}
