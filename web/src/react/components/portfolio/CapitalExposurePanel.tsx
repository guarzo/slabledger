import type { CapitalSummary } from '../../../types/campaigns';
import { formatCents, formatPctFromWhole } from '../../utils/formatters';

interface CapitalExposurePanelProps {
  capital?: CapitalSummary;
}

export default function CapitalExposurePanel({ capital }: CapitalExposurePanelProps) {
  if (!capital) return null;

  const barPct = Math.min(capital.exposurePct, 100);
  const barColor = capital.alertLevel === 'critical' ? 'bg-[var(--danger)]' : capital.alertLevel === 'warning' ? 'bg-[var(--warning)]' : 'bg-[var(--success)]';
  const alertColor = capital.alertLevel === 'critical' ? 'text-[var(--danger)]' : capital.alertLevel === 'warning' ? 'text-[var(--warning)]' : 'text-[var(--success)]';

  const projectedPct = capital.capitalBudgetCents > 0 && capital.projectedExposureCents != null
    ? Math.min((capital.projectedExposureCents / capital.capitalBudgetCents) * 100, 100)
    : 0;
  const showProjectedMarker = projectedPct > barPct && capital.projectedExposureCents != null && capital.projectedExposureCents > capital.outstandingCents;

  return (
    <div className="h-full p-4 bg-[var(--surface-1)] rounded-xl border border-[var(--surface-2)]">
      <h3 className="text-sm font-semibold text-[var(--text-muted)] uppercase tracking-wider mb-3">Capital Exposure</h3>

      <div>
        <div className="flex items-center justify-between mb-2">
          <span className="text-xs text-[var(--text-muted)]">Capital Exposure</span>
          <span className={`text-xs font-medium ${alertColor}`}>{capital.alertLevel.toUpperCase()}</span>
        </div>
        <div className="relative w-full h-2 bg-[var(--surface-2)] rounded-full overflow-visible mb-2">
          <div className={`h-full ${barColor} rounded-full transition-all`} style={{ width: `${barPct}%` }} />
          {showProjectedMarker && (
            <div
              className="absolute top-[-2px] h-[calc(100%+4px)] w-[2px] border-l-2 border-dashed border-[var(--warning-border)]"
              style={{ left: `${projectedPct}%` }}
              title={`Projected: ${formatCents(capital.projectedExposureCents!)} (${projectedPct.toFixed(0)}%)`}
            />
          )}
        </div>
        <div className="flex justify-between text-xs text-[var(--text-muted)]">
          <span>{formatCents(capital.outstandingCents)} outstanding</span>
          {capital.capitalBudgetCents > 0
            ? <span>{formatPctFromWhole(capital.exposurePct)} of {formatCents(capital.capitalBudgetCents)} budget</span>
            : <span>{formatCents(capital.outstandingCents)} deployed</span>
          }
        </div>
        {capital.unpaidInvoiceCount > 0 && (
          <div className="mt-1 text-xs text-[var(--text-muted)]">
            {capital.unpaidInvoiceCount} unpaid invoice{capital.unpaidInvoiceCount !== 1 ? 's' : ''}
            {capital.refundedCents > 0 && <span> | {formatCents(capital.refundedCents)} refunded</span>}
          </div>
        )}
        {capital.projectedExposureCents != null && capital.projectedExposureCents > 0 && (
          <div className="mt-1 text-xs text-[var(--text-muted)]">
            Projected: <span className="text-[var(--text)]">{formatCents(capital.projectedExposureCents)}</span> in {capital.daysToNextInvoice}d
          </div>
        )}
      </div>
    </div>
  );
}
