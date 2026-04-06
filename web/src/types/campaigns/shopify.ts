/**
 * Shopify price sync types
 */

export interface PriceSyncSale {
  soldAt: string;
  grade: string;
  priceCents: number;
  platform: string;
}

export interface PriceSyncPop {
  grade: string;
  count: number;
}

export interface PriceSyncROI {
  grade: string;
  avgSaleCents: number;
  roi: number;
}

export interface PriceSyncIntel {
  sentimentScore: number;
  sentimentTrend: string;
  sentimentMentions: number;
  forecastCents: number;
  forecastConfidence: number;
  forecastDate?: string;
  recentSalesCount: number;
  recentSales?: PriceSyncSale[];
  population?: PriceSyncPop[];
  gradingROI?: PriceSyncROI[];
  insightHeadline?: string;
  insightDetail?: string;
  fetchedAt?: string;
}

export interface ShopifyPriceSyncMatch {
  certNumber: string;
  cardName: string;
  setName?: string;
  cardNumber?: string;
  grade: number;
  grader?: string;
  currentPriceCents: number;
  suggestedPriceCents: number;
  minimumPriceCents: number;
  costBasisCents: number;
  clValueCents: number;
  marketPriceCents: number;
  lastSoldCents: number;
  recommendation: string;
  priceDeltaPct: number;
  hasMarketData: boolean;
  overridePriceCents?: number;
  overrideSource?: string;
  aiSuggestedPriceCents?: number;
  recommendedPriceCents: number;
  recommendedSource: string;
  reviewedAt?: string;
  intel?: PriceSyncIntel;
}

export interface ShopifyPriceSyncResponse {
  matched: ShopifyPriceSyncMatch[];
  unmatched: string[];
}
