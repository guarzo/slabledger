/**
 * Import/export related campaign types
 */

import type { SaleChannel } from './core';
import type { MarketSnapshot } from './market';

/** Shared error shape for import results. */
export interface ImportError {
  row?: number;
  error: string;
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
  byCampaign?: Record<string, { campaignName: string; allocated: number }>;
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
  buyCostCents?: number;
  market?: MarketSnapshot;
  // Populated for existing/sold statuses — used by the intake-row DH-search helper.
  frontImageUrl?: string;
  setName?: string;
  cardNumber?: string;
  cardYear?: string;
  gradeValue?: number;
  population?: number;
}

export interface ResolveCertResponse {
  certNumber: string;
  cardName: string;
  grade: number;
  year: string;
  category: string;
  subject: string;
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

// Market Movers refresh

export interface MMRefreshItemResult {
  certNumber: string;
  cardName?: string;
  oldValueCents: number;
  newValueCents: number;
  status: 'updated' | 'skipped' | 'failed';
  error?: string;
}

export interface MMRefreshResult {
  updated: number;
  notFound: number;
  skipped: number;
  failed: number;
  errors?: ImportError[];
  results?: MMRefreshItemResult[];
}
