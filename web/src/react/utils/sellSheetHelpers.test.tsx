import { describe, it, expect } from 'vitest';
import { clPriceDisplayCents, formatLastSaleDate } from './sellSheetHelpers';

describe('clPriceDisplayCents', () => {
  it('returns CL value when present', () => {
    expect(clPriceDisplayCents({ clValueCents: 27900, recommendedPriceCents: 25000 })).toEqual({ cents: 27900, estimated: false });
  });
  it('falls back to recommended price with estimated flag when CL missing', () => {
    expect(clPriceDisplayCents({ clValueCents: 0, recommendedPriceCents: 18500 })).toEqual({ cents: 18500, estimated: true });
  });
  it('returns null when both are missing', () => {
    expect(clPriceDisplayCents({ clValueCents: 0, recommendedPriceCents: 0 })).toBeNull();
    expect(clPriceDisplayCents({ clValueCents: 0 })).toBeNull();
  });
});

describe('formatLastSaleDate', () => {
  it('formats ISO date as MM/DD/YY', () => {
    expect(formatLastSaleDate('2026-03-12T00:00:00Z')).toBe('03/12/26');
  });
  it('formats date-only ISO', () => {
    expect(formatLastSaleDate('2026-03-12')).toBe('03/12/26');
  });
  it('returns empty string for missing input', () => {
    expect(formatLastSaleDate(undefined)).toBe('');
    expect(formatLastSaleDate('')).toBe('');
  });
  it('returns empty string for unparseable input', () => {
    expect(formatLastSaleDate('not a date')).toBe('');
  });
});
