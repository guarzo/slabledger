/**
 * PSA portal campaign sync API methods (Task 8 endpoints).
 */

import type { Campaign, ListPSACampaignsResponse, PSAProposeResponse, PSAPublishResponse } from '../../types/campaigns';
import type { APIClient } from './client';

declare module './client' {
  interface APIClient {
    listPSACampaigns(): Promise<ListPSACampaignsResponse>;
    psaLink(id: string, psaCampaignRequestId: string): Promise<Campaign>;
    psaPropose(id: string): Promise<PSAProposeResponse>;
    psaPublish(id: string, pushId: string): Promise<PSAPublishResponse>;
  }
}

import { APIClient as _APIClient } from './client';
const proto = _APIClient.prototype;

proto.listPSACampaigns = async function (this: APIClient): Promise<ListPSACampaignsResponse> {
  return this.get<ListPSACampaignsResponse>('/psa-campaigns');
};

proto.psaLink = async function (this: APIClient, id: string, psaCampaignRequestId: string): Promise<Campaign> {
  return this.post<Campaign>(`/campaigns/${id}/psa-link`, { psaCampaignRequestId });
};

proto.psaPropose = async function (this: APIClient, id: string): Promise<PSAProposeResponse> {
  return this.post<PSAProposeResponse>(`/campaigns/${id}/psa-propose`, {});
};

proto.psaPublish = async function (this: APIClient, id: string, pushId: string): Promise<PSAPublishResponse> {
  return this.post<PSAPublishResponse>(`/campaigns/${id}/psa-publish`, { pushId });
};
