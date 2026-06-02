/**
 * Campaign analytics, portfolio, tuning, and projection API methods
 */

import type {
  CampaignPNL, InventoryResult,
  CrackAnalysis, EVPortfolio,
  CapitalSummary, Invoice, PortfolioHealth,
  ChannelVelocity, PortfolioInsights, SuggestionsResponse,
  RevocationFlag, WeeklyReviewSummary,
} from '../../types/campaigns';
import { APIClient } from './client';

declare module './client' {
  interface APIClient {
    // Campaign analytics
    getCampaignPNL(campaignId: string): Promise<CampaignPNL>;
    getInventory(campaignId: string): Promise<InventoryResult>;
    getGlobalInventory(): Promise<InventoryResult>;

    // Capital & Invoices
    getCapitalSummary(): Promise<CapitalSummary>;
    listInvoices(): Promise<Invoice[]>;
    updateInvoice(id: string, data: Partial<Invoice>): Promise<Invoice>;

    // Portfolio
    getPortfolioHealth(): Promise<PortfolioHealth>;
    getPortfolioChannelVelocity(): Promise<ChannelVelocity[]>;
    getPortfolioInsights(): Promise<PortfolioInsights>;
    getCampaignSuggestions(): Promise<SuggestionsResponse>;
    getWeeklyReview(): Promise<WeeklyReviewSummary>;
    listRevocationFlags(): Promise<RevocationFlag[]>;

    // Crack arbitrage
    getCrackCandidates(campaignId: string): Promise<CrackAnalysis[]>;

    // Expected value
    getExpectedValues(campaignId: string): Promise<EVPortfolio>;
  }
}

const proto = APIClient.prototype;

// Campaign analytics endpoints
proto.getCampaignPNL = async function (this: APIClient, campaignId: string): Promise<CampaignPNL> {
  return this.get<CampaignPNL>(`/campaigns/${campaignId}/pnl`);
};

proto.getInventory = async function (this: APIClient, campaignId: string): Promise<InventoryResult> {
  return this.get<InventoryResult>(`/campaigns/${campaignId}/inventory`);
};

proto.getGlobalInventory = async function (this: APIClient): Promise<InventoryResult> {
  return this.get<InventoryResult>('/inventory');
};

// Capital & Invoice endpoints
proto.getCapitalSummary = async function (this: APIClient): Promise<CapitalSummary> {
  return this.get<CapitalSummary>('/credit/summary');
};

proto.listInvoices = async function (this: APIClient): Promise<Invoice[]> {
  return this.get<Invoice[]>('/credit/invoices');
};

proto.updateInvoice = async function (this: APIClient, id: string, data: Partial<Invoice>): Promise<Invoice> {
  return this.put<Invoice>(`/credit/invoices`, { ...data, id });
};

// Portfolio health
proto.getPortfolioHealth = async function (this: APIClient): Promise<PortfolioHealth> {
  return this.get<PortfolioHealth>('/portfolio/health');
};

proto.getPortfolioChannelVelocity = async function (this: APIClient): Promise<ChannelVelocity[]> {
  return this.get<ChannelVelocity[]>('/portfolio/channel-velocity');
};

proto.getPortfolioInsights = async function (this: APIClient): Promise<PortfolioInsights> {
  return this.get<PortfolioInsights>('/portfolio/insights');
};

proto.getCampaignSuggestions = async function (this: APIClient): Promise<SuggestionsResponse> {
  return this.get<SuggestionsResponse>('/portfolio/suggestions');
};

proto.getWeeklyReview = async function (this: APIClient): Promise<WeeklyReviewSummary> {
  return this.get<WeeklyReviewSummary>('/portfolio/weekly-review');
};

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
