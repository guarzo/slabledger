import type { MarketSnapshot } from '../../types/campaigns';
import type { SellSheetItem } from '../../types/campaigns';

/** Upper-case suffixes that should stay uppercase in Pokemon card names. */
const UPPER_SUFFIXES = new Set(['EX', 'GX', 'VMAX', 'V', 'VSTAR']);

/** Title-case a single word, preserving Pokemon suffixes and expanding abbreviations. */
function titleWord(word: string): string {
  const upper = word.toUpperCase();
  if (UPPER_SUFFIXES.has(upper)) return upper;
  if (upper === 'REV.FOIL' || upper === 'REV.' || upper === 'REVFOIL') return 'Reverse Foil';
  if (upper === '1ST' && word.length <= 4) return '1st';
  if (upper === 'ED.' || upper === 'EDITION') return 'Edition';
  if (upper === '1STED.' || upper === '1STEDITION') return '1st Edition';
  return word.charAt(0).toUpperCase() + word.slice(1).toLowerCase();
}

/** Format a card name for customer-facing display: title-case with Pokemon suffix handling. */
export function formatCardName(name: string): string {
  if (!name) return '';
  // Handle "1ST ED." as a combined token
  const normalized = name.replace(/1ST\s+ED\.?/gi, '1st Edition');
  return normalized.split(/\s+/).map(titleWord).join(' ');
}

/** Format grader prefix and grade for display. */
export function gradeDisplay(item: SellSheetItem): string {
  const prefix = item.grader && item.grader !== 'PSA' ? item.grader : 'PSA';
  return `${prefix} ${item.grade}`;
}

/** Build a subtitle from set name and card number. */
export function cardSubtitle(item: SellSheetItem): string | null {
  const parts: string[] = [];
  if (item.setName) parts.push(item.setName);
  if (item.cardNumber) parts.push(`#${item.cardNumber}`);
  return parts.length > 0 ? parts.join(' · ') : null;
}

/** Compute margin percentage code for the sell sheet. Returns "[XX]" where XX is the margin %. Negative means underwater. */
export function marginCode(targetSellPrice: number, costBasisCents: number): string {
  if (costBasisCents <= 0 || targetSellPrice <= 0) return '[0]';
  const pct = Math.floor(((targetSellPrice - costBasisCents) / costBasisCents) * 100);
  return `[${pct}]`;
}

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

/** Check if a SellSheetItem is a "hot seller" based on market data. */
export function isHotSellerFromSellSheet(item: SellSheetItem): boolean {
  return checkHotSeller(item.currentMarket, item.targetSellPrice ?? 0);
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
