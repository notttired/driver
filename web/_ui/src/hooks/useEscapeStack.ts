import { useEffect } from "react";

// One document-level Escape listener; the topmost open layer (highest
// priority) takes the key. Menus dismiss themselves on a CAPTURE-phase
// listener (useMenuDismiss) and stop propagation, so they always win.
export const ESC_PRIORITY = {
  job: 20,
} as const;

type Registered = { priority: number; close: () => void };

const layers = new Set<Registered>();
let listening = false;

function onKeyDown(e: KeyboardEvent) {
  if (e.key !== "Escape") return;
  let top: Registered | undefined;
  for (const l of layers) {
    if (!top || l.priority > top.priority) top = l;
  }
  if (!top) return;
  e.preventDefault();
  top.close();
}

export function useEscapeLayer(open: boolean, priority: number, close: () => void): void {
  useEffect(() => {
    if (!open) return;
    const entry: Registered = { priority, close };
    layers.add(entry);
    if (!listening) {
      document.addEventListener("keydown", onKeyDown);
      listening = true;
    }
    return () => {
      layers.delete(entry);
      if (layers.size === 0 && listening) {
        document.removeEventListener("keydown", onKeyDown);
        listening = false;
      }
    };
  }, [open, priority, close]);
}
