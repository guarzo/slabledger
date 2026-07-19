export interface AllowedEmail {
  Email: string;
  AddedBy: number | null;
  CreatedAt: string;
  Notes: string;
}

export interface AdminUser {
  id: number;
  username: string;
  email: string;
  avatar_url: string;
  is_admin: boolean;
  last_login_at: string | null;
}

// ---------------------------------------------------------------------------
// Integration diagnostics (shared MM + CL shape)
// ---------------------------------------------------------------------------

export interface IntegrationFailureSample {
  purchaseId: string;
  certNumber: string;
  cardName: string;
  reason: string;
  errorAt: string;
}

export interface IntegrationFailuresReport {
  byReason: Record<string, number>;
  samples: IntegrationFailureSample[] | null;
}

// ---------------------------------------------------------------------------
// Card Ladder types
// ---------------------------------------------------------------------------

export interface CLPriceStats {
  unsoldTotal: number;
  withCLValue: number;
  syncedCount: number;
  oldestUpdate: string;
  newestUpdate: string;
  staleCount: number;
}

export interface CLLastRun {
  lastRunAt: string;
  durationMs: number;
  totalPurchases: number;
  updated: number;
  resolved: number;
  noCert: number;
  certResolveFailed: number;
  noValue: number;
  cardsPushed: number;
  cardsRemoved: number;
}

export interface CLStatusResponse {
  configured: boolean;
  email?: string;
  collectionId?: string;
  cardsMapped?: number;
  priceStats?: CLPriceStats;
  lastRun?: CLLastRun;
}

export interface CLSyncResultItem {
  certNumber: string;
  player: string;
  set: string;
  condition: string;
  estimatedValue: number;
  status: string;
  error?: string;
}

export interface CLSyncResult {
  synced: number;
  skipped: number;
  failed: number;
  total: number;
  results: CLSyncResultItem[];
}

// ---------------------------------------------------------------------------
// PSA Sync types
// ---------------------------------------------------------------------------

export interface PSASyncLastRun {
  lastRunAt: string;
  durationMs: number;
  allocated: number;
  updated: number;
  refunded: number;
  unmatched: number;
  ambiguous: number;
  skipped: number;
  failed: number;
  totalRows: number;
  parseErrors: number;
}

export interface PSASyncStatusResponse {
  configured: boolean;
  interval: string;
  lastRun?: PSASyncLastRun;
  pendingCount?: number;
}

export interface PSAPendingItem {
  id: string;
  certNumber: string;
  cardName: string;
  setName: string;
  cardNumber: string;
  grade: number;
  buyCostCents: number;
  purchaseDate: string;
  status: 'ambiguous' | 'unmatched';
  candidates: string[];
  source: 'scheduler' | 'manual';
  createdAt: string;
}
