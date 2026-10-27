import type { BoardModel } from "../../hooks/useBoard.ts";
import type { JobSearchModel } from "../../hooks/useJobSearch.ts";
import { COLUMNS } from "../../lib/columns.ts";
import { jobKey } from "../../lib/jobKey.ts";
import { ArchiveDrawer } from "./ArchiveDrawer.tsx";
import { BoardBar } from "./BoardBar.tsx";
import { BoardColumn } from "./Column.tsx";
import { JobCard } from "./JobCard.tsx";

/** What a card can ask the app to do. Every write goes through the app's
 *  mutate so the board refreshes the same way whatever triggered it. */
export interface BoardActions {
  /** open the job overlay for a board key (jobKey()) */
  onOpenJob(key: string): void;
  /** ⌘-click: open the job's directory in the editor (no overlay) */
  onOpenDir(slug: string): void;
  /** open the new-job row */
  onNewJob(): void;
  onUnarchive(slug: string): void;
  onDeleteForever(slug: string): void;
}

export interface BoardViewProps {
  board: BoardModel;
  search: JobSearchModel;
  actions: BoardActions;
  overlayOpen: boolean;
}

export function BoardView({ board, search, actions, overlayOpen }: BoardViewProps) {
  const { byColumn, archived, active } = search;
  return (
    <>
      <BoardBar search={search} overlayOpen={overlayOpen} />
      <main className="board" id="board">
        {COLUMNS.map((col) => (
          <BoardColumn
            key={col}
            col={col}
            jobs={byColumn[col]}
            total={active ? board.byColumn[col].length : undefined}
            onNewJob={actions.onNewJob}
            footer={
              col === "done" ? (
                <ArchiveDrawer
                  archived={archived}
                  actions={actions}
                  total={active ? board.archived.length : undefined}
                  forceOpen={active && archived.length > 0}
                />
              ) : undefined
            }
            renderCard={(j) => <JobCard key={jobKey(j)} job={j} actions={actions} />}
          />
        ))}
      </main>
    </>
  );
}
