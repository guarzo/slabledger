import { clsx } from 'clsx';
import type { ReactNode } from 'react';
import styles from './StatusPill.module.css';

export type StatusTone = 'success' | 'warning' | 'danger' | 'info' | 'brand' | 'neutral';
export type StatusSize = 'xs' | 'sm';

export function StatusPill({ tone = 'info', size = 'sm', children, className, title }: {
  tone?: StatusTone;
  size?: StatusSize;
  children: ReactNode;
  className?: string;
  title?: string;
}) {
  return (
    <span
      className={clsx(styles.pill, styles[`t-${tone}`], styles[`s-${size}`], className)}
      title={title}
    >
      {children}
    </span>
  );
}
