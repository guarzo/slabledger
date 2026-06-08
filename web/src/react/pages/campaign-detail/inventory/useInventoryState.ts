import { useEffect, useMemo, useRef, useState, useCallback } from 'react';
import { useQueryClient } from '@tanstack/react-query';
import type { AgingItem } from '../../../../types/campaigns';
import { useDebounce } from '../../../hooks/useDebounce';
import { useToast } from '../../../contexts/ToastContext';
import { queryKeys } from '../../../queries/queryKeys';
import { api } from '../../../../js/api';
import { getErrorMessage } from '../../../utils/formatters';
import type { SortKey, SortDir } from './utils';
import { computeInventoryMeta, computeTotals, filterAndSortItems } from './inventoryCalcs';
import type { FilterTab, PriceBand } from './inventoryCalcs';
import { useInventorySelection } from './useInventorySelection';
import { useDHActions } from './useDHActions';
import { usePricingActions } from './usePricingActions';

export function useInventoryState(items: AgingItem[], campaignId?: string) {
  const queryClient = useQueryClient();
  const toast = useToast();
  const selection = useInventorySelection();
  const [sortKey, setSortKey] = useState<SortKey>('name');
  const [sortDir, setSortDir] = useState<SortDir>('asc');
  const [searchQuery, setSearchQuery] = useState('');
  const [filterTab, setFilterTab] = useState<FilterTab>('all');
  const [priceBand, setPriceBand] = useState<PriceBand>('all');
  const userTabChosenRef = useRef(false);
  const debouncedSearch = useDebounce(searchQuery, 300);
  const scrollContainerRef = useRef<HTMLDivElement>(null);
  const mobileScrollRef = useRef<HTMLDivElement>(null);
  const [saleModalOpen, setSaleModalOpen] = useState(false);
  const [saleModalItems, setSaleModalItems] = useState<AgingItem[]>([]);
  // Single-item inline sale recording — expands the row in place rather than
  // opening a modal, so the operator keeps row context (price signals,
  // comp panel) visible while recording.
  const [inlineSaleId, setInlineSaleId] = useState<string | null>(null);

  const invalidateInventory = useCallback(() => {
    if (campaignId) {
      queryClient.invalidateQueries({ queryKey: queryKeys.campaigns.inventory(campaignId) });
    } else {
      queryClient.invalidateQueries({ predicate: (query) => query.queryKey[0] === 'campaigns' && query.queryKey[2] === 'inventory' });
    }
    queryClient.invalidateQueries({ queryKey: queryKeys.portfolio.globalInventory });
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

  const { reviewStats, tabCounts, priceBandCounts, summary } = useMemo(
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

  // Reset scroll + collapse expanded row on sort/filter change.
  // Desktop now uses window scroll (the inner max-h container was removed); mobile
  // keeps its inner scroll container for the per-card list.
  useEffect(() => {
    selection.setExpandedId(null);
    selection.setPinnedIds(prev => prev.size > 0 ? new Set() : prev);
    if (typeof window !== 'undefined') window.scrollTo({ top: 0 });
    mobileScrollRef.current?.scrollTo({ top: 0 });
  }, [sortKey, sortDir, debouncedSearch, filterTab, priceBand]); // eslint-disable-line react-hooks/exhaustive-deps

  const filteredAndSortedItems = useMemo(
    () => filterAndSortItems(items, {
      debouncedSearch,
      filterTab,
      sortKey,
      sortDir,
      pinnedIds: selection.pinnedIds,
      priceBand,
    }),
    [items, debouncedSearch, sortKey, sortDir, filterTab, selection.pinnedIds, priceBand],
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

  // Single-item inline sale: expand the row in place and switch its
  // expanded panel into "recording sale" mode.
  function startInlineSale(saleItem: AgingItem) {
    selection.setExpandedId(saleItem.purchase.id);
    setInlineSaleId(saleItem.purchase.id);
  }

  function cancelInlineSale() {
    setInlineSaleId(null);
  }

  function handleInlineSaleSuccess() {
    setInlineSaleId(null);
    selection.setExpandedId(null);
    invalidateInventory();
  }

  const handleDelete = useCallback(async (item: AgingItem) => {
    const name = item.purchase.cardName || 'this card';
    if (!window.confirm(`Delete "${name}"? This will permanently remove it and any associated sale.`)) return;
    try {
      await api.deletePurchase(item.purchase.campaignId, item.purchase.id);
      toast.success(`Deleted "${name}"`);
      if (selection.expandedId === item.purchase.id) selection.setExpandedId(null);
      invalidateInventory();
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
    priceBand, setPriceBand,
    debouncedSearch,
    reviewStats, tabCounts, priceBandCounts,
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
    inlineSaleId, startInlineSale, cancelInlineSale, handleInlineSaleSuccess,
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
