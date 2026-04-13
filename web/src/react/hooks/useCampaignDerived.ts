import { useMemo } from 'react';
import type { Purchase, Sale } from '../../types/campaigns';

/** Returns sell-through as a formatted percentage string (e.g. "50.0"). */
export function computeSellThrough(totalCards: number, soldCards: number): string {
  if (totalCards === 0) return '0';
  return ((soldCards / totalCards) * 100).toFixed(1);
}

/** Sums the net profit across all sales (in cents). */
export function computeTotalProfit(sales: Sale[]): number {
  return sales.reduce((sum, s) => sum + s.netProfitCents, 0);
}

export function useCampaignDerived(purchases: Purchase[], sales: Sale[]) {
  return useMemo(() => {
    const soldPurchaseIds = new Set(sales.map(s => s.purchaseId));
    const unsoldPurchases = purchases.filter(p => !soldPurchaseIds.has(p.id));
    const totalSpent = purchases.reduce((sum, p) => sum + p.buyCostCents, 0);
    const totalRevenue = sales.reduce((sum, s) => sum + s.salePriceCents, 0);
    const totalProfit = computeTotalProfit(sales);
    const sellThrough = computeSellThrough(purchases.length, sales.length);

    return {
      soldPurchaseIds,
      unsoldPurchases,
      totalSpent,
      totalRevenue,
      totalProfit,
      sellThrough,
    };
  }, [purchases, sales]);
}
