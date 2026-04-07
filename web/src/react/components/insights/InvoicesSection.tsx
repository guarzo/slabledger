import { useState } from 'react';
import { useInvoices, useUpdateInvoice } from '../../queries/useCampaignQueries';
import { useToast } from '../../contexts/ToastContext';
import { formatCents, getErrorMessage, localToday } from '../../utils/formatters';
import { Button, CardShell } from '../../ui';

export default function InvoicesSection() {
  const { data: invoices = [], isLoading, error } = useInvoices();
  const updateInvoice = useUpdateInvoice();
  const toast = useToast();
  const [updatingId, setUpdatingId] = useState<string | null>(null);

  if (isLoading) {
    return (
      <CardShell padding="lg">
        <div className="text-center text-[var(--text-muted)] py-8 text-sm">
          Loading invoices...
        </div>
      </CardShell>
    );
  }

  if (error) {
    return (
      <CardShell padding="lg">
        <div className="text-center text-[var(--danger)] py-8 text-sm">
          Failed to load invoices. Please try again.
        </div>
      </CardShell>
    );
  }

  function handleMarkPaid(id: string) {
    const inv = invoices.find(i => i.id === id);
    if (!inv) {
      toast.error('Invoice not found. Please refresh the page.');
      return;
    }
    setUpdatingId(id);
    const paidDate = localToday();
    updateInvoice.mutate(
      { id, data: { status: 'paid', paidDate, paidCents: inv.totalCents } },
      {
        onSuccess: () => { setUpdatingId(null); toast.success('Invoice marked as paid'); },
        onError: (err) => { setUpdatingId(null); toast.error(getErrorMessage(err, 'Failed to update invoice')); },
      },
    );
  }

  const statusBadge = (status: string) => {
    if (status === 'paid') return 'bg-[var(--success-bg)] text-[var(--success)]';
    if (status === 'partial') return 'bg-[var(--warning-bg)] text-[var(--warning)]';
    return 'bg-[var(--danger-bg)] text-[var(--danger)]';
  };

  return (
    <CardShell padding="lg">
      <div className="mb-4">
        <h2 className="text-base font-semibold text-[var(--text)]">PSA Invoices</h2>
        <p className="text-xs text-[var(--text-muted)] mt-0.5">Payment tracking for PSA submissions</p>
      </div>

      {invoices.length === 0 ? (
        <p className="text-xs text-[var(--text-muted)] py-1">
          No invoices yet — created automatically on PSA import.
        </p>
      ) : (
        <div className="overflow-x-auto">
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b border-[var(--surface-2)]">
                <th className="text-left py-2 px-3 text-[var(--text-muted)] font-medium text-xs">Date</th>
                <th className="text-right py-2 px-3 text-[var(--text-muted)] font-medium text-xs">Total</th>
                <th className="text-right py-2 px-3 text-[var(--text-muted)] font-medium text-xs">Paid</th>
                <th className="text-center py-2 px-3 text-[var(--text-muted)] font-medium text-xs">Status</th>
                <th className="text-right py-2 px-3 text-[var(--text-muted)] font-medium text-xs">Due</th>
                <th className="py-2 px-3"></th>
              </tr>
            </thead>
            <tbody>
              {invoices.map(inv => (
                <tr key={inv.id} className="border-b border-[var(--surface-2)]/50">
                  <td className="py-2 px-3 text-xs text-[var(--text)]">{inv.invoiceDate}</td>
                  <td className="py-2 px-3 text-xs text-right text-[var(--text)]">{formatCents(inv.totalCents)}</td>
                  <td className="py-2 px-3 text-xs text-right text-[var(--text)]">{formatCents(inv.paidCents)}</td>
                  <td className="py-2 px-3 text-xs text-center">
                    <span className={`px-2 py-0.5 rounded-full text-xs font-medium ${statusBadge(inv.status)}`}>
                      {inv.status}
                    </span>
                  </td>
                  <td className="py-2 px-3 text-xs text-right text-[var(--text-muted)]">{inv.dueDate || '-'}</td>
                  <td className="py-2 px-3 text-right">
                    {inv.status !== 'paid' && (
                      <Button
                        size="sm"
                        variant="success"
                        loading={updateInvoice.isPending && updatingId === inv.id}
                        disabled={updateInvoice.isPending}
                        onClick={() => handleMarkPaid(inv.id)}
                      >
                        Mark Paid
                      </Button>
                    )}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </CardShell>
  );
}
