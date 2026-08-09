import type { FailureSummary } from '../../../types/apiStatus';
import PokeballLoader from '../../PokeballLoader';
import { usePricingDiagnostics } from '../../queries/useAdminQueries';
import { formatAdminDate } from './adminUtils';

export function PricingCoverageTab({ enabled = true }: { enabled?: boolean }) {
  const { data: diag, error, isLoading } = usePricingDiagnostics({ enabled });

  if (isLoading) {
    return (
      <div className="py-8" role="status" aria-live="polite" aria-atomic="true">
        <span className="sr-only">Loading pricing diagnostics…</span>
        <PokeballLoader />
      </div>
    );
  }
  if (error && !diag) return <div className="p-3 rounded-lg bg-[var(--danger-bg)] border border-[var(--danger-border)] text-[var(--danger)] text-sm">Failed to load pricing diagnostics</div>;
  if (!diag) return null;

  const {
    listedCards,
    readyToListCards,
    unmatchedCards,
    matchingCards,
    awaitingReceiptCards,
    clPricedCards,
    totalUnsold,
    recentFailures,
  } = diag;
  const listableCards = listedCards + readyToListCards;
  const listedRatio = listableCards > 0 ? listedCards / listableCards : null;
  const listedColor = listedRatio !== null && listedRatio >= 0.80 ? 'var(--success)' : undefined;

  return (
    <div className="space-y-6">
      {error && diag && (
        <div className="p-3 rounded-lg bg-[var(--warning-bg)] border border-[var(--warning-border)] text-[var(--warning)] text-sm">
          Failed to refresh pricing diagnostics; showing cached data.
        </div>
      )}

      {/* Coverage lead: listing progress dominates, intake states are context */}
      <div className="rounded-xl border border-[var(--surface-2)] bg-[var(--surface-1)]/40 p-5">
        <div className="flex flex-col gap-6 lg:flex-row lg:items-start lg:gap-10">
          <div className="lg:w-64 lg:shrink-0">
            <div className="text-[10px] font-semibold uppercase tracking-wider text-[var(--text-muted)]">
              Listed
            </div>
            <div
              className="text-4xl font-semibold tabular-nums leading-tight"
              style={listedColor ? { color: listedColor } : undefined}
            >
              {listedCards}
              <span className="text-xl text-[var(--text-muted)]"> / {listableCards}</span>
            </div>
            <p className="mt-1 text-xs text-[var(--text-muted)]">
              {listedRatio !== null
                ? `${(listedRatio * 100).toFixed(0)}% of listable cards are live`
                : 'Nothing listable yet'}
            </p>
            <p className="mt-3 text-xs text-[var(--text-subtle)]">
              {totalUnsold} cards in inventory · {clPricedCards} priced by Card Ladder
            </p>
          </div>

          <dl className="grid flex-1 grid-cols-2 gap-x-6 gap-y-4 sm:grid-cols-4">
            <Fact
              label="Ready to list"
              value={readyToListCards}
              tone={readyToListCards > 0 ? 'var(--warning)' : undefined}
            />
            <Fact
              label="Unmatched"
              value={unmatchedCards}
              tone={unmatchedCards > 0 ? 'var(--danger)' : undefined}
            />
            <Fact label="Matching" value={matchingCards} />
            <Fact label="Awaiting receipt" value={awaitingReceiptCards} />
          </dl>
        </div>
      </div>

      {/* Recent failure patterns */}
      {recentFailures && recentFailures.length > 0 && (
        <div className="space-y-2">
          <h3 className="text-sm font-medium text-[var(--text)]">Recent Failures (24h)</h3>
          <div className="glass-table max-h-[300px] overflow-y-auto scrollbar-dark">
            <table className="w-full text-sm">
              <thead className="sticky top-0 z-10">
                <tr className="glass-table-header" style={{ backdropFilter: 'blur(12px)' }}>
                  <th className="glass-table-th text-left">Provider</th>
                  <th className="glass-table-th text-left">Error Type</th>
                  <th className="glass-table-th text-right">Count</th>
                  <th className="glass-table-th text-left hidden sm:table-cell">Last Seen</th>
                </tr>
              </thead>
              <tbody>
                {recentFailures.map((f: FailureSummary) => (
                  <tr key={`${f.provider}-${f.errorType}`} className="glass-table-row">
                    <td className="glass-table-td">{f.provider}</td>
                    <td className="glass-table-td">
                      <span className={`inline-block px-2 py-0.5 rounded text-xs ${
                        f.errorType === 'rate_limited' ? 'bg-[var(--warning-bg)] text-[var(--warning)]' :
                        f.errorType === 'not_found' ? 'bg-[var(--surface-2)] text-[var(--text-muted)]' :
                        'bg-[var(--danger-bg)] text-[var(--danger)]'
                      }`}>
                        {f.errorType}
                      </span>
                    </td>
                    <td className="glass-table-td text-right">{f.count}</td>
                    <td className="glass-table-td hidden sm:table-cell text-[var(--text-muted)]">
                      {f.lastSeen ? formatAdminDate(f.lastSeen) : '-'}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </div>
      )}
    </div>
  );
}

function Fact({ label, value, tone }: { label: string; value: number | string; tone?: string }) {
  return (
    <div className="min-w-0">
      <dt className="text-[10px] uppercase tracking-wider text-[var(--text-muted)]">{label}</dt>
      <dd
        className="text-lg font-semibold tabular-nums text-[var(--text)]"
        style={tone ? { color: tone } : undefined}
      >
        {value}
      </dd>
    </div>
  );
}
