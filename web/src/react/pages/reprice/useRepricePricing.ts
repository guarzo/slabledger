import { useEffect, useState } from 'react';
import type { LiquidationPreviewItem } from '../../../types/liquidation';
import { useLocalStorage } from '../../hooks/useLocalStorage';
import { parseDollarsToCents } from './parseDollarsToCents';

interface UseRepricePricingArgs {
  items: LiquidationPreviewItem[];
  /** True while the liquidation preview query is in flight. Reconcile
   *  waits until the query resolves (success OR success-with-empty-rows)
   *  so persisted state isn't wiped during first paint. */
  isLoading: boolean;
}

/**
 * Pricing state for the Reprice page.
 *
 * - `finalPriceInputs` (persisted to localStorage) is the source of truth
 *   for the live textbox value while it's populated. This means
 *   `⌘+Enter` before blur applies the user's current typed value, not a
 *   stale committed value from an earlier blur.
 * - `finalPrices` (in-memory) is set on blur or pill click. It's kept as
 *   a parsed cache so getFinalPrice doesn't re-parse for unblurred-and-
 *   submitted rows that happen to be empty (in which case the input is
 *   '' and we fall through to committed).
 *
 * Precedence in getFinalPrice: live input (if non-empty and parseable)
 *   → committed → suggested. A non-empty but malformed input returns 0
 *   so the row is treated as skipped by `getFinalPrice(item) > 0`
 *   filters in the page.
 */
export function useRepricePricing({ items, isLoading }: UseRepricePricingArgs) {
  const [finalPriceInputs, setFinalPriceInputs] = useLocalStorage<Record<string, string>>('reprice.priceInputs', {});
  const [finalPrices, setFinalPrices] = useState<Record<string, number>>({});

  useEffect(() => {
    if (isLoading) return;
    const itemIds = new Set(items.map(i => i.purchaseId));
    setFinalPriceInputs(prev => {
      const filtered: Record<string, string> = {};
      let dropped = 0;
      for (const [id, val] of Object.entries(prev)) {
        if (itemIds.has(id)) filtered[id] = val;
        else dropped++;
      }
      return dropped === 0 ? prev : filtered;
    });
  }, [items, isLoading, setFinalPriceInputs]);

  function getFinalPrice(item: LiquidationPreviewItem): number {
    const inputVal = finalPriceInputs[item.purchaseId];
    if (inputVal != null && inputVal !== '') {
      const parsed = parseDollarsToCents(inputVal);
      return parsed ?? 0;
    }
    const committed = finalPrices[item.purchaseId];
    if (committed != null) return committed;
    return item.suggestedPriceCents;
  }

  function setPillPrice(id: string, cents: number) {
    setFinalPrices(prev => ({ ...prev, [id]: cents }));
    setFinalPriceInputs(prev => ({ ...prev, [id]: cents > 0 ? (cents / 100).toFixed(2) : '' }));
  }

  function handleInputChange(id: string, val: string) {
    setFinalPriceInputs(prev => ({ ...prev, [id]: val }));
  }

  function handleInputBlur(id: string) {
    const val = finalPriceInputs[id] ?? '';
    if (val === '' || val === '.') {
      setFinalPrices(prev => ({ ...prev, [id]: 0 }));
      setFinalPriceInputs(prev => ({ ...prev, [id]: '' }));
      return;
    }
    const cents = parseDollarsToCents(val);
    if (cents != null) {
      setFinalPrices(prev => ({ ...prev, [id]: cents }));
      setFinalPriceInputs(prev => ({ ...prev, [id]: cents > 0 ? (cents / 100).toFixed(2) : '' }));
    }
  }

  function resetPricing() {
    setFinalPrices({});
    setFinalPriceInputs({});
  }

  return {
    finalPriceInputs,
    getFinalPrice,
    setPillPrice,
    handleInputChange,
    handleInputBlur,
    resetPricing,
  };
}
