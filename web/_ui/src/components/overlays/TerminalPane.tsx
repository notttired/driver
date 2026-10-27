import { useEffect, useRef, useState } from "react";
import { Terminal } from "@xterm/xterm";
import { FitAddon } from "@xterm/addon-fit";
import { WebLinksAddon } from "@xterm/addon-web-links";
import "@xterm/xterm/css/xterm.css";

export const SESSION_ENDED = "session ended — reactivate to continue";

/** `ws(s)://<host>/api/term?slug=…` — same origin, so the vite proxy and the
 *  embedded server both work. */
export function termSocketUrl(slug: string, loc: Pick<Location, "protocol" | "host"> = window.location): string {
  const proto = loc.protocol === "https:" ? "wss:" : "ws:";
  return `${proto}//${loc.host}/api/term?slug=${encodeURIComponent(slug)}`;
}

/** Resolve a `:root` custom property to its computed string (hex primitives
 *  only — color-mix() does not resolve on custom properties). */
function cssVar(name: string, fallback: string): string {
  if (typeof getComputedStyle !== "function") return fallback;
  const v = getComputedStyle(document.documentElement).getPropertyValue(name).trim();
  return v && !v.includes("(") ? v : fallback;
}

function readTheme() {
  const light = document.documentElement.dataset.theme === "light";
  return {
    background: cssVar("--term-bg", light ? "#f6f5fb" : "#0a0a0f"),
    foreground: cssVar("--term-fg", light ? "#1c1b22" : "#e4e4ea"),
    cursor: cssVar("--ac", "#8b6cff"),
    cursorAccent: cssVar("--term-bg", light ? "#f6f5fb" : "#0a0a0f"),
    selectionBackground: cssVar("--term-sel", light ? "#cdc2ff" : "#3a2f73"),
  };
}

export interface TerminalPaneProps {
  slug: string;
  /** true whenever a tmux session can exist — an opened draft (pi at its
   *  prompt), working and blocked jobs, whose session stays even after pi
   *  exits (dead pane, last screen); false for done jobs, whose session was
   *  killed: the pane shows the ended banner without connecting */
  attach: boolean;
}

interface Ended {
  code: number | null;
}

/**
 * An xterm over the daemon's attach socket. Binary frames are pty bytes both
 * ways; text frames are JSON (`resize` out right after open and on every
 * fit, `exit` in right before the server closes). A 404/409 before the
 * upgrade closes the socket at once, which lands in the same banner.
 */
export function TerminalPane({ slug, attach }: TerminalPaneProps) {
  const hostRef = useRef<HTMLDivElement | null>(null);
  const [ended, setEnded] = useState<Ended | null>(attach ? null : { code: null });
  const [connected, setConnected] = useState(false);

  useEffect(() => {
    setEnded(attach ? null : { code: null });
    setConnected(false);
    if (!attach) return;
    const host = hostRef.current;
    if (!host) return;

    const term = new Terminal({
      fontFamily: "'IBM Plex Mono', Menlo, Monaco, monospace",
      fontSize: 12,
      lineHeight: 1.2,
      cursorBlink: true,
      scrollback: 5000,
      allowProposedApi: true,
      theme: readTheme(),
    });
    const fit = new FitAddon();
    term.loadAddon(fit);
    term.loadAddon(new WebLinksAddon());
    term.open(host);

    const ws = new WebSocket(termSocketUrl(slug));
    ws.binaryType = "arraybuffer";
    const encoder = new TextEncoder();
    let closed = false;

    const sendResize = () => {
      if (ws.readyState !== WebSocket.OPEN) return;
      ws.send(JSON.stringify({ type: "resize", cols: term.cols, rows: term.rows }));
    };
    const refit = () => {
      try {
        fit.fit();
      } catch {
        // the host can be display:none for a frame while the panel animates
      }
      sendResize();
    };

    ws.onopen = () => {
      setConnected(true);
      refit();
      term.focus();
    };
    ws.onmessage = (ev: MessageEvent<ArrayBuffer | string>) => {
      if (typeof ev.data === "string") {
        try {
          const msg = JSON.parse(ev.data) as { type?: string; code?: number };
          if (msg.type === "exit") setEnded({ code: typeof msg.code === "number" ? msg.code : null });
        } catch {
          // not for us
        }
        return;
      }
      term.write(new Uint8Array(ev.data));
    };
    ws.onclose = () => {
      closed = true;
      setConnected(false);
      setEnded((e) => e ?? { code: null });
    };
    ws.onerror = () => {
      // close follows; the banner comes from onclose
    };

    const dataSub = term.onData((d) => {
      if (ws.readyState === WebSocket.OPEN) ws.send(encoder.encode(d));
    });
    const binSub = term.onBinary((d) => {
      if (ws.readyState !== WebSocket.OPEN) return;
      const bytes = new Uint8Array(d.length);
      for (let i = 0; i < d.length; i++) bytes[i] = d.charCodeAt(i) & 0xff;
      ws.send(bytes);
    });

    const ro = typeof ResizeObserver === "function" ? new ResizeObserver(() => refit()) : null;
    ro?.observe(host);
    window.addEventListener("resize", refit);
    const themeObs =
      typeof MutationObserver === "function"
        ? new MutationObserver(() => {
            term.options.theme = readTheme();
          })
        : null;
    themeObs?.observe(document.documentElement, { attributes: true, attributeFilter: ["data-theme", "data-accent"] });
    refit();

    return () => {
      window.removeEventListener("resize", refit);
      ro?.disconnect();
      themeObs?.disconnect();
      dataSub.dispose();
      binSub.dispose();
      if (!closed) {
        try {
          ws.close();
        } catch {
          // already gone
        }
      }
      term.dispose();
    };
  }, [slug, attach]);

  return (
    <div className={"term-pane" + (ended ? " ended" : "") + (connected ? " connected" : "")} data-no-drag>
      <div className="term-host" ref={hostRef} />
      {ended ? (
        <div className="term-ended" role="status">
          {SESSION_ENDED}
          {ended.code !== null ? <span className="code"> · exit {ended.code}</span> : null}
        </div>
      ) : null}
    </div>
  );
}
