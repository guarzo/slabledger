import { useState, useMemo } from 'react';
import { useQueryClient } from '@tanstack/react-query';
import type { AgingItem } from '../../../../types/campaigns';
import { api } from '../../../../js/api';
import { useToast } from '../../../contexts/ToastContext';
import { queryKeys } from '../../../queries/queryKeys';
import PriceSignalCard from './PriceSignalCard';
import CompSummaryPanel from './CompSummaryPanel';
import { costBasis, formatShipDate } from './utils';
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

/** Formats a YYYY-MM-DD sale date as a short readable string, e.g. "Apr 10". */
function formatSaleDate(dateStr?: string): string | undefined {
  if (!dateStr) return undefined;
  const d = new Date(dateStr + 'T00:00:00'); // force local midnight to avoid UTC offset shift
  if (isNaN(d.getTime())) return undefined;
  return d.toLocaleDateString('en-US', { month: 'short', day: 'numeric', year: 'numeric' });
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
  const lastSoldCents = snap?.lastSoldCents ?? 0;

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

  return (
    <div className="glass-vrow-expanded px-6 py-4 border-t border-[rgba(255,255,255,0.05)]">
      {/* 3-column price signal grid, 8 cards */}
      <div className="grid grid-cols-3 gap-3 mb-4">
        <PriceSignalCard label="Cost Basis" valueCents={cb} />
        <PriceSignalCard
          label="Card Ladder"
          valueCents={clCents}
          updatedAt={purchase.clSyncedAt}
          freshnessThresholds={{ green: 2, amber: 14 }}
        />
        <PriceSignalCard
          label="Market Movers"
          valueCents={mmCents}
          updatedAt={purchase.mmValueUpdatedAt}
        />
        <PriceSignalCard
          label="DH Market"
          valueCents={dhMidCents}
          highlight={dhMidCents > 0 && dhMidCents > cb ? 'success' : dhMidCents > 0 && dhMidCents < cb ? 'danger' : undefined}
          updatedAt={purchase.dhLastSyncedAt}
          freshnessThresholds={{ green: 1, amber: 3 }}
        />
        <PriceSignalCard
          label="Last Sold"
          valueCents={lastSoldCents}
          subtitle={formatSaleDate(snap?.lastSoldDate)}
        />
        <PriceSignalCard label="Lowest eBay" valueCents={snap?.lowestListCents ?? 0} />
        <PriceSignalCard
          label="Override"
          valueCents={purchase.overridePriceCents ?? 0}
          highlight={purchase.overridePriceCents ? 'warning' : 'muted'}
          updatedAt={purchase.overridePriceCents ? purchase.overrideSetAt : undefined}
        />
        <PriceSignalCard
          label="Listed"
          valueCents={purchase.dhListingPriceCents ?? 0}
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
