import { describe, it, expect } from 'vitest';
import { buildPriceSources, preSelectSource } from './priceDecisionHelpers';

describe('buildPriceSources', () => {
  it('includes Cost and CL first, then DH and Last Sold at end', () => {
    const sources = buildPriceSources({
      clCents: 1000,
      dhMidCents: 2000,
      costCents: 500,
      lastSoldCents: 1800,
    });

    expect(sources.map(s => s.source)).toEqual(['cost_markup', 'cl', 'market', 'last_sold']);
    expect(sources[0]).toMatchObject({ label: 'Cost', source: 'cost_markup', priceCents: 500 });
    expect(sources[1]).toMatchObject({ label: 'CL', source: 'cl', priceCents: 1000 });
    expect(sources[2]).toMatchObject({ label: 'DH', source: 'market', priceCents: 2000 });
    expect(sources[3]).toMatchObject({ label: 'Last Sold', source: 'last_sold', priceCents: 1800 });
  });
});

describe('preSelectSource fallback ordering', () => {
  it('selects the first non-zero source when no reviewedPriceCents given', () => {
    const sources = buildPriceSources({
      clCents: 0,
      dhMidCents: 2000,
      costCents: 500,
      lastSoldCents: 1800,
    });

    const result = preSelectSource(sources);
    expect(result).toEqual({ kind: 'source', source: 'cost_markup' });
  });

  it('selects cost_markup as fallback when it is the first non-zero source', () => {
    const sources = buildPriceSources({
      clCents: 1000,
      dhMidCents: 2000,
      costCents: 500,
      lastSoldCents: 1800,
    });

    const result = preSelectSource(sources);
    expect(result).toEqual({ kind: 'source', source: 'cost_markup' });
  });

  it('selects the first non-zero source when cost and cl are zero', () => {
    const sources = buildPriceSources({
      clCents: 0,
      dhMidCents: 2000,
      costCents: 0,
      lastSoldCents: 1800,
    });

    const result = preSelectSource(sources);
    expect(result).toEqual({ kind: 'source', source: 'market' });
  });

  it('returns none when all sources are zero', () => {
    const sources = buildPriceSources({
      clCents: 0,
      dhMidCents: 0,
      costCents: 0,
      lastSoldCents: 0,
    });

    const result = preSelectSource(sources);
    expect(result).toEqual({ kind: 'none' });
  });

  it('matches exact reviewedPriceCents to the correct source key', () => {
    const sources = buildPriceSources({
      clCents: 1000,
      dhMidCents: 2000,
      costCents: 500,
      lastSoldCents: 1800,
    });

    const result = preSelectSource(sources, 1000);
    expect(result).toEqual({ kind: 'source', source: 'cl' });
  });

  it('returns manual when reviewedPriceCents matches no source', () => {
    const sources = buildPriceSources({
      clCents: 1000,
      dhMidCents: 2000,
      costCents: 500,
      lastSoldCents: 1800,
    });

    const result = preSelectSource(sources, 9999);
    expect(result).toEqual({ kind: 'manual', priceCents: 9999 });
  });
});
