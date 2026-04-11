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
  updated: number;
  mapped: number;
  skipped: number;
  totalCLCards: number;
  cardsPushed: number;
  cardsRemoved: number;
  orphanMappings: number;
  noImageMatch: number;
  noCertMatch: number;
  noValue: number;
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
// Market Movers types
// ---------------------------------------------------------------------------

export interface MMPriceStats {
  unsoldTotal: number;
  withMMPrice: number;
  syncedCount: number;
  oldestUpdate: string;
  newestUpdate: string;
  staleCount: number;
}

export interface MMLastRun {
  lastRunAt: string;
  durationMs: number;
  updated: number;
  newMappings: number;
  skipped: number;
  searchFailed: number;
  totalPurchases: number;
  tokenMismatches: number;
  noSalesData: number;
  uploadedLastRun: number;
  deletedLastRun: number;
}

export interface MMStatusResponse {
  configured: boolean;
  username?: string;
  cardsMapped?: number;
  priceStats?: MMPriceStats;
  lastRun?: MMLastRun;
}

export interface MMSyncError {
  certNumber: string;
  error: string;
}

export interface MMSyncResult {
  synced: number;
  skipped: number;
  failed: number;
  errors?: MMSyncError[];
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
  spreadsheetId: string;
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
