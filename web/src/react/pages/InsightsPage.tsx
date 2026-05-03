import PokeballLoader from '../PokeballLoader';
import { SectionErrorBoundary } from '../ui';
import Button from '../ui/Button';
import CampaignTuningTable from '../components/insights/CampaignTuningTable';
import DoNowSection from '../components/insights/DoNowSection';
import HealthSignalsTiles from '../components/insights/HealthSignalsTiles';
import { useInsightsOverview } from '../queries/useInsightsOverview';
import type { Signals } from '../../types/insights';

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

export default function InsightsPage() {
  const { data, isLoading, isFetching, isError, refetch } = useInsightsOverview();

  return (
    <div className="max-w-6xl mx-auto px-4 space-y-6">
      <header className="flex items-baseline justify-between gap-4">
        <div>
          <h1 className="text-[22px] font-bold text-[var(--text)] tracking-tight">Insights</h1>
          <p className="text-sm text-[var(--text-muted)] mt-1">
            Actions you can take, signals unique to this page, and per-campaign tuning.
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
        const hasActionsOrSignals = data.actions.length > 0 || !isSignalsClear(data.signals);
        const allCampaignsOK = data.campaigns.every(c => c.status === 'OK');
        const fullyHealthy = !hasActionsOrSignals && allCampaignsOK;
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
            {hasActionsOrSignals && (
              <>
                <SectionErrorBoundary sectionName="Do now">
                  <DoNowSection actions={data.actions} />
                </SectionErrorBoundary>
                <SectionErrorBoundary sectionName="Health signals">
                  <HealthSignalsTiles signals={data.signals} />
                </SectionErrorBoundary>
              </>
            )}
            <SectionErrorBoundary sectionName="Campaign tuning">
              <CampaignTuningTable rows={data.campaigns} />
            </SectionErrorBoundary>
          </>
        );
      })()}
    </div>
  );
}
