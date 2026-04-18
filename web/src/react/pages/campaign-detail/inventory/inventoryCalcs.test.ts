import { describe, it, expect } from 'vitest';
import { computeInventoryMeta, filterAndSortItems } from './inventoryCalcs';
import type { AgingItem, Purchase } from '../../../../types/campaigns';

type TestPurchase = Pick<Purchase,
  'id' | 'cardName' | 'gradeValue' | 'certNumber' | 'receivedAt' |
  'campaignId' | 'clValueCents' | 'buyCostCents' | 'psaSourcingFeeCents' | 'purchaseDate' |
  'createdAt' | 'updatedAt' | 'aiSuggestedPriceCents' | 'reviewedAt'
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
        makeItem({ purchase: { id: '1', aiSuggestedPriceCents: 5000, reviewedAt: '2026-04-10T00:00:00Z' } }),
        makeItem({ purchase: { id: '2', reviewedAt: '2026-04-10T00:00:00Z' } }),
      ];
      const meta = computeInventoryMeta(items);
      expect(meta.tabCounts.needs_attention).toBe(1);
    });

    it('returns correct structure with all tab counts', () => {
      const items = [makeItem()];
      const meta = computeInventoryMeta(items);

      expect(meta).toHaveProperty('tabCounts');
      expect(meta.tabCounts).toHaveProperty('needs_attention');
      expect(meta.tabCounts).toHaveProperty('card_show');
      expect(meta.tabCounts).toHaveProperty('in_hand');
      expect(meta.tabCounts).toHaveProperty('ready_to_list');
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

    // A pending-PSA cert can't be brought to a card show, so even when the
    // item matches every other card-show heuristic (grade 7 here) it must be
    // excluded until receivedAt is stamped.
    it('excludes non-on-hand items from card_show filter', () => {
      const items = [
        makeItem({ purchase: { id: '1', gradeValue: 7, receivedAt: '2026-04-08T00:00:00Z' } }),
        makeItem({ purchase: { id: '2', gradeValue: 7, receivedAt: undefined } }),
      ];

      const result = filterAndSortItems(items, {
        debouncedSearch: '',
        showAll: false,
        filterTab: 'card_show',
        sellSheetHas: () => false,
        sortKey: 'days',
        sortDir: 'desc',
        evMap: new Map(),
      });

      expect(result).toHaveLength(1);
      expect(result[0].purchase.id).toBe('1');
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

  describe('computeInventoryMeta card_show count', () => {
    it('only counts on-hand items toward card_show badge', () => {
      const items = [
        makeItem({ purchase: { id: '1', gradeValue: 7, receivedAt: '2026-04-08T00:00:00Z' } }),
        makeItem({ purchase: { id: '2', gradeValue: 7, receivedAt: undefined } }),
        makeItem({ purchase: { id: '3', gradeValue: 7, receivedAt: '2026-04-09T00:00:00Z' } }),
      ];

      const meta = computeInventoryMeta(items);
      expect(meta.tabCounts.card_show).toBe(2);
    });
  });
});
