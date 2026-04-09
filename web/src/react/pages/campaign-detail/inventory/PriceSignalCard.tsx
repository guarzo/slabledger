import { formatCents } from '../../../utils/formatters';

interface PriceSignalCardProps {
  label: string;
  valueCents: number;
  highlight?: 'success' | 'warning' | 'danger' | 'muted';
  /** Optional ISO timestamp indicating when this price was last updated. */
  updatedAt?: string;
}

const highlightColor: Record<NonNullable<PriceSignalCardProps['highlight']>, string> = {
  success: 'text-[var(--success)]',
  warning: 'text-[var(--warning)]',
  danger: 'text-[var(--danger)]',
  muted: 'text-[var(--text-muted)]',
};

/** Returns a freshness label + color based on how many days ago a timestamp is. */
function freshnessInfo(isoDate: string): { label: string; color: string } {
  const updated = new Date(isoDate);
  if (isNaN(updated.getTime())) return { label: '', color: '' };

  const daysAgo = Math.max(0, Math.floor((Date.now() - updated.getTime()) / (1000 * 60 * 60 * 24)));

  if (daysAgo === 0) return { label: 'today', color: 'text-[var(--success)]' };
  if (daysAgo <= 3) return { label: `${daysAgo}d ago`, color: 'text-[var(--success)]' };
  if (daysAgo <= 7) return { label: `${daysAgo}d ago`, color: 'text-[var(--warning)]' };
  return { label: `${daysAgo}d ago`, color: 'text-[var(--danger)]' };
}

export default function PriceSignalCard({ label, valueCents, highlight, updatedAt }: PriceSignalCardProps) {
  const colorClass = highlight ? highlightColor[highlight] : 'text-[var(--text)]';
  const freshness = updatedAt ? freshnessInfo(updatedAt) : null;

  return (
    <div className="rounded-lg bg-[var(--surface-raised)] border border-[var(--border)] px-3 py-2">
      <div className="text-[10px] uppercase tracking-wider text-[var(--text-muted)] mb-0.5 flex items-center justify-between gap-1">
        <span>{label}</span>
        {freshness && freshness.label && (
          <span className={`normal-case tracking-normal ${freshness.color}`}>{freshness.label}</span>
        )}
      </div>
      <div className={`text-sm font-medium tabular-nums ${colorClass}`}>
        {valueCents === 0 ? '\u2014' : formatCents(valueCents)}
      </div>
    </div>
  );
}
