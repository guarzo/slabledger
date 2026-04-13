import { describe, it, expect } from 'vitest';
import { computeSellThrough, computeTotalProfit } from './useCampaignDerived';
import type { Sale } from '../../types/campaigns';

describe('computeSellThrough', () => {
  const cases = [
    {
      name: 'returns "0" when no cards',
      totalCards: 0,
      soldCards: 0,
      expected: '0',
    },
    {
      name: 'returns "100.0" when all sold',
      totalCards: 10,
      soldCards: 10,
      expected: '100.0',
    },
    {
      name: 'returns "50.0" when half sold',
      totalCards: 10,
      soldCards: 5,
      expected: '50.0',
    },
    {
      name: 'returns "33.3" for 1 of 3 sold',
      totalCards: 3,
      soldCards: 1,
      expected: '33.3',
    },
    {
      name: 'returns "0.0" when none sold but have cards',
      totalCards: 5,
      soldCards: 0,
      expected: '0.0',
    },
  ];

  for (const tc of cases) {
    it(tc.name, () => {
      expect(computeSellThrough(tc.totalCards, tc.soldCards)).toBe(tc.expected);
    });
  }
});

describe('computeTotalProfit', () => {
  function makeSale(netProfitCents: number): Sale {
    return {
      id: 'test',
      purchaseId: 'test',
      saleDate: '2024-01-01',
      salePriceCents: 0,
      saleChannel: 'ebay',
      saleFeeCents: 0,
      daysToSell: 0,
      netProfitCents,
      createdAt: '2024-01-01T00:00:00Z',
      updatedAt: '2024-01-01T00:00:00Z',
    };
  }

  const cases = [
    {
      name: 'returns 0 for empty sales',
      sales: [] as Sale[],
      expected: 0,
    },
    {
      name: 'returns single profit',
      sales: [makeSale(1000)],
      expected: 1000,
    },
    {
      name: 'sums positive profits',
      sales: [makeSale(500), makeSale(300), makeSale(200)],
      expected: 1000,
    },
    {
      name: 'handles negative profits (losses)',
      sales: [makeSale(500), makeSale(-200)],
      expected: 300,
    },
    {
      name: 'handles all negative (net loss)',
      sales: [makeSale(-100), makeSale(-200)],
      expected: -300,
    },
  ];

  for (const tc of cases) {
    it(tc.name, () => {
      expect(computeTotalProfit(tc.sales)).toBe(tc.expected);
    });
  }
});
