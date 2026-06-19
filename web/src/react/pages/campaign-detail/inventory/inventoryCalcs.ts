import type { AgingItem, ReviewStats } from '../../../../types/campaigns';
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

export type PriceBand = 'all' | 'lt50' | '50to100' | '100to250' | '250to500' | 'gte500';

export type PriceBandCounts = Record<Exclude<PriceBand, 'all'>, number> & { all: number };

export interface InventoryMeta {
  reviewStats: ReviewStats;
  tabCounts: TabCounts;
  priceBandCounts: PriceBandCounts;
  summary: SummaryStats;
}

/** Bucket an item by its `bestPrice` (cents) into one of the preset bands.
    `bestPrice` is the metric the Price column shows and sorts on, so the band
    a card lands in matches the visible Market value. Items with no price
    (bestPrice === 0) are excluded from all band buckets. */
export function priceBandOf(item: AgingItem): Exclude<PriceBand, 'all'> | null {
  const cents = bestPrice(item);
  if (cents <= 0) return null;
  if (cents < 5000) return 'lt50';
  if (cents < 10000) return '50to100';
  if (cents < 25000) return '100to250';
  if (cents < 50000) return '250to500';
  return 'gte500';
}

export function matchesPriceBand(item: AgingItem, band: PriceBand): boolean {
  if (band === 'all') return true;
  return priceBandOf(item) === band;
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

export function computeTotals(items: AgingItem[]): SummaryStats {
  let totalCost = 0;
  let totalMarket = 0;
  for (const item of items) {
    totalCost += costBasis(item.purchase);
    totalMarket += bestPrice(item);
  }
  return { totalCost, totalMarket, totalPL: totalMarket - totalCost };
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
  const priceBandCounts: PriceBandCounts = {
    all: items.length,
    lt50: 0,
    '50to100': 0,
    '100to250': 0,
    '250to500': 0,
    gte500: 0,
  };
  let totalCost = 0;
  let totalMarket = 0;
  for (const item of items) {
    if (item.hasOpenFlag) stats.flagged++;
    if (item.purchase.reviewedAt) stats.reviewed++;
    if (item.daysHeld >= 60) stats.aging60d++;

    const status = getReviewStatus(item);
    if (needsAttention(item, status)) counts.needs_attention++;

    const band = priceBandOf(item);
    if (band) priceBandCounts[band]++;

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
    priceBandCounts,
    summary: { totalCost, totalMarket, totalPL: totalMarket - totalCost },
  };
}

/** Count items per price band over a given base set. Pass the output of
    `applySearchAndTab` to get counts scoped to the active tab + search, so each
    `$` pill badge equals the rows clicking it would produce in the current view.
    Items with no price (priceBandOf === null) count toward `all` only. */
export function computePriceBandCounts(items: AgingItem[]): PriceBandCounts {
  const counts: PriceBandCounts = {
    all: items.length,
    lt50: 0,
    '50to100': 0,
    '100to250': 0,
    '250to500': 0,
    gte500: 0,
  };
  for (const item of items) {
    const band = priceBandOf(item);
    if (band) counts[band]++;
  }
  return counts;
}

export type FilterTab =
  | 'needs_attention'
  | 'all'
  | 'awaiting_intake'
  | 'pending_dh_match'
  | 'pending_price'
  | 'ready_to_list'
  | 'dh_listed'
  | 'skipped'
  | 'in_hand'; // legacy alias

function sortItems(
  items: AgingItem[],
  sortKey: SortKey,
  sortDir: SortDir,
): AgingItem[] {
  const dir = sortDir === 'asc' ? 1 : -1;
  return [...items].sort((a, b) => {
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
      default:
        return 0;
    }
  });
}

/** Final row ordering: an explicit column → real column sort; `null` → the
    smart urgency order (flagged → large_gap → no_data → needs_review →
    reviewed, then oldest-first). Independent of search. */
function orderItems(items: AgingItem[], sortKey: SortKey | null, sortDir: SortDir): AgingItem[] {
  return sortKey === null
    ? [...items].sort(reviewUrgencySort)
    : sortItems(items, sortKey, sortDir);
}

/** Select the base set for the current view: search wins over the tab filter;
    `all` and the legacy `in_hand` alias apply no narrowing. This is the single
    source of truth for "what rows does the active tab+search show", shared by
    both row filtering and price-band counting. */
export function applySearchAndTab(
  items: AgingItem[],
  debouncedSearch: string,
  filterTab: FilterTab,
): AgingItem[] {
  if (debouncedSearch.trim()) {
    const q = debouncedSearch.toLowerCase();
    return items.filter(i =>
      i.purchase.cardName.toLowerCase().includes(q) ||
      (i.purchase.certNumber && i.purchase.certNumber.toLowerCase().includes(q)) ||
      (i.purchase.setName && i.purchase.setName.toLowerCase().includes(q))
    );
  }
  if (filterTab === 'in_hand' || filterTab === 'all') {
    return items;
  }
  return items.filter(i => {
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

export function filterAndSortItems(
  items: AgingItem[],
  opts: {
    debouncedSearch: string;
    filterTab: FilterTab;
    sortKey: SortKey | null;
    sortDir: SortDir;
    pinnedIds?: ReadonlySet<string>;
    priceBand?: PriceBand;
  },
): AgingItem[] {
  const { debouncedSearch, filterTab, sortKey, sortDir, priceBand = 'all' } = opts;

  if (opts.pinnedIds && opts.pinnedIds.size > 0) {
    const subset = items.filter(i => opts.pinnedIds!.has(i.purchase.id));
    return orderItems(subset, sortKey, sortDir);
  }

  let result = applySearchAndTab(items, debouncedSearch, filterTab);

  if (priceBand !== 'all') {
    result = result.filter(i => matchesPriceBand(i, priceBand));
  }

  return orderItems(result, sortKey, sortDir);
}
