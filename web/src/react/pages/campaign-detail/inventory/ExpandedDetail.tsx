import { useState } from 'react';
import { useQueryClient } from '@tanstack/react-query';
import type { AgingItem } from '../../../../types/campaigns';
import type { PriceFlagReason } from '../../../../types/campaigns/priceReview';
import { api } from '../../../../js/api';
import { useToast } from '../../../contexts/ToastContext';
import { queryKeys } from '../../../queries/queryKeys';
import PriceSignalCard from './PriceSignalCard';
import ReviewActionBar from './ReviewActionBar';
import PriceFlagDialog from './PriceFlagDialog';

interface ExpandedDetailProps {
  item: AgingItem;
  onReviewed?: () => void;
  campaignId?: string;
}

export default function ExpandedDetail({ item, onReviewed, campaignId }: ExpandedDetailProps) {
  const queryClient = useQueryClient();
  const toast = useToast();

  const [flagDialogOpen, setFlagDialogOpen] = useState(false);
  const [isSubmitting, setIsSubmitting] = useState(false);

  const snap = item.currentMarket;
  const purchase = item.purchase;
  const costBasis = purchase.buyCostCents + purchase.psaSourcingFeeCents;

  const quickPicks = [
    { label: 'CL', priceCents: purchase.clValueCents, source: 'cl' },
    { label: 'Market', priceCents: snap?.medianCents ?? 0, source: 'market' },
    { label: 'Last Sold', priceCents: snap?.lastSoldCents ?? 0, source: 'last_sold' },
  ];

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
    } catch {
      toast.error('Failed to save reviewed price');
    } finally {
      setIsSubmitting(false);
    }
  };

  const handleFlagSubmit = async (reason: PriceFlagReason) => {
    setIsSubmitting(true);
    try {
      await api.createPriceFlag(purchase.id, reason);
      toast.success('Price flag submitted');
      setFlagDialogOpen(false);
      invalidateQueries();
      onReviewed?.();
    } catch {
      toast.error('Failed to submit price flag');
    } finally {
      setIsSubmitting(false);
    }
  };

  return (
    <div className="glass-vrow-expanded px-6 py-4 border-t border-[rgba(255,255,255,0.05)]">
      {/* 3x2 price signal grid */}
      <div className="grid grid-cols-3 gap-3 mb-4">
        <PriceSignalCard label="Cost Basis" valueCents={costBasis} />
        <PriceSignalCard label="Card Ladder" valueCents={purchase.clValueCents} />
        <PriceSignalCard
          label="Market (Median)"
          valueCents={snap?.medianCents ?? 0}
          highlight={snap?.medianCents && snap.medianCents > costBasis ? 'success' : snap?.medianCents && snap.medianCents < costBasis ? 'danger' : undefined}
        />
        <PriceSignalCard label="Last Sold" valueCents={snap?.lastSoldCents ?? 0} />
        <PriceSignalCard label="Lowest eBay Listing" valueCents={snap?.lowestListCents ?? 0} />
        <PriceSignalCard
          label="Current Override"
          valueCents={purchase.overridePriceCents ?? 0}
          highlight={purchase.overridePriceCents ? 'warning' : 'muted'}
        />
      </div>

      {/* Review action bar */}
      <ReviewActionBar
        quickPicks={quickPicks}
        onConfirm={handleConfirm}
        onFlag={() => setFlagDialogOpen(true)}
        isSubmitting={isSubmitting}
      />

      {/* Price flag dialog */}
      {flagDialogOpen && (
        <PriceFlagDialog
          cardName={purchase.cardName}
          grade={purchase.gradeValue}
          onSubmit={handleFlagSubmit}
          onCancel={() => setFlagDialogOpen(false)}
          isSubmitting={isSubmitting}
        />
      )}
    </div>
  );
}
