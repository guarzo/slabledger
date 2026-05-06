import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { api } from '../../js/api';
import type {
  PsaExchangeOpportunitiesResponse,
  PsaExchangePolicy,
  PsaExchangePolicySettings,
} from '../../types/psaExchange';
import { queryKeys } from './queryKeys';

export function usePsaExchangeOpportunities() {
  return useQuery<PsaExchangeOpportunitiesResponse>({
    queryKey: queryKeys.psaExchange.opportunities,
    queryFn: () => api.getPsaExchangeOpportunities(),
    staleTime: 60_000, // refetch at most once a minute on focus
  });
}

export function usePsaExchangePolicy() {
  return useQuery<PsaExchangePolicySettings>({
    queryKey: queryKeys.psaExchange.policy,
    queryFn: () => api.getPsaExchangePolicy(),
    staleTime: 30_000,
  });
}

export function useUpdatePsaExchangePolicy() {
  const qc = useQueryClient();
  return useMutation<PsaExchangePolicySettings, Error, PsaExchangePolicy>({
    mutationFn: (p) => api.updatePsaExchangePolicy(p),
    onSuccess: (data) => {
      qc.setQueryData(queryKeys.psaExchange.policy, data);
      // Re-fetch the opportunities table — new policy means new targets/scores.
      qc.invalidateQueries({ queryKey: queryKeys.psaExchange.opportunities });
    },
  });
}
