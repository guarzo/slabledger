import { useEffect, useState } from 'react';
import { useDHStatus, useDHUnmatched, useFixDHMatch, useSelectDHMatch, useDismissDHMatch, useUndismissDHMatch, useReconcileDH, useRetryDHMatch } from '../../queries/useAdminQueries';
import { useToast } from '../../contexts/ToastContext';
import { formatCents } from '../../utils/formatters';
import { Button, CardShell } from '../../ui';
import type { DHUnmatchedCard, DHCandidate } from '../../../types/apiStatus';

const DH_URL_REGEX = /^(https?:\/\/)?(www\.)?doubleholo\.com\/card\/\d+/;
const INITIAL_CANDIDATES_SHOWN = 3;

/* ── Candidate card ─────────────────────────────────────────────── */

function ImagePlaceholder() {
  return (
    <div className="w-10 h-14 rounded bg-[var(--surface-2)] flex items-center justify-center text-[8px] text-[var(--text-muted)]">
      No img
    </div>
  );
}

function CandidateCard({ candidate, onSelect, isPending, isDisabled }: {
  candidate: DHCandidate;
  onSelect: (dhCardId: number) => void;
  isPending: boolean;
  isDisabled: boolean;
}) {
  const [imgFailed, setImgFailed] = useState(false);

  // Reset failure state when the image URL changes (e.g., after re-match).
  useEffect(() => { setImgFailed(false); }, [candidate.image_url]);

  return (
    <div className="flex items-center gap-2 p-2 rounded border border-[var(--border)] bg-[var(--bg-secondary)]">
      {candidate.image_url && !imgFailed ? (
        <img
          src={candidate.image_url}
          alt={candidate.card_name}
          className="w-10 h-14 object-cover rounded"
          onError={() => setImgFailed(true)}
        />
      ) : (
        <ImagePlaceholder />
      )}
      <div className="flex-1 min-w-0">
        <p className="text-xs font-medium text-[var(--text)] truncate">{candidate.card_name}</p>
        <p className="text-[10px] text-[var(--text-muted)] truncate">{candidate.set_name} #{candidate.card_number}</p>
      </div>
      <Button
        variant="secondary"
        size="sm"
        onClick={() => onSelect(candidate.dh_card_id)}
        loading={isPending}
        disabled={isDisabled}
      >
        Select
      </Button>
    </div>
  );
}

/* ── Single unmatched row with candidates + fallback URL input ── */

function UnmatchedRow({ card, onDismiss, isDismissing }: {
  card: DHUnmatchedCard;
  onDismiss: (purchaseId: string) => void;
  isDismissing: boolean;
}) {
  const [url, setUrl] = useState('');
  const [validationError, setValidationError] = useState('');
  const [showAll, setShowAll] = useState(false);
  const [selectingCardId, setSelectingCardId] = useState<number | null>(null);
  const fixMutation = useFixDHMatch();
  const selectMutation = useSelectDHMatch();
  const retryMutation = useRetryDHMatch();
  const [retryError, setRetryError] = useState<string | null>(null);
  const toast = useToast();

  const candidates = card.candidates ?? [];
  const visibleCandidates = showAll ? candidates : candidates.slice(0, INITIAL_CANDIDATES_SHOWN);
  const hiddenCount = Math.max(0, candidates.length - INITIAL_CANDIDATES_SHOWN);

  const handleSelect = async (dhCardId: number) => {
    setSelectingCardId(dhCardId);
    try {
      await selectMutation.mutateAsync({ purchaseId: card.purchase_id, dhCardId });
      toast.success('Match selected');
    } catch {
      toast.error('Failed to select match');
    } finally {
      setSelectingCardId(null);
    }
  };

  const handleFix = async () => {
    setValidationError('');
    if (!DH_URL_REGEX.test(url)) {
      setValidationError('Enter a valid doubleholo.com/card/... URL');
      return;
    }
    try {
      await fixMutation.mutateAsync({ purchaseId: card.purchase_id, dhUrl: url });
      setUrl('');
      toast.success('Match fixed');
    } catch {
      toast.error('Failed to fix match');
    }
  };

  const handleRetry = async () => {
    setRetryError(null);
    try {
      await retryMutation.mutateAsync(card.purchase_id);
      toast.success('Match retry succeeded');
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : 'Retry failed';
      setRetryError(msg);
    }
  };

  return (
    <tr className="border-b border-[var(--surface-2)]/50">
      <td className="py-2 px-3 text-sm font-mono text-[var(--text-muted)]">{card.cert_number}</td>
      <td className="py-2 px-3 text-sm text-[var(--text)]">{card.card_name}</td>
      <td className="py-2 px-3 text-sm text-[var(--text-muted)]">{card.card_number}</td>
      <td className="py-2 px-3 text-sm text-[var(--text-muted)]">{card.set_name}</td>
      <td className="py-2 px-3 text-sm text-[var(--text-muted)] text-right">{card.grade}</td>
      <td className="py-2 px-3 text-sm text-[var(--text-muted)] text-right">{formatCents(card.cl_value_cents)}</td>
      <td className="py-2 px-3">
        <div className="flex flex-col gap-2 min-w-[280px]">
          {candidates.length > 0 && (
            <div className="flex flex-col gap-1">
              {visibleCandidates.map((c) => (
                <CandidateCard key={c.dh_card_id} candidate={c} onSelect={handleSelect} isPending={selectingCardId === c.dh_card_id && selectMutation.isPending} isDisabled={selectMutation.isPending} />
              ))}
              {hiddenCount > 0 && !showAll && (
                <button
                  onClick={() => setShowAll(true)}
                  className="text-xs text-[var(--info)] hover:underline text-left"
                >
                  +{hiddenCount} more candidates
                </button>
              )}
            </div>
          )}
          <div className="flex flex-col gap-1">
            <div className="flex gap-2">
              <input
                type="text"
                value={url}
                onChange={(e) => setUrl(e.target.value)}
                placeholder="doubleholo.com/card/..."
                aria-label="DoubleHolo card URL"
                className="flex-1 text-xs px-2 py-1 rounded border border-[var(--border)] bg-[var(--bg-secondary)] text-[var(--text)] placeholder-[var(--text-muted)] focus:outline-none focus:border-[var(--info)]"
              />
               <Button
                 variant="secondary"
                 size="sm"
                 onClick={handleFix}
                 loading={fixMutation.isPending}
                 disabled={fixMutation.isPending}
               >
                 Fix
               </Button>
               <Button
                 variant="secondary"
                 size="sm"
                 onClick={handleRetry}
                 loading={retryMutation.isPending}
                 disabled={retryMutation.isPending || fixMutation.isPending || selectMutation.isPending}
               >
                 Retry
               </Button>
               <Button
                 variant="ghost"
                 size="sm"
                 onClick={() => onDismiss(card.purchase_id)}
                 loading={isDismissing}
                 disabled={isDismissing}
                 className="text-[var(--text-muted)] hover:text-[var(--text)]"
               >
                 Skip
               </Button>
            </div>
            {validationError && (
              <p className="text-xs text-red-400">{validationError}</p>
            )}
            {retryError && (
              <p className="text-xs text-[var(--error)]">{retryError}</p>
            )}
          </div>
        </div>
      </td>
    </tr>
  );
}

/* ── Dismissed row (simplified, with Undo button) ───────────────── */

function DismissedRow({ card, onUndismiss, isUndismissing }: {
  card: DHUnmatchedCard;
  onUndismiss: (purchaseId: string) => void;
  isUndismissing: boolean;
}) {
  return (
    <tr className="border-b border-[var(--surface-2)]/50 opacity-60">
      <td className="py-1.5 px-3 text-xs font-mono text-[var(--text-muted)]">{card.cert_number}</td>
      <td className="py-1.5 px-3 text-xs text-[var(--text-muted)]">{card.card_name}</td>
      <td className="py-1.5 px-3 text-xs text-[var(--text-muted)]">{card.card_number}</td>
      <td className="py-1.5 px-3 text-xs text-[var(--text-muted)]">{card.set_name}</td>
      <td className="py-1.5 px-3 text-xs text-[var(--text-muted)] text-right">{card.grade}</td>
      <td className="py-1.5 px-3 text-xs text-[var(--text-muted)] text-right">{formatCents(card.cl_value_cents)}</td>
      <td className="py-1.5 px-3">
        <Button
          variant="ghost"
          size="sm"
          onClick={() => onUndismiss(card.purchase_id)}
          loading={isUndismissing}
          disabled={isUndismissing}
          className="text-xs text-[var(--info)]"
        >
          Undo
        </Button>
      </td>
    </tr>
  );
}

/* ── Table header row ────────────────────────────────────────────── */

function TableHeader() {
  return (
    <thead>
      <tr>
        <th className="pb-2 px-3 text-[10px] font-medium uppercase tracking-wider text-[var(--text-muted)]">Cert</th>
        <th className="pb-2 px-3 text-[10px] font-medium uppercase tracking-wider text-[var(--text-muted)]">Card Name</th>
        <th className="pb-2 px-3 text-[10px] font-medium uppercase tracking-wider text-[var(--text-muted)]">Number</th>
        <th className="pb-2 px-3 text-[10px] font-medium uppercase tracking-wider text-[var(--text-muted)]">Set</th>
        <th className="pb-2 px-3 text-[10px] font-medium uppercase tracking-wider text-[var(--text-muted)] text-right">Grade</th>
        <th className="pb-2 px-3 text-[10px] font-medium uppercase tracking-wider text-[var(--text-muted)] text-right">Value</th>
        <th className="pb-2 px-3 text-[10px] font-medium uppercase tracking-wider text-[var(--text-muted)]">Actions</th>
      </tr>
    </thead>
  );
}

/* ── DHUnmatchedSection ──────────────────────────────────────────── */

export default function DHUnmatchedSection() {
  const { data: status } = useDHStatus({ enabled: true });
  const unmatchedCount = status?.unmatched_count ?? 0;
  const dismissedCount = status?.dismissed_count ?? 0;
  const totalCount = unmatchedCount + dismissedCount;
  const dhConfigured = status !== undefined;
  const { data: unmatchedData, isLoading: unmatchedLoading } = useDHUnmatched({ enabled: totalCount > 0 });
  const dismissMutation = useDismissDHMatch();
  const undismissMutation = useUndismissDHMatch();
  const reconcileMutation = useReconcileDH();
  const toast = useToast();

  const [dismissingId, setDismissingId] = useState<string | null>(null);
  const [undismissingId, setUndismissingId] = useState<string | null>(null);

  const handleReconcile = async () => {
    if (!window.confirm(
      'Reconcile local inventory against DoubleHolo?\n\n' +
      'Any local items whose DH listing has disappeared will be reset to pending ' +
      'so the push scheduler re-enrolls them. Runs synchronously — may take a minute.'
    )) return;
    try {
      const result = await reconcileMutation.mutateAsync();
      const parts = [
        `${result.scanned} scanned`,
        `${result.missingOnDH} missing on DH`,
        `${result.reset} reset`,
      ];
      if (result.errors?.length) parts.push(`${result.errors.length} errors`);
      toast.success(`Reconcile done: ${parts.join(' · ')}`);
    } catch {
      toast.error('Reconcile failed — see server logs');
    }
  };

  if (totalCount === 0 && !dhConfigured) return null;

  if (totalCount === 0) {
    return (
      <CardShell padding="lg">
        <div className="flex items-center justify-between">
          <div className="flex items-center gap-2">
            <span className="inline-flex items-center justify-center w-6 h-6 rounded bg-[var(--info)]/15 text-[var(--info)] text-[10px] font-bold leading-none">
              DH
            </span>
            <h2 className="text-sm font-semibold text-[var(--text)]">DoubleHolo Sync</h2>
          </div>
          <Button
            variant="secondary"
            size="sm"
            onClick={handleReconcile}
            loading={reconcileMutation.isPending}
            disabled={reconcileMutation.isPending}
          >
            Reconcile with DoubleHolo
          </Button>
        </div>
        <p className="mt-2 text-xs text-[var(--text-muted)]">
          No unmatched cards. Use reconcile if DH inventory was cleared externally.
        </p>
      </CardShell>
    );
  }

  const handleDismiss = async (purchaseId: string) => {
    setDismissingId(purchaseId);
    try {
      await dismissMutation.mutateAsync(purchaseId);
      toast.success('Card dismissed');
    } catch {
      toast.error('Failed to dismiss card');
    } finally {
      setDismissingId(null);
    }
  };

  const handleUndismiss = async (purchaseId: string) => {
    setUndismissingId(purchaseId);
    try {
      await undismissMutation.mutateAsync(purchaseId);
      toast.success('Card restored');
    } catch {
      toast.error('Failed to restore card');
    } finally {
      setUndismissingId(null);
    }
  };

  const dismissed = unmatchedData?.dismissed ?? [];

  return (
    <CardShell padding="lg">
      <div className="flex items-center justify-between mb-3">
        <div className="flex items-center gap-2">
          <span className="inline-flex items-center justify-center w-6 h-6 rounded bg-[var(--warning)]/15 text-[var(--warning)] text-[10px] font-bold leading-none">
            DH
          </span>
          <h2 className="text-sm font-semibold text-[var(--text)]">Unmatched Cards</h2>
        </div>
        <div className="flex items-center gap-2">
          <Button
            variant="ghost"
            size="sm"
            onClick={handleReconcile}
            loading={reconcileMutation.isPending}
            disabled={reconcileMutation.isPending}
          >
            Reconcile
          </Button>
          {unmatchedCount > 0 && (
            <span className="inline-flex items-center justify-center min-w-[20px] h-5 px-1.5 rounded-full bg-[var(--warning)]/15 text-[var(--warning)] text-xs font-semibold">
              {unmatchedCount}
            </span>
          )}
        </div>
      </div>

      {unmatchedData?.unmatched && unmatchedData.unmatched.length > 0 ? (
        <div className="overflow-x-auto">
          <table className="w-full text-left border-collapse">
            <TableHeader />
            <tbody>
              {unmatchedData.unmatched.map((card) => (
                <UnmatchedRow
                  key={card.purchase_id}
                  card={card}
                  onDismiss={handleDismiss}
                  isDismissing={dismissingId === card.purchase_id}
                />
              ))}
            </tbody>
          </table>
        </div>
      ) : unmatchedLoading ? (
        <p className="text-sm text-[var(--text-muted)]">Loading unmatched cards...</p>
      ) : unmatchedData?.unmatched !== undefined ? (
        <p className="text-sm text-[var(--text-muted)]">No unmatched cards found.</p>
      ) : (
        <p className="text-sm text-[var(--danger)]">Failed to load unmatched cards.</p>
      )}

      {dismissed.length > 0 && (
        <details className="mt-4">
          <summary className="text-xs font-medium text-[var(--text-muted)] cursor-pointer hover:text-[var(--text)] transition-colors">
            Dismissed ({dismissed.length})
          </summary>
          <div className="overflow-x-auto mt-2">
            <table className="w-full text-left border-collapse">
              <TableHeader />
              <tbody>
                {dismissed.map((card) => (
                  <DismissedRow
                    key={card.purchase_id}
                    card={card}
                    onUndismiss={handleUndismiss}
                    isUndismissing={undismissingId === card.purchase_id}
                  />
                ))}
              </tbody>
            </table>
          </div>
        </details>
      )}
    </CardShell>
  );
}
