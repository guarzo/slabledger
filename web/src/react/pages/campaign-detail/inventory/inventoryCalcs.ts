import type { AgingItem, ReviewStats, ExpectedValue } from '../../../../types/campaigns';
import { costBasis, bestPrice, unrealizedPL, getReviewStatus, reviewUrgencySort, isCardShowCandidate } from './utils';
import type { SortKey, SortDir } from './utils';

const EXCEPTION_STATUSES = ['large_gap', 'no_data', 'flagged'] as const;

export interface TabCounts {
  needs_attention: number;
  ai_suggestion: number;
  card_show: number;
  in_hand: number;
  ready_to_list: number;
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
  const counts: TabCounts = { needs_attention: 0, ai_suggestion: 0, card_show: 0, in_hand: 0, ready_to_list: 0, all: items.length };
  let totalCost = 0;
  let totalMarket = 0;
  for (const item of items) {
    if (item.hasOpenFlag) stats.flagged++;
    if (item.purchase.reviewedAt) stats.reviewed++;
    else stats.needsReview++;
    if (item.daysHeld >= 60) stats.aging60d++;

    const status = getReviewStatus(item);
    if ((EXCEPTION_STATUSES as readonly string[]).includes(status) || isDHHeld(item)) {
      counts.needs_attention++;
    }
    if ((item.purchase.aiSuggestedPriceCents ?? 0) > 0) counts.ai_suggestion++;
    if (item.purchase.receivedAt && isCardShowCandidate(item)) counts.card_show++;
    if (item.purchase.receivedAt) counts.in_hand++;
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

// isReadyToList: received (intake complete) but not yet listed on DH.
// Used to surface cert-intake items that have been pushed to DH inventory
// so a human can review price before flipping them live.
export function isReadyToList(item: AgingItem): boolean {
  return !!item.purchase.receivedAt && item.purchase.dhStatus !== 'listed';
}

export type FilterTab = 'needs_attention' | 'ai_suggestion' | 'sell_sheet' | 'all' | 'card_show' | 'in_hand' | 'ready_to_list';

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
        if (filterTab === 'needs_attention') {
          return (EXCEPTION_STATUSES as readonly string[]).includes(getReviewStatus(i)) || isDHHeld(i);
        }
        if (filterTab === 'ai_suggestion') return (i.purchase.aiSuggestedPriceCents ?? 0) > 0;
        if (filterTab === 'card_show') return !!i.purchase.receivedAt && isCardShowCandidate(i);
        if (filterTab === 'in_hand') return !!i.purchase.receivedAt;
        if (filterTab === 'ready_to_list') return isReadyToList(i);
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
