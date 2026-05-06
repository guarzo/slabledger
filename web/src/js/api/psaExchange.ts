/**
 * PSA-Exchange opportunity API methods.
 */

import type {
  PsaExchangeOpportunitiesResponse,
  PsaExchangePolicy,
  PsaExchangePolicySettings,
} from '../../types/psaExchange';
import type { APIClient } from './client';

declare module './client' {
  interface APIClient {
    getPsaExchangeOpportunities(): Promise<PsaExchangeOpportunitiesResponse>;
    getPsaExchangePolicy(): Promise<PsaExchangePolicySettings>;
    updatePsaExchangePolicy(p: PsaExchangePolicy): Promise<PsaExchangePolicySettings>;
  }
}

import { APIClient as _APIClient } from './client';
const proto = _APIClient.prototype;

proto.getPsaExchangeOpportunities = async function (this: APIClient): Promise<PsaExchangeOpportunitiesResponse> {
  return this.get<PsaExchangeOpportunitiesResponse>('/psa-exchange/opportunities');
};

proto.getPsaExchangePolicy = async function (this: APIClient): Promise<PsaExchangePolicySettings> {
  return this.get<PsaExchangePolicySettings>('/psa-exchange/policy');
};

proto.updatePsaExchangePolicy = async function (
  this: APIClient,
  p: PsaExchangePolicy,
): Promise<PsaExchangePolicySettings> {
  return this.put<PsaExchangePolicySettings>('/psa-exchange/policy', p);
};
