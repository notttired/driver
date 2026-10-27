import { useEffect, useState } from "react";

import { age } from "../../lib/format.ts";
import type { WebBoard } from "../../lib/types.ts";
import { AccentToggle } from "./HudControls.tsx";
import { ThemeToggle } from "./ThemeToggle.tsx";

/** how long a failed write's sentence holds the clock slot */
const STATUS_ERROR_MS = 10_000;

export interface ChromeProps {
  board: WebBoard | undefined;
  /** the board poll's latest failure */
  error: Error | undefined;
  /** the latest failed write's server sentence */
  lastError: string | null;
}

export function Chrome({ board, error, lastError }: ChromeProps) {
  const [now, setNow] = useState(() => Date.now());
  useEffect(() => {
    const id = window.setInterval(() => setNow(Date.now()), 1000);
    return () => window.clearInterval(id);
  }, []);

  const [errAt, setErrAt] = useState(0);
  useEffect(() => {
    if (lastError) setErrAt(Date.now());
  }, [lastError]);
  const showErr = !!lastError && now - errAt < STATUS_ERROR_MS;

  const ok = !error && !!board?.daemon_ok;
  const daemonText = error
    ? "driver web server unreachable"
    : !board
      ? "connecting…"
      : board.daemon_ok
        ? `daemon pid ${board.daemon.pid} · tick ${age(board.daemon.last_tick) || "0s"} ago`
        : "daemon not running";

  return (
    <header className="chrome">
      <span className="mark">
        DRIVER<b>▞</b>
      </span>
      <span className={"led" + (ok ? " ok" : "")} id="daemon-led" />
      <span className="stat" id="daemon-stat" title={board?.daemon.last_error || undefined}>
        {daemonText}
      </span>
      <span className="spacer" />
      <AccentToggle />
      <ThemeToggle />
      <span className={"stat dim" + (showErr ? " hot" : "")} id="tick-stat" title={lastError ?? undefined}>
        {showErr ? "✕ " + lastError : new Date(now).toLocaleTimeString()}
        {showErr ? null : <i>▊</i>}
      </span>
    </header>
  );
}
