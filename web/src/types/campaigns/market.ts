/**
 * Market, pricing, cert lookup, and sell sheet types
 */

import type { SaleChannel } from './core';

export interface SourcePrice {
  source: string;
  priceCents: number;
  saleCount?: number;
  trend?: string;
  confidence?: string;
  minCents?: number;
  maxCents?: number;
  avg7DayCents?: number;
  volume7Day?: number;
}

export interface MarketSnapshot {
  lastSoldCents: number;
  lastSoldDate?: string;
  saleCount?: number;
  gradePriceCents: number;
  lowestListCents?: number;
  activeListings?: number;
  salesLast30d?: number;
  salesLast90d?: number;
  conservativeCents?: number;
  medianCents?: number;
  optimisticCents?: number;
  trend30d?: number;
  trend90d?: number;
  volatility?: number;
  // Extended percentiles
  p10Cents?: number;
  p90Cents?: number;
  distSampleSize?: number;
  distPeriodDays?: number;
  // Sales velocity
  dailyVelocity?: number;
  weeklyVelocity?: number;
  monthlyVelocity?: number;
  // Short-term signal
  avg7DayCents?: number;
  // Pricing metadata
  sourceCount?: number;
  confidence?: number;
  // Per-source pricing data
  sourcePrices?: SourcePrice[];
  // Grade estimation flag
  isEstimated?: boolean;
  // Pricing gap flag
  pricingGap?: boolean;
}

export interface CertInfo {
  certNumber: string;
  cardName: string;
  grade: number;
  year: string;
  brand: string;
  subject: string;
  variety?: string;
  cardNumber?: string;
  population: number;
  popHigher: number;
}

export interface CertLookupResult {
  cert: CertInfo;
  market?: MarketSnapshot;
}

export interface QuickAddRequest {
  certNumber: string;
  buyCostCents: number;
  clValueCents?: number;
  purchaseDate?: string;
}

export interface InventorySignals {
  profitCaptureDeclining?: boolean;
  profitCaptureSpike?: boolean;
  crackCandidate?: boolean;
  staleListing?: boolean;
  deepStale?: boolean;
  cutLoss?: boolean;
}

export interface SellSheetItem {
  purchaseId?: string;
  campaignName?: string;
  certNumber: string;
  cardName: string;
  setName?: string;
  cardNumber?: string;
  grade: number;
  grader?: string;
  population?: number;
  buyCostCents: number;
  costBasisCents: number;
  clValueCents: number;
  currentMarket?: MarketSnapshot;
  recommendation: string;
  targetSellPrice: number;
  minimumAcceptPrice: number;
  priceLookupError?: string;
  recommendedChannel?: SaleChannel;
  channelLabel?: string;
  overridePriceCents?: number;
  overrideSource?: string;
  isOverridden?: boolean;
  computedPriceCents?: number;
  aiSuggestedPriceCents?: number;
  aiSuggestedAt?: string;
  signals?: InventorySignals;
}

export interface SellSheet {
  generatedAt: string;
  campaignName: string;
  items: SellSheetItem[];
  totals: {
    totalCostBasis: number;
    totalExpectedRevenue: number;
    totalProjectedProfit: number;
    itemCount: number;
  };
}

// Credit & Invoice types

export interface Invoice {
  id: string;
  invoiceDate: string;
  totalCents: number;
  paidCents: number;
  dueDate?: string;
  paidDate?: string;
  status: 'unpaid' | 'partial' | 'paid';
  createdAt: string;
  updatedAt: string;
}

export interface CashflowConfig {
  capitalBudgetCents: number;
  cashBufferCents: number;
  updatedAt: string;
}

export interface CapitalSummary {
  capitalBudgetCents: number;
  outstandingCents: number;
  exposurePct: number;
  refundedCents: number;
  paidCents: number;
  unpaidInvoiceCount: number;
  alertLevel: 'ok' | 'warning' | 'critical';
  projectedExposureCents?: number;
  daysToNextInvoice?: number;
}

// Activation checklist types

export interface ActivationCheck {
  name: string;
  passed: boolean;
  message: string;
}

export interface ActivationChecklist {
  campaignId: string;
  campaignName: string;
  allPassed: boolean;
  checks: ActivationCheck[];
  warnings: string[];
}
