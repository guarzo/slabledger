import { useMemo, useState } from 'react';
import type { CachedSetEntry } from '../../../types/apiStatus';
import { useAdminCacheStats } from '../../queries/useAdminQueries';
import { MissingCardsTab } from './MissingCardsTab';
import { ProgressBar, SummaryCard } from './shared';

const GO_ZERO_TIME = '0001-01-01T00:00:00Z';

function isPlaceholderTimestamp(value: string | undefined | null): boolean {
  return !value || value === GO_ZERO_TIME;
}

function formatDateSafe(value: string | undefined | null): string {
  if (isPlaceholderTimestamp(value)) return '-';
  const date = new Date(value!);
  if (Number.isNaN(date.getTime())) return '-';
  return date.toLocaleString();
}

function CacheBar({ finalized, total }: { finalized: number; total: number }) {
  return <ProgressBar value={finalized} max={total} warningThreshold={80} dangerThreshold={50} invertColors />;
}

export function CardDataTab() {
  const { data, error } = useAdminCacheStats();
  const [sortField, setSortField] = useState<'name' | 'totalCards' | 'status' | 'releaseDate'>('name');
  const [sortAsc, setSortAsc] = useState(true);
  const errorMessage = error instanceof Error ? error.message : error ? 'Failed to load cache stats' : null;

  const sortedSets = useMemo(() => {
    if (!data?.sets) return [];
    return [...data.sets].sort((a, b) => {
      let cmp = 0;
      if (sortField === 'name') cmp = (a.name ?? '').localeCompare(b.name ?? '');
      else if (sortField === 'totalCards') cmp = (a.totalCards ?? 0) - (b.totalCards ?? 0);
      else if (sortField === 'status') cmp = (a.status ?? '').localeCompare(b.status ?? '');
      else if (sortField === 'releaseDate') cmp = (a.releaseDate ?? '').localeCompare(b.releaseDate ?? '');
      return sortAsc ? cmp : -cmp;
    });
  }, [data?.sets, sortField, sortAsc]);

  const handleSort = (field: typeof sortField) => {
    if (sortField === field) setSortAsc(!sortAsc);
    else { setSortField(field); setSortAsc(true); }
  };

  const sortIcon = (field: typeof sortField) =>
    sortField === field ? (sortAsc ? ' \u25B2' : ' \u25BC') : '';

  if (!data) {
    return errorMessage
      ? <div className="p-3 rounded-lg bg-[var(--danger-bg)] border border-[var(--danger-border)] text-[var(--danger)] text-sm">{errorMessage}</div>
      : <div className="text-center text-[var(--text-muted)] py-8">Loading...</div>;
  }

  if (!data.enabled) {
    return <div className="text-center text-[var(--text-muted)] py-8">Persistent card cache is not enabled.</div>;
  }

  const sets = data.sets ?? [];
  const totalCards = sets.reduce((sum, s) => sum + (s.totalCards ?? 0), 0);
  const finalizedCards = sets.reduce((sum, s) => s.status === 'finalized' ? sum + (s.totalCards ?? 0) : sum, 0);

  return (
    <div className="space-y-8 mt-4">
      <section>
        <h3 className="text-base font-semibold text-[var(--text)] mb-4">Card Cache</h3>
        <div className="space-y-4">
          {/* Summary cards */}
          <div className="grid gap-4 grid-cols-2 lg:grid-cols-4">
            <SummaryCard label="Total Sets" value={data.totalSets ?? 0} />
            <SummaryCard label="Finalized" value={data.finalizedSets ?? 0} color="var(--success)" />
            <SummaryCard label="Discovered (pending)" value={data.discoveredSets ?? 0} color="var(--warning)" />
            <SummaryCard label="Total Cards" value={totalCards.toLocaleString()} />
          </div>

          {/* Progress bar */}
          <div className="space-y-1.5">
            <div className="flex justify-between text-xs text-[var(--text-muted)]">
              <span>{data.finalizedSets ?? 0} / {data.totalSets ?? 0} sets cached</span>
              <span>{finalizedCards.toLocaleString()} / {totalCards.toLocaleString()} cards</span>
            </div>
            <CacheBar finalized={data.finalizedSets ?? 0} total={data.totalSets ?? 0} />
          </div>

          {/* Sets table */}
          <div className="glass-table max-h-[min(500px,calc(100vh-400px))] overflow-y-auto scrollbar-dark">
            <table className="w-full text-sm">
              <thead className="sticky top-0 z-10">
                <tr className="glass-table-header" style={{ backdropFilter: 'blur(12px)' }}>
                  <th className="glass-table-th text-left" aria-sort={sortField === 'name' ? (sortAsc ? 'ascending' : 'descending') : 'none'}>
                    <button type="button" className="cursor-pointer select-none hover:text-[var(--text)] transition-colors" onClick={() => handleSort('name')}>
                      Set{sortIcon('name')}
                    </button>
                  </th>
                  <th className="glass-table-th text-left hidden md:table-cell" aria-sort={sortField === 'releaseDate' ? (sortAsc ? 'ascending' : 'descending') : 'none'}>
                    <button type="button" className="cursor-pointer select-none hover:text-[var(--text)] transition-colors" onClick={() => handleSort('releaseDate')}>
                      Release{sortIcon('releaseDate')}
                    </button>
                  </th>
                  <th className="glass-table-th text-right" aria-sort={sortField === 'totalCards' ? (sortAsc ? 'ascending' : 'descending') : 'none'}>
                    <button type="button" className="cursor-pointer select-none hover:text-[var(--text)] transition-colors" onClick={() => handleSort('totalCards')}>
                      Cards{sortIcon('totalCards')}
                    </button>
                  </th>
                  <th className="glass-table-th text-left" aria-sort={sortField === 'status' ? (sortAsc ? 'ascending' : 'descending') : 'none'}>
                    <button type="button" className="cursor-pointer select-none hover:text-[var(--text)] transition-colors" onClick={() => handleSort('status')}>
                      Status{sortIcon('status')}
                    </button>
                  </th>
                  <th className="glass-table-th text-left hidden lg:table-cell">
                    Fetched
                  </th>
                </tr>
              </thead>
              <tbody>
                {sortedSets.map((s: CachedSetEntry) => (
                  <tr key={s.id} className="glass-table-row">
                    <td className="glass-table-td">
                      <div className="text-[var(--text)] font-medium">{s.name}</div>
                      <div className="text-xs text-[var(--text-muted)]">{s.id}</div>
                    </td>
                    <td className="glass-table-td text-[var(--text-muted)] hidden md:table-cell">{isPlaceholderTimestamp(s.releaseDate) ? '-' : s.releaseDate}</td>
                    <td className="glass-table-td text-right text-[var(--text)] tabular-nums">{(s.totalCards ?? 0).toLocaleString()}</td>
                    <td className="glass-table-td">
                      {(() => {
                        switch (s.status) {
                          case 'finalized':
                            return <span className="px-2 py-0.5 text-xs font-medium rounded-full bg-[var(--success-bg)] text-[var(--success)]">Finalized</span>;
                          case 'discovered':
                            return <span className="px-2 py-0.5 text-xs font-medium rounded-full bg-[var(--warning-bg)] text-[var(--warning)]">Discovered</span>;
                          default:
                            return <span className="px-2 py-0.5 text-xs font-medium rounded-full bg-[var(--surface-2)] text-[var(--text-muted)]">{s.status || 'Unknown'}</span>;
                        }
                      })()}
                    </td>
                    <td className="glass-table-td text-[var(--text-muted)] text-xs hidden lg:table-cell">
                      {formatDateSafe(s.fetchedAt)}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>

          {!isPlaceholderTimestamp(data.lastUpdated) && (
            <div className="text-xs text-[var(--text-muted)] text-right">
              Registry updated: {formatDateSafe(data.lastUpdated)}
            </div>
          )}
        </div>
      </section>

      <hr className="border-[var(--surface-2)]" />

      <section>
        <h3 className="text-base font-semibold text-[var(--text)] mb-4">Missing Cards</h3>
        <MissingCardsTab />
      </section>
    </div>
  );
}
