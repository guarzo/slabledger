import { useState, useMemo } from 'react';
import { Dialog } from 'radix-ui';
import { useQueryClient } from '@tanstack/react-query';
import type { AgingItem, SaleChannel } from '../../../types/campaigns';
import { api } from '../../../js/api';
import { formatCents, localToday, getErrorMessage, dollarsToCents } from '../../utils/formatters';
import { saleChannelLabels, DEFAULT_SALE_CHANNEL, activeSaleChannels } from '../../utils/campaignConstants';
import { useToast } from '../../contexts/ToastContext';
import { Button, Input, Select } from '../../ui';
import { costBasis } from './inventory/utils';
import { invalidateAfterSale } from './saleModal/invalidateAfterSale';

interface RecordSaleModalProps {
  open: boolean;
  onClose: () => void;
  onSuccess?: () => void;
  items: [AgingItem];
}

function prefillPrice(item: AgingItem): number {
  return item.currentMarket?.medianCents || item.purchase.clValueCents || 0;
}

export default function RecordSaleModal({ open, onClose, onSuccess, items }: RecordSaleModalProps) {
  const toast = useToast();
  const queryClient = useQueryClient();
  const item = items[0];

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
    return { [item.purchase.id]: prefillPrice(item) };
  }, [item]);

  // Use initialPrices as defaults, overlaid by user edits
  const effectivePrices = { ...initialPrices, ...prices };

  function resetState() {
    setPrices({});
    setChannel(DEFAULT_SALE_CHANNEL);
    setSaleDate(localToday());
    setOriginalListPrice('');
    setPriceReductions('');
    setDaysListed('');
    setSoldAtAskingPrice(false);
    setShowOutcomeFields(false);
  }

  function handleClose() {
    if (submitting) return;
    resetState();
    onClose();
  }

  async function handleSubmit() {
    if (!saleDate || isNaN(new Date(saleDate).getTime())) {
      toast.error('Please select a sale date');
      return;
    }

    if ((effectivePrices[item.purchase.id] || 0) <= 0) {
      toast.error('Please set a sale price');
      return;
    }

    setSubmitting(true);
    try {
      await api.createSale(item.purchase.campaignId, {
        purchaseId: item.purchase.id,
        saleChannel: channel,
        salePriceCents: effectivePrices[item.purchase.id] ?? 0,
        saleDate,
        ...(originalListPrice ? { originalListPriceCents: dollarsToCents(originalListPrice) } : {}),
        ...(priceReductions ? { priceReductions: parseInt(priceReductions, 10) || 0 } : {}),
        ...(daysListed ? { daysListed: parseInt(daysListed, 10) || 0 } : {}),
        ...(soldAtAskingPrice ? { soldAtAskingPrice: true } : {}),
      });
      toast.success('Sale recorded');

      invalidateAfterSale(queryClient, [item.purchase.campaignId]);

      onSuccess?.();
      resetState();
      onClose();
    } catch (err) {
      toast.error(getErrorMessage(err, 'Failed to record sale'));
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <Dialog.Root open={open} onOpenChange={(isOpen) => { if (!isOpen) handleClose(); }}>
      <Dialog.Portal>
        <Dialog.Overlay className="fixed inset-0 z-40 bg-[var(--surface-overlay)] data-[state=open]:animate-[fadeIn_150ms_ease-out]" />
        <Dialog.Content
          className="fixed left-1/2 top-1/2 z-50 -translate-x-1/2 -translate-y-1/2 bg-[var(--surface-1)] border border-[var(--surface-2)] rounded-xl p-6 max-w-lg w-[calc(100%-2rem)] shadow-xl data-[state=open]:animate-[scaleIn_150ms_ease-out] max-h-[85vh] overflow-y-auto"
        >
          <Dialog.Title className="text-lg font-semibold text-[var(--text)] mb-4">
            Record Sale
          </Dialog.Title>
          <Dialog.Description className="sr-only">
            Enter sale details including channel, date, and price
          </Dialog.Description>

          {/* Item header */}
          <div className="mb-4 p-3 bg-[var(--surface-2)]/30 rounded-lg">
            <div className="text-sm font-medium text-[var(--text)]">{item.purchase.cardName}</div>
            <div className="text-xs text-[var(--text-muted)]">
              {item.purchase.grader ?? 'PSA'} {item.purchase.gradeValue} &middot; Cert #{item.purchase.certNumber}
              &middot; Cost: <span className="tabular-nums">{formatCents(costBasis(item.purchase))}</span>
            </div>
          </div>

          {/* Shared fields */}
          <div className="grid grid-cols-1 md:grid-cols-2 gap-3 mb-4">
            <div>
              <Select
                label="Channel"
                required
                selectSize="sm"
                value={channel}
                onChange={e => setChannel(e.target.value as SaleChannel)}
                options={activeSaleChannels.map(ch => ({ value: ch, label: saleChannelLabels[ch] }))}
              />
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

          {/* Price input */}
          <Input
            label="Sale Price ($)"
            required
            type="number"
            inputSize="sm"
            step="0.01"
            min="0"
            value={effectivePrices[item.purchase.id] == null ? '' : (effectivePrices[item.purchase.id] ?? 0) / 100}
            onChange={e => {
              const raw = e.target.value;
              setPrices(prev => ({
                ...prev,
                [item.purchase.id]: raw === '' ? undefined : Math.round(parseFloat(raw) * 100),
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
                <label htmlFor="sold-at-asking-price" className="flex items-center gap-2 text-xs text-[var(--text-muted)] cursor-pointer">
                  <input
                    id="sold-at-asking-price"
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

          <div className="flex justify-end gap-3 mt-6">
            <Dialog.Close asChild>
              <Button variant="ghost" size="sm" disabled={submitting}>
                Cancel
              </Button>
            </Dialog.Close>
            <Button size="sm" onClick={handleSubmit} loading={submitting}>
              {submitting ? 'Recording...' : 'Record Sale'}
            </Button>
          </div>
        </Dialog.Content>
      </Dialog.Portal>
    </Dialog.Root>
  );
}
