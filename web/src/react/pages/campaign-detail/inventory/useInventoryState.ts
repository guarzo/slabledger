import { useEffect, useMemo, useRef, useState, useCallback } from 'react';
import { useQueryClient } from '@tanstack/react-query';
import type { AgingItem, ExpectedValue, Purchase } from '../../../../types/campaigns';
import type { PriceFlagReason } from '../../../../types/campaigns/priceReview';
import { useDebounce } from '../../../hooks/useDebounce';
import { useToast } from '../../../contexts/ToastContext';
import { useSellSheet } from '../../../hooks/useSellSheet';
import { queryKeys } from '../../../queries/queryKeys';
import { useExpectedValues } from '../../../queries/useCampaignQueries';
import { api, isAPIError } from '../../../../js/api';
import { getErrorMessage } from '../../../utils/formatters';
import { costBasis, bestPrice } from './utils';
import type { SortKey, SortDir } from './utils';
import { computeInventoryMeta, filterAndSortItems } from './inventoryCalcs';
import type { FilterTab } from './inventoryCalcs';

const isAlreadyListedError = (err: unknown): boolean =>
  isAPIError(err) && err.status === 409 && err.data?.error === 'Purchase already listed on DH';

const isEffectiveSuccess = (r: PromiseSettledResult<unknown>): boolean =>
  r.status === 'fulfilled' || (r.status === 'rejected' && isAlreadyListedError(r.reason));

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
  const [fixMatchTarget, setFixMatchTarget] = useState<{
    purchaseId: string;
    cardName: string;
    certNumber?: string;
    currentDHCardId?: number;
  } | null>(null);
  const [sortKey, setSortKey] = useState<SortKey>('name');
  const [sortDir, setSortDir] = useState<SortDir>('asc');
  const [searchQuery, setSearchQuery] = useState('');
  const [isPrinting, setIsPrinting] = useState(false);
  const [statsExpanded, setStatsExpanded] = useState(false);
  const [filterTab, setFilterTab] = useState<FilterTab>('needs_attention');
  const userTabChosenRef = useRef(false);
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

  // Smart default tab: needs_attention → ready_to_list → pending_price → pending_dh_match → all. Runs once when items
  // first arrive and the user hasn't manually selected a tab.
  useEffect(() => {
    if (userTabChosenRef.current || items.length === 0) return;
    // Mark auto-default as resolved on the first non-empty render so a later drop
    // in needs_attention to zero doesn't auto-switch tabs out from under the user.
    userTabChosenRef.current = true;
    if (tabCounts.needs_attention > 0) return;
    if (tabCounts.ready_to_list > 0) {
      setFilterTab('ready_to_list');
    } else if (tabCounts.pending_price > 0) {
      setFilterTab('pending_price');
    } else if (tabCounts.pending_dh_match > 0) {
      setFilterTab('pending_dh_match');
    } else {
      setFilterTab('all');
    }
  }, [items.length, tabCounts.needs_attention, tabCounts.ready_to_list, tabCounts.pending_price, tabCounts.pending_dh_match]);

  const chooseFilterTab = useCallback((tab: FilterTab) => {
    userTabChosenRef.current = true;
    setFilterTab(tab);
  }, []);

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
      queryClient.invalidateQueries({ predicate: (query) => query.queryKey[0] === 'campaigns' && query.queryKey[2] === 'inventory' });
    }
    // InventoryTab renders both campaign-scoped and GlobalInventoryPage
    // (which reads portfolio.globalInventory). Always invalidate the global
    // inventory key so row-level mutations (price saves, DH actions) refresh
    // on the global page too — otherwise the user sees a stale row that
    // looks like the save never landed.
    queryClient.invalidateQueries({ queryKey: queryKeys.portfolio.globalInventory });
    if (opts?.sellSheet) {
      queryClient.invalidateQueries({ queryKey: queryKeys.portfolio.sellSheet });
    }
  }, [campaignId, queryClient]);

  const handleReviewed = useCallback(() => {
    invalidateInventory();
    setExpandedId(null);
  }, [invalidateInventory]);

  const handleResolveFlag = useCallback(async (flagId: number) => {
    try {
      await api.resolvePriceFlag(flagId);
      toast.success('Flag resolved');
      invalidateInventory();
    } catch (err) {
      toast.error(getErrorMessage(err, 'Failed to resolve flag'));
    }
  }, [toast, invalidateInventory]);

  const handleApproveDHPush = useCallback(async (purchaseId: string) => {
    try {
      await api.approveDHPush(purchaseId);
      toast.success('DH push approved — will push on next cycle');
      invalidateInventory();
    } catch (err) {
      toast.error(getErrorMessage(err, 'Failed to approve DH push'));
    }
  }, [toast, invalidateInventory]);

  const [dhListingInFlight, setDHListingInFlight] = useState<Set<string>>(new Set());
  const [dhListedOptimistic, setDHListedOptimistic] = useState<Set<string>>(new Set());
  // Clear optimistic overrides when fresh data arrives (items prop updates after refetch).
  useEffect(() => { if (dhListedOptimistic.size > 0) setDHListedOptimistic(new Set()); }, [items]); // eslint-disable-line react-hooks/exhaustive-deps

  const handleListOnDH = useCallback(async (purchaseId: string) => {
    setDHListingInFlight(prev => new Set(prev).add(purchaseId));
    try {
      await api.listPurchaseOnDH(purchaseId);
      toast.success('Listed on DH');
      setDHListedOptimistic(prev => new Set(prev).add(purchaseId));
      invalidateInventory();
    } catch (err) {
      if (isAlreadyListedError(err)) {
        toast.success('Listed on DH');
        setDHListedOptimistic(prev => new Set(prev).add(purchaseId));
      } else {
        toast.error(getErrorMessage(err, 'Failed to list on DH'));
      }
      invalidateInventory();
    } finally {
      setDHListingInFlight(prev => { const next = new Set(prev); next.delete(purchaseId); return next; });
    }
  }, [toast, invalidateInventory]);

  const handleBulkListOnDH = useCallback(async (purchaseIds: string[]) => {
    if (purchaseIds.length === 0) return;
    setDHListingInFlight(prev => {
      const next = new Set(prev);
      for (const id of purchaseIds) next.add(id);
      return next;
    });
    const CHUNK_SIZE = 5;
    const results: PromiseSettledResult<unknown>[] = [];
    for (let i = 0; i < purchaseIds.length; i += CHUNK_SIZE) {
      const chunk = purchaseIds.slice(i, i + CHUNK_SIZE);
      const chunkResults = await Promise.allSettled(chunk.map(id => api.listPurchaseOnDH(id)));
      results.push(...chunkResults);
      // Update in-flight and optimistic state per chunk so UI responds progressively.
      const chunkSucceeded: string[] = [];
      for (let j = 0; j < chunk.length; j++) {
        if (isEffectiveSuccess(chunkResults[j])) chunkSucceeded.push(chunk[j]);
      }
      if (chunkSucceeded.length > 0) {
        setDHListedOptimistic(prev => {
          const next = new Set(prev);
          for (const id of chunkSucceeded) next.add(id);
          return next;
        });
      }
      setDHListingInFlight(prev => {
        const next = new Set(prev);
        for (const id of chunk) next.delete(id);
        return next;
      });
    }
    const succeededIds = purchaseIds.filter((_, i) => isEffectiveSuccess(results[i]));
    const failed = purchaseIds.length - succeededIds.length;
    if (failed === 0) {
      toast.success(`Listed ${succeededIds.length} on DH`);
    } else if (succeededIds.length === 0) {
      toast.error(`Failed to list ${failed} on DH`);
    } else {
      toast.error(`Listed ${succeededIds.length}, ${failed} failed`);
    }
    if (succeededIds.length > 0) {
      setSelected(prev => {
        const next = new Set(prev);
        for (const id of succeededIds) next.delete(id);
        return next;
      });
    }
    invalidateInventory();
  }, [toast, invalidateInventory]);

  const handleFlagSubmit = useCallback(async (reason: PriceFlagReason) => {
    if (!flagTarget) return;
    setFlagSubmitting(true);
    try {
      await api.createPriceFlag(flagTarget.purchaseId, reason);
      toast.success('Price flag submitted');
      setFlagTarget(null);
      handleReviewed();
    } catch (err) {
      toast.error(getErrorMessage(err, 'Failed to submit price flag'));
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

  // Page-scoped sell-sheet count: only count items on sell sheet that are in hand.
  // sellSheetHas is stable (useCallback over a useMemo'd Set), so this memo only
  // recomputes when items or the sell-sheet contents actually change.
  const pageSellSheetCount = useMemo(() => {
    let count = 0;
    for (const item of items) {
      if (sellSheetHas(item.purchase.id) && !!item.purchase.receivedAt) count++;
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

  function handleFixDHMatch(purchase: Purchase) {
    setFixMatchTarget({
      purchaseId: purchase.id,
      cardName: purchase.cardName,
      certNumber: purchase.certNumber,
      currentDHCardId: purchase.dhCardId,
    });
  }

  function handleFixDHMatchSaved() {
    invalidateInventory();
  }

  const handleUnmatchDH = useCallback(async (purchase: Purchase) => {
    try {
      await api.unmatchDH(purchase.id);
      toast.success('DH match removed');
      invalidateInventory();
    } catch (err) {
      toast.error(getErrorMessage(err, 'Failed to remove DH match'));
    }
  }, [toast, invalidateInventory]);

  function handleSetPrice(item: AgingItem) {
    const currentPrice = bestPrice(item);
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

  const handleDelete = useCallback(async (item: AgingItem) => {
    const name = item.purchase.cardName || 'this card';
    if (!window.confirm(`Delete "${name}"? This will permanently remove it and any associated sale.`)) return;
    try {
      await api.deletePurchase(item.purchase.campaignId, item.purchase.id);
      toast.success(`Deleted "${name}"`);
      if (expandedId === item.purchase.id) setExpandedId(null);
      invalidateInventory({ sellSheet: true });
    } catch (err) {
      toast.error(getErrorMessage(err, 'Failed to delete purchase'));
    }
  }, [toast, expandedId, invalidateInventory]);

  function handlePriceSaved() {
    invalidateInventory({ sellSheet: true });
  }

  const handleInlinePriceSave = useCallback(async (purchaseId: string, priceCents: number) => {
    try {
      await api.setReviewedPrice(purchaseId, priceCents, 'manual');
      toast.success('Price saved');
      invalidateInventory({ sellSheet: true });
    } catch (err) {
      toast.error(getErrorMessage(err, 'Failed to save price'));
      throw err;
    }
  }, [toast, invalidateInventory]);

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
    fixMatchTarget, setFixMatchTarget,
    sortKey, sortDir,
    searchQuery, setSearchQuery,
    isPrinting,
    statsExpanded, setStatsExpanded,
    filterTab, setFilterTab: chooseFilterTab,
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
    handleResolveFlag,
    handleApproveDHPush,
    handleListOnDH,
    dhListingInFlight,
    dhListedOptimistic,
    handleBulkListOnDH,
    handleFlagSubmit,
    handlePrint,
    handleDelete,
    toggleSelect,
    toggleAll,
    toggleExpand,
    openSaleModal,
    closeSaleModal,
    handleFixPricing,
    handleFixDHMatch,
    handleFixDHMatchSaved,
    handleUnmatchDH,
    handleSetPrice,
    handlePriceSaved,
    handleInlinePriceSave,
    handleHintSaved,
    // Sell sheet
    sellSheet,
    // Toast (for inline JSX handlers)
    toast,
  };
}
