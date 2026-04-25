import PokeballLoader from '../../PokeballLoader';
import { usePriceOverrideStats } from '../../queries/useAdminQueries';
import { currency } from '../../utils/formatters';
import { CardShell, ErrorAlert } from '../../ui';
import { SummaryCard } from './shared';

function pct(n: number, total: number): string {
  if (total === 0) return '0';
  return ((n / total) * 100).toFixed(1);
}

export function AIPricingTab({ enabled = true }: { enabled?: boolean }) {
  const { data: stats, error, isLoading } = usePriceOverrideStats({ enabled });

  if (isLoading) {
    return (
      <div className="py-8" role="status" aria-live="polite" aria-atomic="true">
        <span className="sr-only">Loading AI pricing stats…</span>
        <PokeballLoader />
      </div>
    );
  }
  if (error && !stats) return <ErrorAlert message="Failed to load price override stats" className="p-3 rounded-lg bg-[var(--danger-bg)] border border-[var(--danger-border)]" />;
  if (!stats) return null;

  const noOverride = stats.totalUnsold - stats.overrideCount;

  return (
    <div className="space-y-6">
      {error && stats && (
        <div className="p-3 rounded-lg bg-[var(--warning-bg)] border border-[var(--warning-border)] text-[var(--warning)] text-sm">
          Failed to refresh stats — showing cached data
        </div>
      )}

      {/* Top-level summary */}
      <div className="grid grid-cols-2 sm:grid-cols-4 gap-3">
        <SummaryCard label="Total Unsold" value={stats.totalUnsold} />
        <SummaryCard
          label="With Override"
          value={stats.overrideCount}
          sub={`${pct(stats.overrideCount, stats.totalUnsold)}% of inventory`}
          color="var(--brand-400)"
        />
        <SummaryCard
          label="Pending AI Suggestions"
          value={stats.pendingSuggestions}
          sub={stats.pendingSuggestions > 0 ? 'Awaiting review' : 'None'}
          color={stats.pendingSuggestions > 0 ? 'var(--warning)' : undefined}
        />
        <SummaryCard
          label="No Override"
          value={noOverride}
          sub={`${pct(noOverride, stats.totalUnsold)}% using market price`}
        />
      </div>

      {/* Override breakdown by source */}
      <div className="space-y-3">
        <h3 className="text-sm font-medium text-[var(--text)]">Override Sources</h3>
        <div className="grid grid-cols-1 sm:grid-cols-3 gap-3">
          <CardShell variant="default">
            <div className="text-xs text-[var(--text-muted)]">Manual</div>
            <div className="text-xl font-semibold text-[var(--text)]">{stats.manualCount}</div>
            <div className="text-xs text-[var(--text-muted)]">Free-form price entry</div>
          </CardShell>
          <CardShell variant="default">
            <div className="text-xs text-[var(--text-muted)]">12% Cost Markup</div>
            <div className="text-xl font-semibold text-[var(--text)]">{stats.costMarkupCount}</div>
            <div className="text-xs text-[var(--text-muted)]">Quick markup button</div>
          </CardShell>
          <CardShell variant="default">
            <div className="text-xs text-[var(--text-muted)]">AI Accepted</div>
            <div className="text-xl font-semibold" style={{ color: 'var(--ai-accent)' }}>{stats.aiAcceptedCount}</div>
            <div className="text-xs text-[var(--text-muted)]">AI suggestion accepted by user</div>
          </CardShell>
        </div>
      </div>

      {/* Value summary */}
      {(stats.overrideTotalUsd > 0 || stats.suggestionTotalUsd > 0) && (
        <div className="space-y-3">
          <h3 className="text-sm font-medium text-[var(--text)]">Value Summary</h3>
          <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
            {stats.overrideTotalUsd > 0 && (
              <CardShell variant="default">
                <div className="text-xs text-[var(--text-muted)]">Total Override Value</div>
                <div className="text-xl font-semibold text-[var(--text)]">{currency(stats.overrideTotalUsd)}</div>
                <div className="text-xs text-[var(--text-muted)]">Sum of all override prices across {stats.overrideCount} cards</div>
              </CardShell>
            )}
            {stats.suggestionTotalUsd > 0 && (
              <CardShell variant="default">
                <div className="text-xs text-[var(--text-muted)]">Pending Suggestion Value</div>
                <div className="text-xl font-semibold" style={{ color: 'var(--ai-accent)' }}>{currency(stats.suggestionTotalUsd)}</div>
                <div className="text-xs text-[var(--text-muted)]">Sum of AI-suggested prices awaiting review</div>
              </CardShell>
            )}
          </div>
        </div>
      )}

      {/* Empty state */}
      {stats.overrideCount === 0 && stats.pendingSuggestions === 0 && (
        <div className="text-center py-8 text-[var(--text-muted)]">
          <p className="text-sm">No price overrides or AI suggestions yet.</p>
          <p className="text-xs mt-1">Use the $ button on inventory cards to set overrides, or run the AI advisor for pricing suggestions.</p>
        </div>
      )}
    </div>
  );
}
