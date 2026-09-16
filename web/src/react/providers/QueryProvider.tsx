import { useEffect, useLayoutEffect, useMemo } from 'react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import type { ReactNode } from 'react';

export default function QueryProvider({ children, sessionKey = null, routeKey }: { children: ReactNode; sessionKey?: number | null; routeKey?: string }) {
  // Retain cached data across routes, never across identities.
  const queryClient = useMemo(
    () =>
      new QueryClient({
        defaultOptions: {
          queries: {
            staleTime: 5 * 60 * 1000, // 5 minutes
            retry: 2,
            refetchOnWindowFocus: false,
          },
        },
      }),
    // The identity is deliberately the client lifetime boundary.
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [sessionKey],
  );
  useEffect(() => () => {
    queryClient.clear();
  }, [queryClient]);
  useLayoutEffect(() => {
    // Previously route remounts discarded these caches. Keep their fresh-on-entry
    // behavior without discarding cached show data.
    void queryClient.invalidateQueries({ predicate: query => query.queryKey[0] !== 'show-prep', refetchType: 'none' });
  }, [queryClient, routeKey]);

  return (
    <QueryClientProvider client={queryClient}>
      {children}
    </QueryClientProvider>
  );
}
