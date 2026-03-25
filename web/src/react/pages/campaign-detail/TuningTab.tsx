import { useState } from 'react';
import type {
  Campaign, TuningResponse, GradePerformance, PriceTierPerformance,
  CardPerformance, TuningRecommendation, ThresholdBucket,
  MonteCarloResult,
} from '../../../types/campaigns';
import PokeballLoader from '../../PokeballLoader';
import { formatCents, formatPct } from '../../utils/formatters';
import { useToast } from '../../contexts/ToastContext';
import { useMediaQuery } from '../../hooks/useMediaQuery';
import { EmptyState, Button, ConfidenceIndicator } from '../../ui';
import { useTuning, useProjections } from '../../queries/useCampaignQueries';
import AIAnalysisWidget from '../../components/advisor/AIAnalysisWidget';

/** Map a sample count to a 0-1 confidence score for the ConfidenceIndicator. */
function sampleConfidence(n: number): number {
  if (n >= 50) return 1.0;
  if (n >= 20) return 0.7;
  if (n >= 5) return 0.4;
  return 0.15;
}

interface TuningTabProps {
  campaignId: string;
  campaign: Campaign;
  onUpdate: (c: Campaign) => void | Promise<void>;
}

export default function TuningTab({ campaignId, campaign, onUpdate }: TuningTabProps) {
  const { data: tuning, isLoading: loading } = useTuning(campaignId);
  const [tierView, setTierView] = useState<'fixed' | 'relative'>('fixed');
  const toast = useToast();
  const isMobile = useMediaQuery('(max-width: 768px)');

  if (loading) return <PokeballLoader />;

  if (!tuning || ((tuning.byGrade?.length ?? 0) === 0 && (tuning.byFixedTier ?? []).every(t => t.purchaseCount === 0))) {
    return (
      <EmptyState
        icon="🎯"
        title="Not enough data to tune"
        description="Add at least a few purchases to see performance breakdowns and tuning recommendations."
      />
    );
  }

  async function applyRecommendation(rec: TuningRecommendation) {
    const updated = { ...campaign };
    switch (rec.parameter) {
      case 'buyTermsCLPct': {
        const val = parseFloat(rec.suggestedVal) / 100;
        if (!isNaN(val)) updated.buyTermsCLPct = val;
        break;
      }
      case 'dailySpendCap': {
        const match = rec.suggestedVal.match(/\$(\d+)/);
        if (match) updated.dailySpendCapCents = parseInt(match[1]) * 100;
        break;
      }
      case 'phase':
        if (rec.suggestedVal === 'pending' || rec.suggestedVal === 'active') {
          updated.phase = rec.suggestedVal;
        }
        break;
    }
    try {
      await onUpdate(updated);
      toast.success(`Applied: ${rec.parameter} changed to ${rec.suggestedVal}`);
    } catch {
      toast.error(`Failed to apply ${rec.parameter} change`);
    }
  }

  const tiers = tierView === 'fixed' ? (tuning.byFixedTier ?? []) : (tuning.byRelativeTier ?? []);
  const activeTiers = tiers.filter(t => t.purchaseCount > 0);
  const activeGrades = (tuning.byGrade ?? []).filter(g => g.purchaseCount > 0);

  return (
    <div className="space-y-6">
      {/* Recommendations */}
      {(tuning.recommendations ?? []).length > 0 && (
        <Section title="Recommendations">
          <div className="space-y-3">
            {(tuning.recommendations ?? []).map((rec, i) => (
              <RecommendationCard key={i} rec={rec} onApply={() => applyRecommendation(rec)} />
            ))}
          </div>
        </Section>
      )}

      {/* AI Campaign Analysis */}
      <AIAnalysisWidget
        endpoint="campaign-analysis"
        body={{ campaignId }}
        title="AI Campaign Analysis"
        buttonLabel="Analyze Campaign"
        description="Get an AI-powered assessment of this campaign's health, market conditions, and specific tuning recommendations."
      />

      {/* Market Health + Buy Threshold */}
      <div className={`grid ${isMobile ? 'grid-cols-1' : 'grid-cols-2'} gap-6`}>
        {tuning.marketAlignment && (
          <Section title="Market Health">
            <MarketHealthCard alignment={tuning.marketAlignment} />
          </Section>
        )}

        {tuning.buyThreshold && tuning.buyThreshold.bucketedROI.some(b => b.count > 0) && (
          <Section title="Buy Threshold Analysis">
            <ThresholdChart
              buckets={tuning.buyThreshold.bucketedROI}
              currentPct={tuning.buyThreshold.currentPct}
              optimalPct={tuning.buyThreshold.optimalPct}
              confidence={tuning.buyThreshold.confidence}
            />
          </Section>
        )}
      </div>

      {/* Performance by Grade */}
      {activeGrades.length > 0 && (
        <Section title="Performance by Grade">
          {isMobile ? (
            <div className="space-y-3">
              {activeGrades.map(g => <GradeCard key={g.grade} grade={g} />)}
            </div>
          ) : (
            <PerformanceTable
              headers={['Grade', 'Count', 'Sold %', 'Avg DTS', 'ROI', 'Avg CL%']}
              rows={activeGrades.map(g => ({
                key: String(g.grade),
                cells: [
                  `PSA ${g.grade}`,
                  String(g.purchaseCount),
                  formatPct(g.sellThroughPct),
                  g.soldCount > 0 ? `${g.avgDaysToSell.toFixed(0)}d` : '-',
                  formatPct(g.roi),
                  formatPct(g.avgBuyPctOfCL),
                ],
                roiValue: g.roi,
              }))}
            />
          )}
        </Section>
      )}

      {/* Performance by Price Tier */}
      {activeTiers.length > 0 && (
        <Section title="Performance by Price Tier">
          <div className="flex gap-2 mb-3" role="tablist" aria-label="Price tier view">
            <button
              role="tab"
              aria-selected={tierView === 'fixed'}
              tabIndex={tierView === 'fixed' ? 0 : -1}
              onClick={() => setTierView('fixed')}
              className={`text-xs px-3 py-1 rounded-md transition-colors ${
                tierView === 'fixed' ? 'bg-[var(--brand-500)] text-white' : 'text-[var(--text-muted)] hover:text-[var(--text)]'
              }`}
            >
              Fixed
            </button>
            <button
              role="tab"
              aria-selected={tierView === 'relative'}
              tabIndex={tierView === 'relative' ? 0 : -1}
              onClick={() => setTierView('relative')}
              className={`text-xs px-3 py-1 rounded-md transition-colors ${
                tierView === 'relative' ? 'bg-[var(--brand-500)] text-white' : 'text-[var(--text-muted)] hover:text-[var(--text)]'
              }`}
            >
              Relative
            </button>
          </div>
          {isMobile ? (
            <div className="space-y-3">
              {activeTiers.map(t => <TierCard key={t.tierLabel} tier={t} />)}
            </div>
          ) : (
            <PerformanceTable
              headers={['Tier', 'Count', 'Sold %', 'Avg DTS', 'ROI', 'Avg CL%']}
              rows={activeTiers.map(t => ({
                key: t.tierLabel,
                cells: [
                  t.tierLabel,
                  String(t.purchaseCount),
                  formatPct(t.sellThroughPct),
                  t.soldCount > 0 ? `${t.avgDaysToSell.toFixed(0)}d` : '-',
                  formatPct(t.roi),
                  formatPct(t.avgBuyPctOfCL),
                ],
                roiValue: t.roi,
              }))}
            />
          )}
        </Section>
      )}

      {/* Top/Bottom Performers */}
      {(tuning.topPerformers?.length > 0 || tuning.bottomPerformers?.length > 0) && (
        <div className={`grid ${isMobile ? 'grid-cols-1' : 'grid-cols-2'} gap-6`}>
          {tuning.topPerformers?.length > 0 && (
            <Section title="Top Performers">
              <div className="space-y-2">
                {tuning.topPerformers.map(cp => <CardPerfRow key={cp.purchase.id} cp={cp} />)}
              </div>
            </Section>
          )}
          {tuning.bottomPerformers?.length > 0 && (
            <Section title="Bottom Performers">
              <div className="space-y-2">
                {tuning.bottomPerformers.map(cp => <CardPerfRow key={cp.purchase.id} cp={cp} />)}
              </div>
            </Section>
          )}
        </div>
      )}

      {/* Monte Carlo Projections */}
      <MonteCarloSection campaignId={campaignId} isMobile={isMobile} />
    </div>
  );
}

// --- Sub-components ---

function Section({ title, children }: { title: string; children: React.ReactNode }) {
  return (
    <div className="bg-[var(--surface-1)] rounded-xl border border-[var(--surface-2)] p-4">
      <h3 className="text-sm font-semibold text-[var(--text)] mb-3">{title}</h3>
      {children}
    </div>
  );
}

function RecommendationCard({ rec, onApply }: { rec: TuningRecommendation; onApply: () => void }) {
  const impactColors = {
    high: 'text-[var(--danger)] bg-[var(--danger-bg)]',
    medium: 'text-[var(--warning)] bg-[var(--warning-bg)]',
    low: 'text-[var(--info)] bg-[var(--info-bg)]',
  };
  const canApply = rec.parameter === 'buyTermsCLPct' || rec.parameter === 'dailySpendCap' || rec.parameter === 'phase';

  return (
    <div className="p-3 rounded-lg bg-[var(--surface-2)]/50 border border-[var(--surface-2)]">
      <div className="flex items-center justify-between mb-1">
        <div className="flex items-center gap-2">
          <span className={`text-xs font-medium px-2 py-0.5 rounded-full ${impactColors[rec.impact]}`}>
            {rec.impact.toUpperCase()}
          </span>
          <span className="text-sm font-medium text-[var(--text)]">
            {rec.parameter}: {rec.currentVal} → {rec.suggestedVal}
          </span>
        </div>
        {canApply && rec.suggestedVal !== '(informational)' && (
          <Button size="sm" variant="ghost" onClick={onApply}>Apply</Button>
        )}
      </div>
      <p className="text-xs text-[var(--text-muted)]">
        {rec.reasoning}
        <span className="ml-2 opacity-60 inline-flex items-center gap-1">({rec.confidence} samples <ConfidenceIndicator confidence={sampleConfidence(rec.confidence)} size="sm" />)</span>
      </p>
    </div>
  );
}

function MarketHealthCard({ alignment }: { alignment: NonNullable<TuningResponse['marketAlignment']> }) {
  const signalColors = {
    healthy: 'text-[var(--success)]',
    caution: 'text-[var(--warning)]',
    warning: 'text-[var(--danger)]',
  };
  const signalIcons = { healthy: '↑', caution: '→', warning: '↓' };

  return (
    <div className="space-y-3">
      <div className="flex items-center gap-2">
        <span className={`text-lg font-bold ${signalColors[alignment.signal]}`}>
          {signalIcons[alignment.signal]} {alignment.signal.toUpperCase()}
        </span>
      </div>
      <p className="text-xs text-[var(--text-muted)]">{alignment.signalReason}</p>
      <div className="grid grid-cols-2 gap-2 text-sm">
        <Stat label="30d Trend" value={`${(alignment.avgTrend30d * 100).toFixed(1)}%`} />
        <Stat label="Liquidity" value={`${alignment.avgSalesLast30d.toFixed(1)}/mo`} />
        <Stat label="Drift" value={`${(alignment.avgSnapshotDrift * 100).toFixed(1)}%`} />
        <Stat label="Cards" value={`↑${alignment.appreciatingCount} →${alignment.stableCount} ↓${alignment.depreciatingCount}`} />
      </div>
    </div>
  );
}

function ThresholdChart({ buckets, currentPct, optimalPct, confidence }: {
  buckets: ThresholdBucket[];
  currentPct: number;
  optimalPct: number;
  confidence: number;
}) {
  const activeBuckets = buckets.filter(b => b.count > 0);
  if (activeBuckets.length === 0) return null;

  const maxROI = activeBuckets.reduce((max, b) => Math.max(max, Math.abs(b.medianROI)), 0.01);

  return (
    <div className="space-y-3">
      <div className="flex gap-4 text-xs text-[var(--text-muted)]">
        <span>Current: <strong className="text-[var(--text)]">{(currentPct * 100).toFixed(0)}%</strong></span>
        <span>Optimal: <strong className="text-[var(--success)]">{(optimalPct * 100).toFixed(0)}%</strong></span>
        <span className="inline-flex items-center gap-1">Confidence: {confidence} samples <ConfidenceIndicator confidence={sampleConfidence(confidence)} size="sm" /></span>
      </div>
      <div className="flex items-end gap-1 h-24">
        {activeBuckets.map(b => {
          const height = Math.max((Math.abs(b.medianROI) / maxROI) * 100, 4);
          const isPositive = b.medianROI >= 0;
          const mid = (b.rangeMinPct + b.rangeMaxPct) / 2;
          const isCurrent = Math.abs(mid - currentPct) < 0.03;
          const isOptimal = Math.abs(mid - optimalPct) < 0.03;

          return (
            <div key={b.rangeLabel} className="flex-1 flex flex-col items-center gap-1">
              <span className="text-[10px] text-[var(--text-muted)]">{formatPct(b.medianROI)}</span>
              <div
                className={`w-full rounded-t transition-colors ${
                  isOptimal ? 'bg-[var(--success)]' : isCurrent ? 'bg-[var(--info)]' : isPositive ? 'bg-[var(--success)]/40' : 'bg-[var(--danger)]/40'
                }`}
                style={{ height: `${height}%` }}
                title={`${b.rangeLabel}: ${b.count} purchases, ${formatPct(b.medianROI)} median ROI`}
              />
              <span className="text-[9px] text-[var(--text-muted)] whitespace-nowrap">
                {(b.rangeMinPct * 100).toFixed(0)}%
              </span>
            </div>
          );
        })}
      </div>
      <div className="flex gap-3 text-[10px] text-[var(--text-muted)]">
        <span><span className="inline-block w-2 h-2 bg-[var(--info)] rounded-sm mr-1" />Current</span>
        <span><span className="inline-block w-2 h-2 bg-[var(--success)] rounded-sm mr-1" />Optimal</span>
      </div>
    </div>
  );
}

function PerformanceTable({ headers, rows }: {
  headers: string[];
  rows: { key: string; cells: string[]; roiValue: number }[];
}) {
  return (
    <div className="overflow-x-auto">
      <table className="w-full text-sm">
        <thead>
          <tr className="text-[var(--text-muted)] text-xs">
            {headers.map(h => (
              <th key={h} className="text-left py-2 px-3 font-medium">{h}</th>
            ))}
          </tr>
        </thead>
        <tbody>
          {rows.map(row => (
            <tr key={row.key} className="border-t border-[var(--surface-2)]">
              {row.cells.map((cell, i) => (
                <td
                  key={i}
                  className={`py-2 px-3 ${
                    headers[i] === 'ROI'
                      ? row.roiValue >= 0 ? 'text-[var(--success)]' : 'text-[var(--danger)]'
                      : 'text-[var(--text)]'
                  }`}
                >
                  {cell}
                </td>
              ))}
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

function GradeCard({ grade: g }: { grade: GradePerformance }) {
  return (
    <div className="p-3 bg-[var(--surface-2)]/50 rounded-lg">
      <div className="flex items-center justify-between mb-2">
        <span className="font-medium text-[var(--text)]">PSA {g.grade}</span>
        <span className={`text-sm font-medium ${g.roi >= 0 ? 'text-[var(--success)]' : 'text-[var(--danger)]'}`}>
          {formatPct(g.roi)} ROI
        </span>
      </div>
      <div className="grid grid-cols-3 gap-2 text-xs">
        <Stat label="Count" value={String(g.purchaseCount)} />
        <Stat label="Sold" value={formatPct(g.sellThroughPct)} />
        <Stat label="DTS" value={g.soldCount > 0 ? `${g.avgDaysToSell.toFixed(0)}d` : '-'} />
      </div>
    </div>
  );
}

function TierCard({ tier: t }: { tier: PriceTierPerformance }) {
  return (
    <div className="p-3 bg-[var(--surface-2)]/50 rounded-lg">
      <div className="flex items-center justify-between mb-2">
        <span className="font-medium text-[var(--text)]">{t.tierLabel}</span>
        <span className={`text-sm font-medium ${t.roi >= 0 ? 'text-[var(--success)]' : 'text-[var(--danger)]'}`}>
          {formatPct(t.roi)} ROI
        </span>
      </div>
      <div className="grid grid-cols-3 gap-2 text-xs">
        <Stat label="Count" value={String(t.purchaseCount)} />
        <Stat label="Sold" value={formatPct(t.sellThroughPct)} />
        <Stat label="DTS" value={t.soldCount > 0 ? `${t.avgDaysToSell.toFixed(0)}d` : '-'} />
      </div>
    </div>
  );
}

function CardPerfRow({ cp }: { cp: CardPerformance }) {
  const pnl = cp.sale ? cp.realizedPnL : cp.unrealizedPnL;
  const pnlLabel = cp.sale ? 'realized' : 'unrealized';
  const trendArrow = cp.currentMarket?.trend30d
    ? cp.currentMarket.trend30d > 0.05 ? '↑' : cp.currentMarket.trend30d < -0.05 ? '↓' : '→'
    : '';

  return (
    <div className="flex items-center justify-between p-2 rounded-lg bg-[var(--surface-2)]/30">
      <div>
        <span className="text-sm text-[var(--text)]">{cp.purchase.cardName}</span>
        <span className="text-xs text-[var(--text-muted)] ml-2">PSA {cp.purchase.gradeValue}</span>
      </div>
      <div className="flex items-center gap-3 text-sm">
        {trendArrow && (
          <span className={`text-xs ${
            trendArrow === '↑' ? 'text-[var(--success)]' : trendArrow === '↓' ? 'text-[var(--danger)]' : 'text-[var(--text-muted)]'
          }`}>
            {trendArrow}
          </span>
        )}
        <span className={pnl >= 0 ? 'text-[var(--success)]' : 'text-[var(--danger)]'}>
          {pnl >= 0 ? '+' : ''}{formatCents(pnl)}
        </span>
        <span className="text-xs text-[var(--text-muted)]">({pnlLabel})</span>
      </div>
    </div>
  );
}

function MonteCarloSection({ campaignId, isMobile }: { campaignId: string; isMobile: boolean }) {
  const { data: projections, isLoading } = useProjections(campaignId);

  if (isLoading || !projections) return null;

  if (projections.confidence === 'insufficient') {
    return (
      <Section title="Monte Carlo Projections">
        <p className="text-sm text-[var(--text-muted)]">Need more completed cycles for projections.</p>
      </Section>
    );
  }

  const confidenceColors: Record<string, string> = {
    high: 'text-[var(--success)] bg-[var(--success-bg)]',
    medium: 'text-[var(--warning)] bg-[var(--warning-bg)]',
    low: 'text-[var(--danger)] bg-[var(--danger-bg)]',
  };

  const allRows: { result: MonteCarloResult; isCurrent: boolean; isBest: boolean }[] = [
    { result: projections.current, isCurrent: true, isBest: false },
    ...projections.scenarios.map((s, i) => ({
      result: s,
      isCurrent: false,
      isBest: i === projections.bestScenarioIndex,
    })),
  ];

  return (
    <Section title="Monte Carlo Projections">
      <div className="flex items-center gap-3 mb-3">
        <span className={`text-xs font-medium px-2 py-0.5 rounded-full ${confidenceColors[projections.confidence] ?? confidenceColors.low}`}>
          {projections.confidence.toUpperCase()}
        </span>
        <span className="text-xs text-[var(--text-muted)]">
          {projections.sampleSize} samples &middot; {projections.current.simulations.toLocaleString()} simulations
        </span>
      </div>

      {isMobile ? (
        <div className="space-y-3">
          {allRows.map(({ result, isCurrent, isBest }) => (
            <MonteCarloCard key={result.label} result={result} isCurrent={isCurrent} isBest={isBest} />
          ))}
        </div>
      ) : (
        <div className="overflow-x-auto">
          <table className="w-full text-sm">
            <thead>
              <tr className="text-[var(--text-muted)] text-xs">
                <th className="text-left py-2 px-3 font-medium">Scenario</th>
                <th className="text-right py-2 px-3 font-medium">Median ROI</th>
                <th className="text-right py-2 px-3 font-medium">P10-P90 ROI</th>
                <th className="text-right py-2 px-3 font-medium">Median Profit</th>
                <th className="text-right py-2 px-3 font-medium">P10-P90 Profit</th>
                <th className="text-right py-2 px-3 font-medium">Volume</th>
              </tr>
            </thead>
            <tbody>
              {allRows.map(({ result, isCurrent, isBest }) => {
                const rowClass = isBest
                  ? 'bg-[var(--success-bg)]'
                  : isCurrent
                    ? 'bg-[var(--brand-500)]/10'
                    : '';
                return (
                  <tr key={result.label} className={`border-t border-[var(--surface-2)] ${rowClass}`}>
                    <td className="py-2 px-3 text-[var(--text)]">
                      <span className="font-medium">{result.label}</span>
                      {isCurrent && <span className="ml-2 text-[10px] text-[var(--brand-400)]">(current)</span>}
                      {isBest && <span className="ml-2 text-[10px] text-[var(--success)]">(best)</span>}
                    </td>
                    <td className={`py-2 px-3 text-right ${result.medianROI >= 0 ? 'text-[var(--success)]' : 'text-[var(--danger)]'}`}>
                      {formatPct(result.medianROI)}
                    </td>
                    <td className="py-2 px-3 text-right text-[var(--text-muted)]">
                      {formatPct(result.p10ROI)} - {formatPct(result.p90ROI)}
                    </td>
                    <td className={`py-2 px-3 text-right ${result.medianProfitCents >= 0 ? 'text-[var(--success)]' : 'text-[var(--danger)]'}`}>
                      {formatCents(result.medianProfitCents)}
                    </td>
                    <td className="py-2 px-3 text-right text-[var(--text-muted)]">
                      {formatCents(result.p10ProfitCents)} - {formatCents(result.p90ProfitCents)}
                    </td>
                    <td className="py-2 px-3 text-right text-[var(--text)]">
                      {result.medianVolume}
                    </td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        </div>
      )}
    </Section>
  );
}

function MonteCarloCard({ result, isCurrent, isBest }: {
  result: MonteCarloResult;
  isCurrent: boolean;
  isBest: boolean;
}) {
  const borderClass = isBest
    ? 'border-[var(--success-border)]'
    : isCurrent
      ? 'border-[var(--brand-500)]/50'
      : 'border-transparent';

  return (
    <div className={`p-3 bg-[var(--surface-2)]/50 rounded-lg border ${borderClass}`}>
      <div className="flex items-center justify-between mb-2">
        <div className="flex items-center gap-2">
          <span className="font-medium text-[var(--text)]">{result.label}</span>
          {isCurrent && <span className="text-[10px] text-[var(--brand-400)]">(current)</span>}
          {isBest && <span className="text-[10px] text-[var(--success)]">(best)</span>}
        </div>
        <span className={`text-sm font-medium ${result.medianROI >= 0 ? 'text-[var(--success)]' : 'text-[var(--danger)]'}`}>
          {formatPct(result.medianROI)} ROI
        </span>
      </div>
      <div className="grid grid-cols-3 gap-2 text-xs">
        <Stat label="P10-P90 ROI" value={`${formatPct(result.p10ROI)} - ${formatPct(result.p90ROI)}`} />
        <Stat label="Median Profit" value={formatCents(result.medianProfitCents)} />
        <Stat label="Volume" value={String(result.medianVolume)} />
      </div>
    </div>
  );
}

function Stat({ label, value }: { label: string; value: string }) {
  return (
    <div>
      <div className="text-[var(--text-muted)]">{label}</div>
      <div className="text-[var(--text)] font-medium">{value}</div>
    </div>
  );
}
