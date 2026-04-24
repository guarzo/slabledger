import { clsx } from 'clsx';
import styles from './ConfidenceIndicator.module.css';

interface ConfidenceIndicatorProps {
  confidence: 'high' | 'medium' | 'low' | number | null;
  size?: 'sm' | 'md';
}

type Tier = 'high' | 'medium' | 'low' | 'very-low';

const TIER_META: Record<Tier, { filled: number; label: string }> = {
  'high':     { filled: 4, label: 'High confidence' },
  'medium':   { filled: 3, label: 'Medium confidence' },
  'low':      { filled: 2, label: 'Low confidence' },
  'very-low': { filled: 1, label: 'Very low confidence' },
};

function resolveTier(c: 'high' | 'medium' | 'low' | number): Tier {
  if (typeof c === 'string') return c;
  if (c >= 0.8) return 'high';
  if (c >= 0.5) return 'medium';
  if (c >= 0.3) return 'low';
  return 'very-low';
}

export function ConfidenceIndicator({ confidence, size = 'sm' }: ConfidenceIndicatorProps) {
  if (confidence == null) return null;
  const tier = resolveTier(confidence);
  const { filled, label } = TIER_META[tier];

  return (
    <span
      className={clsx(styles.wrap, styles[`s-${size}`], styles[`t-${tier}`])}
      role="img"
      title={label}
      aria-label={label}
    >
      {Array.from({ length: 4 }, (_, i) => (
        <span key={i} className={clsx(styles.dot, i < filled && styles.on)} />
      ))}
    </span>
  );
}

export default ConfidenceIndicator;
