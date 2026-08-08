/**
 * Campaign analytics, portfolio, tuning, and projection API methods
 */

import type {
  CampaignPNL, InventoryResult,
  EVPortfolio,
  CapitalSummary, Invoice, PortfolioHealth,
  WeeklyReviewSummary,
} from '../../types/campaigns';
import { APIClient } from './client';

declare module './client' {
  interface APIClient {
    // Campaign analytics
    getCampaignPNL(campaignId: string): Promise<CampaignPNL>;
    getGlobalInventory(): Promise<InventoryResult>;

    // Capital & Invoices
    getCapitalSummary(): Promise<CapitalSummary>;
    listInvoices(): Promise<Invoice[]>;
    updateInvoice(id: string, data: Partial<Invoice>): Promise<Invoice>;

    // Portfolio
    getPortfolioHealth(): Promise<PortfolioHealth>;
    getWeeklyReview(): Promise<WeeklyReviewSummary>;

    // Expected value
    getExpectedValues(campaignId: string): Promise<EVPortfolio>;
  }
}

const proto = APIClient.prototype;

// Campaign analytics endpoints
proto.getCampaignPNL = async function (this: APIClient, campaignId: string): Promise<CampaignPNL> {
  return this.get<CampaignPNL>(`/campaigns/${encodeURIComponent(campaignId)}/pnl`);
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

proto.getWeeklyReview = async function (this: APIClient): Promise<WeeklyReviewSummary> {
  return this.get<WeeklyReviewSummary>('/portfolio/weekly-review');
};

// Expected value
proto.getExpectedValues = async function (this: APIClient, campaignId: string): Promise<EVPortfolio> {
  return this.get<EVPortfolio>(`/campaigns/${encodeURIComponent(campaignId)}/expected-values`);
};
