import { useMemo } from 'react';
import PokeballLoader from '../PokeballLoader';
import { SectionErrorBoundary } from '../ui';
import Button from '../ui/Button';
import CampaignTuningTable from '../components/insights/CampaignTuningTable';
import DoNowSection from '../components/insights/DoNowSection';
import HealthSignalsTiles from '../components/insights/HealthSignalsTiles';
import { deriveCampaignRecommendations } from '../components/insights/deriveCampaignRecommendations';
import { useInsightsOverview } from '../queries/useInsightsOverview';
import type { Action, Signals } from '../../types/insights';

function isSignalsClear(signals: Signals): boolean {
  return (
    signals.aiAcceptRate.resolved === 0 &&
    signals.liquidationRecoverableUsd === 0 &&
    signals.spikeProfitUsd === 0 &&
    signals.spikeCertCount === 0 &&
    signals.stuckInPipelineCount === 0
  );
}

function formatRefreshedAt(iso: string): string {
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return iso;
  return d.toLocaleString(undefined, {
    month: 'short',
    day: 'numeric',
    hour: 'numeric',
    minute: '2-digit',
  });
}

/** Combine advisor-supplied actions with derived-from-matrix recommendations.
    Real advisor actions lead because they're explicit + impact-tagged. Derived
    recommendations append, deduped by id namespace. The matrix-derivation
    fallback is what makes Insights advice-first instead of checkup-first when
    the advisor hasn't generated digests yet. */
function combineActions(advisor: Action[], derived: Action[]): Action[] {
  const advisorIds = new Set(advisor.map((a) => a.id));
  return [...advisor, ...derived.filter((a) => !advisorIds.has(a.id))];
}

export default function InsightsPage() {
  const { data, isLoading, isFetching, isError, refetch } = useInsightsOverview();

  const recommendations = useMemo(() => {
    if (!data) return [];
    return combineActions(data.actions, deriveCampaignRecommendations(data.campaigns));
  }, [data]);

  return (
    <div className="max-w-6xl mx-auto px-4 space-y-6">
      <header className="flex items-baseline justify-between gap-4">
        <div>
          <h1 className="page-title">Insights</h1>
          <p className="text-sm text-[var(--text-muted)] mt-1">
            What to act on, signals unique to this page, and per-campaign tuning.
          </p>
          {data?.generatedAt && (
            <p className="text-xs text-[var(--text-muted)] tabular-nums mt-1">
              Last refreshed {formatRefreshedAt(data.generatedAt)}
            </p>
          )}
        </div>
        <Button
          variant="ghost"
          size="sm"
          disabled={isFetching}
          onClick={() => { void refetch(); }}
        >
          {isFetching && data ? 'Refreshing…' : 'Refresh'}
        </Button>
      </header>

      {isLoading && (
        <div className="flex items-center justify-center min-h-[40vh]">
          <PokeballLoader />
        </div>
      )}

      {isError && !isLoading && (
        <div className="px-3 py-2 bg-[var(--danger)]/10 border border-[var(--danger)]/20 rounded-lg text-sm text-[var(--danger)]">
          Failed to load insights.{' '}
          <button className="underline" onClick={() => { void refetch(); }}>
            Retry
          </button>
        </div>
      )}

      {data && (() => {
        const hasRecommendations = recommendations.length > 0;
        const hasSignals = !isSignalsClear(data.signals);
        const allCampaignsOK = data.campaigns.every(c => c.status === 'OK');
        const fullyHealthy = !hasRecommendations && !hasSignals && allCampaignsOK;
        return (
          <>
            {fullyHealthy && (
              <div className="rounded-xl border border-[var(--surface-2)] bg-[var(--surface-1)] px-4 py-3 flex items-center gap-2 text-sm">
                <span className="text-[var(--success)]" aria-hidden="true">●</span>
                <span className="text-[var(--text)]">All campaigns healthy</span>
                <span className="text-[var(--text-muted)]" aria-hidden="true">·</span>
                <span className="text-[var(--text-muted)]">no actions or signals right now</span>
              </div>
            )}
            {hasRecommendations && (
              <SectionErrorBoundary sectionName="Do now">
                <DoNowSection actions={recommendations} />
              </SectionErrorBoundary>
            )}
            {hasSignals && (
              <SectionErrorBoundary sectionName="Health signals">
                <HealthSignalsTiles signals={data.signals} />
              </SectionErrorBoundary>
            )}
            <SectionErrorBoundary sectionName="Campaign tuning">
              {/* When recommendations are leading the page, the matrix is the
                  reference table beneath — collapsed by default to keep the
                  page advice-first. When there are no recommendations, the
                  matrix is the page's primary surface and stays open. */}
              <details className="group" open={!hasRecommendations}>
                <summary className="cursor-pointer list-none flex items-center justify-between gap-2 py-2 text-sm text-[var(--text-muted)] hover:text-[var(--text)] transition-colors">
                  <span className="text-[10px] font-semibold uppercase tracking-[0.14em] text-[var(--brand-400)]">
                    All campaigns ({data.campaigns.length})
                  </span>
                  <span
                    aria-hidden="true"
                    className="text-xs transition-transform group-open:rotate-90"
                  >
                    ›
                  </span>
                </summary>
                <div className="mt-2">
                  <CampaignTuningTable rows={data.campaigns} />
                </div>
              </details>
            </SectionErrorBoundary>
          </>
        );
      })()}
    </div>
  );
}
