import type { ReactNode } from 'react';

const toneClass: Record<string, string> = {
  danger: 'text-[var(--danger)]',
  warning: 'text-[var(--warning)]',
  success: 'text-[var(--success)]',
};

interface SectionEyebrowProps {
  tone?: 'danger' | 'warning' | 'success';
  children: ReactNode;
}

/** UPPERCASE tracking-wider section label. Single source of truth. */
export default function SectionEyebrow({ tone, children }: SectionEyebrowProps) {
  const color = (tone && toneClass[tone]) || 'text-[var(--text-muted)]';
  return (
    <div className={`text-[11px] font-bold uppercase tracking-wider ${color}`}>
      {children}
    </div>
  );
}
