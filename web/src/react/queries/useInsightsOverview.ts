import { api } from '../../js/api';
import type { InsightsOverview } from '../../types/insights';
import { createStaticQuery } from './createQuery';

const insightsOverviewKey = ['insights', 'overview'] as const;

export const useInsightsOverview = createStaticQuery<InsightsOverview>(
  insightsOverviewKey,
  () => api.get<InsightsOverview>('/insights/overview'),
  { staleTime: 60_000 },
);
