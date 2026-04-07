import type { PortfolioHealth, CapitalSummary } from '../../../types/campaigns';
import { formatCents, formatPct } from '../../utils/formatters';

interface HeroStatsBarProps {
  health?: PortfolioHealth;
  capital?: CapitalSummary;
}

export default function HeroStatsBar({ health, capital }: HeroStatsBarProps) {
  if (!health) return null;

  const roi = health.realizedROI ?? 0;

  return (
    <div className="mb-7 pb-6 border-b border-[rgba(255,255,255,0.05)]">
      <div className="flex items-end gap-7 flex-wrap">
        <div>
          <div className="text-[11px] font-semibold text-[var(--brand-400)] uppercase tracking-wider mb-0.5">
            Realized ROI
          </div>
          <div className={`text-[32px] font-extrabold tracking-tight leading-none ${roi >= 0 ? 'text-[var(--success)]' : 'text-[var(--danger)]'}`}>
            {formatPct(roi)}
          </div>
        </div>
        <div className="flex flex-wrap gap-6 pb-1">
          <div>
            <div className="text-[10px] text-[var(--text-muted)] uppercase tracking-wider">Deployed</div>
            <div className="text-base font-semibold text-[#cbd5e1]">{formatCents(health.totalDeployedCents ?? 0)}</div>
          </div>
          <div>
            <div className="text-[10px] text-[var(--text-muted)] uppercase tracking-wider">Recovered</div>
            <div className="text-base font-semibold text-[#cbd5e1]">{formatCents(health.totalRecoveredCents ?? 0)}</div>
          </div>
          <div>
            <div className="text-[10px] text-[var(--text-muted)] uppercase tracking-wider">At Risk</div>
            <div className="text-base font-semibold text-[#cbd5e1]">{formatCents(health.totalAtRiskCents ?? 0)}</div>
          </div>
          {capital && (
            <>
              <div className="border-l border-[rgba(255,255,255,0.08)] pl-6">
                <div className="text-[10px] text-[var(--text-muted)] uppercase tracking-wider">Wks to Cover</div>
                <div className={`text-base font-semibold ${
                  capital.alertLevel === 'critical' ? 'text-[var(--danger)]'
                    : capital.alertLevel === 'warning' ? 'text-[var(--warning)]'
                    : capital.recoveryRate30dCents === 0 ? 'text-[var(--text-muted)]'
                    : 'text-[var(--success)]'
                }`}>
                  {capital.recoveryRate30dCents === 0 ? '—' : capital.weeksToCover > 20 ? '20+' : `~${Math.round(capital.weeksToCover)}`}
                </div>
              </div>
              <div>
                <div className="text-[10px] text-[var(--text-muted)] uppercase tracking-wider">Outstanding</div>
                <div className="text-base font-semibold text-[#cbd5e1]">{formatCents(capital.outstandingCents ?? 0)}</div>
              </div>
            </>
          )}
        </div>
      </div>
    </div>
  );
}
