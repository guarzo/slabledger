/** API usage status types for the /api/status/api-usage endpoint */

export interface ProviderDay {
  calls: number;
  limit: number | null;
  remaining?: number | null;
  successRate: number;
  avgLatencyMs: number;
  rateLimitHits: number;
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
  clPricedCards: number;
  mmPricedCards: number;
  totalUnsold: number;
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
  dismissed_count: number;
  pending_count: number;
  mapped_count: number;
  bulk_match_running: boolean;
  bulk_match_error?: string;
  api_health?: DHHealthStats;
  dh_inventory_count?: number;
  dh_listings_count?: number;
  dh_orders_count?: number;
  last_orders_poll_at?: string;
  orders_matched_count_24h?: number;
  orders_orphan_count_24h?: number;
  orders_already_sold_count_24h?: number;
}

export interface DHBulkMatchResponse {
  status: string;
}

export interface DHReconcileResponse {
  scanned: number;
  missingOnDH: number;
  reset: number;
  errors?: string[];
  resetIds?: string[];
}

export interface DHCandidate {
  dh_card_id: number;
  card_name: string;
  set_name: string;
  card_number: string;
  image_url: string;
}

export interface DHUnmatchedCard {
  purchase_id: string;
  card_name: string;
  set_name: string;
  card_number: string;
  cert_number: string;
  grade: number;
  cl_value_cents: number;
  candidates?: DHCandidate[];
}

export interface DHUnmatchedResponse {
  unmatched: DHUnmatchedCard[];
  count: number;
  dismissed: DHUnmatchedCard[];
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

export interface DHSelectMatchRequest {
  purchaseId: string;
  dhCardId: number;
}

export interface DHPushConfig {
  swingPctThreshold: number;
  swingMinCents: number;
  disagreementPctThreshold: number;
  unreviewedChangePctThreshold: number;
  unreviewedChangeMinCents: number;
  updatedAt: string;
}
