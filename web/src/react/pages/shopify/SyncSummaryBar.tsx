import { formatCents } from '../../utils/formatters';
import type { FilterCounts } from './shopifyTypes';

interface SyncSummaryBarProps {
  matchedCount: number;
  unmatchedCount: number;
  noCertCount: number;
  updatedCount: number;
  totalImpactCents: number;
  filterCounts: FilterCounts;
}

export function SyncSummaryBar({
  matchedCount,
  unmatchedCount,
  noCertCount,
  updatedCount,
  totalImpactCents,
  filterCounts,
}: SyncSummaryBarProps) {
  return (
    <div className="flex flex-wrap items-center gap-4 mb-3 p-3 bg-[var(--surface-1)] rounded-xl border border-[var(--surface-2)]">
      <div className="text-sm">
        <span className="text-[var(--success)] font-medium">{matchedCount}</span>
        <span className="text-[var(--text-muted)]"> matched</span>
      </div>
      {unmatchedCount > 0 && (
        <div className="text-sm">
          <span className="text-[var(--warning)] font-medium">{unmatchedCount}</span>
          <span className="text-[var(--text-muted)]"> unmatched certs</span>
        </div>
      )}
      {noCertCount > 0 && (
        <div className="text-sm">
          <span className="text-[var(--text-muted)]">{noCertCount} without certs</span>
        </div>
      )}
      <div className="ml-auto flex items-center gap-4 text-sm">
        <span className="text-[var(--text-muted)]">
          {updatedCount} of {filterCounts.all} marked
        </span>
        {updatedCount > 0 && (
          <span className={`font-semibold border-l border-[var(--surface-2)] pl-4 ${
            totalImpactCents >= 0 ? 'text-[var(--success)]' : 'text-red-400'
          }`}>
            Impact: {totalImpactCents >= 0 ? '+' : ''}{formatCents(totalImpactCents)}
          </span>
        )}
      </div>
    </div>
  );
}
