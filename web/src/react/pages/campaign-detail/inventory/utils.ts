import type { MarketSnapshot, AgingItem, SourcePrice } from '../../../../types/campaigns';
import { formatCents } from '../../../utils/formatters';
import { checkHotSeller } from '../../../utils/sellSheetHelpers';

export type SortKey = 'name' | 'grade' | 'cost' | 'market' | 'pl' | 'days' | 'ev';
export type SortDir = 'asc' | 'desc';

/** Total cost basis for a purchase (buy cost + PSA sourcing fee). */
export function costBasis(p: { buyCostCents: number; psaSourcingFeeCents: number }): number {
  return p.buyCostCents + p.psaSourcingFeeCents;
}

/** Best available market price from a raw snapshot. Falls back across medianCents / gradePriceCents / lastSoldCents. */
export function bestPriceFromSnap(snap: MarketSnapshot): number {
  return snap.medianCents || snap.gradePriceCents || snap.lastSoldCents || 0;
}

/**
 * Best "what this is actually selling for" price for an item. Prefers the most recent real sale
 * from comp analytics when available, since that is the single truest indicator of current market;
 * otherwise falls back to snapshot-derived signals.
 */
export function bestPrice(item: AgingItem): number {
  const compLast = item.compSummary?.lastSaleCents ?? 0;
  if (compLast > 0) return compLast;
  return item.currentMarket ? bestPriceFromSnap(item.currentMarket) : 0;
}

/**
 * The most recent real sale for an item — prefers comp analytics (literal most-recent sold price),
 * falls back to the snapshot's lastSold. Returns null when no sale data exists.
 */
export function mostRecentSale(item: AgingItem): { cents: number; date?: string } | null {
  const cs = item.compSummary;
  if (cs && cs.lastSaleCents > 0) {
    return { cents: cs.lastSaleCents, date: cs.lastSaleDate || undefined };
  }
  const snap = item.currentMarket;
  if (snap && snap.lastSoldCents > 0) {
    return { cents: snap.lastSoldCents, date: snap.lastSoldDate };
  }
  return null;
}

/** Unrealized P/L in cents relative to cost basis, using the same best-price logic as the UI. */
export function unrealizedPL(costBasisCents: number, item: AgingItem): number | null {
  const price = bestPrice(item);
  if (price <= 0) return null;
  return price - costBasisCents;
}

export function marketTrend(snap: MarketSnapshot): 'up' | 'down' | 'stable' | null {
  const trend = snap.trend30d;
  if (trend == null) return null;
  if (trend === 0) return 'stable';
  if (trend > 0.05) return 'up';
  if (trend < -0.05) return 'down';
  return 'stable';
}

/** Velocity label: how fast a card sells. */
export function velocityLabel(snap: MarketSnapshot): string | null {
  if (snap.salesLast30d != null && snap.salesLast30d > 0) {
    return `${snap.salesLast30d}/30d`;
  }
  if (snap.monthlyVelocity != null && snap.monthlyVelocity > 0) {
    return `~${snap.monthlyVelocity}/mo`;
  }
  return null;
}

/** Get a source price by type: 'ebay' matches pokemon/ebay sources, 'estimate' matches estimate sources. */
export function getSourceByType(sources: SourcePrice[] | undefined, type: 'ebay' | 'estimate'): SourcePrice | undefined {
  if (!sources) return undefined;
  if (type === 'ebay') {
    return sources.find(s => /pokemon|ebay/i.test(s.source));
  }
  return sources.find(s => /estimate/i.test(s.source));
}

/** Format a YYYY-MM-DD date string to short form (e.g., "3/5"). */
export function fmtDateShort(dateStr: string): string {
  const [, m, d] = dateStr.split('-');
  return `${parseInt(m, 10)}/${parseInt(d, 10)}`;
}

/** Build a rich tooltip for the market cell. */
export function marketTooltip(snap: MarketSnapshot, costBasisCents: number): string {
  const lines: string[] = [];

  // Price range
  if (snap.conservativeCents && snap.optimisticCents) {
    lines.push(`Range: ${formatCents(snap.conservativeCents)} - ${formatCents(snap.optimisticCents)}`);
  }
  if (snap.p10Cents && snap.p90Cents) {
    lines.push(`P10-P90: ${formatCents(snap.p10Cents)} - ${formatCents(snap.p90Cents)}`);
  }

  // Last sold
  if (snap.lastSoldCents > 0) {
    let line = `Last sold: ${formatCents(snap.lastSoldCents)}`;
    if (snap.lastSoldDate) line += ` (${snap.lastSoldDate})`;
    lines.push(line);
  }

  // 7-day average
  if (snap.avg7DayCents && snap.avg7DayCents > 0) {
    lines.push(`7-day avg: ${formatCents(snap.avg7DayCents)}`);
  }

  // Listings
  if (snap.lowestListCents) {
    let line = `Lowest list: ${formatCents(snap.lowestListCents)}`;
    if (snap.activeListings) line += ` (${snap.activeListings} active)`;
    lines.push(line);
  }

  // Velocity
  if (snap.salesLast30d) lines.push(`${snap.salesLast30d} sales (30d)`);
  if (snap.salesLast90d) lines.push(`${snap.salesLast90d} sales (90d)`);
  if (snap.dailyVelocity && snap.dailyVelocity > 0) {
    lines.push(`Velocity: ${snap.dailyVelocity.toFixed(1)}/day`);
  }

  // Trend
  if (snap.trend30d != null && snap.trend30d !== 0) {
    const sign = snap.trend30d > 0 ? '+' : '';
    lines.push(`30d trend: ${sign}${(snap.trend30d * 100).toFixed(0)}%`);
  }

  // P/L (tooltip context has only the snapshot, so derive from snapshot signals)
  const snapPrice = bestPriceFromSnap(snap);
  if (snapPrice > 0) {
    const pl = snapPrice - costBasisCents;
    const sign = pl >= 0 ? '+' : '';
    lines.push(`Est. P/L: ${sign}${formatCents(pl)}`);
  }

  // Sources
  if (snap.sourcePrices && snap.sourcePrices.length > 0) {
    lines.push('');
    snap.sourcePrices.forEach(sp => {
      let line = `${sp.source}: ${formatCents(sp.priceCents)}`;
      if (sp.saleCount) line += ` (${sp.saleCount} sales)`;
      if (sp.avg7DayCents) line += ` 7d: ${formatCents(sp.avg7DayCents)}`;
      lines.push(line);
    });
  }

  return lines.join('\n');
}

/** Color for P/L display. */
export function plColor(pl: number): string {
  if (pl > 0) return 'text-[var(--success)]';
  if (pl < 0) return 'text-[var(--danger)]';
  return 'text-[var(--text-muted)]';
}

/** Format a P/L value with sign. */
export function formatPL(pl: number): string {
  const sign = pl >= 0 ? '+' : '';
  return `${sign}${formatCents(pl)}`;
}

/** Derive signal direction from an AgingItem. Uses signal.direction if available, falls back to snap.trend30d. */
export function deriveSignalDirection(item: AgingItem): 'rising' | 'falling' | 'stable' | null {
  if (item.signal?.direction) return item.signal.direction;
  if (!item.currentMarket) return null;
  const trend = marketTrend(item.currentMarket);
  if (trend === 'up') return 'rising';
  if (trend === 'down') return 'falling';
  if (trend === 'stable') return 'stable';
  return null;
}

/** Derive signal delta percentage from an AgingItem. */
export function deriveSignalDelta(item: AgingItem): number | null {
  if (item.signal?.deltaPct != null) return item.signal.deltaPct;
  if (item.currentMarket?.trend30d != null && item.currentMarket.trend30d !== 0) {
    return item.currentMarket.trend30d * 100;
  }
  return null;
}

/** Format grade display, showing grader prefix for non-PSA graders. */
export function displayGrade(purchase: { grader?: string; gradeValue: number }): string {
  const grade = purchase.gradeValue;
  if (purchase.grader && purchase.grader !== 'PSA') {
    return `${purchase.grader} ${grade}`;
  }
  return String(grade);
}

export type ReviewStatus = 'needs_review' | 'large_gap' | 'no_data' | 'flagged' | 'reviewed';

export function getReviewStatus(item: AgingItem): ReviewStatus {
  if (item.hasOpenFlag) return 'flagged';
  const snap = item.currentMarket;
  const p = item.purchase;

  if (p.reviewedAt) return 'reviewed';
  if (!snap && (p.clValueCents ?? 0) === 0) return 'no_data';
  if (hasLargeGap(item)) return 'large_gap';

  return 'needs_review';
}

export function hasLargeGap(item: AgingItem): boolean {
  const prices: number[] = [];
  const p = item.purchase;
  const snap = item.currentMarket;

  if (p.clValueCents > 0) prices.push(p.clValueCents);
  if (snap?.medianCents) prices.push(snap.medianCents);
  const cb = costBasis(p);
  if (cb > 0) prices.push(cb);
  if (snap?.lowestListCents) prices.push(snap.lowestListCents);

  if (prices.length < 2) return false;

  for (let i = 0; i < prices.length; i++) {
    for (let j = i + 1; j < prices.length; j++) {
      const max = Math.max(prices[i], prices[j]);
      const min = Math.min(prices[i], prices[j]);
      if (max > 0 && (max - min) / max > 0.3) return true;
    }
  }
  return false;
}

const STATUS_PRIORITY: Record<ReviewStatus, number> = {
  flagged: 0,
  large_gap: 1,
  no_data: 2,
  needs_review: 3,
  reviewed: 4,
};

export function reviewUrgencySort(a: AgingItem, b: AgingItem): number {
  const statusA = getReviewStatus(a);
  const statusB = getReviewStatus(b);
  const priorityDiff = STATUS_PRIORITY[statusA] - STATUS_PRIORITY[statusB];
  if (priorityDiff !== 0) return priorityDiff;
  return b.daysHeld - a.daysHeld;
}

export function statusBorderColor(status: ReviewStatus): string {
  switch (status) {
    case 'flagged':
    case 'large_gap':
      return 'var(--danger)';
    case 'needs_review':
      return 'var(--warning)';
    case 'reviewed':
      return 'var(--success)';
    case 'no_data':
      return 'var(--text-muted)';
  }
}

export function statusBadge(item: AgingItem): { label: string; color: string } {
  if (item.purchase.dhPushStatus === 'held') {
    return { label: 'DH Held', color: 'var(--warning)' };
  }
  const status = getReviewStatus(item);
  switch (status) {
    case 'flagged':
      return { label: 'Flagged', color: 'var(--danger)' };
    case 'large_gap':
      return { label: 'Large Gap', color: 'var(--danger)' };
    case 'no_data':
      return { label: 'No Data', color: 'var(--text-muted)' };
    case 'needs_review':
      return { label: 'Review', color: 'var(--warning)' };
    case 'reviewed': {
      const reviewedAt = item.purchase.reviewedAt;
      const relTime = reviewedAt ? relativeTime(reviewedAt) : '';
      return { label: `✓ ${relTime}`, color: 'var(--success)' };
    }
  }
}

export function relativeTime(isoDate: string): string {
  if (!isoDate) return 'unknown';
  const ts = new Date(isoDate).getTime();
  if (isNaN(ts)) return 'unknown';
  const diff = Math.max(0, Date.now() - ts);
  const days = Math.floor(diff / 86400000);
  if (days === 0) return 'today';
  if (days === 1) return '1d ago';
  if (days < 30) return `${days}d ago`;
  if (days < 365) return `${Math.floor(days / 30)}mo ago`;
  return `${Math.floor(days / 365)}y ago`;
}

/** A card is a "hot seller" if it has 3+ sales in the last 30 days and the last sold price >= target price. */
export function isHotSeller(item: AgingItem): boolean {
  return checkHotSeller(item.currentMarket, item.recommendedPriceCents ?? 0);
}

/** A card is a "card show candidate" if it has in-person signals or matches the legacy heuristic. */
export function isCardShowCandidate(item: AgingItem): boolean {
  if (item.signals?.profitCaptureDeclining || item.signals?.profitCaptureSpike || item.signals?.crackCandidate) {
    return true;
  }
   if (isHotSeller(item)) return true;
   if (item.purchase.gradeValue === 7) return true;
   if (item.currentMarket?.trend30d != null && item.currentMarket.trend30d > 0.05) return true;
   return false;
}

/** Format an ISO date string to "Mon D, YYYY" format (e.g., "Apr 5, 2026"). */
export function formatReceivedDate(iso: string): string {
   const d = new Date(iso);
   if (isNaN(d.getTime())) return iso;
   return d.toLocaleDateString('en-US', { month: 'short', day: 'numeric', year: 'numeric' });
}

/** Format a YYYY-MM-DD date string to "Mon D, YYYY" format (e.g., "Apr 3, 2026"). Uses local Date constructor to avoid UTC shift. */
export function formatShipDate(dateStr: string): string {
  const parts = dateStr.split('-').map(Number);
  if (parts.length !== 3 || parts.some(isNaN)) return dateStr;
  const [year, month, day] = parts;
  const d = new Date(year, month - 1, day);
  if (isNaN(d.getTime())) return dateStr;
  return d.toLocaleDateString('en-US', { month: 'short', day: 'numeric', year: 'numeric' });
}

// Sync dot logic lives in syncDot.ts — re-exported here for backward compat.
export type { SyncDotProps } from './syncDot';
export { syncDotProps } from './syncDot';
