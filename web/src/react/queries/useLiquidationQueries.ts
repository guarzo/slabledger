import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { api } from '../../js/api';
import type { LiquidationPreviewResponse, LiquidationApplyItem, LiquidationApplyResult } from '../../types/liquidation';
import { queryKeys } from './queryKeys';

export function useLiquidationPreview(discountWithCompsPct: number, discountNoCompsPct: number) {
  return useQuery<LiquidationPreviewResponse>({
    queryKey: ['liquidation', 'preview', discountWithCompsPct, discountNoCompsPct],
    queryFn: () => api.getLiquidationPreview(discountWithCompsPct, discountNoCompsPct),
  });
}

export function useApplyLiquidation() {
  const queryClient = useQueryClient();

  return useMutation<LiquidationApplyResult, Error, LiquidationApplyItem[]>({
    mutationFn: (items) => api.applyLiquidation(items),
    onSettled: () => {
      queryClient.invalidateQueries({ queryKey: ['liquidation'] });
      queryClient.invalidateQueries({ queryKey: queryKeys.portfolio.globalInventory });
    },
  });
}
