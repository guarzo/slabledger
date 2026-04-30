import type { ReactNode } from 'react';

interface StickyActionBarProps {
  left: ReactNode;
  right: ReactNode;
}

/**
 * Sticky bottom toolbar — shared between Reprice and PriceReview.
 */
export default function StickyActionBar({ left, right }: StickyActionBarProps) {
  return (
    <div
      className="sticky bottom-0 -mx-4 mt-4 px-4 py-3 bg-[var(--surface-1)] border-t border-[var(--surface-2)] shadow-[0_-4px_12px_rgba(0,0,0,0.25)] flex items-center justify-between gap-3 z-10 flex-wrap"
      role="region"
      aria-label="Actions"
    >
      {left}
      {right}
    </div>
  );
}
