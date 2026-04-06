import type { GradeData } from '../../../types/pricing';
import type { SearchableCard } from '../../utils/marketplaceUrls';
import {
  defaultEbayUrl as sharedDefaultEbayUrl,
  defaultAltUrl as sharedDefaultAltUrl,
  defaultCardLadderUrl as sharedDefaultCardLadderUrl,
  ebayCompletedUrl,
} from '../../utils/marketplaceUrls';
import { currency } from '../../utils/formatters';

/** Narrowed grade key for the grades displayed by CardPriceCard. */
export type PriceCardGradeKey = 'raw' | 'psa8' | 'psa9' | 'psa10';

/** Minimal card identity used by URL helpers — avoids importing from CardPriceCard. */
export interface PriceCardData {
  name: string;
  setName: string;
  number: string;
}

export interface LastSoldEntry {
  lastSoldPrice: number;
  lastSoldDate: string;
  saleCount: number;
}

/* ---------- Formatters ---------- */

export function fmtRange(low: number, high: number): string {
  const fmtPart = (v: number): string => currency(v).replace('$', '');
  return `$${fmtPart(low)}\u2013$${fmtPart(high)}`;
}

export function isNoData(price: number | null | undefined): boolean {
  return price == null || price < 0.01;
}

export function formatEbayDisplay(
  ebayPrice: number | null,
  gradeDetail: GradeData | undefined,
  fallbackPrice: number | undefined,
): string {
  if (ebayPrice != null) return isNoData(ebayPrice) ? '\u2014' : currency(ebayPrice);
  if (gradeDetail) return '\u2014';
  return isNoData(fallbackPrice) ? '\u2014' : currency(fallbackPrice);
}

export function fmtDateShort(dateStr: string): string {
  if (!dateStr) return '\u2014';
  const parts = dateStr.split('-');
  if (parts.length < 3) return '\u2014';
  const month = parseInt(parts[1], 10);
  const day = parseInt(parts[2], 10);
  if (isNaN(month) || isNaN(day)) return '\u2014';
  return `${month}/${day}`;
}

/* ---------- URL helpers ---------- */

function toSearchable(card: PriceCardData): SearchableCard {
  return { name: card.name, setName: card.setName, number: card.number };
}

export function defaultEbayUrl(card: PriceCardData, grade?: PriceCardGradeKey): string {
  return sharedDefaultEbayUrl(toSearchable(card), grade);
}

export function defaultAltUrl(card: PriceCardData, grade?: PriceCardGradeKey): string {
  return sharedDefaultAltUrl(toSearchable(card), grade);
}

export function defaultCardLadderUrl(card: PriceCardData, grade?: PriceCardGradeKey): string {
  return sharedDefaultCardLadderUrl(toSearchable(card), grade);
}

export function localEbayCompletedUrl(card: PriceCardData, grade?: PriceCardGradeKey): string {
  return ebayCompletedUrl(toSearchable(card), grade);
}

/* ---------- Constants ---------- */

export const gradeRows: { key: PriceCardGradeKey; label: string }[] = [
  { key: 'raw', label: 'Raw' },
  { key: 'psa8', label: 'PSA 8' },
  { key: 'psa9', label: 'PSA 9' },
  { key: 'psa10', label: 'PSA 10' },
];

export const gradeBorderColors: Record<PriceCardGradeKey, string> = {
  raw: 'var(--text-muted)',
  psa8: 'var(--grade-psa8)',
  psa9: 'var(--grade-psa9)',
  psa10: 'var(--grade-psa10)',
};
