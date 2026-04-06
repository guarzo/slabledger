import type { ShopifyPriceSyncMatch } from '../../../types/campaigns';
import type { SyncFilter, SyncSort, FilterCounts, SortFn } from './shopifyTypes';

export function computeFilterCounts(mismatches: ShopifyPriceSyncMatch[]): FilterCounts {
  return {
    all: mismatches.length,
    price_drop: mismatches.filter(m => m.recommendedPriceCents < m.currentPriceCents).length,
    price_increase: mismatches.filter(m => m.recommendedPriceCents > m.currentPriceCents).length,
    no_market_data: mismatches.filter(m => !m.hasMarketData).length,
  };
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
