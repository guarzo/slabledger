import type { CompSummary } from '../../../../types/campaigns';
import { formatCents } from '../../../utils/formatters';

interface CompSummaryPanelProps {
  comp: CompSummary;
  costBasisCents?: number;
}

function trendLabel(trend: number): { text: string; color: string } {
  const pct = Math.round(trend * 100);
  if (Math.abs(pct) < 1) return { text: 'flat', color: 'var(--text-muted)' };
  if (pct > 0) return { text: `+${pct}%`, color: 'var(--success)' };
  return { text: `${pct}%`, color: 'var(--danger)' };
}

export default function CompSummaryPanel({ comp, costBasisCents }: CompSummaryPanelProps) {
  const trend = trendLabel(comp.trend90d);
  const aboveCostRatio = comp.recentComps > 0 ? comp.compsAboveCost / comp.recentComps : 0;
  const aboveCostColor = aboveCostRatio >= 0.5 ? 'var(--success)' : 'var(--danger)';

  const maxPlatformSales = comp.byPlatform?.reduce((m, p) => Math.max(m, p.saleCount), 0) ?? 0;

  return (
    <div className="mb-4 rounded-xl border border-[rgba(255,255,255,0.06)] bg-[rgba(255,255,255,0.02)] px-4 py-3">
      <div className="flex items-center justify-between mb-3">
        <div className="flex items-baseline gap-2">
          <span className="text-[10px] font-semibold uppercase tracking-[0.14em] text-[var(--brand-400)]">
            Sales Comps
          </span>
          <span className="text-[10px] text-[var(--text-muted)]">
            {comp.recentComps} recent · {comp.totalComps} total
          </span>
        </div>
      </div>

      {/* Headline stats: 4 columns, scannable */}
      <div className="grid grid-cols-2 md:grid-cols-4 gap-3 mb-3">
        <Stat label="Median" value={comp.medianCents > 0 ? formatCents(comp.medianCents) : '—'} />
        <Stat
          label="Range"
          value={
            comp.lowestCents > 0
              ? `${formatCents(comp.lowestCents)} – ${formatCents(comp.highestCents)}`
              : '—'
          }
        />
        <Stat
          label="Above Cost"
          value={`${comp.compsAboveCost}/${comp.recentComps}`}
          valueColor={costBasisCents && costBasisCents > 0 ? aboveCostColor : undefined}
          sub={
            costBasisCents && costBasisCents > 0
              ? `${Math.round(aboveCostRatio * 100)}%`
              : undefined
          }
        />
        <Stat label="90d Trend" value={trend.text} valueColor={trend.color} />
      </div>

      {/* Platform breakdown */}
      {comp.byPlatform && comp.byPlatform.length > 0 && (
        <div>
          <div className="text-[10px] uppercase tracking-wider text-[var(--text-muted)] mb-1.5">
            By Platform
          </div>
          <div className="grid gap-1">
            {comp.byPlatform.map(p => {
              const barPct = maxPlatformSales > 0 ? (p.saleCount / maxPlatformSales) * 100 : 0;
              return (
                <div
                  key={p.platform}
                  className="grid items-center gap-3 text-[11px]"
                  style={{ gridTemplateColumns: '100px 1fr 60px 120px' }}
                >
                  <span className="text-[var(--text)] truncate" title={p.platform}>
                    {p.platform}
                  </span>
                  <div className="relative h-1 rounded-full bg-[rgba(255,255,255,0.04)] overflow-hidden">
                    <div
                      className="absolute inset-y-0 left-0 rounded-full bg-[var(--brand-500)]/40"
                      style={{ width: `${barPct}%` }}
                    />
                  </div>
                  <span className="text-[var(--text-muted)] tabular-nums">
                    {p.saleCount} sale{p.saleCount === 1 ? '' : 's'}
                  </span>
                  <span className="text-[var(--text-muted)] tabular-nums text-right">
                    med <span className="text-[var(--text)]">{formatCents(p.medianCents)}</span>
                  </span>
                </div>
              );
            })}
          </div>
        </div>
      )}
    </div>
  );
}

interface StatProps {
  label: string;
  value: string;
  valueColor?: string;
  sub?: string;
}

function Stat({ label, value, valueColor, sub }: StatProps) {
  return (
    <div>
      <div className="text-[10px] uppercase tracking-wider text-[var(--text-muted)] mb-0.5">
        {label}
      </div>
      <div
        className="text-sm font-semibold tabular-nums leading-tight"
        style={{ color: valueColor ?? 'var(--text)' }}
      >
        {value}
      </div>
      {sub && <div className="text-[10px] text-[var(--text-muted)] mt-0.5">{sub}</div>}
    </div>
  );
}
