import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { api } from '../../js/api';
import { queryKeys } from './queryKeys';
import type { PostMetricsSnapshot, MetricsSummary } from '../../types/social';

const SOCIAL_STALE_TIME = 30_000;

export function useSocialPosts() {
  return useQuery({
    queryKey: queryKeys.social.list(),
    queryFn: () => api.getSocialPosts(),
    staleTime: SOCIAL_STALE_TIME,
    refetchInterval: (query) => {
      const hasPublishing = query.state.data?.some(p => p.status === 'publishing');
      return hasPublishing ? 3000 : false;
    },
  });
}

export function useSocialPost(id: string) {
  return useQuery({
    queryKey: queryKeys.social.detail(id),
    queryFn: () => api.getSocialPost(id),
    enabled: !!id,
    staleTime: SOCIAL_STALE_TIME,
  });
}

export function useGenerateSocialPosts() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: () => api.generateSocialPosts(),
    onSuccess: () => {
      // Generation is now async (202) — poll for new posts over the next 2 minutes
      const intervals = [3000, 5000, 10000, 15000, 30000, 60000];
      intervals.forEach(delay => {
        setTimeout(() => {
          queryClient.invalidateQueries({ queryKey: queryKeys.social.all });
        }, delay);
      });
    },
  });
}

export function useUpdateSocialCaption() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({ id, caption, hashtags }: { id: string; caption: string; hashtags: string }) =>
      api.updateSocialCaption(id, caption, hashtags),
    onSuccess: (_data, variables) => {
      queryClient.invalidateQueries({ queryKey: queryKeys.social.detail(variables.id) });
      queryClient.invalidateQueries({ queryKey: queryKeys.social.all });
    },
  });
}

export function useDeleteSocialPost() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => api.deleteSocialPost(id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: queryKeys.social.all });
    },
  });
}

export function usePublishSocialPost() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => api.publishSocialPost(id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: queryKeys.social.all });
    },
  });
}

export function useUploadSlides() {
  return useMutation({
    mutationFn: ({ id, slides }: { id: string; slides: Blob[] }) =>
      api.uploadSlides(id, slides),
  });
}

export function useInstagramStatus(enabled = true) {
  return useQuery({
    queryKey: queryKeys.instagram.status(),
    queryFn: () => api.getInstagramStatus(),
    staleTime: 60_000,
    enabled,
  });
}

export function useConnectInstagram() {
  return useMutation({
    mutationFn: () => api.connectInstagram(),
  });
}

export function useDisconnectInstagram() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: () => api.disconnectInstagram(),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: queryKeys.instagram.all });
    },
  });
}

export function usePostMetrics(postId: string | undefined) {
  return useQuery<PostMetricsSnapshot[]>({
    queryKey: [...queryKeys.social.all, 'metrics', postId],
    queryFn: () => api.getPostMetrics(postId!),
    enabled: !!postId,
    staleTime: 5 * 60 * 1000, // 5 minutes
  });
}

export function useMetricsSummary() {
  return useQuery<MetricsSummary[]>({
    queryKey: [...queryKeys.social.all, 'metrics-summary'],
    queryFn: () => api.getMetricsSummary(),
    staleTime: 5 * 60 * 1000,
  });
}
