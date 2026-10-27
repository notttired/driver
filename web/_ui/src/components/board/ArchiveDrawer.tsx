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
  /** unfiltered count while a search is active */
  total?: number;
  /** a search that matched archived jobs holds the drawer open */
  forceOpen?: boolean;
}

/** Collapsible, under DONE; also the done → archived drop target. */
export function ArchiveDrawer({ archived, actions, total, forceOpen }: ArchiveDrawerProps) {
  const [stored, setStored] = useLocalStorageFlag("driver-archive-open", false);
  const open = stored || !!forceOpen;
  const shown = archived.length;
  const hasArchived = (total ?? shown) > 0;
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
        disabled={shown === 0 || !!forceOpen}
        onClick={() => setStored(!stored)}
      >
        <span className="tri">{hasArchived ? (open ? "▾" : "▸") : "·"}</span> ARCHIVED{" "}
        <span className="n">{total === undefined ? shown : `${shown}/${total}`}</span>
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
