import PokeballLoader from '../../PokeballLoader';
import { usePriceOverrideStats } from '../../queries/useAdminQueries';
import { currency } from '../../utils/formatters';
import { CardShell, ErrorAlert } from '../../ui';

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
          Failed to refresh stats; showing cached data.
        </div>
      )}

      {/* Override penetration leads; the two remaining counts are context */}
      <div className="rounded-xl border border-[var(--surface-2)] bg-[var(--surface-1)]/40 p-5">
        <div className="flex flex-col gap-6 sm:flex-row sm:items-start sm:gap-10">
          <div className="sm:w-56 sm:shrink-0">
            <div className="text-[10px] font-semibold uppercase tracking-wider text-[var(--text-muted)]">
              With override
            </div>
            <div className="text-4xl font-semibold tabular-nums leading-tight text-[var(--brand-400)]">
              {stats.overrideCount}
              <span className="text-xl text-[var(--text-muted)]"> / {stats.totalUnsold}</span>
            </div>
            <p className="mt-1 text-xs text-[var(--text-muted)]">
              {pct(stats.overrideCount, stats.totalUnsold)}% of unsold inventory is priced by hand
            </p>
          </div>

          <dl className="grid flex-1 grid-cols-2 gap-x-6 gap-y-4">
            <div className="min-w-0">
              <dt className="text-[10px] uppercase tracking-wider text-[var(--text-muted)]">
                Pending AI suggestions
              </dt>
              <dd
                className="text-lg font-semibold tabular-nums text-[var(--text)]"
                style={stats.pendingSuggestions > 0 ? { color: 'var(--warning)' } : undefined}
              >
                {stats.pendingSuggestions}
              </dd>
              <dd className="text-xs text-[var(--text-muted)]">
                {stats.pendingSuggestions > 0 ? 'Awaiting review' : 'None awaiting review'}
              </dd>
            </div>
            <div className="min-w-0">
              <dt className="text-[10px] uppercase tracking-wider text-[var(--text-muted)]">
                No override
              </dt>
              <dd className="text-lg font-semibold tabular-nums text-[var(--text)]">{noOverride}</dd>
              <dd className="text-xs text-[var(--text-muted)]">
                {pct(noOverride, stats.totalUnsold)}% using market price
              </dd>
            </div>
          </dl>
        </div>
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
          <div className="grid grid-cols-1 sm:grid-cols-2 gap-3 max-w-3xl">
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
          <p className="text-xs mt-1">Use the $ button on inventory cards to set overrides.</p>
        </div>
      )}
    </div>
  );
}
