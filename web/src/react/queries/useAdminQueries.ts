import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { api } from '../../js/api';
import { queryKeys } from './queryKeys';

export function useAllowlist() {
  return useQuery({
    queryKey: queryKeys.admin.allowlist,
    queryFn: () => api.getAdminAllowlist(),
  });
}

export function useAdminUsers() {
  return useQuery({
    queryKey: queryKeys.admin.users,
    queryFn: () => api.getAdminUsers(),
  });
}

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

export function useCardRequests(options?: { enabled?: boolean }) {
  return useQuery({
    queryKey: queryKeys.admin.cardRequests,
    queryFn: () => api.getCardRequests(),
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

export function useCardLadderStatus() {
  return useQuery({
    queryKey: queryKeys.admin.cardLadderStatus,
    queryFn: () => api.getCardLadderStatus(),
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
