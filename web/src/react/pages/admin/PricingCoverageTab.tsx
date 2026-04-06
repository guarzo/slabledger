import type { DiagnosticCard, FailureSummary } from '../../../types/apiStatus';
import { usePricingDiagnostics } from '../../queries/useAdminQueries';
import { ProgressBar, SummaryCard } from './shared';

function pct(n: number, total: number): string {
  if (total === 0) return '0';
  return ((n / total) * 100).toFixed(1);
}

const usdFormat = new Intl.NumberFormat('en-US', { style: 'currency', currency: 'USD' });

export function PricingCoverageTab({ enabled = true }: { enabled?: boolean }) {
  const { data: diag, error, isLoading } = usePricingDiagnostics({ enabled });

  if (isLoading) return <div className="text-center text-[var(--text-muted)] py-8">Loading...</div>;
  if (error && !diag) return <div className="p-3 rounded-lg bg-[var(--danger-bg)] border border-[var(--danger-border)] text-[var(--danger)] text-sm">Failed to load pricing diagnostics</div>;
  if (!diag) return null;

  const { totalCards, fullFusionCards, partialCards, pcOnlyCards, sourceCoverage, pcOnlyCardList, discoveryFailures, recentFailures } = diag;

  return (
    <div className="space-y-6">
      {error && diag && (
        <div className="p-3 rounded-lg bg-[var(--warning-bg)] border border-[var(--warning-border)] text-[var(--warning)] text-sm">
          Failed to refresh pricing diagnostics — showing cached data
        </div>
      )}
      {/* Summary cards */}
      <div className="grid grid-cols-2 sm:grid-cols-4 gap-3">
        <SummaryCard label="Total Cards" value={totalCards} />
        <SummaryCard label="Full Fusion" value={fullFusionCards} sub={`${pct(fullFusionCards, totalCards)}%`} color="var(--success)" />
        <SummaryCard label="Partial" value={partialCards} sub={`${pct(partialCards, totalCards)}%`} color="var(--warning)" />
        <SummaryCard label="PC Only" value={pcOnlyCards} sub={`${pct(pcOnlyCards, totalCards)}%`} color="var(--danger)" />
      </div>

      {/* Source coverage */}
      <div className="space-y-2">
        <h3 className="text-sm font-medium text-[var(--text)]">Source Coverage (7-day)</h3>
        {Object.entries(sourceCoverage).sort(([,a],[,b]) => b - a).map(([source, count]) => (
          <div key={source} className="flex items-center gap-3">
            <span className="text-xs text-[var(--text-muted)] w-28 truncate">{source}</span>
            <div className="flex-1">
              <ProgressBar value={totalCards > 0 ? Math.round((count / totalCards) * 100) : 0} max={100} warningThreshold={50} dangerThreshold={20} invertColors />
            </div>
            <span className="text-xs text-[var(--text-muted)] w-16 text-right">{count} / {totalCards}</span>
          </div>
        ))}
      </div>

      {/* Discovery failures */}
      {discoveryFailures > 0 && (
        <div className="p-3 rounded-lg bg-[var(--warning-bg)] border border-[var(--warning-border)] text-[var(--warning)] text-sm">
          {discoveryFailures} discovery failure{discoveryFailures !== 1 ? 's' : ''} recorded
        </div>
      )}

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

      {/* PC-Only card list */}
      {pcOnlyCardList && pcOnlyCardList.length > 0 && (
        <div className="space-y-2">
          <h3 className="text-sm font-medium text-[var(--text)]">Cards Missing Fusion Data (top {pcOnlyCardList.length})</h3>
          <div className="glass-table max-h-[400px] overflow-y-auto scrollbar-dark">
            <table className="w-full text-sm">
              <thead className="sticky top-0 z-10">
                <tr className="glass-table-header" style={{ backdropFilter: 'blur(12px)' }}>
                  <th className="glass-table-th text-left">Card</th>
                  <th className="glass-table-th text-left hidden sm:table-cell">Set</th>
                  <th className="glass-table-th text-left hidden md:table-cell">Number</th>
                  <th className="glass-table-th text-right">Price</th>
                  <th className="glass-table-th text-left hidden lg:table-cell">Sources</th>
                </tr>
              </thead>
              <tbody>
                {pcOnlyCardList.map((c: DiagnosticCard, i: number) => (
                  <tr key={`${c.cardName}-${c.setName}-${i}`} className="glass-table-row">
                    <td className="glass-table-td font-medium">{c.cardName}</td>
                    <td className="glass-table-td hidden sm:table-cell text-[var(--text-muted)]">{c.setName}</td>
                    <td className="glass-table-td hidden md:table-cell text-[var(--text-muted)]">{c.cardNumber || '-'}</td>
                    <td className="glass-table-td text-right">{usdFormat.format(c.priceUsd)}</td>
                    <td className="glass-table-td hidden lg:table-cell text-[var(--text-muted)]">{c.sources.join(', ')}</td>
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
