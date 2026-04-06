import { useDHStatus, useTriggerDHBulkMatch } from '../../queries/useAdminQueries';
import { useToast } from '../../contexts/ToastContext';
import { CardShell } from '../../ui/CardShell';
import { SummaryCard } from './shared';
import Button from '../../ui/Button';
import type { DHHealthStats } from '../../../types/apiStatus';

function formatTimestamp(ts: string): string {
  if (!ts) return 'Never';
  const d = new Date(ts);
  if (isNaN(d.getTime())) return ts;
  return d.toLocaleString();
}

function formatPct(value: number): string {
  return `${(value * 100).toFixed(1)}%`;
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

function apiHealthColor(successRate: number): string {
  if (successRate >= 0.95) return 'var(--success)';
  if (successRate >= 0.80) return 'var(--warning)';
  return 'var(--danger)';
}

function buildApiHealthCard(apiHealth: DHHealthStats | undefined): { value: string; color: string | undefined; sub: string } {
  if (!apiHealth) {
    return { value: '—', color: undefined, sub: 'No data' };
  }
  const pct = `${(apiHealth.success_rate * 100).toFixed(1)}%`;
  const color = apiHealthColor(apiHealth.success_rate);
  const sub = `${apiHealth.total_calls} calls / ${apiHealth.failures} failures (7d)`;
  return { value: pct, color, sub };
}

function buildMatchRateCard(mappedCount: number, unmatchedCount: number): { value: string; sub: string } {
  const total = mappedCount + unmatchedCount;
  if (total === 0) {
    return { value: '—', sub: '0 matched / 0 unmatched' };
  }
  const rate = mappedCount / total;
  return {
    value: formatPct(rate),
    sub: `${mappedCount} matched / ${unmatchedCount} unmatched`,
  };
}

function buildUnmatchedCard(unmatchedCount: number, mappedCount: number): { value: string; color: string | undefined; sub: string } {
  const total = mappedCount + unmatchedCount;
  const pct = total > 0 ? formatPct(unmatchedCount / total) : '0%';
  return {
    value: String(unmatchedCount),
    color: unmatchedCount > 0 ? 'var(--warning)' : undefined,
    sub: `${pct} of total inventory`,
  };
}

export function DHTab({ enabled = true }: { enabled?: boolean }) {
  const { data: status, isLoading, error } = useDHStatus({ enabled });
  const bulkMatchMutation = useTriggerDHBulkMatch();
  const toast = useToast();

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
        <p className="text-red-400 text-sm">Failed to load DH status. Integration may not be configured.</p>
      </CardShell>
    );
  }

  const isRunning = status?.bulk_match_running ?? false;
  const pendingCount = status?.pending_count ?? 0;
  const mappedCount = status?.mapped_count ?? 0;
  const unmatchedCount = status?.unmatched_count ?? 0;

  const apiHealthCard = buildApiHealthCard(status?.api_health);
  const matchRateCard = buildMatchRateCard(mappedCount, unmatchedCount);
  const unmatchedCard = buildUnmatchedCard(unmatchedCount, mappedCount);

  const handleBulkMatch = async () => {
    try {
      await bulkMatchMutation.mutateAsync();
      toast.success('Bulk match started — progress will update automatically.');
    } catch {
      toast.error('Failed to start bulk match');
    }
  };

  return (
    <div className="space-y-4 mt-4">
      {/* Integration Health */}
      <div>
        <h4 className="text-xs font-semibold text-[var(--text-muted)] uppercase tracking-wide mb-2">Integration Health</h4>
        <div className="grid grid-cols-1 sm:grid-cols-3 gap-3">
          <HealthCard
            label="API Health"
            value={apiHealthCard.value}
            valueColor={apiHealthCard.color}
            sub={apiHealthCard.sub}
          />
          <HealthCard
            label="Match Rate"
            value={matchRateCard.value}
            valueColor="var(--brand-500)"
            sub={matchRateCard.sub}
          />
          <HealthCard
            label="Unmatched"
            value={unmatchedCard.value}
            valueColor={unmatchedCard.color}
            sub={unmatchedCard.sub}
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
          />
          <SummaryCard
            label="Mapped Cards"
            value={mappedCount}
          />
        </div>
      </div>

      {/* Bulk Match */}
      <CardShell padding="lg">
        <h4 className="text-sm font-semibold text-[var(--text)] mb-2">Bulk Match (Backfill)</h4>
        <p className="text-sm text-[var(--text-muted)] mb-3">
          Match unmatched inventory cards against the DH catalog. Cards with high-confidence matches will be automatically mapped.
        </p>
        <Button
          variant="secondary"
          size="sm"
          onClick={handleBulkMatch}
          loading={bulkMatchMutation.isPending}
          disabled={isRunning || bulkMatchMutation.isPending}
        >
          {isRunning ? 'Bulk Match Running...' : 'Run Bulk Match'}
        </Button>
        {isRunning && (
          <p className="mt-2 text-xs text-[var(--text-muted)]">
            Matching in progress — mapped/unmatched counts will update automatically.
          </p>
        )}
      </CardShell>
    </div>
  );
}
