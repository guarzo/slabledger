import type { AgingItem } from '../../../types/campaigns';
import type { PricePreview, ShowEvaluation, ShowSale } from '../../../types/showprep';

// Invented identities and prices, never copied from operator inventory.
export const purchaseId = '11111111-1111-4111-8111-111111111111';
export const otherId = '22222222-2222-4222-8222-222222222222';
export function evaluation(overrides: Partial<ShowEvaluation> = {}): ShowEvaluation {
  return {
    purchaseId, cardName: 'Aurora Dragon', certNumber: '00000001', grader: 'PSA', grade: 9,
    status: 'below_target', reason: 'Recent sales are below this asking price.',
    evidenceNeedsReview: false, evidenceReason: '', availability: 'ready', canAdd: true, canPack: true,
    listedPriceCents: 290000, localPriceCents: 280000, priceMismatch: true, priceAssociationUnclear: false,
    listingSyncedAt: '2026-09-16T08:00:00Z', medianCents: 280000, compCount: 3,
    latestSaleDate: '2026-09-16', windowStart: '2026-08-18', windowEnd: '2026-09-16',
    refreshedAt: '2026-09-16T09:00:00Z', evidenceVersion: 'evidence-a', version: 'saved-a', policyVersion: 'recent-sales-v1',
    recent: { windowStart: '2026-09-10', windowEnd: '2026-09-16', saleIds: ['sale-a', 'sale-b'], count: 2,
      medianCents: 240000, latestSaleDate: '2026-09-16', latestSaleCount: 1,
      latestSaleMinCents: 200000, latestSaleMaxCents: 200000, gapPct: 14.285714 },
    ...overrides,
  };
}
export function inventoryItem(e = evaluation(), overrides: Partial<AgingItem['purchase']> = {}): AgingItem {
  return {
    purchase: { id: e.purchaseId, campaignId: '33333333-3333-4333-8333-333333333333', cardName: e.cardName,
      certNumber: e.certNumber, setName: 'Northern Lights', grader: e.grader, gradeValue: e.grade,
      buyCostCents: 150000, psaSourcingFeeCents: 500, clValueCents: 260000, purchaseDate: '2026-09-01',
      createdAt: '2026-09-01T00:00:00Z', updatedAt: '2026-09-16T00:00:00Z', receivedAt: '2026-09-02T00:00:00Z',
      reviewedPriceCents: e.localPriceCents, dhListingPriceCents: e.listedPriceCents, dhStatus: 'listed', ...overrides },
    daysHeld: 15, campaignName: 'Autumn collection',
  };
}
export const sales: ShowSale[] = [
  { id: 'sale-a', date: '2026-09-16', priceCents: 200000, platform: 'Auction house', url: 'https://example.com/sale-a', listingType: 'auction' },
  { id: 'sale-b', date: '2026-09-15', priceCents: 280000, platform: 'Marketplace', url: 'javascript:alert(1)', listingType: 'fixed_price' },
  { id: 'sale-old', date: '2026-08-28', priceCents: 310000, platform: 'Older auction', url: 'https://example.com/old', listingType: 'auction' },
];
export function preview(cents = 240000, e = evaluation()): PricePreview {
  return { purchaseId: e.purchaseId, currentPriceCents: e.localPriceCents, trialPriceCents: cents,
    // Explicit server outcomes for the two tested trials, not a local assessor.
    status: cents === 254000 ? 'mixed_evidence' : cents === 240000 ? 'supported' : 'needs_review',
    reason: cents === 254000 ? 'The newest sale day conflicts with the recent median.'
      : cents === 240000 ? 'Recent sales support this trial asking price.' : 'No fixture preview supplied for this trial.',
    evidenceNeedsReview: false, evidenceReason: '', evidenceVersion: e.evidenceVersion,
    policyVersion: e.policyVersion, recent: { ...e.recent, gapPct: cents === 254000 ? 5.511811 : cents === 240000 ? 0 : null } };
}
