import { useCallback, useMemo, useState } from "react";

import { deleteJob, openJobDir, unarchiveJob } from "./api/client.ts";
import { BoardView, type BoardActions } from "./components/board/BoardView.tsx";
import { DragProvider } from "./components/board/DragProvider.tsx";
import { GhostJobCard } from "./components/board/JobCard.tsx";
import { Chrome } from "./components/chrome/Chrome.tsx";
import { JobOverlay } from "./components/overlays/JobOverlay.tsx";
import { NewJobOverlay } from "./components/overlays/NewJobOverlay.tsx";
import { Toast, showToast } from "./components/overlays/Toast.tsx";
import { useBoard } from "./hooks/useBoard.ts";
import { useJobSearch } from "./hooks/useJobSearch.ts";

export function App() {
  const board = useBoard();
  const search = useJobSearch(board);
  const [openKey, setOpenKey] = useState<string | null>(null);
  const [newJob, setNewJob] = useState(false);
  const { mutate } = board;

  const actions = useMemo<BoardActions>(
    () => ({
      onOpenJob: (key) => setOpenKey(key),
      // nothing on the board changes, so this bypasses mutate (no refetch)
      onOpenDir: (slug) => {
        openJobDir(slug).catch((e: unknown) => showToast(`${slug}: ${e instanceof Error ? e.message : String(e)}`));
      },
      onNewJob: () => setNewJob(true),
      onUnarchive: (slug) => {
        void mutate(() => unarchiveJob(slug)).then((err) => {
          if (err) showToast(`${slug}: ${err.message}`);
        });
      },
      onDeleteForever: (slug) => {
        void mutate(() => deleteJob(slug)).then((err) => {
          if (err) showToast(`${slug}: ${err.message}`);
        });
      },
    }),
    [mutate],
  );

  const closeOverlay = useCallback(() => setOpenKey(null), []);
  const openJob = useCallback((slug: string) => setOpenKey(slug), []);
  const overlayOpen = openKey !== null || newJob;

  return (
    <>
      <Chrome board={board.board} error={board.error} lastError={board.lastError} />
      <DragProvider
        board={board}
        onOpenJob={openJob}
        onToast={showToast}
        visibleKeys={search.active ? search.visibleKeys : undefined}
        renderOverlay={(job) => (job ? <GhostJobCard job={job} /> : null)}
      >
        <BoardView board={board} search={search} actions={actions} overlayOpen={overlayOpen} />
      </DragProvider>
      <JobOverlay
        openKey={openKey}
        board={board}
        mutate={mutate}
        onClose={closeOverlay}
        onRenamed={setOpenKey}
        onToast={showToast}
      />
      <NewJobOverlay
        open={newJob}
        onClose={() => setNewJob(false)}
        mutate={mutate}
        onToast={showToast}
        defaultDir={board.board?.default_dir ?? ""}
        onCreated={(slug) => {
          setNewJob(false);
          setOpenKey(slug);
        }}
      />
      <Toast />
    </>
  );
}
