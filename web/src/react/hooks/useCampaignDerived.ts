import { useMemo } from 'react';
import type { Purchase, Sale } from '../../types/campaigns';

export function useCampaignDerived(purchases: Purchase[], sales: Sale[]) {
  return useMemo(() => {
    const soldPurchaseIds = new Set(sales.map(s => s.purchaseId));
    const unsoldPurchases = purchases.filter(p => !soldPurchaseIds.has(p.id));
    const totalSpent = purchases.reduce((sum, p) => sum + p.buyCostCents, 0);
    const totalRevenue = sales.reduce((sum, s) => sum + s.salePriceCents, 0);
    const totalProfit = sales.reduce((sum, s) => sum + s.netProfitCents, 0);
    const sellThrough = purchases.length > 0
      ? ((sales.length / purchases.length) * 100).toFixed(1)
      : '0';

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
