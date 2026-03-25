import type { SortKey, SortDir } from './utils';

interface SortableHeaderProps {
  label: string;
  sortKey: SortKey;
  currentKey: SortKey;
  currentDir: SortDir;
  onSort: (key: SortKey) => void;
  className?: string;
  style?: React.CSSProperties;
}

export default function SortableHeader({ label, sortKey, currentKey, currentDir, onSort, className, style }: SortableHeaderProps) {
  const active = currentKey === sortKey;
  return (
    <div
      className={`glass-table-th flex-shrink-0 ${className ?? ''}`}
      style={style}
    >
      <button
        type="button"
        aria-label={`Sort by ${label}${active ? `, sorted ${currentDir === 'asc' ? 'ascending' : 'descending'}` : ''}`}
        className={`inline-flex items-center gap-1 cursor-pointer select-none transition-colors duration-150 bg-transparent border-none p-0 font-inherit text-inherit ${active ? 'text-[var(--brand-400)]' : 'text-[var(--text-muted)] hover:text-[var(--text)]'}`}
        onClick={() => onSort(sortKey)}
      >
        {label}
        {active && <span className="text-[9px]">{currentDir === 'asc' ? '\u25B2' : '\u25BC'}</span>}
      </button>
    </div>
  );
}
