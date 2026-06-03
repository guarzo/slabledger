import { useEffect, useMemo } from 'react';
import type { AgingItem } from '../../../../types/campaigns';
import { formatCents } from '../../../utils/formatters';
import { useMediaQuery } from '../../../hooks/useMediaQuery';
import { Button } from '../../../ui';

export interface InventorySelectionBarProps {
  selectedItems: AgingItem[];
  onRecordSale: () => void;
  onListOnDH: () => void;
  onClear: () => void;
  disabled?: boolean;
}

export default function InventorySelectionBar({
  selectedItems,
  onRecordSale,
  onListOnDH,
  onClear,
  disabled = false,
}: InventorySelectionBarProps) {
  const isMobile = useMediaQuery('(max-width: 768px)');
  const count = selectedItems.length;
  const totalListCents = useMemo(
    () => selectedItems.reduce((sum, i) => sum + (i.purchase.clValueCents ?? 0), 0),
    [selectedItems],
  );

  useEffect(() => {
    if (count === 0 || disabled) return;
    const handleKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') onClear();
    };
    window.addEventListener('keydown', handleKey);
    return () => window.removeEventListener('keydown', handleKey);
  }, [count, disabled, onClear]);

  if (count === 0) return null;

  const containerClass = isMobile
    ? 'fixed bottom-0 inset-x-0 z-50 px-3 py-3 pb-[max(0.75rem,env(safe-area-inset-bottom))] bg-[var(--surface-2)]/95 backdrop-blur border-t border-[var(--border-subtle)]'
    : 'fixed bottom-4 left-1/2 -translate-x-1/2 z-50 px-4 py-2 rounded-full bg-[var(--surface-2)]/95 backdrop-blur border border-[var(--border-subtle)] shadow-lg';

  const layoutClass = isMobile
    ? 'flex flex-wrap items-center gap-2 justify-between'
    : 'flex items-center gap-3';

  return (
    <div role="region" aria-label="Bulk actions for selected cards" className={containerClass}>
      <div className={layoutClass}>
        <span className="text-sm text-[var(--text)] tabular-nums">
          {count} selected
          {totalListCents > 0 && <> · {formatCents(totalListCents)} list</>}
        </span>
        <div className="flex items-center gap-2">
          <Button variant="primary" size="sm" onClick={onRecordSale} disabled={disabled}>
            Record sale ({count})
          </Button>
          <Button variant="secondary" size="sm" onClick={onListOnDH} disabled={disabled}>
            List on DH ({count})
          </Button>
          <Button variant="ghost" size="sm" onClick={onClear} disabled={disabled}>
            Clear
          </Button>
        </div>
      </div>
    </div>
  );
}
