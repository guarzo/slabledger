import { useEffect, useMemo, useRef, useState } from 'react';
import { useVirtualizer } from '@tanstack/react-virtual';
import { Link, useLocation, useSearchParams } from 'react-router-dom';
import { useQueryClient } from '@tanstack/react-query';
import type { ShowEvaluation } from '../../../types/showprep';
import { queryKeys } from '../../queries/queryKeys';
import { PriceReviewWorkspace } from '../price-review/PriceReviewWorkspace';
import { usePriceReviewState } from '../price-review/usePriceReviewState';
import { supportIndicator } from '../show-preparation/showPrepLabels';
import { formatCents } from '../../utils/formatters';
import { useShowReadiness } from '../../queries/useShowReadiness';
import { showPrepKeys, useShowEvaluations } from '../../queries/useShowPrepQueries';
import type { AgingItem } from '../../../types/campaigns';
import type { Purchase } from '../../../types/campaigns/core';
import PokeballLoader from '../../PokeballLoader';
import { useMediaQuery } from '../../hooks/useMediaQuery';
import { Button, EmptyState } from '../../ui';
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
import InventorySelectionBar from './inventory/InventorySelectionBar';
import { ACTIONS_COLUMN_WIDTH } from './inventory/columnWidths';
import '../show-preparation/show-preparation.css';

const EMPTY_EVALUATIONS: Record<string, ShowEvaluation> = {};

export interface InventoryTabProps {
  items: AgingItem[];
  isLoading: boolean;
  campaignId?: string;
  showCampaignColumn?: boolean;
}

export default function InventoryTab({ items, isLoading: loading, campaignId, showCampaignColumn }: InventoryTabProps) {
  const isMobile = useMediaQuery('(max-width: 768px)');
  const [params, setParams] = useSearchParams();
  const location = useLocation();
  const pricing = !campaignId && params.get('view') === 'pricing';
  const reviewId = pricing ? params.get('review') : null;
  const queryClient = useQueryClient();
  const [selectedVersions, setSelectedVersions] = useState<Record<string, string>>({});
  const [selectionBarHeight, setSelectionBarHeight] = useState(0);
  const purchaseIds = useMemo(() => items.map(item => item.purchase.id), [items]);
  const evaluationsQuery = useShowEvaluations(purchaseIds);
  const previousItems = useRef(items);
  const { refetch: recheckEvaluations } = evaluationsQuery;
  useEffect(() => {
    if (previousItems.current === items) return;
    previousItems.current = items;
    // Inventory reads may reveal sold/price/identity changes before the cached
    // show evaluation. Re-read without acknowledging captured selection intent.
    void recheckEvaluations({ cancelRefetch: false });
  }, [items, recheckEvaluations]);
  const evaluations = evaluationsQuery.data?.evaluations ?? EMPTY_EVALUATIONS;
  const state = useInventoryState(items, campaignId);
  const {
    scrollContainerRef, mobileScrollRef,
    selected, setSelected, expandedId,
    saleModalOpen, saleModalItems,
    hintTarget, setHintTarget, priceTarget, setPriceTarget,
    flagTarget, setFlagTarget, flagSubmitting,
    fixMatchTarget, setFixMatchTarget,
    sortKey, sortDir, searchQuery, setSearchQuery,
    filterTab, setFilterTab, debouncedSearch,
    priceBand, setPriceBand, priceBandCounts,
    tabCounts,
    filteredAndSortedItems,
    totalCost, totalMarket, fullInventoryTotals,
    handleSort, handleReviewed, handleResolveFlag, handleApproveDHPush, handleListOnDH, dhListingInFlight, dhListedOptimistic, handleBulkListOnDH, handleFlagSubmit, handleDelete,
    toggleSelect, toggleAll, toggleExpand,
    openSaleModal, closeSaleModal, handleFixPricing, handleFixDHMatch, handleFixDHMatchSaved, handleUnmatchDH, handleRetryDHMatch, dhRetryInFlight, handleSetPrice,
    handlePriceSaved, handleHintSaved, handleInlinePriceSave, handleDismiss, handleUndismiss,
    handleDeselectMissingCL, handleHighlightMissingCL,
    inlineSaleId, startInlineSale, cancelInlineSale, handleInlineSaleSuccess,
  } = state;

  // Keep expiry, UTC rollover and focus observation even without the old header.
  const readiness = useShowReadiness(purchaseIds, evaluations);
  const review = usePriceReviewState(items, evaluations, debouncedSearch, selected);
  const { focus: focusReview } = review;
  useEffect(() => { if (reviewId) focusReview(reviewId); }, [reviewId, focusReview]);
  const visibleItems = pricing ? review.rows : filteredAndSortedItems;
  const outsideView = [...selected].filter(id => !visibleItems.some(item => item.purchase.id === id)).length;
  const revealSelected = pricing ? review.revealSelection : state.revealSelected;

  function reviewURL(id?: string) {
    const next = new URLSearchParams(campaignId ? undefined : params);
    next.set('view', 'pricing');
    if (id) next.set('review', id); else next.delete('review');
    return next;
  }
  function navigateReview(id: string) { focusReview(id); setParams(reviewURL(id)); }
  async function recheckInventory() {
    try {
      // Join the post-save read if it is still running. Never replay the PATCH.
      await queryClient.refetchQueries({ queryKey: campaignId ? queryKeys.campaigns.inventory(campaignId) : queryKeys.portfolio.globalInventory },
        { cancelRefetch: false, throwOnError: true });
      await queryClient.invalidateQueries({ queryKey: showPrepKeys.all }, { cancelRefetch: false });
      return true;
    } catch { return false; }
  }

  const selectedItems = useMemo(
    () => items.filter(i => selected.has(i.purchase.id)),
    [items, selected],
  );

  // Capture only explicit selection intent. A hidden card's background recheck
  // must not advance the version that adding it would acknowledge.
  function captureSelection(ids: string[], adding: boolean) {
    setSelectedVersions(prev => {
      const next = { ...prev };
      for (const id of ids) {
        if (!adding) delete next[id];
        else if (!selected.has(id)) next[id] = evaluations[id]?.version ?? '';
      }
      return next;
    });
  }
  function toggleCard(id: string) {
    captureSelection([id], !selected.has(id));
    toggleSelect(id);
  }
  function toggleVisible() {
    const ids = visibleItems.map(item => item.purchase.id);
    captureSelection(ids, !ids.every(id => selected.has(id)));
    toggleAll(ids);
  }

  // Keep in sync with the conditional modal renders below — every overlay
  // that traps focus or occludes the selection bar belongs here.
  const anyModalOpen =
    saleModalOpen ||
    hintTarget != null ||
    priceTarget != null ||
    flagTarget != null ||
    fixMatchTarget != null;

  // Preserve unchanged stable-key heights on collapse. ResizeObserver measures
  // changed/resized rows; clearing all sizes would restore unmeasured estimates.
  const rowVirtualizer = useVirtualizer({
    count: filteredAndSortedItems.length,
    getScrollElement: () => scrollContainerRef.current,
    // Expanded rows have variable height; measureElement handles actual sizing.
    estimateSize: () => 64,
    overscan: 10,
    // Cache sizes by stable purchase id so filter/sort/expand toggles don't
    // reuse a stale size from a different row at the same index.
    getItemKey: (index) => filteredAndSortedItems[index]?.purchase.id ?? index,
  });

  const mobileVirtualizer = useVirtualizer({
    count: filteredAndSortedItems.length,
    getScrollElement: () => mobileScrollRef.current,
    estimateSize: () => 140,
    overscan: 5,
    getItemKey: (index) => filteredAndSortedItems[index]?.purchase.id ?? index,
  });

  // Mobile uses a sale modal, so the hidden desktop inline intent is abandoned.
  useEffect(() => { if (isMobile) cancelInlineSale(); }, [isMobile, cancelInlineSale]);

  if (loading) return <div className="py-8 text-center"><PokeballLoader /></div>;

  if (items.length === 0 && !pricing) {
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

  const evidenceButton = (item: AgingItem) => {
    const e = evaluations[item.purchase.id];
    const indicator = e ? supportIndicator(e) : { label: evaluationsQuery.isFetching ? 'Loading price support…' : 'Evaluation unavailable', tone: 'muted' };
    return <Link to={`/inventory?${reviewURL(item.purchase.id)}`} className={`show-evidence-trigger show-tone-${indicator.tone}`}
      aria-label={`Review price ${item.purchase.certNumber}: ${indicator.label}`}
      title={`SlabLedger asking ${e && e.localPriceCents > 0 ? formatCents(e.localPriceCents) : 'not set'}`}
      onClick={event => event.stopPropagation()}><strong>{indicator.label}</strong><span aria-hidden="true">→</span></Link>;
  };
  const unavailableEvaluations = Object.keys(evaluationsQuery.data?.errors ?? {}).length + evaluationsQuery.unresolvedCount;
  const emptyMatches = <div className="show-empty-matches">
    <p>{debouncedSearch ? `No cards match "${debouncedSearch}"` : 'No cards in this view'}</p>
  </div>;

  return (
    <div className="show-inventory">
      {!campaignId && <div role="group" aria-label="Inventory view" className="flex gap-2 mb-4">
        <Button variant={!pricing ? 'secondary' : 'ghost'} size="sm" aria-pressed={!pricing} onClick={() => {
          const next = new URLSearchParams(params); next.delete('view'); next.delete('review'); setParams(next);
        }}>Inventory</Button>
        <Button variant={pricing ? 'secondary' : 'ghost'} size="sm" aria-pressed={pricing}
          onClick={() => setParams(reviewURL())}>Price review</Button>
      </div>}
      <InventoryHeader
        items={items}
        filteredCount={filteredAndSortedItems.length}
        totalCost={totalCost}
        totalMarket={totalMarket}
        fullInventoryTotals={fullInventoryTotals}
        searchQuery={searchQuery}
        setSearchQuery={setSearchQuery}
        filterTab={filterTab}
        setFilterTab={setFilterTab}
        tabCounts={tabCounts}
        priceBand={priceBand}
        setPriceBand={setPriceBand}
        priceBandCounts={priceBandCounts}
        debouncedSearch={debouncedSearch}
        selected={selected}
        pricing={pricing}
        onDeselectMissingCL={handleDeselectMissingCL}
        onHighlightMissingCL={handleHighlightMissingCL}
      />

      {!evaluationsQuery.isFetching && (unavailableEvaluations > 0 || readiness.observationError) && <div role="alert" className="text-sm text-[var(--warning)] mb-3">
        {unavailableEvaluations > 0 ? `${unavailableEvaluations} evaluations unavailable. ` : readiness.observationError}
        <button className="show-link" disabled={readiness.observing} onClick={() => {
          if (readiness.observationError) void readiness.retryObservation(); else void evaluationsQuery.refetch();
        }}>Retry evaluation</button>
      </div>}

      {pricing ? <>
        <label className="show-check flex items-center gap-2 text-xs text-[var(--text-muted)] mb-3">
          <input type="checkbox" aria-label="Select all visible cards" checked={visibleItems.length > 0 && visibleItems.every(item => selected.has(item.purchase.id))} onChange={toggleVisible} />Select all
        </label>
        <PriceReviewWorkspace items={items} evaluations={evaluations}
          review={{ ...review, focus: navigateReview, move: delta => { const id = review.move(delta); if (id) navigateReview(id); return id; } }}
          selected={selected} onToggleSelected={toggleCard} onSavePrice={handleInlinePriceSave}
          onRecheckInventory={recheckInventory} detailNavigationKey={reviewId ? location.key : undefined} />
      </> : isMobile ? (
        <div className="space-y-3">
          <label htmlFor="select-all-mobile" className="show-check flex items-center gap-2 text-xs text-[var(--text-muted)] px-1">
            <input id="select-all-mobile" aria-label="Select all visible cards" type="checkbox" checked={filteredAndSortedItems.length > 0 && filteredAndSortedItems.every(i => selected.has(i.purchase.id))}
              onChange={toggleVisible} className="rounded" />
            Select all
          </label>
          {filteredAndSortedItems.length === 0 && emptyMatches}
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
                      onToggle={() => toggleCard(item.purchase.id)}
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
                      showCampaignColumn={showCampaignColumn}
                      priceSupport={evidenceButton(item)}
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
                onChange={toggleVisible} className="rounded accent-[var(--brand-500)]" />
            </div>
            <SortableHeader label="Card" sortKey="name" currentKey={sortKey} currentDir={sortDir} onSort={handleSort} className="flex-1 min-w-[260px]" />
            <SortableHeader label="Gr" sortKey="grade" currentKey={sortKey} currentDir={sortDir} onSort={handleSort} className="text-center flex-shrink-0" style={{ width: '56px' }} />
            <SortableHeader label="Cost" sortKey="cost" currentKey={sortKey} currentDir={sortDir} onSort={handleSort} className="text-right flex-shrink-0" style={{ width: '96px' }} />
            <SortableHeader label="List / Rec" sortKey="market" currentKey={sortKey} currentDir={sortDir} onSort={handleSort} className="text-right flex-shrink-0" style={{ width: '168px' }} />
            <SortableHeader label="P/L" sortKey="pl" currentKey={sortKey} currentDir={sortDir} onSort={handleSort} className="text-right print-hide-col flex-shrink-0" style={{ width: '88px' }} />
            <SortableHeader label="Status" sortKey="days" currentKey={sortKey} currentDir={sortDir} onSort={handleSort} className="text-center print-hide-col flex-shrink-0" style={{ width: '112px' }} />
            <div className="glass-table-th flex-shrink-0 text-center print-hide-actions normal-case" style={{ width: ACTIONS_COLUMN_WIDTH }}>Actions</div>
          </div>
          {/* Rows */}
          {filteredAndSortedItems.length === 0 && emptyMatches}
          <div ref={scrollContainerRef} className="max-h-[600px] overflow-y-auto overflow-x-auto scrollbar-dark">
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
                        onToggle={() => toggleCard(item.purchase.id)}
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
                        priceSupport={evidenceButton(item)}
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

      {selected.size > 0 && <div aria-hidden="true" style={{ height: selectionBarHeight + 32 }} />}

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

      <InventorySelectionBar
        selectedItems={selectedItems} selected={selected} selectedVersions={selectedVersions} evaluations={evaluations}
        onHeightChange={setSelectionBarHeight} outsideView={outsideView} onReveal={revealSelected}
        showingSelected={pricing ? review.showingSelection : state.showingSelected} onHideSelected={pricing ? review.hideSelection : state.hideSelected}
        onAdded={ids => setSelected(prev => { const next = new Set(prev); for (const id of ids) next.delete(id); return next; })}
        onRecordSale={() => openSaleModal(selectedItems)}
        onListOnDH={() => handleBulkListOnDH(selectedItems.map(i => i.purchase.id))}
        onClear={() => setSelected(new Set())}
        disabled={anyModalOpen}
      />
    </div>
  );
}
