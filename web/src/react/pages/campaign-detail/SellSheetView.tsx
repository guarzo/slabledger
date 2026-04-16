import { useMemo } from 'react';
import type { AgingItem } from '../../../types/campaigns';
import type { PriceFlagReason } from '../../../types/campaigns/priceReview';
import { Button } from '../../ui';
import RecordSaleModal from './RecordSaleModal';
import PriceHintDialog from '../../PriceHintDialog';
import PriceOverrideDialog from '../../PriceOverrideDialog';
import PriceFlagDialog from './inventory/PriceFlagDialog';
import { isReadyToList } from './inventory/inventoryCalcs';

/* ── Sell-sheet action bar (shown when items are selected) ───────── */

interface SellSheetActionsProps {
  selected: Set<string>;
  sellSheetActive: boolean;
  items: AgingItem[];
  onAddToSellSheet: (ids: string[]) => void;
  onRemoveFromSellSheet: (ids: string[]) => void;
  onRecordSale: (items: AgingItem[]) => void;
  onBulkListOnDH?: (ids: string[]) => void;
  onClearSelected: () => void;
  isPrinting: boolean;
  pageSellSheetCount: number;
  onPrint: () => void;
}

export function SellSheetActions({
  selected,
  sellSheetActive,
  items,
  onAddToSellSheet,
  onRemoveFromSellSheet,
  onRecordSale,
  onBulkListOnDH,
  onClearSelected,
  isPrinting,
  pageSellSheetCount,
  onPrint,
}: SellSheetActionsProps) {
  const listableIds = useMemo(() => {
    if (!onBulkListOnDH || selected.size === 0) return [];
    return items
      .filter(i => selected.has(i.purchase.id) && isReadyToList(i) && !!i.purchase.dhInventoryId)
      .map(i => i.purchase.id);
  }, [onBulkListOnDH, selected, items]);

  return (
    <>
      {selected.size > 0 && (
        <div className="flex items-center justify-between mb-3 sell-sheet-no-print">
          <span className="text-sm text-[var(--text-muted)]">{selected.size} selected</span>
          <div className="flex items-center gap-2">
            {sellSheetActive ? (
              <Button
                size="sm"
                variant="secondary"
                onClick={() => {
                  onRemoveFromSellSheet(Array.from(selected));
                  onClearSelected();
                }}
              >
                Remove from Sell Sheet ({selected.size})
              </Button>
            ) : (
              <Button
                size="sm"
                variant="secondary"
                onClick={() => {
                  onAddToSellSheet(Array.from(selected));
                  onClearSelected();
                }}
              >
                Add to Sell Sheet ({selected.size})
              </Button>
            )}
            {onBulkListOnDH && listableIds.length > 0 && (
              <button
                type="button"
                onClick={() => onBulkListOnDH(listableIds)}
                className="text-sm font-medium px-3 py-1.5 rounded-md bg-[var(--success)]/15 text-[var(--success)] hover:bg-[var(--success)]/25 transition-colors"
                title="Publish selected items on DH"
              >
                List on DH ({listableIds.length})
              </button>
            )}
            <Button
              size="sm"
              onClick={() => onRecordSale(items.filter(i => selected.has(i.purchase.id)))}
            >
              Record Sale ({selected.size})
            </Button>
          </div>
        </div>
      )}

      {sellSheetActive && pageSellSheetCount > 0 && (
        <div className="flex justify-end mb-3 sell-sheet-no-print">
          <Button
            size="sm"
            variant="secondary"
            disabled={isPrinting}
            onClick={onPrint}
          >
            {isPrinting ? 'Preparing…' : 'Print Sell Sheet'}
          </Button>
        </div>
      )}
    </>
  );
}

/* ── Modal dialogs for price review and sales ────────────────────── */

interface SellSheetModalsProps {
  // RecordSaleModal
  saleModalOpen: boolean;
  saleModalItems: AgingItem[];
  onSaleClose: () => void;
  onSaleSuccess: (soldIds: string[]) => void;
  // PriceHintDialog
  hintTarget: { cardName: string; setName: string; cardNumber: string } | null;
  onHintClose: () => void;
  onHintSaved: () => void;
  // PriceOverrideDialog
  priceTarget: {
    purchaseId: string;
    cardName: string;
    costBasisCents: number;
    currentPriceCents: number;
    currentOverrideCents?: number;
    currentOverrideSource?: string;
    aiSuggestedCents?: number;
  } | null;
  onPriceClose: () => void;
  onPriceSaved: () => void;
  // PriceFlagDialog
  flagTarget: { purchaseId: string; cardName: string; grade: number } | null;
  onFlagCancel: () => void;
  onFlagSubmit: (reason: PriceFlagReason) => void;
  flagSubmitting: boolean;
}

export function SellSheetModals({
  saleModalOpen,
  saleModalItems,
  onSaleClose,
  onSaleSuccess,
  hintTarget,
  onHintClose,
  onHintSaved,
  priceTarget,
  onPriceClose,
  onPriceSaved,
  flagTarget,
  onFlagCancel,
  onFlagSubmit,
  flagSubmitting,
}: SellSheetModalsProps) {
  return (
    <>
      <RecordSaleModal
        open={saleModalOpen}
        onClose={onSaleClose}
        onSuccess={() => onSaleSuccess(saleModalItems.map(i => i.purchase.id))}
        items={saleModalItems}
      />

      {hintTarget && (
        <PriceHintDialog
          cardName={hintTarget.cardName}
          setName={hintTarget.setName}
          cardNumber={hintTarget.cardNumber}
          onClose={onHintClose}
          onSaved={onHintSaved}
        />
      )}

      {priceTarget && (
        <PriceOverrideDialog
          purchaseId={priceTarget.purchaseId}
          cardName={priceTarget.cardName}
          costBasisCents={priceTarget.costBasisCents}
          currentPriceCents={priceTarget.currentPriceCents}
          currentOverrideCents={priceTarget.currentOverrideCents}
          currentOverrideSource={priceTarget.currentOverrideSource}
          aiSuggestedCents={priceTarget.aiSuggestedCents}
          onClose={onPriceClose}
          onSaved={onPriceSaved}
        />
      )}

      {flagTarget && (
        <PriceFlagDialog
          cardName={flagTarget.cardName}
          grade={flagTarget.grade}
          onSubmit={onFlagSubmit}
          onCancel={onFlagCancel}
          isSubmitting={flagSubmitting}
        />
      )}
    </>
  );
}
