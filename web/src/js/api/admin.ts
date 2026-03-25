/**
 * Admin-related API methods
 */

import type { APIUsageResponse, CacheStatsResponse, PricingDiagnosticsResponse, PriceOverrideStats, CachedAnalysis, AdvisorAnalysisType, AIUsageResponse } from '../../types/apiStatus';
import type { AllowedEmail, AdminUser } from '../../types/admin';
import type { APIClient, CardRequestSubmission } from './client';
import { APIError } from './client';

/* ------------------------------------------------------------------ */
/*  Declaration merging — tells TypeScript about the methods we add   */
/* ------------------------------------------------------------------ */

declare module './client' {
  interface APIClient {
    getAdminAllowlist(): Promise<AllowedEmail[]>;
    addAllowedEmail(email: string, notes?: string): Promise<void>;
    removeAllowedEmail(email: string): Promise<void>;
    getAdminUsers(): Promise<AdminUser[]>;
    getAdminApiUsage(): Promise<APIUsageResponse>;
    getAdminCacheStats(): Promise<CacheStatsResponse>;
    getPricingDiagnostics(): Promise<PricingDiagnosticsResponse>;
    getAdvisorCache(type: AdvisorAnalysisType): Promise<CachedAnalysis>;
    refreshAdvisorCache(type: AdvisorAnalysisType): Promise<void>;
    getPriceOverrideStats(): Promise<PriceOverrideStats>;
    getAIUsage(): Promise<AIUsageResponse>;
    getBackup(): Promise<Blob>;
    getCardRequests(): Promise<CardRequestSubmission[]>;
    submitCardRequest(id: number): Promise<{ status: string; requestId: string }>;
    submitAllCardRequests(): Promise<{ submitted: number; errors: number }>;
  }
}

/* ------------------------------------------------------------------ */
/*  Prototype implementations                                         */
/* ------------------------------------------------------------------ */

import { APIClient as _APIClient } from './client';
const proto = _APIClient.prototype;

// Workaround: ensure APIError is referenced so the import is not pruned.
void APIError;

proto.getAdminAllowlist = async function (this: APIClient): Promise<AllowedEmail[]> {
  return this.get<AllowedEmail[]>('/admin/allowlist');
};

proto.addAllowedEmail = async function (this: APIClient, email: string, notes?: string): Promise<void> {
  await this.post('/admin/allowlist', { email, notes });
};

proto.removeAllowedEmail = async function (this: APIClient, email: string): Promise<void> {
  await this.deleteResource(`/admin/allowlist/${encodeURIComponent(email)}`);
};

proto.getAdminUsers = async function (this: APIClient): Promise<AdminUser[]> {
  return this.get<AdminUser[]>('/admin/users');
};

proto.getAdminApiUsage = async function (this: APIClient): Promise<APIUsageResponse> {
  return this.get<APIUsageResponse>('/admin/api-usage');
};

proto.getAdminCacheStats = async function (this: APIClient): Promise<CacheStatsResponse> {
  return this.get<CacheStatsResponse>('/admin/cache-stats');
};

proto.getPricingDiagnostics = async function (this: APIClient): Promise<PricingDiagnosticsResponse> {
  return this.get<PricingDiagnosticsResponse>('/admin/pricing-diagnostics');
};

proto.getAdvisorCache = async function (this: APIClient, type: AdvisorAnalysisType): Promise<CachedAnalysis> {
  return this.get<CachedAnalysis>(`/advisor/cache/${type}`);
};

proto.refreshAdvisorCache = async function (this: APIClient, type: AdvisorAnalysisType): Promise<void> {
  const response = await this.fetchWithRetry(
    `${this.baseURL}/advisor/refresh/${type}`,
    { method: 'POST' }
  );
  if (response.status === 202) {
    // Server returns 202 with { "status": "running" } — consume and discard.
    await response.text();
    return;
  }
  await this.expectNoContent(response);
};

proto.getPriceOverrideStats = async function (this: APIClient): Promise<PriceOverrideStats> {
  return this.get<PriceOverrideStats>('/admin/price-override-stats');
};

proto.getAIUsage = async function (this: APIClient): Promise<AIUsageResponse> {
  return this.get<AIUsageResponse>('/admin/ai-usage');
};

proto.getBackup = async function (this: APIClient): Promise<Blob> {
  const response = await this.fetchWithRetry(`${this.baseURL}/admin/backup`, {});
  return response.blob();
};

proto.getCardRequests = async function (this: APIClient): Promise<CardRequestSubmission[]> {
  return this.get<CardRequestSubmission[]>('/admin/card-requests');
};

proto.submitCardRequest = async function (this: APIClient, id: number): Promise<{ status: string; requestId: string }> {
  return this.post<{ status: string; requestId: string }>(`/admin/card-requests/${id}/submit`);
};

proto.submitAllCardRequests = async function (this: APIClient): Promise<{ submitted: number; errors: number }> {
  return this.post<{ submitted: number; errors: number }>('/admin/card-requests/submit-all');
};
