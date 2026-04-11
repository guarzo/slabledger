/**
 * Core campaign domain types
 */

export type Phase = 'pending' | 'active' | 'closed';
export type SaleChannel = 'ebay' | 'website' | 'inperson' | 'tcgplayer' | 'local' | 'other' | 'gamestop' | 'cardshow' | 'doubleholo';

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
  mmValueCents?: number;
  mmTrendPct?: number;
  mmSales30d?: number;
  mmActiveLowCents?: number;
  mmValueUpdatedAt?: string;
  buyCostCents: number;
  psaSourcingFeeCents: number;
  population?: number;
  purchaseDate: string;
  invoiceDate?: string;
  wasRefunded?: boolean;
  frontImageUrl?: string;
  backImageUrl?: string;
  purchaseSource?: string;
  psaListingTitle?: string;
  receivedAt?: string;    // ISO-8601 datetime — set when cert scanned via intake (in hand)
  psaShipDate?: string;   // YYYY-MM-DD ship date from PSA spreadsheet
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
  snapshotRetryCount?: number;
  dhCardId?: number;
  dhInventoryId?: number;
  dhCertStatus?: string;
  dhListingPriceCents?: number;
  dhChannelsJson?: string;
  dhStatus?: string;
  dhPushStatus?: string;
  dhHoldReason?: string;
  dhCandidatesJson?: string;
  gemRateId?: string;
  psaSpecId?: number;
  clSyncedAt?: string;
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
  orderId?: string;
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
