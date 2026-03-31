import { useState } from 'react';
import { useQueryClient } from '@tanstack/react-query';
import type { AgingItem } from '../../../../types/campaigns';
import { api } from '../../../../js/api';
import { useToast } from '../../../contexts/ToastContext';
import { queryKeys } from '../../../queries/queryKeys';
import PriceSignalCard from './PriceSignalCard';
import PriceDecisionBar from '../../../ui/PriceDecisionBar';
import type { PriceSource } from '../../../ui/PriceDecisionBar';

interface ExpandedDetailProps {
  item: AgingItem;
  onReviewed?: () => void;
  campaignId?: string;
  onOpenFlagDialog?: () => void;
}

export default function ExpandedDetail({ item, onReviewed, campaignId, onOpenFlagDialog }: ExpandedDetailProps) {
  const queryClient = useQueryClient();
  const toast = useToast();
  const [isSubmitting, setIsSubmitting] = useState(false);

  const snap = item.currentMarket;
  const purchase = item.purchase;
  const costBasis = purchase.buyCostCents + purchase.psaSourcingFeeCents;

  const clCents = purchase.clValueCents;
  const marketCents = snap?.medianCents ?? 0;
  const lastSoldCents = snap?.lastSoldCents ?? 0;

  const sources: PriceSource[] = [
    { label: 'CL', priceCents: clCents, source: 'cl' },
    { label: 'Market', priceCents: marketCents, source: 'market' },
    { label: 'Cost', priceCents: costBasis, source: 'cost_basis' },
    { label: 'Last Sold', priceCents: lastSoldCents, source: 'last_sold' },
  ];

  // Pre-selection priority: reviewed > cl > market > cost
  let preSelected: string | undefined;
  if (purchase.reviewedPriceCents && purchase.reviewedPriceCents > 0) {
    const matchingSource = sources.find(s => s.priceCents === purchase.reviewedPriceCents && s.priceCents > 0);
    preSelected = matchingSource?.source;
  }
  if (!preSelected) {
    if (clCents > 0) preSelected = 'cl';
    else if (marketCents > 0) preSelected = 'market';
    else if (costBasis > 0) preSelected = 'cost_basis';
  }

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
        <PriceSignalCard label="Cost Basis" valueCents={costBasis} />
        <PriceSignalCard label="Card Ladder" valueCents={clCents} />
        <PriceSignalCard
          label="Market (Median)"
          valueCents={marketCents}
          highlight={marketCents > 0 && marketCents > costBasis ? 'success' : marketCents > 0 && marketCents < costBasis ? 'danger' : undefined}
        />
        <PriceSignalCard label="Last Sold" valueCents={lastSoldCents} />
        <PriceSignalCard label="Lowest eBay Listing" valueCents={snap?.lowestListCents ?? 0} />
        <PriceSignalCard
          label="Current Override"
          valueCents={purchase.overridePriceCents ?? 0}
          highlight={purchase.overridePriceCents ? 'warning' : 'muted'}
        />
      </div>

      {/* Price decision bar */}
      <PriceDecisionBar
        sources={sources}
        preSelected={preSelected}
        onConfirm={handleConfirm}
        onFlag={onOpenFlagDialog}
        isSubmitting={isSubmitting}
      />
    </div>
  );
}
