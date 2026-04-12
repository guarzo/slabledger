import type { CapitalSummary } from '../../../types/campaigns';
import { formatCents } from '../../utils/formatters';

interface InvoiceReadinessPanelProps {
  capital?: CapitalSummary;
}

/**
 * InvoiceReadinessPanel shows the operator's next PSA invoice at a glance:
 * amount owed, due date, how many cards are still pending return from PSA,
 * and sell-through (units + dollars) for the returned portion.
 * No projections or gap calculations — actuals only.
 */
export default function InvoiceReadinessPanel({ capital }: InvoiceReadinessPanelProps) {
  if (!capital) return null;

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

  const daysUntil = capital.daysUntilInvoiceDue;
  const overdue = daysUntil < 0;

  const daysLabel = (() => {
    if (overdue) {
      const days = Math.abs(daysUntil);
      return `${days} day${days === 1 ? '' : 's'} overdue`;
    }
    if (daysUntil === 0) return 'due today';
    return `${daysUntil} day${daysUntil === 1 ? '' : 's'}`;
  })();

  const st = capital.nextInvoiceSellThrough;
  const sellThroughPct = st.totalPurchaseCount > 0
    ? Math.round((st.soldCount / st.totalPurchaseCount) * 100)
    : 0;

  return (
    <div className="h-full p-4 bg-[var(--surface-1)] rounded-xl border border-[var(--surface-2)]">
      <div className="flex items-center justify-between mb-3">
        <h3 className="text-sm font-semibold text-[var(--text-muted)] uppercase tracking-wider">
          Invoice Readiness
        </h3>
      </div>

      {/* Headline: amount owed on next invoice */}
      <div className="flex items-baseline gap-3 mb-1">
        <span className="text-2xl font-bold text-[var(--text)]">
          {formatCents(capital.nextInvoiceAmountCents)}
        </span>
        <span className="text-xs text-[var(--text-muted)]">
          due {capital.nextInvoiceDueDate ?? '—'} · {daysLabel}
        </span>
      </div>

      {capital.nextInvoiceDate && (
        <div className="text-xs text-[var(--text-muted)] mb-4">
          Invoice dated {capital.nextInvoiceDate}
        </div>
      )}

      {/* Pending receipt */}
      <div className="flex justify-between text-xs mb-2">
        <span className="text-[var(--text-muted)]">Pending receipt (at PSA)</span>
        {capital.nextInvoicePendingReceiptCents > 0 ? (
          <span className="text-[var(--warning)] font-medium">
            {formatCents(capital.nextInvoicePendingReceiptCents)} pending
          </span>
        ) : (
          <span className="text-[var(--success)] font-medium">All received</span>
        )}
      </div>

      {/* Sell-through */}
      <div className="border-t border-[var(--surface-2)] pt-3 mt-2">
        <div className="flex justify-between text-xs mb-1">
          <span className="text-[var(--text-muted)]">Sell-through</span>
          <span className="text-[var(--text)] font-medium">
            {st.soldCount} of {st.totalPurchaseCount} cards ({sellThroughPct}%)
          </span>
        </div>
        <div className="flex justify-between text-xs">
          <span className="text-[var(--text-muted)]">Revenue / cost</span>
          <span className="text-[var(--text)] font-medium">
            {formatCents(st.saleRevenueCents)} / {formatCents(st.totalCostCents)}
          </span>
        </div>
      </div>
    </div>
  );
}
