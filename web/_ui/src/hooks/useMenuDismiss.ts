import { useEffect, type Dispatch, type RefObject, type SetStateAction } from "react";

/** Close a popup menu on Escape (capture phase, so the overlay's own Escape
 *  layer never sees the key) or on a mousedown outside its root. */
export function useMenuDismiss(
  open: boolean,
  setOpen: Dispatch<SetStateAction<boolean>>,
  rootRef: RefObject<HTMLElement | null>,
) {
  useEffect(() => {
    if (!open) return;
    const onKey = (ev: KeyboardEvent) => {
      if (ev.key !== "Escape") return;
      ev.stopPropagation();
      setOpen(false);
    };
    document.addEventListener("keydown", onKey, true);
    return () => document.removeEventListener("keydown", onKey, true);
  }, [open, setOpen]);

  useEffect(() => {
    if (!open) return;
    const onDown = (ev: MouseEvent) => {
      const root = rootRef.current;
      if (root && ev.target instanceof Node && root.contains(ev.target)) return;
      setOpen(false);
    };
    document.addEventListener("mousedown", onDown);
    return () => document.removeEventListener("mousedown", onDown);
  }, [open, setOpen, rootRef]);
}
