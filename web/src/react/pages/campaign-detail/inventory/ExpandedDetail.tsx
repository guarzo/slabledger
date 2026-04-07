import { useState, useMemo } from 'react';
import { useQueryClient } from '@tanstack/react-query';
import type { AgingItem } from '../../../../types/campaigns';
import { api } from '../../../../js/api';
import { useToast } from '../../../contexts/ToastContext';
import { queryKeys } from '../../../queries/queryKeys';
import PriceSignalCard from './PriceSignalCard';
import CompSummaryPanel from './CompSummaryPanel';
import { costBasis } from './utils';
import { PriceDecisionBar, buildPriceSources, preSelectSource, Button } from '../../../ui';

interface ExpandedDetailProps {
  item: AgingItem;
  onReviewed?: () => void;
  campaignId?: string;
  onOpenFlagDialog?: () => void;
  onResolveFlag?: (flagId: number) => void;
  onApproveDHPush?: (purchaseId: string) => void;
  onSetPrice?: () => void;
}

function formatHoldReason(reason: string): string {
  if (reason.startsWith('price_swing:')) {
    const parts = reason.split(':');
    return `Price swing: ${parts[1] || 'Unknown'}`;
  }
  if (reason.startsWith('source_disagreement:')) {
    const parts = reason.split(':');
    return `Source disagreement: ${parts[1] || 'Unknown'}`;
  }
  if (reason.startsWith('unreviewed_cl_change:')) {
    const parts = reason.split(':');
    return `Unreviewed CL change: ${parts[1] || 'Unknown'}`;
  }
  return reason || 'Unknown reason';
}

export default function ExpandedDetail({ item, onReviewed, campaignId, onOpenFlagDialog, onResolveFlag, onApproveDHPush, onSetPrice }: ExpandedDetailProps) {
  const queryClient = useQueryClient();
  const toast = useToast();
  const [isSubmitting, setIsSubmitting] = useState(false);

  const snap = item.currentMarket;
  const purchase = item.purchase;
  const cb = costBasis(purchase);

  const clCents = purchase.clValueCents;
  const marketCents = snap?.medianCents ?? 0;
  const lastSoldCents = snap?.lastSoldCents ?? 0;

  const sources = useMemo(
    () => buildPriceSources({ clCents, marketCents, costCents: cb, lastSoldCents }),
    [clCents, marketCents, cb, lastSoldCents],
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

  return (
    <div className="glass-vrow-expanded px-6 py-4 border-t border-[rgba(255,255,255,0.05)]">
      {/* 3x2 price signal grid */}
      <div className="grid grid-cols-3 gap-3 mb-4">
        <PriceSignalCard label="Cost Basis" valueCents={cb} />
        <PriceSignalCard label="Card Ladder" valueCents={clCents} />
        <PriceSignalCard
          label="Market (Median)"
          valueCents={marketCents}
          highlight={marketCents > 0 && marketCents > cb ? 'success' : marketCents > 0 && marketCents < cb ? 'danger' : undefined}
        />
        <PriceSignalCard label="Last Sold" valueCents={lastSoldCents} />
        <PriceSignalCard label="Lowest eBay Listing" valueCents={snap?.lowestListCents ?? 0} />
        <PriceSignalCard
          label="Current Override"
          valueCents={purchase.overridePriceCents ?? 0}
          highlight={purchase.overridePriceCents ? 'warning' : 'muted'}
        />
      </div>

      {/* Comp Summary Panel */}
      {item.compSummary && item.compSummary.recentComps > 0 && (
        <CompSummaryPanel comp={item.compSummary} />
      )}

      {/* Price decision bar */}
      <PriceDecisionBar
        sources={sources}
        preSelected={preSelected}
        onConfirm={handleConfirm}
        onFlag={onOpenFlagDialog}
        isSubmitting={isSubmitting}
      />

      {/* DH Push Held action */}
      {item.purchase.dhPushStatus === 'held' && onApproveDHPush && (
        <div className="mt-3 p-3 rounded-lg bg-amber-500/10 border border-amber-500/30">
          <div className="flex items-center justify-between">
            <div>
              <span className="text-xs font-semibold text-amber-400 uppercase">DH Push Held</span>
              <p className="text-sm text-[var(--text-muted)] mt-0.5">
                {formatHoldReason(item.purchase.dhHoldReason ?? '')}
              </p>
            </div>
            <div className="flex gap-2">
              {onSetPrice && (
                <Button size="sm" variant="secondary" onClick={onSetPrice}>
                  Adjust Price
                </Button>
              )}
              <Button size="sm" onClick={() => onApproveDHPush(item.purchase.id)}>
                Approve Push
              </Button>
            </div>
          </div>
        </div>
      )}

      {/* Resolve flag action */}
      {item.hasOpenFlag && item.openFlagId && onResolveFlag && (
        <div className="mt-3 flex items-center gap-2">
          <span className="text-xs text-[var(--warning)]">This card has an open price flag</span>
          <Button
            variant="secondary"
            size="sm"
            onClick={() => onResolveFlag(item.openFlagId!)}
          >
            Resolve Flag
          </Button>
        </div>
      )}
    </div>
  );
}
