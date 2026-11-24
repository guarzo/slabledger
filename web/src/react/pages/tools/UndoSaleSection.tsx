import { useState } from 'react';
import { useQueryClient } from '@tanstack/react-query';
import { api } from '../../../js/api';
import { isAPIError } from '../../../js/api/client';
import type { SaleByCertResponse } from '../../../types/campaigns';
import { queryKeys } from '../../queries/queryKeys';
import { Button, CardShell, ConfirmDialog, Input, SectionErrorBoundary } from '../../ui';
import { formatCents, getErrorMessage } from '../../utils/formatters';
import { saleChannelLabels } from '../../utils/campaignConstants';

function channelLabel(channel: string): string {
  return saleChannelLabels[channel as keyof typeof saleChannelLabels] ?? channel;
}

export default function UndoSaleSection() {
  const queryClient = useQueryClient();
  const [cert, setCert] = useState('');
  const [looking, setLooking] = useState(false);
  const [found, setFound] = useState<SaleByCertResponse | null>(null);
  const [notFound, setNotFound] = useState(false);
  const [confirmOpen, setConfirmOpen] = useState(false);
  const [removing, setRemoving] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [success, setSuccess] = useState<string | null>(null);

  async function handleLookup(e?: React.FormEvent) {
    e?.preventDefault();
    const trimmed = cert.trim();
    if (!trimmed) return;
    setLooking(true);
    setFound(null);
    setNotFound(false);
    setError(null);
    setSuccess(null);
    try {
      setFound(await api.getSaleByCert(trimmed));
    } catch (err) {
      if (isAPIError(err) && err.status === 404) setNotFound(true);
      else setError(getErrorMessage(err, 'Lookup failed'));
    } finally {
      setLooking(false);
    }
  }

  async function handleUnsell() {
    if (!found?.sale) return;
    setRemoving(true);
    setError(null);
    try {
      await api.deleteSale(found.purchase.campaignId, found.purchase.id);
      setConfirmOpen(false);
      setFound(null);
      setCert('');
      setSuccess('Sale removed; card is back in inventory');
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: queryKeys.campaigns.all }),
        queryClient.invalidateQueries({ queryKey: queryKeys.portfolio.health }),
        queryClient.invalidateQueries({ queryKey: queryKeys.portfolio.globalInventory }),
        queryClient.invalidateQueries({ queryKey: queryKeys.portfolio.insights }),
        queryClient.invalidateQueries({ queryKey: queryKeys.portfolio.weeklyReview }),
      ]);
    } catch (err) {
      setConfirmOpen(false);
      setError(getErrorMessage(err, 'Failed to remove sale'));
    } finally {
      setRemoving(false);
    }
  }

  const sale = found?.sale ?? null;
  const card = found?.purchase;

  return (
    <SectionErrorBoundary sectionName="Undo a Sale">
      <CardShell variant="default" padding="lg">
        <form className="flex items-end gap-2" onSubmit={handleLookup}>
          <div className="flex-1">
            <Input
              label="PSA cert number"
              inputMode="numeric"
              value={cert}
              onChange={(e) => setCert(e.target.value)}
              placeholder="e.g. 162787413"
            />
          </div>
          <Button type="submit" variant="secondary" loading={looking} disabled={!cert.trim()}>
            Look up
          </Button>
        </form>

        {notFound && (
          <p className="mt-4 text-sm text-[var(--text-muted)]">No card with that cert in inventory.</p>
        )}
        {error && (
          <p role="alert" className="mt-4 text-sm text-[var(--danger)]">{error}</p>
        )}
        {success && (
          <p role="status" className="mt-4 text-sm text-[var(--success)]">{success}</p>
        )}

        {card && (
          <div className="mt-4 space-y-3">
            <div>
              <div className="text-sm font-semibold text-[var(--text)]">{card.cardName}</div>
              <div className="text-xs text-[var(--text-muted)]">
                {card.setName ? `${card.setName} · ` : ''}PSA {card.gradeValue} · Cert <span className="font-mono">{card.certNumber}</span>
              </div>
            </div>
            {sale ? (
              <>
                <dl className="grid grid-cols-3 gap-3 text-xs">
                  <div><dt className="text-[var(--text-muted)]">Channel</dt><dd className="text-[var(--text)]">{channelLabel(sale.saleChannel)}</dd></div>
                  <div><dt className="text-[var(--text-muted)]">Date</dt><dd className="text-[var(--text)]">{sale.saleDate}</dd></div>
                  <div><dt className="text-[var(--text-muted)]">Price</dt><dd className="text-[var(--text)] tabular-nums">{formatCents(sale.salePriceCents)}</dd></div>
                </dl>
                <Button variant="danger" size="sm" onClick={() => setConfirmOpen(true)}>Un-sell</Button>
              </>
            ) : (
              <p className="text-sm text-[var(--text-muted)]">This card isn't recorded as sold.</p>
            )}
          </div>
        )}
      </CardShell>

      <ConfirmDialog
        open={confirmOpen && !!sale && !!card}
        title="Un-sell card"
        message={sale && card
          ? `Un-sell ${card.cardName}? This deletes the ${channelLabel(sale.saleChannel)} sale of ${formatCents(sale.salePriceCents)} on ${sale.saleDate} and returns the card to inventory. If the sale was reported to DoubleHolo, it will be cancelled there too.`
          : ''}
        confirmLabel="Un-sell"
        variant="danger"
        loading={removing}
        onConfirm={handleUnsell}
        onCancel={() => setConfirmOpen(false)}
      />
    </SectionErrorBoundary>
  );
}
