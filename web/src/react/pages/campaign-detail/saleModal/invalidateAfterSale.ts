import type { QueryClient } from '@tanstack/react-query';
import { queryKeys } from '../../../queries/queryKeys';

/**
 * Invalidate all query caches affected by recording a sale.
 * Shared by RecordSaleModal (single) and BulkRecordSaleModal (bulk).
 */
export function invalidateAfterSale(queryClient: QueryClient, campaignIds: Iterable<string>) {
  for (const cid of campaignIds) {
    queryClient.invalidateQueries({ queryKey: queryKeys.campaigns.sales(cid) });
    queryClient.invalidateQueries({ queryKey: queryKeys.campaigns.purchases(cid) });
    queryClient.invalidateQueries({ queryKey: queryKeys.campaigns.pnl(cid) });
    queryClient.invalidateQueries({ queryKey: queryKeys.campaigns.inventory(cid) });
    queryClient.invalidateQueries({ queryKey: queryKeys.campaigns.channelPnl(cid) });
    queryClient.invalidateQueries({ queryKey: ['campaigns', cid, 'fillRate'] });
    queryClient.invalidateQueries({ queryKey: queryKeys.campaigns.daysToSell(cid) });
  }
  queryClient.invalidateQueries({ queryKey: queryKeys.portfolio.globalInventory });
  queryClient.invalidateQueries({ queryKey: queryKeys.portfolio.sellSheet });
  queryClient.invalidateQueries({ queryKey: queryKeys.portfolio.health });
  queryClient.invalidateQueries({ queryKey: queryKeys.portfolio.weeklyReview });
  queryClient.invalidateQueries({ queryKey: queryKeys.portfolio.channelVelocity });
  queryClient.invalidateQueries({ queryKey: queryKeys.portfolio.insights });
  queryClient.invalidateQueries({ queryKey: queryKeys.portfolio.suggestions });
}
