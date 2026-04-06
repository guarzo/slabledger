import { useEffect, useMemo, useRef, useState, useCallback } from 'react';
import { useQueryClient } from '@tanstack/react-query';
import type { AgingItem, ExpectedValue, Purchase } from '../../../../types/campaigns';
import type { PriceFlagReason } from '../../../../types/campaigns/priceReview';
import { useDebounce } from '../../../hooks/useDebounce';
import { useToast } from '../../../contexts/ToastContext';
import { useSellSheet } from '../../../hooks/useSellSheet';
import { queryKeys } from '../../../queries/queryKeys';
import { useExpectedValues } from '../../../queries/useCampaignQueries';
import { api } from '../../../../js/api';
import { costBasis, bestPrice } from './utils';
import type { SortKey, SortDir } from './utils';
import { computeInventoryMeta, filterAndSortItems } from './inventoryCalcs';
import type { FilterTab } from './inventoryCalcs';

export function useInventoryState(items: AgingItem[], campaignId?: string) {
  const queryClient = useQueryClient();
  const toast = useToast();
  const sellSheet = useSellSheet();
  const { has: sellSheetHas } = sellSheet;
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
  const [isPrinting, setIsPrinting] = useState(false);
  const [statsExpanded, setStatsExpanded] = useState(false);
  const [filterTab, setFilterTab] = useState<FilterTab>('needs_review');
  const [showAll, setShowAll] = useState(false);
  const debouncedSearch = useDebounce(searchQuery, 300);
  const scrollContainerRef = useRef<HTMLDivElement>(null);
  const mobileScrollRef = useRef<HTMLDivElement>(null);

  const handlePrint = useCallback(() => {
    setIsPrinting(true);
    requestAnimationFrame(() => {
      requestAnimationFrame(() => {
        window.print();
        setIsPrinting(false);
      });
    });
  }, []);

  const { reviewStats, tabCounts, summary } = useMemo(
    () => computeInventoryMeta(items),
    [items],
  );

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

  const invalidateInventory = useCallback((opts?: { sellSheet?: boolean }) => {
    if (campaignId) {
      queryClient.invalidateQueries({ queryKey: queryKeys.campaigns.inventory(campaignId) });
    } else {
      queryClient.invalidateQueries({ predicate: (query) => query.queryKey[0] === 'campaigns' });
    }
    if (opts?.sellSheet) {
      queryClient.invalidateQueries({ queryKey: queryKeys.portfolio.sellSheet });
    }
  }, [campaignId, queryClient]);

  const handleReviewed = useCallback(() => {
    invalidateInventory();
    setExpandedId(null);
  }, [invalidateInventory]);

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

  // Page-scoped sell-sheet count: only count items actually present on this page
  const pageSellSheetCount = useMemo(() => {
    let count = 0;
    for (const item of items) {
      if (sellSheetHas(item.purchase.id)) count++;
    }
    return count;
  }, [items, sellSheetHas]);

  // Whether the sell-sheet filter is truly active (not bypassed by showAll or search)
  const sellSheetActive = filterTab === 'sell_sheet' && !showAll && !debouncedSearch.trim();

  const filteredAndSortedItems = useMemo(
    () => filterAndSortItems(items, {
      debouncedSearch,
      showAll,
      filterTab,
      sellSheetHas,
      sortKey,
      sortDir,
      evMap,
    }),
    [items, debouncedSearch, sortKey, sortDir, evMap, showAll, filterTab, sellSheetHas],
  );

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
    const currentPrice = item.currentMarket ? bestPrice(item.currentMarket) : 0;
    setPriceTarget({
      purchaseId: item.purchase.id,
      cardName: item.purchase.cardName,
      costBasisCents: costBasis(item.purchase),
      currentPriceCents: currentPrice,
      currentOverrideCents: item.purchase.overridePriceCents,
      currentOverrideSource: item.purchase.overrideSource,
      aiSuggestedCents: item.purchase.aiSuggestedPriceCents,
    });
  }

  function handlePriceSaved() {
    invalidateInventory({ sellSheet: true });
  }

  function handleHintSaved() {
    invalidateInventory();
  }

  const { totalCost, totalMarket, totalPL } = summary;

  return {
    // Refs
    scrollContainerRef,
    mobileScrollRef,
    // State
    selected, setSelected,
    expandedId, setExpandedId,
    saleModalOpen,
    saleModalItems,
    hintTarget, setHintTarget,
    priceTarget, setPriceTarget,
    flagTarget, setFlagTarget,
    flagSubmitting,
    sortKey, sortDir,
    searchQuery, setSearchQuery,
    isPrinting,
    statsExpanded, setStatsExpanded,
    filterTab, setFilterTab,
    showAll, setShowAll,
    debouncedSearch,
    // Computed
    reviewStats, tabCounts,
    showEV: !!showEV,
    evPortfolio,
    evMap,
    pageSellSheetCount,
    sellSheetActive,
    filteredAndSortedItems,
    totalCost, totalMarket, totalPL,
    // Handlers
    handleSort,
    handleReviewed,
    handleFlagSubmit,
    handlePrint,
    toggleSelect,
    toggleAll,
    toggleExpand,
    openSaleModal,
    closeSaleModal,
    handleFixPricing,
    handleSetPrice,
    handlePriceSaved,
    handleHintSaved,
    // Sell sheet
    sellSheet,
    // Toast (for inline JSX handlers)
    toast,
  };
}
