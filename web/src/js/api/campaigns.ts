/**
 * Campaign-related API methods — core CRUD, cards, favorites
 *
 * Sub-modules handle the rest:
 * - campaignPurchases.ts — purchase CRUD, price overrides, sell sheets
 * - campaignAnalytics.ts — analytics, portfolio, tuning, projections
 * - campaignImports.ts — imports, exports, cert entry, price review
 */

import type { FavoriteInput, FavoritesList, ToggleFavoriteResponse } from '../../types/favorites';
import type { Campaign, CreateCampaignInput } from '../../types/campaigns';
import type { CardPricingResponse } from '../../types/pricing';
import type { APIClient, APIRequestOptions, SearchCardsResponse } from './client';

// Side-effect imports: each sub-module patches APIClient.prototype
import './campaignPurchases';
import './campaignAnalytics';
import './campaignImports';

/* ------------------------------------------------------------------ */
/*  Declaration merging — core campaign CRUD + cards + favorites       */
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
    createCampaign(input: CreateCampaignInput): Promise<Campaign>;
    getCampaign(id: string): Promise<Campaign>;
    updateCampaign(id: string, data: Partial<Campaign>): Promise<Campaign>;
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

proto.createCampaign = async function (this: APIClient, input: CreateCampaignInput): Promise<Campaign> {
  return this.post<Campaign>('/campaigns', input);
};

proto.getCampaign = async function (this: APIClient, id: string): Promise<Campaign> {
  return this.get<Campaign>(`/campaigns/${id}`);
};

proto.updateCampaign = async function (this: APIClient, id: string, data: Partial<Campaign>): Promise<Campaign> {
  return this.put<Campaign>(`/campaigns/${id}`, data);
};
