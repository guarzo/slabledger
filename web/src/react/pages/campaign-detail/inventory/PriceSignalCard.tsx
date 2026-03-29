import { formatCents } from '../../../utils/formatters';

interface PriceSignalCardProps {
  label: string;
  valueCents: number;
  highlight?: 'success' | 'warning' | 'danger' | 'muted';
  onClick?: () => void;
  selected?: boolean;
}

const highlightColor: Record<NonNullable<PriceSignalCardProps['highlight']>, string> = {
  success: 'text-[var(--success)]',
  warning: 'text-[var(--warning)]',
  danger: 'text-[var(--danger)]',
  muted: 'text-[var(--text-muted)]',
};

export default function PriceSignalCard({ label, valueCents, highlight, onClick, selected }: PriceSignalCardProps) {
  const colorClass = highlight ? highlightColor[highlight] : 'text-[var(--text)]';

  return (
    <div
      className={`rounded-lg bg-[var(--surface-raised)] border px-3 py-2${
        selected ? ' border-[var(--accent)] ring-1 ring-[var(--accent)]' : ' border-[var(--border)]'
      }${onClick ? ' cursor-pointer hover:border-[var(--text-muted)] transition-colors' : ''}`}
      onClick={onClick}
      role={onClick ? 'button' : undefined}
      tabIndex={onClick ? 0 : undefined}
      onKeyDown={onClick ? (e) => { if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); onClick(); } } : undefined}
    >
      <div className="text-[10px] uppercase tracking-wider text-[var(--text-muted)] mb-0.5">
        {label}
      </div>
      <div className={`text-sm font-medium tabular-nums ${colorClass}`}>
        {valueCents === 0 ? '\u2014' : formatCents(valueCents)}
      </div>
    </div>
  );
}
