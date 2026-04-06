import PokeballLoader from '../PokeballLoader';
import { usePortfolioHealth, useWeeklyReview, useCreditSummary } from '../queries/useCampaignQueries';
import HeroStatsBar from '../components/portfolio/HeroStatsBar';
import WeeklyReviewSection from '../components/portfolio/WeeklyReviewSection';
import WatchlistSection from '../components/watchlist/WatchlistSection';
import AIAnalysisWidget from '../components/advisor/AIAnalysisWidget';
import { SectionErrorBoundary } from '../ui';

export default function DashboardPage() {
  const { data: healthData, isLoading: healthLoading } = usePortfolioHealth();
  const { data: weeklyReview } = useWeeklyReview();
  const { data: creditData } = useCreditSummary();

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
      <HeroStatsBar health={healthData} credit={creditData} />

      {/* Weekly Review */}
      <div className="mb-6">
        <SectionErrorBoundary sectionName="Weekly Review">
          {weeklyReview && <WeeklyReviewSection data={weeklyReview} />}
        </SectionErrorBoundary>
      </div>

      {/* AI Weekly Intelligence */}
      <div className="mb-6">
        <SectionErrorBoundary sectionName="AI Advisor">
          <AIAnalysisWidget
            endpoint="digest"
            cacheType="digest"
            title="Weekly Intelligence"
            buttonLabel="Generate Digest"
            description="Get an AI-powered weekly review with performance insights, credit health assessment, and prioritized action items."
            collapsible
          />
        </SectionErrorBoundary>
      </div>

      {/* Watchlist */}
      <div className="mb-6">
        <WatchlistSection maxItems={4} />
      </div>
    </div>
  );
}
