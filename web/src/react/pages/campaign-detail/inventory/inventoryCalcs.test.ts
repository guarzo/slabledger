import { describe, it, expect } from 'vitest';
import {
  computeInventoryMeta,
  filterAndSortItems,
  isReadyToList,
  needsPriceReview,
  wasUnlistedFromDH,
  isSkipped,
  isDHListed,
  isPendingDHMatch,
  isPendingPrice,
} from './inventoryCalcs';
import type { AgingItem, Purchase } from '../../../../types/campaigns';

type TestPurchase = Pick<Purchase,
  'id' | 'cardName' | 'gradeValue' | 'certNumber' | 'receivedAt' |
  'campaignId' | 'clValueCents' | 'buyCostCents' | 'psaSourcingFeeCents' | 'purchaseDate' |
  'createdAt' | 'updatedAt' | 'aiSuggestedPriceCents' | 'reviewedAt' |
  'dhInventoryId' | 'dhStatus' | 'reviewedPriceCents' | 'dhUnlistedDetectedAt' |
  'overridePriceCents' | 'dhPushStatus'
> & {
  setName?: string;
  cardNumber?: string;
};

function makeItem(overrides?: { purchase?: Partial<TestPurchase> } & Omit<Partial<AgingItem>, 'purchase'>): AgingItem {
  const { purchase: purchaseOverrides, ...agingOverrides } = overrides ?? {};
  const purchase: TestPurchase = {
    id: '1',
    cardName: 'Test Card',
    gradeValue: 8,
    buyCostCents: 1000,
    psaSourcingFeeCents: 0,
    purchaseDate: '2026-01-01',
    certNumber: 'PSA-123456',
    receivedAt: undefined,
    campaignId: 'camp-1',
    createdAt: '2026-01-01T00:00:00Z',
    updatedAt: '2026-01-01T00:00:00Z',
    setName: 'Base',
    cardNumber: '001',
    clValueCents: 2000,
    ...purchaseOverrides,
  };
  return {
    purchase: purchase as AgingItem['purchase'],
    daysHeld: 0,
    ...agingOverrides,
  };
}

describe('inventoryCalcs', () => {
  describe('computeInventoryMeta', () => {
    it('received items without dhInventoryId land in pending_dh_match', () => {
      const items = [
        makeItem({ purchase: { receivedAt: '2026-04-08T00:00:00Z' } }),
        makeItem({ purchase: { receivedAt: undefined } }),
        makeItem({ purchase: { receivedAt: '2026-04-09T00:00:00Z' } }),
      ];

      const meta = computeInventoryMeta(items);
      expect(meta.tabCounts.pending_dh_match).toBe(2);
    });

    it('initializes pending_dh_match to 0 when no items have receivedAt', () => {
      const items = [
        makeItem({ purchase: { receivedAt: undefined } }),
        makeItem({ purchase: { receivedAt: undefined } }),
      ];

      const meta = computeInventoryMeta(items);
      expect(meta.tabCounts.pending_dh_match).toBe(0);
    });

    it('all received items without dhInventoryId land in pending_dh_match', () => {
      const items = [
        makeItem({ purchase: { receivedAt: '2026-04-08T00:00:00Z' } }),
        makeItem({ purchase: { receivedAt: '2026-04-09T00:00:00Z' } }),
      ];

      const meta = computeInventoryMeta(items);
      expect(meta.tabCounts.pending_dh_match).toBe(2);
      expect(meta.tabCounts.all).toBe(2);
    });

    it('counts unreviewed items with AI suggestions under needs_attention', () => {
      const items = [
        // unreviewed with AI suggestion → needs attention
        makeItem({ purchase: { id: '1', aiSuggestedPriceCents: 5000, reviewedAt: undefined, receivedAt: '2026-04-09T00:00:00Z' } }),
        // reviewed with lingering AI suggestion → NOT needs attention (suggestion superseded).
        // Backend invariant: reviewedAt is only stamped when reviewedPriceCents > 0.
        makeItem({ purchase: { id: '2', aiSuggestedPriceCents: 5000, reviewedPriceCents: 8000, reviewedAt: '2026-04-10T00:00:00Z', receivedAt: '2026-04-09T00:00:00Z' } }),
        // override set with lingering AI suggestion → NOT needs attention
        makeItem({ purchase: { id: '3', aiSuggestedPriceCents: 5000, overridePriceCents: 7000, receivedAt: '2026-04-09T00:00:00Z' } }),
        // reviewed, no AI → NOT needs attention
        makeItem({ purchase: { id: '4', reviewedPriceCents: 8000, reviewedAt: '2026-04-10T00:00:00Z', receivedAt: '2026-04-09T00:00:00Z' } }),
      ];
      const meta = computeInventoryMeta(items);
      expect(meta.tabCounts.needs_attention).toBe(1);
    });

    it('drops no_data items from needs_attention once a price override is set', () => {
      const items = [
        // received, no CL, no snapshot → no_data → needs attention
        makeItem({ purchase: { id: '1', receivedAt: '2026-04-09T00:00:00Z', clValueCents: 0, reviewedAt: undefined } }),
        // same, but with override committed → should NOT count
        makeItem({ purchase: { id: '2', receivedAt: '2026-04-09T00:00:00Z', clValueCents: 0, reviewedAt: undefined, overridePriceCents: 7500 } }),
      ];
      const meta = computeInventoryMeta(items);
      expect(meta.tabCounts.needs_attention).toBe(1);
    });

    it('excludes awaiting-intake items from needs_attention', () => {
      const items = [
        // in-hand, unreviewed, no price data → needs attention
        makeItem({ purchase: { id: '1', receivedAt: '2026-04-08T00:00:00Z', clValueCents: 0, reviewedAt: undefined } }),
        // awaiting intake, no data → should NOT count toward needs_attention
        makeItem({ purchase: { id: '2', receivedAt: undefined, clValueCents: 0, reviewedAt: undefined } }),
      ];
      const meta = computeInventoryMeta(items);
      expect(meta.tabCounts.needs_attention).toBe(1);
      expect(meta.tabCounts.awaiting_intake).toBe(1);
    });

    it('returns correct structure with all tab counts', () => {
      const items = [makeItem()];
      const meta = computeInventoryMeta(items);

      expect(meta).toHaveProperty('tabCounts');
      expect(meta.tabCounts).toHaveProperty('needs_attention');
      expect(meta.tabCounts).toHaveProperty('in_hand');
      expect(meta.tabCounts).toHaveProperty('ready_to_list');
      expect(meta.tabCounts).toHaveProperty('awaiting_intake');
      expect(meta.tabCounts).toHaveProperty('all');
    });
  });

  describe('filterAndSortItems', () => {
    it('in_hand tab is a legacy alias for all — returns all items regardless of receivedAt', () => {
      const items = [
        makeItem({ purchase: { id: '1', receivedAt: '2026-04-08T00:00:00Z' } }),
        makeItem({ purchase: { id: '2', receivedAt: undefined } }),
        makeItem({ purchase: { id: '3', receivedAt: '2026-04-09T00:00:00Z' } }),
      ];

      const result = filterAndSortItems(items, {
        debouncedSearch: '',
        showAll: false,
        filterTab: 'in_hand',
        sortKey: 'days',
        sortDir: 'desc',
        evMap: new Map(),
      });

      expect(result).toHaveLength(3);
    });

    it('awaiting_intake tab filters to items without receivedAt', () => {
      const items = [
        makeItem({ purchase: { id: '1', receivedAt: '2026-04-08T00:00:00Z' } }),
        makeItem({ purchase: { id: '2', receivedAt: '' } }),
        makeItem({ purchase: { id: '3', receivedAt: undefined } }),
      ];

      const result = filterAndSortItems(items, {
        debouncedSearch: '',
        showAll: false,
        filterTab: 'awaiting_intake',
        sortKey: 'days',
        sortDir: 'desc',
        evMap: new Map(),
      });

      expect(result).toHaveLength(2);
      expect(result.map(r => r.purchase.id).sort()).toEqual(['2', '3']);
    });

    it('returns all items when no items have receivedAt and in_hand is used', () => {
      const items = [
        makeItem({ purchase: { id: '1', receivedAt: undefined } }),
        makeItem({ purchase: { id: '2', receivedAt: undefined } }),
      ];

      const result = filterAndSortItems(items, {
        debouncedSearch: '',
        showAll: false,
        filterTab: 'in_hand',
        sortKey: 'days',
        sortDir: 'desc',
        evMap: new Map(),
      });

      expect(result).toHaveLength(2);
    });

    it('returns all items when filterTab is all', () => {
      const items = [
        makeItem({ purchase: { id: '1', receivedAt: '2026-04-08T00:00:00Z' } }),
        makeItem({ purchase: { id: '2', receivedAt: undefined } }),
      ];

      const result = filterAndSortItems(items, {
        debouncedSearch: '',
        showAll: false,
        filterTab: 'all',
        sortKey: 'days',
        sortDir: 'desc',
        evMap: new Map(),
      });

      expect(result).toHaveLength(2);
    });

    it('returns all items when showAll is true', () => {
      const items = [
        makeItem({ purchase: { id: '1', receivedAt: '2026-04-08T00:00:00Z' } }),
        makeItem({ purchase: { id: '2', receivedAt: undefined } }),
      ];

      const result = filterAndSortItems(items, {
        debouncedSearch: '',
        showAll: true,
        filterTab: 'in_hand',
        sortKey: 'days',
        sortDir: 'desc',
        evMap: new Map(),
      });

      expect(result).toHaveLength(2);
    });

    it('returns search results regardless of filterTab', () => {
      const items = [
        makeItem({ purchase: { id: '1', cardName: 'Pikachu', receivedAt: '2026-04-08T00:00:00Z' } }),
        makeItem({ purchase: { id: '2', cardName: 'Charizard', receivedAt: undefined } }),
      ];

      const result = filterAndSortItems(items, {
        debouncedSearch: 'Charizard',
        showAll: false,
        filterTab: 'in_hand',
        sortKey: 'days',
        sortDir: 'desc',
        evMap: new Map(),
      });

      expect(result).toHaveLength(1);
      expect(result[0].purchase.cardName).toBe('Charizard');
    });
  });

  describe('isReadyToList', () => {
    it('returns true when received, pushed to DH, not listed, and has a reviewed price', () => {
      const item = makeItem({
        purchase: {
          receivedAt: '2026-04-08T00:00:00Z',
          dhInventoryId: 42,
          dhStatus: 'in_stock',
          reviewedPriceCents: 9000,
        },
      });
      expect(isReadyToList(item)).toBe(true);
    });

    it('returns true when only an override price is set (Set Price dialog commit)', () => {
      const item = makeItem({
        purchase: {
          receivedAt: '2026-04-08T00:00:00Z',
          dhInventoryId: 42,
          dhStatus: 'in_stock',
          reviewedPriceCents: 0,
          overridePriceCents: 12500,
        },
      });
      expect(isReadyToList(item)).toBe(true);
    });

    it('returns false when reviewed and override are both 0', () => {
      const item = makeItem({
        purchase: {
          receivedAt: '2026-04-08T00:00:00Z',
          dhInventoryId: 42,
          dhStatus: 'in_stock',
          reviewedPriceCents: 0,
          overridePriceCents: 0,
        },
      });
      expect(isReadyToList(item)).toBe(false);
    });

    it('returns false when reviewedPriceCents is undefined and no override', () => {
      const item = makeItem({
        purchase: {
          receivedAt: '2026-04-08T00:00:00Z',
          dhInventoryId: 42,
          dhStatus: 'in_stock',
          reviewedPriceCents: undefined,
        },
      });
      expect(isReadyToList(item)).toBe(false);
    });

    it('returns false when not received', () => {
      const item = makeItem({
        purchase: {
          receivedAt: undefined,
          dhInventoryId: 42,
          dhStatus: 'in_stock',
          reviewedPriceCents: 9000,
        },
      });
      expect(isReadyToList(item)).toBe(false);
    });

    it('returns false when not pushed to DH', () => {
      const item = makeItem({
        purchase: {
          receivedAt: '2026-04-08T00:00:00Z',
          dhInventoryId: undefined,
          dhStatus: 'in_stock',
          reviewedPriceCents: 9000,
        },
      });
      expect(isReadyToList(item)).toBe(false);
    });

    it('returns false when already listed', () => {
      const item = makeItem({
        purchase: {
          receivedAt: '2026-04-08T00:00:00Z',
          dhInventoryId: 42,
          dhStatus: 'listed',
          reviewedPriceCents: 9000,
        },
      });
      expect(isReadyToList(item)).toBe(false);
    });
  });

  describe('needsPriceReview', () => {
    it('returns true when received + pushed to DH + not listed + no committed price', () => {
      const item = makeItem({
        purchase: {
          receivedAt: '2026-04-08T00:00:00Z',
          dhInventoryId: 42,
          dhStatus: 'in_stock',
          reviewedPriceCents: 0,
          overridePriceCents: 0,
        },
      });
      expect(needsPriceReview(item)).toBe(true);
    });

    it('returns true when reviewedPriceCents is undefined and no override', () => {
      const item = makeItem({
        purchase: {
          receivedAt: '2026-04-08T00:00:00Z',
          dhInventoryId: 42,
          dhStatus: 'in_stock',
          reviewedPriceCents: undefined,
        },
      });
      expect(needsPriceReview(item)).toBe(true);
    });

    it('returns false when a reviewed price is set', () => {
      const item = makeItem({
        purchase: {
          receivedAt: '2026-04-08T00:00:00Z',
          dhInventoryId: 42,
          dhStatus: 'in_stock',
          reviewedPriceCents: 9000,
        },
      });
      expect(needsPriceReview(item)).toBe(false);
    });

    it('returns false when only an override price is set', () => {
      const item = makeItem({
        purchase: {
          receivedAt: '2026-04-08T00:00:00Z',
          dhInventoryId: 42,
          dhStatus: 'in_stock',
          reviewedPriceCents: 0,
          overridePriceCents: 12500,
        },
      });
      expect(needsPriceReview(item)).toBe(false);
    });

    it('returns false when not received', () => {
      const item = makeItem({
        purchase: {
          receivedAt: undefined,
          dhInventoryId: 42,
          dhStatus: 'in_stock',
          reviewedPriceCents: 0,
        },
      });
      expect(needsPriceReview(item)).toBe(false);
    });

    it('returns false when not pushed to DH', () => {
      const item = makeItem({
        purchase: {
          receivedAt: '2026-04-08T00:00:00Z',
          dhInventoryId: undefined,
          dhStatus: 'in_stock',
          reviewedPriceCents: 0,
        },
      });
      expect(needsPriceReview(item)).toBe(false);
    });

    it('returns false when already listed', () => {
      const item = makeItem({
        purchase: {
          receivedAt: '2026-04-08T00:00:00Z',
          dhInventoryId: 42,
          dhStatus: 'listed',
          reviewedPriceCents: 0,
        },
      });
      expect(needsPriceReview(item)).toBe(false);
    });

    it('is mutually exclusive with isReadyToList for any given item', () => {
      const base = {
        receivedAt: '2026-04-08T00:00:00Z',
        dhInventoryId: 42,
        dhStatus: 'in_stock',
      } as const;

      const withReviewed = makeItem({ purchase: { ...base, reviewedPriceCents: 9000 } });
      expect(isReadyToList(withReviewed)).toBe(true);
      expect(needsPriceReview(withReviewed)).toBe(false);

      const withOverride = makeItem({ purchase: { ...base, overridePriceCents: 12500 } });
      expect(isReadyToList(withOverride)).toBe(true);
      expect(needsPriceReview(withOverride)).toBe(false);

      const withoutPrice = makeItem({ purchase: { ...base, reviewedPriceCents: 0, overridePriceCents: 0 } });
      expect(isReadyToList(withoutPrice)).toBe(false);
      expect(needsPriceReview(withoutPrice)).toBe(true);
    });
  });

  describe('wasUnlistedFromDH', () => {
    it('returns true when dhUnlistedDetectedAt is set', () => {
      const item = makeItem({
        purchase: { dhUnlistedDetectedAt: '2026-04-15T12:00:00Z' },
      });
      expect(wasUnlistedFromDH(item)).toBe(true);
    });

    it('returns false when dhUnlistedDetectedAt is undefined', () => {
      const item = makeItem({ purchase: { dhUnlistedDetectedAt: undefined } });
      expect(wasUnlistedFromDH(item)).toBe(false);
    });

    it('returns false when dhUnlistedDetectedAt is empty string', () => {
      const item = makeItem({ purchase: { dhUnlistedDetectedAt: '' } });
      expect(wasUnlistedFromDH(item)).toBe(false);
    });
  });

  describe('bucket predicates', () => {
    it('isSkipped returns true only when dhPushStatus is dismissed', () => {
      expect(isSkipped(makeItem({ purchase: { dhPushStatus: 'dismissed' } }))).toBe(true);
      expect(isSkipped(makeItem({ purchase: { dhPushStatus: 'matched' } }))).toBe(false);
      expect(isSkipped(makeItem({ purchase: { dhPushStatus: undefined } }))).toBe(false);
    });

    it('isDHListed returns true only when dhStatus is listed', () => {
      expect(isDHListed(makeItem({ purchase: { dhStatus: 'listed' } }))).toBe(true);
      expect(isDHListed(makeItem({ purchase: { dhStatus: 'in stock' } }))).toBe(false);
    });

    it('isPendingDHMatch requires received and no dhInventoryId and not skipped', () => {
      const received = '2026-04-20T00:00:00Z';
      expect(isPendingDHMatch(makeItem({ purchase: { receivedAt: received, dhInventoryId: undefined } }))).toBe(true);
      expect(isPendingDHMatch(makeItem({ purchase: { receivedAt: undefined, dhInventoryId: undefined } }))).toBe(false);
      expect(isPendingDHMatch(makeItem({ purchase: { receivedAt: received, dhInventoryId: 42 } }))).toBe(false);
      expect(isPendingDHMatch(makeItem({ purchase: { receivedAt: received, dhInventoryId: undefined, dhPushStatus: 'dismissed' } }))).toBe(false);
    });

    it('isPendingPrice requires received + matched + no committed price + not listed + not skipped', () => {
      const received = '2026-04-20T00:00:00Z';
      const base = { receivedAt: received, dhInventoryId: 42, dhStatus: 'in stock' as const };
      expect(isPendingPrice(makeItem({ purchase: { ...base } }))).toBe(true);
      expect(isPendingPrice(makeItem({ purchase: { ...base, reviewedPriceCents: 5000 } }))).toBe(false);
      expect(isPendingPrice(makeItem({ purchase: { ...base, overridePriceCents: 5000 } }))).toBe(false);
      expect(isPendingPrice(makeItem({ purchase: { ...base, dhStatus: 'listed' } }))).toBe(false);
      expect(isPendingPrice(makeItem({ purchase: { ...base, dhPushStatus: 'dismissed' } }))).toBe(false);
    });

    it('partition: every item lands in exactly one secondary bucket', () => {
      const received = '2026-04-20T00:00:00Z';
      const items = [
        makeItem({ purchase: { id: 'a', receivedAt: undefined } }),                                              // awaiting_intake
        makeItem({ purchase: { id: 'b', receivedAt: received, dhPushStatus: 'dismissed' } }),                    // skipped
        makeItem({ purchase: { id: 'c', receivedAt: received, dhStatus: 'listed', dhInventoryId: 1 } }),         // dh_listed
        makeItem({ purchase: { id: 'd', receivedAt: received, dhInventoryId: undefined } }),                     // pending_dh_match
        makeItem({ purchase: { id: 'e', receivedAt: received, dhInventoryId: 2, dhStatus: 'in stock' } }),       // pending_price
        makeItem({ purchase: { id: 'f', receivedAt: received, dhInventoryId: 3, dhStatus: 'in stock', reviewedPriceCents: 5000 } }), // ready_to_list
      ];

      const meta = computeInventoryMeta(items);
      expect(meta.tabCounts.awaiting_intake).toBe(1);
      expect(meta.tabCounts.skipped).toBe(1);
      expect(meta.tabCounts.dh_listed).toBe(1);
      expect(meta.tabCounts.pending_dh_match).toBe(1);
      expect(meta.tabCounts.pending_price).toBe(1);
      expect(meta.tabCounts.ready_to_list).toBe(1);
      expect(meta.tabCounts.all).toBe(6);
      // Sum of partitioned buckets equals total.
      const partitioned =
        meta.tabCounts.awaiting_intake +
        meta.tabCounts.skipped +
        meta.tabCounts.dh_listed +
        meta.tabCounts.pending_dh_match +
        meta.tabCounts.pending_price +
        meta.tabCounts.ready_to_list;
      expect(partitioned).toBe(meta.tabCounts.all);
    });
  });
});
