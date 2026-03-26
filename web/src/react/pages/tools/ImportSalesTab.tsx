import { useRef, useState } from 'react';
import { useQueryClient } from '@tanstack/react-query';
import { api } from '../../../js/api';
import type { OrdersImportResult, OrdersImportSkip, BulkSaleResult } from '../../../types/campaigns';
import { queryKeys } from '../../queries/queryKeys';
import { useToast } from '../../contexts/ToastContext';
import { Button, CardShell } from '../../ui';
import { formatCents, getErrorMessage } from '../../utils/formatters';

type Phase = 'upload' | 'review' | 'confirming';

export default function ImportSalesTab() {
  const toast = useToast();
  const queryClient = useQueryClient();
  const fileRef = useRef<HTMLInputElement>(null);

  const [phase, setPhase] = useState<Phase>('upload');
  const [loading, setLoading] = useState(false);
  const [result, setResult] = useState<OrdersImportResult | null>(null);
  const [selected, setSelected] = useState<Set<string>>(new Set());
  const [confirmResult, setConfirmResult] = useState<BulkSaleResult | null>(null);

  async function handleUpload(file: File) {
    try {
      setLoading(true);
      setResult(null);
      setConfirmResult(null);
      const res = await api.importOrdersSales(file);
      setResult(res);
      setSelected(new Set(res.matched.map(m => m.purchaseId)));
      setPhase('review');
    } catch (err) {
      toast.error(getErrorMessage(err, 'Failed to parse orders CSV'));
    } finally {
      setLoading(false);
    }
  }

  async function handleConfirm() {
    if (!result) return;
    const items = result.matched
      .filter(m => selected.has(m.purchaseId))
      .map(m => ({
        purchaseId: m.purchaseId,
        saleChannel: m.saleChannel,
        saleDate: m.saleDate,
        salePriceCents: m.salePriceCents,
      }));

    if (items.length === 0) {
      toast.error('No items selected');
      return;
    }

    try {
      setPhase('confirming');
      const res = await api.confirmOrdersSales(items);
      setConfirmResult(res);
      toast.success(`${res.created} sales created${res.failed > 0 ? `, ${res.failed} failed` : ''}`);

      queryClient.invalidateQueries({ queryKey: queryKeys.campaigns.all });
      queryClient.invalidateQueries({ queryKey: queryKeys.portfolio.health });
      queryClient.invalidateQueries({ queryKey: queryKeys.portfolio.globalInventory });
      queryClient.invalidateQueries({ queryKey: queryKeys.portfolio.sellSheet });
      queryClient.invalidateQueries({ queryKey: queryKeys.portfolio.insights });
      queryClient.invalidateQueries({ queryKey: queryKeys.portfolio.capitalTimeline });
      queryClient.invalidateQueries({ queryKey: queryKeys.portfolio.weeklyReview });

      setPhase('upload');
      setResult(null);
      setSelected(new Set());
    } catch (err) {
      toast.error(getErrorMessage(err, 'Failed to confirm sales'));
      setPhase('review');
    }
  }

  function handleReset() {
    setPhase('upload');
    setResult(null);
    setSelected(new Set());
    setConfirmResult(null);
  }

  function toggleSelect(purchaseId: string) {
    setSelected(prev => {
      const next = new Set(prev);
      if (next.has(purchaseId)) next.delete(purchaseId);
      else next.add(purchaseId);
      return next;
    });
  }

  function toggleAll() {
    if (!result) return;
    if (selected.size === result.matched.length) {
      setSelected(new Set());
    } else {
      setSelected(new Set(result.matched.map(m => m.purchaseId)));
    }
  }

  const channelLabel = (ch: string) => {
    switch (ch) {
      case 'ebay': return 'eBay';
      case 'website': return 'Website';
      default: return ch;
    }
  };

  if (phase === 'upload') {
    return (
      <div className="space-y-4">
        <div className="mb-4">
          <h2 className="text-base font-semibold text-[var(--text)]">Import Sales from Orders</h2>
          <p className="text-xs text-[var(--text-muted)] mt-0.5">
            Upload an orders export CSV to match sales against your inventory by PSA cert number.
            Only PSA-graded cards with cert numbers will be processed.
          </p>
        </div>

        <CardShell variant="default" padding="lg">
          <div className="flex flex-col items-center text-center gap-4 py-4">
            <div className="w-12 h-12 rounded-full bg-[var(--brand-500)]/15 flex items-center justify-center">
              <svg width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" className="text-[var(--brand-500)]" aria-hidden="true">
                <path d="M21 15v4a2 2 0 01-2 2H5a2 2 0 01-2-2v-4" />
                <polyline points="17 8 12 3 7 8" />
                <line x1="12" y1="3" x2="12" y2="15" />
              </svg>
            </div>
            <div>
              <div className="text-sm font-semibold text-[var(--text)]">Upload Orders CSV</div>
              <div className="text-xs text-[var(--text-muted)] mt-1">
                Expects columns: Order, Date, Sales Channel, Product Title, Grading Company, Cert Number, Grade, Qty, Unit Price, Line Subtotal
              </div>
            </div>
            <Button
              size="md"
              variant="primary"
              loading={loading}
              onClick={() => fileRef.current?.click()}
            >
              Choose File
            </Button>
            <input
              ref={fileRef}
              type="file"
              accept=".csv"
              className="hidden"
              onChange={(e) => {
                const file = e.target.files?.[0];
                if (file) handleUpload(file);
                e.target.value = '';
              }}
            />
          </div>
        </CardShell>

        {confirmResult && (
          <div className="p-3 rounded-lg bg-[var(--success-bg)]/30 text-sm">
            <span className="text-[var(--success)] font-medium">{confirmResult.created} sales created</span>
            {confirmResult.failed > 0 && (
              <span className="text-[var(--danger)] ml-2">{confirmResult.failed} failed</span>
            )}
          </div>
        )}
      </div>
    );
  }

  if (!result) return null;

  const matchedCount = result.matched.length;
  const alreadySoldCount = result.alreadySold.length;
  const notFoundCount = result.notFound.length;
  const skippedCount = result.skipped.length;
  const selectedCount = selected.size;

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <div>
          <h2 className="text-base font-semibold text-[var(--text)]">Review Import</h2>
          <div className="flex flex-wrap gap-3 text-xs mt-1">
            {matchedCount > 0 && <span className="text-[var(--success)]">{matchedCount} matched</span>}
            {alreadySoldCount > 0 && <span className="text-[var(--warning)]">{alreadySoldCount} already sold</span>}
            {notFoundCount > 0 && <span className="text-orange-400">{notFoundCount} not found</span>}
            {skippedCount > 0 && <span className="text-[var(--text-muted)]">{skippedCount} skipped</span>}
          </div>
        </div>
        <Button size="sm" variant="ghost" onClick={handleReset}>
          Start Over
        </Button>
      </div>

      {matchedCount > 0 && (
        <CardShell variant="default" padding="none">
          <div className="overflow-x-auto">
            <table className="w-full text-sm">
              <thead>
                <tr className="border-b border-[var(--surface-2)]">
                  <th className="py-2 px-3 text-left">
                    <input
                      type="checkbox"
                      checked={selectedCount === matchedCount}
                      onChange={toggleAll}
                      className="accent-[var(--brand-500)]"
                    />
                  </th>
                  <th className="py-2 px-3 text-left text-xs text-[var(--text-muted)] font-medium">Card</th>
                  <th className="py-2 px-3 text-left text-xs text-[var(--text-muted)] font-medium">Cert #</th>
                  <th className="py-2 px-3 text-left text-xs text-[var(--text-muted)] font-medium">Channel</th>
                  <th className="py-2 px-3 text-left text-xs text-[var(--text-muted)] font-medium">Date</th>
                  <th className="py-2 px-3 text-right text-xs text-[var(--text-muted)] font-medium">Sale Price</th>
                  <th className="py-2 px-3 text-right text-xs text-[var(--text-muted)] font-medium">Fee</th>
                  <th className="py-2 px-3 text-right text-xs text-[var(--text-muted)] font-medium">Cost</th>
                  <th className="py-2 px-3 text-right text-xs text-[var(--text-muted)] font-medium">Net Profit</th>
                </tr>
              </thead>
              <tbody>
                {result.matched.map((m) => (
                  <tr key={m.purchaseId} className="border-b border-[var(--surface-2)]/50 hover:bg-[var(--surface-1)]/50">
                    <td className="py-2 px-3">
                      <input
                        type="checkbox"
                        checked={selected.has(m.purchaseId)}
                        onChange={() => toggleSelect(m.purchaseId)}
                        className="accent-[var(--brand-500)]"
                      />
                    </td>
                    <td className="py-2 px-3 text-xs text-[var(--text)]">
                      <div className="font-medium">{m.cardName}</div>
                      <div className="text-[var(--text-muted)] text-[10px]">{m.productTitle}</div>
                    </td>
                    <td className="py-2 px-3 text-xs text-[var(--text-muted)] font-mono">{m.certNumber}</td>
                    <td className="py-2 px-3 text-xs text-[var(--text)]">{channelLabel(m.saleChannel)}</td>
                    <td className="py-2 px-3 text-xs text-[var(--text-muted)]">{m.saleDate}</td>
                    <td className="py-2 px-3 text-xs text-right text-[var(--text)]">{formatCents(m.salePriceCents)}</td>
                    <td className="py-2 px-3 text-xs text-right text-[var(--text-muted)]">{formatCents(m.saleFeeCents)}</td>
                    <td className="py-2 px-3 text-xs text-right text-[var(--text-muted)]">{formatCents(m.buyCostCents)}</td>
                    <td className={`py-2 px-3 text-xs text-right font-medium ${m.netProfitCents >= 0 ? 'text-[var(--success)]' : 'text-[var(--danger)]'}`}>
                      {m.netProfitCents >= 0 ? '+' : ''}{formatCents(m.netProfitCents)}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
          <div className="flex items-center justify-between p-3 border-t border-[var(--surface-2)]">
            <span className="text-xs text-[var(--text-muted)]">{selectedCount} of {matchedCount} selected</span>
            <Button
              size="sm"
              variant="primary"
              loading={phase === 'confirming'}
              disabled={selectedCount === 0}
              onClick={handleConfirm}
            >
              Confirm {selectedCount} Sale{selectedCount !== 1 ? 's' : ''}
            </Button>
          </div>
        </CardShell>
      )}

      <SkipSection title="Already Sold" items={result.alreadySold} />
      <SkipSection title="Not Found in Inventory" items={result.notFound} />
      <SkipSection title="Skipped (CGC/Ungraded/Duplicate)" items={result.skipped} />
    </div>
  );
}

function SkipSection({ title, items }: { title: string; items: OrdersImportSkip[] }) {
  const [open, setOpen] = useState(false);

  if (items.length === 0) return null;

  return (
    <div className="border border-[var(--surface-2)] rounded-lg">
      <button
        type="button"
        onClick={() => setOpen(!open)}
        className="w-full flex items-center justify-between p-3 text-xs text-[var(--text-muted)] hover:text-[var(--text)]"
      >
        <span>{title} ({items.length})</span>
        <svg
          width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2"
          className={`transform transition-transform ${open ? 'rotate-180' : ''}`}
          aria-hidden="true"
        >
          <polyline points="6 9 12 15 18 9" />
        </svg>
      </button>
      {open && (
        <div className="px-3 pb-3">
          <table className="w-full text-xs">
            <thead>
              <tr className="border-b border-[var(--surface-2)]">
                <th className="py-1 text-left text-[var(--text-muted)] font-medium">Cert #</th>
                <th className="py-1 text-left text-[var(--text-muted)] font-medium">Product Title</th>
                <th className="py-1 text-left text-[var(--text-muted)] font-medium">Reason</th>
              </tr>
            </thead>
            <tbody>
              {items.map((item, idx) => (
                <tr key={`${item.certNumber}-${idx}`} className="border-b border-[var(--surface-2)]/30">
                  <td className="py-1 text-[var(--text-muted)] font-mono">{item.certNumber || '—'}</td>
                  <td className="py-1 text-[var(--text)]">{item.productTitle}</td>
                  <td className="py-1 text-[var(--text-muted)]">{item.reason}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </div>
  );
}
