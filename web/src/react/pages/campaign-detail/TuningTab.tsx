import { useState } from 'react';
import type { Campaign, TuningRecommendation } from '../../../types/campaigns';
import PokeballLoader from '../../PokeballLoader';
import { formatPct } from '../../utils/formatters';
import { useToast } from '../../contexts/ToastContext';
import { useMediaQuery } from '../../hooks/useMediaQuery';
import { EmptyState } from '../../ui';
import { useTuning } from '../../queries/useCampaignQueries';
import AIAnalysisWidget from '../../components/advisor/AIAnalysisWidget';
import {
  Section, RecommendationCard, MarketHealthCard, ThresholdChart,
  PerformanceTable, GradeCard, TierCard, CardPerfRow, MonteCarloSection,
} from './tuning/TuningComponents';

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
