import type { BoardModel } from "../../hooks/useBoard.ts";
import { useCollapsedColumns } from "../../hooks/useCollapsedColumns.ts";
import type { JobSearchModel } from "../../hooks/useJobSearch.ts";
import { WATCHING, isStateColumn, laneId } from "../../lib/columns.ts";
import { jobKey } from "../../lib/jobKey.ts";
import type { WebJob } from "../../lib/types.ts";
import { ArchiveDrawer } from "./ArchiveDrawer.tsx";
import { BoardBar } from "./BoardBar.tsx";
import { BoardColumn } from "./Column.tsx";
import { SortableColumns, useColumns } from "./DragProvider.tsx";
import { JobCard } from "./JobCard.tsx";
import { AddLane, LaneRemove, LaneTitle } from "./LaneControls.tsx";
import { RetryAll } from "./RetryAll.tsx";

/** What a card can ask the app to do. Every write goes through the app's
 *  mutate so the board refreshes the same way whatever triggered it. */
export interface BoardActions {
  /** open the job overlay for a board key (jobKey()) */
  onOpenJob(key: string): void;
  /** ⌘-click: open the job's directory in the editor (no overlay) */
  onOpenDir(slug: string): void;
  onOpenLink(slug: string, url: string): void;
  onRetry(slugs: string[]): Promise<void>;
  /** open the new-job row */
  onNewJob(): void;
  onUnarchive(slug: string): void;
  onDeleteForever(slug: string): void;
  /** a column of your own */
  onAddLane(name: string): void;
  onRenameLane(id: number, name: string): void;
  onRemoveLane(id: number): void;
  /** WATCHING's ✎: open the watch prompt editor */
  onEditWatchPrompt(): void;
}

export interface BoardViewProps {
  board: BoardModel;
  search: JobSearchModel;
  actions: BoardActions;
  overlayOpen: boolean;
  onSearchShortcut: () => void;
}

export function BoardView({ board, search, actions, overlayOpen, onSearchShortcut }: BoardViewProps) {
  const { byColumn, archived, active } = search;
  const columns = useColumns();
  const { isCollapsed, toggle } = useCollapsedColumns();
  const renderCard = (j: WebJob) => <JobCard key={jobKey(j)} job={j} actions={actions} />;
  return (
    <>
      <BoardBar search={search} overlayOpen={overlayOpen} onShortcut={onSearchShortcut} />
      <main className="board" id="board">
        <SortableColumns columns={columns}>
          {columns.map((col) => {
            const all = board.byColumn[col] ?? [];
            const shared = {
              col,
              jobs: byColumn[col] ?? [],
              total: active ? all.length : undefined,
              onNewJob: actions.onNewJob,
              renderCard,
              collapsed: isCollapsed(col),
              onToggleCollapse: () => toggle(col),
            };
            if (isStateColumn(col)) {
              return (
                <BoardColumn
                  key={col}
                  {...shared}
                  label={col.toUpperCase()}
                  head={col === "blocked" ? <RetryAll jobs={byColumn.blocked} onRetry={actions.onRetry} /> : undefined}
                  footer={col === "done" ? <ArchiveDrawer archived={archived} actions={actions} /> : undefined}
                />
              );
            }
            if (col === WATCHING) {
              return (
                <BoardColumn
                  key={col}
                  {...shared}
                  label="WATCHING"
                  head={
                    <button
                      className="col-edit"
                      aria-label="edit the watch prompt"
                      title="edit what WATCH says to pi"
                      onClick={actions.onEditWatchPrompt}
                    >
                      ✎
                    </button>
                  }
                />
              );
            }
            const lane = board.lanes.find((l) => l.id === laneId(col));
            if (!lane) return null;
            return (
              <BoardColumn
                key={col}
                {...shared}
                label={lane.name}
                title={<LaneTitle lane={lane} onRename={actions.onRenameLane} />}
                head={<LaneRemove lane={lane} count={all.length} onRemove={actions.onRemoveLane} />}
              />
            );
          })}
        </SortableColumns>
        <AddLane onAdd={actions.onAddLane} />
      </main>
    </>
  );
}
