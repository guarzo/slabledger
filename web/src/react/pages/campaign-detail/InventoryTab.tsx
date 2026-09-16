import { useEffect, useMemo, useRef, useState } from 'react';
import { useVirtualizer } from '@tanstack/react-virtual';
import type { ShowEvaluation, SupportStatus } from '../../../types/showprep';
import { useShowReadiness } from '../../queries/useShowReadiness';
import { useShowPrepCoverage } from '../../queries/useShowPrepWorker';
import ShowReadinessLine from '../show-preparation/ShowReadinessLine';
import { useShowEvaluations } from '../../queries/useShowPrepQueries';
import ShowEvidenceDisclosure, { ShowEvidenceButton } from '../show-preparation/ShowEvidence';
import { ShowInventoryFilters } from '../show-preparation/ShowInventoryControls';
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
import InventorySelectionBar from './inventory/InventorySelectionBar';
import { ACTIONS_COLUMN_WIDTH } from './inventory/columnWidths';

const EMPTY_EVALUATIONS: Record<string, ShowEvaluation> = {};

export interface InventoryTabProps {
  items: AgingItem[];
  isLoading: boolean;
  campaignId?: string;
  showCampaignColumn?: boolean;
}

export default function InventoryTab({ items, isLoading: loading, campaignId, showCampaignColumn }: InventoryTabProps) {
  const isMobile = useMediaQuery('(max-width: 768px)');
  const [support, setSupport] = useState<SupportStatus | 'all'>('all');
  const [selectedVersions, setSelectedVersions] = useState<Record<string, string>>({});
  const [selectionBarHeight, setSelectionBarHeight] = useState(0);
  // Virtual rows unmount offscreen. Keep evidence disclosure intent with inventory,
  // not the measured row, so scrolling or resizing cannot silently close it.
  const [evidenceExpandedId, setEvidenceExpandedId] = useState<string | null>(null);
  const purchaseIds = useMemo(() => items.map(item => item.purchase.id), [items]);
  const evaluationsQuery = useShowEvaluations(purchaseIds);
  const coverage = useShowPrepCoverage();
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
  const showFilters = useMemo(() => ({ support, evaluations }), [support, evaluations]);
  const state = useInventoryState(items, campaignId, showFilters);
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

  const readiness = useShowReadiness(purchaseIds, evaluations);
  const outsideView = [...selected].filter(id => !filteredAndSortedItems.some(item => item.purchase.id === id)).length;
  const revealSelected = state.revealSelected;

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
    const ids = filteredAndSortedItems.map(item => item.purchase.id);
    captureSelection(ids, !ids.every(id => selected.has(id)));
    toggleAll();
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

  const evidenceButton = (item: AgingItem) => <ShowEvidenceButton purchaseId={item.purchase.id} certNumber={item.purchase.certNumber || ''}
    evaluation={evaluations[item.purchase.id]} loading={evaluationsQuery.isFetching} showListedPrice expanded={evidenceExpandedId === item.purchase.id}
    onClick={() => setEvidenceExpandedId(evidenceExpandedId === item.purchase.id ? null : item.purchase.id)} />;
  const evidencePanel = (item: AgingItem) => evidenceExpandedId === item.purchase.id && <ShowEvidenceDisclosure
    purchaseId={item.purchase.id} certNumber={item.purchase.certNumber || ''} detailsOnly expanded
    evaluation={evaluations[item.purchase.id]} loading={evaluationsQuery.isFetching} error={evaluationsQuery.data?.errors[item.purchase.id]} />;
  const emptyMatches = <div className="show-empty-matches">
    <p>{support !== 'all' ? 'No current matches under these filters.'
      : debouncedSearch ? `No cards match "${debouncedSearch}"` : 'No cards in this view'}</p>
    {support !== 'all' && readiness.incomplete && <p>Comp data coverage is incomplete. Missing or out-of-date evidence is not a current match.</p>}
    {support !== 'all' && <button className="show-link" onClick={() => setSupport('all')}>Clear price support filter</button>}
  </div>;

  return (
    <div className="show-inventory">
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
        showFiltering={support !== 'all'}
        onDeselectMissingCL={handleDeselectMissingCL}
        onHighlightMissingCL={handleHighlightMissingCL}
      />

      <ShowInventoryFilters filters={showFilters} setSupport={setSupport}
        count={filteredAndSortedItems.length}
        pending={evaluationsQuery.isFetching} fetching={evaluationsQuery.isFetching}
        failed={Object.keys(evaluationsQuery.data?.errors ?? {}).length + (evaluationsQuery.isFetching ? 0 : evaluationsQuery.unresolvedCount)}
        onRetry={() => { void evaluationsQuery.refetch(); }}>
        <ShowReadinessLine readiness={readiness} coverage={coverage.data} coverageError={coverage.isError} />
      </ShowInventoryFilters>

      {isMobile ? (
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
                    >{evidencePanel(item)}</MobileCard>
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
                    {evidencePanel(item)}
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
        showingSelected={state.showingSelected} onHideSelected={state.hideSelected}
        onAdded={ids => setSelected(prev => { const next = new Set(prev); for (const id of ids) next.delete(id); return next; })}
        onRecordSale={() => openSaleModal(selectedItems)}
        onListOnDH={() => handleBulkListOnDH(selectedItems.map(i => i.purchase.id))}
        onClear={() => setSelected(new Set())}
        disabled={anyModalOpen}
      />
    </div>
  );
}
