import type { BoardModel } from "../../hooks/useBoard.ts";
import { useCollapsedColumns } from "../../hooks/useCollapsedColumns.ts";
import { isStateColumn, laneId } from "../../lib/columns.ts";
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
}

export interface BoardViewProps {
  board: BoardModel;
  actions: BoardActions;
  /** open the ⌘K palette */
  onSearch: () => void;
}

export function BoardView({ board, actions, onSearch }: BoardViewProps) {
  const { byColumn, archived } = board;
  const columns = useColumns();
  const { isCollapsed, toggle } = useCollapsedColumns();
  const renderCard = (j: WebJob) => <JobCard key={jobKey(j)} job={j} actions={actions} />;
  return (
    <>
      <BoardBar total={(board.board?.jobs ?? []).filter((j) => !j.archived).length} onSearch={onSearch} />
      <main className="board" id="board">
        <SortableColumns columns={columns}>
          {columns.map((col) => {
            const all = byColumn[col] ?? [];
            const shared = {
              col,
              jobs: all,
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
