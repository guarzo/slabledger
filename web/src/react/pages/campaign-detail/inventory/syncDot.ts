// Sync status dot computation — separated from main inventory utilities.

export interface SyncDotProps {
  color: string;   // CSS color value
  tooltip: string; // native title= tooltip string
}

/** Returns color + tooltip for the per-row sync freshness dot.
 *  Green = all 3 synced within 24h
 *  Yellow = ≥1 synced within 24h, not all 3
 *  Red = none synced within 24h (or all timestamps missing)
 */
export function syncDotProps(
  clSyncedAt: string | undefined,
  mmValueUpdatedAt: string | undefined,
  dhLastSyncedAt: string | undefined,
): SyncDotProps {
  const now = Date.now();
  const threshold = 24 * 60 * 60 * 1000; // 24h in ms

  function within24h(ts: string | undefined): boolean {
    if (!ts) return false;
    const t = new Date(ts).getTime();
    if (isNaN(t)) return false;
    if (t >= now) return true; // future or now → treat as just synced
    return now - t <= threshold;
  }

  const cl = within24h(clSyncedAt);
  const mm = within24h(mmValueUpdatedAt);
  const dh = within24h(dhLastSyncedAt);

  const freshCount = [cl, mm, dh].filter(Boolean).length;

  function fmt(label: string, ts: string | undefined): string {
    if (!ts) return `${label} · never`;
    const t = new Date(ts).getTime();
    if (isNaN(t)) return `${label} · unknown`;
    if (t > now) return `${label} · just now`;
    const diffMs = now - t;
    const diffH = Math.floor(diffMs / 3600000);
    const diffM = Math.floor(diffMs / 60000);
    if (diffM < 1) return `${label} · just now`;
    if (diffH < 1) return `${label} · ${diffM}m ago`;
    if (diffH < 24) return `${label} · ${diffH}h ago`;
    const diffD = Math.floor(diffMs / 86400000);
    return `${label} · ${diffD}d ago`;
  }

  const tooltip = [
    fmt('CL', clSyncedAt),
    fmt('MM', mmValueUpdatedAt),
    fmt('DH', dhLastSyncedAt),
  ].join('\n');

  let color: string;
  if (freshCount === 3) {
    color = '#22c55e'; // green
  } else if (freshCount >= 1) {
    color = '#f59e0b'; // yellow/amber
  } else {
    color = '#ef4444'; // red
  }

  return { color, tooltip };
}
