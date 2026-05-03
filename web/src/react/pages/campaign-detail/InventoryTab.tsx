import { useVirtualizer } from '@tanstack/react-virtual';
import type { AgingItem } from '../../../types/campaigns';
import type { Purchase } from '../../../types/campaigns/core';
import PokeballLoader from '../../PokeballLoader';
import { useMediaQuery } from '../../hooks/useMediaQuery';
import { EmptyState } from '../../ui';
import { costBasis, unrealizedPL } from './inventory/utils';
import { needsPriceReview } from './inventory/inventoryCalcs';
import DesktopRow from './inventory/DesktopRow';
import MobileCard from './inventory/MobileCard';
import SortableHeader from './inventory/SortableHeader';
import ExpandedDetail from './inventory/ExpandedDetail';
import { useInventoryState } from './inventory/useInventoryState';
import RecordSaleModal from './RecordSaleModal';
import BulkRecordSaleModal from './BulkRecordSaleModal';
import PriceHintDialog from '../../PriceHintDialog';
import PriceOverrideDialog from '../../PriceOverrideDialog';
import PriceFlagDialog from './inventory/PriceFlagDialog';
import FixDHMatchDialog from './inventory/FixDHMatchDialog';
import InventoryHeader from './inventory/InventoryHeader';

export interface InventoryTabProps {
  items: AgingItem[];
  isLoading: boolean;
  campaignId?: string;
  showCampaignColumn?: boolean;
}

export default function InventoryTab({ items, isLoading: loading, campaignId, showCampaignColumn }: InventoryTabProps) {
  const isMobile = useMediaQuery('(max-width: 768px)');
  const state = useInventoryState(items, campaignId);
  const {
    scrollContainerRef, mobileScrollRef,
    selected, setSelected, expandedId,
    saleModalOpen, saleModalItems,
    hintTarget, setHintTarget, priceTarget, setPriceTarget,
    flagTarget, setFlagTarget, flagSubmitting,
    fixMatchTarget, setFixMatchTarget,
    sortKey, sortDir, searchQuery, setSearchQuery,
    filterTab, setFilterTab, showAll, setShowAll, debouncedSearch,
    tabCounts, showEV, evPortfolio, evMap,
    filteredAndSortedItems,
    totalCost, totalMarket, totalPL, fullInventoryTotals,
    handleSort, handleReviewed, handleResolveFlag, handleApproveDHPush, handleListOnDH, dhListingInFlight, dhListedOptimistic, handleFlagSubmit, handleDelete,
    toggleSelect, toggleAll, toggleExpand,
    openSaleModal, closeSaleModal, handleFixPricing, handleFixDHMatch, handleFixDHMatchSaved, handleUnmatchDH, handleRetryDHMatch, dhRetryInFlight, handleSetPrice,
    handlePriceSaved, handleHintSaved, handleInlinePriceSave, handleDismiss, handleUndismiss,
    handleDeselectMissingCL, handleHighlightMissingCL,
    inlineSaleId, startInlineSale, cancelInlineSale, handleInlineSaleSuccess,
  } = state;

  const rowVirtualizer = useVirtualizer({
    count: filteredAndSortedItems.length,
    getScrollElement: () => scrollContainerRef.current,
    // Expanded rows have variable height; measureElement handles actual sizing.
    estimateSize: () => 64,
    overscan: 10,
  });

  const mobileVirtualizer = useVirtualizer({
    count: filteredAndSortedItems.length,
    getScrollElement: () => mobileScrollRef.current,
    estimateSize: () => 140,
    overscan: 5,
  });

  if (loading) return <div className="py-8 text-center"><PokeballLoader /></div>;

  if (items.length === 0) {
    return (
      <EmptyState
        icon="✅"
        title="All cards sold!"
        description="Your inventory is clear. All purchased cards have been sold."
      />
    );
  }

  const getOnUnmatchDH = (purchase: Purchase) =>
    (purchase.dhPushStatus === 'matched' || purchase.dhPushStatus === 'manual') ? () => handleUnmatchDH(purchase) : undefined;

  const getOnRetryDHMatch = (purchase: Purchase) =>
    purchase.dhPushStatus === 'unmatched' && !dhRetryInFlight.has(purchase.id) ? () => handleRetryDHMatch(purchase) : undefined;

  return (
    <div>
      <InventoryHeader
        items={items}
        filteredCount={filteredAndSortedItems.length}
        totalCost={totalCost}
        totalMarket={totalMarket}
        totalPL={totalPL}
        fullInventoryTotals={fullInventoryTotals}
        showEV={showEV}
        evPortfolio={evPortfolio}
        searchQuery={searchQuery}
        setSearchQuery={setSearchQuery}
        showAll={showAll}
        setShowAll={setShowAll}
        filterTab={filterTab}
        setFilterTab={setFilterTab}
        tabCounts={tabCounts}
        debouncedSearch={debouncedSearch}
        selected={selected}
        campaignId={campaignId}
        onDeselectMissingCL={handleDeselectMissingCL}
        onHighlightMissingCL={handleHighlightMissingCL}
      />

      {isMobile ? (
        <div className="space-y-3">
          <label htmlFor="select-all-mobile" className="flex items-center gap-2 text-xs text-[var(--text-muted)] px-1 sell-sheet-no-print">
            <input id="select-all-mobile" type="checkbox" checked={filteredAndSortedItems.length > 0 && filteredAndSortedItems.every(i => selected.has(i.purchase.id))}
              onChange={toggleAll} className="rounded" />
            Select all
          </label>
          {filteredAndSortedItems.length === 0 && (
            <div className="py-10 text-center text-[var(--text-muted)] text-sm">
              {debouncedSearch ? `No cards match "${debouncedSearch}"` : 'No cards in this view'}
            </div>
          )}
          <div ref={mobileScrollRef} className="max-h-[calc(100vh-280px)] max-h-[calc(100dvh-280px)] overflow-y-auto scrollbar-dark overscroll-contain touch-pan-y">
            <div style={{ height: `${mobileVirtualizer.getTotalSize()}px`, position: 'relative' }}>
              {mobileVirtualizer.getVirtualItems().map(virtualRow => {
                const item = filteredAndSortedItems[virtualRow.index];
                return (
                  <div key={item.purchase.id}
                    data-index={virtualRow.index}
                    ref={mobileVirtualizer.measureElement}
                    style={{
                      position: 'absolute',
                      top: 0,
                      left: 0,
                      width: '100%',
                      transform: `translateY(${virtualRow.start}px)`,
                    }}>
                    <MobileCard
                      item={item}
                      selected={selected.has(item.purchase.id)}
                      onToggle={() => toggleSelect(item.purchase.id)}
                      onRecordSale={() => openSaleModal([item])}
                      onFixPricing={() => handleFixPricing(item.purchase)}
                      onFixDHMatch={() => handleFixDHMatch(item.purchase)}
                      onUnmatchDH={getOnUnmatchDH(item.purchase)}
                      onRetryDHMatch={getOnRetryDHMatch(item.purchase)}
                      onSetPrice={() => handleSetPrice(item)}
                      onDelete={() => handleDelete(item)}
                      onListOnDH={handleListOnDH}
                      onDismiss={() => handleDismiss(item.purchase.id)}
                      onUndismiss={() => handleUndismiss(item.purchase.id)}
                      dhListingLoading={dhListingInFlight.has(item.purchase.id)}
                      dhListedOverride={dhListedOptimistic.has(item.purchase.id)}
                      ev={evMap.get(item.purchase.certNumber)}
                      showCampaignColumn={showCampaignColumn}
                    />
                  </div>
                );
              })}
            </div>
          </div>
        </div>
      ) : (
        <div className="glass-table">
          {/* Sticky header */}
          <div className="glass-table-header flex items-center sticky top-0 z-10" style={{ paddingLeft: '3px' }}>
            <div className="glass-table-th flex-shrink-0 !px-1 print-hide-actions" style={{ width: '28px' }}>
              <input type="checkbox" aria-label="Select all visible cards" checked={filteredAndSortedItems.length > 0 && filteredAndSortedItems.every(i => selected.has(i.purchase.id))}
                onChange={toggleAll} className="rounded accent-[var(--brand-500)]" />
            </div>
            <SortableHeader label="Card" sortKey="name" currentKey={sortKey} currentDir={sortDir} onSort={handleSort} className="flex-1 min-w-0 max-w-[320px]" />
            <SortableHeader label="Gr" sortKey="grade" currentKey={sortKey} currentDir={sortDir} onSort={handleSort} className="text-center" style={{ width: '64px' }} />
            <SortableHeader label="Cost" sortKey="cost" currentKey={sortKey} currentDir={sortDir} onSort={handleSort} className="text-right" style={{ width: '100px' }} />
            <SortableHeader label="List / Rec" sortKey="market" currentKey={sortKey} currentDir={sortDir} onSort={handleSort} className="text-right" style={{ width: '180px' }} />
            <SortableHeader label="P/L" sortKey="pl" currentKey={sortKey} currentDir={sortDir} onSort={handleSort} className="text-right print-hide-col" style={{ width: '80px' }} />
            <SortableHeader label="Status" sortKey="days" currentKey={sortKey} currentDir={sortDir} onSort={handleSort} className="text-center print-hide-col" style={{ width: '120px' }} />
            <div className="glass-table-th flex-shrink-0 text-left print-hide-actions ml-2 pl-3" style={{ minWidth: '200px' }}>Actions</div>
          </div>
          {/* Rows */}
          {filteredAndSortedItems.length === 0 && (
            <div className="py-10 text-center text-[var(--text-muted)] text-sm">
              {debouncedSearch ? `No cards match "${debouncedSearch}"` : 'No cards in this view'}
            </div>
          )}
          <div ref={scrollContainerRef} className="max-h-[600px] overflow-y-auto overflow-x-hidden scrollbar-dark">
            <div style={{ height: `${rowVirtualizer.getTotalSize()}px`, position: 'relative' }}>
              {rowVirtualizer.getVirtualItems().map(virtualRow => {
                const item = filteredAndSortedItems[virtualRow.index];
                const isExpanded = expandedId === item.purchase.id;
                const rowPl = unrealizedPL(costBasis(item.purchase), item);
                const plStatus = rowPl != null ? (rowPl > 0 ? 'positive' : rowPl < 0 ? 'negative' : 'neutral') : 'neutral';
                const isSelected = selected.has(item.purchase.id);
                return (
                  <div key={item.purchase.id}
                    data-index={virtualRow.index}
                    ref={rowVirtualizer.measureElement}
                    className="glass-vrow"
                    data-stripe={virtualRow.index % 2 === 1}
                    data-selected={isSelected}
                    data-pl={plStatus}
                    style={{
                      position: 'absolute',
                      top: 0,
                      left: 0,
                      width: '100%',
                      transform: `translateY(${virtualRow.start}px)`,
                    }}>
                    <div className="text-sm">
                      <DesktopRow
                        item={item}
                        selected={isSelected}
                        onToggle={() => toggleSelect(item.purchase.id)}
                        onExpand={() => toggleExpand(item.purchase.id)}
                        onRecordSale={() => startInlineSale(item)}
                        onFixPricing={() => handleFixPricing(item.purchase)}
                        onFixDHMatch={() => handleFixDHMatch(item.purchase)}
                        onUnmatchDH={getOnUnmatchDH(item.purchase)}
                        onRetryDHMatch={getOnRetryDHMatch(item.purchase)}
                        onSetPrice={() => handleSetPrice(item)}
                        onDelete={() => handleDelete(item)}
                        onListOnDH={handleListOnDH}
                        onInlinePriceSave={handleInlinePriceSave}
                        onDismiss={() => handleDismiss(item.purchase.id)}
                        onUndismiss={() => handleUndismiss(item.purchase.id)}
                        dhListingLoading={dhListingInFlight.has(item.purchase.id)}
                        dhListedOverride={dhListedOptimistic.has(item.purchase.id)}
                        showCampaignColumn={showCampaignColumn}
                      />
                    </div>
                    {isExpanded && <ExpandedDetail item={item} onReviewed={handleReviewed} campaignId={campaignId} onOpenFlagDialog={() => setFlagTarget({ purchaseId: item.purchase.id, cardName: item.purchase.cardName, grade: item.purchase.gradeValue })} onResolveFlag={handleResolveFlag} onApproveDHPush={handleApproveDHPush} onSetPrice={() => handleSetPrice(item)} combineWithList={needsPriceReview(item)} recordingSale={inlineSaleId === item.purchase.id} onCancelInlineSale={cancelInlineSale} onInlineSaleSuccess={handleInlineSaleSuccess} />}
                  </div>
                );
              })}
            </div>
          </div>
        </div>
      )}

      {saleModalItems.length === 1 ? (
        <RecordSaleModal
          open={saleModalOpen}
          onClose={closeSaleModal}
          onSuccess={() => setSelected(prev => {
            const next = new Set(prev);
            for (const id of saleModalItems.map(i => i.purchase.id)) next.delete(id);
            return next;
          })}
          items={saleModalItems as [AgingItem]}
        />
      ) : (
        <BulkRecordSaleModal
          open={saleModalOpen}
          onClose={closeSaleModal}
          onSuccess={() => setSelected(prev => {
            const next = new Set(prev);
            for (const id of saleModalItems.map(i => i.purchase.id)) next.delete(id);
            return next;
          })}
          items={saleModalItems}
        />
      )}

      {hintTarget && (
        <PriceHintDialog
          cardName={hintTarget.cardName}
          setName={hintTarget.setName}
          cardNumber={hintTarget.cardNumber}
          onClose={() => setHintTarget(null)}
          onSaved={handleHintSaved}
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
          onClose={() => setPriceTarget(null)}
          onSaved={handlePriceSaved}
        />
      )}

      {flagTarget && (
        <PriceFlagDialog
          cardName={flagTarget.cardName}
          grade={flagTarget.grade}
          onSubmit={handleFlagSubmit}
          onCancel={() => setFlagTarget(null)}
          isSubmitting={flagSubmitting}
        />
      )}

      {fixMatchTarget && (
        <FixDHMatchDialog
          purchaseId={fixMatchTarget.purchaseId}
          cardName={fixMatchTarget.cardName}
          certNumber={fixMatchTarget.certNumber}
          currentDHCardId={fixMatchTarget.currentDHCardId}
          onClose={() => setFixMatchTarget(null)}
          onSaved={handleFixDHMatchSaved}
        />
      )}
    </div>
  );
}
