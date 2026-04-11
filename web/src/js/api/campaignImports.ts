/**
 * Campaign import/export API methods: PSA, CL, external, orders, Shopify, eBay, cert entry, price review
 */

import type {
  GlobalImportResult, PSAImportResult, ExternalImportResult,
  CertLookupResult, ShopifyPriceSyncResponse,
  CertImportResult, EbayExportListResponse, EbayExportGenerateItem,
  OrdersImportResult, OrdersConfirmItem, BulkSaleResult,
  ScanCertResponse, ResolveCertResponse, MMRefreshResult,
} from '../../types/campaigns';
import type { PriceFlagsResponse } from '../../types/campaigns/priceReview';
import type { PSAPendingItem } from '../../types/admin';
import { APIClient } from './client';

declare module './client' {
  interface APIClient {
    // Cert lookup
    lookupCert(certNumber: string): Promise<CertLookupResult>;

    // Global imports / exports
    globalImportCL(file: File): Promise<GlobalImportResult>;
    globalExportCL(missingCLOnly?: boolean): Promise<Blob>;
    globalExportMM(missingMMOnly?: boolean): Promise<Blob>;
    globalRefreshMM(file: File): Promise<MMRefreshResult>;

    // PSA / External imports
    globalImportPSA(file: File): Promise<PSAImportResult>;
    syncPSASheets(): Promise<PSAImportResult>;
    globalImportExternal(file: File): Promise<ExternalImportResult>;

    // Orders sales import
    importOrdersSales(file: File): Promise<OrdersImportResult>;
    confirmOrdersSales(items: OrdersConfirmItem[]): Promise<BulkSaleResult>;

    // Shopify
    shopifyPriceSync(items: { certNumber: string; currentPriceCents: number; grader: string }[]): Promise<ShopifyPriceSyncResponse>;

    // Cert entry & eBay export
    importCerts(certNumbers: string[]): Promise<CertImportResult>;
    scanCert(certNumber: string): Promise<ScanCertResponse>;
    resolveCert(certNumber: string): Promise<ResolveCertResponse>;
    listEbayExportItems(flaggedOnly: boolean): Promise<EbayExportListResponse>;
    generateEbayCSV(items: EbayExportGenerateItem[]): Promise<Blob>;

    // Price review & flags
    setReviewedPrice(purchaseId: string, priceCents: number, source: string): Promise<{ success: boolean; reviewedAt: string }>;
    createPriceFlag(purchaseId: string, reason: string): Promise<{ id: number; flaggedAt: string }>;
    listPriceFlags(status?: string): Promise<PriceFlagsResponse>;
    resolvePriceFlag(flagId: number): Promise<void>;

    // PSA pending items
    listPSAPendingItems(): Promise<{ items: PSAPendingItem[] }>;
    assignPSAPendingItem(id: string, campaignId: string): Promise<unknown>;
    dismissPSAPendingItem(id: string): Promise<void>;
  }
}

const proto = APIClient.prototype;

// Cert lookup
proto.lookupCert = async function (this: APIClient, certNumber: string): Promise<CertLookupResult> {
  return this.get<CertLookupResult>(`/certs/${certNumber}`);
};

// Global purchase endpoints (cross-campaign)
proto.globalImportCL = async function (this: APIClient, file: File): Promise<GlobalImportResult> {
  return this.uploadFile<GlobalImportResult>('/purchases/import-cl', file);
};

proto.globalExportCL = async function (this: APIClient, missingCLOnly?: boolean): Promise<Blob> {
  const params = missingCLOnly ? '?missing_cl_only=true' : '';
  const response = await this.fetchWithRetry(
    `${this.baseURL}/purchases/export-cl${params}`,
    {},
  );
  return response.blob();
};

proto.globalExportMM = async function (this: APIClient, missingMMOnly?: boolean): Promise<Blob> {
  const params = missingMMOnly ? '?missing_mm_only=true' : '';
  const response = await this.fetchWithRetry(
    `${this.baseURL}/purchases/export-mm${params}`,
    {},
  );
  return response.blob();
};

proto.globalRefreshMM = async function (this: APIClient, file: File): Promise<MMRefreshResult> {
  return this.uploadFile<MMRefreshResult>('/purchases/refresh-mm', file);
};

// PSA CSV import (global)
proto.globalImportPSA = async function (this: APIClient, file: File): Promise<PSAImportResult> {
  return this.uploadFile<PSAImportResult>('/purchases/import-psa', file);
};

// PSA Google Sheets sync
proto.syncPSASheets = async function (this: APIClient): Promise<PSAImportResult> {
  return this.post<PSAImportResult>('/purchases/sync-psa-sheets', {});
};

// External (Shopify) CSV import
proto.globalImportExternal = async function (this: APIClient, file: File): Promise<ExternalImportResult> {
  return this.uploadFile<ExternalImportResult>('/purchases/import-external', file);
};

// Orders sales import
proto.importOrdersSales = async function (this: APIClient, file: File): Promise<OrdersImportResult> {
  return this.uploadFile<OrdersImportResult>('/purchases/import-orders', file);
};

proto.confirmOrdersSales = async function (this: APIClient, items: OrdersConfirmItem[]): Promise<BulkSaleResult> {
  return this.post<BulkSaleResult>('/purchases/import-orders/confirm', items);
};

// Shopify price sync
proto.shopifyPriceSync = async function (this: APIClient, items: { certNumber: string; currentPriceCents: number; grader: string }[]): Promise<ShopifyPriceSyncResponse> {
  return this.post<ShopifyPriceSyncResponse>('/shopify/price-sync', { items });
};

// Cert entry
proto.importCerts = async function (this: APIClient, certNumbers: string[]): Promise<CertImportResult> {
  return this.post<CertImportResult>('/purchases/import-certs', { certNumbers });
};

proto.scanCert = async function (this: APIClient, certNumber: string): Promise<ScanCertResponse> {
  return this.post<ScanCertResponse>('/purchases/scan-cert', { certNumber });
};

proto.resolveCert = async function (this: APIClient, certNumber: string): Promise<ResolveCertResponse> {
  return this.post<ResolveCertResponse>('/purchases/resolve-cert', { certNumber });
};

// eBay export
proto.listEbayExportItems = async function (this: APIClient, flaggedOnly: boolean): Promise<EbayExportListResponse> {
  const params = flaggedOnly ? '?flagged_only=true' : '';
  return this.get<EbayExportListResponse>(`/purchases/export-ebay${params}`);
};

proto.generateEbayCSV = async function (this: APIClient, items: EbayExportGenerateItem[]): Promise<Blob> {
  const response = await this.fetchWithRetry(
    `${this.baseURL}/purchases/export-ebay/generate`,
    {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ items }),
    },
  );
  return response.blob();
};

// Price review & flag endpoints
proto.setReviewedPrice = async function (
  this: APIClient, purchaseId: string, priceCents: number, source: string,
): Promise<{ success: boolean; reviewedAt: string }> {
  const response = await this.fetchWithRetry(
    `${this.baseURL}/purchases/${purchaseId}/review-price`,
    {
      method: 'PATCH',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ priceCents, source }),
    }
  );
  return response.json() as Promise<{ success: boolean; reviewedAt: string }>;
};

proto.createPriceFlag = async function (
  this: APIClient, purchaseId: string, reason: string,
): Promise<{ id: number; flaggedAt: string }> {
  return this.post<{ id: number; flaggedAt: string }>(
    `/purchases/${purchaseId}/flag`,
    { reason },
  );
};

proto.listPriceFlags = async function (
  this: APIClient, status = 'open',
): Promise<PriceFlagsResponse> {
  return this.get<PriceFlagsResponse>(`/admin/price-flags?status=${encodeURIComponent(status)}`);
};

proto.resolvePriceFlag = async function (this: APIClient, flagId: number): Promise<void> {
  const response = await this.fetchWithRetry(
    `${this.baseURL}/admin/price-flags/${flagId}/resolve`,
    { method: 'PATCH' }
  );
  await this.expectNoContent(response);
};

// PSA pending items
proto.listPSAPendingItems = async function (this: APIClient): Promise<{ items: PSAPendingItem[] }> {
  return this.get<{ items: PSAPendingItem[] }>('/admin/psa-sync/pending');
};

proto.assignPSAPendingItem = async function (this: APIClient, id: string, campaignId: string): Promise<unknown> {
  return this.post(`/admin/psa-sync/pending/${encodeURIComponent(id)}/assign`, { campaignId });
};

proto.dismissPSAPendingItem = async function (this: APIClient, id: string): Promise<void> {
  await this.deleteResource(`/admin/psa-sync/pending/${encodeURIComponent(id)}`);
};
