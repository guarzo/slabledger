/**
 * Admin-related API methods
 */

import type { APIUsageResponse, PricingDiagnosticsResponse, PriceOverrideStats, AIUsageResponse, DHStatusResponse, DHBulkMatchResponse, DHFixMatchRequest, DHFixMatchResponse, DHRetryMatchResponse, DHPushConfig, DHReconcileTriggerResult } from '../../types/apiStatus';
import type { AllowedEmail, AdminUser, CLStatusResponse, CLSyncResult, IntegrationFailuresReport, MMStatusResponse, MMSyncResult, PSASyncStatusResponse } from '../../types/admin';
import type { APIClient } from './client';
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
    getPricingDiagnostics(): Promise<PricingDiagnosticsResponse>;
    getPriceOverrideStats(): Promise<PriceOverrideStats>;
    getAIUsage(): Promise<AIUsageResponse>;
    getBackup(): Promise<Blob>;
    getCardLadderStatus(): Promise<CLStatusResponse>;
    getCardLadderFailures(limit?: number): Promise<IntegrationFailuresReport>;
    saveCardLadderConfig(config: { email: string; password: string; collectionId: string; firebaseApiKey: string }): Promise<{ status: string }>;
    triggerCardLadderRefresh(): Promise<{ status: string }>;
    syncCardLadderCollection(): Promise<CLSyncResult>;
    getMarketMoversStatus(): Promise<MMStatusResponse>;
    getMarketMoversFailures(limit?: number): Promise<IntegrationFailuresReport>;
    saveMarketMoversConfig(config: { username: string; password: string }): Promise<{ status: string }>;
    triggerMarketMoversRefresh(): Promise<{ status: string }>;
    syncMarketMoversCollection(): Promise<MMSyncResult>;
    getDHStatus(): Promise<DHStatusResponse>;
    triggerDHBulkMatch(): Promise<DHBulkMatchResponse>;
    fixDHMatch(req: DHFixMatchRequest): Promise<DHFixMatchResponse>;
    retryDHMatch(purchaseId: string): Promise<DHRetryMatchResponse>;
    unmatchDH(purchaseId: string): Promise<{ status: string }>;
    dismissDHMatch(purchaseId: string): Promise<{ status: string }>;
    undismissDHMatch(purchaseId: string): Promise<{ status: string }>;
    approveDHPush(purchaseId: string): Promise<{ status: string }>;
    getDHPushConfig(): Promise<DHPushConfig>;
    saveDHPushConfig(config: DHPushConfig): Promise<DHPushConfig>;
    getPSASyncStatus(): Promise<PSASyncStatusResponse>;
    triggerPSASyncRefresh(): Promise<{ status: string }>;
    triggerDHReconcile(): Promise<DHReconcileTriggerResult>;
    getDHTombstoneCount(): Promise<{ count: number }>;
    clearDHTombstones(): Promise<{ cleared: number }>;
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

proto.getPricingDiagnostics = async function (this: APIClient): Promise<PricingDiagnosticsResponse> {
  return this.get<PricingDiagnosticsResponse>('/admin/pricing-diagnostics');
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

proto.getCardLadderStatus = async function (this: APIClient) {
  return this.get<CLStatusResponse>('/admin/cardladder/status');
};

proto.getCardLadderFailures = async function (this: APIClient, limit?: number) {
  const q = limit ? `?limit=${limit}` : '';
  return this.get<IntegrationFailuresReport>(`/admin/cardladder/failures${q}`);
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

proto.getMarketMoversFailures = async function (this: APIClient, limit?: number) {
  const q = limit ? `?limit=${limit}` : '';
  return this.get<IntegrationFailuresReport>(`/admin/marketmovers/failures${q}`);
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

// Backend serializes up to 3 DH calls (push + confirm + optional delist on a
// card swap) plus local writes, each under a 30s/retry DH client budget and a
// 1 RPS rate limiter. The default 30s client timeout aborts successful-but-slow
// requests mid-flight; 90s covers the realistic worst case.
proto.fixDHMatch = async function (this: APIClient, req: DHFixMatchRequest): Promise<DHFixMatchResponse> {
  return this.post<DHFixMatchResponse>('/dh/fix-match', req, { timeoutMs: 90_000 });
};

proto.retryDHMatch = async function (this: APIClient, purchaseId: string): Promise<DHRetryMatchResponse> {
  return this.post<DHRetryMatchResponse>('/dh/retry-match', { purchaseId });
};

proto.unmatchDH = async function (this: APIClient, purchaseId: string): Promise<{ status: string }> {
  return this.post<{ status: string }>('/dh/unmatch', { purchaseId });
};

proto.dismissDHMatch = async function (this: APIClient, purchaseId: string): Promise<{ status: string }> {
  return this.post<{ status: string }>('/dh/dismiss', { purchaseId });
};

proto.undismissDHMatch = async function (this: APIClient, purchaseId: string): Promise<{ status: string }> {
  return this.post<{ status: string }>('/dh/undismiss', { purchaseId });
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

proto.triggerDHReconcile = async function (this: APIClient) {
  return this.post<DHReconcileTriggerResult>('/admin/dh-reconcile/trigger');
};

proto.getDHTombstoneCount = async function (this: APIClient) {
  return this.get<{ count: number }>('/admin/dh-tombstones/count');
};

proto.clearDHTombstones = async function (this: APIClient) {
  return this.post<{ cleared: number }>('/admin/dh-tombstones/clear');
};
