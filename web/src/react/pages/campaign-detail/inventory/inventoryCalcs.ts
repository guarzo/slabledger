import type { AgingItem, ReviewStats, ExpectedValue } from '../../../../types/campaigns';
import { costBasis, bestPrice, unrealizedPL, getReviewStatus, reviewUrgencySort } from './utils';
import type { SortKey, SortDir } from './utils';

const EXCEPTION_STATUSES = ['large_gap', 'no_data', 'flagged'] as const;

export interface TabCounts {
  needs_attention: number;
  in_hand: number;
  ready_to_list: number;
  awaiting_intake: number;
  all: number;
}

export interface SummaryStats {
  totalCost: number;
  totalMarket: number;
  totalPL: number;
}

export interface InventoryMeta {
  reviewStats: ReviewStats;
  tabCounts: TabCounts;
  summary: SummaryStats;
}

export function computeInventoryMeta(items: AgingItem[]): InventoryMeta {
  const stats: ReviewStats = { total: items.length, needsReview: 0, reviewed: 0, flagged: 0, aging60d: 0 };
  const counts: TabCounts = { needs_attention: 0, in_hand: 0, ready_to_list: 0, awaiting_intake: 0, all: items.length };
  let totalCost = 0;
  let totalMarket = 0;
  for (const item of items) {
    if (item.hasOpenFlag) stats.flagged++;
    if (item.purchase.reviewedAt) stats.reviewed++;
    else if (item.purchase.receivedAt) stats.needsReview++;
    if (item.daysHeld >= 60) stats.aging60d++;

    const status = getReviewStatus(item);
    if (needsAttention(item, status)) {
      counts.needs_attention++;
    }
    if (item.purchase.receivedAt) counts.in_hand++;
    else counts.awaiting_intake++;
    if (isReadyToList(item)) counts.ready_to_list++;

    totalCost += costBasis(item.purchase);
    totalMarket += bestPrice(item);
  }
  return {
    reviewStats: stats,
    tabCounts: counts,
    summary: { totalCost, totalMarket, totalPL: totalMarket > 0 ? totalMarket - totalCost : 0 },
  };
}

export function isDHHeld(item: AgingItem): boolean {
  return item.purchase.dhPushStatus === 'held';
}

export function needsAttention(item: AgingItem, status = getReviewStatus(item)): boolean {
  // Pre-intake cards can't be meaningfully acted on yet — don't flag them as needing attention.
  if (!item.purchase.receivedAt) return false;
  if ((EXCEPTION_STATUSES as readonly string[]).includes(status)) return true;
  if (isDHHeld(item)) return true;
  if ((item.purchase.aiSuggestedPriceCents ?? 0) > 0) return true;
  return false;
}

// isReadyToList: received (intake complete), pushed to DH inventory, not yet
// listed, AND has a reviewed price. Without a reviewed price the "List on DH"
// action would 409, so those items belong in `needsPriceReview` instead.
export function isReadyToList(item: AgingItem): boolean {
  return (
    !!item.purchase.receivedAt &&
    !!item.purchase.dhInventoryId &&
    item.purchase.dhStatus !== 'listed' &&
    (item.purchase.reviewedPriceCents ?? 0) > 0
  );
}

// needsPriceReview: received, pushed to DH, not listed, but no reviewed price.
// Drives the "Set price" row button (which expands the row to reveal the
// PriceDecisionBar) rather than a "List on DH" button that would hit a 409.
export function needsPriceReview(item: AgingItem): boolean {
  return (
    !!item.purchase.receivedAt &&
    !!item.purchase.dhInventoryId &&
    item.purchase.dhStatus !== 'listed' &&
    (item.purchase.reviewedPriceCents ?? 0) === 0
  );
}

// wasUnlistedFromDH: the DH reconciler detected this item was deleted
// from DH's authoritative inventory snapshot. Drives the
// "Re-list (removed from DH)" row badge. Clears on successful re-list.
export function wasUnlistedFromDH(item: AgingItem): boolean {
  return !!item.purchase.dhUnlistedDetectedAt;
}

export type FilterTab = 'needs_attention' | 'sell_sheet' | 'all' | 'in_hand' | 'ready_to_list' | 'awaiting_intake';

export function filterAndSortItems(
  items: AgingItem[],
  opts: {
    debouncedSearch: string;
    showAll: boolean;
    filterTab: FilterTab;
    sellSheetHas: (id: string) => boolean;
    sortKey: SortKey;
    sortDir: SortDir;
    evMap: Map<string, ExpectedValue>;
  },
): AgingItem[] {
  const { debouncedSearch, showAll, filterTab, sellSheetHas, sortKey, sortDir, evMap } = opts;
  let result = items;

  // Search always overrides: if search query is set, search all items regardless of tab
  if (debouncedSearch.trim()) {
    const q = debouncedSearch.toLowerCase();
    result = result.filter(i =>
      i.purchase.cardName.toLowerCase().includes(q) ||
      (i.purchase.certNumber && i.purchase.certNumber.toLowerCase().includes(q)) ||
      (i.purchase.setName && i.purchase.setName.toLowerCase().includes(q))
    );
  } else if (!showAll) {
    // Filter by active tab using getReviewStatus
    if (filterTab === 'sell_sheet') {
      result = result.filter(i => sellSheetHas(i.purchase.id) && !!i.purchase.receivedAt);
    } else if (filterTab !== 'all') {
      result = result.filter(i => {
        if (filterTab === 'needs_attention') return needsAttention(i);
        if (filterTab === 'in_hand') return !!i.purchase.receivedAt;
        if (filterTab === 'ready_to_list') return isReadyToList(i);
        if (filterTab === 'awaiting_intake') return !i.purchase.receivedAt;
        return false;
      });
    }
  }

  // Sort: when !showAll and no search, use queue urgency sort; otherwise use user-selected sort
  if (!showAll && !debouncedSearch.trim()) {
    return [...result].sort(reviewUrgencySort);
  }

  const dir = sortDir === 'asc' ? 1 : -1;
  return [...result].sort((a, b) => {
    switch (sortKey) {
      case 'name':
        return dir * a.purchase.cardName.localeCompare(b.purchase.cardName);
      case 'grade':
        return dir * (a.purchase.gradeValue - b.purchase.gradeValue);
      case 'cost':
        return dir * (costBasis(a.purchase) - costBasis(b.purchase));
      case 'market': {
        const ma = bestPrice(a);
        const mb = bestPrice(b);
        return dir * (ma - mb);
      }
      case 'pl': {
        const pa = unrealizedPL(costBasis(a.purchase), a) ?? -Infinity;
        const pb = unrealizedPL(costBasis(b.purchase), b) ?? -Infinity;
        return dir * (pa - pb);
      }
      case 'days':
        return dir * (a.daysHeld - b.daysHeld);
      case 'ev': {
        const ea = evMap.get(a.purchase.certNumber)?.evCents ?? -Infinity;
        const eb = evMap.get(b.purchase.certNumber)?.evCents ?? -Infinity;
        return dir * (ea - eb);
      }
      default:
        return 0;
    }
  });
}
