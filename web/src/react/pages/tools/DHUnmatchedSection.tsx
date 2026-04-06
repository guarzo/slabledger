import { useState } from 'react';
import { useDHStatus, useDHUnmatched, useFixDHMatch } from '../../queries/useAdminQueries';
import { useToast } from '../../contexts/ToastContext';
import { Button, CardShell } from '../../ui';
import type { DHUnmatchedCard } from '../../../types/apiStatus';

const DH_URL_REGEX = /doubleholo\.com\/card\/\d+/;

function formatCents(cents: number): string {
  return `$${(cents / 100).toFixed(2)}`;
}

/* ── Single unmatched row with inline fix input ──────────────────── */

function UnmatchedRow({ card }: { card: DHUnmatchedCard }) {
  const [url, setUrl] = useState('');
  const [validationError, setValidationError] = useState('');
  const fixMutation = useFixDHMatch();
  const toast = useToast();

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
    <tr className="border-t border-[var(--border)] even:bg-[var(--surface-1)]/30">
      <td className="py-2 px-3 text-sm font-mono text-[var(--text-muted)]">{card.cert_number}</td>
      <td className="py-2 px-3 text-sm text-[var(--text)]">{card.card_name}</td>
      <td className="py-2 px-3 text-sm text-[var(--text-muted)]">{card.card_number}</td>
      <td className="py-2 px-3 text-sm text-[var(--text-muted)]">{card.set_name}</td>
      <td className="py-2 px-3 text-sm text-[var(--text-muted)] text-right">{card.grade}</td>
      <td className="py-2 px-3 text-sm text-[var(--text-muted)] text-right">{formatCents(card.cl_value_cents)}</td>
      <td className="py-2 px-3">
        <div className="flex flex-col gap-1 min-w-[260px]">
          <div className="flex gap-2">
            <input
              type="text"
              value={url}
              onChange={(e) => setUrl(e.target.value)}
              placeholder="doubleholo.com/card/..."
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
      </td>
    </tr>
  );
}

/* ── DHUnmatchedSection ──────────────────────────────────────────── */

export default function DHUnmatchedSection() {
  const { data: status } = useDHStatus({ enabled: true });
  const unmatchedCount = status?.unmatched_count ?? 0;
  const { data: unmatchedData } = useDHUnmatched({ enabled: unmatchedCount > 0 });

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
      ) : (
        <p className="text-sm text-[var(--text-muted)]">Loading unmatched cards...</p>
      )}
    </CardShell>
  );
}
