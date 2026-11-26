import { Suspense, lazy, useCallback, useEffect, useMemo, useRef, useState } from "react";

import {
  ApiTimeout,
  archiveJob,
  deleteJob,
  doneJob,
  fetchJobForm,
  fetchModels,
  killJob,
  openJobDir,
  pinJobLink,
  queueJob,
  reactivateJob,
  retryJob,
  retryThread,
  reviewJob,
  seenJob,
  setJobColor,
  stopJob,
  stopThread,
  unarchiveJob,
  watchJob,
} from "../../api/client.ts";
import type { BoardModel } from "../../hooks/useBoard.ts";
import { ESC_PRIORITY } from "../../hooks/useEscapeStack.ts";
import { ARCHIVE_KEY_PREFIX, slugOfKey } from "../../lib/jobKey.ts";
import { isOpened, statusToken } from "../../lib/status.ts";
import type { JobForm, ModelsResponse, WebJob, WebThread } from "../../lib/types.ts";
import { readLS, writeLS } from "../../hooks/useLocalStorage.ts";
import { DEFAULT_SPLIT, MAX_SPLIT, MIN_SPLIT, SPLIT_KEY, clampSplit, splitFromPointer } from "../../lib/split.ts";
import { ConversationPanel } from "./ConversationPanel.tsx";
import { ForkButton, ForkRow, suggestForkName, useForkControl, type ForkControlState } from "./ForkControl.tsx";
import { LaunchForm } from "./LaunchForm.tsx";
import { LinksPanel } from "./LinksPanel.tsx";
import { NotesPanel } from "./NotesPanel.tsx";
import { Overlay, type Mutate } from "./Overlay.tsx";
import { RenameButton, RenameRow, useRenameControl, type RenameControlState } from "./RenameControl.tsx";
import { SideSwitch, type SideView } from "./SideSwitch.tsx";
import { SwatchStrip } from "./SwatchStrip.tsx";
import { ThreadTabs } from "./ThreadTabs.tsx";

// xterm is half the bundle; it loads the first time an opened job's pane shows
const TerminalPane = lazy(() =>
  import("./TerminalPane.tsx").then((m) => ({ default: m.TerminalPane })),
);

type Flush = () => Promise<void>;
type RegisterFlush = (fn: Flush | null) => void;

export interface JobOverlayProps {
  /** board key (jobKey()) of the open job, null when closed */
  openKey: string | null;
  /** the tab shown: a thread id, 0 for the job's own pi (the app's, so the
   *  palette can open a job on a thread) */
  thread: number;
  onThread: (id: number) => void;
  board: BoardModel;
  mutate: Mutate;
  onClose: () => void;
  /** the rename landed; the app re-points openKey */
  onRenamed: (key: string) => void;
  onToast: (message: string) => void;
}

/**
 * The job detail view. Head: slug (+ rename), colour, status, dir, actions by
 * state. Body: pi's pane once the job is opened (a draft whose pi is up,
 * working, blocked, done), the launch form (dir, model, START) for a
 * not-started draft; sidebar: notes and the conversation record side by
 * side, links beneath. The body is keyed by slug — a rename or a different job
 * is a fresh mount — while the rename and fork controls and the auto-close
 * rule live up here where they survive that remount.
 */
export function JobOverlay({ openKey, thread, onThread, board, mutate, onClose, onRenamed, onToast }: JobOverlayProps) {
  const open = openKey !== null;
  const slug = openKey === null ? null : slugOfKey(openKey);
  const archived = openKey?.startsWith(ARCHIVE_KEY_PREFIX) ?? false;
  const job = openKey === null ? undefined : board.byKey.get(openKey);

  // auto-close when the job leaves the board (archive, unarchive, delete, a
  // rename from elsewhere) — captured lazily, since a just-created draft may
  // not be on the board yet; state changes are not departures
  const seen = useRef(false);
  useEffect(() => {
    seen.current = false;
  }, [openKey]);
  useEffect(() => {
    if (openKey === null) return;
    if (board.byKey.has(openKey)) seen.current = true;
    else if (seen.current) onClose();
  }, [board.byKey, openKey, onClose]);

  // pending autosaves (form, notes) are flushed before a rename or a submit
  const flushers = useRef(new Map<string, Flush>());
  const registerFlush = useCallback(
    (name: string): RegisterFlush =>
      (fn) => {
        if (fn) flushers.current.set(name, fn);
        else flushers.current.delete(name);
      },
    [],
  );
  const registerFormFlush = useMemo(() => registerFlush("form"), [registerFlush]);
  const registerNotesFlush = useMemo(() => registerFlush("notes"), [registerFlush]);
  const flushAll = useCallback(async () => {
    await Promise.all([...flushers.current.values()].map((f) => f()));
  }, []);

  const rename = useRenameControl(
    slug,
    job?.name ?? "",
    (to) => onRenamed(archived ? ARCHIVE_KEY_PREFIX + to : to),
    { open, beforeSubmit: flushAll },
  );

  // the fork box opens on "<name> fork" (then "fork 2", …): a name no job on
  // the board has; a landed fork becomes the open job
  const byKey = board.byKey;
  const baseName = job?.name || job?.display_slug || slug || "";
  const forkName = useMemo(() => suggestForkName(baseName, (s) => byKey.has(s)), [baseName, byKey]);
  const fork = useForkControl(
    slug,
    forkName,
    (to, name) => {
      void board.refetch();
      onRenamed(to);
      onToast(`forked as ${name}`);
    },
    { open, beforeSubmit: flushAll },
  );

  return (
    <Overlay id="job-overlay" open={open} priority={ESC_PRIORITY.job} onClose={onClose} panelClassName="job-panel">
      {slug !== null ? (
        <JobPanel
          key={slug}
          slug={slug}
          archived={archived}
          job={job}
          thread={thread}
          onThread={onThread}
          board={board}
          mutate={mutate}
          onClose={onClose}
          onToast={onToast}
          rename={rename}
          fork={fork}
          registerFormFlush={registerFormFlush}
          registerNotesFlush={registerNotesFlush}
          flushAll={flushAll}
        />
      ) : null}
    </Overlay>
  );
}

interface JobPanelProps {
  slug: string;
  archived: boolean;
  job: WebJob | undefined;
  thread: number;
  onThread: (id: number) => void;
  board: BoardModel;
  mutate: Mutate;
  onClose: () => void;
  onToast: (message: string) => void;
  rename: RenameControlState;
  fork: ForkControlState;
  registerFormFlush: RegisterFlush;
  registerNotesFlush: RegisterFlush;
  flushAll: Flush;
}

function JobPanel({
  slug,
  archived,
  job,
  thread,
  onThread,
  board,
  mutate,
  onClose,
  onToast,
  rename,
  fork,
  registerFormFlush,
  registerNotesFlush,
  flushAll,
}: JobPanelProps) {
  const [form, setForm] = useState<JobForm | null>(null);
  const [formError, setFormError] = useState("");
  const [epoch, setEpoch] = useState(0);
  const [models, setModels] = useState<ModelsResponse | null>(null);
  const [busy, setBusy] = useState<string | null>(null);

  const loadForm = useCallback(
    (signal?: AbortSignal) =>
      fetchJobForm(slug, signal)
        .then((f) => {
          setForm(f);
          setFormError("");
        })
        .catch((e: unknown) => {
          if (signal?.aborted) return;
          setFormError(e instanceof Error ? e.message : String(e));
        }),
    [slug],
  );

  // on mount, on an explicit reload, and when the board's state for the job
  // flips or its pi comes up (START swaps the launch form for the pane)
  const state = job?.state;
  const boardOpened = job ? isOpened(job) : undefined;
  useEffect(() => {
    const ac = new AbortController();
    void loadForm(ac.signal);
    return () => ac.abort();
  }, [loadForm, epoch, state, boardOpened]);
  // the board is the fresher word; the form stands in until the job is on it
  const opened = boardOpened ?? (form ? !form.editable : false);

  // the attention ring waits for exactly this: the tab open in front of the
  // operator. One acknowledgement per ring — the board refetch after the
  // write drops the flag; should it come back while the tab is still in
  // view (pi went quiet again), they are looking at it, so it goes again.
  const attention = job?.attention ?? false;
  const tabs = job?.threads ?? [];
  const due = tabs.some((t) => t.attention) ? (tabs.find((t) => t.id === thread)?.attention ?? false) : attention;
  const acknowledged = useRef<string | null>(null);
  useEffect(() => {
    if (!due) {
      acknowledged.current = null;
      return;
    }
    const key = `${slug}/${thread}`;
    if (acknowledged.current === key) return;
    acknowledged.current = key;
    void mutate(() => seenJob(slug, thread));
  }, [due, slug, thread, mutate]);

  // the model datalist is only needed by the launch form
  const editable = form?.editable ?? false;
  useEffect(() => {
    if (!editable || models) return;
    let gone = false;
    fetchModels()
      .then((m) => {
        if (!gone) setModels(m);
      })
      .catch(() => {
        // the datalist is a convenience; typing a model id still works
      });
    return () => {
      gone = true;
    };
  }, [editable, models]);

  const run = async (name: string, op: () => Promise<unknown>, opts: { flush?: boolean } = {}) => {
    if (busy) return;
    setBusy(name);
    try {
      if (opts.flush) await flushAll();
      const err = await mutate(op);
      if (err instanceof ApiTimeout) onToast(`${slug}: still starting — the board will catch up`);
      else if (err) onToast(`${slug}: ${err.message}`);
    } finally {
      setBusy(null);
    }
  };

  const openLink = (url: string) => void mutate(() => pinJobLink(slug, url));

  const tok = statusToken(job ?? { state: form?.state ?? "draft" });
  const dir = job?.dir ?? form?.dir ?? "";
  // the tabs: the job's own pi and its threads; the one shown is the app's
  // (another job starts on its own pi) and falls back to the job's own when
  // the thread it was on is closed
  const title = title0(job, form, slug);
  const jobThreads = job?.threads;
  const threads = useMemo<WebThread[]>(
    () => jobThreads ?? [{ id: 0, name: title, live: job?.live ?? false, pi: job?.pi, retryable: job?.retryable }],
    [jobThreads, title, job?.live, job?.pi, job?.retryable],
  );
  const setThread = onThread;
  // the sidebar: the conversation, or the notes when asked
  const [side, setSide] = useState<SideView>("conversation");
  const sideHead = <SideSwitch view={side} onSwitch={setSide} />;
  const cur: WebThread = threads.find((t) => t.id === thread) ?? threads[0] ?? { id: 0, name: title, live: false };
  useEffect(() => {
    if (thread !== 0 && !threads.some((t) => t.id === thread)) setThread(0);
  }, [thread, threads]);
  const live = cur.live;
  // attach whenever a tmux session can exist: opened drafts, working and
  // blocked jobs keep theirs even after pi exits (dead pane, last screen
  // intact); done jobs had theirs killed, so the pane shows the ended banner
  // straight away.
  const attach = opened && job?.state !== "done";
  // main/sidebar split, the main pane's share in percent; dragged on the
  // divider, remembered per browser.
  const [split, setSplit] = useState(() => clampSplit(Number(readLS(SPLIT_KEY) ?? DEFAULT_SPLIT)));
  const bodyRef = useRef<HTMLDivElement | null>(null);
  useEffect(() => {
    writeLS(SPLIT_KEY, String(split));
  }, [split]);
  const dragSplit = (e: React.PointerEvent<HTMLDivElement>) => {
    if (!e.currentTarget.hasPointerCapture(e.pointerId)) return;
    const r = bodyRef.current?.getBoundingClientRect();
    if (r) setSplit(splitFromPointer(e.clientX, r.left, r.width));
  };
  const effState = archived ? "archived" : (job?.state ?? form?.state ?? "draft");

  const actions = (() => {
    const b = (name: string, label: string, op: () => Promise<unknown>, extra: { cls?: string; disabled?: boolean; title?: string; confirm?: string; flush?: boolean } = {}) => (
      <button
        key={name}
        className={extra.cls ?? "btn"}
        data-action={name}
        disabled={busy !== null || extra.disabled === true}
        title={extra.title}
        onClick={() => {
          if (extra.confirm && !window.confirm(extra.confirm)) return;
          void run(name, op, { flush: extra.flush });
        }}
      >
        {busy === name ? <span className="submit-spinner" aria-hidden="true" /> : null}
        {label}
      </button>
    );
    switch (effState) {
      case "draft":
        // two kinds of draft: not started (launch form, START) and opened
        // (pi at its prompt, nothing sent — the pane, DONE and CANCEL)
        return opened
          ? [
              b("done", "✓ DONE", () => doneJob(slug), { title: "close pi, mark done" }),
              b("cancel", "✕ CANCEL", () => killJob(slug), {
                cls: "btn ghost danger",
                title: "close pi; the job goes back to not started",
                confirm: `Cancel ${title}? Its pi is closed and the job goes back to not started.`,
              }),
            ]
          : [
              b("start", "▶ START", () => queueJob(slug), {
                cls: "btn primary",
                title: "open pi in the job's directory (drag to WORKING does the same)",
                flush: true,
              }),
            ];
      case "working":
      case "blocked":
        return [
          ...(cur.retryable
            ? [
                b("retry", "↻ RETRY", () => (cur.id ? retryThread(slug, cur.id) : retryJob(slug)), {
                  cls: "btn primary",
                  title:
                    cur.pi?.state === "exited"
                      ? "start pi again on this conversation; it continues if it was cut off mid-turn"
                      : "pi gave up on its last turn: tell it to continue",
                }),
              ]
            : []),
          b("stop", "■ STOP", () => (cur.id ? stopThread(slug, cur.id) : stopJob(slug)), {
            disabled: !live,
            title: "send Escape to this tab's pane — interrupt what its pi is doing",
          }),
          b("review", "🔍 REVIEW", () => reviewJob(slug, cur.id), {
            disabled: !live,
            title: "ask this tab's pi for an adversarial review of its work from a second agent (ae-deep-reviewer); it reports the findings and changes nothing until you say",
          }),
          b("watch", "👁 WATCH", () => watchJob(slug, cur.id), {
            disabled: !live,
            title: "ask this tab's pi to watch over the shipped work: the merge, the deploy, then its health for 30 minutes",
          }),
          b("done", "✓ DONE", () => doneJob(slug), { title: "kill the session, mark done" }),
          b("cancel", "✕ CANCEL", () => killJob(slug), {
            cls: "btn ghost danger",
            title: "kill the session and return the job to DRAFT",
            confirm: `Cancel ${title}? Its session is killed and it returns to DRAFT.`,
          }),
        ];
      case "done":
        return [
          b("reactivate", "↻ REACTIVATE", () => reactivateJob(slug), {
            cls: "btn primary",
            disabled: job?.reactivatable === false,
            title: job?.reactivatable === false ? "its directory is gone" : "relaunch pi on the same session",
          }),
          b("archive", "▣ ARCHIVE", () => archiveJob(slug), {
            disabled: live,
            title: live ? "a live session cannot be archived" : "archive (frees the slug)",
          }),
        ];
      case "archived":
        return [
          b("unarchive", "⬆ UNARCHIVE", () => unarchiveJob(slug)),
          b("delete", "✕ DELETE FOREVER", () => deleteJob(slug), {
            cls: "btn ghost danger",
            confirm: `Delete ${title} forever? This removes its job directory.`,
          }),
        ];
      default:
        return [];
    }
  })();

  return (
    <>
      <div className="panel-head">
        <span className="panel-title" title={slug}>
          {title}
        </span>
        {job && title !== job.slug ? (
          <span className="as" title={archived ? "its name in the archive" : "its slug — the API, the job dir and the tmux session go by it"}>
            as {job.slug}
          </span>
        ) : null}
        <RenameButton state={rename} visible={!archived} />
        <ForkButton state={fork} visible={!archived} />
        <SwatchStrip
          compact
          value={job?.color ?? form?.color}
          disabled={archived || busy !== null}
          onPick={(c) => void run("color", () => setJobColor(slug, c))}
        />
        <span className={`status status-${tok.variant}` + (live ? " live" : "")} title={tok.title ?? tok.detail}>
          {archived ? "■ archived" : tok.text}
          {tok.detail ? <i className="tool">{tok.detail}</i> : null}
        </span>
        {dir ? (
          <span
            className="dir"
            title={archived ? dir : `${dir}\n⌘-click: open in editor`}
            onClick={(e) => {
              if (!e.metaKey || archived) return;
              openJobDir(slug).catch((err: unknown) => onToast(`${slug}: ${err instanceof Error ? err.message : String(err)}`));
            }}
          >
            {dir}
          </span>
        ) : null}
        <span className="spacer" />
        {actions}
        <button className="btn ghost" id="job-close" title="close (Esc)" onClick={onClose}>
          ✕
        </button>
      </div>
      <RenameRow state={rename} />
      <ForkRow state={fork} column={effState} />
      <div className="job-body" ref={bodyRef} style={{ "--job-split": String(split) } as React.CSSProperties}>
        <div className="job-main">
          {formError ? (
            <div className="answer-note">
              ✗ {formError}{" "}
              <button className="btn" onClick={() => setEpoch((e) => e + 1)}>
                retry
              </button>
            </div>
          ) : form === null ? (
            <div className="editor-hint">loading…</div>
          ) : !opened ? (
            <LaunchForm
              key={`${slug}:${epoch}`}
              form={form}
              models={models}
              onSaved={(mtime) => setForm((f) => (f ? { ...f, mtime } : f))}
              onReload={() => setEpoch((e) => e + 1)}
              registerFlush={registerFormFlush}
            />
          ) : (
            <>
              {attach ? (
                <ThreadTabs
                  slug={slug}
                  threads={threads}
                  current={cur.id}
                  onSelect={setThread}
                  onChanged={() => board.refetch()}
                  onToast={onToast}
                />
              ) : null}
              <Suspense fallback={<div className="editor-hint">loading terminal…</div>}>
                <TerminalPane key={`${slug}:${cur.id}`} slug={slug} thread={cur.id} attach={attach} onOpenLink={openLink} />
              </Suspense>
            </>
          )}
        </div>
        <div
          className="job-divider"
          role="separator"
          aria-orientation="vertical"
          aria-label="resize panes"
          aria-valuenow={split}
          aria-valuemin={MIN_SPLIT}
          aria-valuemax={MAX_SPLIT}
          tabIndex={0}
          title="drag to resize · double-click for half and half"
          onPointerDown={(e) => {
            e.preventDefault();
            e.currentTarget.setPointerCapture(e.pointerId);
            document.body.classList.add("splitting");
          }}
          onPointerMove={dragSplit}
          onPointerUp={(e) => {
            e.currentTarget.releasePointerCapture(e.pointerId);
            document.body.classList.remove("splitting");
          }}
          onPointerCancel={() => document.body.classList.remove("splitting")}
          onDoubleClick={() => setSplit(DEFAULT_SPLIT)}
          onKeyDown={(e) => {
            if (e.key === "ArrowLeft") setSplit((v) => clampSplit(v - 5));
            else if (e.key === "ArrowRight") setSplit((v) => clampSplit(v + 5));
            else if (e.key === "Home") setSplit(DEFAULT_SPLIT);
          }}
        />
        <aside className="job-side">
          {form !== null ? (
            <>
              <div className="side-main">
                <ConversationPanel
                  key={`${slug}:${cur.id}`}
                  slug={slug}
                  thread={cur.id}
                  opened={opened}
                  pi={cur.pi ?? job?.pi}
                  head={sideHead}
                  hidden={side !== "conversation"}
                  onOpenLink={openLink}
                />
                <NotesPanel
                  slug={slug}
                  initial={form.notes}
                  head={sideHead}
                  hidden={side !== "notes"}
                  onSaved={() => void board.refetch()}
                  registerFlush={registerNotesFlush}
                />
              </div>
              <LinksPanel
                slug={slug}
                links={job?.links ?? form.links}
                onChanged={() => {
                  void board.refetch();
                  void loadForm();
                }}
              />
            </>
          ) : null}
        </aside>
      </div>
    </>
  );
}

/** What the overlay calls the job: its name, else its slug. */
function title0(job: WebJob | undefined, form: JobForm | null, slug: string): string {
  return job?.name || form?.name || job?.display_slug || form?.slug || slug;
}
