import type { FilterCounts, SyncFilter, SyncSort } from './shopifyTypes';
import { SyncSummaryBar } from './SyncSummaryBar';
import { SyncFilterToolbar } from './SyncFilterToolbar';

interface SyncReviewPhaseProps {
  // Summary bar props
  matchedCount: number;
  unmatchedCount: number;
  noCertCount: number;
  updatedCount: number;
  totalMismatches: number;
  totalImpactCents: number;
  filterCounts: FilterCounts;
  // Toolbar props
  filter: SyncFilter;
  sort: SyncSort;
  onFilterChange: (f: SyncFilter) => void;
  onSortChange: (s: SyncSort) => void;
  onUpdateAll: () => void;
  onExport: () => void;
  // Footer and unmatched
  alignedCount: number;
  unmatched: string[];
  // Section table renders
  children: React.ReactNode;
}

export function SyncReviewPhase({
  matchedCount,
  unmatchedCount,
  noCertCount,
  updatedCount,
  totalMismatches,
  totalImpactCents,
  filterCounts,
  filter,
  sort,
  onFilterChange,
  onSortChange,
  onUpdateAll,
  onExport,
  alignedCount,
  unmatched,
  children,
}: SyncReviewPhaseProps) {
  return (
    <>
      <SyncSummaryBar
        matchedCount={matchedCount}
        unmatchedCount={unmatchedCount}
        noCertCount={noCertCount}
        updatedCount={updatedCount}
        totalMismatches={totalMismatches}
        totalImpactCents={totalImpactCents}
        filterCounts={filterCounts}
      />

      <SyncFilterToolbar
        filter={filter}
        sort={sort}
        filterCounts={filterCounts}
        updatedCount={updatedCount}
        onFilterChange={onFilterChange}
        onSortChange={onSortChange}
        onUpdateAll={onUpdateAll}
        onExport={onExport}
      />

      {children}

      {alignedCount > 0 && (
        <div className="text-center text-sm text-[var(--text-muted)] py-3 mt-2 border-t border-[var(--surface-2)]">
          {alignedCount} card{alignedCount !== 1 ? 's' : ''} already aligned — not shown
        </div>
      )}

      {unmatched.length > 0 && (
        <details className="mt-4">
          <summary className="text-sm text-[var(--text-muted)] cursor-pointer hover:text-[var(--text)]">
            {unmatched.length} unmatched cert numbers (not found in inventory)
          </summary>
          <div className="mt-2 p-3 bg-[var(--surface-1)] rounded-lg text-xs text-[var(--text-muted)]">
            {unmatched.join(', ')}
          </div>
        </details>
      )}
    </>
  );
}
