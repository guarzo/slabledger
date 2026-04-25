import type { CampaignPNL } from '../../../types/campaigns';
import { formatCents, formatPct } from '../../utils/formatters';
import StatCard from '../../ui/StatCard';

interface PortfolioSummaryProps {
  campaignCount: number;
  pnlMap: Record<string, CampaignPNL>;
}

export default function PortfolioSummary({ campaignCount, pnlMap }: PortfolioSummaryProps) {
  const pnls = Object.values(pnlMap);
  if (pnls.length === 0) return null;

  const totalSpent = pnls.reduce((sum, p) => sum + p.totalSpendCents, 0);
  const totalRevenue = pnls.reduce((sum, p) => sum + p.totalRevenueCents, 0);
  const totalProfit = pnls.reduce((sum, p) => sum + p.netProfitCents, 0);
  const totalUnsold = pnls.reduce((sum, p) => sum + p.totalPurchases - p.totalSold, 0);
  const roi = totalSpent > 0 ? totalProfit / totalSpent : 0;

  return (
    <div className="mb-6 space-y-3">
      <div className="grid grid-cols-2 md:grid-cols-5 gap-3">
        <StatCard label="Campaigns" value={`${campaignCount}`} />
        <StatCard label="Invested" value={formatCents(totalSpent)} />
        <StatCard label="Revenue" value={formatCents(totalRevenue)} />
        <StatCard label="P&L" value={`${formatCents(totalProfit)} (${formatPct(roi)})`} color={totalProfit >= 0 ? 'green' : 'red'} />
        <StatCard label="Unsold" value={`${totalUnsold}`} />
      </div>
      {totalSpent > 0 && (() => {
        const recoveryPct = (totalRevenue / totalSpent) * 100;
        const clampedRecovery = Math.max(0, Math.min(100, recoveryPct));
        return (
          <div>
            <div className="text-sm tabular-nums text-[var(--text-muted)] mb-1.5">
              {formatCents(totalRevenue)} of {formatCents(totalSpent)} recovered (
              <span className="text-[var(--text)] font-medium">{recoveryPct.toFixed(0)}%</span>
              )
            </div>
            <div
              className="w-full h-1 rounded-full bg-[var(--surface-2)] overflow-hidden"
              role="progressbar"
              aria-label={`Capital recovered: ${formatCents(totalRevenue)} of ${formatCents(totalSpent)} invested (${recoveryPct.toFixed(0)}%)`}
              aria-valuenow={Math.round(clampedRecovery)}
              aria-valuetext={`${recoveryPct.toFixed(0)}%`}
              aria-valuemin={0}
              aria-valuemax={100}
            >
              <div
                className="h-full rounded-full transition-all duration-500 bg-[var(--brand-500)]"
                style={{ width: `${clampedRecovery}%` }}
              />
            </div>
          </div>
        );
      })()}
    </div>
  );
}
