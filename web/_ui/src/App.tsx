import { useCallback, useEffect, useMemo, useState } from "react";

import { addLane, deleteJob, fetchBrowserContext, openJobDir, pasteIntoJob, pinJobLink, removeLane, renameLane, retryJob, unarchiveJob } from "./api/client.ts";
import { BoardView, type BoardActions } from "./components/board/BoardView.tsx";
import { DragProvider } from "./components/board/DragProvider.tsx";
import { GhostJobCard } from "./components/board/JobCard.tsx";
import { Chrome } from "./components/chrome/Chrome.tsx";
import { JobOverlay } from "./components/overlays/JobOverlay.tsx";
import { NewJobOverlay } from "./components/overlays/NewJobOverlay.tsx";
import { SettingsOverlay } from "./components/overlays/SettingsOverlay.tsx";
import { Toast, showToast } from "./components/overlays/Toast.tsx";
import { closeThreadNow } from "./components/overlays/ThreadTabs.tsx";
import { CommandPalette, type PalettePick } from "./components/palette/CommandPalette.tsx";
import { useBoard } from "./hooks/useBoard.ts";
import { readLS, writeLS } from "./hooks/useLocalStorage.ts";
import { slugOfKey } from "./lib/jobKey.ts";

const NO_JOBS: never[] = [];

export function App() {
  const board = useBoard();
  const [openKey, setOpenKeyRaw] = useState<string | null>(null);
  // the tab the open job shows (a thread id; 0 = its own pi): each job
  // remembers the tab it was left on (this browser's memory, like the
  // folded columns); the palette can ask for a thread
  const [openThread, setOpenThreadRaw] = useState(0);
  const setOpenKey = useCallback((key: string | null) => {
    setOpenKeyRaw((was) => {
      if (key !== was) setOpenThreadRaw(key === null ? 0 : rememberedThread(key));
      return key;
    });
  }, []);
  const setOpenThread = useCallback(
    (id: number) => {
      setOpenThreadRaw(id);
      if (openKey !== null) rememberThread(openKey, id);
    },
    [openKey],
  );
  const [newJob, setNewJob] = useState(false);
  const [palette, setPalette] = useState(false);
  const [settings, setSettings] = useState(false);
  const { mutate } = board;

  const actions = useMemo<BoardActions>(
    () => ({
      onOpenJob: (key) => setOpenKey(key),
      // nothing on the board changes, so this bypasses mutate (no refetch)
      onOpenDir: (slug) => {
        openJobDir(slug).catch((e: unknown) => showToast(`${slug}: ${e instanceof Error ? e.message : String(e)}`));
      },
      onOpenLink: (slug, url) => {
        void mutate(() => pinJobLink(slug, url));
      },
      onRetry: async (slugs) => {
        await Promise.all(
          slugs.map(async (slug) => {
            const err = await mutate(() => retryJob(slug));
            if (err) showToast(`${slug}: ${err.message}`);
          }),
        );
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
      onAddLane: (name) => {
        void mutate(() => addLane(name)).then((err) => {
          if (err) showToast(`${name}: ${err.message}`);
        });
      },
      onRenameLane: (id, name) => {
        void mutate(() => renameLane(id, name)).then((err) => {
          if (err) showToast(`${name}: ${err.message}`);
        });
      },
      onRemoveLane: (id) => {
        void mutate(() => removeLane(id)).then((err) => {
          if (err) showToast(err.message);
        });
      },
    }),
    [mutate],
  );

  const closeOverlay = useCallback(() => setOpenKey(null), [setOpenKey]);
  const openJob = useCallback((slug: string) => setOpenKey(slug), [setOpenKey]);
  const overlayOpen = openKey !== null || newJob || settings;
  const openPalette = useCallback(() => setPalette(true), []);
  const closePalette = useCallback(() => setPalette(false), []);
  // a pick lands on the pane: the job, on the tab asked for, over whatever
  // was open
  const onPick = useCallback(
    (pick: PalettePick) => {
      setNewJob(false);
      setOpenKey(pick.key);
      setOpenThreadRaw(pick.thread);
      rememberThread(pick.key, pick.thread);
    },
    [setOpenKey],
  );

  const boardLoaded = board.board !== undefined;
  useEffect(() => {
    const open = () => {
      const asked = window.driverPendingOpen;
      if (!asked || !boardLoaded) return;
      window.driverPendingOpen = undefined;
      setPalette(false);
      setSettings(false);
      onPick({ key: asked.slug, thread: asked.thread });
    };
    open();
    window.addEventListener("driver:open", open);
    return () => window.removeEventListener("driver:open", open);
  }, [onPick, boardLoaded]);

  // ⌘⇧V: the browser's front tab, pasted into the tab in view (the job's
  // own pi or the thread), nothing sent — the question is yours to add
  const pasteContext = useCallback(() => {
    if (openKey === null) return;
    const key = openKey;
    const slug = slugOfKey(key);
    const thread = openThread;
    fetchBrowserContext()
      .then((ctx) => pasteIntoJob(slug, thread, ctx.prompt).then(() => showToast(`pasted “${ctx.title}” into ${slug}`)))
      .catch((e: unknown) => showToast(`paste context: ${e instanceof Error ? e.message : String(e)} — see ⚙`));
  }, [openKey, openThread]);

  // ⌘K: the palette, from anywhere (ctrl-K too, unless a pane has it); ⌘N: a
  // new job; ⌘W: close what is on top — the palette, a dialog, the thread tab
  // in view, else the job overlay; the board itself never
  // (Driver.app leaves the key to the page for this)
  const byKey = board.byKey;
  useEffect(() => {
    const onKeyDown = (e: KeyboardEvent) => {
      if (e.altKey) return;
      if (e.shiftKey) {
        if (e.metaKey && !e.ctrlKey && e.code === "KeyV") {
          e.preventDefault();
          pasteContext();
        }
        return;
      }
      const key = e.key.toLowerCase();
      if (key !== "n" && key !== "k" && key !== "w") return;
      if (!e.metaKey && !(e.ctrlKey && !overlayOpen)) return;
      e.preventDefault();
      if (key === "k") {
        setPalette(true);
        return;
      }
      if (key === "n") {
        setPalette(false);
        setOpenKey(null);
        setNewJob(true);
        return;
      }
      if (palette) setPalette(false);
      else if (settings) setSettings(false);
      else if (newJob) setNewJob(false);
      else if (openKey !== null) {
        // the tab in view goes — the first too, while another is open; the
        // last tab stays and the overlay closes instead
        const tabs = byKey.get(openKey)?.threads ?? [];
        const t = tabs.length > 1 ? tabs.find((x) => x.id === openThread) : undefined;
        if (t) {
          closeThreadNow(slugOfKey(openKey), t, { current: openThread, onSelect: setOpenThread, onChanged: () => board.refetch(), onToast: showToast });
        } else setOpenKey(null);
      }
    };
    document.addEventListener("keydown", onKeyDown);
    return () => document.removeEventListener("keydown", onKeyDown);
  }, [overlayOpen, setOpenKey, setOpenThread, palette, settings, newJob, openKey, openThread, byKey, board, pasteContext]);

  return (
    <>
      <Chrome board={board.board} error={board.error} lastError={board.lastError} onSettings={() => setSettings(true)} />
      <DragProvider board={board} onOpenJob={openJob} onToast={showToast} renderOverlay={(job) => (job ? <GhostJobCard job={job} /> : null)}>
        <BoardView board={board} actions={actions} onSearch={openPalette} />
      </DragProvider>
      <JobOverlay
        openKey={openKey}
        thread={openThread}
        onThread={setOpenThread}
        board={board}
        mutate={mutate}
        onClose={closeOverlay}
        onRenamed={setOpenKey}
        onToast={showToast}
      />
      <CommandPalette
        open={palette}
        jobs={board.board?.jobs ?? NO_JOBS}
        lanes={board.lanes}
        onPick={onPick}
        onOpenDir={actions.onOpenDir}
        onClose={closePalette}
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
      <SettingsOverlay open={settings} onClose={() => setSettings(false)} onToast={showToast} />
      <Toast />
    </>
  );
}

/** The tab a job was last left on, per board key, in this browser. */
const THREAD_KEY_PREFIX = "driver.thread:";

function rememberedThread(key: string): number {
  const v = Number(readLS(THREAD_KEY_PREFIX + key) ?? 0);
  return Number.isInteger(v) && v > 0 ? v : 0;
}

function rememberThread(key: string, id: number): void {
  writeLS(THREAD_KEY_PREFIX + key, id ? String(id) : null);
}
