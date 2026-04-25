import { clsx } from 'clsx';
import styles from './RecommendationBadge.module.css';

export type RecTier =
  | 'MUST BUY' | 'STRONG BUY' | 'BUY'
  | 'BUY WITH CAUTION' | 'WATCH' | 'AVOID';

export type RecSeverity = 'act' | 'tune' | 'ok';

const slug: Record<RecTier, string> = {
  'MUST BUY': 'must-buy', 'STRONG BUY': 'strong-buy', 'BUY': 'buy',
  'BUY WITH CAUTION': 'buy-caution', 'WATCH': 'watch', 'AVOID': 'avoid',
};

type TierProps = { tier: RecTier; className?: string };
type SeverityProps = { label: string; severity: RecSeverity; className?: string };

export function RecommendationBadge(props: TierProps | SeverityProps) {
  if ('tier' in props) {
    const { tier, className } = props;
    return <span className={clsx(styles.rec, styles[`t-${slug[tier]}`], className)}>{tier}</span>;
  }
  const { label, severity, className } = props;
  return <span className={clsx(styles.rec, styles.soft, styles[`s-${severity}`], className)}>{label}</span>;
}
