import { Button } from '../../ui';
import type { SyncFilter, SyncSort, FilterCounts } from './shopifyTypes';

interface SyncFilterToolbarProps {
  filter: SyncFilter;
  sort: SyncSort;
  filterCounts: FilterCounts;
  updatedCount: number;
  onFilterChange: (f: SyncFilter) => void;
  onSortChange: (s: SyncSort) => void;
  onUpdateAll: () => void;
  onExport: () => void;
}

export function SyncFilterToolbar({
  filter,
  sort,
  filterCounts,
  updatedCount,
  onFilterChange,
  onSortChange,
  onUpdateAll,
  onExport,
}: SyncFilterToolbarProps) {
  return (
    <div className="flex flex-wrap items-center gap-2 mb-4">
      <span className="text-[10px] font-semibold text-[var(--text-muted)] uppercase tracking-wide mr-1">Show:</span>
      {([
        ['all', `All (${filterCounts.all})`],
        ['price_drop', `Drops (${filterCounts.price_drop})`],
        ['price_increase', `Increases (${filterCounts.price_increase})`],
        ['no_market_data', `No Market (${filterCounts.no_market_data})`],
      ] as [SyncFilter, string][]).map(([key, label]) => (
        <button
          key={key}
          onClick={() => onFilterChange(key)}
          className={`text-xs px-3 py-1 rounded-md border transition-colors ${
            filter === key
              ? 'border-[var(--accent)] bg-[var(--accent)]/10 text-[var(--accent)]'
              : 'border-[var(--border)] text-[var(--text-muted)] hover:text-[var(--text)] hover:border-[var(--text-muted)]'
          }`}
        >
          {label}
        </button>
      ))}

      <select
        value={sort}
        onChange={e => onSortChange(e.target.value as SyncSort)}
        className="ml-auto text-xs px-2 py-1 rounded-md border border-[var(--border)] bg-[var(--surface-raised)] text-[var(--text-muted)] cursor-pointer"
      >
        <option value="delta">Sort: Largest Delta</option>
        <option value="value">Sort: Highest Value</option>
        <option value="margin">Sort: Most Margin</option>
        <option value="name">Sort: Card Name</option>
      </select>

      <Button size="sm" variant="success" onClick={onUpdateAll}>Update All</Button>
      <Button
        size="sm"
        variant="primary"
        disabled={updatedCount === 0}
        onClick={onExport}
      >
        Export ({updatedCount})
      </Button>
    </div>
  );
}
