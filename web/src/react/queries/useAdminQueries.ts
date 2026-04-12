import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { api } from '../../js/api';
import { queryKeys } from './queryKeys';
import type { DHFixMatchRequest, DHSelectMatchRequest } from '../../types/apiStatus';

/** Options shared by all admin read queries */
interface AdminQueryOptions {
  enabled?: boolean;
}

/**
 * Factory for admin query hooks. Reduces boilerplate for the enabled option.
 * Each generated hook accepts AdminQueryOptions and defaults enabled to true.
 */
function createAdminQuery<T>(
  queryKey: readonly unknown[],
  queryFn: () => Promise<T>,
) {
  return (options?: AdminQueryOptions) =>
    useQuery<T>({
      queryKey,
      queryFn,
      enabled: options?.enabled ?? true,
    });
}

export const useAllowlist = createAdminQuery(
  queryKeys.admin.allowlist,
  () => api.getAdminAllowlist(),
);

export const useAdminUsers = createAdminQuery(
  queryKeys.admin.users,
  () => api.getAdminUsers(),
);

export const useCardRequests = createAdminQuery(
  queryKeys.admin.cardRequests,
  () => api.getCardRequests(),
);

export function useAdminApiUsage(options?: { enabled?: boolean }) {
  return useQuery({
    queryKey: queryKeys.admin.apiUsage,
    queryFn: () => api.getAdminApiUsage(),
    refetchInterval: 60_000,
    staleTime: 30_000,
    enabled: options?.enabled ?? true,
  });
}

export function useAdminCacheStats(options?: { enabled?: boolean }) {
  return useQuery({
    queryKey: queryKeys.admin.cacheStats,
    queryFn: () => api.getAdminCacheStats(),
    staleTime: 60_000,
    enabled: options?.enabled ?? true,
  });
}

export function useAddAllowedEmail() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ email, notes }: { email: string; notes?: string }) =>
      api.addAllowedEmail(email, notes),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: queryKeys.admin.allowlist });
    },
  });
}

export function useRemoveAllowedEmail() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (email: string) => api.removeAllowedEmail(email),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: queryKeys.admin.allowlist });
    },
  });
}

export function usePriceOverrideStats(options?: { enabled?: boolean }) {
  return useQuery({
    queryKey: queryKeys.admin.priceOverrideStats,
    queryFn: () => api.getPriceOverrideStats(),
    staleTime: 60_000,
    enabled: options?.enabled ?? true,
  });
}

export function usePricingDiagnostics(options?: { enabled?: boolean }) {
  return useQuery({
    queryKey: queryKeys.admin.pricingDiagnostics,
    queryFn: () => api.getPricingDiagnostics(),
    staleTime: 60_000,
    enabled: options?.enabled ?? true,
  });
}

export function useSubmitCardRequest() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: number) => api.submitCardRequest(id),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: queryKeys.admin.cardRequests });
    },
  });
}

export function useSubmitAllCardRequests() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: () => api.submitAllCardRequests(),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: queryKeys.admin.cardRequests });
    },
  });
}

export function useAIUsage(options?: { enabled?: boolean }) {
  return useQuery({
    queryKey: queryKeys.admin.aiUsage,
    queryFn: () => api.getAIUsage(),
    refetchInterval: 60_000,
    staleTime: 30_000,
    enabled: options?.enabled ?? true,
  });
}

export function usePriceFlags(status: 'open' | 'resolved' | 'all', options?: { enabled?: boolean }) {
  return useQuery({
    queryKey: queryKeys.admin.priceFlags(status),
    queryFn: () => api.listPriceFlags(status),
    staleTime: 30_000,
    enabled: options?.enabled ?? true,
  });
}

export function useResolvePriceFlag() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (flagId: number) => api.resolvePriceFlag(flagId),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: queryKeys.admin.priceFlags('open') });
      qc.invalidateQueries({ queryKey: queryKeys.admin.priceFlags('resolved') });
      qc.invalidateQueries({ queryKey: queryKeys.admin.priceFlags('all') });
    },
  });
}

export function useCardLadderStatus(options?: { enabled?: boolean }) {
  return useQuery({
    queryKey: queryKeys.admin.cardLadderStatus,
    queryFn: () => api.getCardLadderStatus(),
    staleTime: 60_000,
    enabled: options?.enabled ?? true,
  });
}

export function useCardLadderFailures(options?: { enabled?: boolean }) {
  return useQuery({
    queryKey: queryKeys.admin.cardLadderFailures,
    queryFn: () => api.getCardLadderFailures(50),
    staleTime: 60_000,
    enabled: options?.enabled ?? false,
  });
}

export function useSaveCardLadderConfig() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (config: { email: string; password: string; collectionId: string; firebaseApiKey: string }) =>
      api.saveCardLadderConfig(config),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: queryKeys.admin.cardLadderStatus });
    },
  });
}

export function useTriggerCardLadderRefresh() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: () => api.triggerCardLadderRefresh(),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: queryKeys.admin.cardLadderStatus });
    },
  });
}

export function useSyncCardLadderCollection() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: () => api.syncCardLadderCollection(),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: queryKeys.admin.cardLadderStatus });
    },
  });
}

export function useMarketMoversStatus(options?: { enabled?: boolean }) {
  return useQuery({
    queryKey: queryKeys.admin.marketMoversStatus,
    queryFn: () => api.getMarketMoversStatus(),
    staleTime: 60_000,
    enabled: options?.enabled ?? true,
  });
}

export function useMarketMoversFailures(options?: { enabled?: boolean }) {
  return useQuery({
    queryKey: queryKeys.admin.marketMoversFailures,
    queryFn: () => api.getMarketMoversFailures(50),
    staleTime: 60_000,
    enabled: options?.enabled ?? false,
  });
}

export function useSaveMarketMoversConfig() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (config: { username: string; password: string }) =>
      api.saveMarketMoversConfig(config),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: queryKeys.admin.marketMoversStatus });
    },
  });
}

export function useTriggerMarketMoversRefresh() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: () => api.triggerMarketMoversRefresh(),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: queryKeys.admin.marketMoversStatus });
    },
  });
}

export function useSyncMarketMoversCollection() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: () => api.syncMarketMoversCollection(),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: queryKeys.admin.marketMoversStatus });
    },
  });
}

export function useDHStatus(options?: { enabled?: boolean }) {
  return useQuery({
    queryKey: queryKeys.admin.dhStatus,
    queryFn: () => api.getDHStatus(),
    staleTime: 60_000,
    enabled: options?.enabled ?? true,
    // Poll every 5s while bulk match is running so mapped/unmatched counts update live.
    refetchInterval: (query) =>
      query.state.data?.bulk_match_running ? 5_000 : false,
  });
}

export function useTriggerDHBulkMatch() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: () => api.triggerDHBulkMatch(),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: queryKeys.admin.dhStatus });
    },
  });
}

export function useDHUnmatched(options?: { enabled?: boolean }) {
  return useQuery({
    queryKey: queryKeys.admin.dhUnmatched,
    queryFn: () => api.getDHUnmatched(),
    staleTime: 60_000,
    enabled: options?.enabled ?? true,
  });
}

export function useFixDHMatch() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (req: DHFixMatchRequest) => api.fixDHMatch(req),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: queryKeys.admin.dhUnmatched });
      qc.invalidateQueries({ queryKey: queryKeys.admin.dhStatus });
    },
  });
}

export function useSelectDHMatch() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (req: DHSelectMatchRequest) => api.selectDHMatch(req),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: queryKeys.admin.dhUnmatched });
      qc.invalidateQueries({ queryKey: queryKeys.admin.dhStatus });
    },
  });
}

export function useDismissDHMatch() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (purchaseId: string) => api.dismissDHMatch(purchaseId),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: queryKeys.admin.dhUnmatched });
      qc.invalidateQueries({ queryKey: queryKeys.admin.dhStatus });
    },
  });
}

export function useUndismissDHMatch() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (purchaseId: string) => api.undismissDHMatch(purchaseId),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: queryKeys.admin.dhUnmatched });
      qc.invalidateQueries({ queryKey: queryKeys.admin.dhStatus });
    },
  });
}

export function usePSASyncStatus(options?: { enabled?: boolean }) {
  return useQuery({
    queryKey: queryKeys.admin.psaSyncStatus,
    queryFn: () => api.getPSASyncStatus(),
    staleTime: 60_000,
    enabled: options?.enabled ?? true,
  });
}

export function useTriggerPSASyncRefresh() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: () => api.triggerPSASyncRefresh(),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: queryKeys.admin.psaSyncStatus });
      qc.invalidateQueries({ queryKey: queryKeys.purchases.psaPendingItems });
    },
  });
}
