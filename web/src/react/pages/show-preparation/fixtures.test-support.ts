import type { ShowEvaluation, ShowListDetail, ShowListItem } from '../../../types/showprep';
import type { AgingItem } from '../../../types/campaigns';

export const purchaseId = '11111111-1111-4111-8111-111111111111';
export const listId = '22222222-2222-4222-8222-222222222222';
export function evaluation(overrides: Partial<ShowEvaluation> = {}): ShowEvaluation {
  return {
    purchaseId, cardName: 'Pikachu Special Illustration Rare with a very long variant title',
    certNumber: '12345678', grader: 'PSA', grade: 10, status: 'supported', reason: '',
    evidenceNeedsReview: false, evidenceReason: '',
    availability: 'ready', canAdd: true, canPack: true, listedPriceCents: 30000,
    localPriceCents: 30000, priceMismatch: false, priceAssociationUnclear: false,
    listingSyncedAt: '2026-09-14T08:00:00Z', medianCents: 28000, compCount: 2,
    latestSaleDate: '2026-09-13', windowStart: '2026-08-16', windowEnd: '2026-09-14',
    refreshedAt: '2026-09-14T09:00:00Z', evidenceVersion: 'evidence-1', version: 'eval-1',
    ...overrides,
  };
}
export function member(overrides: Partial<ShowListItem> = {}): ShowListItem {
  return {
    id: '33333333-3333-4333-8333-333333333333', purchaseId,
    cardName: evaluation().cardName, certNumber: '12345678', grader: 'PSA', grade: 10,
    addedAt: '2026-09-14T10:00:00Z', packedAt: '', version: 1,
    acknowledgedPriceCents: 30000, acknowledgedStatus: 'supported',
    evaluation: evaluation(), priceChanged: false, supportChanged: false, ...overrides,
  };
}
export function detail(items: ShowListItem[] = [member()]): ShowListDetail {
  return {
    list: { id: listId, name: 'September show', createdAt: '2026-09-14T10:00:00Z', updatedAt: '2026-09-14T10:00:00Z' },
    items,
    summary: { totalCount: 1, packedCount: 0, notReceivedCount: 0, unavailableCount: 0,
      knownValueCents: 30000, missingPriceCount: 0, ambiguousPriceCount: 0 },
  };
}
export function inventoryItem(e = evaluation(), overrides: Partial<AgingItem['purchase']> = {}): AgingItem {
  return {
    purchase: {
      id: e.purchaseId, campaignId: listId, cardName: e.cardName, setName: 'Scarlet & Violet',
      certNumber: e.certNumber, grader: e.grader, gradeValue: e.grade, receivedAt: '2026-09-01T00:00:00Z',
      dhStatus: 'listed', dhInventoryId: 123, reviewedPriceCents: 30000,
      dhListingPriceCents: e.listedPriceCents, dhLastSyncedAt: e.listingSyncedAt,
      buyCostCents: 20000, psaSourcingFeeCents: 500, clValueCents: 30000,
      purchaseDate: '2026-09-01', createdAt: '2026-09-01T00:00:00Z', updatedAt: '2026-09-14T00:00:00Z', ...overrides,
    },
    daysHeld: 13,
  };
}
