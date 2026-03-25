import type { CampaignPNL } from '../../../types/campaigns';
import { formatCents, formatPct } from '../../utils/formatters';

export default function PNLBadge({ pnl }: { pnl: CampaignPNL }) {
  const isProfit = pnl.netProfitCents >= 0;
  const profitColor = isProfit ? 'text-[var(--success)]' : 'text-[var(--danger)]';
  const accentColor = isProfit ? 'var(--success)' : 'var(--danger)';

  return (
    <div className="grid grid-cols-3 gap-2">
      <div
        className="bg-[var(--surface-0)] rounded-lg p-2 border-l-2"
        style={{ borderLeftColor: accentColor }}
      >
        <div className="text-xs text-[var(--text-muted)]">Net Profit</div>
        <div className={`text-sm font-semibold ${profitColor}`}>{formatCents(pnl.netProfitCents)}</div>
      </div>
      <div
        className="bg-[var(--surface-0)] rounded-lg p-2 border-l-2"
        style={{ borderLeftColor: accentColor }}
      >
        <div className="text-xs text-[var(--text-muted)]">ROI</div>
        <div className={`text-sm font-semibold ${profitColor}`}>{formatPct(pnl.roi)}</div>
      </div>
      <div
        className="bg-[var(--surface-0)] rounded-lg p-2 border-l-2"
        style={{ borderLeftColor: 'var(--brand-500)' }}
      >
        <div className="text-xs text-[var(--text-muted)]">Sell-Through</div>
        <div className="text-sm font-semibold text-[var(--text)]">
          {pnl.totalSold}/{pnl.totalPurchases} ({formatPct(pnl.sellThroughPct)})
        </div>
      </div>
    </div>
  );
}
