export type StatCardSize = 'lg' | 'md' | 'sm';

export interface StatCardProps {
  label: string;
  value: string;
  color?: 'green' | 'red' | 'yellow';
  size?: StatCardSize;
  /** Optional secondary line below the value */
  sub?: string;
}

export default function StatCard({ label, value, color, size = 'md', sub }: StatCardProps) {
  const colorClass =
    color === 'green'
      ? 'text-[var(--success)]'
      : color === 'red'
      ? 'text-[var(--danger)]'
      : color === 'yellow'
      ? 'text-[var(--warning)]'
      : 'text-[var(--text)]';

  if (size === 'sm') {
    return (
      <div className="flex gap-2 items-baseline">
        <span className="text-xs uppercase tracking-wider text-[var(--text-muted)]">
          {label}
        </span>
        <span className={`text-sm font-semibold tabular-nums ${colorClass}`}>{value}</span>
      </div>
    );
  }

  const isLg = size === 'lg';
  const valueClass = isLg ? 'text-3xl font-extrabold' : 'text-xl font-bold';
  const padding = isLg ? 'p-5' : 'p-3';
  const border = isLg ? 'border-[var(--surface-3)]' : 'border-[var(--surface-2)]';
  const span = isLg ? 'col-span-full md:col-span-2' : '';
  const cardClass = ['bg-[var(--surface-1)]', 'rounded-xl', 'border', border, padding, 'text-center', span]
    .filter(Boolean)
    .join(' ');

  return (
    <div className={cardClass}>
      <div className="text-xs uppercase tracking-wider text-[var(--text-muted)] mb-1">
        {label}
      </div>
      <div className={`${valueClass} tabular-nums ${colorClass}`}>{value}</div>
      {sub && <div className="text-[11px] text-[var(--text-muted)] mt-0.5">{sub}</div>}
    </div>
  );
}
