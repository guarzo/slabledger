import type { MarketSnapshot } from '../../types/campaigns';

/**
 * Check if a card is a "hot seller": 3+ sales in 30 days and last sold >= target price.
 * Works with any type that has the required fields (SellSheetItem, AgingItem, etc.).
 */
export function checkHotSeller(snap: MarketSnapshot | undefined, targetPriceCents: number): boolean {
  if (!snap || !snap.salesLast30d || snap.salesLast30d < 3) return false;
  if (!snap.lastSoldCents || snap.lastSoldCents <= 0) return false;
  if (targetPriceCents <= 0) return false;
  return snap.lastSoldCents >= targetPriceCents;
}

/**
 * Resolve the CL price to display on the printed sell sheet.
 * Returns null when neither CL nor recommended price is available.
 * `estimated: true` means the value came from the recommended price fallback
 * and should be rendered with a `~` prefix.
 */
export function clPriceDisplayCents(
  src: { clValueCents?: number; recommendedPriceCents?: number },
): { cents: number; estimated: boolean } | null {
  if (src.clValueCents && src.clValueCents > 0) {
    return { cents: src.clValueCents, estimated: false };
  }
  if (src.recommendedPriceCents && src.recommendedPriceCents > 0) {
    return { cents: src.recommendedPriceCents, estimated: true };
  }
  return null;
}

/**
 * Format an ISO date as MM/DD/YY for the printed last-sale column.
 * Returns '' for missing/unparseable input.
 */
export function formatLastSaleDate(iso?: string): string {
  if (!iso) return '';
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return '';
  const mm = String(d.getUTCMonth() + 1).padStart(2, '0');
  const dd = String(d.getUTCDate()).padStart(2, '0');
  const yy = String(d.getUTCFullYear()).slice(-2);
  return `${mm}/${dd}/${yy}`;
}

/** Format cents as a whole-dollar string with thousands separator (e.g. 27900 → "$279"). */
export function dollars(cents: number): string {
  return `$${Math.round(cents / 100).toLocaleString('en-US')}`;
}
