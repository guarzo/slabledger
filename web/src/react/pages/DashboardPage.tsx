import PokeballLoader from '../PokeballLoader';
import { usePortfolioHealth, useWeeklyReview, useCapitalSummary } from '../queries/useCampaignQueries';
import HeroStatsBar from '../components/portfolio/HeroStatsBar';
import InvoiceReadinessPanel from '../components/portfolio/InvoiceReadinessPanel';
import WeeklyReviewSection from '../components/portfolio/WeeklyReviewSection';
import AIAnalysisWidget from '../components/advisor/AIAnalysisWidget';
import PicksList from '../components/picks/PicksList';
import AcquisitionWatchlist from '../components/picks/AcquisitionWatchlist';
import { SectionErrorBoundary } from '../ui';

export default function DashboardPage() {
  const { data: healthData, isLoading: healthLoading } = usePortfolioHealth();
  const { data: weeklyReview } = useWeeklyReview();
  const { data: capitalData } = useCapitalSummary();

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
     </div>
  );
}
