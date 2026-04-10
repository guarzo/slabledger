/**
 * Admin-related API methods
 */

import type { APIUsageResponse, CacheStatsResponse, PricingDiagnosticsResponse, PriceOverrideStats, CachedAnalysis, AdvisorAnalysisType, AIUsageResponse, DHStatusResponse, DHBulkMatchResponse, DHUnmatchedResponse, DHFixMatchRequest, DHFixMatchResponse, DHSelectMatchRequest, DHPushConfig } from '../../types/apiStatus';
import type { AllowedEmail, AdminUser, CLStatusResponse, CLSyncResult, MMStatusResponse, MMSyncResult, PSASyncStatusResponse } from '../../types/admin';
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
    getCardLadderStatus(): Promise<CLStatusResponse>;
    saveCardLadderConfig(config: { email: string; password: string; collectionId: string; firebaseApiKey: string }): Promise<{ status: string }>;
    triggerCardLadderRefresh(): Promise<{ status: string }>;
    syncCardLadderCollection(): Promise<CLSyncResult>;
    getMarketMoversStatus(): Promise<MMStatusResponse>;
    saveMarketMoversConfig(config: { username: string; password: string }): Promise<{ status: string }>;
    triggerMarketMoversRefresh(): Promise<{ status: string }>;
    syncMarketMoversCollection(): Promise<MMSyncResult>;
    getDHStatus(): Promise<DHStatusResponse>;
    triggerDHBulkMatch(): Promise<DHBulkMatchResponse>;
    getDHUnmatched(): Promise<DHUnmatchedResponse>;
    fixDHMatch(req: DHFixMatchRequest): Promise<DHFixMatchResponse>;
    selectDHMatch(req: DHSelectMatchRequest): Promise<DHFixMatchResponse>;
    approveDHPush(purchaseId: string): Promise<{ status: string }>;
    getDHPushConfig(): Promise<DHPushConfig>;
    saveDHPushConfig(config: DHPushConfig): Promise<DHPushConfig>;
    getPSASyncStatus(): Promise<PSASyncStatusResponse>;
    triggerPSASyncRefresh(): Promise<{ status: string }>;
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

proto.getCardLadderStatus = async function (this: APIClient) {
  return this.get<CLStatusResponse>('/admin/cardladder/status');
};

proto.saveCardLadderConfig = async function (this: APIClient, config: { email: string; password: string; collectionId: string; firebaseApiKey: string }) {
  return this.post<{ status: string }>('/admin/cardladder/config', config);
};

proto.triggerCardLadderRefresh = async function (this: APIClient) {
  return this.post<{ status: string }>('/admin/cardladder/refresh');
};

proto.syncCardLadderCollection = async function (this: APIClient) {
  return this.post<CLSyncResult>('/admin/cardladder/sync-to-cl');
};

proto.getMarketMoversStatus = async function (this: APIClient) {
  return this.get<MMStatusResponse>('/admin/marketmovers/status');
};

proto.saveMarketMoversConfig = async function (this: APIClient, config: { username: string; password: string }) {
  return this.post<{ status: string }>('/admin/marketmovers/config', config);
};

proto.triggerMarketMoversRefresh = async function (this: APIClient) {
  return this.post<{ status: string }>('/admin/marketmovers/refresh');
};

proto.syncMarketMoversCollection = async function (this: APIClient) {
  return this.post<MMSyncResult>('/admin/marketmovers/sync-collection');
};

proto.getDHStatus = async function (this: APIClient): Promise<DHStatusResponse> {
  return this.get<DHStatusResponse>('/dh/status');
};

proto.triggerDHBulkMatch = async function (this: APIClient): Promise<DHBulkMatchResponse> {
  return this.post<DHBulkMatchResponse>('/dh/match');
};

proto.getDHUnmatched = async function (this: APIClient): Promise<DHUnmatchedResponse> {
  return this.get<DHUnmatchedResponse>('/dh/unmatched');
};

proto.fixDHMatch = async function (this: APIClient, req: DHFixMatchRequest): Promise<DHFixMatchResponse> {
  return this.post<DHFixMatchResponse>('/dh/fix-match', req);
};

proto.selectDHMatch = async function (this: APIClient, req: DHSelectMatchRequest): Promise<DHFixMatchResponse> {
  return this.post<DHFixMatchResponse>('/dh/select-match', req);
};

proto.approveDHPush = async function (this: APIClient, purchaseId: string): Promise<{ status: string }> {
  return this.post<{ status: string }>(`/dh/approve/${encodeURIComponent(purchaseId)}`);
};

proto.getDHPushConfig = async function (this: APIClient): Promise<DHPushConfig> {
  return this.get<DHPushConfig>('/admin/dh-push-config');
};

proto.saveDHPushConfig = async function (this: APIClient, config: DHPushConfig): Promise<DHPushConfig> {
  return this.put<DHPushConfig>('/admin/dh-push-config', config);
};

proto.getPSASyncStatus = async function (this: APIClient): Promise<PSASyncStatusResponse> {
  return this.get<PSASyncStatusResponse>('/admin/psa-sync/status');
};

proto.triggerPSASyncRefresh = async function (this: APIClient) {
  return this.post<{ status: string }>('/admin/psa-sync/refresh');
};
