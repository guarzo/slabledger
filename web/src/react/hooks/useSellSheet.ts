import { useCallback, useMemo } from 'react';
import { useLocalStorage } from './useLocalStorage';

const STORAGE_KEY = 'sellSheetIds';

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
}

export function useSellSheet(): SellSheetHook {
  const [storedIds, setStoredIds] = useLocalStorage<string[]>(STORAGE_KEY, []);

  const itemsSet = useMemo(() => new Set(storedIds), [storedIds]);

  const add = useCallback((ids: string[]) => {
    setStoredIds(prev => {
      const set = new Set(prev);
      for (const id of ids) set.add(id);
      return Array.from(set);
    });
  }, [setStoredIds]);

  const remove = useCallback((ids: string[]) => {
    setStoredIds(prev => {
      const toRemove = new Set(ids);
      return prev.filter(id => !toRemove.has(id));
    });
  }, [setStoredIds]);

  const clear = useCallback(() => {
    setStoredIds([]);
  }, [setStoredIds]);

  const has = useCallback((id: string) => itemsSet.has(id), [itemsSet]);

  return { items: itemsSet, add, remove, clear, has, count: itemsSet.size };
}
