import type { PortfolioHealth, CapitalSummary } from '../../../types/campaigns';
import { formatCents, formatPct, formatWeeksToCover } from '../../utils/formatters';

interface HeroStatsBarProps {
  health?: PortfolioHealth;
  capital?: CapitalSummary;
}

export default function HeroStatsBar({ health, capital }: HeroStatsBarProps) {
  if (!health) return null;

  // Onboarding: all-zero state
  const hasActivity = health.totalDeployedCents > 0 || health.totalRecoveredCents > 0 || health.realizedROI !== 0;
  if (!hasActivity) {
    return (
      <div className="mb-6 p-6 bg-[var(--surface-1)] rounded-xl border border-[var(--surface-2)] text-center">
        <h2 className="text-lg font-semibold text-[var(--text)] mb-2">Welcome to SlabLedger</h2>
        <p className="text-sm text-[var(--text-muted)] max-w-md mx-auto mb-4">
          Your portfolio dashboard will come alive once you start tracking. Here&apos;s how to get started:
        </p>
        <div className="flex flex-col sm:flex-row gap-3 justify-center text-sm text-[var(--text-muted)]">
          <div className="flex items-center gap-2">
            <span className="w-6 h-6 rounded-full bg-[var(--brand-500)]/20 text-[var(--brand-400)] flex items-center justify-center text-xs font-bold">1</span>
            <span>Create a campaign</span>
          </div>
          <div className="flex items-center gap-2">
            <span className="w-6 h-6 rounded-full bg-[var(--brand-500)]/20 text-[var(--brand-400)] flex items-center justify-center text-xs font-bold">2</span>
            <span>Import PSA purchases</span>
          </div>
          <div className="flex items-center gap-2">
            <span className="w-6 h-6 rounded-full bg-[var(--brand-500)]/20 text-[var(--brand-400)] flex items-center justify-center text-xs font-bold">3</span>
            <span>Record sales as you go</span>
          </div>
        </div>
      </div>
    );
  }

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
                  {capital.outstandingCents === 0 && capital.recoveryRate30dCents > 0 ? '0' : formatWeeksToCover(capital.weeksToCover, capital.recoveryRate30dCents > 0)}
                </div>
              </div>
              <div>
                <div className="text-[10px] text-[var(--text-muted)] uppercase tracking-wider">Outstanding</div>
                <div className="text-base font-semibold text-[#cbd5e1]">{formatCents(capital.outstandingCents)}</div>
              </div>
            </>
          )}
        </div>
      </div>
    </div>
  );
}
