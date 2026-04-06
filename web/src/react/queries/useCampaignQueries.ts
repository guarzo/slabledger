import { useQuery, useMutation, useQueryClient, type QueryClient } from '@tanstack/react-query';
import { api } from '../../js/api';
import type { Campaign, CreateCampaignInput, CreatePurchaseInput, CreateSaleInput, Purchase, Sale, Invoice } from '../../types/campaigns';
import { queryKeys } from './queryKeys';

/**
 * Invalidates all queries related to purchases for a given campaign.
 * Call from mutation onSettled handlers to keep data consistent.
 */
function invalidatePurchaseRelatedQueries(queryClient: QueryClient, campaignId: string): void {
  queryClient.invalidateQueries({ queryKey: queryKeys.campaigns.purchases(campaignId) });
  queryClient.invalidateQueries({ queryKey: queryKeys.campaigns.pnl(campaignId) });
  queryClient.invalidateQueries({ queryKey: queryKeys.campaigns.inventory(campaignId) });
  queryClient.invalidateQueries({ queryKey: queryKeys.portfolio.globalInventory });
  queryClient.invalidateQueries({ queryKey: queryKeys.portfolio.sellSheet });
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

export function useCampaign(id: string) {
  return useQuery({
    queryKey: queryKeys.campaigns.detail(id),
    queryFn: () => api.getCampaign(id),
    enabled: !!id,
    staleTime: CAMPAIGN_STALE_TIME,
  });
}

export function usePurchases(campaignId: string) {
  return useQuery({
    queryKey: queryKeys.campaigns.purchases(campaignId),
    queryFn: () => api.listPurchases(campaignId),
    enabled: !!campaignId,
    staleTime: CAMPAIGN_STALE_TIME,
  });
}

export function useSales(campaignId: string) {
  return useQuery({
    queryKey: queryKeys.campaigns.sales(campaignId),
    queryFn: () => api.listSales(campaignId),
    enabled: !!campaignId,
  });
}

export function useCampaignPNL(campaignId: string) {
  return useQuery({
    queryKey: queryKeys.campaigns.pnl(campaignId),
    queryFn: () => api.getCampaignPNL(campaignId),
    enabled: !!campaignId,
    staleTime: ANALYTICS_STALE_TIME,
  });
}

export function useChannelPNL(campaignId: string) {
  return useQuery({
    queryKey: queryKeys.campaigns.channelPnl(campaignId),
    queryFn: () => api.getPNLByChannel(campaignId),
    enabled: !!campaignId,
  });
}

export function useFillRate(campaignId: string, days: number = 30) {
  return useQuery({
    queryKey: queryKeys.campaigns.fillRate(campaignId, days),
    queryFn: () => api.getFillRate(campaignId, days),
    enabled: !!campaignId,
  });
}

export function useDaysToSell(campaignId: string) {
  return useQuery({
    queryKey: queryKeys.campaigns.daysToSell(campaignId),
    queryFn: () => api.getDaysToSell(campaignId),
    enabled: !!campaignId,
  });
}

export function useInventory(campaignId: string, opts?: { enabled?: boolean }) {
  return useQuery({
    queryKey: queryKeys.campaigns.inventory(campaignId),
    queryFn: () => api.getInventory(campaignId),
    enabled: !!campaignId && (opts?.enabled ?? true),
    staleTime: ANALYTICS_STALE_TIME,
  });
}

export function useTuning(campaignId: string) {
  return useQuery({
    queryKey: queryKeys.campaigns.tuning(campaignId),
    queryFn: () => api.getCampaignTuning(campaignId),
    enabled: !!campaignId,
    staleTime: ANALYTICS_STALE_TIME,
  });
}

// Capital & Invoice queries

export function useCapitalSummary() {
  return useQuery({
    queryKey: queryKeys.credit.summary,
    queryFn: () => api.getCapitalSummary(),
  });
}

export function useInvoices() {
  return useQuery({
    queryKey: queryKeys.credit.invoices,
    queryFn: () => api.listInvoices(),
  });
}

export function usePortfolioHealth() {
  return useQuery({
    queryKey: queryKeys.portfolio.health,
    queryFn: () => api.getPortfolioHealth(),
    staleTime: ANALYTICS_STALE_TIME,
  });
}

export function usePortfolioChannelVelocity() {
  return useQuery({
    queryKey: queryKeys.portfolio.channelVelocity,
    queryFn: () => api.getPortfolioChannelVelocity(),
  });
}

export function usePortfolioInsights() {
  return useQuery({
    queryKey: queryKeys.portfolio.insights,
    queryFn: () => api.getPortfolioInsights(),
    staleTime: ANALYTICS_STALE_TIME,
  });
}

export function useCapitalTimeline() {
  return useQuery({
    queryKey: queryKeys.portfolio.capitalTimeline,
    queryFn: () => api.getCapitalTimeline(),
  });
}

export function useWeeklyReview() {
  return useQuery({
    queryKey: queryKeys.portfolio.weeklyReview,
    queryFn: () => api.getWeeklyReview(),
  });
}

export function useGlobalSellSheet() {
  return useQuery({
    queryKey: queryKeys.portfolio.sellSheet,
    queryFn: () => api.generateGlobalSellSheet(),
  });
}

export function useGlobalInventory() {
  return useQuery({
    queryKey: queryKeys.portfolio.globalInventory,
    queryFn: () => api.getGlobalInventory(),
    staleTime: ANALYTICS_STALE_TIME,
  });
}

export function useCrackCandidates(campaignId: string) {
  return useQuery({
    queryKey: queryKeys.campaigns.crackCandidates(campaignId),
    queryFn: () => api.getCrackCandidates(campaignId),
    enabled: !!campaignId,
  });
}

export function useExpectedValues(campaignId: string) {
  return useQuery({
    queryKey: queryKeys.campaigns.expectedValues(campaignId),
    queryFn: () => api.getExpectedValues(campaignId),
    enabled: !!campaignId,
  });
}

export function useActivationChecklist(campaignId: string) {
  return useQuery({
    queryKey: queryKeys.campaigns.activationChecklist(campaignId),
    queryFn: () => api.getActivationChecklist(campaignId),
    enabled: !!campaignId,
  });
}

export function useProjections(campaignId: string) {
  return useQuery({
    queryKey: queryKeys.campaigns.projections(campaignId),
    queryFn: () => api.getProjections(campaignId),
    enabled: !!campaignId,
  });
}

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

export function useImportPSA() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (file: File) => api.globalImportPSA(file),
    onSettled: () => {
      queryClient.invalidateQueries({ queryKey: queryKeys.campaigns.all });
      queryClient.invalidateQueries({ queryKey: queryKeys.credit.summary });
      queryClient.invalidateQueries({ queryKey: queryKeys.credit.invoices });
      queryClient.invalidateQueries({ queryKey: queryKeys.portfolio.health });
      queryClient.invalidateQueries({ queryKey: queryKeys.portfolio.insights });
      queryClient.invalidateQueries({ queryKey: queryKeys.portfolio.suggestions });
      queryClient.invalidateQueries({ queryKey: queryKeys.portfolio.globalInventory });
      queryClient.invalidateQueries({ queryKey: queryKeys.portfolio.sellSheet });
    },
  });
}

// Mutations

export function useUpdateCampaign(campaignId: string) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (data: Partial<Campaign>) => api.updateCampaign(campaignId, data),
    onSuccess: (updated) => {
      queryClient.setQueryData(queryKeys.campaigns.detail(campaignId), updated);
    },
    onSettled: () => {
      queryClient.invalidateQueries({ queryKey: queryKeys.campaigns.detail(campaignId) });
    },
  });
}

export function useCreateCampaign() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (data: CreateCampaignInput) => api.createCampaign(data),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: queryKeys.campaigns.all });
    },
  });
}

export function useCreatePurchase(campaignId: string) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (data: CreatePurchaseInput) => api.createPurchase(campaignId, data),
    onMutate: async (newPurchase) => {
      await queryClient.cancelQueries({
        queryKey: queryKeys.campaigns.purchases(campaignId),
      });
      const previous = queryClient.getQueryData(
        queryKeys.campaigns.purchases(campaignId),
      );
      queryClient.setQueryData(
        queryKeys.campaigns.purchases(campaignId),
        (old: Purchase[] = []) => [
          ...old,
          {
            ...newPurchase,
            id: `optimistic-${Date.now()}`,
            createdAt: new Date().toISOString(),
          },
        ],
      );
      return { previous };
    },
    onError: (_err, _vars, context) => {
      if (context?.previous) {
        queryClient.setQueryData(
          queryKeys.campaigns.purchases(campaignId),
          context.previous,
        );
      }
    },
    onSettled: () => {
      invalidatePurchaseRelatedQueries(queryClient, campaignId);
    },
  });
}

export function useDeletePurchase(campaignId: string) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (purchaseId: string) => api.deletePurchase(campaignId, purchaseId),
    onMutate: async (purchaseId) => {
      await queryClient.cancelQueries({
        queryKey: queryKeys.campaigns.purchases(campaignId),
      });
      const previous = queryClient.getQueryData(
        queryKeys.campaigns.purchases(campaignId),
      );
      queryClient.setQueryData(
        queryKeys.campaigns.purchases(campaignId),
        (old: Purchase[] = []) => old.filter(p => p.id !== purchaseId),
      );
      return { previous };
    },
    onError: (_err, _vars, context) => {
      if (context?.previous) {
        queryClient.setQueryData(
          queryKeys.campaigns.purchases(campaignId),
          context.previous,
        );
      }
    },
    onSettled: () => {
      invalidatePurchaseRelatedQueries(queryClient, campaignId);
    },
  });
}

export function useReassignPurchase(campaignId: string) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({ purchaseId, targetCampaignId }: { purchaseId: string; targetCampaignId: string }) =>
      api.reassignPurchase(purchaseId, targetCampaignId),
    onMutate: async ({ purchaseId }) => {
      await queryClient.cancelQueries({ queryKey: queryKeys.campaigns.purchases(campaignId) });
      const previous = queryClient.getQueryData(queryKeys.campaigns.purchases(campaignId));
      queryClient.setQueryData(
        queryKeys.campaigns.purchases(campaignId),
        (old: Purchase[] = []) => old.filter(p => p.id !== purchaseId),
      );
      return { previous };
    },
    onError: (_err, _vars, context) => {
      if (context?.previous) {
        queryClient.setQueryData(queryKeys.campaigns.purchases(campaignId), context.previous);
      }
    },
    onSettled: (_data, _err, vars) => {
      invalidatePurchaseRelatedQueries(queryClient, campaignId);
      invalidatePurchaseRelatedQueries(queryClient, vars.targetCampaignId);
      queryClient.invalidateQueries({ queryKey: queryKeys.portfolio.health });
      queryClient.invalidateQueries({ queryKey: queryKeys.portfolio.insights });
      queryClient.invalidateQueries({ queryKey: queryKeys.portfolio.suggestions });
    },
  });
}

export function useCreateSale(campaignId: string) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (data: CreateSaleInput) => api.createSale(campaignId, data),
    onMutate: async (newSale) => {
      await queryClient.cancelQueries({
        queryKey: queryKeys.campaigns.sales(campaignId),
      });
      const previous = queryClient.getQueryData(
        queryKeys.campaigns.sales(campaignId),
      );
      queryClient.setQueryData(
        queryKeys.campaigns.sales(campaignId),
        (old: Sale[] = []) => [
          ...old,
          {
            ...newSale,
            id: `optimistic-${Date.now()}`,
            createdAt: new Date().toISOString(),
          },
        ],
      );
      return { previous };
    },
    onError: (_err, _vars, context) => {
      if (context?.previous) {
        queryClient.setQueryData(
          queryKeys.campaigns.sales(campaignId),
          context.previous,
        );
      }
    },
    onSettled: () => {
      queryClient.invalidateQueries({ queryKey: queryKeys.campaigns.sales(campaignId) });
      invalidatePurchaseRelatedQueries(queryClient, campaignId);
    },
  });
}

export function useCreateBulkSales(campaignId: string) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (data: { saleChannel: string; saleDate: string; items: { purchaseId: string; salePriceCents: number }[] }) =>
      api.createBulkSales(campaignId, data.saleChannel, data.saleDate, data.items),
    onSettled: () => {
      queryClient.invalidateQueries({ queryKey: queryKeys.campaigns.sales(campaignId) });
      invalidatePurchaseRelatedQueries(queryClient, campaignId);
      queryClient.invalidateQueries({ queryKey: queryKeys.portfolio.health });
      queryClient.invalidateQueries({ queryKey: queryKeys.portfolio.insights });
    },
  });
}
