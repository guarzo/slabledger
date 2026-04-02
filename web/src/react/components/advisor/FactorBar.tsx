import { type Factor, type DataGap, FACTOR_DISPLAY_NAMES } from '../../../types/scoring';

interface FactorBarProps {
  factor?: Factor;
  gap?: DataGap;
}

export function FactorBar({ factor, gap }: FactorBarProps) {
  if (gap) {
    return (
      <div className="flex items-center gap-2">
        <span className="text-[11px] w-[100px] text-right italic" style={{ color: 'var(--text-subtle)' }}>
          {FACTOR_DISPLAY_NAMES[gap.factor] ?? gap.factor}
        </span>
        <div className="flex-1 h-2 rounded border border-dashed" style={{ borderColor: 'var(--surface-3)' }} />
        <span className="text-[11px] w-10 italic" style={{ color: 'var(--text-subtle)' }}>n/a</span>
      </div>
    );
  }

  if (!factor) return null;

  const label = FACTOR_DISPLAY_NAMES[factor.name] ?? factor.name;
  const pct = Math.abs(factor.value) * 50;
  const isPositive = factor.value >= 0;
  const color = isPositive ? 'var(--success)' : 'var(--danger)';

  return (
    <div className="flex items-center gap-2">
      <span className="text-[11px] w-[100px] text-right" style={{ color: 'var(--text-muted)' }}>
        {label}
      </span>
      <div className="flex-1 h-2 rounded relative overflow-hidden" style={{ background: 'var(--surface-3)' }}>
        <div className="absolute top-0 h-full w-px" style={{ left: '50%', background: 'var(--surface-4)' }} />
        <div
          className="absolute top-0 h-full rounded"
          style={{
            ...(isPositive
              ? { left: '50%', width: `${pct}%` }
              : { right: '50%', width: `${pct}%` }),
            background: color,
          }}
        />
      </div>
      <span className="text-[11px] w-10 font-medium" style={{ color }}>
        {factor.value >= 0 ? '+' : ''}{factor.value.toFixed(2)}
      </span>
    </div>
  );
}
