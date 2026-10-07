/**
 * Purchase-related API methods: CRUD, price overrides, AI suggestions, cert lookup, quick-add
 */

import type {
  Purchase, Sale, CreateSaleInput,
  QuickAddRequest, ConfirmReturnRequest, ConfirmedReturnState, DHSaleCheck,
} from '../../types/campaigns';
import type { PriceHint } from '../../types/pricing';
import { APIClient } from './client';

declare module './client' {
  interface APIClient {
    // Purchases
    deletePurchase(campaignId: string, purchaseId: string): Promise<void>;

    // Sales
    createSale(campaignId: string, input: CreateSaleInput): Promise<Sale>;
    deleteSale(campaignId: string, purchaseId: string): Promise<void>;
    getConfirmedReturnState(purchaseId: string): Promise<ConfirmedReturnState>;
    getDHSaleCheck(purchaseId: string): Promise<DHSaleCheck>;
    confirmPurchaseReturn(purchaseId: string, request: ConfirmReturnRequest): Promise<ConfirmedReturnState>;

    // Quick-add
    quickAddPurchase(campaignId: string, req: QuickAddRequest): Promise<Purchase>;

    // Price override & AI suggestion
    setPriceOverride(purchaseId: string, priceCents: number, source: string): Promise<void>;
    clearPriceOverride(purchaseId: string): Promise<void>;
    acceptAISuggestion(purchaseId: string): Promise<void>;
    dismissAISuggestion(purchaseId: string): Promise<void>;

    // DH listing (manual transition from in_stock to listed)
    listPurchaseOnDH(purchaseId: string): Promise<{ listed: number; synced: number; skipped: number; total: number }>;

    // Bulk sales
    createBulkSales(campaignId: string, saleChannel: string, saleDate: string, items: import('../../types/campaigns').BulkSaleItemInput[]): Promise<import('../../types/campaigns').BulkSaleResult>;

    // Price hints
    savePriceHint(hint: PriceHint): Promise<{ status: string }>;
  }
}

const proto = APIClient.prototype;

proto.deletePurchase = async function (this: APIClient, campaignId: string, purchaseId: string): Promise<void> {
  await this.deleteResource(`/campaigns/${encodeURIComponent(campaignId)}/purchases/${encodeURIComponent(purchaseId)}`);
};

proto.createSale = async function (this: APIClient, campaignId: string, input: CreateSaleInput): Promise<Sale> {
  return this.post<Sale>(`/campaigns/${encodeURIComponent(campaignId)}/sales`, input);
};

proto.deleteSale = async function (this: APIClient, campaignId: string, purchaseId: string): Promise<void> {
  await this.deleteResource(`/campaigns/${encodeURIComponent(campaignId)}/purchases/${encodeURIComponent(purchaseId)}/sale`);
};

proto.getConfirmedReturnState = async function (this: APIClient, purchaseId: string): Promise<ConfirmedReturnState> {
  return this.get<ConfirmedReturnState>(`/purchases/${encodeURIComponent(purchaseId)}/confirmed-return`);
};

proto.getDHSaleCheck = async function (this: APIClient, purchaseId: string): Promise<DHSaleCheck> {
  return this.get<DHSaleCheck>(`/purchases/${encodeURIComponent(purchaseId)}/dh-sale-check`);
};

proto.confirmPurchaseReturn = async function (this: APIClient, purchaseId: string, request: ConfirmReturnRequest): Promise<ConfirmedReturnState> {
  const endpoint = `/purchases/${encodeURIComponent(purchaseId)}/confirm-return`;
  if (request.operationId) return this.post<ConfirmedReturnState>(endpoint, request);
  // A new return has no server-issued operation ID yet. A lost response or 5xx
  // may mean the mutation ran; start on the final transport attempt so the hook
  // reads durable state before any further POST. Keyed replays keep normal retry.
  const response = await this.fetchWithRetry(`${this.baseURL}${endpoint}`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(request),
  }, this.maxRetries);
  return response.json();
};

proto.quickAddPurchase = async function (this: APIClient, campaignId: string, req: QuickAddRequest): Promise<Purchase> {
  return this.post<Purchase>(`/campaigns/${encodeURIComponent(campaignId)}/purchases/quick-add`, req);
};

proto.setPriceOverride = async function (this: APIClient, purchaseId: string, priceCents: number, source: string): Promise<void> {
  const response = await this.fetchWithRetry(
    `${this.baseURL}/purchases/${encodeURIComponent(purchaseId)}/price-override`,
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
    `${this.baseURL}/purchases/${encodeURIComponent(purchaseId)}/price-override`,
    { method: 'DELETE' }
  );
  await this.expectNoContent(response);
};

proto.acceptAISuggestion = async function (this: APIClient, purchaseId: string): Promise<void> {
  const response = await this.fetchWithRetry(
    `${this.baseURL}/purchases/${encodeURIComponent(purchaseId)}/accept-ai-suggestion`,
    { method: 'POST' },
  );
  await this.expectNoContent(response);
};

proto.dismissAISuggestion = async function (this: APIClient, purchaseId: string): Promise<void> {
  const response = await this.fetchWithRetry(
    `${this.baseURL}/purchases/${encodeURIComponent(purchaseId)}/ai-suggestion`,
    { method: 'DELETE' }
  );
  await this.expectNoContent(response);
};

proto.listPurchaseOnDH = async function (this: APIClient, purchaseId: string): Promise<{ listed: number; synced: number; skipped: number; total: number }> {
  // An uncertain response may have dispatched to DH. Never automatically
  // replay this unkeyed mutation; the server retains an attempt for review.
  const response = await this.fetchWithRetry(
    `${this.baseURL}/purchases/${encodeURIComponent(purchaseId)}/list-on-dh`,
    { method: 'POST', headers: { 'Content-Type': 'application/json' } },
    this.maxRetries,
    { timeoutMs: 90_000 },
  );
  return response.json() as Promise<{ listed: number; synced: number; skipped: number; total: number }>;
};

proto.createBulkSales = async function (this: APIClient, campaignId: string, saleChannel: string, saleDate: string, items: import('../../types/campaigns').BulkSaleItemInput[]): Promise<import('../../types/campaigns').BulkSaleResult> {
  return this.post<import('../../types/campaigns').BulkSaleResult>(`/campaigns/${encodeURIComponent(campaignId)}/sales/bulk`, { saleChannel, saleDate, items });
};

proto.savePriceHint = async function (this: APIClient, hint: PriceHint): Promise<{ status: string }> {
  return this.post<{ status: string }>('/price-hints', hint);
};
