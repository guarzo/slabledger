export type SupportStatus = 'supported' | 'thin_evidence' | 'below_target' | 'no_recent_comps' | 'needs_review' | 'no_listed_price';
export type Availability = 'ready' | 'not_received' | 'sold' | 'refunded' | 'campaign_closed' | 'removed' | 'unknown';

export interface ShowEvaluation {
  purchaseId: string;
  cardName: string;
  certNumber: string;
  grader: string;
  grade: number;
  status: SupportStatus;
  reason: string;
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
