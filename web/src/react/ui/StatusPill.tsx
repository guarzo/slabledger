import { clsx } from 'clsx';
import type { ReactNode } from 'react';
import styles from './StatusPill.module.css';

export type StatusTone = 'success' | 'warning' | 'danger' | 'info' | 'brand' | 'neutral';

export function StatusPill({ tone = 'info', children, className }: {
  tone?: StatusTone; children: ReactNode; className?: string;
}) {
  return <span className={clsx(styles.pill, styles[`t-${tone}`], className)}>{children}</span>;
}
