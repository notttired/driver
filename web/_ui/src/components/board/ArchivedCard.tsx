import type { CSSProperties } from "react";

import { age, dirBase, jobColorVar } from "../../lib/format.ts";
import { jobKey } from "../../lib/jobKey.ts";
import type { WebJob } from "../../lib/types.ts";
import type { BoardActions } from "./BoardView.tsx";

export function ArchivedCard({ job, actions }: { job: WebJob; actions: BoardActions }) {
  const key = jobKey(job);
  const jc = jobColorVar(job);
  const name = job.display_slug || job.slug;
  const style = {
    "--state": "var(--text-2)",
    ...(jc ? { "--job-color": jc } : {}),
  } as CSSProperties;
  return (
    <div
      className="card archived"
      style={style}
      data-slug={job.slug}
      data-editable="false"
      onClick={() => {
        if (window.getSelection()?.toString()) return;
        actions.onOpenJob(key);
      }}
    >
      <div className="card-top">
        <span className="slug">{name}</span>
        {job.display_slug && job.display_slug !== job.slug ? (
          <span className="as">as {job.slug}</span>
        ) : null}
        <span className="state">■ archived</span>
      </div>
      <div className="meta">
        {job.dir ? (
          <span className="repo muted" title={job.dir}>
            {dirBase(job.dir)}
          </span>
        ) : null}
        <span className="age" data-age>
          {age(job.finished_at || job.mtime)}
        </span>
      </div>
      <div className="card-actions">
        <button
          className="btn unarchive"
          data-no-drag
          onClick={(e) => {
            e.stopPropagation();
            actions.onUnarchive(job.slug);
          }}
        >
          ⬆ unarchive
        </button>
        <button
          className="btn delete"
          data-no-drag
          onClick={(e) => {
            e.stopPropagation();
            if (window.confirm(`Delete ${name} forever? This removes its job directory.`)) {
              actions.onDeleteForever(job.slug);
            }
          }}
        >
          ✕ delete forever
        </button>
      </div>
    </div>
  );
}
