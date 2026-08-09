/**
 * Core campaign domain types
 */

import type { SubjectRef } from './psaCampaign';

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
  /** Curated-spec-list language tokens; empty = open net (buys any language).
      Mirrors Go's inventory.Campaign.TargetLanguages (json: targetLanguages). */
  targetLanguages: string[];
  subjectFilterMode: string;
  subjects: SubjectRef[];
  deniedSpecs: SubjectRef[];
  phase: Phase;
  psaSourcingFeeCents: number;
  ebayFeePct: number;
  expectedFillRate: number;
  psaCampaignRequestId?: string;
  createdAt: string;
  updatedAt: string;
  /** Derived at the HTTP layer from the campaign id, never persisted.
      Mirrors Go's inventory.Campaign.Kind / SetKind (json: kind). */
  kind: 'external' | 'standard';
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
  dhLastSyncedAt?: string;
  /**
   * Set by the DH reconciler when a purchase previously known to DH is missing
   * from the authoritative inventory snapshot (i.e. the item was unlisted or
   * deleted on DH's side). Cleared on a successful re-list. Drives the
   * "Re-list (removed from DH)" row badge.
   */
  dhUnlistedDetectedAt?: string;
  gemRateId?: string;
  psaSpecId?: number;
  clSyncedAt?: string;
  clLastError?: 'no_value' | 'catalog_fallback' | 'api_error' | 'quota_exhausted' | 'no_image_match' | 'no_cert_match';
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
  // Decision provenance at time of purchase
  clConfidenceAtPurchase?: number;
  populationAtPurchase?: number;
  dhConfidenceAtPurchase?: number;
  sourceCountAtPurchase?: number;
  activeListingsAtPurchase?: number;
  salesLast30dAtPurchase?: number;
  // Campaign attribution provenance
  /** Raw campaign name PSA reported, verbatim. */
  psaCampaignName?: string;
  attributionSource?: 'psa' | 'inferred' | 'manual';
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
  // Decision provenance at time of sale
  /** Sale was driven by invoice timing pressure. Always present on the wire (no omitempty). */
  forcedLiquidation: boolean;
  saleReason?: string;
  clValueAtSaleCents?: number;
  channelFeePctAtSale?: number;
  // Itemized card-show sale provenance (see migration 000040)
  theirCompCents?: number;
  /** One of 'itemized' | 'estimated' | 'manual' | 'unknown'; mirrors Go's inventory.PriceSource. */
  priceSource?: string;
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
  /** Curated-spec-list language tokens; empty = open net (buys any language).
      Mirrors Go's inventory.Campaign.TargetLanguages (json: targetLanguages). */
  targetLanguages: string[];
  subjectFilterMode: string;
  subjects: SubjectRef[];
  deniedSpecs: SubjectRef[];
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

export interface BulkSaleItemInput {
  purchaseId: string;
  salePriceCents: number;
  originalListPriceCents?: number;
  priceReductions?: number;
  daysListed?: number;
  saleReason?: string;
  /** Provenance of salePriceCents. Mirrors Go's inventory.BulkSaleInput.PriceSource
      (json: priceSource, omitempty): 'itemized' | 'estimated' | 'manual'. Omitted
      leaves the server's unknown sentinel, which is the empty string (Go's
      PriceSourceUnknown = ""), not the literal string 'unknown'. */
  priceSource?: string;
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
  saleReason?: string;
}
