import { useState } from 'react';
import { useDHStatus, useDHUnmatched, useFixDHMatch, useSelectDHMatch } from '../../queries/useAdminQueries';
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

  return (
    <div className="flex items-center gap-2 p-2 rounded border border-[var(--border)] bg-[var(--bg-secondary)]">
      {candidate.image_url && !imgFailed ? (
        <img
          src={candidate.image_url}
          alt={candidate.card_name}
          className="w-10 h-14 object-cover rounded"
          loading="lazy"
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

function UnmatchedRow({ card }: { card: DHUnmatchedCard }) {
  const [url, setUrl] = useState('');
  const [validationError, setValidationError] = useState('');
  const [showAll, setShowAll] = useState(false);
  const [selectingCardId, setSelectingCardId] = useState<number | null>(null);
  const fixMutation = useFixDHMatch();
  const selectMutation = useSelectDHMatch();
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
            </div>
            {validationError && (
              <p className="text-xs text-red-400">{validationError}</p>
            )}
          </div>
        </div>
      </td>
    </tr>
  );
}

/* ── DHUnmatchedSection ──────────────────────────────────────────── */

export default function DHUnmatchedSection() {
  const { data: status } = useDHStatus({ enabled: true });
  const unmatchedCount = status?.unmatched_count ?? 0;
  const { data: unmatchedData, isLoading: unmatchedLoading } = useDHUnmatched({ enabled: unmatchedCount > 0 });

  if (unmatchedCount === 0) return null;

  return (
    <CardShell padding="lg">
      <div className="flex items-center justify-between mb-3">
        <div className="flex items-center gap-2">
          <span className="inline-flex items-center justify-center w-6 h-6 rounded bg-[var(--warning)]/15 text-[var(--warning)] text-[10px] font-bold leading-none">
            DH
          </span>
          <h3 className="text-sm font-semibold text-[var(--text)]">Unmatched Cards</h3>
        </div>
        <span className="inline-flex items-center justify-center min-w-[20px] h-5 px-1.5 rounded-full bg-[var(--warning)]/15 text-[var(--warning)] text-xs font-semibold">
          {unmatchedCount}
        </span>
      </div>

      {unmatchedData?.unmatched && unmatchedData.unmatched.length > 0 ? (
        <div className="overflow-x-auto">
          <table className="w-full text-left border-collapse">
            <thead>
              <tr>
                <th className="pb-2 px-3 text-[10px] font-medium uppercase tracking-wider text-[var(--text-muted)]">Cert</th>
                <th className="pb-2 px-3 text-[10px] font-medium uppercase tracking-wider text-[var(--text-muted)]">Card Name</th>
                <th className="pb-2 px-3 text-[10px] font-medium uppercase tracking-wider text-[var(--text-muted)]">Number</th>
                <th className="pb-2 px-3 text-[10px] font-medium uppercase tracking-wider text-[var(--text-muted)]">Set</th>
                <th className="pb-2 px-3 text-[10px] font-medium uppercase tracking-wider text-[var(--text-muted)] text-right">Grade</th>
                <th className="pb-2 px-3 text-[10px] font-medium uppercase tracking-wider text-[var(--text-muted)] text-right">Value</th>
                <th className="pb-2 px-3 text-[10px] font-medium uppercase tracking-wider text-[var(--text-muted)]">Fix</th>
              </tr>
            </thead>
            <tbody>
              {unmatchedData.unmatched.map((card) => (
                <UnmatchedRow key={card.purchase_id} card={card} />
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
    </CardShell>
  );
}
