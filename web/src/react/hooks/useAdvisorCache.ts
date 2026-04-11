import { useQuery, useQueryClient } from '@tanstack/react-query';
import { useCallback, useMemo } from 'react';
import { api } from '../../js/api';
import { reportError } from '../../js/errors';
import type { AdvisorAnalysisType } from '../../types/apiStatus';

const CACHE_KEY_PREFIX = ['advisor', 'cache'] as const;

export function useAdvisorCache(type: AdvisorAnalysisType) {
  const queryClient = useQueryClient();
  const queryKey = useMemo(() => [...CACHE_KEY_PREFIX, type], [type]);

  const { data, isLoading, error } = useQuery({
    queryKey,
    queryFn: () => api.getAdvisorCache(type),
    refetchInterval: (query) => {
      const status = query.state.data?.status;
      return status === 'running' ? 5_000 : 60_000;
    },
    staleTime: 10_000,
  });

  const refresh = useCallback(async () => {
    try {
      await api.refreshAdvisorCache(type);
      await queryClient.invalidateQueries({ queryKey });
    } catch (err) {
      reportError('useAdvisorCache/refresh', err);
      throw err;
    }
  }, [type, queryClient, queryKey]);

  return { data, isLoading, error, refresh };
}
