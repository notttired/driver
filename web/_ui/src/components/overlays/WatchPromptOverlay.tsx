import { useState } from "react";

import { setWatchPrompt } from "../../api/client.ts";
import { ESC_PRIORITY } from "../../hooks/useEscapeStack.ts";
import { Overlay, type Mutate } from "./Overlay.tsx";

export interface WatchPromptOverlayProps {
  open: boolean;
  onClose: () => void;
  /** what WATCH says to pi today */
  prompt: string;
  mutate: Mutate;
  onToast: (message: string) => void;
}

/** The WATCHING column's prompt: what WATCH pastes into pi. One text for the
 *  whole board; RESET goes back to the daemon's default. */
export function WatchPromptOverlay({ open, onClose, prompt, mutate, onToast }: WatchPromptOverlayProps) {
  return (
    <Overlay id="watch-prompt-overlay" open={open} priority={ESC_PRIORITY.job} onClose={onClose} panelClassName="new-panel">
      <WatchPromptBody onClose={onClose} prompt={prompt} mutate={mutate} onToast={onToast} />
    </Overlay>
  );
}

function WatchPromptBody({ onClose, prompt, mutate, onToast }: Omit<WatchPromptOverlayProps, "open">) {
  const [text, setText] = useState(prompt);
  const [busy, setBusy] = useState(false);
  const save = async (value: string) => {
    if (busy) return;
    setBusy(true);
    const err = await mutate(() => setWatchPrompt(value));
    setBusy(false);
    if (err) {
      onToast(`watch prompt: ${err.message}`);
      return;
    }
    onClose();
  };
  return (
    <>
      <div className="panel-head">
        <span className="panel-title">WATCH PROMPT</span>
        <span className="spacer" />
        <button className="btn ghost" title="close (Esc)" onClick={onClose}>
          ✕
        </button>
      </div>
      <div className="editor-body">
        <label className="f-field grow">
          WHAT WATCH SAYS TO PI
          <textarea
            className="notes-text watch-text"
            aria-label="watch prompt"
            autoFocus
            spellCheck={false}
            value={text}
            disabled={busy}
            onChange={(e) => setText(e.target.value)}
          />
        </label>
        <div className="editor-foot">
          <span className="rename-hint">
            → pasted into pi's pane as one message; a job whose pi is gone gets it as pi's first message
          </span>
          <span className="spacer" />
          <button className="btn ghost" disabled={busy} title="back to the built-in prompt" onClick={() => void save("")}>
            ↺ RESET
          </button>
          <button
            className="btn primary"
            disabled={busy || !text.trim() || text === prompt}
            title="save the prompt for every WATCH from now on"
            onClick={() => void save(text)}
          >
            ✓ SAVE
          </button>
        </div>
      </div>
    </>
  );
}
