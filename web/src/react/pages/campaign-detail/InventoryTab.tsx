import { useEffect } from 'react';
import { useVirtualizer } from '@tanstack/react-virtual';
import type { AgingItem } from '../../../types/campaigns';
import type { Purchase } from '../../../types/campaigns/core';
import PokeballLoader from '../../PokeballLoader';
import { useMediaQuery } from '../../hooks/useMediaQuery';
import { EmptyState } from '../../ui';
import { costBasis, unrealizedPL } from './inventory/utils';
import '../../../styles/print-sell-sheet.css';
import DesktopRow from './inventory/DesktopRow';
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
    isPrinting, statsExpanded, setStatsExpanded,
    filterTab, setFilterTab, showAll, setShowAll, debouncedSearch,
    reviewStats, tabCounts, showEV, evPortfolio, evMap,
    pageSellSheetCount, sellSheetActive, filteredAndSortedItems,
    totalCost, totalMarket, totalPL,
    handleSort, handleReviewed, handleResolveFlag, handleApproveDHPush, handleListOnDH, dhListingInFlight, dhListedOptimistic, handleBulkListOnDH, handleFlagSubmit, handlePrint, handleDelete,
    toggleSelect, toggleAll, toggleExpand,
    openSaleModal, closeSaleModal, handleFixPricing, handleFixDHMatch, handleFixDHMatchSaved, handleUnmatchDH, handleSetPrice,
    handlePriceSaved, handleHintSaved, handleInlinePriceSave, sellSheet, toast,
  } = state;

  const rowVirtualizer = useVirtualizer({
    count: filteredAndSortedItems.length,
    getScrollElement: () => scrollContainerRef.current,
    estimateSize: (index) => {
      const item = filteredAndSortedItems[index];
      return item && expandedId === item.purchase.id ? 268 : 64;
    },
    overscan: 10,
  });

  // Force virtualizer to recalculate sizes when a row expands/collapses
  useEffect(() => {
    rowVirtualizer.measure();
  }, [expandedId, rowVirtualizer]);

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
    purchase.dhPushStatus === 'matched' ? () => handleUnmatchDH(purchase) : undefined;

  return (
    <div>
      <InventoryHeader
        isMobile={isMobile}
        items={items}
        filteredCount={filteredAndSortedItems.length}
        totalCost={totalCost}
        totalMarket={totalMarket}
        totalPL={totalPL}
        statsExpanded={statsExpanded}
        setStatsExpanded={setStatsExpanded}
        showEV={showEV}
        evPortfolio={evPortfolio}
        reviewStats={reviewStats}
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
        onStatClick={(_target) => { setShowAll(false); setFilterTab('needs_attention'); }}
        onAddToSellSheet={(ids) => { sellSheet.add(ids); toast.success(`Added ${ids.length} item${ids.length > 1 ? 's' : ''} to sell sheet`); }}
        onRemoveFromSellSheet={(ids) => { sellSheet.remove(ids); toast.success(`Removed ${ids.length} item${ids.length > 1 ? 's' : ''} from sell sheet`); }}
        onRecordSale={openSaleModal}
        onBulkListOnDH={handleBulkListOnDH}
        onClearSelected={() => setSelected(new Set())}
        onPrint={handlePrint}
      />

      {isMobile && sellSheetActive ? (
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
          <div ref={mobileScrollRef} className={isPrinting ? '' : 'max-h-[calc(100vh-280px)] max-h-[calc(100dvh-280px)] overflow-y-auto scrollbar-dark overscroll-contain touch-pan-y'}>
            {isPrinting ? (
              filteredAndSortedItems.map((item) => (
                <div key={item.purchase.id}>
                  <MobileCard
                    item={item}
                    selected={selected.has(item.purchase.id)}
                    onToggle={() => toggleSelect(item.purchase.id)}
                    onRecordSale={() => openSaleModal([item])}
                    onFixPricing={() => handleFixPricing(item.purchase)}
                    onFixDHMatch={() => handleFixDHMatch(item.purchase)}
                    onUnmatchDH={getOnUnmatchDH(item.purchase)}
                    onSetPrice={() => handleSetPrice(item)}
                    onDelete={() => handleDelete(item)}
                    onListOnDH={handleListOnDH}
                    dhListingLoading={dhListingInFlight.has(item.purchase.id)}
                    dhListedOverride={dhListedOptimistic.has(item.purchase.id)}
                    ev={evMap.get(item.purchase.certNumber)}
                    showCampaignColumn={showCampaignColumn}
                    isOnSellSheet={!sellSheetActive && sellSheet.has(item.purchase.id)}
                  />
                </div>
              ))
            ) : (
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
                        onSetPrice={() => handleSetPrice(item)}
                        onDelete={() => handleDelete(item)}
                        onListOnDH={handleListOnDH}
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
            )}
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
            <SortableHeader label="Card" sortKey="name" currentKey={sortKey} currentDir={sortDir} onSort={handleSort} className="flex-1 min-w-0" />
            <SortableHeader label="Gr" sortKey="grade" currentKey={sortKey} currentDir={sortDir} onSort={handleSort} className="text-center" style={{ width: '48px' }} />
            <SortableHeader label="Cost" sortKey="cost" currentKey={sortKey} currentDir={sortDir} onSort={handleSort} className="text-right" style={{ width: '72px' }} />
            <SortableHeader label="List / Rec" sortKey="market" currentKey={sortKey} currentDir={sortDir} onSort={handleSort} className="text-right" style={{ width: '140px' }} />
            <SortableHeader label="P/L" sortKey="pl" currentKey={sortKey} currentDir={sortDir} onSort={handleSort} className="text-right print-hide-col" style={{ width: '72px' }} />
            <SortableHeader label="Days" sortKey="days" currentKey={sortKey} currentDir={sortDir} onSort={handleSort} className="text-center print-hide-col" style={{ width: '40px' }} />
            <div className="glass-table-th flex-shrink-0 text-center print-hide-col" style={{ width: '20px' }}></div>
            <div className="glass-table-th flex-shrink-0 text-center print-hide-actions" style={{ width: '48px' }}>List</div>
            <div className="glass-table-th flex-shrink-0 text-center print-hide-actions" style={{ width: '48px' }}>Sell</div>
            <div className="glass-table-th flex-shrink-0 !px-1 print-hide-actions" style={{ width: '28px' }}></div>
          </div>
          {/* Rows */}
          <div ref={scrollContainerRef} className={isPrinting ? '' : 'max-h-[600px] overflow-y-auto overflow-x-hidden scrollbar-dark'}>
            {isPrinting ? (
              filteredAndSortedItems.map((item, index) => {
                const isExpanded = expandedId === item.purchase.id;
                const rowPl = unrealizedPL(costBasis(item.purchase), item);
                const plStatus = rowPl != null ? (rowPl > 0 ? 'positive' : rowPl < 0 ? 'negative' : 'neutral') : 'neutral';
                const isSelected = selected.has(item.purchase.id);
                return (
                  <div key={item.purchase.id} className="glass-vrow" data-stripe={index % 2 === 1} data-selected={isSelected} data-pl={plStatus}>
                    <div className="text-sm">
                    <DesktopRow
                      item={item}
                      selected={isSelected}
                      onToggle={() => toggleSelect(item.purchase.id)}
                      onExpand={() => toggleExpand(item.purchase.id)}
                      onRecordSale={() => openSaleModal([item])}
                      onFixPricing={() => handleFixPricing(item.purchase)}
                      onFixDHMatch={() => handleFixDHMatch(item.purchase)}
                      onUnmatchDH={getOnUnmatchDH(item.purchase)}
                      onSetPrice={() => handleSetPrice(item)}
                      onDelete={() => handleDelete(item)}
                      onListOnDH={handleListOnDH}
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
                    {isExpanded && <ExpandedDetail item={item} onReviewed={handleReviewed} campaignId={campaignId} onOpenFlagDialog={() => setFlagTarget({ purchaseId: item.purchase.id, cardName: item.purchase.cardName, grade: item.purchase.gradeValue })} onResolveFlag={handleResolveFlag} onApproveDHPush={handleApproveDHPush} onSetPrice={() => handleSetPrice(item)} />}
                  </div>
                );
              })
            ) : (
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
                          onRecordSale={() => openSaleModal([item])}
                          onFixPricing={() => handleFixPricing(item.purchase)}
                          onFixDHMatch={() => handleFixDHMatch(item.purchase)}
                          onUnmatchDH={getOnUnmatchDH(item.purchase)}
                          onSetPrice={() => handleSetPrice(item)}
                          onDelete={() => handleDelete(item)}
                          onListOnDH={handleListOnDH}
                          onInlinePriceSave={handleInlinePriceSave}
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
                      {isExpanded && <ExpandedDetail item={item} onReviewed={handleReviewed} campaignId={campaignId} onOpenFlagDialog={() => setFlagTarget({ purchaseId: item.purchase.id, cardName: item.purchase.cardName, grade: item.purchase.gradeValue })} onResolveFlag={handleResolveFlag} onApproveDHPush={handleApproveDHPush} onSetPrice={() => handleSetPrice(item)} />}
                    </div>
                  );
                })}
              </div>
            )}
          </div>
        </div>
      )}

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
