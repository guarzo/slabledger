/**
 * PSA-Exchange opportunity API methods.
 */

import type { PsaExchangeOpportunitiesResponse } from '../../types/psaExchange';
import type { APIClient } from './client';

declare module './client' {
  interface APIClient {
    getPsaExchangeOpportunities(): Promise<PsaExchangeOpportunitiesResponse>;
  }
}

import { APIClient as _APIClient } from './client';
const proto = _APIClient.prototype;

proto.getPsaExchangeOpportunities = async function (this: APIClient): Promise<PsaExchangeOpportunitiesResponse> {
  return this.get<PsaExchangeOpportunitiesResponse>('/psa-exchange/opportunities');
};
