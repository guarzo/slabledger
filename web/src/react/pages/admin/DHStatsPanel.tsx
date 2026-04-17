import { useDHStatus } from '../../queries/useAdminQueries';
import { formatPct } from '../../utils/formatters';
import { CardShell } from '../../ui/CardShell';
import { SummaryCard } from './shared';
import { formatAdminDate } from './adminUtils';

function formatTimestamp(ts: string): string {
  return formatAdminDate(ts) === '-' ? 'Never' : formatAdminDate(ts);
}

interface HealthCardProps {
  label: string;
  value: string;
  valueColor?: string;
  sub?: string;
}

function HealthCard({ label, value, valueColor, sub }: HealthCardProps) {
  return (
    <div className="rounded-xl bg-[var(--surface-1)] border border-[var(--surface-2)] p-4">
      <div className="text-xs text-[var(--text-muted)] mb-1">{label}</div>
      <div className="text-2xl font-bold" style={valueColor ? { color: valueColor } : undefined}>
        {value}
      </div>
      {sub && <div className="text-xs text-[var(--text-muted)] mt-1">{sub}</div>}
    </div>
  );
}

export function DHStatsPanel({ enabled = true }: { enabled?: boolean }) {
  const { data: status, isLoading, error } = useDHStatus({ enabled });

  if (!enabled) {
    return (
      <CardShell padding="lg">
        <p className="text-[var(--text-muted)]">DH integration is not configured.</p>
      </CardShell>
    );
  }

  if (isLoading) {
    return (
      <CardShell padding="lg">
        <p className="text-[var(--text-muted)]">Loading DH status...</p>
      </CardShell>
    );
  }

  if (error && !status) {
    return (
      <CardShell padding="lg">
        <p className="text-[var(--danger)] text-sm">Failed to load DH status. Integration may not be configured.</p>
      </CardShell>
    );
  }

  const enrolledPending = status?.pending_count ?? 0;
  const pendingCount = status?.pending_received_count ?? 0;
  const awaitingReceipt = Math.max(enrolledPending - pendingCount, 0);
  const mappedCount = status?.mapped_count ?? 0;
  const unmatchedCount = status?.unmatched_count ?? 0;

  const total = mappedCount + unmatchedCount;
  const apiHealth = status?.api_health;
  const apiHealthValue = apiHealth ? formatPct(apiHealth.success_rate) : '—';
  const apiHealthClr = apiHealth ? (apiHealth.success_rate >= 0.95 ? 'var(--success)' : apiHealth.success_rate >= 0.80 ? 'var(--warning)' : 'var(--danger)') : undefined;
  const apiHealthSub = apiHealth ? `${apiHealth.total_calls} calls / ${apiHealth.failures} failures (7d)` : 'No data';
  const matchRateValue = total > 0 ? formatPct(mappedCount / total) : '—';
  const matchRateSub = total > 0 ? `${mappedCount} matched / ${unmatchedCount} unmatched` : '0 matched / 0 unmatched';
  const unmatchedPct = total > 0 ? formatPct(unmatchedCount / total) : '0%';

  return (
    <div className="space-y-4 mt-4">
      {/* Integration Health */}
      <div>
        <h4 className="text-xs font-semibold text-[var(--text-muted)] uppercase tracking-wide mb-2">Integration Health</h4>
        <div className="grid grid-cols-1 sm:grid-cols-3 gap-3">
          <HealthCard
            label="API Health"
            value={apiHealthValue}
            valueColor={apiHealthClr}
            sub={apiHealthSub}
          />
          <HealthCard
            label="Match Rate"
            value={matchRateValue}
            valueColor="var(--brand-500)"
            sub={matchRateSub}
          />
          <HealthCard
            label="Unmatched"
            value={String(unmatchedCount)}
            valueColor={unmatchedCount > 0 ? 'var(--warning)' : undefined}
            sub={`${unmatchedPct} of total inventory`}
          />
        </div>
      </div>

      {/* DoubleHolo Counts */}
      <div>
        <h4 className="text-xs font-semibold text-[var(--text-muted)] uppercase tracking-wide mb-2">DoubleHolo Counts</h4>
        <div className="grid grid-cols-1 sm:grid-cols-3 gap-3">
          <SummaryCard
            label="Inventory"
            value={status?.dh_inventory_count ?? '—'}
          />
          <SummaryCard
            label="Listings"
            value={status?.dh_listings_count ?? '—'}
          />
          <SummaryCard
            label="Orders"
            value={status?.dh_orders_count ?? '—'}
          />
        </div>
      </div>

      {/* Orders ingest health */}
      {status?.last_orders_poll_at && (
        <div className="rounded-xl bg-[var(--surface-1)] border border-[var(--surface-2)] p-4">
          <h4 className="text-xs font-semibold text-[var(--text-muted)] uppercase tracking-wide mb-3">Orders ingest (24h)</h4>
          <div className="grid grid-cols-2 sm:grid-cols-4 gap-3">
            <div>
              <div className="text-xs text-[var(--text-muted)] mb-1">Last poll</div>
              <div className="font-mono text-sm text-[var(--text-secondary)]">{new Date(status.last_orders_poll_at).toLocaleString()}</div>
            </div>
            <div>
              <div className="text-xs text-[var(--text-muted)] mb-1">Matched</div>
              <div className="text-lg font-semibold text-[var(--success)]">{status.orders_matched_count_24h ?? 0}</div>
            </div>
            <div>
              <div className="text-xs text-[var(--text-muted)] mb-1">Orphan</div>
              <div className="text-lg font-semibold text-[var(--warning)]">{status.orders_orphan_count_24h ?? 0}</div>
            </div>
            <div>
              <div className="text-xs text-[var(--text-muted)] mb-1">Already sold</div>
              <div className="text-lg font-semibold text-[var(--text-secondary)]">{status.orders_already_sold_count_24h ?? 0}</div>
            </div>
          </div>
        </div>
      )}

      {/* Market Data */}
      <div>
        <h4 className="text-xs font-semibold text-[var(--text-muted)] uppercase tracking-wide mb-2">Market Data</h4>
        <div className="grid grid-cols-2 sm:grid-cols-4 gap-3">
          <SummaryCard
            label="Market Intelligence"
            value={status?.intelligence_count ?? 0}
            sub={`Last: ${formatTimestamp(status?.intelligence_last_fetch ?? '')}`}
          />
          <SummaryCard
            label="Suggestions"
            value={status?.suggestions_count ?? 0}
            sub={`Last: ${formatTimestamp(status?.suggestions_last_fetch ?? '')}`}
          />
          <SummaryCard
            label="Pending Push"
            value={pendingCount}
            color={pendingCount > 0 ? 'var(--info)' : undefined}
            sub={awaitingReceipt > 0 ? `${awaitingReceipt} awaiting receipt` : undefined}
          />
          <SummaryCard
            label="Mapped Cards"
            value={mappedCount}
          />
        </div>
      </div>
    </div>
  );
}
