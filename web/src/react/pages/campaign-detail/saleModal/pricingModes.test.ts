import { describe, it, expect } from 'vitest';
import { computeSalePrice } from './pricingModes';
import type { AgingItem } from '../../../../types/campaigns';

function itemWithCL(clValueCents: number): AgingItem {
  return {
    purchase: {
      id: 'p1',
      campaignId: 'c1',
      cardName: 'Test',
      setName: 'Set',
      certNumber: '1',
      grader: 'PSA',
      gradeValue: 10,
      cardNumber: '1',
      buyCostCents: 0,
      psaSourcingFeeCents: 0,
      clValueCents,
      frontImageUrl: '',
      purchaseDate: '2026-01-01',
      receivedAt: undefined,
      createdAt: '2026-01-01T00:00:00Z',
      updatedAt: '2026-01-01T00:00:00Z',
    },
    daysHeld: 0,
    campaignName: 'C',
    currentMarket: undefined,
    signal: undefined,
    priceAnomaly: false,
  } as AgingItem;
}

describe('computeSalePrice', () => {
  describe('% of CL mode', () => {
    it('multiplies CL by percent and rounds to nearest cent', () => {
      // 4700 * 70 / 100 = 3290
      expect(computeSalePrice(itemWithCL(4700), 'pctOfCL', 70)).toBe(3290);
    });

    it('rounds half-cents away from zero', () => {
      // 4701 * 70 / 100 = 3290.7 -> 3291
      expect(computeSalePrice(itemWithCL(4701), 'pctOfCL', 70)).toBe(3291);
    });

    it('returns 0 when CL is missing', () => {
      expect(computeSalePrice(itemWithCL(0), 'pctOfCL', 70)).toBe(0);
    });

    it('returns 0 for zero percent', () => {
      expect(computeSalePrice(itemWithCL(4700), 'pctOfCL', 0)).toBe(0);
    });

    it('returns CL value for 100 percent', () => {
      expect(computeSalePrice(itemWithCL(4700), 'pctOfCL', 100)).toBe(4700);
    });

    it('clamps negative percent to 0', () => {
      expect(computeSalePrice(itemWithCL(4700), 'pctOfCL', -10)).toBe(0);
    });
  });

  describe('Flat $ mode', () => {
    it('returns the flat cents value regardless of CL', () => {
      expect(computeSalePrice(itemWithCL(4700), 'flat', 500)).toBe(500);
    });

    it('returns 0 when value is 0', () => {
      expect(computeSalePrice(itemWithCL(4700), 'flat', 0)).toBe(0);
    });

    it('clamps negative flat value to 0', () => {
      expect(computeSalePrice(itemWithCL(4700), 'flat', -100)).toBe(0);
    });
  });
});
