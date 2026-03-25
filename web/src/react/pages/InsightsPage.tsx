import PokeballLoader from '../PokeballLoader';
import { useWeeklyReview, useCapitalTimeline, useCreditSummary } from '../queries/useCampaignQueries';
import WeeklyReviewSection from '../components/portfolio/WeeklyReviewSection';
import CapitalTimelineChart from '../components/portfolio/CapitalTimelineChart';
import CreditHealthPanel from '../components/portfolio/CreditHealthPanel';
import InsightsSection from '../components/insights/InsightsSection';
import { SectionErrorBoundary } from '../ui';

export default function InsightsPage() {
  const { data: weeklyReview, isLoading: weeklyLoading } = useWeeklyReview();
  const { data: capitalTimeline } = useCapitalTimeline();
  const { data: creditData } = useCreditSummary();

  const hasCapitalTimeline = capitalTimeline && capitalTimeline.dataPoints?.length > 0;

  if (weeklyLoading) {
    return (
      <div className="flex items-center justify-center min-h-[50vh]">
        <PokeballLoader />
      </div>
    );
  }

  return (
    <div className="max-w-6xl mx-auto px-4">
      <div className="mb-6">
        <h1 className="text-2xl font-bold text-gradient text-gradient-premium">Insights</h1>
        <p className="mt-1 text-sm text-[var(--text-muted)]">Portfolio analytics and performance</p>
      </div>

      <div className="space-y-6">
        {weeklyReview && (
          <SectionErrorBoundary sectionName="Weekly Review">
            <WeeklyReviewSection data={weeklyReview} />
          </SectionErrorBoundary>
        )}

        {hasCapitalTimeline && (
          <SectionErrorBoundary sectionName="Capital Timeline">
            <CapitalTimelineChart data={capitalTimeline} />
          </SectionErrorBoundary>
        )}

        <SectionErrorBoundary sectionName="Credit Health">
          <CreditHealthPanel credit={creditData} />
        </SectionErrorBoundary>

        <SectionErrorBoundary sectionName="Portfolio Insights">
          <InsightsSection />
        </SectionErrorBoundary>
      </div>
    </div>
  );
}
