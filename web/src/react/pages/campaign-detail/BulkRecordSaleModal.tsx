import { useMemo, useState } from 'react';
import { Dialog } from 'radix-ui';
import { useQueryClient } from '@tanstack/react-query';
import type { AgingItem, SaleChannel } from '../../../types/campaigns';
import { api } from '../../../js/api';
import { formatCents, localToday, getErrorMessage } from '../../utils/formatters';
import { saleChannelLabels, DEFAULT_SALE_CHANNEL, activeSaleChannels } from '../../utils/campaignConstants';
import { useToast } from '../../contexts/ToastContext';
import { Button, Input, Select } from '../../ui';
import { queryKeys } from '../../queries/queryKeys';
import { costBasis } from './inventory/utils';
import { computeSalePrice, type PricingMode } from './saleModal/pricingModes';

interface Props {
  open: boolean;
  onClose: () => void;
  onSuccess?: () => void;
  items: AgingItem[];
}

export default function BulkRecordSaleModal({ open, onClose, onSuccess, items }: Props) {
  const toast = useToast();
  const queryClient = useQueryClient();

  const [channel, setChannel] = useState<SaleChannel>(DEFAULT_SALE_CHANNEL);
  const [saleDate, setSaleDate] = useState(localToday());
  const [pricingMode, setPricingMode] = useState<PricingMode>('pctOfCL');
  const [fillValue, setFillValue] = useState<number>(0);
  const [submitting, setSubmitting] = useState(false);

  const computedPrices = useMemo(() => {
    const m: Record<string, number> = {};
    for (const item of items) {
      m[item.purchase.id] = computeSalePrice(item, pricingMode, fillValue);
    }
    return m;
  }, [items, pricingMode, fillValue]);

  function reset() {
    setChannel(DEFAULT_SALE_CHANNEL);
    setSaleDate(localToday());
    setPricingMode('pctOfCL');
    setFillValue(0);
  }

  function handleClose() {
    if (submitting) return;
    reset();
    onClose();
  }

  async function handleSubmit() {
    if (!saleDate || isNaN(new Date(saleDate).getTime())) {
      toast.error('Please select a sale date');
      return;
    }

    const invalid = items.filter(i => (computedPrices[i.purchase.id] || 0) <= 0);
    if (invalid.length > 0) {
      toast.error(`${invalid.length} card(s) have no sale price set`);
      return;
    }

    setSubmitting(true);
    try {
      const groups = new Map<string, { purchaseId: string; salePriceCents: number }[]>();
      for (const item of items) {
        const cid = item.purchase.campaignId;
        if (!groups.has(cid)) groups.set(cid, []);
        groups.get(cid)!.push({
          purchaseId: item.purchase.id,
          salePriceCents: computedPrices[item.purchase.id] ?? 0,
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
        return;
      }

      // Invalidate caches for affected campaigns
      const affected = new Set(items.map(i => i.purchase.campaignId));
      for (const cid of affected) {
        queryClient.invalidateQueries({ queryKey: queryKeys.campaigns.sales(cid) });
        queryClient.invalidateQueries({ queryKey: queryKeys.campaigns.purchases(cid) });
        queryClient.invalidateQueries({ queryKey: queryKeys.campaigns.pnl(cid) });
        queryClient.invalidateQueries({ queryKey: queryKeys.campaigns.inventory(cid) });
        queryClient.invalidateQueries({ queryKey: queryKeys.campaigns.channelPnl(cid) });
        queryClient.invalidateQueries({ queryKey: ['campaigns', cid, 'fillRate'] });
        queryClient.invalidateQueries({ queryKey: queryKeys.campaigns.daysToSell(cid) });
      }
      queryClient.invalidateQueries({ queryKey: queryKeys.portfolio.globalInventory });
      queryClient.invalidateQueries({ queryKey: queryKeys.portfolio.sellSheet });
      queryClient.invalidateQueries({ queryKey: queryKeys.portfolio.health });
      queryClient.invalidateQueries({ queryKey: queryKeys.portfolio.weeklyReview });
      queryClient.invalidateQueries({ queryKey: queryKeys.portfolio.channelVelocity });
      queryClient.invalidateQueries({ queryKey: queryKeys.portfolio.insights });
      queryClient.invalidateQueries({ queryKey: queryKeys.portfolio.suggestions });

      onSuccess?.();
      reset();
      onClose();
    } catch (err) {
      toast.error(getErrorMessage(err, 'Failed to record sales'));
    } finally {
      setSubmitting(false);
    }
  }

  const fillInputLabel = pricingMode === 'pctOfCL' ? '% of CL' : 'Flat $';

  return (
    <Dialog.Root open={open} onOpenChange={(isOpen) => { if (!isOpen) handleClose(); }}>
      <Dialog.Portal>
        <Dialog.Overlay className="fixed inset-0 z-40 bg-black/50 data-[state=open]:animate-[fadeIn_150ms_ease-out]" />
        <Dialog.Content
          className="fixed left-1/2 top-1/2 z-50 -translate-x-1/2 -translate-y-1/2 bg-[var(--surface-1)] border border-[var(--surface-2)] rounded-xl p-6 max-w-lg w-[calc(100%-2rem)] shadow-xl data-[state=open]:animate-[scaleIn_150ms_ease-out] max-h-[85vh] overflow-y-auto"
        >
          <Dialog.Title className="text-lg font-semibold text-[var(--text)] mb-4">
            Record Sale ({items.length} cards)
          </Dialog.Title>
          <Dialog.Description className="sr-only">
            Enter sale details for multiple cards
          </Dialog.Description>

          <div className="grid grid-cols-1 md:grid-cols-2 gap-3 mb-4">
            <Select
              label="Channel"
              required
              selectSize="sm"
              value={channel}
              onChange={e => setChannel(e.target.value as SaleChannel)}
              options={activeSaleChannels.map(ch => ({ value: ch, label: saleChannelLabels[ch] }))}
            />
            <Input
              label="Sale Date"
              required
              type="date"
              inputSize="sm"
              value={saleDate}
              onChange={e => setSaleDate(e.target.value)}
            />
          </div>

          <div className="mb-4">
            <div className="flex items-center gap-4 mb-2">
              <label className="flex items-center gap-2 text-sm">
                <input
                  type="radio"
                  name="pricing-mode"
                  checked={pricingMode === 'pctOfCL'}
                  onChange={() => setPricingMode('pctOfCL')}
                />
                Percent of CL
              </label>
              <label className="flex items-center gap-2 text-sm">
                <input
                  type="radio"
                  name="pricing-mode"
                  checked={pricingMode === 'flat'}
                  onChange={() => setPricingMode('flat')}
                />
                Fixed price
              </label>
            </div>
            <Input
              id="fill-value"
              label={fillInputLabel}
              type="number"
              inputSize="sm"
              min="0"
              step={pricingMode === 'pctOfCL' ? '1' : '0.01'}
              value={pricingMode === 'pctOfCL' ? (fillValue || '') : (fillValue ? fillValue / 100 : '')}
              onChange={e => {
                const raw = e.target.value;
                if (raw === '') { setFillValue(0); return; }
                const n = parseFloat(raw);
                if (Number.isNaN(n)) { setFillValue(0); return; }
                setFillValue(pricingMode === 'pctOfCL' ? n : Math.round(n * 100));
              }}
            />
          </div>

          <div className="space-y-2 max-h-60 overflow-y-auto mb-4">
            {items.map(item => (
              <div key={item.purchase.id} className="flex items-center gap-3 p-2 rounded-lg border border-[var(--surface-2)] bg-[var(--surface-2)]/20">
                <div className="flex-1 min-w-0">
                  <div className="text-sm text-[var(--text)] truncate">{item.purchase.cardName}</div>
                  <div className="text-xs text-[var(--text-muted)]">
                    {item.purchase.grader ?? 'PSA'} {item.purchase.gradeValue} | Cost: <span className="tabular-nums">{formatCents(costBasis(item.purchase))}</span>
                    {item.purchase.clValueCents ? <> | CL: <span className="tabular-nums">{formatCents(item.purchase.clValueCents)}</span></> : ''}
                    {item.campaignName ? ` | ${item.campaignName}` : ''}
                  </div>
                </div>
                <div className="flex-shrink-0 w-28 text-right text-sm tabular-nums">
                  {formatCents(computedPrices[item.purchase.id] ?? 0)}
                </div>
              </div>
            ))}
          </div>

          <div className="flex justify-end gap-3 mt-6">
            <Dialog.Close asChild>
              <Button variant="ghost" size="sm" disabled={submitting}>Cancel</Button>
            </Dialog.Close>
            <Button size="sm" onClick={handleSubmit} loading={submitting}>
              {submitting ? 'Recording...' : `Record ${items.length} Sales`}
            </Button>
          </div>
        </Dialog.Content>
      </Dialog.Portal>
    </Dialog.Root>
  );
}
