import type { CompSummary } from '../../../../types/campaigns';
import { formatCents } from '../../../utils/formatters';

interface CompSummaryPanelProps {
  comp: CompSummary;
}

function trendLabel(trend: number): { text: string; color: string } {
  const pct = Math.round(trend * 100);
  if (Math.abs(pct) < 1) return { text: 'flat', color: 'var(--text-muted)' };
  if (pct > 0) return { text: `+${pct}%`, color: 'var(--success)' };
  return { text: `${pct}%`, color: 'var(--danger)' };
}

export default function CompSummaryPanel({ comp }: CompSummaryPanelProps) {
  const trend = trendLabel(comp.trend90d);

  return (
    <div className="mb-4 p-3 rounded-lg bg-[rgba(255,255,255,0.03)] border border-[rgba(255,255,255,0.06)]">
      <div className="flex items-center gap-2 mb-2">
        <span className="text-xs font-semibold text-[var(--brand-400)] uppercase tracking-wider">
          Sales Comps
        </span>
        <span className="text-[10px] text-[var(--text-muted)]">
          {comp.recentComps} recent (90d) / {comp.totalComps} total
        </span>
      </div>

      <div className="grid grid-cols-4 gap-3 mb-3">
        <div>
          <div className="text-[10px] text-[var(--text-muted)] uppercase tracking-wider">Median</div>
          <div className="text-sm font-semibold text-[var(--text)]">{formatCents(comp.medianCents)}</div>
        </div>
        <div>
          <div className="text-[10px] text-[var(--text-muted)] uppercase tracking-wider">High / Low</div>
          <div className="text-sm font-semibold text-[var(--text)]">
            {formatCents(comp.highestCents)} / {formatCents(comp.lowestCents)}
          </div>
        </div>
        <div>
          <div className="text-[10px] text-[var(--text-muted)] uppercase tracking-wider">Above Cost</div>
          <div className="text-sm font-semibold">
            <span className={comp.compsAboveCost > comp.recentComps / 2 ? 'text-[var(--success)]' : 'text-[var(--danger)]'}>
              {comp.compsAboveCost}/{comp.recentComps}
            </span>
          </div>
        </div>
        <div>
          <div className="text-[10px] text-[var(--text-muted)] uppercase tracking-wider">90d Trend</div>
          <div className="text-sm font-semibold" style={{ color: trend.color }}>{trend.text}</div>
        </div>
      </div>

      <div className="grid grid-cols-4 gap-3 mb-3">
        <div>
          <div className="text-[10px] text-[var(--text-muted)] uppercase tracking-wider">Above CL</div>
          <div className="text-sm font-semibold text-[var(--text)]">
            {comp.compsAboveCL}/{comp.recentComps}
          </div>
        </div>
        <div>
          <div className="text-[10px] text-[var(--text-muted)] uppercase tracking-wider">Last Sale</div>
          <div className="text-sm text-[var(--text)]">{comp.lastSaleDate || '-'}</div>
        </div>
      </div>

      {comp.byPlatform && comp.byPlatform.length > 0 && (
        <>
          <div className="text-[10px] text-[var(--text-muted)] uppercase tracking-wider mb-1">Platform Breakdown</div>
          <div className="grid gap-1">
            {comp.byPlatform.map(p => (
              <div key={p.platform} className="flex items-center gap-3 text-xs">
                <span className="text-[var(--text)] w-20 truncate">{p.platform}</span>
                <span className="text-[var(--text-muted)]">{p.saleCount} sales</span>
                <span className="text-[var(--text)]">med {formatCents(p.medianCents)}</span>
                <span className="text-[var(--text-muted)]">{formatCents(p.lowCents)} – {formatCents(p.highCents)}</span>
              </div>
            ))}
          </div>
        </>
      )}
    </div>
  );
}
