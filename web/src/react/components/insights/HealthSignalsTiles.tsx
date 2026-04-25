import type { Signals } from '../../../types/insights';
import { currency } from '../../utils/formatters';
import CardShell from '../../ui/CardShell';

type Tone = 'good' | 'warn' | 'bad' | 'muted';

const toneClass: Record<Tone, string> = {
  good: 'text-[var(--success)]',
  warn: 'text-[var(--warning)]',
  bad: 'text-[var(--danger)]',
  muted: 'text-[var(--text-muted)]',
};

export default function HealthSignalsTiles({ signals }: { signals: Signals }) {
  return (
    <section className="space-y-2">
      <div className="text-[11px] font-bold uppercase tracking-wider text-[var(--text-muted)]">
        Health signals (not on dashboard)
      </div>
      <div className="grid grid-cols-2 md:grid-cols-4 gap-2.5">
        <Tile
          label="AI accept rate (7d)"
          value={formatPct(signals.aiAcceptRate.pct, signals.aiAcceptRate.resolved)}
          sub={`${signals.aiAcceptRate.accepted} accepted / ${signals.aiAcceptRate.resolved} resolved`}
          tone={toneForAcceptRate(signals.aiAcceptRate.pct, signals.aiAcceptRate.resolved)}
        />
        <Tile
          label="Liquidation recoverable"
          value={currency(signals.liquidationRecoverableUsd)}
          sub={
            signals.liquidationRecoverableUsd > 0
              ? 'capital freed by current plan'
              : 'no forced-markdowns today'
          }
          tone={signals.liquidationRecoverableUsd > 0 ? 'good' : 'muted'}
        />
        <Tile
          label="Spike profit queued"
          value={currency(signals.spikeProfitUsd)}
          sub={`${signals.spikeCertCount} cert${signals.spikeCertCount === 1 ? '' : 's'} awaiting capture`}
          tone={signals.spikeProfitUsd > 0 ? 'good' : 'muted'}
        />
        <Tile
          label="Stuck in DH pipeline"
          value={String(signals.stuckInPipelineCount)}
          sub="in hand > 14d, not listed"
          tone={signals.stuckInPipelineCount > 0 ? 'warn' : 'muted'}
        />
      </div>
    </section>
  );
}

function Tile({ label, value, sub, tone }: { label: string; value: string; sub: string; tone: Tone }) {
  return (
    <CardShell padding="sm">
      <div className="text-[10px] uppercase tracking-wider text-[var(--text-muted)]">{label}</div>
      <div className={`text-lg font-bold tabular-nums ${toneClass[tone]}`}>{value}</div>
      <div className="text-[11px] text-[var(--text-muted)]">{sub}</div>
    </CardShell>
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
