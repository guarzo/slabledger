import type { ReactNode } from 'react';

interface StickyActionBarProps {
  left: ReactNode;
  right: ReactNode;
}

/**
 * Sticky bottom toolbar with glass blur — shared between Reprice and PriceReview.
 */
export default function StickyActionBar({ left, right }: StickyActionBarProps) {
  return (
    <div
      className="sticky bottom-0 -mx-4 mt-4 px-4 py-3 bg-[var(--surface-1)]/80 backdrop-blur-xl border-t border-[var(--surface-2)] flex items-center justify-between gap-3 z-10 flex-wrap"
      role="region"
      aria-label="Actions"
    >
      {left}
      {right}
    </div>
  );
}
