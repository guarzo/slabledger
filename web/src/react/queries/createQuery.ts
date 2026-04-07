import { useQuery, type UseQueryResult } from '@tanstack/react-query';

interface QueryOptions {
  staleTime?: number;
}

/**
 * Creates a query hook for a parameterized query (e.g., useCampaign(id)).
 * Automatically sets `enabled: !!param` so the query is skipped for falsy params.
 */
export function createParamQuery<T, P extends string>(
  keyFn: (param: P) => readonly unknown[],
  apiFn: (param: P) => Promise<T>,
  options?: QueryOptions,
): (param: P) => UseQueryResult<T> {
  return (param: P) =>
    useQuery({
      queryKey: keyFn(param),
      queryFn: () => apiFn(param),
      enabled: !!param,
      staleTime: options?.staleTime,
    });
}

/**
 * Creates a query hook for a non-parameterized query (e.g., useCapitalSummary()).
 */
export function createStaticQuery<T>(
  queryKey: readonly unknown[],
  apiFn: () => Promise<T>,
  options?: QueryOptions,
): () => UseQueryResult<T> {
  return () =>
    useQuery({
      queryKey,
      queryFn: apiFn,
      staleTime: options?.staleTime,
    });
}
