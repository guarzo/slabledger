export type ConfidenceLevel = 'high' | 'medium' | 'low' | 'none';

export interface LiquidationPreviewItem {
  purchaseId: string;
  certNumber: string;
  cardName: string;
  setName: string;
  cardNumber: string;
  grade: number;
  campaignName: string;
  buyCostCents: number;
  clValueCents: number;
  compPriceCents: number;
  compCount: number;
  mostRecentCompDate: string;
  confidenceLevel: ConfidenceLevel;
  gapPct: number;
  currentReviewedPriceCents: number;
  suggestedPriceCents: number;
  belowCost: boolean;
}

export interface LiquidationPreviewSummary {
  totalCards: number;
  withComps: number;
  withoutComps: number;
  noData: number;
  totalCurrentValueCents: number;
  totalSuggestedValueCents: number;
  belowCostCount: number;
}

export interface LiquidationPreviewResponse {
  items: LiquidationPreviewItem[];
  summary: LiquidationPreviewSummary;
}

export interface LiquidationApplyItem {
  purchaseId: string;
  newPriceCents: number;
}

export interface LiquidationApplyResult {
  applied: number;
  failed: number;
  errors: string[];
}
