import { Link } from 'react-router-dom';
import PokeballLoader from '../PokeballLoader';
import { usePortfolioHealth, useWeeklyReview, useCapitalSummary } from '../queries/useCampaignQueries';
import HeroStatsBar from '../components/portfolio/HeroStatsBar';
import InvoiceReadinessPanel from '../components/portfolio/InvoiceReadinessPanel';
import WeeklyReviewSection from '../components/portfolio/WeeklyReviewSection';
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

      {/* Insights hub link — replaces the inline AI widget */}
      <Link
        to="/insights"
        className="mb-6 flex items-center justify-between gap-4 p-4 rounded-xl border border-[var(--surface-2)] bg-[var(--surface-1)] hover:border-[var(--brand-500)]/40 hover:bg-[var(--surface-2)]/30 transition-colors"
      >
        <div className="flex items-center gap-3">
          <span className="text-xl" aria-hidden="true">&#x2728;</span>
          <div>
            <div className="text-sm font-semibold text-[var(--text)]">Weekly digest and liquidation plan live on Insights</div>
            <div className="text-xs text-[var(--text-muted)]">Structured AI reports with section-by-section breakdown.</div>
          </div>
        </div>
        <span className="text-sm text-[var(--brand-400)] font-medium">Open Insights &rarr;</span>
      </Link>

      </div>
   );
}
