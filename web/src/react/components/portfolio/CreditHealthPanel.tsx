import type { CreditSummary } from '../../../types/campaigns';
import { formatCents, formatPctFromWhole } from '../../utils/formatters';

interface CreditHealthPanelProps {
  credit?: CreditSummary;
}

export default function CreditHealthPanel({ credit }: CreditHealthPanelProps) {
  if (!credit) return null;

  const barPct = Math.min(credit.utilizationPct, 100);
  const barColor = credit.alertLevel === 'critical' ? 'bg-[var(--danger)]' : credit.alertLevel === 'warning' ? 'bg-[var(--warning)]' : 'bg-[var(--success)]';
  const alertColor = credit.alertLevel === 'critical' ? 'text-[var(--danger)]' : credit.alertLevel === 'warning' ? 'text-[var(--warning)]' : 'text-[var(--success)]';

  const projectedPct = credit.creditLimitCents > 0 && credit.projectedExposureCents != null
    ? Math.min((credit.projectedExposureCents / credit.creditLimitCents) * 100, 100)
    : 0;
  const showProjectedMarker = projectedPct > barPct && credit.projectedExposureCents != null && credit.projectedExposureCents > credit.outstandingCents;

  return (
    <div className="h-full p-4 bg-[var(--surface-1)] rounded-xl border border-[var(--surface-2)]">
      <h3 className="text-sm font-semibold text-[var(--text-muted)] uppercase tracking-wider mb-3">Credit Utilization</h3>

      <div>
        <div className="flex items-center justify-between mb-2">
          <span className="text-xs text-[var(--text-muted)]">Credit Utilization</span>
          <span className={`text-xs font-medium ${alertColor}`}>{credit.alertLevel.toUpperCase()}</span>
        </div>
        <div className="relative w-full h-2 bg-[var(--surface-2)] rounded-full overflow-visible mb-2">
          <div className={`h-full ${barColor} rounded-full transition-all`} style={{ width: `${barPct}%` }} />
          {showProjectedMarker && (
            <div
              className="absolute top-[-2px] h-[calc(100%+4px)] w-[2px] border-l-2 border-dashed border-[var(--warning-border)]"
              style={{ left: `${projectedPct}%` }}
              title={`Projected: ${formatCents(credit.projectedExposureCents!)} (${projectedPct.toFixed(0)}%)`}
            />
          )}
        </div>
        <div className="flex justify-between text-xs text-[var(--text-muted)]">
          <span>{formatCents(credit.outstandingCents)} outstanding</span>
          <span>{formatPctFromWhole(credit.utilizationPct)} of {formatCents(credit.creditLimitCents)}</span>
        </div>
        {credit.unpaidInvoiceCount > 0 && (
          <div className="mt-1 text-xs text-[var(--text-muted)]">
            {credit.unpaidInvoiceCount} unpaid invoice{credit.unpaidInvoiceCount !== 1 ? 's' : ''}
            {credit.refundedCents > 0 && <span> | {formatCents(credit.refundedCents)} refunded</span>}
          </div>
        )}
        {credit.projectedExposureCents != null && credit.projectedExposureCents > 0 && (
          <div className="mt-1 text-xs text-[var(--text-muted)]">
            Projected: <span className="text-[var(--text)]">{formatCents(credit.projectedExposureCents)}</span> in {credit.daysToNextInvoice}d
          </div>
        )}
      </div>
    </div>
  );
}
