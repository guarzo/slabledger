import { useRef } from 'react';
import { useVirtualizer } from '@tanstack/react-virtual';
import type { AgingItem } from '../../../../types/campaigns';
import { Button } from '../../../ui';
import MobileSellSheetRow from './MobileSellSheetRow';

interface MobileSellSheetViewProps {
  items: AgingItem[];
  onRecordSale: (item: AgingItem) => void;
  onExit: () => void;
  searchQuery: string;
  onSearch: (query: string) => void;
  sellSheetCount: number;
  isPrinting: boolean;
  onPrint: () => void;
}

export default function MobileSellSheetView({
  items,
  onRecordSale,
  onExit,
  searchQuery,
  onSearch,
  sellSheetCount,
  isPrinting,
  onPrint,
}: MobileSellSheetViewProps) {
  const scrollRef = useRef<HTMLDivElement>(null);

  const virtualizer = useVirtualizer({
    count: items.length,
    getScrollElement: () => scrollRef.current,
    estimateSize: () => 36,
    overscan: 10,
  });

  return (
    <div className="sell-sheet-no-print">
      {/* Compact header */}
      <div className="flex items-center justify-between px-3 py-2.5 bg-[var(--surface-1)] border-b border-[var(--surface-2)]">
        <div className="flex items-center gap-2">
          <span className="text-sm font-bold text-[var(--text)]">Sell Sheet</span>
          <span className="text-xs text-[var(--text-muted)]">{sellSheetCount} items</span>
        </div>
        <div className="flex items-center gap-2">
          <Button size="sm" variant="secondary" disabled={isPrinting} onClick={onPrint}>
            {isPrinting ? 'Preparing\u2026' : 'Print'}
          </Button>
          <button
            type="button"
            onClick={onExit}
            className="text-xs px-2 py-1 rounded border border-[var(--surface-2)] text-[var(--text-muted)] hover:text-[var(--text)]"
          >
            Exit
          </button>
        </div>
      </div>

      {/* Search */}
      <div className="px-3 py-1.5 border-b border-[rgba(255,255,255,0.04)]">
        <input
          type="text"
          placeholder="Search sell sheet\u2026"
          aria-label="Search sell sheet"
          value={searchQuery}
          onChange={(e) => onSearch(e.target.value)}
          className="w-full bg-[var(--surface-1)] border border-[var(--surface-2)] rounded-md px-2 py-1.5 text-xs text-[var(--text)] placeholder-[var(--text-muted)] focus:outline-none focus:border-[var(--brand-500)]"
        />
      </div>

      {/* Column headers */}
      <div
        className="grid items-center px-2.5 py-1.5 text-[var(--text-muted)] border-b-2 border-[var(--surface-2)]"
        style={{
          gridTemplateColumns: '1fr 24px 52px 52px 52px 56px',
          fontSize: '9px',
          textTransform: 'uppercase',
          letterSpacing: '0.5px',
        }}
      >
        <span>Card</span>
        <span className="text-center">Gr</span>
        <span className="text-right">Cost</span>
        <span className="text-right">Mkt</span>
        <span className="text-right">CL</span>
        <span className="text-right">Rec</span>
      </div>

      {/* Scrollable rows */}
      {items.length === 0 ? (
        <div className="text-center py-12">
          <div className="text-[var(--text-muted)] text-sm">No items on your sell sheet.</div>
          <div className="text-[var(--text-muted)] text-xs mt-1">
            Select items from any tab and tap &ldquo;Add to Sell Sheet&rdquo;.
          </div>
        </div>
      ) : (
        <div
          ref={scrollRef}
          className="overflow-y-auto scrollbar-dark overscroll-contain touch-pan-y"
          style={{ maxHeight: 'calc(100dvh - 130px)' }}
        >
          <div style={{ height: `${virtualizer.getTotalSize()}px`, position: 'relative' }}>
            {virtualizer.getVirtualItems().map((virtualRow) => {
              const item = items[virtualRow.index];
              return (
                <div
                  key={item.purchase.id}
                  style={{
                    position: 'absolute',
                    top: 0,
                    left: 0,
                    width: '100%',
                    transform: `translateY(${virtualRow.start}px)`,
                  }}
                >
                  <MobileSellSheetRow
                    item={item}
                    onTap={() => onRecordSale(item)}
                  />
                </div>
              );
            })}
          </div>
        </div>
      )}
    </div>
  );
}
