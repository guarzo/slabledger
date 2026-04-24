import { formatCents } from '../utils/formatters';
import styles from './MarginBadge.module.css';

export function MarginBadge({ cents }: { cents: number }) {
  const pos = cents >= 0;
  return (
    <span className={pos ? styles.pos : styles.neg}>
      {pos ? '+' : ''}{formatCents(cents)} margin
    </span>
  );
}
