import { clsx } from 'clsx';
import GradeBadge from '@/react/ui/GradeBadge';
import { RecommendationBadge, type RecTier } from '@/react/ui/RecommendationBadge';
import styles from './InventoryRow.module.css';

type Direction = 'rising' | 'falling' | 'stable';

interface InventoryRowProps {
  name: string;
  set: string;
  grader: 'PSA' | 'BGS' | 'CGC';
  grade: number;
  blackLabel?: boolean;
  deployedCents: number;
  marketCents: number;
  direction: Direction;
  marketDeltaPct: number;
  daysHeld: number;
  rec: RecTier;
  onClick?: () => void;
  ariaLabel?: string;
}

const fmt = (cents: number) =>
  `$${((cents ?? 0) / 100).toLocaleString('en-US', { minimumFractionDigits: 2 })}`;

const DIR_CLASS: Record<Direction, string> = {
  rising: styles.dirRising, falling: styles.dirFalling, stable: styles.dirStable,
};
const DIR_ARROW: Record<Direction, string> = { rising: '↗', falling: '↘', stable: '→' };

const AGE_CLASS: Record<string, string> = {
  fresh: styles.ageFresh, neutral: styles.ageNeutral,
  warning: styles.ageWarning, danger: styles.ageDanger,
};
const ageTone = (d: number) => d > 90 ? 'danger' : d > 60 ? 'warning' : d < 30 ? 'fresh' : 'neutral';

export function InventoryRow(p: InventoryRowProps) {
  const Tag = p.onClick ? 'button' : 'div';
  const days = Math.max(0, p.daysHeld ?? 0);
  const deltaPct = p.marketDeltaPct ?? 0;
  return (
    <Tag
      className={clsx(styles.row, p.onClick && styles.interactive)}
      onClick={p.onClick}
      type={p.onClick ? 'button' : undefined}
      aria-label={p.ariaLabel}
    >
      <div className={styles.gradeCol}>
        <GradeBadge grader={p.grader} grade={p.grade} blackLabel={p.blackLabel} size="md" />
      </div>
      <div className={styles.nameCol}>
        <div className={styles.name}>{p.name}</div>
        <div className={styles.set}>{p.set}</div>
      </div>
      <div className={styles.numCol}>
        <div className={styles.numLabel}>Deployed</div>
        <div className={styles.numValue}>{fmt(p.deployedCents)}</div>
      </div>
      <div className={clsx(styles.numCol, DIR_CLASS[p.direction])}>
        <div className={styles.numLabel}>Market</div>
        <div className={styles.marketRow}>
          <span className={styles.numValue}>{fmt(p.marketCents)}</span>
          <span className={styles.dirChip}>
            <span aria-hidden="true">{DIR_ARROW[p.direction]}</span>
            {' '}{deltaPct >= 0 ? '+' : ''}{deltaPct.toFixed(1)}%
          </span>
        </div>
      </div>
      <div className={clsx(styles.ageCol, AGE_CLASS[ageTone(days)])}>
        <div className={styles.numLabel}>Age</div>
        <div className={styles.numValue}>
          <span className={styles.ageBar}>
            <span style={{ width: `${Math.min(100, (days / 90) * 100)}%` }} />
          </span>
          {days}d
        </div>
      </div>
      <div className={styles.recCol}>
        <RecommendationBadge tier={p.rec} />
      </div>
    </Tag>
  );
}
