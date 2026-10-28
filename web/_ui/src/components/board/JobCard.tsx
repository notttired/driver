import type { CSSProperties, MouseEvent } from "react";

import { age, columnFor, dirBase, jobColorVar, jobTitle, nameColorVar } from "../../lib/format.ts";
import { jobKey } from "../../lib/jobKey.ts";
import { statusToken } from "../../lib/status.ts";
import type { WebJob } from "../../lib/types.ts";
import type { BoardActions } from "./BoardView.tsx";
import { useCardSubmitting, useSortableCard, type SortableCard } from "./DragProvider.tsx";
import { LinkChips } from "./chips/LinkChips.tsx";

export interface JobCardProps {
  job: WebJob;
  actions: BoardActions;
}

/** A board card: sortable, clickable, drawn with its job colour on the left edge. */
export function JobCard({ job, actions }: JobCardProps) {
  const key = jobKey(job);
  const drag = useSortableCard(key);
  const submitting = useCardSubmitting(key);
  return (
    <JobCardView
      job={job}
      drag={drag}
      submitting={submitting}
      onOpen={(e) => {
        // a click that ends a text selection is not a click on the card
        if (window.getSelection()?.toString()) return;
        // ⌘-click: the job's directory in the editor, not the overlay
        if (e.metaKey) {
          actions.onOpenDir(job.slug);
          return;
        }
        // a card mid-submit is about to change state; let it land first
        if (submitting) return;
        actions.onOpenJob(key);
      }}
    />
  );
}

/** The DragOverlay's copy: same markup, no sortable registration. */
export function GhostJobCard({ job }: { job: WebJob }) {
  const submitting = useCardSubmitting(jobKey(job));
  return <JobCardView job={job} submitting={submitting} ghost />;
}

interface JobCardViewProps {
  job: WebJob;
  submitting: boolean;
  drag?: SortableCard;
  ghost?: boolean;
  onOpen?: (e: MouseEvent<HTMLDivElement>) => void;
}

function JobCardView({ job, submitting, drag, ghost, onOpen }: JobCardViewProps) {
  const col = columnFor(job.state);
  const jc = jobColorVar(job);
  const base = dirBase(job.dir);
  const tok = statusToken(job);

  const style = {
    ...(jc ? { "--job-color": jc } : {}),
    ...(drag?.style ?? {}),
  } as CSSProperties;

  return (
    <div
      ref={drag?.setNodeRef}
      className={
        "card editable" +
        (drag?.className ? " " + drag.className : "") +
        (submitting ? " submitting" : "") +
        (job.attention ? " attention" : "") +
        (ghost ? " dragging-overlay" : "")
      }
      style={style}
      title={job.attention ? "turned blocked since you last opened it" : undefined}
      data-slug={job.slug}
      data-col={col}
      onClick={onOpen}
      {...(drag?.attributes ?? {})}
      {...(drag?.listeners ?? {})}
    >
      <div className="card-top">
        {submitting ? <span className="submit-spinner" aria-hidden="true" /> : null}
        <span className="slug" title={job.name ? job.display_slug || job.slug : undefined}>
          {jobTitle(job)}
        </span>
        {job.has_notes ? (
          <span className="notes-mark" title="has notes">
            ✎
          </span>
        ) : null}
      </div>
      <div className="meta">
        {base ? (
          <span
            className="repo"
            style={{ "--repo-color": nameColorVar(base) } as CSSProperties}
            title={`${job.dir}\n⌘-click the card: open in editor`}
          >
            {base}
          </span>
        ) : null}
        <span className="age" data-age title={job.model ? `model ${job.model}` : undefined}>
          {age(job.mtime)}
        </span>
        <span className={`status status-${col} status-${tok.variant}`} title={tok.detail}>
          {tok.text}
          {tok.detail ? <i className="tool">{tok.detail}</i> : null}
        </span>
      </div>
      {submitting ? (
        <div className="submitting-line">starting session… (request in flight)</div>
      ) : null}
      <LinkChips links={job.links} />
    </div>
  );
}
