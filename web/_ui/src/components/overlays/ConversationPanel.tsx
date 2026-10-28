import { Fragment, useLayoutEffect, useRef } from "react";

import { fetchConversation } from "../../api/client.ts";
import { usePoll } from "../../hooks/usePoll.ts";
import { clockLabel } from "../../lib/format.ts";
import { splitUrls } from "../../lib/links.ts";
import type { WebConversation, WebMessage, WebPi } from "../../lib/types.ts";

export const CONVERSATION_POLL_MS = 3000;

export interface ConversationPanelProps {
  slug: string;
  /** the job has (or had) a pi session; nothing to read before that */
  opened: boolean;
  /** the board's word on pi, for the row of a turn still in progress */
  pi?: WebPi;
}

/**
 * The record of the conversation so far, beside the notes: what you typed
 * and pi's final answer to each message — the last thing it said before it
 * stopped — as the daemon reads them from the session file
 * (GET /api/job/conversation). The narration between tool calls, the tool
 * calls and their output stay in the pane. Polled while the overlay is open;
 * the list keeps to its end until you scroll up.
 */
export function ConversationPanel({ slug, opened, pi }: ConversationPanelProps) {
  const last = useRef<WebConversation | null>(null);
  const { data, error } = usePoll<WebConversation>(
    async (signal) => {
      const next = await fetchConversation(slug, signal);
      // an unchanged rev is an unchanged record: hand back the old object so
      // nothing re-renders
      if (last.current && last.current.rev === next.rev) return last.current;
      last.current = next;
      return next;
    },
    CONVERSATION_POLL_MS,
    { enabled: opened },
  );
  const messages = data?.messages ?? [];
  const pending = data?.pending ?? false;

  const listRef = useRef<HTMLOListElement | null>(null);
  const pinned = useRef(true);
  useLayoutEffect(() => {
    const el = listRef.current;
    if (el && pinned.current) el.scrollTop = el.scrollHeight;
  }, [data]);

  const piState = pi?.state;
  const working = piState === "working" || piState === "starting";
  const turns = messages.filter((m) => m.role === "user").length;
  const empty = !opened || (data !== undefined && messages.length === 0 && !pending);

  return (
    <section className="side-panel convo-panel">
      <div className="side-head">
        <b>CONVERSATION</b>
        <span className={"side-hint" + (error ? " err" : "")}>
          {error ? `✗ ${error.message}` : turns ? `${turns} ${turns === 1 ? "turn" : "turns"}` : ""}
        </span>
      </div>
      {empty ? (
        <div className="convo-empty">
          {opened
            ? "nothing yet — what you type to pi and its final answer to each message appear here"
            : "the record starts with the first message you send pi"}
        </div>
      ) : (
        <ol
          className="convo-list"
          ref={listRef}
          onScroll={(e) => {
            const el = e.currentTarget;
            pinned.current = el.scrollHeight - el.scrollTop - el.clientHeight < 24;
          }}
        >
          {messages.map((m, i) => (
            <MessageRow key={i} m={m} />
          ))}
          {pending ? (
            <li className={"convo-msg pi" + (working ? " convo-pending" : "")}>
              <span className="convo-who">pi</span>
              <p className="convo-text convo-end">
                {working ? "working…" : piState === "dialog" ? "asking you something in the pane" : "stopped before answering"}
              </p>
            </li>
          ) : null}
        </ol>
      )}
    </section>
  );
}

function MessageRow({ m }: { m: WebMessage }) {
  const you = m.role === "user";
  const when = clockLabel(m.ts);
  return (
    <li className={"convo-msg " + (you ? "you" : "pi")}>
      <span className="convo-who">
        {you ? "you" : "pi"}
        {when && m.ts ? <time dateTime={new Date(m.ts * 1000).toISOString()}>{when}</time> : null}
      </span>
      {m.text ? <p className="convo-text">{linkify(m.text)}</p> : null}
      {m.end ? <p className="convo-text convo-end">{endLabel(m)}</p> : null}
    </li>
  );
}

function endLabel(m: WebMessage): string {
  switch (m.end) {
    case "aborted":
      return "— interrupted";
    case "error":
      return m.error ? `— error: ${m.error}` : "— error";
    case "length":
      return "— cut off by the output limit";
    default:
      return m.end ? `— ${m.end}` : "";
  }
}

/** The text with its URLs as anchors: click or ⌘-click opens the page in the
 *  browser (Driver.app routes it there), as in the links panel. */
function linkify(text: string) {
  return splitUrls(text).map((p, i) =>
    p.url ? (
      <a key={i} href={p.url} target="_blank" rel="noreferrer" title="click or ⌘-click: open in the browser">
        {p.text}
      </a>
    ) : (
      <Fragment key={i}>{p.text}</Fragment>
    ),
  );
}
