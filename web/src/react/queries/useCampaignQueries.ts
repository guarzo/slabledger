import { useQuery, useMutation, useQueryClient, type QueryClient } from '@tanstack/react-query';
import { api } from '../../js/api';
import type { Campaign, CreateCampaignInput, Invoice } from '../../types/campaigns';
import { queryKeys } from './queryKeys';
import { createParamQuery, createStaticQuery } from './createQuery';

/**
 * Invalidates all queries related to purchases for a given campaign.
 * Call from mutation onSettled handlers to keep data consistent.
 */
function invalidatePurchaseRelatedQueries(queryClient: QueryClient, campaignId: string): void {
  queryClient.invalidateQueries({ queryKey: queryKeys.campaigns.purchases(campaignId) });
  queryClient.invalidateQueries({ queryKey: queryKeys.campaigns.pnl(campaignId) });
  queryClient.invalidateQueries({ queryKey: queryKeys.campaigns.inventory(campaignId) });
  queryClient.invalidateQueries({ queryKey: queryKeys.portfolio.globalInventory });
}

/** Default stale time for campaign data (30 seconds). */
const CAMPAIGN_STALE_TIME = 30_000;

/** Longer stale time for expensive analytics queries (2 minutes). */
const ANALYTICS_STALE_TIME = 120_000;

export function useCampaigns(activeOnly: boolean) {
  return useQuery({
    queryKey: [...queryKeys.campaigns.all, activeOnly],
    queryFn: () => api.listCampaigns(activeOnly),
    staleTime: CAMPAIGN_STALE_TIME,
  });
}

/**
 * Query options factory for campaign PNL — use with useQueries() for bulk fetching.
 */
export const campaignPNLQueryOptions = (id: string) => ({
  queryKey: queryKeys.campaigns.pnl(id),
  queryFn: () => api.getCampaignPNL(id),
  enabled: !!id,
  staleTime: ANALYTICS_STALE_TIME,
});

// Capital & Invoice queries

export const useCapitalSummary = createStaticQuery(
  queryKeys.credit.summary, () => api.getCapitalSummary(),
);

export const useInvoices = createStaticQuery(
  queryKeys.credit.invoices, () => api.listInvoices(),
);

export const usePortfolioHealth = createStaticQuery(
  queryKeys.portfolio.health, () => api.getPortfolioHealth(), { staleTime: ANALYTICS_STALE_TIME },
);

export const useWeeklyReview = createStaticQuery(
  queryKeys.portfolio.weeklyReview, () => api.getWeeklyReview(),
);

export function useGlobalInventory() {
  const query = useQuery({
    queryKey: queryKeys.portfolio.globalInventory,
    queryFn: () => api.getGlobalInventory(),
    staleTime: ANALYTICS_STALE_TIME,
  });
  return { ...query, data: query.data?.items, warnings: query.data?.warnings };
}

export const useExpectedValues = createParamQuery(
  queryKeys.campaigns.expectedValues, (id) => api.getExpectedValues(id),
);

// Credit & Invoice mutations

export function useUpdateInvoice() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({ id, data }: { id: string; data: Partial<Invoice> }) => api.updateInvoice(id, data),
    onSettled: () => {
      queryClient.invalidateQueries({ queryKey: queryKeys.credit.invoices });
      queryClient.invalidateQueries({ queryKey: queryKeys.credit.summary });
    },
  });
}

// Mutations

export function useCreateCampaign() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (data: CreateCampaignInput) => api.createCampaign(data),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: queryKeys.campaigns.all });
    },
  });
}

export function useUpdateCampaign() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({ id, data, ifUnmodifiedSince }: { id: string; data: Partial<Campaign>; ifUnmodifiedSince?: string }) =>
      api.updateCampaign(id, data, ifUnmodifiedSince),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: queryKeys.campaigns.all });
    },
  });
}

export function usePSAPendingItems() {
  return useQuery({
    queryKey: queryKeys.purchases.psaPendingItems,
    queryFn: () => api.listPSAPendingItems(),
    staleTime: 30_000,
  });
}

/**
 * Resolves a PSA cert number to real card details. Used to display a meaningful
 * card name for pending items whose PSA listing title was a placeholder (e.g.
 * "PSA Offer - <cert>" from instant-offer acquisitions that never got a title).
 * Enabled only when a cert number is provided; results are cached indefinitely
 * since cert metadata never changes.
 */
export function useCertLookup(certNumber: string | undefined, enabled: boolean) {
  return useQuery({
    queryKey: queryKeys.certs.lookup(certNumber ?? ''),
    queryFn: () => api.lookupCert(certNumber as string),
    enabled: enabled && !!certNumber,
    staleTime: Infinity,
    gcTime: Infinity,
    retry: false,
  });
}

export function useAssignPendingItem() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, campaignId }: { id: string; campaignId: string }) =>
      api.assignPSAPendingItem(id, campaignId),
    onSuccess: (_data, variables) => {
      qc.invalidateQueries({ queryKey: queryKeys.purchases.psaPendingItems });
      qc.invalidateQueries({ queryKey: queryKeys.admin.psaSyncStatus });
      invalidatePurchaseRelatedQueries(qc, variables.campaignId);
    },
  });
}

export function useDismissPendingItem() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => api.dismissPSAPendingItem(id),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: queryKeys.purchases.psaPendingItems });
      qc.invalidateQueries({ queryKey: queryKeys.admin.psaSyncStatus });
    },
  });
}
