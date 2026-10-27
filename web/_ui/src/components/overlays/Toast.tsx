import { useEffect, useState } from "react";

type Listener = (message: string, seq: number) => void;

let listener: Listener | null = null;
let seq = 0;

/** Bottom-centre notice for a few seconds — a refused drag, a failed reorder. */
export function showToast(message: string): void {
  seq++;
  listener?.(message, seq);
}

export function Toast() {
  const [state, setState] = useState<{ message: string; seq: number } | null>(null);
  const [shown, setShown] = useState(false);

  useEffect(() => {
    listener = (message, s) => {
      setState({ message, seq: s });
      setShown(true);
    };
    return () => {
      listener = null;
    };
  }, []);

  useEffect(() => {
    if (!state) return;
    const id = window.setTimeout(() => setShown(false), 4000);
    return () => window.clearTimeout(id);
  }, [state]);

  return (
    <div id="toast" className={shown ? "toast show" : "toast"} role="status">
      {state?.message ?? ""}
    </div>
  );
}
