import { useCallback, useEffect, useMemo } from 'react';
import type { LiquidationPreviewItem } from '../../../types/liquidation';
import { useLocalStorage } from '../../hooks/useLocalStorage';

interface UseRepriceSelectionArgs {
  items: LiquidationPreviewItem[];
  /** Wait until the liquidation preview query has resolved before
   *  reconciling persisted selection against the current items set. */
  isLoading: boolean;
}

/**
 * Persisted row selection for the Reprice page. Stored as an array in
 * localStorage (Sets don't JSON-serialize) and exposed as a Set so
 * callers keep using the natural `.has` / `.add` / `.delete` shape.
 *
 * Reconciliation: when the query resolves, drop any persisted IDs that
 * are no longer in the current item cohort so `selected.size` and the
 * "N skipped" confirm-message line reflect only actionable rows.
 */
export function useRepriceSelection({ items, isLoading }: UseRepriceSelectionArgs) {
  const [selectedArr, setSelectedArr] = useLocalStorage<string[]>('reprice.selected', []);
  const selected = useMemo(() => new Set(selectedArr), [selectedArr]);

  const setSelected = useCallback(
    (updater: Set<string> | ((prev: Set<string>) => Set<string>)) => {
      setSelectedArr(prev => {
        const next = typeof updater === 'function' ? updater(new Set(prev)) : updater;
        return Array.from(next);
      });
    },
    [setSelectedArr],
  );

  useEffect(() => {
    if (isLoading) return;
    const itemIds = new Set(items.map(i => i.purchaseId));
    setSelectedArr(prev => {
      const filtered = prev.filter(id => itemIds.has(id));
      return filtered.length === prev.length ? prev : filtered;
    });
  }, [items, isLoading, setSelectedArr]);

  const toggleSelect = useCallback((id: string) => {
    setSelected(prev => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });
  }, [setSelected]);

  const selectAll = useCallback(
    () => setSelected(new Set(items.map(i => i.purchaseId))),
    [items, setSelected],
  );
  const deselectAll = useCallback(() => setSelected(new Set()), [setSelected]);
  const acceptItem = useCallback(
    (id: string) => setSelected(prev => new Set(prev).add(id)),
    [setSelected],
  );

  return { selected, setSelected, toggleSelect, selectAll, deselectAll, acceptItem };
}
