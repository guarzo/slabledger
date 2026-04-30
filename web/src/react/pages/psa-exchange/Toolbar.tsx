import { clsx } from 'clsx';
import type { Filters, QuickView } from './utils';

interface ToolbarProps {
  quickView: QuickView;
  onQuickViewChange: (v: QuickView) => void;
  filters: Filters;
  onFiltersChange: (f: Filters) => void;
  groupDuplicates: boolean;
  onGroupDuplicatesChange: (v: boolean) => void;
}

const QUICK_VIEWS: { value: QuickView; label: string; hint: string }[] = [
  { value: 'all', label: 'All', hint: 'Default ranking by score' },
  { value: 'takeAtList', label: 'Take at list', hint: 'List price ≤ our target offer' },
  { value: 'highLiquidity', label: 'High liquidity', hint: 'Velocity ≥ 5 / mo and confidence ≥ 5' },
];

const GRADE_OPTIONS = [10, 9.5, 9, 8];

export default function Toolbar({
  quickView,
  onQuickViewChange,
  filters,
  onFiltersChange,
  groupDuplicates,
  onGroupDuplicatesChange,
}: ToolbarProps) {
  const setSearch = (search: string) => onFiltersChange({ ...filters, search });
  const setMinEdge = (minEdgePct: number) => onFiltersChange({ ...filters, minEdgePct });
  const toggleGrade = (g: number) => {
    const next = filters.grades.includes(g)
      ? filters.grades.filter((x) => x !== g)
      : [...filters.grades, g];
    onFiltersChange({ ...filters, grades: next });
  };
  const setTakeAtList = (takeAtListOnly: boolean) =>
    onFiltersChange({ ...filters, takeAtListOnly });

  return (
    <div className="space-y-3">
      <div role="radiogroup" aria-label="Quick view" className="flex flex-wrap items-center gap-1.5">
        {QUICK_VIEWS.map((q) => (
          <button
            key={q.value}
            type="button"
            role="radio"
            aria-checked={quickView === q.value}
            title={q.hint}
            onClick={() => onQuickViewChange(q.value)}
            className={clsx(
              'px-3 py-1.5 rounded-full text-xs font-medium transition-colors',
              'border border-[var(--surface-2)]',
              quickView === q.value
                ? 'bg-[var(--brand-500)] text-white border-[var(--brand-500)]'
                : 'bg-[var(--surface-1)] text-[var(--text)] hover:border-[var(--brand-500)]/40',
            )}
          >
            {q.label}
          </button>
        ))}
      </div>

      <div className="flex flex-wrap items-end gap-3">
        <div className="flex-1 min-w-[14rem]">
          <label className="block text-[10px] uppercase tracking-wide text-[var(--text-muted)] mb-1">
            Search
          </label>
          <input
            type="search"
            value={filters.search}
            onChange={(e) => setSearch(e.target.value)}
            placeholder="Description or cert…"
            className="w-full px-3 py-1.5 text-sm bg-[var(--surface-2)] border border-[var(--surface-2)] rounded-md text-[var(--text)] placeholder:text-[var(--text-muted)] focus:outline-none focus:border-[var(--brand-500)] focus:ring-1 focus:ring-[var(--brand-500)]/30"
          />
        </div>

        <div>
          <label className="block text-[10px] uppercase tracking-wide text-[var(--text-muted)] mb-1">
            Grade
          </label>
          <div className="flex gap-1">
            {GRADE_OPTIONS.map((g) => {
              const on = filters.grades.includes(g);
              return (
                <button
                  key={g}
                  type="button"
                  aria-pressed={on}
                  onClick={() => toggleGrade(g)}
                  className={clsx(
                    'px-2.5 py-1 rounded-md text-xs font-medium border transition-colors tabular-nums',
                    on
                      ? 'bg-[var(--brand-500)] text-white border-[var(--brand-500)]'
                      : 'bg-[var(--surface-1)] text-[var(--text)] border-[var(--surface-2)] hover:border-[var(--brand-500)]/40',
                  )}
                >
                  {g}
                </button>
              );
            })}
          </div>
        </div>

        <div className="min-w-[12rem]">
          <label
            htmlFor="psa-exchange-min-edge"
            className="block text-[10px] uppercase tracking-wide text-[var(--text-muted)] mb-1"
          >
            Min edge {(filters.minEdgePct * 100).toFixed(0)}%
          </label>
          <input
            id="psa-exchange-min-edge"
            type="range"
            min={0}
            max={0.6}
            step={0.05}
            value={filters.minEdgePct}
            onChange={(e) => setMinEdge(Number(e.target.value))}
            className="w-full accent-[var(--brand-500)]"
          />
        </div>

        <label className="inline-flex items-center gap-2 text-xs text-[var(--text)] py-1.5 cursor-pointer">
          <input
            type="checkbox"
            checked={filters.takeAtListOnly}
            onChange={(e) => setTakeAtList(e.target.checked)}
            className="h-3.5 w-3.5 accent-[var(--brand-500)]"
          />
          Take at list only
        </label>

        <label className="inline-flex items-center gap-2 text-xs text-[var(--text)] py-1.5 cursor-pointer">
          <input
            type="checkbox"
            checked={groupDuplicates}
            onChange={(e) => onGroupDuplicatesChange(e.target.checked)}
            className="h-3.5 w-3.5 accent-[var(--brand-500)]"
          />
          Group duplicates
        </label>
      </div>
    </div>
  );
}
