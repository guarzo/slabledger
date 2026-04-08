import type { PriceSource } from './PriceDecisionBar';

export type PreSelection =
  | { kind: 'source'; source: string }
  | { kind: 'manual'; priceCents: number }
  | { kind: 'none' };

/** Standard price source labels and keys used across all PriceDecisionBar consumers. */
export function buildPriceSources(prices: {
  clCents: number;
  marketCents: number;
  costCents: number;
  lastSoldCents: number;
  mmCents?: number;
}): PriceSource[] {
  const sources: PriceSource[] = [
    { label: 'CL', priceCents: prices.clCents, source: 'cl' },
    { label: 'Market', priceCents: prices.marketCents, source: 'market' },
    { label: 'Cost', priceCents: prices.costCents, source: 'cost_markup' },
    { label: 'Last Sold', priceCents: prices.lastSoldCents, source: 'last_sold' },
  ];
  if (prices.mmCents && prices.mmCents > 0) {
    sources.push({ label: 'MM', priceCents: prices.mmCents, source: 'mm' });
  }
  return sources;
}

/**
 * Pick the best pre-selected source for a PriceDecisionBar.
 * Returns { kind: 'source' } when a source matches the reviewed price or as a fallback,
 * { kind: 'manual' } when reviewedPriceCents exists but doesn't match any source,
 * or { kind: 'none' } when no selection is possible.
 */
export function preSelectSource(sources: PriceSource[], reviewedPriceCents?: number): PreSelection {
  if (reviewedPriceCents && reviewedPriceCents > 0) {
    const match = sources.find(s => s.priceCents === reviewedPriceCents && s.priceCents > 0);
    if (match) return { kind: 'source', source: match.source };
    return { kind: 'manual', priceCents: reviewedPriceCents };
  }
  const fallback = sources.find(s => s.priceCents > 0);
  if (fallback) return { kind: 'source', source: fallback.source };
  return { kind: 'none' };
}
