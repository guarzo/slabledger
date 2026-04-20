import { Link } from 'react-router-dom';
import type { CapitalSummary } from '../../../types/campaigns';
import { formatCents, formatWeeksToCover } from '../../utils/formatters';
import TrendArrow from '../../ui/TrendArrow';

interface CapitalExposurePanelProps {
  capital?: CapitalSummary;
}

const trendToArrow = { improving: 'up', declining: 'down', stable: 'stable' } as const;

function alertBadgeColor(level: string): string {
  if (level === 'critical') return 'bg-[var(--danger)] text-white';
  if (level === 'warning') return 'bg-[var(--warning)] text-black';
  return 'bg-[var(--success)] text-white';
}

function weeksBadge(capital: CapitalSummary) {
  if (capital.recoveryRate30dCents === 0) {
    return <span className="px-2 py-0.5 rounded-full text-xs font-medium bg-[var(--surface-2)] text-[var(--text-muted)]">No sales data</span>;
  }
  if (capital.outstandingCents === 0) {
    return <span className="px-2 py-0.5 rounded-full text-xs font-medium bg-[var(--success)] text-white">Covered</span>;
  }
  const label = `${formatWeeksToCover(capital.weeksToCover, true)} wks`;
  return <span className={`px-2 py-0.5 rounded-full text-xs font-medium ${alertBadgeColor(capital.alertLevel)}`}>{label}</span>;
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
          {formatCents(capital.recoveryRate30dCents)}/30d recovered <TrendArrow trend={trendToArrow[capital.recoveryTrend]} />
        </div>
      )}

      {capital.recoveryRate30dCents === 0 && capital.outstandingCents > 0 && (
        <div className="text-xs text-[var(--text-muted)] mb-2">No recovery data yet</div>
      )}

      {(capital.unpaidInvoiceCount > 0 || capital.refundedCents > 0) && (
        <div className="text-xs text-[var(--text-muted)]">
          {capital.unpaidInvoiceCount > 0 && (
            <Link
              to="/invoices"
              aria-label={`Open ${capital.unpaidInvoiceCount} unpaid invoice${capital.unpaidInvoiceCount !== 1 ? 's' : ''}`}
              className="text-[var(--warning)] hover:underline"
            >
              {capital.unpaidInvoiceCount} unpaid invoice{capital.unpaidInvoiceCount !== 1 ? 's' : ''} →
            </Link>
          )}
          {capital.refundedCents > 0 && (
            <span>{capital.unpaidInvoiceCount > 0 ? ' | ' : ''}{formatCents(capital.refundedCents)} refunded</span>
          )}
        </div>
      )}
    </div>
  );
}
