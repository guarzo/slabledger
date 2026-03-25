/**
 * Section
 *
 * Card-like container with a title and children content.
 */
import type { ReactNode } from 'react';

export interface SectionProps {
  title: string;
  children: ReactNode;
  className?: string;
}

export default function Section({ title, children, className = 'mb-4' }: SectionProps) {
  return (
    <div className={`bg-[var(--surface-1)] rounded-xl border border-[var(--surface-2)] p-4 ${className}`}>
      <h3 className="text-lg font-semibold text-[var(--text)] mb-3">{title}</h3>
      {children}
    </div>
  );
}
