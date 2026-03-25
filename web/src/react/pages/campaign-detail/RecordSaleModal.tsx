import { useState, useMemo } from 'react';
import { Dialog } from 'radix-ui';
import { useQueryClient } from '@tanstack/react-query';
import type { AgingItem, SaleChannel } from '../../../types/campaigns';
import { api } from '../../../js/api';
import { formatCents, localToday, getErrorMessage } from '../../utils/formatters';
import { saleChannelLabels, DEFAULT_SALE_CHANNEL } from '../../utils/campaignConstants';
import { useToast } from '../../contexts/ToastContext';
import { Button, Input, Select } from '../../ui';
import { queryKeys } from '../../queries/queryKeys';

interface RecordSaleModalProps {
  open: boolean;
  onClose: () => void;
  onSuccess?: () => void;
  items: AgingItem[];
}

function prefillPrice(item: AgingItem): number {
  return item.currentMarket?.medianCents || item.purchase.clValueCents || 0;
}

export default function RecordSaleModal({ open, onClose, onSuccess, items }: RecordSaleModalProps) {
  const toast = useToast();
  const queryClient = useQueryClient();
  const isSingle = items.length === 1;

  const [channel, setChannel] = useState<SaleChannel>(DEFAULT_SALE_CHANNEL);
  const [saleDate, setSaleDate] = useState(localToday());
  const [prices, setPrices] = useState<Record<string, number | undefined>>({});
  const [submitting, setSubmitting] = useState(false);
  const [originalListPrice, setOriginalListPrice] = useState('');
  const [priceReductions, setPriceReductions] = useState('');
  const [daysListed, setDaysListed] = useState('');
  const [soldAtAskingPrice, setSoldAtAskingPrice] = useState(false);
  const [showOutcomeFields, setShowOutcomeFields] = useState(false);

  // Initialize prices when items change
  const initialPrices = useMemo(() => {
    const p: Record<string, number> = {};
    for (const item of items) {
      p[item.purchase.id] = prefillPrice(item);
    }
    return p;
  }, [items]);

  // Use initialPrices as defaults, overlaid by user edits
  const effectivePrices = { ...initialPrices, ...prices };

  function handleClose() {
    if (submitting) return;
    setPrices({});
    setChannel(DEFAULT_SALE_CHANNEL);
    setSaleDate(localToday());
    onClose();
  }

  function resetAndClose() {
    setPrices({});
    setChannel(DEFAULT_SALE_CHANNEL);
    setSaleDate(localToday());
    setOriginalListPrice('');
    setPriceReductions('');
    setDaysListed('');
    setSoldAtAskingPrice(false);
    setShowOutcomeFields(false);
    onClose();
  }

  function gameStopWarnings(item: AgingItem): string[] {
    const warnings: string[] = [];
    if (channel !== 'gamestop') return warnings;
    const grade = item.purchase.gradeValue;
    const graderLabel = item.purchase.grader ?? 'PSA';
    if (graderLabel !== 'PSA')
      warnings.push(`GameStop estimates apply to PSA-graded slabs only (found ${graderLabel}).`);
    if (grade < 8)
      warnings.push(`GameStop typically requires grade 8 or higher (${graderLabel} ${grade}).`);
    if (item.purchase.clValueCents > 150000)
      warnings.push('GameStop CL limit is typically $1,500. Consider eBay auction.');
    return warnings;
  }

  function gameStopPayout(item: AgingItem): { min: number; max: number } | null {
    if (channel !== 'gamestop') return null;
    const isPSA = !item.purchase.grader || item.purchase.grader === 'PSA';
    if (!isPSA) return null;
    const cl = item.purchase.clValueCents;
    if (cl <= 0) return null;
    return { min: Math.round(cl * 0.70), max: Math.round(cl * 0.90) };
  }

  async function handleSubmit() {
    if (!saleDate || isNaN(new Date(saleDate).getTime())) {
      toast.error('Please select a sale date');
      return;
    }

    const invalidItems = items.filter(i => (effectivePrices[i.purchase.id] || 0) <= 0);
    if (invalidItems.length > 0) {
      toast.error(`${invalidItems.length} card(s) have no sale price set`);
      return;
    }

    setSubmitting(true);
    try {
      if (isSingle) {
        const item = items[0];
        await api.createSale(item.purchase.campaignId, {
          purchaseId: item.purchase.id,
          saleChannel: channel,
          salePriceCents: effectivePrices[item.purchase.id] ?? 0,
          saleDate,
          ...(originalListPrice ? { originalListPriceCents: Math.round(parseFloat(originalListPrice) * 100) } : {}),
          ...(priceReductions ? { priceReductions: parseInt(priceReductions, 10) } : {}),
          ...(daysListed ? { daysListed: parseInt(daysListed, 10) } : {}),
          ...(soldAtAskingPrice ? { soldAtAskingPrice: true } : {}),
        });
        toast.success('Sale recorded');
      } else {
        // Group by campaignId for bulk calls
        const groups = new Map<string, { purchaseId: string; salePriceCents: number }[]>();
        for (const item of items) {
          const cid = item.purchase.campaignId;
          if (!groups.has(cid)) groups.set(cid, []);
          groups.get(cid)!.push({
            purchaseId: item.purchase.id,
            salePriceCents: effectivePrices[item.purchase.id] ?? 0,
          });
        }

        const groupEntries = Array.from(groups.entries());
        const results = await Promise.allSettled(
          groupEntries.map(([cid, groupItems]) =>
            api.createBulkSales(cid, channel, saleDate, groupItems)
          )
        );

        let totalCreated = 0;
        let totalFailed = 0;
        for (let i = 0; i < results.length; i++) {
          const r = results[i];
          if (r.status === 'fulfilled') {
            totalCreated += r.value.created;
            totalFailed += r.value.failed;
            if (r.value.errors) {
              for (const err of r.value.errors.slice(0, 3)) {
                toast.error(`Failed: ${err.error}`);
              }
            }
          } else {
            totalFailed += groupEntries[i][1].length;
            toast.error(getErrorMessage(r.reason, 'Bulk sale failed'));
          }
        }
        if (totalCreated > 0) {
          toast.success(`${totalCreated} sale(s) recorded${totalFailed > 0 ? `, ${totalFailed} failed` : ''}`);
        } else {
          toast.error(`All ${totalFailed} sale(s) failed`);
          return; // Keep modal open on total failure
        }
      }

      // Invalidate caches for all affected campaigns + global
      const affectedCampaignIds = new Set(items.map(i => i.purchase.campaignId));
      for (const cid of affectedCampaignIds) {
        queryClient.invalidateQueries({ queryKey: queryKeys.campaigns.sales(cid) });
        queryClient.invalidateQueries({ queryKey: queryKeys.campaigns.purchases(cid) });
        queryClient.invalidateQueries({ queryKey: queryKeys.campaigns.pnl(cid) });
        queryClient.invalidateQueries({ queryKey: queryKeys.campaigns.inventory(cid) });
      }
      queryClient.invalidateQueries({ queryKey: queryKeys.portfolio.globalInventory });
      queryClient.invalidateQueries({ queryKey: queryKeys.portfolio.sellSheet });
      queryClient.invalidateQueries({ queryKey: queryKeys.portfolio.health });
      queryClient.invalidateQueries({ queryKey: queryKeys.portfolio.weeklyReview });
      queryClient.invalidateQueries({ queryKey: queryKeys.portfolio.capitalTimeline });
      queryClient.invalidateQueries({ queryKey: queryKeys.portfolio.channelVelocity });
      queryClient.invalidateQueries({ queryKey: queryKeys.portfolio.insights });
      queryClient.invalidateQueries({ queryKey: queryKeys.portfolio.suggestions });
      for (const cid of affectedCampaignIds) {
        queryClient.invalidateQueries({ queryKey: queryKeys.campaigns.channelPnl(cid) });
        queryClient.invalidateQueries({ queryKey: ['campaigns', cid, 'fillRate'] });
        queryClient.invalidateQueries({ queryKey: queryKeys.campaigns.daysToSell(cid) });
      }

      onSuccess?.();
      resetAndClose();
    } catch (err) {
      toast.error(getErrorMessage(err, 'Failed to record sale'));
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <Dialog.Root open={open} onOpenChange={(isOpen) => { if (!isOpen) handleClose(); }}>
      <Dialog.Portal>
        <Dialog.Overlay className="fixed inset-0 z-40 bg-black/50 data-[state=open]:animate-[fadeIn_150ms_ease-out]" />
        <Dialog.Content
          className="fixed left-1/2 top-1/2 z-50 -translate-x-1/2 -translate-y-1/2 bg-[var(--surface-1)] border border-[var(--surface-2)] rounded-xl p-6 max-w-lg w-[calc(100%-2rem)] shadow-xl data-[state=open]:animate-[scaleIn_150ms_ease-out] max-h-[85vh] overflow-y-auto"
        >
          <Dialog.Title className="text-lg font-semibold text-[var(--text)] mb-4">
            Record Sale{items.length > 1 ? ` (${items.length} cards)` : ''}
          </Dialog.Title>
          <Dialog.Description className="sr-only">
            Enter sale details including channel, date, and price
          </Dialog.Description>

          {/* Single item header */}
          {isSingle && items[0] && (
            <div className="mb-4 p-3 bg-[var(--surface-2)]/30 rounded-lg">
              <div className="text-sm font-medium text-[var(--text)]">{items[0].purchase.cardName}</div>
              <div className="text-xs text-[var(--text-muted)]">
                {items[0].purchase.grader ?? 'PSA'} {items[0].purchase.gradeValue} &middot; Cert #{items[0].purchase.certNumber}
                &middot; Cost: {formatCents(items[0].purchase.buyCostCents + items[0].purchase.psaSourcingFeeCents)}
              </div>
            </div>
          )}

          {/* Shared fields */}
          <div className="grid grid-cols-1 md:grid-cols-2 gap-3 mb-4">
            <div>
              <Select
                label="Channel"
                required
                selectSize="sm"
                value={channel}
                onChange={e => setChannel(e.target.value as SaleChannel)}
                options={(Object.entries(saleChannelLabels) as [SaleChannel, string][]).map(([val, label]) => ({ value: val, label }))}
              />
              {/* Single item GameStop payout estimate */}
              {isSingle && items[0] && (() => {
                const payout = gameStopPayout(items[0]);
                if (payout) {
                  return (
                    <div className="mt-1 text-xs">
                      <span className="text-[var(--text-muted)]">Est. payout: </span>
                      <span className="text-[var(--brand-500)] font-medium">{formatCents(payout.min)} - {formatCents(payout.max)}</span>
                      <span className="text-[var(--text-muted)]"> (70-90% of {formatCents(items[0].purchase.clValueCents)} CL)</span>
                    </div>
                  );
                }
                if (channel === 'gamestop') {
                  return <div className="mt-1 text-xs text-[var(--text-muted)]">GameStop: PSA 8+ only, max $1,500 CL</div>;
                }
                return null;
              })()}
            </div>
            <Input
              label="Sale Date"
              required
              type="date"
              inputSize="sm"
              value={saleDate}
              onChange={e => setSaleDate(e.target.value)}
            />
          </div>

          {/* GameStop warnings for single item */}
          {isSingle && items[0] && (() => {
            const warnings = gameStopWarnings(items[0]);
            return warnings.length > 0 ? (
              <div className="mb-4 p-2 rounded-lg bg-[var(--warning-bg)] border border-[var(--warning-border)] text-xs text-[var(--warning)]">
                {warnings.map((w, i) => <div key={i}>{w}</div>)}
              </div>
            ) : null;
          })()}

          {/* Price input(s) */}
          {isSingle && items[0] ? (
            <>
              <Input
                label="Sale Price ($)"
                required
                type="number"
                inputSize="sm"
                step="0.01"
                min="0"
                value={effectivePrices[items[0].purchase.id] == null ? '' : (effectivePrices[items[0].purchase.id] ?? 0) / 100}
                onChange={e => {
                  const raw = e.target.value;
                  setPrices(prev => ({
                    ...prev,
                    [items[0].purchase.id]: raw === '' ? undefined : Math.round(parseFloat(raw) * 100),
                  }));
                }}
              />
              {/* Sale outcome details (optional, collapsible) */}
              <button
                type="button"
                onClick={() => setShowOutcomeFields(!showOutcomeFields)}
                className="mt-2 text-xs text-[var(--text-muted)] hover:text-[var(--text)] transition-colors"
              >
                {showOutcomeFields ? 'Hide' : 'Add'} listing details {showOutcomeFields ? '▴' : '▾'}
              </button>
              {showOutcomeFields && (
                <div className="mt-2 grid grid-cols-2 gap-2 p-3 bg-[var(--surface-2)]/30 rounded-lg">
                  <Input
                    label="Original List Price ($)"
                    type="number"
                    inputSize="sm"
                    step="0.01"
                    min="0"
                    value={originalListPrice}
                    onChange={e => setOriginalListPrice(e.target.value)}
                    helper="Initial listing price"
                  />
                  <Input
                    label="Price Reductions"
                    type="number"
                    inputSize="sm"
                    min="0"
                    step="1"
                    value={priceReductions}
                    onChange={e => setPriceReductions(e.target.value)}
                    helper="Times price was lowered"
                  />
                  <Input
                    label="Days Listed"
                    type="number"
                    inputSize="sm"
                    min="0"
                    step="1"
                    value={daysListed}
                    onChange={e => setDaysListed(e.target.value)}
                    helper="Days on the platform"
                  />
                  <div className="flex items-end pb-1">
                    <label className="flex items-center gap-2 text-xs text-[var(--text-muted)] cursor-pointer">
                      <input
                        type="checkbox"
                        checked={soldAtAskingPrice}
                        onChange={e => setSoldAtAskingPrice(e.target.checked)}
                        className="rounded border-[var(--surface-2)]"
                      />
                      Sold at asking price
                    </label>
                  </div>
                </div>
              )}
            </>
          ) : (
            <div className="space-y-2 max-h-60 overflow-y-auto mb-4">
              {items.map(item => {
                const warnings = gameStopWarnings(item);
                return (
                  <div key={item.purchase.id} className="flex items-center gap-3 p-2 rounded-lg border border-[var(--surface-2)] bg-[var(--surface-2)]/20">
                    <div className="flex-1 min-w-0">
                      <div className="text-sm text-[var(--text)] truncate">{item.purchase.cardName}</div>
                      <div className="text-xs text-[var(--text-muted)]">
                        {item.purchase.grader ?? 'PSA'} {item.purchase.gradeValue} | Cost: {formatCents(item.purchase.buyCostCents + item.purchase.psaSourcingFeeCents)}
                        {item.purchase.clValueCents ? ` | CL: ${formatCents(item.purchase.clValueCents)}` : ''}
                        {item.campaignName ? ` | ${item.campaignName}` : ''}
                      </div>
                      {warnings.length > 0 && (
                        <div className="mt-1 text-[10px] text-[var(--warning)]">{warnings.join(' ')}</div>
                      )}
                    </div>
                    <div className="flex-shrink-0 w-28">
                      <input
                        type="number"
                        step="0.01"
                        min="0"
                        placeholder="Price $"
                        aria-label={`Sale price for ${item.purchase.cardName} ${item.purchase.grader ?? 'PSA'} ${item.purchase.gradeValue}`}
                        value={effectivePrices[item.purchase.id] == null ? '' : (effectivePrices[item.purchase.id] ?? 0) / 100}
                        onChange={e => {
                          const raw = e.target.value;
                          setPrices(prev => ({
                            ...prev,
                            [item.purchase.id]: raw === '' ? undefined : Math.round(parseFloat(raw) * 100),
                          }));
                        }}
                        className="w-full px-2 py-1 text-sm rounded bg-[var(--surface-2)] border border-[var(--surface-2)] text-[var(--text)] focus:outline-none focus:border-[var(--brand-500)]"
                      />
                    </div>
                  </div>
                );
              })}
            </div>
          )}

          <div className="flex justify-end gap-3 mt-6">
            <Dialog.Close asChild>
              <Button variant="ghost" size="sm" disabled={submitting}>
                Cancel
              </Button>
            </Dialog.Close>
            <Button size="sm" onClick={handleSubmit} loading={submitting}>
              {submitting ? 'Recording...' : `Record ${items.length > 1 ? `${items.length} Sales` : 'Sale'}`}
            </Button>
          </div>
        </Dialog.Content>
      </Dialog.Portal>
    </Dialog.Root>
  );
}
