import type { ReactNode } from "react";

import type { Column as ColumnKey } from "../../lib/columns.ts";
import type { WebJob } from "../../lib/types.ts";
import { SortableColumn, useColumnDrop, useColumnJobs } from "./DragProvider.tsx";

export interface ColumnProps {
  col: ColumnKey;
  jobs: WebJob[];
  /** the unfiltered count while a search is active — "2/7" */
  total?: number;
  renderCard(job: WebJob): ReactNode;
  footer?: ReactNode;
  onNewJob(): void;
}

export function BoardColumn({ col, jobs, total, renderCard, footer, onNewJob }: ColumnProps) {
  const ordered = useColumnJobs(col, jobs);
  const drop = useColumnDrop(col);
  return (
    <section
      ref={drop.setNodeRef}
      className={
        "col" + (ordered.length ? " has-cards" : "") + (drop.className ? " " + drop.className : "")
      }
      data-col={col}
    >
      <div className="col-head">
        <b>{col.toUpperCase()}</b>
        <span className="n">{total === undefined ? ordered.length : `${ordered.length}/${total}`}</span>
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
