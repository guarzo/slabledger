/**
 * TrendArrow - Renders a directional arrow indicating price trend.
 *
 * up = green arrow up, down = red arrow down, stable = muted right arrow.
 */
import { clsx } from 'clsx';

interface TrendArrowProps {
  trend: 'up' | 'down' | 'stable' | null;
  size?: 'sm' | 'md';
}

const trendConfig = {
  up:     { symbol: '\u2191', color: 'var(--success)',    label: 'Trending up' },
  down:   { symbol: '\u2193', color: 'var(--error)',      label: 'Trending down' },
  stable: { symbol: '\u2192', color: 'var(--text-muted)', label: 'Stable' },
} as const;

export function TrendArrow({ trend, size = 'sm' }: TrendArrowProps) {
  if (trend == null) return null;

  const { symbol, color, label } = trendConfig[trend];

  return (
    <span
      className={clsx(
        'inline-flex items-center font-medium',
        size === 'md' ? 'text-sm' : 'text-xs',
      )}
      style={{ color }}
      role="img"
      title={label}
      aria-label={label}
    >
      {symbol}
    </span>
  );
}

export default TrendArrow;
