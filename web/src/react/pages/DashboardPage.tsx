import { useMemo } from 'react';
import { Link } from 'react-router-dom';
import PokeballLoader from '../PokeballLoader';
import { formatCents } from '../utils/formatters';
import { useCampaigns, usePortfolioHealth, useWeeklyReview, useCreditSummary } from '../queries/useCampaignQueries';
import HeroStatsBar from '../components/portfolio/HeroStatsBar';
import WeeklyReviewSection from '../components/portfolio/WeeklyReviewSection';
import WatchlistSection from '../components/watchlist/WatchlistSection';
import AIAnalysisWidget from '../components/advisor/AIAnalysisWidget';
import { SectionErrorBoundary } from '../ui';

export default function DashboardPage() {
  const { data: allCampaigns = [], isLoading: campaignsLoading } = useCampaigns(false);
  const { data: healthData } = usePortfolioHealth();
  const { data: weeklyReview } = useWeeklyReview();
  const { data: creditData } = useCreditSummary();

  const activeCampaigns = useMemo(
    () => allCampaigns.filter(c => c.phase === 'active'),
    [allCampaigns]
  );

  const healthMap = useMemo(() => {
    const map: Record<string, string> = {};
    healthData?.campaigns?.forEach(ch => { map[ch.campaignId] = ch.healthStatus; });
    return map;
  }, [healthData]);

  if (campaignsLoading) {
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

      {/* Tier 2: Campaigns + Weekly Review */}
      <div className="mb-6">
        <div className="space-y-6">
          {/* Active Campaigns */}
          {activeCampaigns.length > 0 && (
            <div>
              <h2 className="text-sm font-semibold text-[var(--text-muted)] uppercase tracking-wider mb-3">Active Campaigns</h2>
              <div className="grid gap-3 grid-cols-1 sm:grid-cols-2 lg:grid-cols-3">
                {activeCampaigns.map(c => {
                  const health = healthMap[c.id];
                  const healthColor = health === 'critical' ? 'bg-[var(--danger)]' : health === 'warning' ? 'bg-[var(--warning)]' : 'bg-[var(--success)]';
                  return (
                    <Link
                      key={c.id}
                      to={`/campaigns/${c.id}`}
                      className="p-4 bg-[var(--surface-1)] rounded-xl border border-[var(--surface-2)] hover:border-[var(--brand-500)]/50 hover:-translate-y-0.5 hover:shadow-[var(--shadow-2)] transition-all"
                    >
                      <div className="flex items-center gap-2 mb-1">
                        <h3 className="text-sm font-semibold text-[var(--text)] truncate">{c.name}</h3>
                        {health && (
                          <span className={`inline-block w-2 h-2 rounded-full shrink-0 ${healthColor}`} title={`Health: ${health}`} />
                        )}
                      </div>
                      <div className="flex gap-3 text-xs text-[var(--text-muted)]">
                        <span>{c.sport}</span>
                        {c.gradeRange && <span>PSA {c.gradeRange}</span>}
                        <span>Cap: {formatCents(c.dailySpendCapCents)}/d</span>
                      </div>
                    </Link>
                  );
                })}
              </div>
            </div>
          )}

          {/* Weekly Activity */}
          <SectionErrorBoundary sectionName="Weekly Review">
            {weeklyReview && <WeeklyReviewSection data={weeklyReview} />}
          </SectionErrorBoundary>
        </div>
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
