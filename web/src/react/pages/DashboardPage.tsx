import { useMemo } from 'react';
import PokeballLoader from '../PokeballLoader';
import {
  usePortfolioHealth,
  useWeeklyReview,
  useCapitalSummary,
  useGlobalInventory,
} from '../queries/useCampaignQueries';
import HeroStatsBar from '../components/portfolio/HeroStatsBar';
import NextMovesPanel from '../components/portfolio/NextMovesPanel';
import InvoiceReadinessPanel from '../components/portfolio/InvoiceReadinessPanel';
import WeeklyReviewSection from '../components/portfolio/WeeklyReviewSection';
import TopPerformersSection from '../components/portfolio/TopPerformersSection';
import { computeInventoryMeta } from './campaign-detail/inventory/inventoryCalcs';
import { SectionErrorBoundary } from '../ui';

export default function DashboardPage() {
  const { data: healthData, isLoading: healthLoading } = usePortfolioHealth();
  const { data: weeklyReview } = useWeeklyReview();
  const { data: capitalData } = useCapitalSummary();
  const { data: inventoryItems } = useGlobalInventory();

  const inventoryCounts = useMemo(() => {
    if (!inventoryItems || inventoryItems.length === 0) {
      return { needsAttention: 0, pendingListings: 0 };
    }
    const { tabCounts } = computeInventoryMeta(inventoryItems);
    return {
      needsAttention: tabCounts.needs_attention,
      pendingListings: tabCounts.ready_to_list,
    };
  }, [inventoryItems]);

  if (healthLoading) {
    return (
      <div className="flex items-center justify-center min-h-[50vh]">
        <PokeballLoader />
      </div>
    );
  }

  return (
    <div className="max-w-[1600px] mx-auto px-4">
      {/* Header */}
      <div className="mb-6">
        <h1 className="page-title">Dashboard</h1>
      </div>

      {/* Tier 1: Hero Stats Bar */}
      <HeroStatsBar
        health={healthData}
        capital={capitalData}
        needsAttentionCount={inventoryCounts.needsAttention}
        pendingListingsCount={inventoryCounts.pendingListings}
        hideInvoiceChip
      />

      {/* Next Moves — what to act on today */}
      <SectionErrorBoundary sectionName="Next Moves">
        <NextMovesPanel />
      </SectionErrorBoundary>

      {/* Invoice Readiness */}
      {capitalData && (
        <div className="mb-6">
          <SectionErrorBoundary sectionName="Invoice Readiness">
            <InvoiceReadinessPanel capital={capitalData} />
          </SectionErrorBoundary>
        </div>
      )}

      {/* Weekly Review */}
      {weeklyReview && (
        <div className="mb-6">
          <SectionErrorBoundary sectionName="Weekly Review">
            <WeeklyReviewSection data={weeklyReview} />
          </SectionErrorBoundary>
        </div>
      )}

      {/* Top Performers — campaign-level */}
      {healthData && healthData.campaigns.length > 0 && (
        <div className="mb-6">
          <SectionErrorBoundary sectionName="Top Performers">
            <TopPerformersSection campaigns={healthData.campaigns} />
          </SectionErrorBoundary>
        </div>
      )}
    </div>
  );
}
