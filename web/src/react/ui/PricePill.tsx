import { clsx } from 'clsx';
import styles from './PricePill.module.css';
import { formatCents } from '../utils/formatters';

interface PricePillProps {
  label: string;
  priceCents: number;
  selected?: boolean;
  recommended?: boolean;
  disabled?: boolean;
  onClick: () => void;
}

export function PricePill({ label, priceCents, selected, recommended, disabled, onClick }: PricePillProps) {
  const hasPrice = priceCents > 0;
  return (
    <button
      type="button"
      onClick={onClick}
      disabled={disabled || !hasPrice}
      aria-pressed={selected}
      className={clsx(
        styles.pill,
        selected && styles.selected,
        recommended && styles.recommended,
      )}
    >
      <span className={styles.label}>{label}</span>
      <span className={styles.price}>{hasPrice ? formatCents(priceCents) : '\u2014'}</span>
    </button>
  );
}
