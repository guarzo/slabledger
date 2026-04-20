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

function daysUntil(dateStr?: string): number {
  if (!dateStr) return 0;
  const d = new Date(dateStr);
  if (Number.isNaN(d.getTime())) return 0;
  const now = new Date();
  const msPerDay = 1000 * 60 * 60 * 24;
  return Math.round((d.getTime() - now.getTime()) / msPerDay);
}

function InvoiceRow({ inv }: { inv: Invoice }) {
  const outstanding = Math.max(inv.totalCents - inv.paidCents, 0);
  const d2d = daysUntil(inv.dueDate);
  const overdue = inv.status !== 'paid' && d2d < 0;
  return (
    <tr className="border-b border-[var(--surface-2)] last:border-b-0 hover:bg-[var(--surface-2)]/30">
      <td className="py-2.5 pr-4 text-sm text-[var(--text)] tabular-nums">{formatDate(inv.invoiceDate)}</td>
      <td className="py-2.5 pr-4 text-sm text-[var(--text)] tabular-nums">
        {formatDate(inv.dueDate)}
        {inv.dueDate && inv.status !== 'paid' && (
          <span className={`ml-2 text-[11px] ${overdue ? 'text-[var(--danger)]' : 'text-[var(--text-muted)]'}`}>
            {overdue ? `${-d2d}d overdue` : d2d === 0 ? 'today' : `${d2d}d`}
          </span>
        )}
      </td>
      <td className="py-2.5 pr-4 text-sm text-right text-[var(--text)] tabular-nums">{formatCents(inv.totalCents)}</td>
      <td className="py-2.5 pr-4 text-sm text-right text-[var(--text-muted)] tabular-nums">{formatCents(inv.paidCents)}</td>
      <td className="py-2.5 pr-4 text-sm text-right tabular-nums font-semibold text-[var(--text)]">
        {inv.status === 'paid' ? <span className="text-[var(--text-muted)]">—</span> : formatCents(outstanding)}
      </td>
      <td className="py-2.5 pr-4 text-sm text-right text-[var(--text-muted)] tabular-nums">
        {inv.pendingReceiptCents > 0 ? formatCents(inv.pendingReceiptCents) : '—'}
      </td>
      <td className="py-2.5">{statusPill(inv.status, d2d)}</td>
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
              {` · ${overdue ? `${-d2d}d overdue` : d2d === 0 ? 'due today' : `${d2d}d`}`}
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
      <div className="grid grid-cols-1 sm:grid-cols-3 gap-3 mb-5">
        <div className="rounded-xl bg-[var(--surface-1)] border border-[var(--surface-2)] p-4">
          <div className="text-[10px] text-[var(--text-muted)] uppercase tracking-wider">Open invoices</div>
          <div className="text-2xl font-bold text-[var(--text)] tabular-nums">{openInvoices.length}</div>
        </div>
        <div className="rounded-xl bg-[var(--surface-1)] border border-[var(--surface-2)] p-4">
          <div className="text-[10px] text-[var(--text-muted)] uppercase tracking-wider">Outstanding</div>
          <div className="text-2xl font-bold text-[var(--warning)] tabular-nums">{formatCents(outstandingTotal)}</div>
        </div>
        <div className="rounded-xl bg-[var(--surface-1)] border border-[var(--surface-2)] p-4">
          <div className="text-[10px] text-[var(--text-muted)] uppercase tracking-wider">Pending receipt</div>
          <div className="text-2xl font-bold text-[var(--text)] tabular-nums">{formatCents(pendingReceiptTotal)}</div>
          <div className="text-[11px] text-[var(--text-muted)] mt-1">cards still at PSA</div>
        </div>
      </div>

      <CardShell padding="none">
        {/* Mobile: stacked cards — the 7-column table overflows <480px */}
        <div className="md:hidden">
          {sorted.map((inv) => <InvoiceCard key={inv.id} inv={inv} />)}
        </div>
        {/* Desktop: table */}
        <div className="hidden md:block overflow-x-auto">
          <table className="w-full text-left">
            <thead>
              <tr className="text-[11px] font-semibold text-[var(--text-muted)] uppercase tracking-wider border-b border-[var(--surface-2)]">
                <th className="pl-4 py-2.5 pr-4">Invoice date</th>
                <th className="py-2.5 pr-4">Due</th>
                <th className="py-2.5 pr-4 text-right">Total</th>
                <th className="py-2.5 pr-4 text-right">Paid</th>
                <th className="py-2.5 pr-4 text-right">Outstanding</th>
                <th className="py-2.5 pr-4 text-right">Pending receipt</th>
                <th className="py-2.5 pr-4">Status</th>
              </tr>
            </thead>
            <tbody>
              {sorted.map((inv) => <InvoiceRow key={inv.id} inv={inv} />)}
            </tbody>
          </table>
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
