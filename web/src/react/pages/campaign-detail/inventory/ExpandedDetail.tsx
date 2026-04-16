import { useState, useMemo } from 'react';
import { useQueryClient } from '@tanstack/react-query';
import type { AgingItem } from '../../../../types/campaigns';
import { api } from '../../../../js/api';
import { useToast } from '../../../contexts/ToastContext';
import { queryKeys } from '../../../queries/queryKeys';
import SellPriceHero from './SellPriceHero';
import SignalChip from './SignalChip';
import CompSummaryPanel from './CompSummaryPanel';
import { costBasis, formatShipDate, mostRecentSale } from './utils';
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

export default function ExpandedDetail({ item, onReviewed, campaignId, onOpenFlagDialog, onResolveFlag, onApproveDHPush, onSetPrice }: ExpandedDetailProps) {
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

  const lowestListCents = snap?.lowestListCents ?? 0;
  const overrideCents = purchase.overridePriceCents ?? 0;
  const listedCents = purchase.dhListingPriceCents ?? 0;

  return (
    <div className="glass-vrow-expanded px-6 py-4 border-t border-[rgba(255,255,255,0.05)]">
      {/* Hero: most recent sale + comp context + range bar */}
      <SellPriceHero item={item} costBasisCents={cb} />

      <div className="mb-4 flex flex-wrap items-stretch gap-2">
        <SignalChip label="Cost Basis" valueCents={cb} />
        <SignalChip
          label="Card Ladder"
          valueCents={clCents}
          deltaVsCostCents={clCents > 0 ? clCents - cb : undefined}
          updatedAt={purchase.clSyncedAt}
          freshnessThresholds={{ green: 2, amber: 14 }}
        />
        <SignalChip
          label="Market Movers"
          valueCents={mmCents}
          hideWhenZero
          deltaVsCostCents={mmCents > 0 ? mmCents - cb : undefined}
          updatedAt={purchase.mmValueUpdatedAt}
        />
        <SignalChip
          label="DH Market"
          valueCents={dhMidCents}
          hideWhenZero
          deltaVsCostCents={dhMidCents > 0 ? dhMidCents - cb : undefined}
          updatedAt={purchase.dhLastSyncedAt}
          freshnessThresholds={{ green: 1, amber: 3 }}
        />
        <SignalChip label="Lowest eBay" valueCents={lowestListCents} hideWhenZero />
        <SignalChip
          label="Override"
          valueCents={overrideCents}
          hideWhenZero
          tone="warning"
          updatedAt={purchase.overrideSetAt}
        />
        <SignalChip label="Listed" valueCents={listedCents} hideWhenZero />
      </div>

      {item.compSummary && item.compSummary.recentComps > 0 && (
        <CompSummaryPanel comp={item.compSummary} costBasisCents={cb} />
      )}

      {/* Price decision bar */}
      <PriceDecisionBar
        sources={sources}
        preSelected={preSelected}
        onConfirm={handleConfirm}
        onFlag={onOpenFlagDialog}
        isSubmitting={isSubmitting}
      />

      {/* Ship date chip (only when not yet received) */}
      {purchase.psaShipDate && !purchase.receivedAt && (
        <div className="mt-2 inline-flex items-center gap-1.5 rounded-md border border-[rgba(148,163,184,0.15)] bg-[rgba(148,163,184,0.08)] px-2.5 py-1">
          <svg width="11" height="11" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" className="text-[var(--text-muted)] shrink-0">
            <rect x="3" y="4" width="18" height="18" rx="2" ry="2"/>
            <line x1="16" y1="2" x2="16" y2="6"/>
            <line x1="8" y1="2" x2="8" y2="6"/>
            <line x1="3" y1="10" x2="21" y2="10"/>
          </svg>
          <span className="text-[10px] font-semibold uppercase tracking-wide text-[var(--text-muted)]">Shipped</span>
          <span className="text-xs text-[var(--text-muted)]">{formatShipDate(purchase.psaShipDate)}</span>
        </div>
      )}
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
