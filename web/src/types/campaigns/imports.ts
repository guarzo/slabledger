/**
 * Import/export related campaign types
 */

import type { SaleChannel } from './core';

/** Shared error shape for import results. */
export interface ImportError {
  row?: number;
  error: string;
}

/** Status values returned by CL import endpoints. */
export type GlobalImportResultStatus =
  | 'allocated' | 'refreshed' | 'updated' | 'refunded'
  | 'unmatched' | 'ambiguous' | 'skipped' | 'failed';

/** Per-row result for CL (global) imports. */
export interface GlobalImportItemResult {
  certNumber: string;
  cardName?: string;
  grade?: number;
  status: GlobalImportResultStatus;
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

/** Status values returned by PSA import endpoints. */
export type PSAImportResultStatus =
  | 'allocated' | 'updated' | 'refunded'
  | 'unmatched' | 'ambiguous' | 'skipped' | 'failed';

/** Per-row result for PSA imports (no clValueCents). */
export interface PSAImportItemResult {
  certNumber: string;
  cardName?: string;
  grade?: number;
  status: PSAImportResultStatus;
  campaignId?: string;
  campaignName?: string;
  candidates?: string[];
  error?: string;
  population?: number;
}

export interface GlobalImportResult {
  allocated: number;
  refreshed: number;
  unmatched: number;
  ambiguous: number;
  skipped: number;
  failed: number;
  errors?: ImportError[];
  results?: GlobalImportItemResult[];
  byCampaign?: Record<string, { campaignName: string; allocated: number; refreshed: number }>;
}

export interface PSAImportResult {
  allocated: number;
  updated: number;
  refunded: number;
  unmatched: number;
  ambiguous: number;
  skipped: number;
  failed: number;
  invoicesCreated?: number;
  invoicesUpdated?: number;
  certEnrichmentPending?: number;
  errors?: ImportError[];
  results?: PSAImportItemResult[];
  byCampaign?: Record<string, { campaignName: string; allocated: number; refreshed: number }>;
}

export interface ExternalImportResult {
  imported: number;
  skipped: number;
  updated: number;
  failed: number;
  errors?: ImportError[];
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
  soldExisting: number;
  failed: number;
  errors: CertImportError[];
  soldItems?: CertImportSoldItem[];
}

export interface CertImportSoldItem {
  certNumber: string;
  purchaseId: string;
  cardName: string;
  campaignId: string;
}

export interface ScanCertResponse {
  status: 'new' | 'existing' | 'sold';
  cardName?: string;
  purchaseId?: string;
  campaignId?: string;
}

export interface ResolveCertResponse {
  certNumber: string;
  cardName: string;
  grade: number;
  year: string;
  category: string;
  subject: string;
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
  costBasisCents: number;
  lastSoldCents: number;
  reviewedPriceCents?: number;
  reviewedAt?: string;
}

export interface EbayExportListResponse {
  items: EbayExportItem[];
}

export interface EbayExportGenerateItem {
  purchaseId: string;
  priceCents: number;
}

// Orders sales import types

export interface OrdersImportMatch {
  certNumber: string;
  productTitle: string;
  saleChannel: SaleChannel;
  saleDate: string;
  salePriceCents: number;
  saleFeeCents: number;
  purchaseId: string;
  campaignId: string;
  cardName: string;
  buyCostCents: number;
  netProfitCents: number;
  campaignLookupFailed?: boolean;
}

export interface OrdersImportSkip {
  certNumber: string;
  productTitle: string;
  reason: string;
}

export interface OrdersImportResult {
  matched: OrdersImportMatch[];
  alreadySold: OrdersImportSkip[];
  notFound: OrdersImportSkip[];
  skipped: OrdersImportSkip[];
}

export interface OrdersConfirmItem {
  purchaseId: string;
  saleChannel: SaleChannel;
  saleDate: string;
  salePriceCents: number;
  orderId?: string;
}

export interface BulkSaleResult {
  created: number;
  failed: number;
  errors?: { purchaseId: string; error: string }[];
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
