import type { AgingItem, ReviewStats, ExpectedValue } from '../../../../types/campaigns';
import { costBasis, bestPrice, unrealizedPL, getReviewStatus, reviewUrgencySort, isCardShowCandidate } from './utils';
import type { SortKey, SortDir } from './utils';

const EXCEPTION_STATUSES = ['large_gap', 'no_data', 'flagged'] as const;

export interface TabCounts {
  needs_attention: number;
  ai_suggestion: number;
  card_show: number;
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
  const stats: ReviewStats = { total: items.length, needsReview: 0, reviewed: 0, flagged: 0 };
  const counts: TabCounts = { needs_attention: 0, ai_suggestion: 0, card_show: 0, all: items.length };
  let totalCost = 0;
  let totalMarket = 0;
  for (const item of items) {
    if (item.hasOpenFlag) stats.flagged++;
    if (item.purchase.reviewedAt) stats.reviewed++;
    else stats.needsReview++;

    const status = getReviewStatus(item);
    if ((EXCEPTION_STATUSES as readonly string[]).includes(status) || isDHHeld(item)) {
      counts.needs_attention++;
    }
    if ((item.purchase.aiSuggestedPriceCents ?? 0) > 0) counts.ai_suggestion++;
    if (isCardShowCandidate(item)) counts.card_show++;

    totalCost += costBasis(item.purchase);
    if (item.currentMarket) totalMarket += bestPrice(item.currentMarket);
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

export type FilterTab = 'needs_attention' | 'ai_suggestion' | 'sell_sheet' | 'all' | 'card_show';

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
      result = result.filter(i => sellSheetHas(i.purchase.id));
    } else if (filterTab !== 'all') {
      result = result.filter(i => {
        if (filterTab === 'needs_attention') {
          return (EXCEPTION_STATUSES as readonly string[]).includes(getReviewStatus(i)) || isDHHeld(i);
        }
        if (filterTab === 'ai_suggestion') return (i.purchase.aiSuggestedPriceCents ?? 0) > 0;
        if (filterTab === 'card_show') return isCardShowCandidate(i);
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
        const ma = a.currentMarket ? bestPrice(a.currentMarket) : 0;
        const mb = b.currentMarket ? bestPrice(b.currentMarket) : 0;
        return dir * (ma - mb);
      }
      case 'pl': {
        const pa = unrealizedPL(costBasis(a.purchase), a.currentMarket) ?? -Infinity;
        const pb = unrealizedPL(costBasis(b.purchase), b.currentMarket) ?? -Infinity;
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
