import { useState, useCallback, type Dispatch, type SetStateAction } from 'react';

export interface InventorySelection {
  selected: ReadonlySet<string>;
  setSelected: Dispatch<SetStateAction<Set<string>>>;
  pinnedIds: ReadonlySet<string>;
  setPinnedIds: Dispatch<SetStateAction<Set<string>>>;
  expandedId: string | null;
  setExpandedId: Dispatch<SetStateAction<string | null>>;
  toggleSelect: (purchaseId: string) => void;
  toggleExpand: (purchaseId: string) => void;
  handleDeselectMissingCL: (purchaseIds: string[]) => void;
  handleHighlightMissingCL: (purchaseIds: string[]) => void;
}

export function useInventorySelection(): InventorySelection {
  const [selected, setSelected] = useState<Set<string>>(new Set());
  const [pinnedIds, setPinnedIds] = useState<Set<string>>(new Set());
  const [expandedId, setExpandedId] = useState<string | null>(null);

  const handleDeselectMissingCL = useCallback((purchaseIds: string[]) => {
    setSelected(prev => {
      const next = new Set(prev);
      for (const id of purchaseIds) next.delete(id);
      return next;
    });
  }, []);

  const handleHighlightMissingCL = useCallback((purchaseIds: string[]) => {
    setPinnedIds(new Set(purchaseIds));
  }, []);

  const toggleSelect = useCallback((purchaseId: string) => {
    setSelected(prev => {
      const next = new Set(prev);
      if (next.has(purchaseId)) next.delete(purchaseId);
      else next.add(purchaseId);
      return next;
    });
  }, []);

  const toggleExpand = useCallback((purchaseId: string) => {
    setExpandedId(prev => prev === purchaseId ? null : purchaseId);
  }, []);

  return {
    selected, setSelected,
    pinnedIds, setPinnedIds,
    expandedId, setExpandedId,
    toggleSelect,
    toggleExpand,
    handleDeselectMissingCL,
    handleHighlightMissingCL,
  };
}
