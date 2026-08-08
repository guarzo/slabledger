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
    psaPublish(id: string, pushId: string, payloadDigest?: string): Promise<PSAPublishResponse>;
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
  return this.post<Campaign>(`/campaigns/${encodeURIComponent(id)}/psa-link`, { psaCampaignRequestId });
};

proto.psaPropose = async function (this: APIClient, id: string): Promise<PSAProposeResponse> {
  return this.post<PSAProposeResponse>(`/campaigns/${encodeURIComponent(id)}/psa-propose`, {});
};

proto.psaProposeCreate = async function (this: APIClient, id: string): Promise<PSAProposeCreateResponse> {
  return this.post<PSAProposeCreateResponse>(`/campaigns/${encodeURIComponent(id)}/psa-propose-create`, {});
};

// payloadDigest binds the approval to the payload the operator was shown: the
// server re-digests the queued row and rejects with 409 if it no longer matches.
// It is optional on the wire so an older client still works, but this client
// always sends it when the propose response or push row carried one.
proto.psaPublish = async function (this: APIClient, id: string, pushId: string, payloadDigest?: string): Promise<PSAPublishResponse> {
  return this.post<PSAPublishResponse>(`/campaigns/${encodeURIComponent(id)}/psa-publish`, { pushId, payloadDigest });
};

proto.listPSAPushes = async function (this: APIClient): Promise<ListPSAPushesResponse> {
  return this.get<ListPSAPushesResponse>('/psa-pushes');
};

// Served from the persisted PSA portal catalog (CatalogStore), not a live portal
// call — the main server has no portal session. See docs/psa-harvester.md.
proto.listPSASubjects = async function (this: APIClient): Promise<PSASubjectsResponse> {
  return this.get<PSASubjectsResponse>('/psa/subjects');
};
