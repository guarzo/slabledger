import { clsx } from 'clsx';
import { useRef, useCallback } from 'react';
import styles from './Segmented.module.css';

interface SegmentedProps<T extends string> {
  options: { value: T; label: string }[];
  value: T;
  onChange: (v: T) => void;
  ariaLabel: string;
}

export function Segmented<T extends string>({ options, value, onChange, ariaLabel }: SegmentedProps<T>) {
  const itemsRef = useRef<(HTMLButtonElement | null)[]>([]);

  const setRef = useCallback((el: HTMLButtonElement | null, i: number) => {
    itemsRef.current[i] = el;
  }, []);

  const handleKeyDown = (e: React.KeyboardEvent, index: number) => {
    let next: number | null = null;
    switch (e.key) {
      case 'ArrowRight':
      case 'ArrowDown':
        next = (index + 1) % options.length;
        break;
      case 'ArrowLeft':
      case 'ArrowUp':
        next = (index - 1 + options.length) % options.length;
        break;
      case 'Home':
        next = 0;
        break;
      case 'End':
        next = options.length - 1;
        break;
      default:
        return;
    }
    e.preventDefault();
    onChange(options[next].value);
    itemsRef.current[next]?.focus();
  };

  return (
    <div className={styles.seg} role="radiogroup" aria-label={ariaLabel}>
      {options.map((o, i) => (
        <button
          key={o.value}
          ref={(el) => setRef(el, i)}
          role="radio"
          aria-checked={value === o.value}
          tabIndex={value === o.value ? 0 : -1}
          className={clsx(styles.item, value === o.value && styles.on)}
          onClick={() => onChange(o.value)}
          onKeyDown={(e) => handleKeyDown(e, i)}
          type="button"
        >
          {o.label}
        </button>
      ))}
    </div>
  );
}
