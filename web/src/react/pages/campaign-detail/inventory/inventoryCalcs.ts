import type { AgingItem, ReviewStats, ExpectedValue } from '../../../../types/campaigns';
import { costBasis, bestPrice, unrealizedPL, getReviewStatus, reviewUrgencySort, isCardShowCandidate } from './utils';
import type { SortKey, SortDir } from './utils';

export interface TabCounts {
  needs_review: number;
  large_gap: number;
  no_data: number;
  flagged: number;
  card_show: number;
  all: number;
}

export interface SummaryStats {
  totalCost: number;
  totalMarket: number;
  totalPL: number;
}

export function computeReviewStatsAndCounts(items: AgingItem[]): { reviewStats: ReviewStats; tabCounts: TabCounts } {
  const stats: ReviewStats = { total: items.length, needsReview: 0, reviewed: 0, flagged: 0 };
  const counts: TabCounts = { needs_review: 0, large_gap: 0, no_data: 0, flagged: 0, card_show: 0, all: items.length };
  for (const item of items) {
    if (item.hasOpenFlag) stats.flagged++;
    if (item.purchase.reviewedAt) stats.reviewed++;
    else stats.needsReview++;

    const status = getReviewStatus(item);
    if (status === 'needs_review') { counts.needs_review++; }
    else if (status === 'large_gap') { counts.needs_review++; counts.large_gap++; }
    else if (status === 'no_data') { counts.needs_review++; counts.no_data++; }
    else if (status === 'flagged') counts.flagged++;
    if (isCardShowCandidate(item)) counts.card_show++;
  }
  return { reviewStats: stats, tabCounts: counts };
}

export function computeSummaryStats(items: AgingItem[]): SummaryStats {
  const totalCost = items.reduce((sum, i) => sum + costBasis(i.purchase), 0);
  const totalMarket = items.reduce((sum, i) => {
    if (!i.currentMarket) return sum;
    return sum + bestPrice(i.currentMarket);
  }, 0);
  return { totalCost, totalMarket, totalPL: totalMarket > 0 ? totalMarket - totalCost : 0 };
}

export type FilterTab = 'needs_review' | 'large_gap' | 'no_data' | 'flagged' | 'card_show' | 'all' | 'sell_sheet';

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
        const status = getReviewStatus(i);
        if (filterTab === 'large_gap') return status === 'large_gap';
        if (filterTab === 'no_data') return status === 'no_data';
        if (filterTab === 'flagged') return status === 'flagged';
        if (filterTab === 'card_show') return isCardShowCandidate(i);
        // 'needs_review' tab shows needs_review + large_gap + no_data (all unreviewed/unflagged)
        return status === 'needs_review' || status === 'large_gap' || status === 'no_data';
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
