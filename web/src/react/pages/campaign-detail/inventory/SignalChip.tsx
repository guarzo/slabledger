import { formatCents } from '../../../utils/formatters';

interface FreshnessThresholds {
  green: number;
  amber: number;
}

const defaultThresholds: FreshnessThresholds = { green: 3, amber: 7 };

interface SignalChipProps {
  label: string;
  valueCents: number;
  /** When true, render nothing if valueCents is 0. Cost / CL anchors pass false. */
  hideWhenZero?: boolean;
  /** Optional context delta vs. cost basis (in cents), rendered as colored % tag. */
  deltaVsCostCents?: number;
  /** ISO timestamp for freshness dot. Omitted = no dot. */
  updatedAt?: string;
  freshnessThresholds?: FreshnessThresholds;
  /** Emphasize the value with a highlight color (override / warning use case). */
  tone?: 'default' | 'warning' | 'success' | 'danger';
  /** Optional subtitle (e.g. sale date). */
  subtitle?: string;
}

function freshnessDotColor(iso: string, thresholds: FreshnessThresholds): string | null {
  const t = new Date(iso).getTime();
  if (isNaN(t)) return null;
  const daysAgo = Math.max(0, Math.floor((Date.now() - t) / 86400000));
  if (daysAgo <= thresholds.green) return 'var(--success)';
  if (daysAgo <= thresholds.amber) return 'var(--warning)';
  return 'var(--danger)';
}

const toneColor: Record<NonNullable<SignalChipProps['tone']>, string> = {
  default: 'var(--text)',
  warning: 'var(--warning)',
  success: 'var(--success)',
  danger: 'var(--danger)',
};

export default function SignalChip({
  label,
  valueCents,
  hideWhenZero = false,
  deltaVsCostCents,
  updatedAt,
  freshnessThresholds = defaultThresholds,
  tone = 'default',
  subtitle,
}: SignalChipProps) {
  if (hideWhenZero && valueCents <= 0) return null;

  const dotColor = updatedAt ? freshnessDotColor(updatedAt, freshnessThresholds) : null;
  const showDelta = deltaVsCostCents != null && valueCents > 0;
  const deltaSign = showDelta && deltaVsCostCents !== undefined
    ? (deltaVsCostCents > 0 ? '+' : deltaVsCostCents < 0 ? '' : '')
    : '';
  const deltaColor = showDelta && deltaVsCostCents !== undefined
    ? (deltaVsCostCents > 0 ? 'var(--success)' : deltaVsCostCents < 0 ? 'var(--danger)' : 'var(--text-muted)')
    : 'var(--text-muted)';

  return (
    <div className="inline-flex items-center gap-2 rounded-md border border-[rgba(255,255,255,0.06)] bg-[rgba(255,255,255,0.02)] px-2.5 py-1.5">
      {dotColor && (
        <span
          aria-hidden
          className="w-1.5 h-1.5 rounded-full shrink-0"
          style={{ background: dotColor }}
        />
      )}
      <div className="flex flex-col leading-tight min-w-0">
        <span className="text-[9px] font-semibold uppercase tracking-[0.1em] text-[var(--text-muted)]">
          {label}
        </span>
        <div className="flex items-baseline gap-1.5">
          <span
            className="text-xs font-semibold tabular-nums"
            style={{ color: valueCents > 0 ? toneColor[tone] : 'var(--text-muted)' }}
          >
            {valueCents > 0 ? formatCents(valueCents) : '\u2014'}
          </span>
          {showDelta && (
            <span className="text-[10px] tabular-nums" style={{ color: deltaColor }}>
              {deltaSign}{Math.round((deltaVsCostCents! / Math.max(1, valueCents - deltaVsCostCents!)) * 100)}%
            </span>
          )}
        </div>
        {subtitle && (
          <span className="text-[9px] text-[var(--text-muted)] mt-0.5">{subtitle}</span>
        )}
      </div>
    </div>
  );
}
