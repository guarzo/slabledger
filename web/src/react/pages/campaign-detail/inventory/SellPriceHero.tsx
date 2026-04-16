import type { AgingItem } from '../../../../types/campaigns';
import { formatCents } from '../../../utils/formatters';
import { mostRecentSale } from './utils';

interface SellPriceHeroProps {
  item: AgingItem;
  costBasisCents: number;
}

function formatSaleDate(dateStr?: string): string | undefined {
  if (!dateStr) return undefined;
  const d = new Date(dateStr + 'T00:00:00');
  if (isNaN(d.getTime())) return undefined;
  return d.toLocaleDateString('en-US', { month: 'short', day: 'numeric', year: 'numeric' });
}

function clampPct(v: number): number {
  return Math.max(0, Math.min(100, v));
}

export default function SellPriceHero({ item, costBasisCents }: SellPriceHeroProps) {
  const recent = mostRecentSale(item);
  const cs = item.compSummary;
  const trendPct = cs ? Math.round(cs.trend90d * 100) : 0;
  const trendColor =
    !cs || Math.abs(trendPct) < 1 ? 'var(--text-muted)'
      : trendPct > 0 ? 'var(--success)'
      : 'var(--danger)';
  const trendLabel = !cs ? '—'
    : Math.abs(trendPct) < 1 ? 'flat'
    : trendPct > 0 ? `+${trendPct}%`
    : `${trendPct}%`;

  // Range bar — needs at least low and high to render
  const hasRange = cs && cs.lowestCents > 0 && cs.highestCents > cs.lowestCents;
  let medianPct = 50;
  let costPct: number | null = null;
  let recentPct: number | null = null;
  if (hasRange) {
    const span = cs.highestCents - cs.lowestCents;
    medianPct = clampPct(((cs.medianCents - cs.lowestCents) / span) * 100);
    if (costBasisCents > 0) {
      costPct = clampPct(((costBasisCents - cs.lowestCents) / span) * 100);
    }
    if (recent && recent.cents > 0) {
      recentPct = clampPct(((recent.cents - cs.lowestCents) / span) * 100);
    }
  }

  return (
    <div className="mb-4 rounded-xl border border-[rgba(255,255,255,0.08)] bg-gradient-to-br from-[rgba(99,102,241,0.06)] to-[rgba(255,255,255,0.02)] px-5 py-4">
      <div className="flex items-start justify-between gap-6 flex-wrap">
        {/* Left: Most recent sale headline */}
        <div className="min-w-0">
          <div className="text-[10px] font-semibold uppercase tracking-[0.14em] text-[var(--brand-400)] mb-1">
            Most Recent Sale
          </div>
          {recent ? (
            <div className="flex items-baseline gap-2 flex-wrap">
              <span className="text-3xl font-semibold tabular-nums text-[var(--text)] leading-none">
                {formatCents(recent.cents)}
              </span>
              {recent.date && (
                <span className="text-xs text-[var(--text-muted)]">
                  {formatSaleDate(recent.date)}
                </span>
              )}
            </div>
          ) : (
            <div className="text-sm text-[var(--text-muted)] italic">No recent sales</div>
          )}
        </div>

        {/* Right: Stat trio */}
        <div className="flex gap-6">
          <HeroStat
            label="90d Comps"
            value={cs ? `${cs.recentComps}` : '—'}
            sub={cs ? `${cs.totalComps} total` : undefined}
          />
          <HeroStat
            label="90d Median"
            value={cs && cs.medianCents > 0 ? formatCents(cs.medianCents) : '—'}
            sub={cs && cs.compsAboveCost > 0 ? `${cs.compsAboveCost}/${cs.recentComps} above cost` : undefined}
          />
          <HeroStat
            label="90d Trend"
            value={trendLabel}
            valueColor={trendColor}
          />
        </div>
      </div>

      {/* Range bar */}
      {hasRange && (
        <div className="mt-4">
          <div className="relative h-1.5 rounded-full bg-[rgba(255,255,255,0.06)] overflow-visible">
            {/* fill — low to high */}
            <div className="absolute inset-0 rounded-full bg-gradient-to-r from-[rgba(248,113,113,0.35)] via-[rgba(148,163,184,0.5)] to-[rgba(52,211,153,0.4)]" />
            {/* median tick */}
            <TickMark pct={medianPct} color="var(--brand-400)" label="med" />
            {/* cost tick */}
            {costPct !== null && (
              <TickMark pct={costPct} color="var(--warning)" label="cost" direction="down" />
            )}
            {/* last-sold tick */}
            {recentPct !== null && (
              <TickMark pct={recentPct} color="var(--text)" label="last" direction="down" />
            )}
          </div>
          <div className="mt-4 flex justify-between text-[10px] tabular-nums text-[var(--text-muted)]">
            <span>{formatCents(cs.lowestCents)}</span>
            <span>{formatCents(cs.highestCents)}</span>
          </div>
        </div>
      )}
    </div>
  );
}

interface HeroStatProps {
  label: string;
  value: string;
  valueColor?: string;
  sub?: string;
}

function HeroStat({ label, value, valueColor, sub }: HeroStatProps) {
  return (
    <div className="min-w-[72px]">
      <div className="text-[10px] uppercase tracking-wider text-[var(--text-muted)] mb-0.5">
        {label}
      </div>
      <div
        className="text-sm font-semibold tabular-nums leading-tight"
        style={{ color: valueColor ?? 'var(--text)' }}
      >
        {value}
      </div>
      {sub && (
        <div className="text-[10px] text-[var(--text-muted)] mt-0.5 whitespace-nowrap">{sub}</div>
      )}
    </div>
  );
}

interface TickMarkProps {
  pct: number;
  color: string;
  label: string;
  direction?: 'up' | 'down';
}

function TickMark({ pct, color, label, direction = 'up' }: TickMarkProps) {
  const aboveBar = direction === 'up';
  return (
    <div
      className="absolute top-1/2 -translate-x-1/2 -translate-y-1/2 flex flex-col items-center pointer-events-none"
      style={{ left: `${pct}%` }}
    >
      {aboveBar && (
        <span className="text-[9px] font-semibold uppercase tracking-wider mb-0.5" style={{ color }}>
          {label}
        </span>
      )}
      <span className="block w-[2px] h-3 rounded-full" style={{ background: color }} />
      {!aboveBar && (
        <span className="text-[9px] font-semibold uppercase tracking-wider mt-0.5" style={{ color }}>
          {label}
        </span>
      )}
    </div>
  );
}
