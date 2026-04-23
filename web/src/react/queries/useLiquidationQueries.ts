import { useState, useCallback } from 'react';
import { useMutation, useQueryClient } from '@tanstack/react-query';
import { api } from '../../js/api';
import type { LiquidationPreviewResponse, LiquidationApplyItem, LiquidationApplyResult } from '../../types/liquidation';
import { queryKeys } from './queryKeys';

export function useLiquidationPreview() {
  const [data, setData] = useState<LiquidationPreviewResponse | null>(null);
  const [isLoading, setIsLoading] = useState(false);
  const [error, setError] = useState<Error | null>(null);

  const fetchPreview = useCallback(async (baseDiscountPct: number, noCompDiscountPct: number) => {
    setIsLoading(true);
    setError(null);
    try {
      const result = await api.getLiquidationPreview(baseDiscountPct, noCompDiscountPct);
      setData(result);
    } catch (err) {
      setError(err instanceof Error ? err : new Error('Failed to fetch liquidation preview'));
    } finally {
      setIsLoading(false);
    }
  }, []);

  return { data, isLoading, error, fetchPreview };
}

export function useApplyLiquidation() {
  const queryClient = useQueryClient();

  return useMutation<LiquidationApplyResult, Error, LiquidationApplyItem[]>({
    mutationFn: (items) => api.applyLiquidation(items),
    onSettled: () => {
      queryClient.invalidateQueries({ queryKey: queryKeys.portfolio.globalInventory });
    },
  });
}
