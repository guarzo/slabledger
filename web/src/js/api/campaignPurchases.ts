/**
 * Purchase-related API methods: CRUD, price overrides, AI suggestions, cert lookup, quick-add
 */

import type {
  Purchase, Sale, CreatePurchaseInput, CreateSaleInput,
  QuickAddRequest,
} from '../../types/campaigns';
import type { PriceHint } from '../../types/pricing';
import { APIClient } from './client';

declare module './client' {
  interface APIClient {
    // Purchases
    createPurchase(campaignId: string, input: CreatePurchaseInput): Promise<Purchase>;
    deletePurchase(campaignId: string, purchaseId: string): Promise<void>;

    // Sales
    createSale(campaignId: string, input: CreateSaleInput): Promise<Sale>;
    deleteSale(campaignId: string, purchaseId: string): Promise<void>;

    // Quick-add
    quickAddPurchase(campaignId: string, req: QuickAddRequest): Promise<Purchase>;

    // Price override & AI suggestion
    setPriceOverride(purchaseId: string, priceCents: number, source: string): Promise<void>;
    clearPriceOverride(purchaseId: string): Promise<void>;
    acceptAISuggestion(purchaseId: string): Promise<void>;
    dismissAISuggestion(purchaseId: string): Promise<void>;

    // Reassign
    reassignPurchase(purchaseId: string, campaignId: string): Promise<void>;

    // DH listing (manual transition from in_stock to listed)
    listPurchaseOnDH(purchaseId: string): Promise<{ listed: number; synced: number; skipped: number; total: number }>;

    // Bulk sales
    createBulkSales(campaignId: string, saleChannel: string, saleDate: string, items: { purchaseId: string; salePriceCents: number }[]): Promise<import('../../types/campaigns').BulkSaleResult>;

    // Price hints
    savePriceHint(hint: PriceHint): Promise<{ status: string }>;
  }
}

const proto = APIClient.prototype;

proto.createPurchase = async function (this: APIClient, campaignId: string, input: CreatePurchaseInput): Promise<Purchase> {
  return this.post<Purchase>(`/campaigns/${campaignId}/purchases`, input);
};

proto.deletePurchase = async function (this: APIClient, campaignId: string, purchaseId: string): Promise<void> {
  await this.deleteResource(`/campaigns/${campaignId}/purchases/${purchaseId}`);
};

proto.createSale = async function (this: APIClient, campaignId: string, input: CreateSaleInput): Promise<Sale> {
  return this.post<Sale>(`/campaigns/${campaignId}/sales`, input);
};

proto.deleteSale = async function (this: APIClient, campaignId: string, purchaseId: string): Promise<void> {
  await this.deleteResource(`/campaigns/${campaignId}/purchases/${purchaseId}/sale`);
};

proto.quickAddPurchase = async function (this: APIClient, campaignId: string, req: QuickAddRequest): Promise<Purchase> {
  return this.post<Purchase>(`/campaigns/${campaignId}/purchases/quick-add`, req);
};

proto.setPriceOverride = async function (this: APIClient, purchaseId: string, priceCents: number, source: string): Promise<void> {
  const response = await this.fetchWithRetry(
    `${this.baseURL}/purchases/${purchaseId}/price-override`,
    {
      method: 'PATCH',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ priceCents, source }),
    }
  );
  await this.expectNoContent(response);
};

proto.clearPriceOverride = async function (this: APIClient, purchaseId: string): Promise<void> {
  const response = await this.fetchWithRetry(
    `${this.baseURL}/purchases/${purchaseId}/price-override`,
    { method: 'DELETE' }
  );
  await this.expectNoContent(response);
};

proto.acceptAISuggestion = async function (this: APIClient, purchaseId: string): Promise<void> {
  const response = await this.fetchWithRetry(
    `${this.baseURL}/purchases/${purchaseId}/accept-ai-suggestion`,
    { method: 'POST' },
  );
  await this.expectNoContent(response);
};

proto.dismissAISuggestion = async function (this: APIClient, purchaseId: string): Promise<void> {
  const response = await this.fetchWithRetry(
    `${this.baseURL}/purchases/${purchaseId}/ai-suggestion`,
    { method: 'DELETE' }
  );
  await this.expectNoContent(response);
};

proto.reassignPurchase = async function (this: APIClient, purchaseId: string, campaignId: string): Promise<void> {
  const response = await this.fetchWithRetry(
    `${this.baseURL}/purchases/${purchaseId}/campaign`,
    {
      method: 'PATCH',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ campaignId }),
    }
  );
  await this.expectNoContent(response);
};

proto.listPurchaseOnDH = async function (this: APIClient, purchaseId: string): Promise<{ listed: number; synced: number; skipped: number; total: number }> {
  return this.post<{ listed: number; synced: number; skipped: number; total: number }>(`/purchases/${encodeURIComponent(purchaseId)}/list-on-dh`);
};

proto.createBulkSales = async function (this: APIClient, campaignId: string, saleChannel: string, saleDate: string, items: { purchaseId: string; salePriceCents: number }[]): Promise<import('../../types/campaigns').BulkSaleResult> {
  return this.post<import('../../types/campaigns').BulkSaleResult>(`/campaigns/${campaignId}/sales/bulk`, { saleChannel, saleDate, items });
};

proto.savePriceHint = async function (this: APIClient, hint: PriceHint): Promise<{ status: string }> {
  return this.post<{ status: string }>('/price-hints', hint);
};
