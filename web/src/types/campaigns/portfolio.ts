/**
 * Portfolio insights, suggestions, capital timeline, and weekly review types
 */

import type { SaleChannel } from './core';
import type { ChannelPNL } from './analytics';

// Portfolio Insights types

export interface SegmentPerformance {
  label: string;
  dimension: string;
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
  avgMarginPct: number;
  bestChannel?: SaleChannel;
  campaignCount: number;
  latestSaleDate?: string;
}

export interface CoverageGap {
  segment: SegmentPerformance;
  reason: string;
  opportunity: string;
}

export interface InsightsDataSummary {
  totalPurchases: number;
  totalSales: number;
  campaignsAnalyzed: number;
  dateRange: string;
  overallROI: number;
}

export interface CampaignPNLBrief {
  campaignId: string;
  roi: number;
  spendCents: number;
  profitCents: number;
  soldCount: number;
  purchaseCount: number;
}

export interface PortfolioInsights {
  byCharacter: SegmentPerformance[];
  byGrade: SegmentPerformance[];
  byEra: SegmentPerformance[];
  byPriceTier: SegmentPerformance[];
  byChannel: ChannelPNL[];
  byCharacterGrade: SegmentPerformance[];
  coverageGaps: CoverageGap[];
  campaignMetrics?: CampaignPNLBrief[];
  dataSummary: InsightsDataSummary;
}

// Campaign Suggestions types

export interface CampaignSuggestionParams {
  name: string;
  yearRange?: string;
  gradeRange?: string;
  priceRange?: string;
  buyTermsCLPct?: number;
  buyTermsCLPctOptimistic?: number;
  dailySpendCapCents?: number;
  // Plain subject names for display only — a suggestion is pre-portal and
  // carries no PSA subject ids to preserve.
  subjects?: string[];
  // Polarity of `subjects` on an "adjust" suggestion for an existing
  // campaign: 'add' means these should be added to the campaign's current
  // targeting list, 'remove' means removed from it. Absent on "new"/"gap"
  // suggestions, where `subjects` is the full initial list for a brand-new
  // campaign rather than a delta against an existing one.
  subjectsAction?: 'add' | 'remove';
  primaryExit?: string;
}

export interface ExpectedMetrics {
  expectedROI: number;
  expectedMarginPct: number;
  avgDaysToSell: number;
  dataConfidence: string;
}

export interface CampaignSuggestion {
  type: 'new' | 'adjust' | 'gap';
  title: string;
  rationale: string;
  confidence: 'high' | 'medium' | 'low';
  dataPoints: number;
  suggestedParams: CampaignSuggestionParams;
  expectedMetrics: ExpectedMetrics;
}

export interface SuggestionsResponse {
  newCampaigns: CampaignSuggestion[];
  adjustments: CampaignSuggestion[];
  dataSummary: InsightsDataSummary;
}

export interface WeeklyPerformer {
  cardName: string;
  certNumber: string;
  grade: number;
  profitCents: number;
  channel: string;
  daysToSell: number;
  grader?: string;
}

export interface WeeklyReviewSummary {
  weekStart: string;
  weekEnd: string;
  purchasesThisWeek: number;
  purchasesLastWeek: number;
  spendThisWeekCents: number;
  spendLastWeekCents: number;
  salesThisWeek: number;
  salesLastWeek: number;
  revenueThisWeekCents: number;
  revenueLastWeekCents: number;
  profitThisWeekCents: number;
  profitLastWeekCents: number;
  byChannel: ChannelPNL[];
  weeksToCover: number;
  topPerformers: WeeklyPerformer[];
  bottomPerformers: WeeklyPerformer[];
}
