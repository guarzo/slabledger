import PokeballLoader from '../PokeballLoader';
import { usePortfolioHealth, useWeeklyReview, useCapitalSummary, useCapitalTimeline } from '../queries/useCampaignQueries';
import HeroStatsBar from '../components/portfolio/HeroStatsBar';
import CapitalTimelineChart from '../components/portfolio/CapitalTimelineChart';
import CapitalExposurePanel from '../components/portfolio/CapitalExposurePanel';
import WeeklyReviewSection from '../components/portfolio/WeeklyReviewSection';
import WatchlistSection from '../components/watchlist/WatchlistSection';
import AIAnalysisWidget from '../components/advisor/AIAnalysisWidget';
import PicksList from '../components/picks/PicksList';
import AcquisitionWatchlist from '../components/picks/AcquisitionWatchlist';
import { SectionErrorBoundary } from '../ui';

export default function DashboardPage() {
  const { data: healthData, isLoading: healthLoading } = usePortfolioHealth();
  const { data: weeklyReview } = useWeeklyReview();
  const { data: capitalData } = useCapitalSummary();
  const { data: capitalTimeline } = useCapitalTimeline();

  if (healthLoading) {
    return (
      <div className="flex items-center justify-center min-h-[50vh]">
        <PokeballLoader />
      </div>
    );
  }

  return (
    <div className="max-w-6xl mx-auto px-4">
      {/* Header */}
      <div className="mb-6">
        <h1 className="text-[22px] font-bold text-[var(--text)] tracking-tight">Dashboard</h1>
      </div>

      {/* Tier 1: Hero Stats Bar */}
      <HeroStatsBar health={healthData} capital={capitalData} />

      {/* Capital Exposure + Timeline */}
      {(capitalData || capitalTimeline) && (
        <div className="mb-6">
          <SectionErrorBoundary sectionName="Capital Exposure">
            <div className="grid grid-cols-1 lg:grid-cols-2 gap-4">
              <CapitalExposurePanel capital={capitalData} />
              {capitalTimeline && <CapitalTimelineChart data={capitalTimeline} />}
            </div>
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

      {/* AI Weekly Intelligence */}
      <div className="mb-6">
        <SectionErrorBoundary sectionName="AI Advisor">
          <AIAnalysisWidget
            endpoint="digest"
            cacheType="digest"
            title="Weekly Intelligence"
            buttonLabel="Generate Digest"
            description="Get an AI-powered weekly review with performance insights, capital exposure assessment, and prioritized action items."
            collapsible
          />
        </SectionErrorBoundary>
      </div>

      {/* Opportunities */}
      <div className="mb-6">
        <SectionErrorBoundary sectionName="Opportunities">
          <div className="space-y-4">
            <PicksList />
            <AcquisitionWatchlist />
          </div>
        </SectionErrorBoundary>
      </div>

      {/* Watchlist */}
      <div className="mb-6">
        <WatchlistSection maxItems={8} />
      </div>
    </div>
  );
}
