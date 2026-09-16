import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import type { AgingItem } from '../../../types/campaigns';
import type { ShowEvaluation } from '../../../types/showprep';
import { buildPriceReview, type PriceDraft, type PriceReviewFilter, type PriceReviewSort } from './priceReviewModel';

export type { PriceDraft, PriceReviewFilter, PriceReviewSort } from './priceReviewModel';

// Owned by the inventory view switch, not by the conditional workspace mount.
export function usePriceReviewState(items: AgingItem[], evaluations: Record<string, ShowEvaluation>, search: string) {
  const [filter, setFilter] = useState<PriceReviewFilter>('all');
  const [sort, setSort] = useState<PriceReviewSort>('attention');
  const [descending, setDescending] = useState(true);
  const [activeId, setActiveId] = useState<string | null>(null);
  const [drafts, setDrafts] = useState<Record<string, PriceDraft>>({});
  const { rows, counts } = useMemo(() => buildPriceReview(items, evaluations, { search, filter, sort, descending }),
    [items, evaluations, search, filter, sort, descending]);
  const ids = useMemo(() => rows.map(item => item.purchase.id), [rows]);
  // Retain only navigation identity, never an old purchase or authorization token.
  const anchorQueue = useRef<string[]>([]);
  useEffect(() => {
    if (activeId === null && ids.length) setActiveId(ids[0]);
    if (activeId === null || ids.includes(activeId)) anchorQueue.current = ids;
  }, [ids, activeId]);
  const focus = useCallback((id: string) => setActiveId(id), []);
  const move = useCallback((delta: -1 | 1) => {
    if (!ids.length) return;
    const current = activeId === null ? -1 : ids.indexOf(activeId);
    if (current >= 0) {
      setActiveId(ids[Math.max(0, Math.min(ids.length - 1, current + delta))]);
      return;
    }
    const anchor = activeId === null ? -1 : anchorQueue.current.indexOf(activeId);
    for (let n = anchor + delta; anchor >= 0 && n >= 0 && n < anchorQueue.current.length; n += delta) {
      if (ids.includes(anchorQueue.current[n])) { setActiveId(anchorQueue.current[n]); return; }
    }
    setActiveId(delta === 1 ? ids[0] : ids[ids.length - 1]);
  }, [activeId, ids]);
  const setDraft = useCallback((id: string, draft: PriceDraft) => setDrafts(old => ({ ...old, [id]: draft })), []);
  const clearDraft = useCallback((id: string) => setDrafts(old => {
    const next = { ...old }; delete next[id]; return next;
  }), []);
  return { filter, setFilter, sort, setSort, descending, setDescending, rows, counts, activeId, focus, move,
    drafts, setDraft, clearDraft, outsideFilter: activeId !== null && !ids.includes(activeId) };
}
