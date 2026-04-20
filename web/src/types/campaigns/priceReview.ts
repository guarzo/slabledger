import type { SourcePrice } from './market';

export type PriceFlagReason =
  | 'wrong_match'
  | 'stale_data'
  | 'wrong_grade'
  | 'source_disagreement'
  | 'other';

export const PRICE_FLAG_LABELS: Record<PriceFlagReason, string> = {
  wrong_match: 'Wrong card match',
  stale_data: 'Stale / outdated data',
  wrong_grade: 'Wrong grade pricing',
  source_disagreement: 'Source disagreement',
  other: 'Other',
};

export interface PriceFlag {
  id: number;
  purchaseId: string;
  flaggedBy: number;
  flaggedAt: string;
  reason: PriceFlagReason;
  resolvedAt?: string;
  resolvedBy?: number;
}

export interface PriceFlagWithContext extends PriceFlag {
  cardName: string;
  setName?: string;
  cardNumber?: string;
  grade: number;
  certNumber: string;
  flaggedByEmail: string;
  marketPriceCents: number;
  clValueCents: number;
  reviewedPriceCents: number;
  sourcePrices?: SourcePrice[];
}

export interface PriceFlagsResponse {
  flags: PriceFlagWithContext[];
  total: number;
}

export interface ReviewStats {
  total: number;
  reviewed: number;
  flagged: number;
  aging60d: number;
}
