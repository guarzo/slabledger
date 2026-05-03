import { useVirtualizer, useWindowVirtualizer } from '@tanstack/react-virtual';
import { useMemo, useRef, useLayoutEffect, useState } from 'react';
import type { AgingItem } from '../../../types/campaigns';
import type { Purchase } from '../../../types/campaigns/core';
import PokeballLoader from '../../PokeballLoader';
import { useMediaQuery } from '../../hooks/useMediaQuery';
import { EmptyState } from '../../ui';
import { costBasis, unrealizedPL } from './inventory/utils';
import { needsPriceReview } from './inventory/inventoryCalcs';
import '../../../styles/print-sell-sheet.css';
import DesktopRow from './inventory/DesktopRow';
import SellSheetPrintRow from './inventory/SellSheetPrintRow';
import { clPriceDisplayCents, dollars } from '../../utils/sellSheetHelpers';
import MobileCard from './inventory/MobileCard';
import MobileSellSheetView from './inventory/MobileSellSheetView';
import SortableHeader from './inventory/SortableHeader';
import ExpandedDetail from './inventory/ExpandedDetail';
import { useInventoryState } from './inventory/useInventoryState';
import { SellSheetModals } from './SellSheetView';
import InventoryHeader from './inventory/InventoryHeader';

export interface InventoryTabProps {
  items: AgingItem[];
  isLoading: boolean;
  campaignId?: string;
  showCampaignColumn?: boolean;
}

function sortForPrint(items: AgingItem[]): AgingItem[] {
  const score = (i: AgingItem) =>
    clPriceDisplayCents({
      clValueCents: i.purchase.clValueCents,
      recommendedPriceCents: i.recommendedPriceCents,
    })?.cents ?? 0;
  return [...items].sort((a, b) => {
    if (b.purchase.gradeValue !== a.purchase.gradeValue) {
      return b.purchase.gradeValue - a.purchase.gradeValue;
    }
    return score(b) - score(a);
  });
}

function clTotalCents(items: AgingItem[]): number {
  return items.reduce((sum, i) => {
    const cl = clPriceDisplayCents({
      clValueCents: i.purchase.clValueCents,
      recommendedPriceCents: i.recommendedPriceCents,
    });
    return sum + (cl?.cents ?? 0);
  }, 0);
}

export default function InventoryTab({ items, isLoading: loading, campaignId, showCampaignColumn }: InventoryTabProps) {
  const isMobile = useMediaQuery('(max-width: 768px)');
  const state = useInventoryState(items, campaignId);
  const {
    scrollContainerRef: _scrollContainerRef, mobileScrollRef,
    selected, setSelected, expandedId,
    saleModalOpen, saleModalItems,
    hintTarget, setHintTarget, priceTarget, setPriceTarget,
    flagTarget, setFlagTarget, flagSubmitting,
    fixMatchTarget, setFixMatchTarget,
    sortKey, sortDir, searchQuery, setSearchQuery,
    isPrinting,
    filterTab, setFilterTab, showAll, setShowAll, debouncedSearch,
    tabCounts, showEV, evPortfolio, evMap,
    pageSellSheetCount, sellSheetActive, filteredAndSortedItems,
    totalCost, totalMarket, totalPL, fullInventoryTotals,
    handleSort, handleReviewed, handleResolveFlag, handleApproveDHPush, handleListOnDH, dhListingInFlight, dhListedOptimistic, handleBulkListOnDH, handleFlagSubmit, handlePrint, handleDelete,
    toggleSelect, toggleAll, toggleExpand,
    openSaleModal, closeSaleModal, handleFixPricing, handleFixDHMatch, handleFixDHMatchSaved, handleUnmatchDH, handleRetryDHMatch, dhRetryInFlight, handleSetPrice,
    handlePriceSaved, handleHintSaved, handleInlinePriceSave, handleDismiss, handleUndismiss, sellSheet, toast,
    handleDeselectMissingCL, handleHighlightMissingCL,
    inlineSaleId, startInlineSale, cancelInlineSale, handleInlineSaleSuccess,
  } = state;

  const listParentRef = useRef<HTMLDivElement | null>(null);
  const [listScrollMargin, setListScrollMargin] = useState(0);

  // Track the list's distance from the top of the document so the window
  // virtualizer accounts for everything rendered above it (header, totals,
  // filter pills) when computing virtual row offsets.
  useLayoutEffect(() => {
    const el = listParentRef.current;
    if (!el) return;
    const update = () => {
      const rect = el.getBoundingClientRect();
      setListScrollMargin(rect.top + window.scrollY);
    };
    update();
    window.addEventListener('resize', update);
    return () => window.removeEventListener('resize', update);
  }, []);

  const rowVirtualizer = useWindowVirtualizer({
    count: filteredAndSortedItems.length,
    // Expanded rows have variable height; measureElement handles actual sizing.
    estimateSize: () => 64,
    overscan: 10,
    scrollMargin: listScrollMargin,
  });

  const mobileVirtualizer = useVirtualizer({
    count: filteredAndSortedItems.length,
    getScrollElement: () => mobileScrollRef.current,
    estimateSize: () => 140,
    overscan: 5,
  });

  const printSortedItems = useMemo(() => sortForPrint(filteredAndSortedItems), [filteredAndSortedItems]);
  const printClTotalCents = useMemo(() => clTotalCents(filteredAndSortedItems), [filteredAndSortedItems]);

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
        isMobile={isMobile}
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
        pageSellSheetCount={pageSellSheetCount}
        debouncedSearch={debouncedSearch}
        sellSheetActive={sellSheetActive}
        selected={selected}
        campaignId={campaignId}
        isPrinting={isPrinting}
        onAddToSellSheet={(ids) => { sellSheet.add(ids); toast.success(`Added ${ids.length} item${ids.length > 1 ? 's' : ''} to sell sheet`); }}
        onRemoveFromSellSheet={(ids) => { sellSheet.remove(ids); toast.success(`Removed ${ids.length} item${ids.length > 1 ? 's' : ''} from sell sheet`); }}
        onRecordSale={openSaleModal}
        onBulkListOnDH={handleBulkListOnDH}
        onClearSelected={() => setSelected(new Set())}
        onPrint={handlePrint}
        onDeselectMissingCL={handleDeselectMissingCL}
        onHighlightMissingCL={handleHighlightMissingCL}
      />

      {isPrinting && (
        <div className="sell-sheet-print">
          <div className="sell-sheet-print-header">
            <h1>Sell Sheet</h1>
            <div className="meta">
              {new Date().toLocaleDateString('en-US')} &middot; {filteredAndSortedItems.length} cards
            </div>
          </div>
          <div className="sell-sheet-print-thead">
            <div className="sell-sheet-print-headrow">
              <div className="sell-sheet-print-cell" data-cell="num">#</div>
              <div className="sell-sheet-print-cell" data-cell="card">Card</div>
              <div className="sell-sheet-print-cell" data-cell="grade">Grade</div>
              <div className="sell-sheet-print-cell" data-cell="cert">Cert</div>
              <div className="sell-sheet-print-cell" data-cell="cl">CL Price</div>
              <div className="sell-sheet-print-cell" data-cell="agreed">Agreed $</div>
            </div>
          </div>
          {printSortedItems.map((item, idx) => (
            <SellSheetPrintRow key={item.purchase.id} item={item} rowNumber={idx + 1} />
          ))}
          <div className="sell-sheet-print-footer">
            <div className="totals-row">
              <span><span className="label">Items:</span> {filteredAndSortedItems.length}</span>
              <span><span className="label">CL Total:</span> {dollars(printClTotalCents)}</span>
              <span><span className="label">Agreed:</span> <span className="blank-line" /></span>
            </div>
          </div>
        </div>
      )}

      {!isPrinting && (
      isMobile && sellSheetActive ? (
        <MobileSellSheetView
          items={filteredAndSortedItems}
          onRecordSale={(item) => openSaleModal([item])}
          onExit={() => setFilterTab('needs_attention')}
          searchQuery={searchQuery}
          onSearch={setSearchQuery}
          sellSheetCount={pageSellSheetCount}
          isPrinting={isPrinting}
          onPrint={handlePrint}
        />
      ) : isMobile ? (
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
                      isOnSellSheet={!sellSheetActive && sellSheet.has(item.purchase.id)}
                      onRemoveFromSellSheet={sellSheet.has(item.purchase.id) ? () => {
                        sellSheet.remove([item.purchase.id]);
                        toast.success('Removed from sell sheet');
                      } : undefined}
                    />
                  </div>
                );
              })}
            </div>
          </div>
        </div>
      ) : (
        <div className="glass-table">
          {/* Sticky header — sticks against the page scroll now that the inner scroll is gone */}
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
          {/* Rows — virtualizes against window scroll, no inner overflow container */}
          {filteredAndSortedItems.length === 0 && (
            <div className="py-10 text-center text-[var(--text-muted)] text-sm">
              {debouncedSearch ? `No cards match "${debouncedSearch}"` : 'No cards in this view'}
            </div>
          )}
          <div ref={listParentRef} className="overflow-x-hidden">
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
                      transform: `translateY(${virtualRow.start - rowVirtualizer.options.scrollMargin}px)`,
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
                        isOnSellSheet={!sellSheetActive && sellSheet.has(item.purchase.id)}
                        onRemoveFromSellSheet={sellSheet.has(item.purchase.id) ? () => {
                          sellSheet.remove([item.purchase.id]);
                          toast.success('Removed from sell sheet');
                        } : undefined}
                      />
                    </div>
                    {isExpanded && <ExpandedDetail item={item} onReviewed={handleReviewed} campaignId={campaignId} onOpenFlagDialog={() => setFlagTarget({ purchaseId: item.purchase.id, cardName: item.purchase.cardName, grade: item.purchase.gradeValue })} onResolveFlag={handleResolveFlag} onApproveDHPush={handleApproveDHPush} onSetPrice={() => handleSetPrice(item)} combineWithList={needsPriceReview(item)} recordingSale={inlineSaleId === item.purchase.id} onCancelInlineSale={cancelInlineSale} onInlineSaleSuccess={handleInlineSaleSuccess} />}
                  </div>
                );
              })}
            </div>
          </div>
        </div>
      ))}

      <SellSheetModals
        saleModalOpen={saleModalOpen}
        saleModalItems={saleModalItems}
        onSaleClose={closeSaleModal}
        onSaleSuccess={(soldIds) => setSelected(prev => {
          const next = new Set(prev);
          for (const id of soldIds) next.delete(id);
          return next;
        })}
        hintTarget={hintTarget}
        onHintClose={() => setHintTarget(null)}
        onHintSaved={handleHintSaved}
        priceTarget={priceTarget}
        onPriceClose={() => setPriceTarget(null)}
        onPriceSaved={handlePriceSaved}
        flagTarget={flagTarget}
        onFlagCancel={() => setFlagTarget(null)}
        onFlagSubmit={handleFlagSubmit}
        flagSubmitting={flagSubmitting}
        fixMatchTarget={fixMatchTarget}
        onFixMatchClose={() => setFixMatchTarget(null)}
        onFixMatchSaved={handleFixDHMatchSaved}
      />
    </div>
  );
}
