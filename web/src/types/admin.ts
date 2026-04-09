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
// Card Ladder types
// ---------------------------------------------------------------------------

export interface CLLastRun {
  lastRunAt: string;
  durationMs: number;
  updated: number;
  mapped: number;
  skipped: number;
  totalCLCards: number;
  cardsPushed: number;
  cardsRemoved: number;
}

export interface CLStatusResponse {
  configured: boolean;
  email?: string;
  collectionId?: string;
  cardsMapped?: number;
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
