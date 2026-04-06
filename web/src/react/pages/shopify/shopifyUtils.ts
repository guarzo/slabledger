import type { ShopifyPriceSyncMatch } from '../../../types/campaigns';
import type { SyncFilter, SyncSort, FilterCounts, SortFn } from './shopifyTypes';

export function computeFilterCounts(mismatches: ShopifyPriceSyncMatch[]): FilterCounts {
  let price_drop = 0, price_increase = 0, no_market_data = 0;
  for (const m of mismatches) {
    if (m.recommendedPriceCents < m.currentPriceCents) price_drop++;
    if (m.recommendedPriceCents > m.currentPriceCents) price_increase++;
    if (!m.hasMarketData) no_market_data++;
  }
  return { all: mismatches.length, price_drop, price_increase, no_market_data };
}

export function applyFilter(mismatches: ShopifyPriceSyncMatch[], filter: SyncFilter): ShopifyPriceSyncMatch[] {
  switch (filter) {
    case 'price_drop': return mismatches.filter(m => m.recommendedPriceCents < m.currentPriceCents);
    case 'price_increase': return mismatches.filter(m => m.recommendedPriceCents > m.currentPriceCents);
    case 'no_market_data': return mismatches.filter(m => !m.hasMarketData);
    default: return mismatches;
  }
}

export function getSortFn(sort: SyncSort): SortFn {
  return (a, b) => {
    switch (sort) {
      case 'value': return Math.max(b.currentPriceCents, b.recommendedPriceCents) - Math.max(a.currentPriceCents, a.recommendedPriceCents);
      case 'margin': return (b.recommendedPriceCents - b.costBasisCents) - (a.recommendedPriceCents - a.costBasisCents);
      case 'name': return a.cardName.localeCompare(b.cardName);
      default: return Math.abs(b.recommendedPriceCents - b.currentPriceCents) - Math.abs(a.recommendedPriceCents - a.currentPriceCents);
    }
  };
}
