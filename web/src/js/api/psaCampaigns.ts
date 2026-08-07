/**
 * PSA portal campaign sync API methods (Task 8 endpoints) and catalog reads.
 */

import type { Campaign, ListPSACampaignsResponse, PSAProposeResponse, PSAProposeCreateResponse, PSAPublishResponse, ListPSAPushesResponse, PSASubjectsResponse } from '../../types/campaigns';
import type { APIClient } from './client';

declare module './client' {
  interface APIClient {
    listPSACampaigns(): Promise<ListPSACampaignsResponse>;
    psaLink(id: string, psaCampaignRequestId: string): Promise<Campaign>;
    psaPropose(id: string): Promise<PSAProposeResponse>;
    psaProposeCreate(id: string): Promise<PSAProposeCreateResponse>;
    psaPublish(id: string, pushId: string): Promise<PSAPublishResponse>;
    listPSAPushes(): Promise<ListPSAPushesResponse>;
    listPSASubjects(): Promise<PSASubjectsResponse>;
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

proto.psaProposeCreate = async function (this: APIClient, id: string): Promise<PSAProposeCreateResponse> {
  return this.post<PSAProposeCreateResponse>(`/campaigns/${id}/psa-propose-create`, {});
};

proto.psaPublish = async function (this: APIClient, id: string, pushId: string): Promise<PSAPublishResponse> {
  return this.post<PSAPublishResponse>(`/campaigns/${id}/psa-publish`, { pushId });
};

proto.listPSAPushes = async function (this: APIClient): Promise<ListPSAPushesResponse> {
  return this.get<ListPSAPushesResponse>('/psa-pushes');
};

// Served from the persisted PSA portal catalog (CatalogStore), not a live portal
// call — the main server has no portal session. See docs/psa-harvester.md.
proto.listPSASubjects = async function (this: APIClient): Promise<PSASubjectsResponse> {
  return this.get<PSASubjectsResponse>('/psa/subjects');
};
