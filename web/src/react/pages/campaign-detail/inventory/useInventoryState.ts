import { useEffect, useMemo, useRef, useState, useCallback } from 'react';
import { useQueryClient } from '@tanstack/react-query';
import type { AgingItem, ExpectedValue } from '../../../../types/campaigns';
import { useDebounce } from '../../../hooks/useDebounce';
import { useToast } from '../../../contexts/ToastContext';
import { queryKeys } from '../../../queries/queryKeys';
import { useExpectedValues } from '../../../queries/useCampaignQueries';
import { api } from '../../../../js/api';
import { getErrorMessage } from '../../../utils/formatters';
import type { SortKey, SortDir } from './utils';
import { computeInventoryMeta, computeTotals, filterAndSortItems } from './inventoryCalcs';
import type { FilterTab } from './inventoryCalcs';
import { useInventorySelection } from './useInventorySelection';
import { useDHActions } from './useDHActions';
import { usePricingActions } from './usePricingActions';

export function useInventoryState(items: AgingItem[], campaignId?: string) {
  const queryClient = useQueryClient();
  const toast = useToast();
  const { data: evPortfolio } = useExpectedValues(campaignId ?? '');
  const selection = useInventorySelection();
  const [sortKey, setSortKey] = useState<SortKey>('name');
  const [sortDir, setSortDir] = useState<SortDir>('asc');
  const [searchQuery, setSearchQuery] = useState('');
  const [filterTab, setFilterTab] = useState<FilterTab>('all');
  const userTabChosenRef = useRef(false);
  const [showAll, setShowAll] = useState(false);
  const debouncedSearch = useDebounce(searchQuery, 300);
  const scrollContainerRef = useRef<HTMLDivElement>(null);
  const mobileScrollRef = useRef<HTMLDivElement>(null);
  const [saleModalOpen, setSaleModalOpen] = useState(false);
  const [saleModalItems, setSaleModalItems] = useState<AgingItem[]>([]);

  const invalidateInventory = useCallback((opts?: { sellSheet?: boolean }) => {
    if (campaignId) {
      queryClient.invalidateQueries({ queryKey: queryKeys.campaigns.inventory(campaignId) });
    } else {
      queryClient.invalidateQueries({ predicate: (query) => query.queryKey[0] === 'campaigns' && query.queryKey[2] === 'inventory' });
    }
    queryClient.invalidateQueries({ queryKey: queryKeys.portfolio.globalInventory });
    if (opts?.sellSheet) {
      queryClient.invalidateQueries({ queryKey: queryKeys.portfolio.sellSheet });
    }
  }, [campaignId, queryClient]);

  const handleReviewed = useCallback(() => {
    invalidateInventory();
    selection.setExpandedId(null);
  }, [invalidateInventory, selection.setExpandedId]);

  const dhActions = useDHActions({
    toast,
    invalidateInventory,
    items,
    setSelected: selection.setSelected,
  });

  const pricingActions = usePricingActions({
    toast,
    invalidateInventory,
    onReviewed: handleReviewed,
  });
  // Stale pinnedIds fix
  useEffect(() => {
    if (selection.selected.size === 0 || items.length === 0) {
      selection.setPinnedIds(new Set());
    }
  }, [items, selection.selected.size]); // eslint-disable-line react-hooks/exhaustive-deps

  const { reviewStats, tabCounts, summary } = useMemo(
    () => computeInventoryMeta(items),
    [items],
  );

  // Smart default tab: needs_attention if > 0, else all
  useEffect(() => {
    if (userTabChosenRef.current || items.length === 0) return;
    userTabChosenRef.current = true;
    if (tabCounts.needs_attention > 0) {
      setFilterTab('needs_attention');
    } else {
      setFilterTab('all');
    }
  }, [items.length, tabCounts.needs_attention]);

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
    selection.setExpandedId(null);
    selection.setPinnedIds(prev => prev.size > 0 ? new Set() : prev);
    scrollContainerRef.current?.scrollTo({ top: 0 });
    mobileScrollRef.current?.scrollTo({ top: 0 });
  }, [sortKey, sortDir, debouncedSearch, filterTab, showAll]); // eslint-disable-line react-hooks/exhaustive-deps

  // Build EV lookup map
  const showEV = !!campaignId && evPortfolio && evPortfolio.items?.length > 0 && evPortfolio.minDataPoints >= 30;
  const evMap = useMemo(() => {
    if (!showEV) return new Map<string, ExpectedValue>();
    const map = new Map<string, ExpectedValue>();
    for (const ev of evPortfolio.items) {
      map.set(ev.certNumber, ev);
    }
    return map;
  }, [showEV, evPortfolio]);

  const filteredAndSortedItems = useMemo(
    () => filterAndSortItems(items, {
      debouncedSearch,
      showAll,
      filterTab,
      sortKey,
      sortDir,
      evMap,
      pinnedIds: selection.pinnedIds,
    }),
    [items, debouncedSearch, sortKey, sortDir, evMap, showAll, filterTab, selection.pinnedIds],
  );

  const filteredTotals = useMemo(() => computeTotals(filteredAndSortedItems), [filteredAndSortedItems]);

  function toggleAll() {
    const visibleIds = filteredAndSortedItems.map(i => i.purchase.id);
    const allVisibleSelected = visibleIds.length > 0 && visibleIds.every(id => selection.selected.has(id));
    selection.setSelected(prev => {
      const next = new Set(prev);
      if (allVisibleSelected) {
        for (const id of visibleIds) next.delete(id);
      } else {
        for (const id of visibleIds) next.add(id);
      }
      return next;
    });
  }

  function openSaleModal(saleItems: AgingItem[]) {
    setSaleModalItems(saleItems);
    setSaleModalOpen(true);
  }

  function closeSaleModal() {
    setSaleModalOpen(false);
    setSaleModalItems([]);
  }

  const handleDelete = useCallback(async (item: AgingItem) => {
    const name = item.purchase.cardName || 'this card';
    if (!window.confirm(`Delete "${name}"? This will permanently remove it and any associated sale.`)) return;
    try {
      await api.deletePurchase(item.purchase.campaignId, item.purchase.id);
      toast.success(`Deleted "${name}"`);
      if (selection.expandedId === item.purchase.id) selection.setExpandedId(null);
      invalidateInventory({ sellSheet: true });
    } catch (err) {
      toast.error(getErrorMessage(err, 'Failed to delete purchase'));
    }
  }, [toast, selection.expandedId, invalidateInventory, selection.setExpandedId]);

  const { totalCost, totalMarket, totalPL } = filteredTotals;
  const fullInventoryTotals = summary;

  return {
    scrollContainerRef, mobileScrollRef,
    selected: selection.selected, setSelected: selection.setSelected,
    expandedId: selection.expandedId, setExpandedId: selection.setExpandedId,
    saleModalOpen, saleModalItems,
    hintTarget: pricingActions.hintTarget, setHintTarget: pricingActions.setHintTarget,
    priceTarget: pricingActions.priceTarget, setPriceTarget: pricingActions.setPriceTarget,
    flagTarget: pricingActions.flagTarget, setFlagTarget: pricingActions.setFlagTarget,
    flagSubmitting: pricingActions.flagSubmitting,
    fixMatchTarget: dhActions.fixMatchTarget, setFixMatchTarget: dhActions.setFixMatchTarget,
    sortKey, sortDir, searchQuery, setSearchQuery,
    filterTab, setFilterTab: chooseFilterTab,
    showAll, setShowAll, debouncedSearch,
    reviewStats, tabCounts, showEV: !!showEV, evPortfolio, evMap,
    filteredAndSortedItems,
    totalCost, totalMarket, totalPL, fullInventoryTotals,
    handleSort, handleReviewed,
    handleResolveFlag: pricingActions.handleResolveFlag,
    handleApproveDHPush: dhActions.handleApproveDHPush,
    handleDismiss: dhActions.handleDismiss, handleUndismiss: dhActions.handleUndismiss,
    handleListOnDH: dhActions.handleListOnDH,
    dhListingInFlight: dhActions.dhListingInFlight, dhListedOptimistic: dhActions.dhListedOptimistic,
    handleBulkListOnDH: dhActions.handleBulkListOnDH,
    handleFlagSubmit: pricingActions.handleFlagSubmit,
    handleDelete,
    toggleSelect: selection.toggleSelect, toggleAll, toggleExpand: selection.toggleExpand,
    openSaleModal, closeSaleModal,
    handleFixPricing: pricingActions.handleFixPricing,
    handleFixDHMatch: dhActions.handleFixDHMatch, handleFixDHMatchSaved: dhActions.handleFixDHMatchSaved,
    handleUnmatchDH: dhActions.handleUnmatchDH, handleRetryDHMatch: dhActions.handleRetryDHMatch,
    dhRetryInFlight: dhActions.dhRetryInFlight,
    handleSetPrice: pricingActions.handleSetPrice, handlePriceSaved: pricingActions.handlePriceSaved,
    handleInlinePriceSave: pricingActions.handleInlinePriceSave, handleHintSaved: pricingActions.handleHintSaved,
    pinnedIds: selection.pinnedIds, handleDeselectMissingCL: selection.handleDeselectMissingCL, handleHighlightMissingCL: selection.handleHighlightMissingCL,
    toast,
  };
}
