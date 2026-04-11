import { describe, it, expect } from 'vitest';
import { computeInventoryMeta, filterAndSortItems } from './inventoryCalcs';
import type { AgingItem } from '../../../../types/campaigns';

function makeItem(overrides?: Partial<AgingItem>): AgingItem {
  const item: AgingItem = {
    purchase: {
      id: '1',
      cardName: 'Test Card',
      gradeValue: 8,
      costCents: 1000,
      certNumber: 'PSA-123456',
      receivedAt: undefined,
      campaignId: 'camp-1',
      setName: 'Base',
      cardNumber: '001',
      clValueCents: 2000,
      ...overrides?.purchase,
    } as any,
    daysHeld: 0,
    ...overrides,
  };
  return item;
}

describe('inventoryCalcs', () => {
  describe('computeInventoryMeta', () => {
    it('counts in_hand items with receivedAt set', () => {
      const items = [
        makeItem({ purchase: { receivedAt: '2026-04-08T00:00:00Z' } as any }),
        makeItem({ purchase: { receivedAt: undefined } as any }),
        makeItem({ purchase: { receivedAt: '2026-04-09T00:00:00Z' } as any }),
      ];

      const meta = computeInventoryMeta(items);
      expect(meta.tabCounts.in_hand).toBe(2);
    });

    it('initializes in_hand count to 0 when no items have receivedAt', () => {
      const items = [
        makeItem({ purchase: { receivedAt: undefined } as any }),
        makeItem({ purchase: { receivedAt: undefined } as any }),
      ];

      const meta = computeInventoryMeta(items);
      expect(meta.tabCounts.in_hand).toBe(0);
    });

    it('counts all items in in_hand when all have receivedAt', () => {
      const items = [
        makeItem({ purchase: { receivedAt: '2026-04-08T00:00:00Z' } as any }),
        makeItem({ purchase: { receivedAt: '2026-04-09T00:00:00Z' } as any }),
      ];

      const meta = computeInventoryMeta(items);
      expect(meta.tabCounts.in_hand).toBe(2);
      expect(meta.tabCounts.all).toBe(2);
    });

    it('returns correct structure with all tab counts', () => {
      const items = [makeItem()];
      const meta = computeInventoryMeta(items);

      expect(meta).toHaveProperty('tabCounts');
      expect(meta.tabCounts).toHaveProperty('needs_attention');
      expect(meta.tabCounts).toHaveProperty('ai_suggestion');
      expect(meta.tabCounts).toHaveProperty('card_show');
      expect(meta.tabCounts).toHaveProperty('in_hand');
      expect(meta.tabCounts).toHaveProperty('all');
    });
  });

  describe('filterAndSortItems', () => {
    it('filters items by in_hand tab (receivedAt present)', () => {
      const items = [
        makeItem({ purchase: { id: '1', receivedAt: '2026-04-08T00:00:00Z' } as any }),
        makeItem({ purchase: { id: '2', receivedAt: undefined } as any }),
        makeItem({ purchase: { id: '3', receivedAt: '2026-04-09T00:00:00Z' } as any }),
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
        makeItem({ purchase: { id: '1', receivedAt: '2026-04-08T00:00:00Z' } as any }),
        makeItem({ purchase: { id: '2', receivedAt: '' as any } as any }),
        makeItem({ purchase: { id: '3', receivedAt: undefined } as any }),
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
        makeItem({ purchase: { id: '1', receivedAt: undefined } as any }),
        makeItem({ purchase: { id: '2', receivedAt: undefined } as any }),
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
        makeItem({ purchase: { id: '1', receivedAt: '2026-04-08T00:00:00Z' } as any }),
        makeItem({ purchase: { id: '2', receivedAt: undefined } as any }),
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
        makeItem({ purchase: { id: '1', receivedAt: '2026-04-08T00:00:00Z' } as any }),
        makeItem({ purchase: { id: '2', receivedAt: undefined } as any }),
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
        makeItem({ purchase: { id: '1', cardName: 'Pikachu', receivedAt: '2026-04-08T00:00:00Z' } as any }),
        makeItem({ purchase: { id: '2', cardName: 'Charizard', receivedAt: undefined } as any }),
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
  });
});
