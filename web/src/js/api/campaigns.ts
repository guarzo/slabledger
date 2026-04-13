/**
 * Campaign-related API methods — core CRUD
 *
 * Sub-modules handle the rest:
 * - campaignPurchases.ts — purchase CRUD, price overrides, sell sheets
 * - campaignAnalytics.ts — analytics, portfolio, tuning, projections
 * - campaignImports.ts — imports, exports, cert entry, price review
 */

import type { Campaign, CreateCampaignInput } from '../../types/campaigns';
import type { APIClient } from './client';

// Side-effect imports: each sub-module patches APIClient.prototype
import './campaignPurchases';
import './campaignAnalytics';
import './campaignImports';

/* ------------------------------------------------------------------ */
/*  Declaration merging — core campaign CRUD                           */
/* ------------------------------------------------------------------ */

declare module './client' {
  interface APIClient {
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
