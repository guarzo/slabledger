import { Link } from 'react-router-dom';
import type { Invoice } from '../../types/campaigns';
import { useInvoices } from '../queries/useCampaignQueries';
import { formatCents } from '../utils/formatters';
import { CardShell, SectionErrorBoundary } from '../ui';
import PokeballLoader from '../PokeballLoader';

function formatDate(d?: string): string {
  if (!d) return '—';
  const iso = d.slice(0, 10);
  return iso;
}

function statusPill(status: Invoice['status'], daysToDue: number) {
  if (status === 'paid') {
    return <span className="text-[11px] font-medium px-2 py-0.5 rounded-full bg-[var(--success)]/10 text-[var(--success)]">Paid</span>;
  }
  if (status === 'partial') {
    return <span className="text-[11px] font-medium px-2 py-0.5 rounded-full bg-[var(--warning)]/10 text-[var(--warning)]">Partial</span>;
  }
  const overdue = daysToDue < 0;
  return (
    <span className={`text-[11px] font-medium px-2 py-0.5 rounded-full ${overdue ? 'bg-[var(--danger)]/10 text-[var(--danger)]' : 'bg-[var(--warning)]/10 text-[var(--warning)]'}`}>
      {overdue ? 'Overdue' : 'Unpaid'}
    </span>
  );
}

// Compare dates at day granularity. ISO date-only strings (YYYY-MM-DD) are
// parsed by JS as UTC midnight, but new Date() is local — mixing them skews
// the diff by up to a day in most timezones. Anchor both ends at UTC
// midnight when the input is date-only so the result is stable.
function daysUntil(dateStr?: string): number {
  if (!dateStr) return 0;
  const msPerDay = 1000 * 60 * 60 * 24;
  const now = new Date();
  const todayUtc = Date.UTC(now.getFullYear(), now.getMonth(), now.getDate());
  const isoDateOnly = /^\d{4}-\d{2}-\d{2}$/.exec(dateStr);
  if (isoDateOnly) {
    const [y, m, d] = dateStr.split('-').map(Number);
    const targetUtc = Date.UTC(y, m - 1, d);
    return Math.floor((targetUtc - todayUtc) / msPerDay);
  }
  const parsed = new Date(dateStr);
  if (Number.isNaN(parsed.getTime())) return 0;
  return Math.floor((parsed.getTime() - todayUtc) / msPerDay);
}

function InvoiceRow({ inv }: { inv: Invoice }) {
  const outstanding = Math.max(inv.totalCents - inv.paidCents, 0);
  const d2d = daysUntil(inv.dueDate);
  const overdue = inv.status !== 'paid' && d2d < 0;
  return (
    <tr className="glass-table-row">
      <td className="glass-table-td tabular-nums">{formatDate(inv.invoiceDate)}</td>
      <td className="glass-table-td tabular-nums">
        {formatDate(inv.dueDate)}
        {inv.dueDate && inv.status !== 'paid' && (
          <span className={`ml-2 text-[11px] ${overdue ? 'text-[var(--danger)]' : 'text-[var(--text-muted)]'}`}>
            {overdue ? `${-d2d}d overdue` : d2d === 0 ? 'today' : `${d2d}d`}
          </span>
        )}
      </td>
      <td className="glass-table-td text-right tabular-nums">{formatCents(inv.totalCents)}</td>
      <td className="glass-table-td text-right text-[var(--text-muted)] tabular-nums">{formatCents(inv.paidCents)}</td>
      <td className="glass-table-td text-right tabular-nums font-semibold">
        {inv.status === 'paid' ? <span className="text-[var(--text-muted)]">—</span> : formatCents(outstanding)}
      </td>
      <td className="glass-table-td text-right text-[var(--text-muted)] tabular-nums">
        {inv.pendingReceiptCents > 0 ? formatCents(inv.pendingReceiptCents) : '—'}
      </td>
      <td className="glass-table-td">{statusPill(inv.status, d2d)}</td>
    </tr>
  );
}

function InvoiceCard({ inv }: { inv: Invoice }) {
  const outstanding = Math.max(inv.totalCents - inv.paidCents, 0);
  const d2d = daysUntil(inv.dueDate);
  const overdue = inv.status !== 'paid' && d2d < 0;
  return (
    <div className="p-4 border-b border-[var(--surface-2)] last:border-b-0">
      <div className="flex items-start justify-between gap-3 mb-2">
        <div>
          <div className="text-[10px] text-[var(--text-muted)] uppercase tracking-wider">Invoice</div>
          <div className="text-sm text-[var(--text)] tabular-nums">{formatDate(inv.invoiceDate)}</div>
        </div>
        {statusPill(inv.status, d2d)}
      </div>
      {inv.status !== 'paid' && (
        <div className="mb-2">
          <div className="text-[10px] text-[var(--text-muted)] uppercase tracking-wider">Outstanding</div>
          <div className="text-xl font-bold text-[var(--warning)] tabular-nums">{formatCents(outstanding)}</div>
          {inv.dueDate && (
            <div className={`text-[11px] ${overdue ? 'text-[var(--danger)]' : 'text-[var(--text-muted)]'}`}>
              due {formatDate(inv.dueDate)}
              {` · ${overdue ? `${-d2d}d overdue` : d2d === 0 ? 'today' : `${d2d}d`}`}
            </div>
          )}
        </div>
      )}
      <div className="grid grid-cols-3 gap-2 text-[11px]">
        <div>
          <div className="text-[10px] text-[var(--text-muted)] uppercase tracking-wider">Total</div>
          <div className="text-[var(--text)] tabular-nums">{formatCents(inv.totalCents)}</div>
        </div>
        <div>
          <div className="text-[10px] text-[var(--text-muted)] uppercase tracking-wider">Paid</div>
          <div className="text-[var(--text-muted)] tabular-nums">{formatCents(inv.paidCents)}</div>
        </div>
        <div>
          <div className="text-[10px] text-[var(--text-muted)] uppercase tracking-wider">Pending</div>
          <div className="text-[var(--text-muted)] tabular-nums">
            {inv.pendingReceiptCents > 0 ? formatCents(inv.pendingReceiptCents) : '—'}
          </div>
        </div>
      </div>
    </div>
  );
}

function InvoicesContent() {
  const { data: invoices = [], isLoading, isError, error } = useInvoices();

  if (isLoading) {
    return (
      <div className="flex justify-center py-16"><PokeballLoader /></div>
    );
  }

  if (isError) {
    return (
      <CardShell padding="lg">
        <p className="text-[var(--danger)] text-sm">
          Failed to load invoices: {error instanceof Error ? error.message : 'unknown error'}
        </p>
      </CardShell>
    );
  }

  if (invoices.length === 0) {
    return (
      <CardShell padding="lg">
        <div className="text-center py-8">
          <div className="text-sm text-[var(--text-muted)] mb-1">No invoices yet.</div>
          <div className="text-xs text-[var(--text-muted)]">
            Invoices appear after PSA imports are processed and billed.
          </div>
        </div>
      </CardShell>
    );
  }

  // Sort: unpaid/partial first (by due date asc), then paid (by paid date desc).
  const sorted = [...invoices].sort((a, b) => {
    const aOpen = a.status !== 'paid';
    const bOpen = b.status !== 'paid';
    if (aOpen !== bOpen) return aOpen ? -1 : 1;
    if (aOpen) {
      const ad = a.dueDate ?? a.invoiceDate;
      const bd = b.dueDate ?? b.invoiceDate;
      return ad.localeCompare(bd);
    }
    const ap = a.paidDate ?? a.invoiceDate;
    const bp = b.paidDate ?? b.invoiceDate;
    return bp.localeCompare(ap);
  });

  const openInvoices = invoices.filter(i => i.status !== 'paid');
  const outstandingTotal = openInvoices.reduce((sum, i) => sum + Math.max(i.totalCents - i.paidCents, 0), 0);
  const pendingReceiptTotal = openInvoices.reduce((sum, i) => sum + i.pendingReceiptCents, 0);

  return (
    <>
      <section
        className="flex flex-wrap items-end gap-x-7 gap-y-3 pb-6 mb-5 border-b border-[rgba(255,255,255,0.05)]"
        aria-label="Invoice summary"
      >
        <div className="flex flex-col gap-1.5">
          <div className="text-[11px] font-semibold uppercase tracking-[0.12em] text-[var(--brand-400)]">Outstanding</div>
          <div className="text-[clamp(28px,3.5vw,40px)] font-extrabold leading-none tabular-nums text-[var(--warning)]">
            {formatCents(outstandingTotal)}
          </div>
        </div>
        <div className="w-px self-stretch bg-[rgba(255,255,255,0.05)] my-1" aria-hidden />
        <div className="flex flex-wrap items-end gap-x-8 gap-y-3 pb-1">
          <div className="flex flex-col gap-1 min-w-[80px]">
            <div className="text-[10px] font-medium uppercase tracking-[0.08em] text-[var(--text-muted)]">Open invoices</div>
            <div className="text-lg font-bold tabular-nums text-[var(--text)]">{openInvoices.length}</div>
          </div>
          <div className="flex flex-col gap-1 min-w-[80px]">
            <div className="text-[10px] font-medium uppercase tracking-[0.08em] text-[var(--text-muted)]">Pending receipt</div>
            <div className="text-lg font-bold tabular-nums text-[var(--text)]">{formatCents(pendingReceiptTotal)}</div>
            <div className="text-[9px] text-[var(--text-muted)] tracking-[0.06em] opacity-75">cards still at PSA</div>
          </div>
        </div>
      </section>

      <CardShell padding="none">
        {/* Mobile: stacked cards — the 7-column table overflows <480px */}
        <div className="md:hidden">
          {sorted.map((inv) => <InvoiceCard key={inv.id} inv={inv} />)}
        </div>
        {/* Desktop: table */}
        <div className="hidden md:block overflow-x-auto">
          <div className="glass-table">
          <table className="w-full text-left">
            <thead>
              <tr className="glass-table-header">
                <th className="glass-table-th text-left">Invoice date</th>
                <th className="glass-table-th text-left">Due</th>
                <th className="glass-table-th text-right">Total</th>
                <th className="glass-table-th text-right">Paid</th>
                <th className="glass-table-th text-right">Outstanding</th>
                <th className="glass-table-th text-right">Pending receipt</th>
                <th className="glass-table-th text-left">Status</th>
              </tr>
            </thead>
            <tbody>
              {sorted.map((inv) => <InvoiceRow key={inv.id} inv={inv} />)}
            </tbody>
          </table>
          </div>
        </div>
      </CardShell>
    </>
  );
}

export default function InvoicesPage() {
  return (
    <div className="max-w-6xl mx-auto px-4">
      <div className="mb-5">
        <nav className="text-xs text-[var(--text-muted)] mb-1" aria-label="Breadcrumb">
          <Link to="/" className="hover:text-[var(--text)]">Dashboard</Link>
          <span className="mx-1.5">/</span>
          <span className="text-[var(--text)]">Invoices</span>
        </nav>
        <h1 className="text-[22px] font-bold text-[var(--text)] tracking-tight">Invoices</h1>
        <p className="mt-1 text-sm text-[var(--text-muted)]">
          PSA grading invoices and their payment status.
        </p>
      </div>

      <SectionErrorBoundary sectionName="Invoices">
        <InvoicesContent />
      </SectionErrorBoundary>
    </div>
  );
}
