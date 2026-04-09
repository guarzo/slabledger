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

/** Compact CL sync status dot with tooltip text. */
function CLSyncIndicator({ syncedAt }: { syncedAt?: string }) {
  if (!syncedAt) {
    return (
      <div className="mt-2 flex items-center gap-1.5 text-xs text-[var(--text-muted)]">
        <span className="inline-block w-2 h-2 rounded-full bg-gray-500" />
        <span>Not synced to CardLadder</span>
      </div>
    );
  }
  const parsed = new Date(syncedAt);
  const ageMs = Date.now() - parsed.getTime();
  const ageDays = Math.floor(ageMs / 86_400_000);
  // Green = synced within 2 days, amber = within 14 days, red = stale
  const color = ageDays <= 2 ? 'bg-emerald-400' : ageDays <= 14 ? 'bg-amber-400' : 'bg-red-400';
  const label = ageDays === 0 ? 'today' : ageDays === 1 ? 'yesterday' : `${ageDays}d ago`;
  return (
    <div className="mt-2 flex items-center gap-1.5 text-xs text-[var(--text-muted)]">
      <span className={`inline-block w-2 h-2 rounded-full ${color}`} />
      <span>CL synced {label}</span>
    </div>
  );
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
  const marketCents = snap?.medianCents ?? 0;
  const lastSoldCents = snap?.lastSoldCents ?? 0;

  const sources = useMemo(
    () => buildPriceSources({ clCents, marketCents, costCents: cb, lastSoldCents, mmCents }),
    [clCents, marketCents, cb, lastSoldCents, mmCents],
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
        <PriceSignalCard label="Market Movers" valueCents={mmCents} updatedAt={purchase.mmValueUpdatedAt} />
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

      {/* CL Sync indicator */}
      {purchase.certNumber && (
        <CLSyncIndicator syncedAt={purchase.clSyncedAt} />
      )}

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
