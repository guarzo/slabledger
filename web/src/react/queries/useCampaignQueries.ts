import { useQuery, useMutation, useQueryClient, type QueryClient } from '@tanstack/react-query';
import { api } from '../../js/api';
import type { Campaign, CreateCampaignInput, CreatePurchaseInput, CreateSaleInput, Purchase, Sale, Invoice } from '../../types/campaigns';
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

export const useCampaign = createParamQuery(
  queryKeys.campaigns.detail, (id) => api.getCampaign(id), { staleTime: CAMPAIGN_STALE_TIME },
);

export const usePurchases = createParamQuery(
  queryKeys.campaigns.purchases, (id) => api.listPurchases(id), { staleTime: CAMPAIGN_STALE_TIME },
);

export const useSales = createParamQuery(
  queryKeys.campaigns.sales, (id) => api.listSales(id),
);

export const useCampaignPNL = createParamQuery(
  queryKeys.campaigns.pnl, (id) => api.getCampaignPNL(id), { staleTime: ANALYTICS_STALE_TIME },
);

export const useChannelPNL = createParamQuery(
  queryKeys.campaigns.channelPnl, (id) => api.getPNLByChannel(id),
);

export function useFillRate(campaignId: string, days: number = 30) {
  return useQuery({
    queryKey: queryKeys.campaigns.fillRate(campaignId, days),
    queryFn: () => api.getFillRate(campaignId, days),
    enabled: !!campaignId,
  });
}

export const useDaysToSell = createParamQuery(
  queryKeys.campaigns.daysToSell, (id) => api.getDaysToSell(id),
);

export function useInventory(campaignId: string, opts?: { enabled?: boolean }) {
  const query = useQuery({
    queryKey: queryKeys.campaigns.inventory(campaignId),
    queryFn: () => api.getInventory(campaignId),
    enabled: !!campaignId && (opts?.enabled ?? true),
    staleTime: ANALYTICS_STALE_TIME,
  });
  return { ...query, data: query.data?.items, warnings: query.data?.warnings };
}

export const useTuning = createParamQuery(
  queryKeys.campaigns.tuning, (id) => api.getCampaignTuning(id), { staleTime: ANALYTICS_STALE_TIME },
);

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

export const usePortfolioChannelVelocity = createStaticQuery(
  queryKeys.portfolio.channelVelocity, () => api.getPortfolioChannelVelocity(),
);

export const usePortfolioInsights = createStaticQuery(
  queryKeys.portfolio.insights, () => api.getPortfolioInsights(), { staleTime: ANALYTICS_STALE_TIME },
);

export const useCapitalTimeline = createStaticQuery(
  queryKeys.portfolio.capitalTimeline, () => api.getCapitalTimeline(),
);

export const useWeeklyReview = createStaticQuery(
  queryKeys.portfolio.weeklyReview, () => api.getWeeklyReview(),
);

export const useGlobalSellSheet = createStaticQuery(
  queryKeys.portfolio.sellSheet, () => api.generateGlobalSellSheet(),
);

export function useGlobalInventory() {
  const query = useQuery({
    queryKey: queryKeys.portfolio.globalInventory,
    queryFn: () => api.getGlobalInventory(),
    staleTime: ANALYTICS_STALE_TIME,
  });
  return { ...query, data: query.data?.items, warnings: query.data?.warnings };
}

export const useCrackCandidates = createParamQuery(
  queryKeys.campaigns.crackCandidates, (id) => api.getCrackCandidates(id),
);

export const useExpectedValues = createParamQuery(
  queryKeys.campaigns.expectedValues, (id) => api.getExpectedValues(id),
);

export const useActivationChecklist = createParamQuery(
  queryKeys.campaigns.activationChecklist, (id) => api.getActivationChecklist(id),
);

export const useProjections = createParamQuery(
  queryKeys.campaigns.projections, (id) => api.getProjections(id),
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

export function usePSAPendingItems() {
  return useQuery({
    queryKey: queryKeys.purchases.psaPendingItems,
    queryFn: () => api.listPSAPendingItems(),
    staleTime: 30_000,
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
