import { formatCents } from '../../../utils/formatters';

interface PriceSignalCardProps {
  label: string;
  valueCents: number;
  highlight?: 'success' | 'warning' | 'danger' | 'muted';
}

const highlightColor: Record<string, string> = {
  success: 'text-[var(--success)]',
  warning: 'text-[var(--warning)]',
  danger: 'text-[var(--danger)]',
  muted: 'text-[var(--text-muted)]',
};

export default function PriceSignalCard({ label, valueCents, highlight }: PriceSignalCardProps) {
  const colorClass = highlight ? highlightColor[highlight] : 'text-[var(--text)]';

  return (
    <div className="rounded-lg bg-[var(--surface-raised)] border border-[var(--border)] px-3 py-2">
      <div className="text-[10px] uppercase tracking-wider text-[var(--text-muted)] mb-0.5">
        {label}
      </div>
      <div className={`text-sm font-medium tabular-nums ${colorClass}`}>
        {valueCents === 0 ? '\u2014' : formatCents(valueCents)}
      </div>
    </div>
  );
}
