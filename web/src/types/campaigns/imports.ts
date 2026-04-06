/**
 * Import/export related campaign types
 */

import type { SaleChannel } from './core';

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
  errors?: { row?: number; error: string }[];
  results?: ImportResultItem[];
  byCampaign?: Record<string, { campaignName: string; allocated: number; refreshed: number }>;
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

export type ScanCertResponse =
  | { status: 'new' }
  | { status: 'existing' | 'sold'; cardName: string; purchaseId: string; campaignId: string };

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
