import { useState, useMemo, useRef, useEffect } from 'react';
import { useQueryClient } from '@tanstack/react-query';
import type { Purchase, Campaign } from '../../../types/campaigns';
import { formatCents, getErrorMessage } from '../../utils/formatters';
import { useMediaQuery } from '../../hooks/useMediaQuery';
import { useToast } from '../../contexts/ToastContext';
import { EmptyState, TrendBadge } from '../../ui';
import { queryKeys } from '../../queries/queryKeys';
import { useDeletePurchase, useReassignPurchase, useCampaigns } from '../../queries/useCampaignQueries';
import QuickAddSection from './QuickAddSection';
import { displayGrade } from './inventory/utils';

/** Color-coded dot indicating whether a purchase has pricing data */
function PricingBadge({ purchase }: { purchase: Purchase }) {
  if (purchase.snapshotStatus === 'pending') {
    return <span className="inline-block w-2 h-2 rounded-full bg-[var(--warning)] animate-pulse" title="Pricing pending..." />;
  }
  if (purchase.snapshotStatus === 'failed' || purchase.snapshotStatus === 'exhausted') {
    return <span className="inline-block w-2 h-2 rounded-full bg-[var(--danger)]" title="Price lookup failed" />;
  }
  const hasPricing = purchase.medianCents != null && purchase.medianCents > 0;
  if (hasPricing) {
    return <span className="inline-block w-2 h-2 rounded-full bg-[var(--success)]" title="Pricing available" />;
  }
  return <span className="inline-block w-2 h-2 rounded-full bg-[var(--danger)]" title="No pricing data" />;
}

interface PurchasesTabProps {
  campaignId: string;
  purchases: Purchase[];
  soldPurchaseIds: Set<string>;
}

function ReassignControl({ purchase, otherCampaigns, reassigningId, setReassigningId, isMutating, onReassign }: {
  purchase: Purchase;
  otherCampaigns: Campaign[];
  reassigningId: string | null;
  setReassigningId: (id: string | null) => void;
  isMutating: boolean;
  onReassign: (purchaseId: string, cardName: string, targetCampaignId: string) => void;
}) {
  const selectRef = useRef<HTMLSelectElement>(null);
  const isOpen = reassigningId === purchase.id;

  useEffect(() => {
    if (isOpen) selectRef.current?.focus();
  }, [isOpen]);

  if (otherCampaigns.length === 0) return null;

  if (isOpen) {
    const isAssigning = isMutating;
    return (
      <div className="flex items-center gap-1">
        <select
          ref={selectRef}
          disabled={isAssigning}
          className="text-xs bg-[var(--surface-2)] text-[var(--text)] rounded px-1 py-0.5 border border-[var(--surface-2)] disabled:opacity-50"
          aria-label={`Move ${purchase.cardName} to campaign`}
          defaultValue=""
          onChange={e => {
            if (isAssigning) return;
            if (e.target.value) {
              onReassign(purchase.id, purchase.cardName, e.target.value);
            }
          }}
        >
          <option value="" disabled>Move to...</option>
          {otherCampaigns.map(c => <option key={c.id} value={c.id}>{c.name}</option>)}
        </select>
        <button
          type="button"
          disabled={isAssigning}
          className="text-xs text-[var(--text-muted)] hover:text-[var(--text)] disabled:opacity-50"
          onClick={() => setReassigningId(null)}
        >
          Cancel
        </button>
      </div>
    );
  }

  return (
    <button
      type="button"
      onClick={() => setReassigningId(purchase.id)}
      disabled={isMutating}
      className="text-[var(--info)] text-xs disabled:opacity-50"
      title="Move to another campaign"
      aria-label="Move purchase to another campaign"
    >
      &#x2197;
    </button>
  );
}

function PurchaseMobileCard({ purchase, soldPurchaseIds, otherCampaigns, reassigningId, setReassigningId, isMutating, onReassign, onDelete }: {
  purchase: Purchase;
  soldPurchaseIds: Set<string>;
  otherCampaigns: Campaign[];
  reassigningId: string | null;
  setReassigningId: (id: string | null) => void;
  isMutating: boolean;
  onReassign: (purchaseId: string, cardName: string, targetCampaignId: string) => void;
  onDelete: (purchaseId: string, cardName: string) => void;
}) {
  const isSold = soldPurchaseIds.has(purchase.id);

  return (
    <div className="p-3 rounded-xl border border-[var(--surface-2)] transition-colors duration-150 hover:border-[var(--surface-3)]"
      style={{ background: 'var(--glass-bg)', backdropFilter: 'blur(8px)' }}>
      <div className="flex items-start justify-between mb-2">
        <div>
          <div className="text-sm font-medium text-[var(--text)]">{purchase.cardName}</div>
          <div className="text-xs text-[var(--text-muted)]">Cert #{purchase.certNumber} &middot; {(purchase.grader || 'PSA').toUpperCase()} {purchase.gradeValue}</div>
        </div>
        <div className="flex items-center gap-2">
          {isSold ? (
            <span className="text-[var(--success)] text-xs font-medium px-2 py-0.5 bg-[var(--success-bg)] rounded">Sold</span>
          ) : (
            <span className="text-[var(--warning)] text-xs font-medium px-2 py-0.5 bg-[var(--warning-bg)] rounded">Unsold</span>
          )}
          {!isSold && (
            <ReassignControl
              purchase={purchase} otherCampaigns={otherCampaigns}
              reassigningId={reassigningId} setReassigningId={setReassigningId}
              isMutating={isMutating} onReassign={onReassign}
            />
          )}
          {!isSold && (
            <button
              type="button"
              onClick={() => onDelete(purchase.id, purchase.cardName)}
              disabled={isMutating}
              className="text-[var(--danger)] text-xs disabled:opacity-50"
              title="Delete purchase"
              aria-label="Delete purchase"
            >
              ✕
            </button>
          )}
        </div>
      </div>
      <div className="grid grid-cols-2 gap-x-4 gap-y-1 text-xs">
        <div><span className="text-[var(--text-muted)]">Cost:</span> <span className="text-[var(--text)]">{formatCents(purchase.buyCostCents)}</span></div>
        <div><span className="text-[var(--text-muted)]">Date:</span> <span className="text-[var(--text)]">{purchase.purchaseDate}</span></div>
        <div className="flex items-center gap-1">
          <PricingBadge purchase={purchase} />
          <span className="text-[var(--text-muted)]">Mkt:</span>{' '}
          {purchase.medianCents != null ? (
            <span className="text-[var(--text)]">
              {formatCents(purchase.medianCents)}
              <TrendBadge value={purchase.trend30d} />
            </span>
          ) : (
            <span className="text-[var(--text-muted)]">—</span>
          )}
        </div>
      </div>
    </div>
  );
}

function PurchaseDesktopRow({ purchase, soldPurchaseIds, otherCampaigns, reassigningId, setReassigningId, isMutating, onReassign, onDelete }: {
  purchase: Purchase;
  soldPurchaseIds: Set<string>;
  otherCampaigns: Campaign[];
  reassigningId: string | null;
  setReassigningId: (id: string | null) => void;
  isMutating: boolean;
  onReassign: (purchaseId: string, cardName: string, targetCampaignId: string) => void;
  onDelete: (purchaseId: string, cardName: string) => void;
}) {
  const isSold = soldPurchaseIds.has(purchase.id);

  return (
    <tr className="glass-table-row">
      <td className="glass-table-td text-[var(--text)] font-medium">{purchase.cardName}</td>
      <td className="glass-table-td text-[var(--text-muted)]">{purchase.certNumber}</td>
      <td className="glass-table-td text-center text-[var(--text)]">{displayGrade(purchase)}</td>
      <td className="glass-table-td text-right text-[var(--text)] tabular-nums">{formatCents(purchase.buyCostCents)}</td>
      <td className="glass-table-td text-right">
        <span className="inline-flex items-center gap-1">
          <PricingBadge purchase={purchase} />
          {purchase.medianCents != null ? (
            <span className="text-xs text-[var(--text-muted)] tabular-nums" title={`Last sold: ${formatCents(purchase.lastSoldCents || 0)}, Trend: ${((purchase.trend30d || 0) * 100).toFixed(0)}%`}>
              {formatCents(purchase.medianCents)}
              <TrendBadge value={purchase.trend30d} />
            </span>
          ) : (
            <span className="text-xs text-[var(--text-muted)]">—</span>
          )}
        </span>
      </td>
      <td className="glass-table-td text-[var(--text-muted)]">{purchase.purchaseDate}</td>
      <td className="glass-table-td text-center">
        {isSold ? (
          <span className="text-[var(--success)] text-xs font-medium">Sold</span>
        ) : (
          <span className="text-[var(--warning)] text-xs font-medium">Unsold</span>
        )}
      </td>
      <td className="glass-table-td text-center">
        {!isSold && (
          <div className="flex items-center justify-center gap-2">
            <ReassignControl
              purchase={purchase} otherCampaigns={otherCampaigns}
              reassigningId={reassigningId} setReassigningId={setReassigningId}
              isMutating={isMutating} onReassign={onReassign}
            />
            <button
              type="button"
              onClick={() => onDelete(purchase.id, purchase.cardName)}
              disabled={isMutating}
              className="text-[var(--danger)]/60 hover:text-[var(--danger)] text-xs disabled:opacity-50 transition-colors"
              title="Delete purchase"
              aria-label="Delete purchase"
            >
              ✕
            </button>
          </div>
        )}
      </td>
    </tr>
  );
}

export default function PurchasesTab({ campaignId, purchases, soldPurchaseIds }: PurchasesTabProps) {
  const isMobile = useMediaQuery('(max-width: 768px)');
  const queryClient = useQueryClient();
  const deleteMutation = useDeletePurchase(campaignId);
  const reassignMutation = useReassignPurchase(campaignId);
  const { data: allCampaigns = [] } = useCampaigns(false);
  const toast = useToast();
  const [reassigningId, setReassigningId] = useState<string | null>(null);

  const otherCampaigns = useMemo(() => allCampaigns.filter(c => c.id !== campaignId), [allCampaigns, campaignId]);
  const isMutating = deleteMutation.isPending || reassignMutation.isPending;

  const handleExternalAdd = () => {
    queryClient.invalidateQueries({ queryKey: queryKeys.campaigns.purchases(campaignId) });
    queryClient.invalidateQueries({ queryKey: queryKeys.campaigns.pnl(campaignId) });
    queryClient.invalidateQueries({ queryKey: queryKeys.campaigns.inventory(campaignId) });
  };

  const handleDelete = (purchaseId: string, cardName: string) => {
    if (!window.confirm(`Delete purchase "${cardName}"? This will also remove any associated sale.`)) return;
    deleteMutation.mutate(purchaseId);
  };

  const handleReassign = (purchaseId: string, cardName: string, targetCampaignId: string) => {
    const targetName = allCampaigns.find(c => c.id === targetCampaignId)?.name ?? targetCampaignId;
    if (!window.confirm(`Move "${cardName}" to campaign "${targetName}"?`)) {
      setReassigningId(null);
      return;
    }
    reassignMutation.mutate({ purchaseId, targetCampaignId }, {
      onError: (err) => {
        toast.error(getErrorMessage(err, 'Failed to reassign purchase'));
      },
      onSettled: () => setReassigningId(null),
    });
  };

  const sharedProps = {
    soldPurchaseIds,
    otherCampaigns,
    reassigningId,
    setReassigningId,
    isMutating,
    onReassign: handleReassign,
    onDelete: handleDelete,
  };

  return (
    <div id="tabpanel-purchases" role="tabpanel" aria-labelledby="purchases">
      <div className="mb-4">
        <QuickAddSection campaignId={campaignId} onAdded={handleExternalAdd} />
      </div>

      {purchases.length === 0 ? (
        <EmptyState
          icon="📦"
          title="No purchases yet"
          description="Add cards via PSA cert number or import from CSV."
        />
      ) : isMobile ? (
        <div className="space-y-3">
          {purchases.map(p => (
            <PurchaseMobileCard key={p.id} purchase={p} {...sharedProps} />
          ))}
        </div>
      ) : (
        <div className="glass-table">
          <table className="w-full text-sm">
            <thead>
              <tr className="glass-table-header">
                <th scope="col" className="glass-table-th text-left">Card</th>
                <th scope="col" className="glass-table-th text-left">Cert #</th>
                <th scope="col" className="glass-table-th text-center">Grade</th>
                <th scope="col" className="glass-table-th text-right">Buy Cost</th>
                <th scope="col" className="glass-table-th text-right">Mkt Snapshot</th>
                <th scope="col" className="glass-table-th text-left">Date</th>
                <th scope="col" className="glass-table-th text-center">Status</th>
                <th scope="col" className="glass-table-th w-20"><span className="sr-only">Actions</span></th>
              </tr>
            </thead>
            <tbody>
              {purchases.map(p => (
                <PurchaseDesktopRow key={p.id} purchase={p} {...sharedProps} />
              ))}
            </tbody>
          </table>
        </div>
      )}

    </div>
  );
}
