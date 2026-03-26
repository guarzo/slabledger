/**
 * Analytics, tuning, and performance types
 */

import type { MarketSnapshot, Phase, Purchase, Sale, SaleChannel } from './core';

export interface CampaignPNL {
  campaignId: string;
  totalSpendCents: number;
  totalRevenueCents: number;
  totalFeesCents: number;
  netProfitCents: number;
  roi: number;
  avgDaysToSell: number;
  totalPurchases: number;
  totalSold: number;
  totalUnsold: number;
  sellThroughPct: number;
}

export interface ChannelPNL {
  channel: SaleChannel;
  saleCount: number;
  revenueCents: number;
  feesCents: number;
  netProfitCents: number;
  avgDaysToSell: number;
}

export interface DailySpend {
  date: string;
  spendCents: number;
  capCents: number;
  fillRatePct: number;
  purchaseCount: number;
}

export interface DaysToSellBucket {
  label: string;
  min: number;
  max: number;
  count: number;
}

export interface MarketSignal {
  cardName: string;
  certNumber: string;
  grade: number;
  clValueCents: number;
  lastSoldCents: number;
  deltaPct: number;
  direction: 'rising' | 'falling' | 'stable';
  recommendation: string;
}

export interface AgingItem {
  purchase: Purchase;
  daysHeld: number;
  campaignName?: string;
  signal?: MarketSignal;
  currentMarket?: MarketSnapshot;
  priceAnomaly?: boolean;
  anomalyReason?: string;
  hasOpenFlag?: boolean;
  recommendedPriceCents?: number;
  recommendedSource?: string;
}

// Tuning types

export interface GradePerformance {
  grade: number;
  purchaseCount: number;
  soldCount: number;
  sellThroughPct: number;
  avgDaysToSell: number;
  totalSpendCents: number;
  totalRevenueCents: number;
  totalFeesCents: number;
  netProfitCents: number;
  roi: number;
  avgBuyPctOfCL: number;
}

export interface PriceTierPerformance {
  tierLabel: string;
  tierMinCents: number;
  tierMaxCents: number;
  purchaseCount: number;
  soldCount: number;
  sellThroughPct: number;
  avgDaysToSell: number;
  totalSpendCents: number;
  totalRevenueCents: number;
  totalFeesCents: number;
  netProfitCents: number;
  roi: number;
  avgBuyPctOfCL: number;
}

export interface CardPerformance {
  purchase: Purchase;
  buyPctOfCL: number;
  sale?: Sale;
  currentMarket?: MarketSnapshot;
  realizedPnL: number;
  unrealizedPnL: number;
}

export interface BuyThresholdDataPoint {
  purchaseId: string;
  buyPctOfCL: number;
  roi: number;
  sold: boolean;
  costBasisCents: number;
  profitCents: number;
}

export interface ThresholdBucket {
  rangeLabel: string;
  rangeMinPct: number;
  rangeMaxPct: number;
  count: number;
  avgROI: number;
  medianROI: number;
  totalProfitCents: number;
}

export interface BuyThresholdAnalysis {
  dataPoints: BuyThresholdDataPoint[];
  optimalPct: number;
  currentPct: number;
  bucketedROI: ThresholdBucket[];
  sampleSize: number;
  confidence: number;
}

export interface MarketAlignmentData {
  avgTrend30d: number;
  avgTrend90d: number;
  avgVolatility: number;
  avgSalesLast30d: number;
  avgSnapshotDrift: number;
  appreciatingCount: number;
  depreciatingCount: number;
  stableCount: number;
  signal: 'healthy' | 'caution' | 'warning';
  signalReason: string;
  sampleSize: number;
}

export interface TuningRecommendation {
  parameter: string;
  currentVal: string;
  suggestedVal: string;
  reasoning: string;
  impact: 'high' | 'medium' | 'low';
  confidence: number;
  dataPoints: number;
}

export interface TuningResponse {
  campaignId: string;
  campaignName: string;
  byGrade: GradePerformance[];
  byFixedTier: PriceTierPerformance[];
  byRelativeTier: PriceTierPerformance[];
  topPerformers: CardPerformance[];
  bottomPerformers: CardPerformance[];
  buyThreshold?: BuyThresholdAnalysis;
  marketAlignment?: MarketAlignmentData;
  recommendations: TuningRecommendation[];
}

// Channel velocity types

export interface ChannelVelocity {
  channel: SaleChannel;
  saleCount: number;
  avgDaysToSell: number;
  revenueCents: number;
}

// Portfolio health types

export interface CampaignHealth {
  campaignId: string;
  campaignName: string;
  phase: Phase;
  roi: number;
  sellThroughPct: number;
  avgDaysToSell: number;
  totalPurchases: number;
  totalUnsold: number;
  capitalAtRiskCents: number;
  healthStatus: 'healthy' | 'warning' | 'critical';
  healthReason: string;
}

export interface PortfolioHealth {
  campaigns: CampaignHealth[];
  totalDeployedCents: number;
  totalRecoveredCents: number;
  totalAtRiskCents: number;
  overallROI: number;
}

// Crack Arbitrage types

export interface CrackAnalysis {
  purchaseId: string;
  cardName: string;
  certNumber: string;
  grade: number;
  buyCostCents: number;
  costBasisCents: number;
  rawMarketCents: number;
  breakevenRawCents: number;
  gradedNetCents: number;
  crackNetCents: number;
  crackAdvantageCents: number;
  isCrackCandidate: boolean;
  crackROI: number;
  gradedROI: number;
}

// Expected Value types

export interface ExpectedValue {
  cardName: string;
  certNumber: string;
  grade: number;
  costBasisCents: number;
  sellProbability: number;
  expectedSalePriceCents: number;
  expectedFeesCents: number;
  expectedProfitCents: number;
  carryingCostCents: number;
  evCents: number;
  evPerDollar: number;
  segmentSellThrough: number;
  liquidityFactor: number;
  trendAdjustment: number;
  confidence: 'high' | 'medium' | 'low';
}

export interface EVPortfolio {
  items: ExpectedValue[];
  totalEvCents: number;
  positiveCount: number;
  negativeCount: number;
  minDataPoints: number;
}

// Monte Carlo Simulation types

export interface MonteCarloResult {
  label: string;
  simulations: number;
  medianROI: number;
  p10ROI: number;
  p90ROI: number;
  medianProfitCents: number;
  p10ProfitCents: number;
  p90ProfitCents: number;
  medianVolume: number;
}

export interface MonteCarloComparison {
  current: MonteCarloResult;
  scenarios: MonteCarloResult[];
  bestScenarioIndex: number;
  sampleSize: number;
  confidence: 'high' | 'medium' | 'low' | 'insufficient';
}
