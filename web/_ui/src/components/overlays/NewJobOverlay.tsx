import { useEffect, useState } from "react";

import { ApiTimeout, createJob, fetchDirs, fetchModels, queueJob } from "../../api/client.ts";
import { useDebouncedCallback } from "../../hooks/useDebouncedDraft.ts";
import { ESC_PRIORITY } from "../../hooks/useEscapeStack.ts";
import { slugify } from "../../lib/slug.ts";
import type { ModelsResponse } from "../../lib/types.ts";
import { Overlay, type Mutate } from "./Overlay.tsx";

const DIRS_DEBOUNCE_MS = 200;

export interface NewJobOverlayProps {
  open: boolean;
  onClose: () => void;
  /** the job exists and pi has been asked to open in it; the app opens its overlay */
  onCreated: (slug: string) => void;
  mutate: Mutate;
  onToast: (message: string) => void;
  /** the daemon's default dir, seeded into the directory field */
  defaultDir: string;
}

export function NewJobOverlay({ open, onClose, onCreated, mutate, onToast, defaultDir }: NewJobOverlayProps) {
  return (
    <Overlay id="new-job-overlay" open={open} priority={ESC_PRIORITY.job} onClose={onClose} panelClassName="new-panel">
      <NewJobBody onClose={onClose} onCreated={onCreated} mutate={mutate} onToast={onToast} defaultDir={defaultDir} />
    </Overlay>
  );
}

type Busy = null | "creating" | "starting";

/**
 * Name, directory, model — then START: the job is created and pi opened in
 * it in one go. The dialog waits for pi's tmux session to come up so the job
 * overlay that follows opens straight onto the pane. There is no spec: the
 * first message is typed to pi itself.
 */
function NewJobBody({ onClose, onCreated, mutate, onToast, defaultDir }: Omit<NewJobOverlayProps, "open">) {
  const [name, setName] = useState("");
  const [dir, setDir] = useState(defaultDir);
  const [model, setModel] = useState("");
  const [busy, setBusy] = useState<Busy>(null);
  const [error, setError] = useState("");
  const [dirs, setDirs] = useState<string[]>([]);
  const [models, setModels] = useState<ModelsResponse | null>(null);
  const slug = slugify(name);

  useEffect(() => {
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
  }, []);

  // directory autocomplete follows the typed prefix
  const dirLookup = useDebouncedCallback((q: string) => {
    fetchDirs(q)
      .then((r) => setDirs(r.dirs))
      .catch(() => setDirs([]));
  }, DIRS_DEBOUNCE_MS);
  useEffect(() => {
    dirLookup.call(dir);
  }, [dir, dirLookup]);

  const start = async () => {
    if (!slug || busy) return;
    setBusy("creating");
    setError("");
    let created: string;
    try {
      created = (await createJob({ slug, name: name.trim(), dir, model })).slug;
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : String(e));
      setBusy(null);
      return;
    }
    // the job exists; opening pi takes a moment (tmux + pi startup). A
    // refusal leaves a not-started draft — its overlay has the form and START.
    setBusy("starting");
    const err = await mutate(() => queueJob(created));
    if (err instanceof ApiTimeout) onToast(`${created}: still starting — the board will catch up`);
    else if (err) onToast(`${created}: ${err.message}`);
    setBusy(null);
    onCreated(created);
  };

  const onEnter = (e: React.KeyboardEvent<HTMLInputElement>) => {
    if (e.key === "Enter") {
      e.preventDefault();
      void start();
    }
  };
  const locked = busy !== null;

  return (
    <>
      <div className="panel-head">
        <span className="panel-title">NEW JOB</span>
        <span className="spacer" />
        <button className="btn ghost" title="close (Esc)" onClick={onClose}>
          ✕
        </button>
      </div>
      <div className="editor-body">
        <label className="f-field grow">
          NAME
          <input
            id="editor-slug"
            placeholder="what this job is about"
            autoFocus
            spellCheck={false}
            value={name}
            disabled={locked}
            onChange={(e) => {
              setName(e.target.value);
              setError("");
            }}
            onKeyDown={onEnter}
          />
        </label>
        <div className="f-row">
          <label className="f-field grow">
            DIRECTORY
            <input
              id="new-dir"
              list="new-dir-options"
              autoComplete="off"
              spellCheck={false}
              placeholder="~/src/…"
              value={dir}
              disabled={locked}
              onChange={(e) => {
                setDir(e.target.value);
                setError("");
              }}
              onKeyDown={onEnter}
            />
            <datalist id="new-dir-options">
              {dirs.map((d) => (
                <option key={d} value={d} />
              ))}
            </datalist>
          </label>
          <label className="f-field">
            MODEL
            <input
              id="new-model"
              list="new-model-options"
              autoComplete="off"
              spellCheck={false}
              placeholder={models?.default ? `default · ${models.default}` : "default"}
              value={model}
              disabled={locked}
              onChange={(e) => setModel(e.target.value)}
              onKeyDown={onEnter}
            />
            <datalist id="new-model-options">
              {(models?.models ?? []).map((m) => (
                <option key={`${m.provider}/${m.id}`} value={`${m.provider}/${m.id}`} />
              ))}
            </datalist>
          </label>
        </div>
        <div className="editor-foot">
          <span className={"rename-hint" + (error ? " err" : "")} id="new-hint">
            {error ? (
              <>✗ {error}</>
            ) : busy === "starting" ? (
              "opening pi…"
            ) : slug ? (
              <>
                → slug <span className="slug">{slug}</span> · START opens pi in the directory; what you type first is its first prompt
              </>
            ) : (
              "→ a name needs at least one letter or digit"
            )}
          </span>
          <span className="spacer" />
          <button
            className="btn primary"
            id="new-start"
            disabled={!slug || locked}
            title="create the job and open pi in its directory"
            onClick={() => void start()}
          >
            {busy ? <span className="submit-spinner" aria-hidden="true" /> : null}▶ START
          </button>
        </div>
      </div>
    </>
  );
}
