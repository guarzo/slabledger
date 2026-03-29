import { useState } from 'react';
import { useQueryClient } from '@tanstack/react-query';
import type { AgingItem } from '../../../../types/campaigns';
import type { PriceFlagReason } from '../../../../types/campaigns/priceReview';
import { api } from '../../../../js/api';
import { useToast } from '../../../contexts/ToastContext';
import { queryKeys } from '../../../queries/queryKeys';
import PriceSignalCard from './PriceSignalCard';
import ReviewActionBar from './ReviewActionBar';
import type { QuickPick } from './ReviewActionBar';
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
  const [selectedPick, setSelectedPick] = useState<QuickPick | null>(null);

  const snap = item.currentMarket;
  const purchase = item.purchase;
  const costBasis = purchase.buyCostCents + purchase.psaSourcingFeeCents;

  const clCents = purchase.clValueCents;
  const marketCents = snap?.medianCents ?? 0;
  const lastSoldCents = snap?.lastSoldCents ?? 0;

  const quickPicks: QuickPick[] = [
    { label: 'CL', priceCents: clCents, source: 'cl' },
    { label: 'Market', priceCents: marketCents, source: 'market' },
    { label: 'Last Sold', priceCents: lastSoldCents, source: 'last_sold' },
  ];

  const handleCardSelect = (source: string) => {
    const pick = quickPicks.find(p => p.source === source);
    if (pick && pick.priceCents > 0) {
      setSelectedPick(pick);
    }
  };

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
      setSelectedPick(null);
      invalidateQueries();
      onReviewed?.();
    } catch (err) {
      const message = err instanceof Error ? err.message : 'Failed to save reviewed price';
      toast.error(message);
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
    } catch (err) {
      const message = err instanceof Error ? err.message : 'Failed to submit price flag';
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
        <PriceSignalCard
          label="Card Ladder"
          valueCents={clCents}
          onClick={clCents > 0 ? () => handleCardSelect('cl') : undefined}
          selected={selectedPick?.source === 'cl'}
        />
        <PriceSignalCard
          label="Market (Median)"
          valueCents={marketCents}
          highlight={marketCents > 0 && marketCents > costBasis ? 'success' : marketCents > 0 && marketCents < costBasis ? 'danger' : undefined}
          onClick={marketCents > 0 ? () => handleCardSelect('market') : undefined}
          selected={selectedPick?.source === 'market'}
        />
        <PriceSignalCard
          label="Last Sold"
          valueCents={lastSoldCents}
          onClick={lastSoldCents > 0 ? () => handleCardSelect('last_sold') : undefined}
          selected={selectedPick?.source === 'last_sold'}
        />
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
        selectedPick={selectedPick}
        onPickSelect={setSelectedPick}
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
