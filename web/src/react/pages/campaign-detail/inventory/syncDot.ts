// Sync status dot computation — separated from main inventory utilities.
//
// The dot answers "is the price data on this row trustworthy right now?", not
// "did we ping the upstream API recently?". A green dot that sits next to a
// row with no CL price is exactly the bug this module is designed to avoid —
// presence of a value matters, not whether the sync loop ran.

export interface SyncDotProps {
  color: string;   // CSS color value
  tooltip: string; // native title= tooltip string
}

export interface SyncDotInput {
  clSyncedAt?: string;
  mmValueUpdatedAt?: string;
  dhLastSyncedAt?: string;
  /** Does DH (primary price source) currently have a usable price on the row? */
  hasDHPrice?: boolean;
  /** Did CL return a non-zero value, directly or via catalog fallback? */
  clHasValue?: boolean;
  /** Does MM currently have a usable value on the row? */
  hasMMValue?: boolean;
  /** Current CL error/status tag, if any (e.g. 'no_value', 'catalog_fallback'). */
  clLastError?: string;
}

/** Returns color + tooltip for the per-row sync freshness dot.
 *
 *  Green  — DH has a price AND DH synced within 24h (primary source is healthy and current).
 *  Yellow — DH synced recently but is missing a price, OR DH stale while CL/MM still have a value.
 *  Red    — no fresh data from any source.
 *  Grey   — nothing has ever synced.
 */
export function syncDotProps(input: SyncDotInput): SyncDotProps {
  const { clSyncedAt, mmValueUpdatedAt, dhLastSyncedAt, hasDHPrice, clHasValue, hasMMValue, clLastError } = input;
  const now = Date.now();
  const threshold = 24 * 60 * 60 * 1000;

  function within24h(ts: string | undefined): boolean {
    if (!ts) return false;
    const t = new Date(ts).getTime();
    if (isNaN(t)) return false;
    if (t >= now) return true;
    return now - t <= threshold;
  }

  const dhFresh = within24h(dhLastSyncedAt);
  const clFresh = within24h(clSyncedAt);
  const mmFresh = within24h(mmValueUpdatedAt);

  const hasAnyTimestamp = !!(clSyncedAt || mmValueUpdatedAt || dhLastSyncedAt);
  const anyFreshWithValue =
    (dhFresh && hasDHPrice) ||
    (clFresh && clHasValue) ||
    (mmFresh && hasMMValue);

  let color: string;
  if (dhFresh && hasDHPrice) {
    color = '#22c55e'; // green — primary source is healthy
  } else if (anyFreshWithValue || dhFresh || clFresh || mmFresh) {
    color = '#f59e0b'; // yellow — stale, partial, or synced-without-value
  } else if (!hasAnyTimestamp) {
    color = '#6b7280'; // grey — never synced
  } else {
    color = '#ef4444'; // red — nothing fresh
  }

  const tooltip = [
    clLine(clSyncedAt, clHasValue, clLastError),
    sourceLine('MM', mmValueUpdatedAt, hasMMValue),
    sourceLine('DH', dhLastSyncedAt, hasDHPrice),
  ].join('\n');

  return { color, tooltip };
}

function clLine(ts: string | undefined, hasValue: boolean | undefined, lastError: string | undefined): string {
  if (!ts) return 'CL · never';
  const age = timeAgo(ts);
  if (lastError === 'no_value') return `CL · matched, no value · ${age}`;
  if (lastError === 'catalog_fallback') return `CL · catalog fallback · ${age}`;
  if (lastError === 'api_error') return `CL · api error · ${age}`;
  if (hasValue) return `CL · ✓ · ${age}`;
  return `CL · ${age}`;
}

function sourceLine(label: string, ts: string | undefined, hasValue: boolean | undefined): string {
  if (!ts) return `${label} · never`;
  const age = timeAgo(ts);
  if (hasValue) return `${label} · ✓ · ${age}`;
  return `${label} · no value · ${age}`;
}

function timeAgo(ts: string): string {
  const now = Date.now();
  const t = new Date(ts).getTime();
  if (isNaN(t)) return 'unknown';
  if (t > now) return 'just now';
  const diffMs = now - t;
  const diffM = Math.floor(diffMs / 60000);
  if (diffM < 1) return 'just now';
  const diffH = Math.floor(diffMs / 3600000);
  if (diffH < 1) return `${diffM}m ago`;
  if (diffH < 24) return `${diffH}h ago`;
  const diffD = Math.floor(diffMs / 86400000);
  return `${diffD}d ago`;
}
