import { useEffect } from 'react';
import { useVirtualizer } from '@tanstack/react-virtual';
import type { AgingItem } from '../../../types/campaigns';
import PokeballLoader from '../../PokeballLoader';
import { formatCents, formatPct } from '../../utils/formatters';
import { useMediaQuery } from '../../hooks/useMediaQuery';
import { EmptyState, Button } from '../../ui';
import RecordSaleModal from './RecordSaleModal';
import PriceHintDialog from '../../PriceHintDialog';
import PriceOverrideDialog from '../../PriceOverrideDialog';
import { costBasis, unrealizedPL, formatPL } from './inventory/utils';
import '../../../styles/print-sell-sheet.css';
import DesktopRow from './inventory/DesktopRow';
import MobileCard from './inventory/MobileCard';
import MobileSellSheetView from './inventory/MobileSellSheetView';
import CrackCandidatesBanner from './inventory/CrackCandidatesBanner';
import SortableHeader from './inventory/SortableHeader';
import ExpandedDetail from './inventory/ExpandedDetail';
import PriceFlagDialog from './inventory/PriceFlagDialog';
import ReviewSummaryBar from './inventory/ReviewSummaryBar';
import type { StatClickTarget } from './inventory/ReviewSummaryBar';
import { useInventoryState } from './inventory/useInventoryState';

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
    sortKey, sortDir, searchQuery, setSearchQuery,
    isPrinting, statsExpanded, setStatsExpanded,
    filterTab, setFilterTab, showAll, setShowAll, debouncedSearch,
    reviewStats, tabCounts, showEV, evPortfolio, evMap,
    pageSellSheetCount, sellSheetActive, filteredAndSortedItems,
    totalCost, totalMarket, totalPL,
    handleSort, handleReviewed, handleResolveFlag, handleApproveDHPush, handleFlagSubmit, handlePrint, handleDelete,
    toggleSelect, toggleAll, toggleExpand,
    openSaleModal, closeSaleModal, handleFixPricing, handleSetPrice,
    handlePriceSaved, handleHintSaved, sellSheet, toast,
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

  const handleStatClick = (target: StatClickTarget) => {
    setShowAll(false);
    if (target === 'flagged') {
      setFilterTab('needs_attention');
    }
  };

  if (items.length === 0) {
    return (
      <EmptyState
        icon="✅"
        title="All cards sold!"
        description="Your inventory is clear. All purchased cards have been sold."
      />
    );
  }

  return (
    <div>
      {!(isMobile && sellSheetActive) && (<>
      {/* Summary stat cards — collapsible on mobile */}
      {isMobile ? (
        <div className="mb-4 sell-sheet-no-print">
          <button
            type="button"
            onClick={() => setStatsExpanded(prev => !prev)}
            aria-expanded={statsExpanded}
            aria-controls="inventory-stats-panel"
            className="flex items-center justify-between w-full bg-[var(--surface-1)] rounded-xl border border-[var(--surface-2)] px-3 py-2.5 text-left"
          >
            <div className="flex items-center gap-3 min-w-0">
              <span className="text-xs text-[var(--text-muted)]">{items.length} cards</span>
              <span className="text-xs font-semibold text-[var(--text)]">{formatCents(totalCost)}</span>
              {totalMarket > 0 && (
                <span className={`text-xs font-semibold ${totalPL > 0 ? 'text-[var(--success)]' : totalPL < 0 ? 'text-[var(--danger)]' : 'text-[var(--text)]'}`}>
                  {formatPL(totalPL)}
                </span>
              )}
            </div>
            <svg className={`w-4 h-4 text-[var(--text-muted)] transition-transform ${statsExpanded ? 'rotate-180' : ''}`} fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2} aria-hidden="true">
              <path strokeLinecap="round" strokeLinejoin="round" d="M19 9l-7 7-7-7" />
            </svg>
          </button>
          {statsExpanded && (
            <div id="inventory-stats-panel" className="mt-3 pb-4 border-b border-[rgba(255,255,255,0.05)]">
              <div className="mb-2">
                <div className="text-[11px] font-semibold text-[var(--brand-400)] uppercase tracking-wider mb-0.5">Unrealized P/L</div>
                <div className={`text-2xl font-extrabold tracking-tight ${totalPL >= 0 ? 'text-[var(--success)]' : 'text-[var(--danger)]'}`}>
                  {totalMarket > 0 ? formatPL(totalPL) : '-'}
                </div>
                {totalMarket > 0 && totalCost > 0 && (
                  <div className={`text-xs mt-0.5 ${totalPL >= 0 ? 'text-emerald-400' : 'text-red-400'}`}>
                    {totalPL > 0 ? '+' : ''}{formatPct(totalPL / totalCost)} return
                  </div>
                )}
              </div>
              <div className="grid grid-cols-3 gap-3">
                <div>
                  <div className="text-[10px] text-[var(--text-muted)] uppercase tracking-wider">Cards</div>
                  <div className="text-sm font-semibold text-[var(--text-secondary,#cbd5e1)]">{items.length}</div>
                </div>
                <div>
                  <div className="text-[10px] text-[var(--text-muted)] uppercase tracking-wider">Cost Basis</div>
                  <div className="text-sm font-semibold text-[var(--text-secondary,#cbd5e1)]">{formatCents(totalCost)}</div>
                </div>
                <div>
                  <div className="text-[10px] text-[var(--text-muted)] uppercase tracking-wider">Market</div>
                  <div className="text-sm font-semibold text-[var(--text-secondary,#cbd5e1)]">{totalMarket > 0 ? formatCents(totalMarket) : '-'}</div>
                </div>
              </div>
            </div>
          )}
        </div>
      ) : (
        <div className="mb-7 pb-6 border-b border-[rgba(255,255,255,0.05)] sell-sheet-no-print">
          <div className="flex items-end gap-7">
            <div>
              <div className="text-[11px] font-semibold text-[var(--brand-400)] uppercase tracking-wider mb-0.5">
                Unrealized P/L
              </div>
              <div className={`text-[32px] font-extrabold tracking-tight leading-none ${totalPL >= 0 ? 'text-[var(--success)]' : 'text-[var(--danger)]'}`}>
                {totalMarket > 0 ? formatPL(totalPL) : '-'}
              </div>
              {totalMarket > 0 && totalCost > 0 && (
                <div className={`text-xs mt-1 ${totalPL >= 0 ? 'text-emerald-400' : 'text-red-400'}`}>
                  {totalPL > 0 ? '+' : ''}{formatPct(totalPL / totalCost)} return
                </div>
              )}
            </div>
            <div className="flex gap-6 pb-1">
              <div>
                <div className="text-[10px] text-[var(--text-muted)] uppercase tracking-wider">Cards</div>
                <div className="text-base font-semibold text-[var(--text-secondary,#cbd5e1)]">{items.length}</div>
              </div>
              <div>
                <div className="text-[10px] text-[var(--text-muted)] uppercase tracking-wider">Cost Basis</div>
                <div className="text-base font-semibold text-[var(--text-secondary,#cbd5e1)]">{formatCents(totalCost)}</div>
              </div>
              <div>
                <div className="text-[10px] text-[var(--text-muted)] uppercase tracking-wider">Market Value</div>
                <div className="text-base font-semibold text-[var(--text-secondary,#cbd5e1)]">{totalMarket > 0 ? formatCents(totalMarket) : '-'}</div>
              </div>
              {showEV && (
                <div>
                  <div className="text-[10px] text-[var(--text-muted)] uppercase tracking-wider">Portfolio EV</div>
                  <div className={`text-base font-semibold ${evPortfolio!.totalEvCents >= 0 ? 'text-[var(--success)]' : 'text-[var(--danger)]'}`}>
                    {formatPL(evPortfolio!.totalEvCents)}
                  </div>
                </div>
              )}
            </div>
          </div>
        </div>
      )}

      {selected.size > 0 && (
        <div className="flex items-center justify-between mb-3 sell-sheet-no-print">
          <span className="text-sm text-[var(--text-muted)]">{selected.size} selected</span>
          <div className="flex items-center gap-2">
            {sellSheetActive ? (
              <Button
                size="sm"
                variant="secondary"
                onClick={() => {
                  sellSheet.remove(Array.from(selected));
                  setSelected(new Set());
                  toast.success(`Removed ${selected.size} item${selected.size > 1 ? 's' : ''} from sell sheet`);
                }}
              >
                Remove from Sell Sheet ({selected.size})
              </Button>
            ) : (
              <Button
                size="sm"
                variant="secondary"
                onClick={() => {
                  sellSheet.add(Array.from(selected));
                  setSelected(new Set());
                  toast.success(`Added ${selected.size} item${selected.size > 1 ? 's' : ''} to sell sheet`);
                }}
              >
                Add to Sell Sheet ({selected.size})
              </Button>
            )}
            <Button
              size="sm"
              onClick={() => openSaleModal(items.filter(i => selected.has(i.purchase.id)))}
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
            onClick={handlePrint}
          >
            {isPrinting ? 'Preparing…' : 'Print Sell Sheet'}
          </Button>
        </div>
      )}

      {/* Crack Candidates Banner */}
      {campaignId && <div className="sell-sheet-no-print"><CrackCandidatesBanner campaignId={campaignId} /></div>}

      {/* Review Summary Bar */}
      <div className="mb-4 sell-sheet-no-print">
        <ReviewSummaryBar
          stats={reviewStats}
          searchQuery={searchQuery}
          onSearchChange={setSearchQuery}
          showAll={showAll}
          onToggleShowAll={() => setShowAll(prev => !prev)}
          onStatClick={handleStatClick}
        />
      </div>

      {/* Filter tabs — visible when not in showAll mode */}
      {!showAll && (
        <div className="flex items-center gap-2 mb-3 overflow-x-auto scrollbar-none sell-sheet-no-print">
          {([
            { key: 'needs_attention' as const, label: 'Needs Attention', color: 'var(--warning)' },
            { key: 'ai_suggestion' as const, label: 'AI Suggestions', color: 'var(--brand-400)' },
            { key: 'sell_sheet' as const, label: 'Sell Sheet', color: 'var(--brand-400)' },
            { key: 'all' as const, label: 'All', color: 'var(--text)' },
            { key: 'card_show' as const, label: 'Card Show', color: 'var(--brand-400)' },
          ] as const).filter(tab => {
            if (tab.key === 'ai_suggestion') return tabCounts.ai_suggestion > 0;
            return true;
          }).map(tab => {
            const count = tab.key === 'sell_sheet' ? pageSellSheetCount : tabCounts[tab.key];
            return (
              <button
                key={tab.key}
                type="button"
                onClick={() => setFilterTab(tab.key)}
                className={`shrink-0 text-xs font-medium px-3 py-1.5 rounded-full border transition-colors ${
                  filterTab === tab.key
                    ? 'border-[var(--brand-500)] bg-[var(--brand-500)]/10 text-[var(--brand-400)]'
                    : 'border-[var(--surface-2)] text-[var(--text-muted)] hover:text-[var(--text)] hover:border-[var(--text-muted)]'
                }`}
              >
                {tab.label}
                <span
                  className="ml-1.5 inline-block min-w-[18px] text-center text-[10px] font-semibold px-1 py-[1px] rounded-full"
                  style={{
                    background: filterTab === tab.key ? `color-mix(in srgb, ${tab.color} 15%, transparent)` : 'rgba(255,255,255,0.06)',
                    color: filterTab === tab.key ? tab.color : 'var(--text-muted)',
                  }}
                >
                  {count}
                </span>
              </button>
            );
          })}
        </div>
      )}

      {debouncedSearch && (
        <div className="text-xs text-[var(--text-subtle)] mb-2 pl-1 sell-sheet-no-print">
          {filteredAndSortedItems.length} of {items.length} cards
        </div>
      )}

      {sellSheetActive && filteredAndSortedItems.length === 0 && (
        <div className="text-center py-12">
          <div className="text-[var(--text-muted)] text-sm">No items on your sell sheet.</div>
          <div className="text-[var(--text-muted)] text-xs mt-1">Select items from any tab and click &ldquo;Add to Sell Sheet&rdquo;.</div>
        </div>
      )}
      </>)}

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
          <label className="flex items-center gap-2 text-xs text-[var(--text-muted)] px-1 sell-sheet-no-print">
            <input type="checkbox" checked={filteredAndSortedItems.length > 0 && filteredAndSortedItems.every(i => selected.has(i.purchase.id))}
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
                    onSetPrice={() => handleSetPrice(item)}
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
                        onSetPrice={() => handleSetPrice(item)}
                        ev={evMap.get(item.purchase.certNumber)}
                        showCampaignColumn={showCampaignColumn}
                        isOnSellSheet={!sellSheetActive && sellSheet.has(item.purchase.id)}
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
            <SortableHeader label="Market" sortKey="market" currentKey={sortKey} currentDir={sortDir} onSort={handleSort} className="text-right" style={{ width: '120px' }} />
            <SortableHeader label="P/L" sortKey="pl" currentKey={sortKey} currentDir={sortDir} onSort={handleSort} className="text-right print-hide-col" style={{ width: '72px' }} />
            <SortableHeader label="Days" sortKey="days" currentKey={sortKey} currentDir={sortDir} onSort={handleSort} className="text-center print-hide-col" style={{ width: '40px' }} />
            <div className="glass-table-th flex-shrink-0 text-center print-hide-actions" style={{ width: '48px' }}>Sell</div>
            <div className="glass-table-th flex-shrink-0 !px-1 print-hide-actions" style={{ width: '28px' }}></div>
          </div>
          {/* Rows */}
          <div ref={scrollContainerRef} className={isPrinting ? '' : 'max-h-[600px] overflow-y-auto overflow-x-hidden scrollbar-dark'}>
            {isPrinting ? (
              filteredAndSortedItems.map((item, index) => {
                const isExpanded = expandedId === item.purchase.id;
                const rowPl = unrealizedPL(costBasis(item.purchase), item.currentMarket);
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
                        onSetPrice={() => handleSetPrice(item)}
                        onDelete={() => handleDelete(item)}
                        showCampaignColumn={showCampaignColumn}
                        isOnSellSheet={!sellSheetActive && sellSheet.has(item.purchase.id)}
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
                  const rowPl = unrealizedPL(costBasis(item.purchase), item.currentMarket);
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
                          onSetPrice={() => handleSetPrice(item)}
                          onDelete={() => handleDelete(item)}
                          showCampaignColumn={showCampaignColumn}
                          isOnSellSheet={!sellSheetActive && sellSheet.has(item.purchase.id)}
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

      <RecordSaleModal
        open={saleModalOpen}
        onClose={closeSaleModal}
        onSuccess={() => setSelected(prev => {
          const next = new Set(prev);
          for (const item of saleModalItems) {
            next.delete(item.purchase.id);
          }
          return next;
        })}
        items={saleModalItems}
      />

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
    </div>
  );
}
