import { clsx } from 'clsx';
import './GradeBadge.css';

interface GradeBadgeProps {
  grader?: string;
  /** 1–10, or 9.5 / 8.5 for half-grades. */
  grade: number;
  /** BGS "Black Label" 10 — renders in slate instead of gold. */
  blackLabel?: boolean;
  size?: 'sm' | 'md' | 'lg';
  className?: string;
}

const bucket = (g: number) => Math.min(10, Math.max(1, Math.ceil(g)));

export default function GradeBadge({
  grader = 'PSA',
  grade,
  blackLabel,
  size = 'sm',
  className,
}: GradeBadgeProps) {
  const tier = blackLabel ? 'black-label' : bucket(grade);
  const graderLabel = grader.toUpperCase();

  return (
    <span
      className={clsx(
        'grade-badge',
        `grade-badge--tier-${tier}`,
        `grade-badge--${size}`,
        className,
      )}
      aria-label={`${graderLabel} grade ${grade}${blackLabel ? ' Black Label' : ''}`}
      title={`${graderLabel} ${grade}`}
    >
      <span className="grade-badge__grader">{graderLabel}</span>
      <span className="grade-badge__grade">{grade}</span>
    </span>
  );
}
