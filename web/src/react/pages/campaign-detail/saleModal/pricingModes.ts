import type { AgingItem } from '../../../../types/campaigns';

export type PricingMode = 'pctOfCL' | 'flat';

/**
 * Compute the sale price (in cents) for a single item given a uniform pricing mode.
 *
 * - 'pctOfCL': item.purchase.clValueCents × value / 100, rounded.
 * - 'flat':    value (already in cents), unchanged.
 *
 * Negative inputs are clamped to 0.
 */
export function computeSalePrice(
  item: AgingItem,
  mode: PricingMode,
  value: number,
): number {
  if (value <= 0) return 0;

  if (mode === 'flat') {
    return Math.round(value);
  }

  // pctOfCL
  const cl = item.purchase.clValueCents ?? 0;
  if (cl <= 0) return 0;
  return Math.round((cl * value) / 100);
}
