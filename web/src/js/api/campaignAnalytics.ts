/**
 * Campaign analytics, portfolio, tuning, and projection API methods
 */

import type {
  CampaignPNL, ChannelPNL, DailySpend, DaysToSellBucket, InventoryResult,
  TuningResponse, CrackAnalysis, EVPortfolio, ActivationChecklist,
  MonteCarloComparison, CapitalSummary, Invoice, PortfolioHealth,
  ChannelVelocity, PortfolioInsights, SuggestionsResponse,
  RevocationFlag, CapitalTimeline, WeeklyReviewSummary,
} from '../../types/campaigns';
import { APIClient, isAPIError } from './client';

declare module './client' {
  interface APIClient {
    // Campaign analytics
    getCampaignPNL(campaignId: string): Promise<CampaignPNL>;
    getPNLByChannel(campaignId: string): Promise<ChannelPNL[]>;
    getFillRate(campaignId: string, days?: number): Promise<DailySpend[]>;
    getDaysToSell(campaignId: string): Promise<DaysToSellBucket[]>;
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
  }
}

const proto = APIClient.prototype;

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

proto.getCapitalTimeline = async function (this: APIClient): Promise<CapitalTimeline> {
  return this.get<CapitalTimeline>('/portfolio/capital-timeline');
};

proto.getWeeklyReview = async function (this: APIClient): Promise<WeeklyReviewSummary> {
  return this.get<WeeklyReviewSummary>('/portfolio/weekly-review');
};

proto.listRevocationFlags = async function (this: APIClient): Promise<RevocationFlag[]> {
  return this.get<RevocationFlag[]>('/portfolio/revocations');
};

// Campaign tuning
proto.getCampaignTuning = async function (this: APIClient, campaignId: string): Promise<TuningResponse> {
  return this.get<TuningResponse>(`/campaigns/${campaignId}/tuning`);
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
// The server returns HTTP 422 with {error:"insufficient_data", minRequired, available}
// when the campaign has fewer than 10 completed sales. We surface that as a
// MonteCarloComparison with confidence:"insufficient" so UI consumers can branch
// on the payload instead of having to catch an exception.
proto.getProjections = async function (this: APIClient, campaignId: string): Promise<MonteCarloComparison> {
  try {
    return await this.get<MonteCarloComparison>(`/campaigns/${campaignId}/projections`);
  } catch (err) {
    if (isAPIError(err) && err.status === 422) {
      const body = err.data as { available?: number } | undefined;
      return {
        current: emptyMonteCarloResult('current'),
        scenarios: [],
        bestScenarioIndex: 0,
        sampleSize: body?.available ?? 0,
        confidence: 'insufficient',
      };
    }
    throw err;
  }
};

function emptyMonteCarloResult(label: string) {
  return {
    label,
    simulations: 0,
    medianROI: 0,
    p10ROI: 0,
    p90ROI: 0,
    medianProfitCents: 0,
    p10ProfitCents: 0,
    p90ProfitCents: 0,
    medianVolume: 0,
  };
}
