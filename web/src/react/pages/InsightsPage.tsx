import { useCallback, useMemo, useState } from 'react';
import SectionedReport, { type SectionSchema } from '../components/advisor/SectionedReport';
import { useAdvisorCache } from '../hooks/useAdvisorCache';
import { useAdvisorStream } from '../hooks/useAdvisorStream';
import { useCampaigns } from '../queries/useCampaignQueries';
import { Button } from '../ui';
import type { AdvisorAnalysisType } from '../../types/apiStatus';

const DIGEST_SCHEMA: SectionSchema[] = [
  { heading: 'Executive Summary', icon: '📌' },
  { heading: 'Top Actions', icon: '🎯' },
  { heading: 'Portfolio Performance', icon: '📊' },
  { heading: 'Capital & Cashflow', icon: '💰' },
  { heading: 'Portfolio Health', icon: '🩺' },
  { heading: 'Watchlist & Alerts', icon: '👀' },
];

const LIQUIDATION_SCHEMA: SectionSchema[] = [
  { heading: 'Aggressive Markdowns', icon: '📉' },
  { heading: 'Auction Candidates', icon: '🔨' },
  { heading: 'Hold / Wait', icon: '⏸️' },
  { heading: 'Dead Weight', icon: '⚓' },
  { heading: 'Totals', icon: '🧮' },
];

const CAMPAIGN_SCHEMA: SectionSchema[] = [
  { heading: 'Performance Snapshot', icon: '📊' },
  { heading: "What's Working", icon: '✅' },
  { heading: "What's Not", icon: '⚠️' },
  { heading: 'Tuning Recommendations', icon: '🎛️' },
  { heading: 'Inventory Position', icon: '📦' },
];

export default function InsightsPage() {
  return (
    <div className="max-w-6xl mx-auto px-4 space-y-6">
      <header className="flex items-baseline justify-between gap-4">
        <div>
          <h1 className="text-2xl font-bold text-[var(--text)]">Insights</h1>
          <p className="text-sm text-[var(--text-muted)] mt-1">
            AI-authored analyses of your portfolio, inventory, and individual campaigns.
          </p>
        </div>
      </header>

      <CachedReportPanel
        cacheType="digest"
        title="Weekly Digest"
        description="A weekly read on performance, cash flow, and where to focus next."
        schema={DIGEST_SCHEMA}
      />

      <CachedReportPanel
        cacheType="liquidation"
        title="Liquidation Analysis"
        description="What to mark down, auction, hold, or walk away from — with capital-freed totals."
        schema={LIQUIDATION_SCHEMA}
      />

      <CampaignReportPanel />
    </div>
  );
}

function PanelShell({
  title,
  description,
  updatedLabel,
  right,
  children,
}: {
  title: string;
  description: string;
  updatedLabel?: string | null;
  right?: React.ReactNode;
  children: React.ReactNode;
}) {
  return (
    <section className="space-y-3">
      <div className="flex items-center justify-between gap-4">
        <div>
          <h2 className="text-lg font-semibold text-[var(--text)]">{title}</h2>
          <p className="text-xs text-[var(--text-muted)] mt-0.5">{description}</p>
        </div>
        <div className="flex items-center gap-2">
          {updatedLabel && (
            <span className="text-[10px] text-[var(--text-muted)] bg-[var(--surface-2)] px-1.5 py-0.5 rounded">
              {updatedLabel}
            </span>
          )}
          {right}
        </div>
      </div>
      {children}
    </section>
  );
}

function CachedReportPanel({
  cacheType,
  title,
  description,
  schema,
}: {
  cacheType: AdvisorAnalysisType;
  title: string;
  description: string;
  schema: SectionSchema[];
}) {
  const { data, isLoading, refresh } = useAdvisorCache(cacheType);
  const [refreshError, setRefreshError] = useState<string | null>(null);

  const status = data?.status ?? 'empty';
  const isRunning = status === 'running';
  const hasContent = status === 'complete' && !!data?.content;
  const hasError = status === 'error';

  const handleRefresh = useCallback(async () => {
    setRefreshError(null);
    try {
      await refresh();
    } catch {
      setRefreshError('Failed to trigger refresh. Please try again.');
    }
  }, [refresh]);

  return (
    <PanelShell
      title={title}
      description={description}
      updatedLabel={data?.updatedAt ? formatAge(data.updatedAt) : null}
      right={
        <Button
          variant="primary"
          size="sm"
          onClick={() => { void handleRefresh(); }}
          disabled={isRunning}
          loading={isRunning || isLoading}
        >
          {hasContent ? 'Refresh' : 'Generate'}
        </Button>
      }
    >
      {isRunning && <RunningBanner label="Generating analysis" />}
      {(hasError || refreshError) && (
        <ErrorBanner>{refreshError ?? `Analysis failed: ${data?.errorMessage ?? 'Unknown error'}`}</ErrorBanner>
      )}
      {hasContent ? (
        <SectionedReport markdown={data!.content!} schema={schema} cacheKey={`insights:${cacheType}`} />
      ) : !isRunning && !hasError && !refreshError ? (
        <p className="text-sm text-[var(--text-muted)] bg-[var(--surface-1)] border border-[var(--surface-2)] rounded-xl p-4">
          No report yet. Click <strong>Generate</strong> to produce one.
        </p>
      ) : null}
    </PanelShell>
  );
}

function CampaignReportPanel() {
  const { data: campaigns = [] } = useCampaigns(false);
  const [selectedId, setSelectedId] = useState<string>('');
  const selectedCampaign = useMemo(
    () => campaigns.find(c => c.id === selectedId) ?? null,
    [campaigns, selectedId],
  );
  const { content, isStreaming, error, toolStatus, run, reset } = useAdvisorStream();

  const handleGenerate = useCallback(() => {
    if (!selectedId) return;
    reset();
    void run('campaign-analysis', { campaignId: selectedId });
  }, [selectedId, run, reset]);

  return (
    <PanelShell
      title="Campaign Analysis"
      description="Pick a campaign to analyze — health, tuning, and inventory position."
      right={
        <Button
          variant="primary"
          size="sm"
          onClick={handleGenerate}
          disabled={!selectedId || isStreaming}
          loading={isStreaming}
        >
          {content ? 'Regenerate' : 'Analyze'}
        </Button>
      }
    >
      <div className="flex items-center gap-2 bg-[var(--surface-1)] border border-[var(--surface-2)] rounded-xl p-3">
        <label className="text-xs font-semibold uppercase tracking-wider text-[var(--text-muted)]">
          Campaign
        </label>
        <select
          value={selectedId}
          onChange={e => {
            setSelectedId(e.target.value);
            reset();
          }}
          className="flex-1 bg-[var(--surface-2)] border border-[var(--surface-2)] rounded-md px-3 py-1.5 text-sm text-[var(--text)]"
        >
          <option value="">Select a campaign…</option>
          {campaigns.map(c => (
            <option key={c.id} value={c.id}>
              {c.name}
            </option>
          ))}
        </select>
        {selectedCampaign && (
          <span className="text-xs text-[var(--text-muted)]">
            {selectedCampaign.phase}
          </span>
        )}
      </div>

      {toolStatus && <RunningBanner label={toolStatus} />}
      {error && <ErrorBanner>{error}</ErrorBanner>}

      {content ? (
        <SectionedReport
          markdown={content}
          schema={CAMPAIGN_SCHEMA}
          cacheKey={`insights:campaign:${selectedId}`}
        />
      ) : !isStreaming && !error && selectedId ? (
        <p className="text-sm text-[var(--text-muted)] bg-[var(--surface-1)] border border-[var(--surface-2)] rounded-xl p-4">
          Click <strong>Analyze</strong> to generate the report for {selectedCampaign?.name ?? 'this campaign'}.
        </p>
      ) : null}
    </PanelShell>
  );
}

function RunningBanner({ label }: { label: string }) {
  return (
    <div className="flex items-center gap-2 px-3 py-2 bg-[var(--surface-2)]/50 rounded-lg text-xs text-[var(--text-muted)]">
      <span className="inline-block w-2 h-2 bg-[var(--brand-500)] rounded-full animate-pulse" />
      {label}…
    </div>
  );
}

function ErrorBanner({ children }: { children: React.ReactNode }) {
  return (
    <div className="px-3 py-2 bg-[var(--danger)]/10 border border-[var(--danger)]/20 rounded-lg text-xs text-[var(--danger)]">
      {children}
    </div>
  );
}

function formatAge(updatedAt: string): string {
  const t = new Date(updatedAt).getTime();
  if (!Number.isFinite(t)) return 'Updated unknown';
  const ms = Math.max(0, Date.now() - t);
  const mins = Math.floor(ms / 60_000);
  if (mins < 1) return 'Updated just now';
  if (mins < 60) return `Updated ${mins}m ago`;
  const hours = Math.floor(mins / 60);
  if (hours < 24) return `Updated ${hours}h ago`;
  return `Updated ${Math.floor(hours / 24)}d ago`;
}
