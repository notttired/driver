import { useLocalStorageFlag } from "../../hooks/useLocalStorage.ts";
import { jobKey } from "../../lib/jobKey.ts";
import { ARCHIVED } from "../../lib/moves.ts";
import type { WebJob } from "../../lib/types.ts";
import { ArchivedCard } from "./ArchivedCard.tsx";
import type { BoardActions } from "./BoardView.tsx";
import { useColumnDrop } from "./DragProvider.tsx";

export interface ArchiveDrawerProps {
  archived: WebJob[];
  actions: BoardActions;
}

/** Collapsible, under DONE; also the done → archived drop target. */
export function ArchiveDrawer({ archived, actions }: ArchiveDrawerProps) {
  const [open, setStored] = useLocalStorageFlag("driver-archive-open", false);
  const shown = archived.length;
  const drop = useColumnDrop(ARCHIVED);

  return (
    <div
      ref={drop.setNodeRef}
      className={"archive-drawer" + (drop.className ? " " + drop.className : "")}
      data-col="archived"
    >
      <button
        className="archive-head"
        id="archive-toggle"
        disabled={shown === 0}
        onClick={() => setStored(!open)}
      >
        <span className="tri">{shown > 0 ? (open ? "▾" : "▸") : "·"}</span> ARCHIVED <span className="n">{shown}</span>
      </button>
      {shown > 0 ? (
        <div className="archive-cards" hidden={!open}>
          {archived.map((j) => (
            <ArchivedCard key={jobKey(j)} job={j} actions={actions} />
          ))}
        </div>
      ) : null}
    </div>
  );
}
