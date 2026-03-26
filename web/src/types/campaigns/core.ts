/**
 * Core campaign domain types
 */

export type Phase = 'pending' | 'active' | 'closed';
export type SaleChannel = 'ebay' | 'tcgplayer' | 'local' | 'other' | 'gamestop' | 'website' | 'cardshow';

export interface Campaign {
  id: string;
  name: string;
  sport: string;
  yearRange: string;
  gradeRange: string;
  priceRange: string;
  clConfidence: string;
  buyTermsCLPct: number;
  dailySpendCapCents: number;
  inclusionList: string;
  exclusionMode: boolean;
  phase: Phase;
  psaSourcingFeeCents: number;
  ebayFeePct: number;
  expectedFillRate: number;
  createdAt: string;
  updatedAt: string;
}

export interface Purchase {
  id: string;
  campaignId: string;
  cardName: string;
  certNumber: string;
  cardNumber?: string;    // Card number within set
  setName?: string;       // Set/category name
  grader?: string;
  gradeValue: number;
  clValueCents: number;
  buyCostCents: number;
  psaSourcingFeeCents: number;
  population?: number;
  purchaseDate: string;
  vaultStatus?: string;
  invoiceDate?: string;
  wasRefunded?: boolean;
  frontImageUrl?: string;
  backImageUrl?: string;
  purchaseSource?: string;
  psaListingTitle?: string;
  snapshotStatus?: '' | 'pending' | 'failed' | 'exhausted';
  overridePriceCents?: number;
  overrideSource?: string;
  overrideSetAt?: string;
  aiSuggestedPriceCents?: number;
  aiSuggestedAt?: string;
  cardYear?: string;
  ebayExportFlaggedAt?: string;
  reviewedPriceCents?: number;
  reviewedAt?: string;
  reviewSource?: string;
  createdAt: string;
  updatedAt: string;
  // Market snapshot at time of purchase
  lastSoldCents?: number;
  lowestListCents?: number;
  conservativeCents?: number;
  medianCents?: number;
  activeListings?: number;
  salesLast30d?: number;
  trend30d?: number;
  snapshotDate?: string;
}

export interface Sale {
  id: string;
  purchaseId: string;
  saleChannel: SaleChannel;
  salePriceCents: number;
  saleFeeCents: number;
  saleDate: string;
  daysToSell: number;
  netProfitCents: number;
  createdAt: string;
  updatedAt: string;
  // Market snapshot at time of sale
  lastSoldCents?: number;
  lowestListCents?: number;
  conservativeCents?: number;
  medianCents?: number;
  activeListings?: number;
  salesLast30d?: number;
  trend30d?: number;
  snapshotDate?: string;
  // Sale outcome enrichment
  originalListPriceCents?: number;
  priceReductions?: number;
  daysListed?: number;
  soldAtAskingPrice?: boolean;
  wasCracked?: boolean;
}

export interface CreateCampaignInput {
  name: string;
  sport: string;
  yearRange: string;
  gradeRange: string;
  priceRange: string;
  clConfidence: string;
  buyTermsCLPct: number;
  dailySpendCapCents: number;
  inclusionList: string;
  exclusionMode: boolean;
  psaSourcingFeeCents: number;
  ebayFeePct: number;
  phase?: Phase;
}

export interface CreatePurchaseInput {
  cardName: string;
  certNumber: string;
  gradeValue: number;
  clValueCents?: number;
  buyCostCents?: number;
  psaSourcingFeeCents: number;
  purchaseDate: string;
  setName?: string;
  cardNumber?: string;
  population?: number;
}

/** Status values returned by CL and PSA import endpoints. */
export type ImportResultStatus =
  | 'allocated' | 'refreshed' | 'updated' | 'refunded'
  | 'unmatched' | 'ambiguous' | 'skipped' | 'failed';

/** Shared per-row result item used by both CL and PSA imports. */
export interface ImportResultItem {
  certNumber: string;
  cardName?: string;
  grade?: number;
  status: ImportResultStatus;
  campaignId?: string;
  campaignName?: string;
  candidates?: string[];
  error?: string;
  buyCostCents?: number;
  clValueCents?: number;
  purchaseDate?: string;
  setName?: string;
  cardNumber?: string;
  population?: number;
}

export interface CreateSaleInput {
  purchaseId: string;
  saleChannel: SaleChannel;
  salePriceCents: number;
  saleDate: string;
  originalListPriceCents?: number;
  priceReductions?: number;
  daysListed?: number;
  soldAtAskingPrice?: boolean;
  wasCracked?: boolean;
}

// Cert lookup types

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
  // Fusion metadata
  sourceCount?: number;
  fusionConfidence?: number;
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

// Shopify price sync types

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
  recommendation: string;
  priceDeltaPct: number;
  hasMarketData: boolean;
  overridePriceCents?: number;
  overrideSource?: string;
  aiSuggestedPriceCents?: number;
  recommendedPriceCents: number;
  recommendedSource: string;
  reviewedAt?: string;
}

export interface ShopifyPriceSyncResponse {
  matched: ShopifyPriceSyncMatch[];
  unmatched: string[];
}

// PSA Import types

export interface PSAImportResult {
  allocated: number;
  updated: number;
  refunded: number;
  unmatched: number;
  ambiguous: number;
  skipped: number;
  failed: number;
  invoicesCreated?: number;
  errors?: { row?: number; error: string }[];
  results?: ImportResultItem[];
  byCampaign?: Record<string, { campaignName: string; allocated: number; refreshed: number }>;
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
  creditLimitCents: number;
  cashBufferCents: number;
  updatedAt: string;
}

export interface CreditSummary {
  creditLimitCents: number;
  outstandingCents: number;
  utilizationPct: number;
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

export interface GlobalImportResult {
  allocated: number;
  refreshed: number;
  unmatched: number;
  ambiguous: number;
  skipped: number;
  failed: number;
  errors?: { row?: number; error: string }[];
  results?: ImportResultItem[];
  byCampaign?: Record<string, { campaignName: string; allocated: number; refreshed: number }>;
}

export interface RevocationFlag {
  id: string;
  segmentLabel: string;
  segmentDimension: string;
  reason: string;
  status: 'pending' | 'sent';
  emailText: string;
  createdAt: string;
  sentAt?: string;
}

export interface BulkSaleResult {
  created: number;
  failed: number;
  errors?: { purchaseId: string; error: string }[];
}

export interface ExternalImportResult {
  imported: number;
  skipped: number;
  updated: number;
  failed: number;
  errors?: { row?: number; error: string }[];
  results?: ExternalImportItemResult[];
}

export interface ExternalImportItemResult {
  certNumber: string;
  cardName?: string;
  setName?: string;
  cardNumber?: string;
  status: 'imported' | 'skipped' | 'updated' | 'failed';
  error?: string;
}

// Cert entry

export interface CertImportError {
  certNumber: string;
  error: string;
}

export interface CertImportResult {
  imported: number;
  alreadyExisted: number;
  failed: number;
  errors: CertImportError[];
}

// eBay export

export interface EbayExportItem {
  purchaseId: string;
  certNumber: string;
  cardName: string;
  setName: string;
  cardNumber: string;
  cardYear: string;
  gradeValue: number;
  grader: string;
  clValueCents: number;
  marketMedianCents: number;
  suggestedPriceCents: number;
  hasCLValue: boolean;
  hasMarketData: boolean;
  frontImageUrl?: string;
  backImageUrl?: string;
}

export interface EbayExportListResponse {
  items: EbayExportItem[];
}

export interface EbayExportGenerateItem {
  purchaseId: string;
  priceCents: number;
}
