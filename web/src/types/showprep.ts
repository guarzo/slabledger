export type SupportStatus = 'supported' | 'thin_evidence' | 'below_target' | 'mixed_evidence' | 'no_recent_comps' | 'needs_review' | 'no_listed_price';
export type Availability = 'ready' | 'not_received' | 'sold' | 'refunded' | 'campaign_closed' | 'removed' | 'unknown';

export type ReadinessState = 'not_checked' | 'current' | 'stale' | 'running' | 'interrupted' | 'failed' | 'invalid' | 'unavailable';
export type RefreshEligibility = 'needed' | 'not_needed' | 'wait' | 'retry_only' | 'unavailable';
export interface ShowReadiness {
  state: ReadinessState;
  refreshEligibility: RefreshEligibility;
  identityKey: string;
  /** UTC RFC3339Nano boundary, or empty when inapplicable. */
  expiresAt: string;
  /** Read-only observation boundary; never permission to retry automatically. */
  retryAt: string;
}

export interface RecentPriceEvidence {
  windowStart: string;
  windowEnd: string;
  saleIds: string[];
  count: number;
  medianCents: number;
  latestSaleDate: string;
  latestSaleCount: number;
  latestSaleMinCents: number;
  latestSaleMaxCents: number;
  gapPct: number | null;
}

export interface ShowEvaluation {
  purchaseId: string;
  cardName: string;
  certNumber: string;
  grader: string;
  grade: number;
  status: SupportStatus;
  reason: string;
  evidenceNeedsReview: boolean;
  evidenceReason: string;
  availability: Availability;
  canAdd: boolean;
  canPack: boolean;
  listedPriceCents: number;
  localPriceCents: number;
  priceMismatch: boolean;
  priceAssociationUnclear: boolean;
  listingSyncedAt: string;
  medianCents: number;
  compCount: number;
  latestSaleDate: string;
  windowStart: string;
  windowEnd: string;
  refreshedAt: string;
  evidenceVersion: string;
  version: string;
  policyVersion: string;
  recent: RecentPriceEvidence;
  /** Untrusted additive metadata: consume only through getShowReadiness. */
  readiness?: unknown;
}
export interface ShowSale {
  id: string;
  date: string;
  priceCents: number;
  platform: string;
  url: string;
  listingType: string;
}
export interface ShowEvidence { evaluation: ShowEvaluation; sales: ShowSale[] }
export interface ShowList { id: string; name: string; createdAt: string; updatedAt: string }
export interface ShowListItem {
  id: string;
  purchaseId: string;
  cardName: string;
  certNumber: string;
  grader: string;
  grade: number;
  addedAt: string;
  packedAt: string;
  version: number;
  acknowledgedPriceCents: number;
  acknowledgedStatus: SupportStatus;
  evaluation: ShowEvaluation;
  priceChanged: boolean;
  supportChanged: boolean;
}
export interface ShowListDetail {
  list: ShowList;
  items: ShowListItem[];
  summary: {
    totalCount: number;
    packedCount: number;
    notReceivedCount: number;
    unavailableCount: number;
    knownValueCents: number;
    missingPriceCount: number;
    ambiguousPriceCount: number;
  };
}
export interface ShowItemUpdate {
  version: number;
  evaluationVersion: string;
  packed?: boolean;
  acknowledge?: boolean;
}
export interface ShowItemAdd { purchaseId: string; evaluationVersion: string }
