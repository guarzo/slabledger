import type { Signals } from '../../../types/insights';
import { currency } from '../../utils/formatters';
import SectionEyebrow from '../../ui/SectionEyebrow';
import StatCard from '../../ui/StatCard';

type Tone = 'good' | 'warn' | 'bad' | 'muted';

const toneToColor: Record<Tone, 'green' | 'red' | 'yellow' | undefined> = {
  good: 'green',
  warn: 'yellow',
  bad: 'red',
  muted: undefined,
};

export default function HealthSignalsTiles({ signals }: { signals: Signals }) {
  return (
    <section className="space-y-2">
      <SectionEyebrow>Health signals (not on dashboard)</SectionEyebrow>
      <div className="grid grid-cols-2 md:grid-cols-4 gap-2.5">
        <StatCard
          label="AI accept rate (7d)"
          value={formatPct(signals.aiAcceptRate.pct, signals.aiAcceptRate.resolved)}
          sub={`${signals.aiAcceptRate.accepted} accepted / ${signals.aiAcceptRate.resolved} resolved`}
          color={toneToColor[toneForAcceptRate(signals.aiAcceptRate.pct, signals.aiAcceptRate.resolved)]}
        />
        <StatCard
          label="Liquidation recoverable"
          value={currency(signals.liquidationRecoverableUsd)}
          sub={
            signals.liquidationRecoverableUsd > 0
              ? 'capital freed by current plan'
              : 'no forced-markdowns today'
          }
          color={toneToColor[signals.liquidationRecoverableUsd > 0 ? 'good' : 'muted']}
        />
        <StatCard
          label="Spike profit queued"
          value={currency(signals.spikeProfitUsd)}
          sub={`${signals.spikeCertCount} cert${signals.spikeCertCount === 1 ? '' : 's'} awaiting capture`}
          color={toneToColor[signals.spikeProfitUsd > 0 ? 'good' : 'muted']}
        />
        <StatCard
          label="Stuck in DH pipeline"
          value={String(signals.stuckInPipelineCount)}
          sub="in hand > 14d, not listed"
          color={toneToColor[signals.stuckInPipelineCount > 0 ? 'warn' : 'muted']}
        />
      </div>
    </section>
  );
}

function formatPct(pct: number, resolved: number): string {
  if (resolved === 0) return '—';
  return `${pct.toFixed(1)}%`;
}

function toneForAcceptRate(pct: number, resolved: number): Tone {
  if (resolved === 0) return 'muted';
  if (pct < 25) return 'bad';
  if (pct < 50) return 'warn';
  return 'good';
}
