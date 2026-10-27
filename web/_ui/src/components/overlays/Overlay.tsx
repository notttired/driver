import type { ReactNode } from "react";

import { useEscapeLayer } from "../../hooks/useEscapeStack.ts";

export interface OverlayProps {
  id: string;
  open: boolean;
  priority: number;
  onClose: () => void;
  children: ReactNode;
  panelClassName?: string;
}

/** The scrim + panel every modal shares. Children mount only while open, so
 *  closing is a wholesale reset; a backdrop click or Escape closes. */
export function Overlay({ id, open, priority, onClose, children, panelClassName }: OverlayProps) {
  useEscapeLayer(open, priority, onClose);
  return (
    <div
      className="overlay"
      id={id}
      hidden={!open}
      onClick={(e) => {
        if (e.target === e.currentTarget) onClose();
      }}
    >
      <div className={panelClassName ? `panel ${panelClassName}` : "panel"}>
        {open ? children : null}
      </div>
    </div>
  );
}

/** The app's write path handed to overlays: run the request, refetch the
 *  board either way, and hand back the failure (its message is the server's
 *  own sentence) or null. */
export type Mutate = (op: () => Promise<unknown>) => Promise<Error | null>;
