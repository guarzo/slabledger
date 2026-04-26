import { describe, it, expect } from 'vitest';
import { clPriceDisplayCents } from './sellSheetHelpers';

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
