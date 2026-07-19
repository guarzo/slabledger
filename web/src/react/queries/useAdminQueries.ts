import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { api } from '../../js/api';
import { queryKeys } from './queryKeys';

/** Options shared by all admin read queries */
export interface AdminQueryOptions {
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

export function useAdminApiUsage(options?: AdminQueryOptions) {
  return useQuery({
    queryKey: queryKeys.admin.apiUsage,
    queryFn: () => api.getAdminApiUsage(),
    refetchInterval: 60_000,
    staleTime: 30_000,
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

export function usePriceOverrideStats(options?: AdminQueryOptions) {
  return useQuery({
    queryKey: queryKeys.admin.priceOverrideStats,
    queryFn: () => api.getPriceOverrideStats(),
    staleTime: 60_000,
    enabled: options?.enabled ?? true,
  });
}

export function usePricingDiagnostics(options?: AdminQueryOptions) {
  return useQuery({
    queryKey: queryKeys.admin.pricingDiagnostics,
    queryFn: () => api.getPricingDiagnostics(),
    staleTime: 60_000,
    enabled: options?.enabled ?? true,
  });
}

export function useAIUsage(options?: AdminQueryOptions) {
  return useQuery({
    queryKey: queryKeys.admin.aiUsage,
    queryFn: () => api.getAIUsage(),
    refetchInterval: 60_000,
    staleTime: 30_000,
    enabled: options?.enabled ?? true,
  });
}

export function usePriceFlags(status: 'open' | 'resolved' | 'all', options?: AdminQueryOptions) {
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

export function useCardLadderStatus(options?: AdminQueryOptions) {
  return useQuery({
    queryKey: queryKeys.admin.cardLadderStatus,
    queryFn: () => api.getCardLadderStatus(),
    staleTime: 60_000,
    enabled: options?.enabled ?? true,
  });
}

export function useCardLadderFailures(options?: AdminQueryOptions) {
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

export function useDHStatus(options?: AdminQueryOptions) {
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

export function useUnmatchDH() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (purchaseId: string) => api.unmatchDH(purchaseId),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: queryKeys.admin.dhUnmatched });
      qc.invalidateQueries({ queryKey: queryKeys.admin.dhStatus });
    },
  });
}

export function useTriggerDHReconcile() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: () => api.triggerDHReconcile(),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: queryKeys.admin.dhStatus });
      // Reconcile resets dh_inventory_id on rows DH deleted, which shifts
      // the "Pending DH Listing" tab count — refresh inventory views so the
      // operator sees the new counts without a manual reload.
      qc.invalidateQueries({ queryKey: queryKeys.campaigns.all });
      qc.invalidateQueries({ queryKey: queryKeys.portfolio.globalInventory });
    },
  });
}

export function usePSASyncStatus(options?: AdminQueryOptions) {
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

export function useDHTombstoneCount(options?: AdminQueryOptions) {
  return useQuery({
    queryKey: queryKeys.admin.dhTombstoneCount,
    queryFn: () => api.getDHTombstoneCount(),
    staleTime: 60_000,
    enabled: options?.enabled ?? true,
  });
}

export function useClearDHTombstones() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: () => api.clearDHTombstones(),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: queryKeys.admin.dhTombstoneCount });
    },
  });
}
