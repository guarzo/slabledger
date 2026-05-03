import { useState, useMemo } from 'react';
import { useQueryClient } from '@tanstack/react-query';
import type { AgingItem } from '../../../../types/campaigns';
import { api, isAPIError } from '../../../../js/api';
import { useToast } from '../../../contexts/ToastContext';
import { queryKeys } from '../../../queries/queryKeys';
import SellPriceHero from './SellPriceHero';
import { costBasis, formatShipDate, isShipmentOverdue, mostRecentSale } from './utils';
import { formatCents } from '../../../utils/formatters';
import { PriceDecisionBar, buildPriceSources, preSelectSource, Button } from '../../../ui';
import RecordSaleForm from '../RecordSaleForm';

interface ExpandedDetailProps {
  item: AgingItem;
  onReviewed?: () => void;
  campaignId?: string;
  onOpenFlagDialog?: () => void;
  onResolveFlag?: (flagId: number) => void;
  onApproveDHPush?: (purchaseId: string) => void;
  onSetPrice?: () => void;
  combineWithList?: boolean;
  /** When true, swap the pricing panel for an inline RecordSaleForm. */
  recordingSale?: boolean;
  onCancelInlineSale?: () => void;
  onInlineSaleSuccess?: () => void;
}

const holdReasonLabels: Record<string, string> = {
  'price_swing:': 'Price swing',
  'source_disagreement:': 'Source disagreement',
  'unreviewed_cl_change:': 'Unreviewed CL change',
};

function formatHoldReason(reason: string): string {
  for (const [prefix, label] of Object.entries(holdReasonLabels)) {
    if (reason.startsWith(prefix)) {
      return `${label}: ${reason.slice(prefix.length) || 'Unknown'}`;
    }
  }
  return reason || 'Unknown reason';
}

export default function ExpandedDetail({ item, onReviewed, campaignId, onOpenFlagDialog, onResolveFlag, onApproveDHPush, onSetPrice, combineWithList, recordingSale, onCancelInlineSale, onInlineSaleSuccess }: ExpandedDetailProps) {
  const queryClient = useQueryClient();
  const toast = useToast();
  const [isSubmitting, setIsSubmitting] = useState(false);

  const snap = item.currentMarket;
  const purchase = item.purchase;
  const cb = costBasis(purchase);

  const clCents = purchase.clValueCents;
  const mmCents = purchase.mmValueCents ?? 0;
  const dhMidCents = snap?.midPriceCents ?? 0;
  const lastSoldCents = mostRecentSale(item)?.cents ?? 0;

  const sources = useMemo(
    () => buildPriceSources({ clCents, dhMidCents, costCents: cb, lastSoldCents, mmCents }),
    [clCents, dhMidCents, cb, lastSoldCents, mmCents],
  );

  const preSelected = useMemo(
    () => preSelectSource(sources, purchase.reviewedPriceCents),
    [sources, purchase.reviewedPriceCents],
  );

  const invalidateQueries = () => {
    if (campaignId) {
      queryClient.invalidateQueries({ queryKey: queryKeys.campaigns.inventory(campaignId) });
    } else {
      queryClient.invalidateQueries({
        predicate: (query) => query.queryKey[0] === 'campaigns' && query.queryKey[2] === 'inventory',
      });
    }
    queryClient.invalidateQueries({ queryKey: queryKeys.portfolio.globalInventory });
  };

  const handleConfirm = async (priceCents: number, source: string) => {
    setIsSubmitting(true);
    try {
      await api.setReviewedPrice(purchase.id, priceCents, source);
      toast.success('Reviewed price saved');
      invalidateQueries();
      onReviewed?.();
    } catch (err) {
      const message = err instanceof Error ? err.message : 'Failed to save reviewed price';
      toast.error(message);
    } finally {
      setIsSubmitting(false);
    }
  };

  const handleSetAndList = async (priceCents: number, source: string) => {
    setIsSubmitting(true);
    try {
      await api.setReviewedPrice(purchase.id, priceCents, source);
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Failed to save reviewed price');
      setIsSubmitting(false);
      return;
    }
    try {
      await api.listPurchaseOnDH(purchase.id);
      toast.success('Price set and listed on DH');
      invalidateQueries();
      onReviewed?.();
    } catch (err) {
      if (isAPIError(err) && err.status === 409 && err.data?.error === 'Purchase already listed on DH') {
        toast.success('Price set and listed on DH');
        invalidateQueries();
        onReviewed?.();
        return;
      }
      const msg = err instanceof Error ? err.message : 'Listing failed';
      toast.error(
        msg.toLowerCase().includes('stock')
          ? 'DH push pending. Check back after sync.'
          : msg,
      );
      invalidateQueries();
    } finally {
      setIsSubmitting(false);
    }
  };

  const lowestListCents = snap?.lowestListCents ?? 0;
  const overrideCents = purchase.overridePriceCents ?? 0;
  const listedCents = purchase.dhListingPriceCents ?? 0;

  if (recordingSale && onCancelInlineSale && onInlineSaleSuccess) {
    return (
      <div className="glass-vrow-expanded px-6 py-4 border-t border-[rgba(255,255,255,0.05)]">
        <div className="max-w-2xl">
          <div className="text-sm font-semibold text-[var(--text)] mb-3">Record sale</div>
          <RecordSaleForm
            item={item}
            onCancel={onCancelInlineSale}
            onSuccess={onInlineSaleSuccess}
            hideItemHeader
          />
        </div>
      </div>
    );
  }

  // Build context-meta items (only render those that are active/non-zero)
  const metaPills: Array<{ label: string; value: string; tone?: 'warning' | 'default' }> = [];
  if (overrideCents > 0) metaPills.push({ label: 'Override', value: formatCents(overrideCents), tone: 'warning' });
  if (listedCents > 0) metaPills.push({ label: 'Listed', value: formatCents(listedCents) });
  if (lowestListCents > 0) metaPills.push({ label: 'Lowest eBay', value: formatCents(lowestListCents) });

  const shipOverdue = purchase.psaShipDate && !purchase.receivedAt && isShipmentOverdue(purchase.psaShipDate);
  const showShipChip = purchase.psaShipDate && !purchase.receivedAt;
  const showHold = item.purchase.dhPushStatus === 'held' && !!onApproveDHPush;
  const showFlag = item.hasOpenFlag && !!item.openFlagId && !!onResolveFlag;

  return (
    <div className="glass-vrow-expanded px-6 py-4 border-t border-[rgba(255,255,255,0.05)]">
      {/* 1. PRICE DECISION — top, gravity center */}
      <PriceDecisionBar
        sources={sources}
        preSelected={preSelected}
        onConfirm={combineWithList ? handleSetAndList : handleConfirm}
        onFlag={onOpenFlagDialog}
        isSubmitting={isSubmitting}
        confirmLabel={combineWithList ? 'List on DH' : undefined}
        secondaryConfirm={combineWithList ? { label: 'Set Price', onConfirm: handleConfirm } : undefined}
        costBasisCents={cb}
      />

      {/* 2. HERO — most recent sale + comp summary + range bar (typography on the page, no card) */}
      <div className="mt-4 pt-4 border-t border-[rgba(255,255,255,0.05)]">
        <SellPriceHero item={item} costBasisCents={cb} flat />
      </div>

      {/* 3. CONTEXT META — only when something to show */}
      {metaPills.length > 0 && (
        <div className="mt-3 flex flex-wrap items-center gap-x-5 gap-y-1.5 text-xs">
          {metaPills.map((p) => (
            <div key={p.label} className="inline-flex items-baseline gap-1.5">
              <span className="text-[10px] font-semibold uppercase tracking-wider text-[var(--text-muted)]">
                {p.label}
              </span>
              <span
                className="tabular-nums font-semibold"
                style={{ color: p.tone === 'warning' ? 'var(--warning)' : 'var(--text)' }}
              >
                {p.value}
              </span>
            </div>
          ))}
        </div>
      )}

      {/* 4. STATE PILLS — ship / hold / flag inline */}
      {(showShipChip || showHold || showFlag) && (
        <div className="mt-3 flex flex-wrap items-center gap-2">
          {showShipChip && (
            <span className={`inline-flex items-center gap-1.5 rounded-md border px-2.5 py-1 text-[11px] font-semibold uppercase tracking-wider ${
              shipOverdue
                ? 'border-[var(--warning-border)] bg-[var(--warning-bg)] text-[var(--warning)]'
                : 'border-[rgba(148,163,184,0.15)] bg-[rgba(148,163,184,0.08)] text-[var(--text-muted)]'
            }`}>
              {shipOverdue ? 'Overdue' : 'Shipped'} · {formatShipDate(purchase.psaShipDate!)}
            </span>
          )}

          {showHold && (
            <span className="inline-flex items-center gap-2 rounded-md border border-[var(--warning-border)] bg-[var(--warning-bg)] px-2.5 py-1">
              <span className="text-[11px] font-semibold uppercase tracking-wider text-[var(--warning)]">
                DH Push Held
              </span>
              <span className="text-xs text-[var(--text-muted)]">
                {formatHoldReason(item.purchase.dhHoldReason ?? '')}
              </span>
              {onSetPrice && (
                <Button size="sm" variant="ghost" onClick={onSetPrice}>
                  Adjust
                </Button>
              )}
              <Button size="sm" onClick={() => onApproveDHPush!(item.purchase.id)}>
                Approve
              </Button>
            </span>
          )}

          {showFlag && (
            <span className="inline-flex items-center gap-2 rounded-md border border-[var(--warning-border)] bg-[var(--warning-bg)] px-2.5 py-1">
              <span className="text-[11px] font-semibold uppercase tracking-wider text-[var(--warning)]">
                Price flag open
              </span>
              <Button
                variant="ghost"
                size="sm"
                onClick={() => onResolveFlag!(item.openFlagId!)}
              >
                Resolve
              </Button>
            </span>
          )}
        </div>
      )}
    </div>
  );
}
