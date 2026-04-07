import type { CapitalSummary } from '../../../types/campaigns';
import { formatCents } from '../../utils/formatters';

interface CapitalExposurePanelProps {
  capital?: CapitalSummary;
}

function weeksBadge(capital: CapitalSummary) {
  if (capital.recoveryRate30dCents === 0) {
    return <span className="px-2 py-0.5 rounded-full text-xs font-medium bg-[var(--surface-2)] text-[var(--text-muted)]">No sales data</span>;
  }
  const weeks = capital.weeksToCover;
  const label = weeks > 20 ? '20+ wks' : `~${Math.round(weeks)} wks`;
  const color = capital.alertLevel === 'critical' ? 'bg-[var(--danger)] text-white'
    : capital.alertLevel === 'warning' ? 'bg-[var(--warning)] text-black'
    : 'bg-[var(--success)] text-white';
  return <span className={`px-2 py-0.5 rounded-full text-xs font-medium ${color}`}>{label}</span>;
}

function trendArrow(trend: string) {
  if (trend === 'improving') return <span className="text-[var(--success)]" title="Improving">&#9650;</span>;
  if (trend === 'declining') return <span className="text-[var(--danger)]" title="Declining">&#9660;</span>;
  return <span className="text-[var(--text-muted)]" title="Stable">&#9654;</span>;
}

export default function CapitalExposurePanel({ capital }: CapitalExposurePanelProps) {
  if (!capital) return null;

  const outstandingColor = capital.outstandingCents === 0 ? 'text-[var(--success)]' : 'text-[var(--text)]';

  return (
    <div className="h-full p-4 bg-[var(--surface-1)] rounded-xl border border-[var(--surface-2)]">
      <h3 className="text-sm font-semibold text-[var(--text-muted)] uppercase tracking-wider mb-3">Capital Exposure</h3>

      <div className="flex items-baseline gap-3 mb-2">
        <span className={`text-2xl font-bold ${outstandingColor}`}>{formatCents(capital.outstandingCents)}</span>
        {weeksBadge(capital)}
      </div>

      {capital.recoveryRate30dCents > 0 && (
        <div className="text-xs text-[var(--text-muted)] mb-2">
          {formatCents(capital.recoveryRate30dCents)}/mo recovered {trendArrow(capital.recoveryTrend)}
        </div>
      )}

      {capital.recoveryRate30dCents === 0 && capital.outstandingCents > 0 && (
        <div className="text-xs text-[var(--text-muted)] mb-2">No recovery data yet</div>
      )}

      {(capital.unpaidInvoiceCount > 0 || capital.refundedCents > 0) && (
        <div className="text-xs text-[var(--text-muted)]">
          {capital.unpaidInvoiceCount > 0 && (
            <span>{capital.unpaidInvoiceCount} unpaid invoice{capital.unpaidInvoiceCount !== 1 ? 's' : ''}</span>
          )}
          {capital.refundedCents > 0 && (
            <span>{capital.unpaidInvoiceCount > 0 ? ' | ' : ''}{formatCents(capital.refundedCents)} refunded</span>
          )}
        </div>
      )}
    </div>
  );
}
