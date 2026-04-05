import { useCallback, useMemo, useEffect, useRef } from 'react';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { api } from '../../js/api';
import { queryKeys } from '../queries/queryKeys';

const LEGACY_STORAGE_KEY = 'sellSheetIds';

export interface SellSheetHook {
  /** Set of purchase IDs currently on the sell sheet */
  items: Set<string>;
  /** Add purchase IDs to the sell sheet */
  add: (ids: string[]) => void;
  /** Remove purchase IDs from the sell sheet */
  remove: (ids: string[]) => void;
  /** Clear all items from the sell sheet */
  clear: () => void;
  /** Check if a purchase ID is on the sell sheet */
  has: (id: string) => boolean;
  /** Number of items on the sell sheet */
  count: number;
  /** Whether the initial load is in progress */
  isLoading: boolean;
}

export function useSellSheet(): SellSheetHook {
  const queryClient = useQueryClient();
  const migratedRef = useRef(false);

  const { data: ids = [], isLoading } = useQuery({
    queryKey: queryKeys.portfolio.sellSheetItems,
    queryFn: async () => {
      const res = await api.getSellSheetItems();
      return res.purchaseIds;
    },
    staleTime: 30_000,
  });

  const itemsSet = useMemo(() => new Set(ids), [ids]);

  // One-time migration from localStorage
  useEffect(() => {
    if (isLoading || migratedRef.current) return;
    migratedRef.current = true;
    try {
      const raw = localStorage.getItem(LEGACY_STORAGE_KEY);
      if (!raw) return;
      const legacyIds: string[] = JSON.parse(raw);
      if (!Array.isArray(legacyIds) || legacyIds.length === 0) return;
      // Only migrate if server is empty (avoid duplicating on re-renders)
      if (ids.length > 0) {
        localStorage.removeItem(LEGACY_STORAGE_KEY);
        return;
      }
      api.addSellSheetItems(legacyIds).then(() => {
        localStorage.removeItem(LEGACY_STORAGE_KEY);
        queryClient.invalidateQueries({ queryKey: queryKeys.portfolio.sellSheetItems });
      });
    } catch {
      // Corrupted localStorage — just remove it
      localStorage.removeItem(LEGACY_STORAGE_KEY);
    }
  }, [isLoading, ids, queryClient]);

  const addMutation = useMutation({
    mutationFn: (newIds: string[]) => api.addSellSheetItems(newIds),
    onMutate: async (newIds) => {
      await queryClient.cancelQueries({ queryKey: queryKeys.portfolio.sellSheetItems });
      const prev = queryClient.getQueryData<string[]>(queryKeys.portfolio.sellSheetItems) ?? [];
      const merged = Array.from(new Set([...prev, ...newIds]));
      queryClient.setQueryData(queryKeys.portfolio.sellSheetItems, merged);
      return { prev };
    },
    onError: (_err, _vars, context) => {
      if (context?.prev) queryClient.setQueryData(queryKeys.portfolio.sellSheetItems, context.prev);
    },
    onSettled: () => {
      queryClient.invalidateQueries({ queryKey: queryKeys.portfolio.sellSheetItems });
    },
  });

  const removeMutation = useMutation({
    mutationFn: (removeIds: string[]) => api.removeSellSheetItems(removeIds),
    onMutate: async (removeIds) => {
      await queryClient.cancelQueries({ queryKey: queryKeys.portfolio.sellSheetItems });
      const prev = queryClient.getQueryData<string[]>(queryKeys.portfolio.sellSheetItems) ?? [];
      const removeSet = new Set(removeIds);
      queryClient.setQueryData(queryKeys.portfolio.sellSheetItems, prev.filter(id => !removeSet.has(id)));
      return { prev };
    },
    onError: (_err, _vars, context) => {
      if (context?.prev) queryClient.setQueryData(queryKeys.portfolio.sellSheetItems, context.prev);
    },
    onSettled: () => {
      queryClient.invalidateQueries({ queryKey: queryKeys.portfolio.sellSheetItems });
    },
  });

  const clearMutation = useMutation({
    mutationFn: () => api.clearSellSheetItems(),
    onMutate: async () => {
      await queryClient.cancelQueries({ queryKey: queryKeys.portfolio.sellSheetItems });
      const prev = queryClient.getQueryData<string[]>(queryKeys.portfolio.sellSheetItems) ?? [];
      queryClient.setQueryData(queryKeys.portfolio.sellSheetItems, []);
      return { prev };
    },
    onError: (_err, _vars, context) => {
      if (context?.prev) queryClient.setQueryData(queryKeys.portfolio.sellSheetItems, context.prev);
    },
    onSettled: () => {
      queryClient.invalidateQueries({ queryKey: queryKeys.portfolio.sellSheetItems });
    },
  });

  const add = useCallback((newIds: string[]) => addMutation.mutate(newIds), [addMutation]);
  const remove = useCallback((removeIds: string[]) => removeMutation.mutate(removeIds), [removeMutation]);
  const clear = useCallback(() => clearMutation.mutate(), [clearMutation]);
  const has = useCallback((id: string) => itemsSet.has(id), [itemsSet]);

  return { items: itemsSet, add, remove, clear, has, count: itemsSet.size, isLoading };
}
