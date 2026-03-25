import type { MarketSnapshot, AgingItem, SourcePrice } from '../../../../types/campaigns';
import { formatCents } from '../../../utils/formatters';

export type SortKey = 'name' | 'grade' | 'cost' | 'market' | 'pl' | 'days' | 'ev';
export type SortDir = 'asc' | 'desc';

/** Best available market price for a card. */
export function bestPrice(snap: MarketSnapshot): number {
  return snap.medianCents || snap.gradePriceCents || snap.lastSoldCents || 0;
}

/** Unrealized P/L in cents relative to cost basis. */
export function unrealizedPL(costBasis: number, snap: MarketSnapshot | undefined): number | null {
  if (!snap) return null;
  const price = bestPrice(snap);
  if (price <= 0) return null;
  return price - costBasis;
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

/** Get a source price by type: 'ebay' matches pokemon/ebay sources, 'estimate' matches cardhedger/estimate. */
export function getSourceByType(sources: SourcePrice[] | undefined, type: 'ebay' | 'estimate'): SourcePrice | undefined {
  if (!sources) return undefined;
  if (type === 'ebay') {
    return sources.find(s => /pokemon|ebay/i.test(s.source));
  }
  return sources.find(s => /cardhedger|estimate/i.test(s.source));
}

/** Format a YYYY-MM-DD date string to short form (e.g., "3/5"). */
export function fmtDateShort(dateStr: string): string {
  const [, m, d] = dateStr.split('-');
  return `${parseInt(m, 10)}/${parseInt(d, 10)}`;
}

/** Build a rich tooltip for the market cell. */
export function marketTooltip(snap: MarketSnapshot, costBasis: number): string {
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

  // P/L
  const pl = unrealizedPL(costBasis, snap);
  if (pl != null) {
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
