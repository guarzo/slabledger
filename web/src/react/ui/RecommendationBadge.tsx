import { clsx } from 'clsx';
import styles from './RecommendationBadge.module.css';

export type RecTier =
  | 'MUST BUY' | 'STRONG BUY' | 'BUY'
  | 'BUY WITH CAUTION' | 'WATCH' | 'AVOID';

const slug: Record<RecTier, string> = {
  'MUST BUY': 'must-buy', 'STRONG BUY': 'strong-buy', 'BUY': 'buy',
  'BUY WITH CAUTION': 'buy-caution', 'WATCH': 'watch', 'AVOID': 'avoid',
};

export function RecommendationBadge({ tier, className }: { tier: RecTier; className?: string }) {
  return <span className={clsx(styles.rec, styles[`t-${slug[tier]}`], className)}>{tier}</span>;
}
