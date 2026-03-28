import { useInvoices, useUpdateInvoice } from '../../queries/useCampaignQueries';
import { useToast } from '../../contexts/ToastContext';
import { formatCents, getErrorMessage } from '../../utils/formatters';
import { Button } from '../../ui';

export default function InvoicesTab() {
  const { data: invoices = [] } = useInvoices();
  const updateInvoice = useUpdateInvoice();
  const toast = useToast();

  if (invoices.length === 0) {
    return (
      <div className="text-center text-[var(--text-muted)] py-8 text-sm">
        No invoices yet. Invoices are created automatically during PSA imports.
      </div>
    );
  }

  function handleMarkPaid(id: string) {
    const inv = invoices.find(i => i.id === id);
    if (!inv) return;
    const now = new Date();
    const paidDate = `${now.getFullYear()}-${String(now.getMonth() + 1).padStart(2, '0')}-${String(now.getDate()).padStart(2, '0')}`;
    updateInvoice.mutate(
      { id, data: { ...inv, status: 'paid', paidDate, paidCents: inv.totalCents } },
      {
        onSuccess: () => toast.success('Invoice marked as paid'),
        onError: (err) => toast.error(getErrorMessage(err, 'Failed to update invoice')),
      },
    );
  }

  const statusBadge = (status: string) => {
    if (status === 'paid') return 'bg-[var(--success-bg)] text-[var(--success)]';
    if (status === 'partial') return 'bg-[var(--warning-bg)] text-[var(--warning)]';
    return 'bg-[var(--danger-bg)] text-[var(--danger)]';
  };

  return (
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
                    loading={updateInvoice.isPending}
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
  );
}
