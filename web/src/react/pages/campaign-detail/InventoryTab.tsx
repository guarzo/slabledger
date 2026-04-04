import { useEffect, useMemo, useRef, useState, useCallback } from 'react';
import { useQueryClient } from '@tanstack/react-query';
import { useVirtualizer } from '@tanstack/react-virtual';
import type { AgingItem, ExpectedValue, Purchase, ReviewStats } from '../../../types/campaigns';
import PokeballLoader from '../../PokeballLoader';
import { formatCents, formatPct } from '../../utils/formatters';
import { useMediaQuery } from '../../hooks/useMediaQuery';
import { useDebounce } from '../../hooks/useDebounce';
import { useToast } from '../../contexts/ToastContext';
import { useSellSheet } from '../../hooks/useSellSheet';
import { EmptyState, Button } from '../../ui';
import { queryKeys } from '../../queries/queryKeys';
import { useExpectedValues } from '../../queries/useCampaignQueries';
import { api } from '../../../js/api';
import type { PriceFlagReason } from '../../../types/campaigns/priceReview';
import RecordSaleModal from './RecordSaleModal';
import PriceHintDialog from '../../PriceHintDialog';
import PriceOverrideDialog from '../../PriceOverrideDialog';
import { bestPrice, unrealizedPL, formatPL, getReviewStatus, reviewUrgencySort, isCardShowCandidate } from './inventory/utils';
import type { SortKey, SortDir } from './inventory/utils';
import DesktopRow from './inventory/DesktopRow';
import MobileCard from './inventory/MobileCard';
import CrackCandidatesBanner from './inventory/CrackCandidatesBanner';
import SortableHeader from './inventory/SortableHeader';
import ExpandedDetail from './inventory/ExpandedDetail';
import PriceFlagDialog from './inventory/PriceFlagDialog';
import ReviewSummaryBar from './inventory/ReviewSummaryBar';

export interface InventoryTabProps {
  items: AgingItem[];
  isLoading: boolean;
  campaignId?: string;
  showCampaignColumn?: boolean;
}

export default function InventoryTab({ items, isLoading: loading, campaignId, showCampaignColumn }: InventoryTabProps) {
  const queryClient = useQueryClient();
  const toast = useToast();
  const sellSheet = useSellSheet();
  const { data: evPortfolio } = useExpectedValues(campaignId ?? '');
  const [selected, setSelected] = useState<Set<string>>(new Set());
  const [expandedId, setExpandedId] = useState<string | null>(null);
  const [saleModalOpen, setSaleModalOpen] = useState(false);
  const [saleModalItems, setSaleModalItems] = useState<AgingItem[]>([]);
  const [hintTarget, setHintTarget] = useState<{ cardName: string; setName: string; cardNumber: string } | null>(null);
  const [priceTarget, setPriceTarget] = useState<{
    purchaseId: string;
    cardName: string;
    costBasisCents: number;
    currentPriceCents: number;
    currentOverrideCents?: number;
    currentOverrideSource?: string;
    aiSuggestedCents?: number;
  } | null>(null);
  const [flagTarget, setFlagTarget] = useState<{ purchaseId: string; cardName: string; grade: number } | null>(null);
  const [flagSubmitting, setFlagSubmitting] = useState(false);
  const [sortKey, setSortKey] = useState<SortKey>('name');
  const [sortDir, setSortDir] = useState<SortDir>('asc');
  const [searchQuery, setSearchQuery] = useState('');
  const [statsExpanded, setStatsExpanded] = useState(false);
  const [filterTab, setFilterTab] = useState<'needs_review' | 'large_gap' | 'no_data' | 'flagged' | 'card_show' | 'all' | 'sell_sheet'>('needs_review');
  const [showAll, setShowAll] = useState(false);
  const debouncedSearch = useDebounce(searchQuery, 300);
  const isMobile = useMediaQuery('(max-width: 768px)');
  const scrollContainerRef = useRef<HTMLDivElement>(null);
  const mobileScrollRef = useRef<HTMLDivElement>(null);

  // Compute review stats and filter tab counts in a single pass
  const { reviewStats, tabCounts } = useMemo(() => {
    const stats: ReviewStats = { total: items.length, needsReview: 0, reviewed: 0, flagged: 0 };
    const counts = { needs_review: 0, large_gap: 0, no_data: 0, flagged: 0, card_show: 0, all: items.length };
    for (const item of items) {
      if (item.hasOpenFlag) stats.flagged++;
      if (item.purchase.reviewedAt) stats.reviewed++;
      else stats.needsReview++;

      const status = getReviewStatus(item);
      if (status === 'needs_review') { counts.needs_review++; }
      else if (status === 'large_gap') { counts.needs_review++; counts.large_gap++; }
      else if (status === 'no_data') { counts.needs_review++; counts.no_data++; }
      else if (status === 'flagged') counts.flagged++;
      if (isCardShowCandidate(item)) counts.card_show++;
    }
    return { reviewStats: stats, tabCounts: counts };
  }, [items]);

  function handleSort(key: SortKey) {
    if (sortKey === key) {
      setSortDir(prev => prev === 'asc' ? 'desc' : 'asc');
    } else {
      setSortKey(key);
      setSortDir('asc');
    }
  }

  // Reset scroll + collapse expanded row on sort/filter change
  useEffect(() => {
    setExpandedId(null);
    scrollContainerRef.current?.scrollTo({ top: 0 });
    mobileScrollRef.current?.scrollTo({ top: 0 });
  }, [sortKey, sortDir, debouncedSearch, filterTab, showAll]);

  const handleReviewed = useCallback(() => {
    if (campaignId) {
      queryClient.invalidateQueries({ queryKey: queryKeys.campaigns.inventory(campaignId) });
    } else {
      queryClient.invalidateQueries({ predicate: (query) => query.queryKey[0] === 'campaigns' });
    }
    setExpandedId(null);
  }, [campaignId, queryClient]);

  const handleFlagSubmit = useCallback(async (reason: PriceFlagReason) => {
    if (!flagTarget) return;
    setFlagSubmitting(true);
    try {
      await api.createPriceFlag(flagTarget.purchaseId, reason);
      toast.success('Price flag submitted');
      setFlagTarget(null);
      handleReviewed();
    } catch (err) {
      const message = err instanceof Error ? err.message : 'Failed to submit price flag';
      toast.error(message);
    } finally {
      setFlagSubmitting(false);
    }
  }, [flagTarget, toast, handleReviewed]);

  // Build EV lookup map by certNumber; hide if insufficient data points or no campaignId
  const showEV = !!campaignId && evPortfolio && evPortfolio.items?.length > 0 && evPortfolio.minDataPoints >= 30;
  const evMap = useMemo(() => {
    if (!showEV) return new Map<string, ExpectedValue>();
    const map = new Map<string, ExpectedValue>();
    for (const ev of evPortfolio.items) {
      map.set(ev.certNumber, ev);
    }
    return map;
  }, [showEV, evPortfolio]);

  const filteredAndSortedItems = useMemo(() => {
    let result = items;

    // Search always overrides: if search query is set, search all items regardless of tab
    if (debouncedSearch.trim()) {
      const q = debouncedSearch.toLowerCase();
      result = result.filter(i =>
        i.purchase.cardName.toLowerCase().includes(q) ||
        (i.purchase.certNumber && i.purchase.certNumber.toLowerCase().includes(q)) ||
        (i.purchase.setName && i.purchase.setName.toLowerCase().includes(q))
      );
    } else if (!showAll) {
      // Filter by active tab using getReviewStatus
      if (filterTab === 'sell_sheet') {
        result = result.filter(i => sellSheet.has(i.purchase.id));
      } else if (filterTab !== 'all') {
        result = result.filter(i => {
          const status = getReviewStatus(i);
          if (filterTab === 'large_gap') return status === 'large_gap';
          if (filterTab === 'no_data') return status === 'no_data';
          if (filterTab === 'flagged') return status === 'flagged';
          if (filterTab === 'card_show') return isCardShowCandidate(i);
          // 'needs_review' tab shows needs_review + large_gap + no_data (all unreviewed/unflagged)
          return status === 'needs_review' || status === 'large_gap' || status === 'no_data';
        });
      }
    }

    // Sort: when !showAll and no search, use queue urgency sort; otherwise use user-selected sort
    if (!showAll && !debouncedSearch.trim()) {
      return [...result].sort(reviewUrgencySort);
    }

    const dir = sortDir === 'asc' ? 1 : -1;
    return [...result].sort((a, b) => {
      switch (sortKey) {
        case 'name':
          return dir * a.purchase.cardName.localeCompare(b.purchase.cardName);
        case 'grade':
          return dir * (a.purchase.gradeValue - b.purchase.gradeValue);
        case 'cost': {
          const ca = a.purchase.buyCostCents + a.purchase.psaSourcingFeeCents;
          const cb = b.purchase.buyCostCents + b.purchase.psaSourcingFeeCents;
          return dir * (ca - cb);
        }
        case 'market': {
          const ma = a.currentMarket ? bestPrice(a.currentMarket) : 0;
          const mb = b.currentMarket ? bestPrice(b.currentMarket) : 0;
          return dir * (ma - mb);
        }
        case 'pl': {
          const pa = unrealizedPL(a.purchase.buyCostCents + a.purchase.psaSourcingFeeCents, a.currentMarket) ?? -Infinity;
          const pb = unrealizedPL(b.purchase.buyCostCents + b.purchase.psaSourcingFeeCents, b.currentMarket) ?? -Infinity;
          return dir * (pa - pb);
        }
        case 'days':
          return dir * (a.daysHeld - b.daysHeld);
        case 'ev': {
          const ea = evMap.get(a.purchase.certNumber)?.evCents ?? -Infinity;
          const eb = evMap.get(b.purchase.certNumber)?.evCents ?? -Infinity;
          return dir * (ea - eb);
        }
        default:
          return 0;
      }
    });
  }, [items, debouncedSearch, sortKey, sortDir, evMap, showAll, filterTab, sellSheet]);

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
  // so getTotalSize() is correct and the scroll container adjusts its height.
  useEffect(() => {
    rowVirtualizer.measure();
  }, [expandedId, rowVirtualizer]);

  const mobileVirtualizer = useVirtualizer({
    count: filteredAndSortedItems.length,
    getScrollElement: () => mobileScrollRef.current,
    estimateSize: () => 140,
    overscan: 5,
  });

  function toggleSelect(purchaseId: string) {
    setSelected(prev => {
      const next = new Set(prev);
      if (next.has(purchaseId)) next.delete(purchaseId);
      else next.add(purchaseId);
      return next;
    });
  }

  function toggleAll() {
    const visibleIds = filteredAndSortedItems.map(i => i.purchase.id);
    const allVisibleSelected = visibleIds.length > 0 && visibleIds.every(id => selected.has(id));
    setSelected(prev => {
      const next = new Set(prev);
      if (allVisibleSelected) {
        for (const id of visibleIds) next.delete(id);
      } else {
        for (const id of visibleIds) next.add(id);
      }
      return next;
    });
  }

  function toggleExpand(purchaseId: string) {
    setExpandedId(prev => prev === purchaseId ? null : purchaseId);
  }

  function openSaleModal(saleItems: AgingItem[]) {
    setSaleModalItems(saleItems);
    setSaleModalOpen(true);
  }

  function closeSaleModal() {
    setSaleModalOpen(false);
    setSaleModalItems([]);
  }

  function handleFixPricing(purchase: Purchase) {
    if (!purchase.setName || !purchase.cardNumber) {
      toast.error('Cannot create hint: set name and card number are required');
      return;
    }
    setHintTarget({ cardName: purchase.cardName, setName: purchase.setName, cardNumber: purchase.cardNumber });
  }

  function handleSetPrice(item: AgingItem) {
    const costBasis = item.purchase.buyCostCents + item.purchase.psaSourcingFeeCents;
    const currentPrice = item.currentMarket ? bestPrice(item.currentMarket) : 0;
    setPriceTarget({
      purchaseId: item.purchase.id,
      cardName: item.purchase.cardName,
      costBasisCents: costBasis,
      currentPriceCents: currentPrice,
      currentOverrideCents: item.purchase.overridePriceCents,
      currentOverrideSource: item.purchase.overrideSource,
      aiSuggestedCents: item.purchase.aiSuggestedPriceCents,
    });
  }

  function handlePriceSaved() {
    if (campaignId) {
      queryClient.invalidateQueries({ queryKey: queryKeys.campaigns.inventory(campaignId) });
    } else {
      queryClient.invalidateQueries({ predicate: (query) => query.queryKey[0] === 'campaigns' });
    }
    // Always invalidate sell sheet — overrides affect global sell sheet regardless of view
    queryClient.invalidateQueries({ queryKey: queryKeys.portfolio.sellSheet });
  }

  function handleHintSaved() {
    if (campaignId) {
      queryClient.invalidateQueries({ queryKey: queryKeys.campaigns.inventory(campaignId) });
    } else {
      queryClient.invalidateQueries({ predicate: (query) => query.queryKey[0] === 'campaigns' && query.queryKey[2] === 'inventory' });
    }
  }

  // Summary stats
  const totalCost = items.reduce((sum, i) => sum + i.purchase.buyCostCents + i.purchase.psaSourcingFeeCents, 0);
  const totalMarket = items.reduce((sum, i) => {
    if (!i.currentMarket) return sum;
    return sum + bestPrice(i.currentMarket);
  }, 0);
  const totalPL = totalMarket > 0 ? totalMarket - totalCost : 0;

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

  return (
    <div>
      {/* Summary stat cards — collapsible on mobile */}
      {isMobile ? (
        <div className="mb-4">
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
        <div className="mb-7 pb-6 border-b border-[rgba(255,255,255,0.05)]">
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
                  <div className={`text-base font-semibold ${evPortfolio.totalEvCents >= 0 ? 'text-[var(--success)]' : 'text-[var(--danger)]'}`}>
                    {formatPL(evPortfolio.totalEvCents)}
                  </div>
                </div>
              )}
            </div>
          </div>
        </div>
      )}

      {selected.size > 0 && (
        <div className="flex items-center justify-between mb-3">
          <span className="text-sm text-[var(--text-muted)]">{selected.size} selected</span>
          <div className="flex items-center gap-2">
            {filterTab === 'sell_sheet' ? (
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

      {filterTab === 'sell_sheet' && sellSheet.count > 0 && (
        <div className="flex justify-end mb-3">
          <Button
            size="sm"
            variant="secondary"
            onClick={() => window.print()}
          >
            Print Sell Sheet
          </Button>
        </div>
      )}

      {/* Crack Candidates Banner */}
      {campaignId && <CrackCandidatesBanner campaignId={campaignId} />}

      {/* Review Summary Bar */}
      <div className="mb-4">
        <ReviewSummaryBar
          stats={reviewStats}
          searchQuery={searchQuery}
          onSearchChange={setSearchQuery}
          showAll={showAll}
          onToggleShowAll={() => setShowAll(prev => !prev)}
        />
      </div>

      {/* Filter tabs — visible when not in showAll mode */}
      {!showAll && (
        <div className="flex items-center gap-2 mb-3 flex-wrap">
          {([
            { key: 'needs_review' as const, label: 'Needs Review', color: 'var(--warning)' },
            { key: 'large_gap' as const, label: 'Large Gap', color: 'var(--danger)' },
            { key: 'no_data' as const, label: 'No Data', color: 'var(--text-muted)' },
            { key: 'flagged' as const, label: 'Flagged', color: 'var(--danger)' },
            { key: 'card_show' as const, label: 'Card Show', color: 'var(--brand-400)' },
            { key: 'all' as const, label: 'All', color: 'var(--text)' },
          ] as const).map(tab => (
            <button
              key={tab.key}
              type="button"
              onClick={() => setFilterTab(tab.key)}
              className={`text-xs font-medium px-3 py-1.5 rounded-full border transition-colors ${
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
                {tabCounts[tab.key]}
              </span>
            </button>
          ))}
          <button
            type="button"
            onClick={() => setFilterTab('sell_sheet')}
            className={`text-xs font-medium px-3 py-1.5 rounded-full border transition-colors ${
              filterTab === 'sell_sheet'
                ? 'border-[var(--brand-500)] bg-[var(--brand-500)]/10 text-[var(--brand-400)]'
                : 'border-[var(--surface-2)] text-[var(--text-muted)] hover:text-[var(--text)] hover:border-[var(--text-muted)]'
            }`}
          >
            Sell Sheet
            <span
              className="ml-1.5 inline-block min-w-[18px] text-center text-[10px] font-semibold px-1 py-[1px] rounded-full"
              style={{
                background: filterTab === 'sell_sheet' ? 'color-mix(in srgb, var(--brand-400) 15%, transparent)' : 'rgba(255,255,255,0.06)',
                color: filterTab === 'sell_sheet' ? 'var(--brand-400)' : 'var(--text-muted)',
              }}
            >
              {sellSheet.count}
            </span>
          </button>
        </div>
      )}

      {debouncedSearch && (
        <div className="text-xs text-[var(--text-subtle)] mb-2 pl-1">
          {filteredAndSortedItems.length} of {items.length} cards
        </div>
      )}

      {filterTab === 'sell_sheet' && filteredAndSortedItems.length === 0 && !debouncedSearch && (
        <div className="text-center py-12">
          <div className="text-[var(--text-muted)] text-sm">No items on your sell sheet.</div>
          <div className="text-[var(--text-muted)] text-xs mt-1">Select items from any tab and click &ldquo;Add to Sell Sheet&rdquo;.</div>
        </div>
      )}

      {isMobile ? (
        <div className="space-y-3">
          <label className="flex items-center gap-2 text-xs text-[var(--text-muted)] px-1">
            <input type="checkbox" checked={filteredAndSortedItems.length > 0 && filteredAndSortedItems.every(i => selected.has(i.purchase.id))}
              onChange={toggleAll} className="rounded" />
            Select all
          </label>
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
                      onSetPrice={() => handleSetPrice(item)}
                      ev={evMap.get(item.purchase.certNumber)}
                      showCampaignColumn={showCampaignColumn}
                      isOnSellSheet={filterTab !== 'sell_sheet' && sellSheet.has(item.purchase.id)}
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
            <div className="glass-table-th flex-shrink-0 !px-1" style={{ width: '28px' }}>
              <input type="checkbox" aria-label="Select all visible cards" checked={filteredAndSortedItems.length > 0 && filteredAndSortedItems.every(i => selected.has(i.purchase.id))}
                onChange={toggleAll} className="rounded accent-[var(--brand-500)]" />
            </div>
            <SortableHeader label="Card" sortKey="name" currentKey={sortKey} currentDir={sortDir} onSort={handleSort} className="flex-1 min-w-0" />
            <SortableHeader label="Gr" sortKey="grade" currentKey={sortKey} currentDir={sortDir} onSort={handleSort} className="text-center" style={{ width: '36px' }} />
            <SortableHeader label="Cost" sortKey="cost" currentKey={sortKey} currentDir={sortDir} onSort={handleSort} className="text-right" style={{ width: '72px' }} />
            <SortableHeader label="Market" sortKey="market" currentKey={sortKey} currentDir={sortDir} onSort={handleSort} className="text-right" style={{ width: '120px' }} />
            <div className="glass-table-th flex-shrink-0 text-right" style={{ width: '68px' }}>CL</div>
            <SortableHeader label="P/L" sortKey="pl" currentKey={sortKey} currentDir={sortDir} onSort={handleSort} className="text-right" style={{ width: '72px' }} />
            <SortableHeader label="Days" sortKey="days" currentKey={sortKey} currentDir={sortDir} onSort={handleSort} className="text-center" style={{ width: '40px' }} />
            <div className="glass-table-th flex-shrink-0 text-center" style={{ width: '48px' }}>Signal</div>
            <div className="glass-table-th flex-shrink-0 text-right" style={{ width: '68px' }}>Rec.</div>
            <div className="glass-table-th flex-shrink-0 text-center" style={{ width: '72px' }}>Status</div>
            {showEV && <SortableHeader label="EV" sortKey="ev" currentKey={sortKey} currentDir={sortDir} onSort={handleSort} className="text-right" style={{ width: '64px' }} />}
            <div className="glass-table-th flex-shrink-0 !px-1" style={{ width: '28px' }}></div>
          </div>
          {/* Rows */}
          <div ref={scrollContainerRef} className="max-h-[600px] overflow-y-auto overflow-x-hidden scrollbar-dark">
            <div style={{ height: `${rowVirtualizer.getTotalSize()}px`, position: 'relative' }}>
              {rowVirtualizer.getVirtualItems().map(virtualRow => {
                const item = filteredAndSortedItems[virtualRow.index];
                const isExpanded = expandedId === item.purchase.id;
                const rowPl = unrealizedPL(item.purchase.buyCostCents + item.purchase.psaSourcingFeeCents, item.currentMarket);
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
                        ev={evMap.get(item.purchase.certNumber)}
                        showEV={!!showEV}
                        showCampaignColumn={showCampaignColumn}
                        isOnSellSheet={filterTab !== 'sell_sheet' && sellSheet.has(item.purchase.id)}
                      />
                    </div>
                    {isExpanded && <ExpandedDetail item={item} onReviewed={handleReviewed} campaignId={campaignId} onOpenFlagDialog={() => setFlagTarget({ purchaseId: item.purchase.id, cardName: item.purchase.cardName, grade: item.purchase.gradeValue })} />}
                  </div>
                );
              })}
            </div>
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
