import type { FailureSummary } from '../../../types/apiStatus';
import { usePricingDiagnostics } from '../../queries/useAdminQueries';
import { SummaryCard } from './shared';

export function PricingCoverageTab({ enabled = true }: { enabled?: boolean }) {
  const { data: diag, error, isLoading } = usePricingDiagnostics({ enabled });

  if (isLoading) return <div className="text-center text-[var(--text-muted)] py-8">Loading...</div>;
  if (error && !diag) return <div className="p-3 rounded-lg bg-[var(--danger-bg)] border border-[var(--danger-border)] text-[var(--danger)] text-sm">Failed to load pricing diagnostics</div>;
  if (!diag) return null;

  const { totalMappedCards, unmappedCards, recentFailures } = diag;
  const totalCards = totalMappedCards + unmappedCards;

  return (
    <div className="space-y-6">
      {error && diag && (
        <div className="p-3 rounded-lg bg-[var(--warning-bg)] border border-[var(--warning-border)] text-[var(--warning)] text-sm">
          Failed to refresh pricing diagnostics — showing cached data
        </div>
      )}

      {/* Summary cards */}
      <div className="grid grid-cols-2 sm:grid-cols-3 gap-3">
        <SummaryCard label="Inventory Cards" value={totalCards} />
        <SummaryCard label="DH Mapped" value={totalMappedCards} color="var(--success)" />
        <SummaryCard label="Unmapped" value={unmappedCards} color={unmappedCards > 0 ? 'var(--warning)' : undefined} />
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
                      {f.lastSeen ? new Date(f.lastSeen).toLocaleTimeString() : '-'}
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
