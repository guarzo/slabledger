import { clsx } from 'clsx';
import { Slider } from 'radix-ui';
import styles from './ConfidenceRating.module.css';

export interface ConfidenceRatingProps {
  value: number;
  onChange: (value: number) => void;
  label?: string;
}

const TICKS = [1, 2, 3, 4, 5];

export function ConfidenceRating({ value, onChange, label }: ConfidenceRatingProps) {
  const clamped = isNaN(value) ? 1 : Math.max(1, Math.min(5, value));

  return (
    <div className={styles.wrap}>
      {label && <div className={styles.label}>{label}</div>}

      <Slider.Root
        className={styles.root}
        value={[clamped]}
        onValueChange={(values) => onChange(values[0])}
        min={1}
        max={5}
        step={1}
      >
        <Slider.Track className={styles.track}>
          <Slider.Range className={styles.range} />
        </Slider.Track>
        <Slider.Thumb className={styles.thumb} aria-label="Minimum confidence level" />
      </Slider.Root>

      <div className={styles.ticks}>
        {TICKS.map((tick) => (
          <span key={tick} className={clsx(styles.tick, tick <= clamped && styles.on)}>
            {tick}
          </span>
        ))}
      </div>

      <p className={styles.footer}>Min: {clamped} / 5</p>
    </div>
  );
}

ConfidenceRating.displayName = 'ConfidenceRating';

export default ConfidenceRating;
