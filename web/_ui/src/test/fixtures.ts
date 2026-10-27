import type { WebBoard, WebDaemon, WebJob } from "../lib/types.ts";

const NOW = Math.floor(Date.now() / 1000);

export function makeDaemon(over: Partial<WebDaemon> = {}): WebDaemon {
  return { pid: 4242, started_at: NOW - 3600, last_tick: NOW - 2, live: 1, ...over };
}

export function makeJob(over: Partial<WebJob> = {}): WebJob {
  return {
    slug: "demo",
    dir: "/Users/me/src/demo",
    state: "draft",
    live: false,
    has_notes: false,
    mtime: NOW - 300,
    ...over,
  };
}

export function makeBoard(over: Partial<WebBoard> = {}): WebBoard {
  return {
    daemon: makeDaemon(),
    daemon_ok: true,
    jobs: [],
    contract: 1,
    operator: "me",
    default_dir: "/Users/me/src",
    ...over,
  };
}
