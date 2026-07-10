import { CardLadderTab } from './CardLadderTab';
import { DHTab } from './DHTab';
import { MarketMoversTab } from './MarketMoversTab';
import { PSASyncTab } from './PSASyncTab';
import { useCardLadderStatus, useDHStatus, useMarketMoversStatus, usePSASyncStatus } from '../../queries/useAdminQueries';
import SalesImportSection from '../tools/SalesImportSection';
import { StatusPill } from '../../ui';

const SECTION_HEADER = 'text-sm font-semibold uppercase tracking-wider text-[var(--text-muted)] mb-3';

/** Relative time pulse — "X ago" with second/minute/hour/day buckets.
    Returns null when the timestamp is missing or in the future, so call
    sites can render-or-omit without a guard. Intentionally coarse (no
    "5.32 minutes ago") to match the "last activity hint" intent rather
    than a precision indicator. */
function relativeTimeAgo(iso?: string | null): string | null {
  if (!iso) return null;
  const t = new Date(iso).getTime();
  if (Number.isNaN(t)) return null;
  const diffMs = Date.now() - t;
  if (diffMs < 0) return null;
  const sec = Math.floor(diffMs / 1000);
  if (sec < 60) return `${sec}s ago`;
  const min = Math.floor(sec / 60);
  if (min < 60) return `${min}m ago`;
  const hr = Math.floor(min / 60);
  if (hr < 24) return `${hr}h ago`;
  const d = Math.floor(hr / 24);
  return `${d}d ago`;
}

function PulseLine({ at }: { at?: string | null }) {
  const rel = relativeTimeAgo(at);
  if (!rel) return null;
  return (
    <span className="text-[10px] text-[var(--text-subtle)] tabular-nums tracking-wider uppercase">
      Last call {rel}
    </span>
  );
}

export function IntegrationsTab({ enabled = true }: { enabled?: boolean }) {
  const { data: dhStatus } = useDHStatus({ enabled });
  const { data: clStatus } = useCardLadderStatus({ enabled });
  const { data: mmStatus } = useMarketMoversStatus({ enabled });
  const { data: psaStatus } = usePSASyncStatus({ enabled });

  const dhHealthy = dhStatus?.api_health ? dhStatus.api_health.success_rate >= 0.95 : false;
  const clConnected = clStatus?.configured ?? false;
  const mmConnected = mmStatus?.configured ?? false;
  const psaConfigured = psaStatus?.configured ?? false;

  // Page-level DH error banner: surfaced when DH has either an active
  // bulk-match error or a measurably-bad recent success rate. Both signals
  // are already reachable via the per-section UI, but at the section level
  // they're easy to miss while scanning the page top-down. The banner
  // catches the operator's eye immediately on page load when something is
  // broken globally, without duplicating routine "configure credentials"
  // hints from the per-section error states.
  const dhBanner = dhStatus?.bulk_match_error
    ? `DoubleHolo bulk-match error: ${dhStatus.bulk_match_error}`
    : (dhStatus?.api_health && dhStatus.api_health.total_calls > 0 && dhStatus.api_health.success_rate < 0.5)
      ? `DoubleHolo API success rate is ${(dhStatus.api_health.success_rate * 100).toFixed(0)}% (${dhStatus.api_health.failures} of ${dhStatus.api_health.total_calls} recent calls failed). Check credentials and rate limits.`
      : null;

  // Pick the most-recent activity timestamp per integration as the "last
  // call" signal. DH has multiple candidate timestamps; take the freshest.
  // Compare numerically (ms-epoch) rather than lexically — ISO-8601 strings
  // with consistent `Z` suffixes happen to sort right, but a mixed-format
  // payload (e.g. one offset like +05:00 next to a Z value) would sort wrong
  // silently. Numeric compare is honest about what "freshest" means.
  const dhLastCall = ([
    dhStatus?.last_orders_poll_at,
    dhStatus?.intelligence_last_fetch,
    dhStatus?.suggestions_last_fetch,
  ]
    .filter((t): t is string => !!t)
    .map((t) => ({ t, ms: Date.parse(t) }))
    .filter((x) => Number.isFinite(x.ms))
    .reduce<{ t: string; ms: number } | null>(
      (best, x) => (best === null || x.ms > best.ms ? x : best),
      null,
    ))?.t;

  return (
    <div className="space-y-8 mt-4">
      {dhBanner && (
        <div
          role="alert"
          aria-live="assertive"
          className="rounded-lg border border-[var(--danger-border)] bg-[var(--danger-bg)] px-4 py-3 text-sm text-[var(--danger)] flex items-start gap-2"
        >
          <span aria-hidden="true">▴</span>
          <span>{dhBanner}</span>
        </div>
      )}

      <section>
        <div className="flex items-center justify-between mb-1">
          <h3 className={SECTION_HEADER + ' !mb-0'}>DoubleHolo</h3>
          <div className="flex items-center gap-3">
            <PulseLine at={dhLastCall} />
            {dhHealthy ? (
              <StatusPill tone="success">Healthy</StatusPill>
            ) : (
              <StatusPill tone="neutral">Unknown</StatusPill>
            )}
          </div>
        </div>
        <div className="mb-3" />
        <DHTab enabled={enabled} />
      </section>

      <hr className="border-[var(--surface-2)]" />

      <section>
        <div className="flex items-center justify-between mb-1">
          <h3 className={SECTION_HEADER + ' !mb-0'}>Card Ladder</h3>
          <div className="flex items-center gap-3">
            <PulseLine at={clStatus?.lastRun?.lastRunAt} />
            {clConnected ? (
              <StatusPill tone="success">Connected</StatusPill>
            ) : (
              <StatusPill tone="danger">Not connected</StatusPill>
            )}
          </div>
        </div>
        <div className="mb-3" />
        <CardLadderTab enabled={enabled} />
      </section>

      <hr className="border-[var(--surface-2)]" />

      <section>
        <div className="flex items-center justify-between mb-1">
          <h3 className={SECTION_HEADER + ' !mb-0'}>Market Movers</h3>
          <div className="flex items-center gap-3">
            <PulseLine at={mmStatus?.lastRun?.lastRunAt} />
            {mmConnected ? (
              <StatusPill tone="success">Connected</StatusPill>
            ) : (
              <StatusPill tone="danger">Not connected</StatusPill>
            )}
          </div>
        </div>
        <div className="mb-3" />
        <MarketMoversTab enabled={enabled} />
      </section>

      <hr className="border-[var(--surface-2)]" />

      <section>
        <div className="flex items-center justify-between mb-1">
          <h3 className={SECTION_HEADER + ' !mb-0'}>PSA Portal Sync</h3>
          <div className="flex items-center gap-3">
            <PulseLine at={psaStatus?.lastRun?.lastRunAt} />
            {psaConfigured ? (
              <StatusPill tone="success">Configured</StatusPill>
            ) : (
              <StatusPill tone="danger">Not configured</StatusPill>
            )}
          </div>
        </div>
        <div className="mb-3" />
        <PSASyncTab enabled={enabled} />
      </section>

      <hr className="border-[var(--surface-2)]" />

      <section>
        <div className="mb-3">
          <h3 className={SECTION_HEADER}>Import Sales</h3>
          <p className="text-xs text-[var(--text-muted)] mt-0.5">Import sales from order CSVs.</p>
        </div>
        <SalesImportSection />
      </section>
    </div>
  );
}
