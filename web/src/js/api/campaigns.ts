/**
 * Campaign-related API methods
 */

import type { FavoriteInput, FavoritesList, ToggleFavoriteResponse } from '../../types/favorites';
import type {
  Campaign, Purchase, Sale, CreateCampaignInput, CreatePurchaseInput, CreateSaleInput,
  CampaignPNL, ChannelPNL, DailySpend, DaysToSellBucket, AgingItem, CertLookupResult,
  QuickAddRequest, SellSheet, TuningResponse, GlobalImportResult, PSAImportResult,
  ExternalImportResult, CreditSummary, Invoice, PortfolioHealth, ChannelVelocity,
  PortfolioInsights, SuggestionsResponse, RevocationFlag, CapitalTimeline,
  WeeklyReviewSummary, CrackAnalysis, EVPortfolio, ActivationChecklist,
  MonteCarloComparison, BulkSaleResult, ShopifyPriceSyncResponse,
  CertImportResult, EbayExportListResponse, EbayExportGenerateItem,
  OrdersImportResult, OrdersConfirmItem,
} from '../../types/campaigns';
import type { PriceFlagsResponse } from '../../types/campaigns/priceReview';
import type { CardPricingResponse, PriceHint } from '../../types/pricing';
import type { APIClient, APIRequestOptions, SearchCardsResponse } from './client';
import { APIError, isAPIError } from './client';

/* ------------------------------------------------------------------ */
/*  Declaration merging — tells TypeScript about the methods we add   */
/* ------------------------------------------------------------------ */

declare module './client' {
  interface APIClient {
    // Card pricing
    getCardPricing(name: string, set: string, number: string, options?: APIRequestOptions): Promise<CardPricingResponse>;

    // Cards
    searchCards(query: string, limit?: number): Promise<SearchCardsResponse>;

    // Favorites
    getFavorites(page?: number, pageSize?: number): Promise<FavoritesList>;
    toggleFavorite(input: FavoriteInput): Promise<ToggleFavoriteResponse>;

    // Campaign CRUD
    listCampaigns(activeOnly?: boolean): Promise<Campaign[]>;
    deleteCampaign(id: string): Promise<void>;
    deletePurchase(campaignId: string, purchaseId: string): Promise<void>;
    createCampaign(input: CreateCampaignInput): Promise<Campaign>;
    getCampaign(id: string): Promise<Campaign>;
    updateCampaign(id: string, data: Partial<Campaign>): Promise<Campaign>;

    // Purchases
    listPurchases(campaignId: string, limit?: number, offset?: number): Promise<Purchase[]>;
    createPurchase(campaignId: string, input: CreatePurchaseInput): Promise<Purchase>;

    // Sales
    listSales(campaignId: string, limit?: number, offset?: number): Promise<Sale[]>;
    createSale(campaignId: string, input: CreateSaleInput): Promise<Sale>;

    // Campaign analytics
    getCampaignPNL(campaignId: string): Promise<CampaignPNL>;
    getPNLByChannel(campaignId: string): Promise<ChannelPNL[]>;
    getFillRate(campaignId: string, days?: number): Promise<DailySpend[]>;
    getDaysToSell(campaignId: string): Promise<DaysToSellBucket[]>;
    getInventory(campaignId: string): Promise<AgingItem[]>;
    getGlobalInventory(): Promise<AgingItem[]>;

    // Cert lookup
    lookupCert(certNumber: string): Promise<CertLookupResult>;

    // Quick-add
    quickAddPurchase(campaignId: string, req: QuickAddRequest): Promise<Purchase>;

    // Sell sheet
    generateGlobalSellSheet(): Promise<SellSheet>;

    // Global imports
    globalImportCL(file: File): Promise<GlobalImportResult>;
    globalExportCL(missingCLOnly?: boolean): Promise<Blob>;
    reassignPurchase(purchaseId: string, campaignId: string): Promise<void>;

    // Price override & AI suggestion
    setPriceOverride(purchaseId: string, priceCents: number, source: string): Promise<void>;
    clearPriceOverride(purchaseId: string): Promise<void>;
    acceptAISuggestion(purchaseId: string): Promise<void>;
    dismissAISuggestion(purchaseId: string): Promise<void>;

    // PSA / External imports
    globalImportPSA(file: File): Promise<PSAImportResult>;
    globalImportExternal(file: File): Promise<ExternalImportResult>;

    // Orders sales import
    importOrdersSales(file: File): Promise<OrdersImportResult>;
    confirmOrdersSales(items: OrdersConfirmItem[]): Promise<BulkSaleResult>;

    // Shopify
    shopifyPriceSync(items: { certNumber: string; currentPriceCents: number; grader: string }[]): Promise<ShopifyPriceSyncResponse>;

    // Credit & Invoices
    getCreditSummary(): Promise<CreditSummary>;
    listInvoices(): Promise<Invoice[]>;
    updateInvoice(id: string, data: Partial<Invoice>): Promise<Invoice>;

    // Portfolio
    getPortfolioHealth(): Promise<PortfolioHealth>;
    getPortfolioChannelVelocity(): Promise<ChannelVelocity[]>;
    getPortfolioInsights(): Promise<PortfolioInsights>;
    getCampaignSuggestions(): Promise<SuggestionsResponse>;
    getCapitalTimeline(): Promise<CapitalTimeline>;
    getWeeklyReview(): Promise<WeeklyReviewSummary>;
    listRevocationFlags(): Promise<RevocationFlag[]>;

    // Campaign tuning
    getCampaignTuning(campaignId: string): Promise<TuningResponse>;

    // Crack arbitrage
    getCrackCandidates(campaignId: string): Promise<CrackAnalysis[]>;

    // Expected value
    getExpectedValues(campaignId: string): Promise<EVPortfolio>;

    // Activation checklist
    getActivationChecklist(campaignId: string): Promise<ActivationChecklist>;

    // Monte Carlo
    getProjections(campaignId: string): Promise<MonteCarloComparison>;

    // Bulk sales
    createBulkSales(campaignId: string, saleChannel: string, saleDate: string, items: { purchaseId: string; salePriceCents: number }[]): Promise<BulkSaleResult>;

    // Price hints
    savePriceHint(hint: PriceHint): Promise<{ status: string }>;

    // Cert entry & eBay export
    importCerts(certNumbers: string[]): Promise<CertImportResult>;
    listEbayExportItems(flaggedOnly: boolean): Promise<EbayExportListResponse>;
    generateEbayCSV(items: EbayExportGenerateItem[]): Promise<Blob>;

    // Price review & flags
    setReviewedPrice(purchaseId: string, priceCents: number, source: string): Promise<{ success: boolean; reviewedAt: string }>;
    createPriceFlag(purchaseId: string, reason: string): Promise<{ id: number; flaggedAt: string }>;
    listPriceFlags(status?: string): Promise<PriceFlagsResponse>;
    resolvePriceFlag(flagId: number): Promise<void>;
  }
}

/* ------------------------------------------------------------------ */
/*  Prototype implementations                                         */
/* ------------------------------------------------------------------ */

import { APIClient as _APIClient } from './client';
const proto = _APIClient.prototype;

// Card pricing endpoints
proto.getCardPricing = async function (
  this: APIClient, name: string, set: string, number: string, options?: APIRequestOptions,
): Promise<CardPricingResponse> {
  const params = new URLSearchParams({ name, set, number });
  return this.get<CardPricingResponse>(`/cards/pricing?${params.toString()}`, {
    ...options,
    timeoutMs: options?.timeoutMs ?? 90_000,
  });
};

// Cards endpoints
proto.searchCards = async function (this: APIClient, query: string, limit = 10): Promise<SearchCardsResponse> {
  return this.get<SearchCardsResponse>(`/cards/search?q=${encodeURIComponent(query)}&limit=${limit}`);
};

// Favorites endpoints
proto.getFavorites = async function (this: APIClient, page = 1, pageSize = 100): Promise<FavoritesList> {
  return this.get<FavoritesList>(`/favorites?page=${page}&page_size=${pageSize}`);
};

proto.toggleFavorite = async function (this: APIClient, input: FavoriteInput): Promise<ToggleFavoriteResponse> {
  return this.post<ToggleFavoriteResponse>('/favorites/toggle', input);
};

// Campaign endpoints
proto.listCampaigns = async function (this: APIClient, activeOnly = false): Promise<Campaign[]> {
  const params = activeOnly ? '?activeOnly=true' : '';
  return this.get<Campaign[]>(`/campaigns${params}`);
};

proto.deleteCampaign = async function (this: APIClient, id: string): Promise<void> {
  await this.deleteResource(`/campaigns/${id}`);
};

proto.deletePurchase = async function (this: APIClient, campaignId: string, purchaseId: string): Promise<void> {
  await this.deleteResource(`/campaigns/${campaignId}/purchases/${purchaseId}`);
};

proto.createCampaign = async function (this: APIClient, input: CreateCampaignInput): Promise<Campaign> {
  return this.post<Campaign>('/campaigns', input);
};

proto.getCampaign = async function (this: APIClient, id: string): Promise<Campaign> {
  return this.get<Campaign>(`/campaigns/${id}`);
};

proto.updateCampaign = async function (this: APIClient, id: string, data: Partial<Campaign>): Promise<Campaign> {
  return this.put<Campaign>(`/campaigns/${id}`, data);
};

proto.listPurchases = async function (this: APIClient, campaignId: string, limit = 50, offset = 0): Promise<Purchase[]> {
  return this.get<Purchase[]>(`/campaigns/${campaignId}/purchases?limit=${limit}&offset=${offset}`);
};

proto.createPurchase = async function (this: APIClient, campaignId: string, input: CreatePurchaseInput): Promise<Purchase> {
  return this.post<Purchase>(`/campaigns/${campaignId}/purchases`, input);
};

proto.listSales = async function (this: APIClient, campaignId: string, limit = 50, offset = 0): Promise<Sale[]> {
  return this.get<Sale[]>(`/campaigns/${campaignId}/sales?limit=${limit}&offset=${offset}`);
};

proto.createSale = async function (this: APIClient, campaignId: string, input: CreateSaleInput): Promise<Sale> {
  return this.post<Sale>(`/campaigns/${campaignId}/sales`, input);
};

// Campaign analytics endpoints
proto.getCampaignPNL = async function (this: APIClient, campaignId: string): Promise<CampaignPNL> {
  return this.get<CampaignPNL>(`/campaigns/${campaignId}/pnl`);
};

proto.getPNLByChannel = async function (this: APIClient, campaignId: string): Promise<ChannelPNL[]> {
  return this.get<ChannelPNL[]>(`/campaigns/${campaignId}/pnl-by-channel`);
};

proto.getFillRate = async function (this: APIClient, campaignId: string, days = 30): Promise<DailySpend[]> {
  return this.get<DailySpend[]>(`/campaigns/${campaignId}/fill-rate?days=${days}`);
};

proto.getDaysToSell = async function (this: APIClient, campaignId: string): Promise<DaysToSellBucket[]> {
  return this.get<DaysToSellBucket[]>(`/campaigns/${campaignId}/days-to-sell`);
};

proto.getInventory = async function (this: APIClient, campaignId: string): Promise<AgingItem[]> {
  return this.get<AgingItem[]>(`/campaigns/${campaignId}/inventory`);
};

proto.getGlobalInventory = async function (this: APIClient): Promise<AgingItem[]> {
  return this.get<AgingItem[]>('/inventory');
};

// Cert lookup
proto.lookupCert = async function (this: APIClient, certNumber: string): Promise<CertLookupResult> {
  return this.get<CertLookupResult>(`/certs/${certNumber}`);
};

// Quick-add purchase from cert
proto.quickAddPurchase = async function (this: APIClient, campaignId: string, req: QuickAddRequest): Promise<Purchase> {
  return this.post<Purchase>(`/campaigns/${campaignId}/purchases/quick-add`, req);
};

// Generate global sell sheet (all unsold inventory)
proto.generateGlobalSellSheet = async function (this: APIClient): Promise<SellSheet> {
  return this.post<SellSheet>('/sell-sheet', {});
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

// Price override & AI suggestion methods
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
  // No retry — accepting a suggestion is not idempotent (second attempt would
  // fail with "no AI suggestion to accept" if the first request succeeded).
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

// PSA CSV import (global) — no retry for file uploads
proto.globalImportPSA = async function (this: APIClient, file: File): Promise<PSAImportResult> {
  return this.uploadFile<PSAImportResult>('/purchases/import-psa', file);
};

// External (Shopify) CSV import
proto.globalImportExternal = async function (this: APIClient, file: File): Promise<ExternalImportResult> {
  return this.uploadFile<ExternalImportResult>('/purchases/import-external', file);
};

// Orders sales import (upload CSV, get categorized matches)
proto.importOrdersSales = async function (this: APIClient, file: File): Promise<OrdersImportResult> {
  return this.uploadFile<OrdersImportResult>('/purchases/import-orders', file);
};

// Orders sales confirm (create sales for confirmed matches)
proto.confirmOrdersSales = async function (this: APIClient, items: OrdersConfirmItem[]): Promise<BulkSaleResult> {
  return this.post<BulkSaleResult>('/purchases/import-orders/confirm', items);
};

// Shopify price sync
proto.shopifyPriceSync = async function (this: APIClient, items: { certNumber: string; currentPriceCents: number; grader: string }[]): Promise<ShopifyPriceSyncResponse> {
  return this.post<ShopifyPriceSyncResponse>('/shopify/price-sync', { items });
};

// Credit & Invoice endpoints
proto.getCreditSummary = async function (this: APIClient): Promise<CreditSummary> {
  return this.get<CreditSummary>('/credit/summary');
};

proto.listInvoices = async function (this: APIClient): Promise<Invoice[]> {
  return this.get<Invoice[]>('/credit/invoices');
};

proto.updateInvoice = async function (this: APIClient, id: string, data: Partial<Invoice>): Promise<Invoice> {
  return this.put<Invoice>(`/credit/invoices`, { id, ...data });
};

// Portfolio health
proto.getPortfolioHealth = async function (this: APIClient): Promise<PortfolioHealth> {
  return this.get<PortfolioHealth>('/portfolio/health');
};

// Portfolio channel velocity
proto.getPortfolioChannelVelocity = async function (this: APIClient): Promise<ChannelVelocity[]> {
  return this.get<ChannelVelocity[]>('/portfolio/channel-velocity');
};

// Campaign tuning
proto.getCampaignTuning = async function (this: APIClient, campaignId: string): Promise<TuningResponse> {
  return this.get<TuningResponse>(`/campaigns/${campaignId}/tuning`);
};

// Portfolio insights
proto.getPortfolioInsights = async function (this: APIClient): Promise<PortfolioInsights> {
  return this.get<PortfolioInsights>('/portfolio/insights');
};

// Campaign suggestions
proto.getCampaignSuggestions = async function (this: APIClient): Promise<SuggestionsResponse> {
  return this.get<SuggestionsResponse>('/portfolio/suggestions');
};

// Capital timeline
proto.getCapitalTimeline = async function (this: APIClient): Promise<CapitalTimeline> {
  return this.get<CapitalTimeline>('/portfolio/capital-timeline');
};

// Weekly review
proto.getWeeklyReview = async function (this: APIClient): Promise<WeeklyReviewSummary> {
  return this.get<WeeklyReviewSummary>('/portfolio/weekly-review');
};

// Revocation flags
proto.listRevocationFlags = async function (this: APIClient): Promise<RevocationFlag[]> {
  return this.get<RevocationFlag[]>('/portfolio/revocations');
};

// Crack arbitrage
proto.getCrackCandidates = async function (this: APIClient, campaignId: string): Promise<CrackAnalysis[]> {
  return this.get<CrackAnalysis[]>(`/campaigns/${campaignId}/crack-candidates`);
};

// Expected value
proto.getExpectedValues = async function (this: APIClient, campaignId: string): Promise<EVPortfolio> {
  return this.get<EVPortfolio>(`/campaigns/${campaignId}/expected-values`);
};

// Activation checklist
proto.getActivationChecklist = async function (this: APIClient, campaignId: string): Promise<ActivationChecklist> {
  return this.get<ActivationChecklist>(`/campaigns/${campaignId}/activation-checklist`);
};

// Monte Carlo projections
proto.getProjections = async function (this: APIClient, campaignId: string): Promise<MonteCarloComparison> {
  return this.get<MonteCarloComparison>(`/campaigns/${campaignId}/projections`);
};

// Bulk sales
proto.createBulkSales = async function (this: APIClient, campaignId: string, saleChannel: string, saleDate: string, items: { purchaseId: string; salePriceCents: number }[]): Promise<BulkSaleResult> {
  return this.post<BulkSaleResult>(`/campaigns/${campaignId}/sales/bulk`, { saleChannel, saleDate, items });
};

// Price hints endpoints
proto.savePriceHint = async function (this: APIClient, hint: PriceHint): Promise<{ status: string }> {
  return this.post<{ status: string }>('/price-hints', hint);
};

// Cert entry
proto.importCerts = async function (
  this: APIClient, certNumbers: string[],
): Promise<CertImportResult> {
  return this.post<CertImportResult>('/purchases/import-certs', { certNumbers });
};

// eBay export
proto.listEbayExportItems = async function (
  this: APIClient, flaggedOnly: boolean,
): Promise<EbayExportListResponse> {
  const params = flaggedOnly ? '?flagged_only=true' : '';
  return this.get<EbayExportListResponse>(`/purchases/export-ebay${params}`);
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

proto.resolvePriceFlag = async function (
  this: APIClient, flagId: number,
): Promise<void> {
  const response = await this.fetchWithRetry(
    `${this.baseURL}/admin/price-flags/${flagId}/resolve`,
    { method: 'PATCH' }
  );
  await this.expectNoContent(response);
};

proto.generateEbayCSV = async function (
  this: APIClient, items: EbayExportGenerateItem[],
): Promise<Blob> {
  // No retry — export clears ebay_export_flagged_at on the server; retrying
  // a failed request could produce duplicate side effects.
  const { controller, cleanup } = this.createTimeoutController(this.defaultTimeoutMs);
  try {
    const response = await fetch(
      `${this.baseURL}/purchases/export-ebay/generate`,
      {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ items }),
        signal: controller.signal,
        credentials: 'include',
      },
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
    return response.blob();
  } catch (err) {
    cleanup();
    if (isAPIError(err)) throw err;
    if (err instanceof Error && err.name === 'AbortError') {
      throw new APIError('Request was cancelled', 0, 'CANCELLED');
    }
    const message = err instanceof Error ? err.message : 'Network error';
    throw new APIError(message, 0, 'NETWORK_ERROR');
  }
};
