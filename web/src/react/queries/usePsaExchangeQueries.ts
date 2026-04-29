import { useQuery } from '@tanstack/react-query';
import { api } from '../../js/api';
import type { PsaExchangeOpportunitiesResponse } from '../../types/psaExchange';
import { queryKeys } from './queryKeys';

export function usePsaExchangeOpportunities() {
  return useQuery<PsaExchangeOpportunitiesResponse>({
    queryKey: queryKeys.psaExchange.opportunities,
    queryFn: () => api.getPsaExchangeOpportunities(),
    staleTime: 60_000, // refetch at most once a minute on focus
  });
}
