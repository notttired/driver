import type { CSSProperties, ReactNode } from "react";

import type { Column as ColumnKey } from "../../lib/columns.ts";
import type { WebJob } from "../../lib/types.ts";
import { SortableColumn, useColumnDrop, useColumnJobs, useSortableColumn } from "./DragProvider.tsx";

export interface ColumnProps {
  col: ColumnKey;
  /** what the column is called, for the strip and the controls' labels */
  label: string;
  /** the head's title; the label, as a plain heading, by default */
  title?: ReactNode;
  jobs: WebJob[];
  /** the unfiltered count while a search is active — "2/7" */
  total?: number;
  renderCard(job: WebJob): ReactNode;
  head?: ReactNode;
  footer?: ReactNode;
  onNewJob(): void;
  collapsed: boolean;
  onToggleCollapse(): void;
}

export function BoardColumn({
  col,
  label,
  title,
  jobs,
  total,
  renderCard,
  head,
  footer,
  onNewJob,
  collapsed,
  onToggleCollapse,
}: ColumnProps) {
  const ordered = useColumnJobs(col, jobs);
  const drop = useColumnDrop(col);
  const sort = useSortableColumn(col);
  const count = total === undefined ? String(ordered.length) : `${ordered.length}/${total}`;
  const setRef = (el: HTMLElement | null) => {
    drop.setNodeRef(el);
    sort.setNodeRef(el);
  };
  const className =
    "col" +
    (ordered.length ? " has-cards" : "") +
    (collapsed ? " collapsed" : "") +
    (sort.isDragging ? " col-dragging" : "") +
    (drop.className ? " " + drop.className : "");

  if (collapsed) {
    return (
      <section ref={setRef} className={className} data-col={col} style={sort.style as CSSProperties}>
        <button
          className="col-strip"
          aria-label={`expand ${label}`}
          title={`${label} — ${count} · click to expand`}
          onClick={onToggleCollapse}
          {...sort.stripHandle}
        >
          <span className="n">{count}</span>
          <span className="strip-name">{label}</span>
        </button>
      </section>
    );
  }

  return (
    <section ref={setRef} className={className} data-col={col} style={sort.style as CSSProperties}>
      <div className="col-head" {...sort.handle}>
        {title ?? <b>{label}</b>}
        <span className="n">{count}</span>
        {head}
        <button className="col-collapse" aria-label={`collapse ${label}`} title="collapse" onClick={onToggleCollapse}>
          «
        </button>
      </div>
      <div className="col-cards">
        {col === "draft" ? (
          <button className="btn-new" id="new-job" onClick={onNewJob}>
            + NEW JOB
          </button>
        ) : null}
        {ordered.length ? (
          <SortableColumn column={col} jobs={jobs}>
            {ordered.map(renderCard)}
          </SortableColumn>
        ) : col === "draft" ? null : (
          <div className="empty">·</div>
        )}
      </div>
      {footer}
    </section>
  );
}
