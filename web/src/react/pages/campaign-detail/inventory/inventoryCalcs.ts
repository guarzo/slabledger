import type { AgingItem, ReviewStats, ExpectedValue } from '../../../../types/campaigns';
import { costBasis, bestPrice, unrealizedPL, getReviewStatus, reviewUrgencySort } from './utils';
import type { SortKey, SortDir } from './utils';

const EXCEPTION_STATUSES = ['large_gap', 'no_data', 'flagged'] as const;

export interface TabCounts {
  needs_attention: number;
  awaiting_intake: number;
  pending_dh_match: number;
  pending_price: number;
  ready_to_list: number;
  dh_listed: number;
  skipped: number;
  /** Legacy alias for bookmarks using the old filter key. Equals `all`. */
  in_hand: number;
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

export function isDHHeld(item: AgingItem): boolean {
  return item.purchase.dhPushStatus === 'held';
}

function hasCommittedPrice(item: AgingItem): boolean {
  return (
    (item.purchase.reviewedPriceCents ?? 0) > 0 ||
    (item.purchase.overridePriceCents ?? 0) > 0
  );
}

export function isSkipped(item: AgingItem): boolean {
  return item.purchase.dhPushStatus === 'dismissed';
}

export function isDHListed(item: AgingItem): boolean {
  return item.purchase.dhStatus === 'listed';
}

export function isPendingDHMatch(item: AgingItem): boolean {
  if (!item.purchase.receivedAt) return false;
  if (isSkipped(item)) return false;
  if (isDHListed(item)) return false;
  return !item.purchase.dhInventoryId;
}

export function isPendingPrice(item: AgingItem): boolean {
  if (!item.purchase.receivedAt) return false;
  if (isSkipped(item)) return false;
  if (isDHListed(item)) return false;
  if (!item.purchase.dhInventoryId) return false;
  return !hasCommittedPrice(item);
}

export function isReadyToList(item: AgingItem): boolean {
  if (isSkipped(item)) return false;
  return (
    !!item.purchase.receivedAt &&
    !!item.purchase.dhInventoryId &&
    item.purchase.dhStatus !== 'listed' &&
    hasCommittedPrice(item)
  );
}

// needsPriceReview: Existing alias — same semantic as isPendingPrice.
export function needsPriceReview(item: AgingItem): boolean {
  return isPendingPrice(item);
}

export type ActionIntent = 'fix_match' | 'set_and_list' | 'list' | 'restore' | 'none';

// deriveActionIntent picks a single primary row action from the item's own
// status. Shared by DesktopRow and MobileCard so the two stay in lockstep.
export function deriveActionIntent(item: AgingItem): ActionIntent {
  if (isSkipped(item)) return 'restore';
  if (isPendingDHMatch(item)) return 'fix_match';
  if (isPendingPrice(item)) return 'set_and_list';
  if (isReadyToList(item)) return 'list';
  return 'none';
}

export function canDismiss(intent: ActionIntent): boolean {
  return intent === 'fix_match' || intent === 'set_and_list' || intent === 'list';
}

// wasUnlistedFromDH: the DH reconciler detected this item was deleted
// from DH's authoritative inventory snapshot. Drives the
// "Re-list (removed from DH)" row badge. Clears on successful re-list.
export function wasUnlistedFromDH(item: AgingItem): boolean {
  return !!item.purchase.dhUnlistedDetectedAt;
}

export function needsAttention(item: AgingItem, status = getReviewStatus(item)): boolean {
  if (!item.purchase.receivedAt) return false;
  if (isSkipped(item)) return false;
  if ((EXCEPTION_STATUSES as readonly string[]).includes(status)) return true;
  if (isDHHeld(item)) return true;
  if (!hasCommittedPrice(item) && (item.purchase.aiSuggestedPriceCents ?? 0) > 0) return true;
  return false;
}

export function computeInventoryMeta(items: AgingItem[]): InventoryMeta {
  const stats: ReviewStats = { total: items.length, reviewed: 0, flagged: 0, aging60d: 0 };
  const counts: TabCounts = {
    needs_attention: 0,
    awaiting_intake: 0,
    pending_dh_match: 0,
    pending_price: 0,
    ready_to_list: 0,
    dh_listed: 0,
    skipped: 0,
    in_hand: 0,
    all: items.length,
  };
  let totalCost = 0;
  let totalMarket = 0;
  for (const item of items) {
    if (item.hasOpenFlag) stats.flagged++;
    if (item.purchase.reviewedAt) stats.reviewed++;
    if (item.daysHeld >= 60) stats.aging60d++;

    const status = getReviewStatus(item);
    if (needsAttention(item, status)) counts.needs_attention++;

    // Secondary-row partition: evaluate top-down, first match wins.
    if (!item.purchase.receivedAt) {
      counts.awaiting_intake++;
    } else if (isSkipped(item)) {
      counts.skipped++;
    } else if (isDHListed(item)) {
      counts.dh_listed++;
    } else if (isPendingDHMatch(item)) {
      counts.pending_dh_match++;
    } else if (isPendingPrice(item)) {
      counts.pending_price++;
    } else if (isReadyToList(item)) {
      counts.ready_to_list++;
    }

    totalCost += costBasis(item.purchase);
    totalMarket += bestPrice(item);
  }
  counts.in_hand = counts.all; // alias kept for old bookmarks
  return {
    reviewStats: stats,
    tabCounts: counts,
    summary: { totalCost, totalMarket, totalPL: totalMarket - totalCost },
  };
}

export type FilterTab =
  | 'needs_attention'
  | 'sell_sheet'
  | 'all'
  | 'awaiting_intake'
  | 'pending_dh_match'
  | 'pending_price'
  | 'ready_to_list'
  | 'dh_listed'
  | 'skipped'
  | 'in_hand'; // legacy alias

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
    pinnedIds?: Set<string>;
  },
): AgingItem[] {
  const { debouncedSearch, showAll, filterTab, sellSheetHas, sortKey, sortDir, evMap } = opts;

  if (opts.pinnedIds && opts.pinnedIds.size > 0) {
    const subset = items.filter(i => opts.pinnedIds!.has(i.purchase.id));
    const dir = sortDir === 'asc' ? 1 : -1;
    return [...subset].sort((a, b) => {
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
  let result = items;

  if (debouncedSearch.trim()) {
    const q = debouncedSearch.toLowerCase();
    result = result.filter(i =>
      i.purchase.cardName.toLowerCase().includes(q) ||
      (i.purchase.certNumber && i.purchase.certNumber.toLowerCase().includes(q)) ||
      (i.purchase.setName && i.purchase.setName.toLowerCase().includes(q))
    );
  } else if (!showAll) {
    if (filterTab === 'sell_sheet') {
      result = result.filter(i => sellSheetHas(i.purchase.id) && !!i.purchase.receivedAt);
    } else if (filterTab === 'in_hand') {
      // Legacy alias: treat as `all`.
      // result stays as-is
    } else if (filterTab !== 'all') {
      result = result.filter(i => {
        switch (filterTab) {
          case 'needs_attention': return needsAttention(i);
          case 'awaiting_intake': return !i.purchase.receivedAt;
          case 'pending_dh_match': return isPendingDHMatch(i);
          case 'pending_price': return isPendingPrice(i);
          case 'ready_to_list': return isReadyToList(i);
          case 'dh_listed': return isDHListed(i);
          case 'skipped': return isSkipped(i);
          default: return false;
        }
      });
    }
  }

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
