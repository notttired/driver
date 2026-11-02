import { useState } from "react";

import type { WebJob } from "../../lib/types.ts";

export function RetryAll({ jobs, onRetry }: { jobs: WebJob[]; onRetry: (slugs: string[]) => Promise<void> }) {
  const [busy, setBusy] = useState(false);
  const slugs = jobs.filter((j) => j.retryable).map((j) => j.slug);
  if (!slugs.length) return null;
  return (
    <button
      className="btn ghost col-retry"
      id="retry-all"
      disabled={busy}
      title="start again every job here whose pi exited or gave up on its last turn"
      onClick={() => {
        setBusy(true);
        void onRetry(slugs).finally(() => setBusy(false));
      }}
    >
      ↻ retry {slugs.length}
    </button>
  );
}
