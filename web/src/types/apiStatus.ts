/** API usage status types for the /api/status/api-usage endpoint */

export interface ProviderDay {
  calls: number;
  limit: number | null;
  remaining?: number | null;
  successRate: number;
  avgLatencyMs: number;
  rateLimitHits: number;
  minuteCalls: number;
  last429At?: string;
}

export interface ProviderStatus {
  name: string;
  today: ProviderDay;
  blocked: boolean;
  blockedUntil?: string;
  lastCallAt?: string;
}

export interface APIUsageResponse {
  providers: ProviderStatus[];
  timestamp: string;
}

/** Cache statistics types for the /api/admin/cache-stats endpoint */

export interface CachedSetEntry {
  id: string;
  name: string;
  series: string;
  releaseDate: string;
  totalCards: number;
  status: string;
  fetchedAt: string;
}

export interface CacheStatsResponse {
  enabled: boolean;
  totalSets?: number;
  finalizedSets?: number;
  discoveredSets?: number;
  lastUpdated?: string;
  registryVersion?: string;
  sets?: CachedSetEntry[];
}

/** Pricing diagnostics types for the /api/admin/pricing-diagnostics endpoint */

export interface FailureSummary {
  provider: string;
  errorType: string;
  count: number;
  lastSeen: string;
}

export interface PricingDiagnosticsResponse {
  totalMappedCards: number;
  unmappedCards: number;
  recentFailures: FailureSummary[];
}

export type AdvisorAnalysisType = 'digest' | 'liquidation';

export interface CachedAnalysis {
  status: 'empty' | 'pending' | 'running' | 'complete' | 'error';
  content?: string;
  errorMessage?: string;
  updatedAt?: string;
}

export interface PriceOverrideStats {
  totalUnsold: number;
  overrideCount: number;
  manualCount: number;
  costMarkupCount: number;
  aiAcceptedCount: number;
  pendingSuggestions: number;
  overrideTotalUsd: number;
  suggestionTotalUsd: number;
}

/** AI usage status types for the /api/admin/ai-usage endpoint */

export interface AISummary {
  totalCalls: number;
  successRate: number;
  totalInputTokens: number;
  totalOutputTokens: number;
  totalTokens: number;
  avgLatencyMs: number;
  rateLimitHits: number;
  callsLast24h: number;
  lastCallAt?: string;
  totalCostCents: number;
}

export interface AIOperationSummary {
  operation: string;
  calls: number;
  errors: number;
  successRate: number;
  avgLatencyMs: number;
  totalTokens: number;
  totalCostCents: number;
}

export interface AIUsageResponse {
  configured: boolean;
  summary: AISummary;
  operations: AIOperationSummary[];
  timestamp: string;
}

/** DH integration status types */

export interface DHHealthStats {
  total_calls: number;
  failures: number;
  success_rate: number;
}

export interface DHStatusResponse {
  intelligence_count: number;
  intelligence_last_fetch: string;
  suggestions_count: number;
  suggestions_last_fetch: string;
  unmatched_count: number;
  pending_count: number;
  mapped_count: number;
  bulk_match_running: boolean;
  api_health?: DHHealthStats;
  dh_inventory_count?: number;
  dh_listings_count?: number;
  dh_orders_count?: number;
}

export interface DHBulkMatchResponse {
  status: string;
}

export interface DHUnmatchedCard {
  purchase_id: string;
  card_name: string;
  set_name: string;
  card_number: string;
  cert_number: string;
  grade: number;
  cl_value_cents: number;
}

export interface DHUnmatchedResponse {
  unmatched: DHUnmatchedCard[];
  count: number;
}

export interface DHFixMatchRequest {
  purchaseId: string;
  dhUrl: string;
}

export interface DHFixMatchResponse {
  status: string;
  dhCardId: number;
  dhInventoryId: number;
}
