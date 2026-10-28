/**
 * ⌘← / ⌘→ in the pane: line start / line end, as everywhere else on macOS.
 *
 * xterm has no sequence for a ⌘-arrow and drops the key, so pi's editor never
 * hears of it (WebKit then moves the caret of xterm's hidden textarea, which
 * is nothing). We send what the Home / End keys send — xterm's own encoding,
 * honouring application cursor keys mode; tmux knows both forms and pi binds
 * `home` / `end` to its line start / end. Any other modifier is left to xterm.
 */
export function metaArrowSequence(
  ev: Pick<KeyboardEvent, "key" | "metaKey" | "altKey" | "ctrlKey" | "shiftKey">,
  applicationCursorKeys: boolean,
): string | null {
  if (!ev.metaKey || ev.altKey || ev.ctrlKey || ev.shiftKey) return null;
  switch (ev.key) {
    case "ArrowLeft":
      return applicationCursorKeys ? "\x1bOH" : "\x1b[H";
    case "ArrowRight":
      return applicationCursorKeys ? "\x1bOF" : "\x1b[F";
    default:
      return null;
  }
}
