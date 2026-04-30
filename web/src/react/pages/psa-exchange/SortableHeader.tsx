import { clsx } from 'clsx';
import type { SortDir, SortKey } from './utils';

interface SortableHeaderProps {
  label: string;
  sortKey: SortKey;
  currentKey: SortKey;
  currentDir: SortDir;
  onSort: (key: SortKey) => void;
  align?: 'left' | 'right';
  className?: string;
}

export default function SortableHeader({
  label,
  sortKey,
  currentKey,
  currentDir,
  onSort,
  align = 'left',
  className,
}: SortableHeaderProps) {
  const active = currentKey === sortKey;
  return (
    <th
      scope="col"
      className={clsx(
        'p-2 text-[11px] uppercase tracking-wide font-semibold',
        align === 'right' ? 'text-right' : 'text-left',
        className,
      )}
    >
      <button
        type="button"
        onClick={() => onSort(sortKey)}
        aria-label={`Sort by ${label}${active ? `, currently ${currentDir === 'asc' ? 'ascending' : 'descending'}` : ''}`}
        aria-sort={active ? (currentDir === 'asc' ? 'ascending' : 'descending') : 'none'}
        className={clsx(
          'inline-flex items-center gap-1 select-none cursor-pointer bg-transparent border-none p-0 font-inherit',
          align === 'right' ? 'flex-row-reverse' : '',
          active ? 'text-[var(--brand-400)]' : 'text-[var(--text-muted)] hover:text-[var(--brand-400)]',
        )}
      >
        {label}
        <span className="text-[8px] w-2" aria-hidden="true">
          {active ? (currentDir === 'asc' ? '▲' : '▼') : ''}
        </span>
      </button>
    </th>
  );
}
