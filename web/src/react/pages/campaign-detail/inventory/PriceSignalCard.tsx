import { formatCents } from '../../../utils/formatters';

interface FreshnessThresholds {
  /** Days to show green (≤ this value). */
  green: number;
  /** Days to show amber (> green, ≤ this value). Red above amber. */
  amber: number;
}

const defaultThresholds: FreshnessThresholds = { green: 3, amber: 7 };

interface PriceSignalCardProps {
  label: string;
  valueCents: number;
  highlight?: 'success' | 'warning' | 'danger' | 'muted';
  /** Optional ISO timestamp indicating when this price was last updated. */
  updatedAt?: string;
  /** Overrides the default freshness thresholds (green ≤3d, amber ≤7d). */
  freshnessThresholds?: FreshnessThresholds;
  /** Optional subtitle text shown below the price (e.g. a sale date like "Apr 10"). */
  subtitle?: string;
}

const highlightColor: Record<NonNullable<PriceSignalCardProps['highlight']>, string> = {
  success: 'text-[var(--success)]',
  warning: 'text-[var(--warning)]',
  danger: 'text-[var(--danger)]',
  muted: 'text-[var(--text-muted)]',
};

/** Returns a freshness label + color based on how many days ago a timestamp is. */
function freshnessInfo(
  isoDate: string,
  thresholds: FreshnessThresholds,
): { label: string; color: string } {
  const updated = new Date(isoDate);
  if (isNaN(updated.getTime())) return { label: '', color: '' };

  const daysAgo = Math.max(0, Math.floor((Date.now() - updated.getTime()) / (1000 * 60 * 60 * 24)));

  if (daysAgo === 0) return { label: 'today', color: 'text-[var(--success)]' };
  if (daysAgo <= thresholds.green) return { label: `${daysAgo}d ago`, color: 'text-[var(--success)]' };
  if (daysAgo <= thresholds.amber) return { label: `${daysAgo}d ago`, color: 'text-[var(--warning)]' };
  return { label: `${daysAgo}d ago`, color: 'text-[var(--danger)]' };
}

export default function PriceSignalCard({
  label,
  valueCents,
  highlight,
  updatedAt,
  freshnessThresholds = defaultThresholds,
  subtitle,
}: PriceSignalCardProps) {
  const colorClass = highlight ? highlightColor[highlight] : 'text-[var(--text)]';
  const freshness = updatedAt ? freshnessInfo(updatedAt, freshnessThresholds) : null;

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
      {subtitle && (
        <div className="text-[10px] text-[var(--text-muted)] mt-0.5">{subtitle}</div>
      )}
    </div>
  );
}
