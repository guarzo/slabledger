/** Pricing types for the card pricing API response */

/** eBay sold data (smart market price) */
export interface EbayGradeData {
  price: number;
  confidence: 'high' | 'medium' | 'low' | null;
  salesCount: number;
  trend: 'up' | 'down' | 'stable' | null;
  median: number | null;
  min: number | null;
  max: number | null;
  avg7day: number | null;
  volume7day: number | null;
}

/** Multi-platform estimate from the price-estimate endpoint */
export interface EstimateGradeData {
  price: number;
  low: number;
  high: number;
  confidence: number; // 0-1 numeric
}

/** Combined data for a single grade */
export interface GradeData {
  ebay: EbayGradeData | null;
  estimate: EstimateGradeData | null;
}

export type GradeKey = 'raw' | 'psa1' | 'psa2' | 'psa3' | 'psa4' | 'psa5' | 'psa6' | 'psa7' | 'psa8' | 'psa9' | 'psa10';

export interface MarketOverview {
  activeListings: number;
  lowestListing: number;
  sales30d: number;
  sales90d: number;
}

export interface SalesVelocity {
  dailyAverage: number;
  weeklyAverage: number;
  monthlyTotal: number;
}

/** Price hint mapping for manual price corrections */
export interface PriceHint {
  cardName: string;
  setName: string;
  cardNumber: string;
  provider: 'doubleholo';
  externalId: string;
}
