import { describe, it, expect } from 'vitest';
import {
  computeInventoryMeta,
  filterAndSortItems,
  isReadyToList,
  needsPriceReview,
  wasUnlistedFromDH,
} from './inventoryCalcs';
import type { AgingItem, Purchase } from '../../../../types/campaigns';

type TestPurchase = Pick<Purchase,
  'id' | 'cardName' | 'gradeValue' | 'certNumber' | 'receivedAt' |
  'campaignId' | 'clValueCents' | 'buyCostCents' | 'psaSourcingFeeCents' | 'purchaseDate' |
  'createdAt' | 'updatedAt' | 'aiSuggestedPriceCents' | 'reviewedAt' |
  'dhInventoryId' | 'dhStatus' | 'reviewedPriceCents' | 'dhUnlistedDetectedAt'
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
    it('counts in_hand items with receivedAt set', () => {
      const items = [
        makeItem({ purchase: { receivedAt: '2026-04-08T00:00:00Z' } }),
        makeItem({ purchase: { receivedAt: undefined } }),
        makeItem({ purchase: { receivedAt: '2026-04-09T00:00:00Z' } }),
      ];

      const meta = computeInventoryMeta(items);
      expect(meta.tabCounts.in_hand).toBe(2);
    });

    it('initializes in_hand count to 0 when no items have receivedAt', () => {
      const items = [
        makeItem({ purchase: { receivedAt: undefined } }),
        makeItem({ purchase: { receivedAt: undefined } }),
      ];

      const meta = computeInventoryMeta(items);
      expect(meta.tabCounts.in_hand).toBe(0);
    });

    it('counts all items in in_hand when all have receivedAt', () => {
      const items = [
        makeItem({ purchase: { receivedAt: '2026-04-08T00:00:00Z' } }),
        makeItem({ purchase: { receivedAt: '2026-04-09T00:00:00Z' } }),
      ];

      const meta = computeInventoryMeta(items);
      expect(meta.tabCounts.in_hand).toBe(2);
      expect(meta.tabCounts.all).toBe(2);
    });

    it('counts items with AI suggestions under needs_attention', () => {
      const items = [
        makeItem({ purchase: { id: '1', aiSuggestedPriceCents: 5000, reviewedAt: '2026-04-10T00:00:00Z', receivedAt: '2026-04-09T00:00:00Z' } }),
        makeItem({ purchase: { id: '2', reviewedAt: '2026-04-10T00:00:00Z', receivedAt: '2026-04-09T00:00:00Z' } }),
      ];
      const meta = computeInventoryMeta(items);
      expect(meta.tabCounts.needs_attention).toBe(1);
    });

    it('excludes awaiting-intake items from needsReview and needs_attention', () => {
      const items = [
        // in-hand, unreviewed, no price data → needs attention + unreviewed
        makeItem({ purchase: { id: '1', receivedAt: '2026-04-08T00:00:00Z', clValueCents: 0, reviewedAt: undefined } }),
        // awaiting intake, no data → should NOT count toward unreviewed or needs_attention
        makeItem({ purchase: { id: '2', receivedAt: undefined, clValueCents: 0, reviewedAt: undefined } }),
      ];
      const meta = computeInventoryMeta(items);
      expect(meta.reviewStats.needsReview).toBe(1);
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
    it('filters items by in_hand tab (receivedAt present)', () => {
      const items = [
        makeItem({ purchase: { id: '1', receivedAt: '2026-04-08T00:00:00Z' } }),
        makeItem({ purchase: { id: '2', receivedAt: undefined } }),
        makeItem({ purchase: { id: '3', receivedAt: '2026-04-09T00:00:00Z' } }),
      ];

      const result = filterAndSortItems(items, {
        debouncedSearch: '',
        showAll: false,
        filterTab: 'in_hand',
        sellSheetHas: () => false,
        sortKey: 'days',
        sortDir: 'desc',
        evMap: new Map(),
      });

      expect(result).toHaveLength(2);
      expect(result[0].purchase.id).toBe('1');
      expect(result[1].purchase.id).toBe('3');
    });

    it('filters items by in_hand tab (receivedAt empty string)', () => {
      const items = [
        makeItem({ purchase: { id: '1', receivedAt: '2026-04-08T00:00:00Z' } }),
        makeItem({ purchase: { id: '2', receivedAt: '' } }),
        makeItem({ purchase: { id: '3', receivedAt: undefined } }),
      ];

      const result = filterAndSortItems(items, {
        debouncedSearch: '',
        showAll: false,
        filterTab: 'in_hand',
        sellSheetHas: () => false,
        sortKey: 'days',
        sortDir: 'desc',
        evMap: new Map(),
      });

      expect(result).toHaveLength(1);
      expect(result[0].purchase.id).toBe('1');
    });

    it('returns empty array when no items have receivedAt', () => {
      const items = [
        makeItem({ purchase: { id: '1', receivedAt: undefined } }),
        makeItem({ purchase: { id: '2', receivedAt: undefined } }),
      ];

      const result = filterAndSortItems(items, {
        debouncedSearch: '',
        showAll: false,
        filterTab: 'in_hand',
        sellSheetHas: () => false,
        sortKey: 'days',
        sortDir: 'desc',
        evMap: new Map(),
      });

      expect(result).toHaveLength(0);
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
        sellSheetHas: () => false,
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
        sellSheetHas: () => false,
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
        sellSheetHas: () => false,
        sortKey: 'days',
        sortDir: 'desc',
        evMap: new Map(),
      });

      expect(result).toHaveLength(1);
      expect(result[0].purchase.cardName).toBe('Charizard');
    });

    // Users can pre-add a cert to their sell sheet before it arrives, but
    // the sell-sheet view (and any printed sheet) should hide it until the
    // cert is physically received.
    it('excludes non-on-hand items from sell_sheet filter', () => {
      const items = [
        makeItem({ purchase: { id: '1', receivedAt: '2026-04-08T00:00:00Z' } }),
        makeItem({ purchase: { id: '2', receivedAt: undefined } }),
      ];
      const sellSheetIds = new Set(['1', '2']);

      const result = filterAndSortItems(items, {
        debouncedSearch: '',
        showAll: false,
        filterTab: 'sell_sheet',
        sellSheetHas: (id) => sellSheetIds.has(id),
        sortKey: 'days',
        sortDir: 'desc',
        evMap: new Map(),
      });

      expect(result).toHaveLength(1);
      expect(result[0].purchase.id).toBe('1');
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

    it('returns false when reviewedPriceCents is 0', () => {
      const item = makeItem({
        purchase: {
          receivedAt: '2026-04-08T00:00:00Z',
          dhInventoryId: 42,
          dhStatus: 'in_stock',
          reviewedPriceCents: 0,
        },
      });
      expect(isReadyToList(item)).toBe(false);
    });

    it('returns false when reviewedPriceCents is undefined', () => {
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
    it('returns true when received + pushed to DH + not listed + no reviewed price', () => {
      const item = makeItem({
        purchase: {
          receivedAt: '2026-04-08T00:00:00Z',
          dhInventoryId: 42,
          dhStatus: 'in_stock',
          reviewedPriceCents: 0,
        },
      });
      expect(needsPriceReview(item)).toBe(true);
    });

    it('returns true when reviewedPriceCents is undefined', () => {
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

      const withPrice = makeItem({ purchase: { ...base, reviewedPriceCents: 9000 } });
      expect(isReadyToList(withPrice)).toBe(true);
      expect(needsPriceReview(withPrice)).toBe(false);

      const withoutPrice = makeItem({ purchase: { ...base, reviewedPriceCents: 0 } });
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
});
