import type { CapitalSummary } from '../../../types/campaigns';
import { formatCents } from '../../utils/formatters';

interface InvoiceReadinessPanelProps {
  capital?: CapitalSummary;
}

/**
 * InvoiceReadinessPanel surfaces the operator's next PSA invoice exposure at a
 * glance: amount owed, due date, projected recovery velocity, cash buffer, and
 * any remaining cash gap. Color-codes covered vs short states so the operator
 * can tell in one glance whether the current invoice cycle is safe.
 */
export default function InvoiceReadinessPanel({ capital }: InvoiceReadinessPanelProps) {
  if (!capital) return null;

  // Empty state: no money owed. Keyed only on the amount so that invoices
  // with a missing nextInvoiceDate (data-entry edge case) still render as
  // long as there's an amount to show. The nextInvoiceDate block below is
  // already guarded against the missing value.
  if ((capital.nextInvoiceAmountCents ?? 0) === 0) {
    return (
      <div className="h-full p-4 bg-[var(--surface-1)] rounded-xl border border-[var(--surface-2)]">
        <h3 className="text-sm font-semibold text-[var(--text-muted)] uppercase tracking-wider mb-3">
          Invoice Readiness
        </h3>
        <div className="text-sm text-[var(--text-muted)]">No upcoming invoice.</div>
      </div>
    );
  }

  const amountCents = capital.nextInvoiceAmountCents;
  const projectedCents = capital.projectedRecoveryCents;
  const bufferCents = capital.cashBufferCents;
  const gapCents = capital.projectedCashGapCents;
  const coveredCents = Math.max(0, projectedCents + bufferCents);

  // Coverage as 0-100 percent of the next invoice.
  const rawCoverage = amountCents > 0 ? (coveredCents / amountCents) * 100 : 0;
  const coveragePct = Math.min(100, Math.max(0, rawCoverage));

  const covered = gapCents === 0;
  // Negative days means the due date is in the past — that is overdue. "Due
  // today" (exactly 0) should not be treated as overdue; the daysLabel below
  // surfaces it as "due today" instead.
  const overdue = capital.daysUntilInvoiceDue < 0;

  // Color coding: covered -> success, gap -> danger if overdue otherwise warning.
  const accentClass = covered
    ? 'text-[var(--success)]'
    : overdue
      ? 'text-[var(--danger)]'
      : 'text-[var(--warning)]';
  const barClass = covered
    ? 'bg-[var(--success)]'
    : overdue
      ? 'bg-[var(--danger)]'
      : 'bg-[var(--warning)]';
  const statusBadgeClass = covered
    ? 'bg-[var(--success)] text-white'
    : overdue
      ? 'bg-[var(--danger)] text-white'
      : 'bg-[var(--warning)] text-black';

  const statusLabel = covered ? 'Covered' : overdue ? 'Overdue' : 'Gap';

  const daysLabel = (() => {
    if (capital.daysUntilInvoiceDue < 0) {
      const days = Math.abs(capital.daysUntilInvoiceDue);
      return `${days} day${days === 1 ? '' : 's'} overdue`;
    }
    if (capital.daysUntilInvoiceDue === 0) return 'due today';
    return `${capital.daysUntilInvoiceDue} day${capital.daysUntilInvoiceDue === 1 ? '' : 's'}`;
  })();

  return (
    <div className="h-full p-4 bg-[var(--surface-1)] rounded-xl border border-[var(--surface-2)]">
      <div className="flex items-center justify-between mb-3">
        <h3 className="text-sm font-semibold text-[var(--text-muted)] uppercase tracking-wider">
          Invoice Readiness
        </h3>
        <span className={`px-2 py-0.5 rounded-full text-xs font-medium ${statusBadgeClass}`}>
          {statusLabel}
        </span>
      </div>

      {/* Headline: amount owed on next invoice */}
      <div className="flex items-baseline gap-3 mb-2">
        <span className={`text-2xl font-bold ${accentClass}`}>{formatCents(amountCents)}</span>
        <span className="text-xs text-[var(--text-muted)]">
          due {capital.nextInvoiceDueDate ?? '—'} ({daysLabel})
        </span>
      </div>

      {capital.nextInvoiceDate && (
        <div className="text-xs text-[var(--text-muted)] mb-3">
          Invoice dated {capital.nextInvoiceDate}
        </div>
      )}

      {/* Coverage bar: (projected + buffer) / amount */}
      <div className="mb-2">
        <div
          className="relative h-2 w-full rounded-full bg-[var(--surface-2)] overflow-hidden"
          role="progressbar"
          aria-label="Invoice coverage"
          aria-valuenow={Math.round(coveragePct)}
          aria-valuemin={0}
          aria-valuemax={100}
        >
          <div
            className={`h-full ${barClass} transition-[width] duration-500`}
            style={{ width: `${coveragePct}%` }}
          />
        </div>
        <div className="flex items-center justify-between mt-1 text-xs text-[var(--text-muted)]">
          <span>
            {formatCents(coveredCents)} projected + buffer
          </span>
          <span>{Math.round(coveragePct)}%</span>
        </div>
      </div>

      {/* Component breakdown */}
      <div className="grid grid-cols-2 gap-x-4 gap-y-1 text-xs text-[var(--text-muted)] mb-3">
        <div className="flex justify-between">
          <span>Projected recovery</span>
          <span className="text-[var(--text)] font-medium">{formatCents(projectedCents)}</span>
        </div>
        <div className="flex justify-between">
          <span>Cash buffer</span>
          <span className="text-[var(--text)] font-medium">{formatCents(bufferCents)}</span>
        </div>
      </div>

      {/* Status message */}
      {covered ? (
        <div className="text-xs text-[var(--success)] font-medium">
          Covered by projected recovery + cash buffer.
        </div>
      ) : (
        <div className="text-xs">
          <div className={`font-semibold ${accentClass}`}>
            Short {formatCents(gapCents)} at current recovery velocity
          </div>
          <div className="text-[var(--text-muted)] mt-0.5">
            Consider lowering CL buy % on the biggest liquidation-loss campaigns
            (see Suggestions tab).
          </div>
        </div>
      )}
    </div>
  );
}
