/**
 * Purchase-related API methods: CRUD, price overrides, AI suggestions, cert lookup, quick-add
 */

import type {
  Purchase, Sale, CreatePurchaseInput, CreateSaleInput,
  QuickAddRequest, SellSheet,
} from '../../types/campaigns';
import type { PriceHint } from '../../types/pricing';
import { APIClient, APIError, isAPIError } from './client';

declare module './client' {
  interface APIClient {
    // Purchases
    listPurchases(campaignId: string, limit?: number, offset?: number): Promise<Purchase[]>;
    createPurchase(campaignId: string, input: CreatePurchaseInput): Promise<Purchase>;
    deletePurchase(campaignId: string, purchaseId: string): Promise<void>;

    // Sales
    listSales(campaignId: string, limit?: number, offset?: number): Promise<Sale[]>;
    createSale(campaignId: string, input: CreateSaleInput): Promise<Sale>;
    deleteSale(campaignId: string, purchaseId: string): Promise<void>;

    // Quick-add
    quickAddPurchase(campaignId: string, req: QuickAddRequest): Promise<Purchase>;

    // Sell sheet
    generateGlobalSellSheet(): Promise<SellSheet>;
    generateSellSheet(campaignId: string, purchaseIds: string[]): Promise<SellSheet>;
    generateSelectedSellSheet(purchaseIds: string[]): Promise<SellSheet>;

    // Price override & AI suggestion
    setPriceOverride(purchaseId: string, priceCents: number, source: string): Promise<void>;
    clearPriceOverride(purchaseId: string): Promise<void>;
    acceptAISuggestion(purchaseId: string): Promise<void>;
    dismissAISuggestion(purchaseId: string): Promise<void>;

    // Reassign
    reassignPurchase(purchaseId: string, campaignId: string): Promise<void>;

    // Bulk sales
    createBulkSales(campaignId: string, saleChannel: string, saleDate: string, items: { purchaseId: string; salePriceCents: number }[]): Promise<import('../../types/campaigns').BulkSaleResult>;

    // Price hints
    savePriceHint(hint: PriceHint): Promise<{ status: string }>;

    // Sell sheet item persistence
    getSellSheetItems(): Promise<{ purchaseIds: string[] }>;
    addSellSheetItems(purchaseIds: string[]): Promise<void>;
    removeSellSheetItems(purchaseIds: string[]): Promise<void>;
    clearSellSheetItems(): Promise<void>;
  }
}

const proto = APIClient.prototype;

proto.listPurchases = async function (this: APIClient, campaignId: string, limit = 50, offset = 0): Promise<Purchase[]> {
  return this.get<Purchase[]>(`/campaigns/${campaignId}/purchases?limit=${limit}&offset=${offset}`);
};

proto.createPurchase = async function (this: APIClient, campaignId: string, input: CreatePurchaseInput): Promise<Purchase> {
  return this.post<Purchase>(`/campaigns/${campaignId}/purchases`, input);
};

proto.deletePurchase = async function (this: APIClient, campaignId: string, purchaseId: string): Promise<void> {
  await this.deleteResource(`/campaigns/${campaignId}/purchases/${purchaseId}`);
};

proto.listSales = async function (this: APIClient, campaignId: string, limit = 50, offset = 0): Promise<Sale[]> {
  return this.get<Sale[]>(`/campaigns/${campaignId}/sales?limit=${limit}&offset=${offset}`);
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

proto.generateGlobalSellSheet = async function (this: APIClient): Promise<SellSheet> {
  return this.post<SellSheet>('/sell-sheet', {});
};

proto.generateSellSheet = async function (this: APIClient, campaignId: string, purchaseIds: string[]): Promise<SellSheet> {
  return this.post<SellSheet>(`/campaigns/${campaignId}/sell-sheet`, { purchaseIds });
};

proto.generateSelectedSellSheet = async function (this: APIClient, purchaseIds: string[]): Promise<SellSheet> {
  return this.post<SellSheet>('/portfolio/sell-sheet', { purchaseIds });
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
  const { controller, cleanup } = this.createTimeoutController(this.defaultTimeoutMs);
  try {
    const response = await fetch(
      `${this.baseURL}/purchases/${purchaseId}/accept-ai-suggestion`,
      { method: 'POST', signal: controller.signal, credentials: 'include' }
    );
    cleanup();
    if (!response.ok) {
      let data: { message?: string; code?: string; error?: string } = { error: response.statusText };
      try { data = await response.json(); } catch { /* keep default */ }
      throw new APIError(
        data.message || `API error: ${response.status} ${response.statusText}`,
        response.status,
        data.code,
        data,
      );
    }
    await this.expectNoContent(response);
  } catch (err) {
    cleanup();
    if (isAPIError(err)) {
      throw err;
    }
    if (err instanceof Error && err.name === 'AbortError') {
      throw new APIError('Request was cancelled', 0, 'CANCELLED');
    }
    const message = err instanceof Error ? err.message : 'Network error';
    throw new APIError(message, 0, 'NETWORK_ERROR');
  }
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

proto.createBulkSales = async function (this: APIClient, campaignId: string, saleChannel: string, saleDate: string, items: { purchaseId: string; salePriceCents: number }[]): Promise<import('../../types/campaigns').BulkSaleResult> {
  return this.post<import('../../types/campaigns').BulkSaleResult>(`/campaigns/${campaignId}/sales/bulk`, { saleChannel, saleDate, items });
};

proto.savePriceHint = async function (this: APIClient, hint: PriceHint): Promise<{ status: string }> {
  return this.post<{ status: string }>('/price-hints', hint);
};

// --- Sell sheet item persistence ---

proto.getSellSheetItems = async function (this: APIClient): Promise<{ purchaseIds: string[] }> {
  return this.get<{ purchaseIds: string[] }>('/sell-sheet/items');
};

proto.addSellSheetItems = async function (this: APIClient, purchaseIds: string[]): Promise<void> {
  const response = await this.fetchWithRetry(
    `${this.baseURL}/sell-sheet/items`,
    {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ purchaseIds }),
    },
  );
  await this.expectNoContent(response);
};

proto.removeSellSheetItems = async function (this: APIClient, purchaseIds: string[]): Promise<void> {
  const response = await this.fetchWithRetry(
    `${this.baseURL}/sell-sheet/items`,
    {
      method: 'DELETE',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ purchaseIds }),
    },
  );
  await this.expectNoContent(response);
};

proto.clearSellSheetItems = async function (this: APIClient): Promise<void> {
  await this.deleteResource('/sell-sheet/items/all');
};
