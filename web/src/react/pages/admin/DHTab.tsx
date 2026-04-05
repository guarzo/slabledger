import { useState } from 'react';
import { useDHStatus, useTriggerDHBulkMatch, useDHUnmatched, useFixDHMatch } from '../../queries/useAdminQueries';
import { useToast } from '../../contexts/ToastContext';
import { CardShell } from '../../ui/CardShell';
import { SummaryCard } from './shared';
import Button from '../../ui/Button';
import type { DHUnmatchedCard } from '../../../types/apiStatus';

function formatTimestamp(ts: string): string {
  if (!ts) return 'Never';
  const d = new Date(ts);
  if (isNaN(d.getTime())) return ts;
  return d.toLocaleString();
}

function formatCents(cents: number): string {
  return `$${(cents / 100).toFixed(2)}`;
}

const DH_URL_REGEX = /doubleholo\.com\/card\/\d+/;

interface UnmatchedRowProps {
  card: DHUnmatchedCard;
}

function UnmatchedRow({ card }: UnmatchedRowProps) {
  const [url, setUrl] = useState('');
  const [validationError, setValidationError] = useState('');
  const fixMutation = useFixDHMatch();
  const toast = useToast();

  const handleFix = async () => {
    setValidationError('');
    if (!DH_URL_REGEX.test(url)) {
      setValidationError('Enter a valid DoubleHolo card URL (e.g. doubleholo.com/card/123)');
      return;
    }
    try {
      await fixMutation.mutateAsync({ purchaseId: card.purchase_id, dhUrl: url });
      setUrl('');
      toast.success('Match fixed');
    } catch {
      setValidationError('Failed to fix match. Please try again.');
    }
  };

  return (
    <tr className="border-t border-[var(--border)]">
      <td className="py-2 px-3 text-sm text-[var(--text-muted)]">{card.cert_number}</td>
      <td className="py-2 px-3 text-sm text-[var(--text)]">{card.card_name}</td>
      <td className="py-2 px-3 text-sm text-[var(--text-muted)]">{card.card_number}</td>
      <td className="py-2 px-3 text-sm text-[var(--text-muted)]">{card.set_name}</td>
      <td className="py-2 px-3 text-sm text-[var(--text-muted)]">{card.grade}</td>
      <td className="py-2 px-3 text-sm text-[var(--text-muted)]">{formatCents(card.cl_value_cents)}</td>
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

export function DHTab({ enabled = true }: { enabled?: boolean }) {
  const { data: status, isLoading, error } = useDHStatus({ enabled });
  const bulkMatchMutation = useTriggerDHBulkMatch();
  const toast = useToast();

  const unmatchedCount = status?.unmatched_count ?? 0;
  const { data: unmatchedData } = useDHUnmatched({ enabled: enabled && unmatchedCount > 0 });

  if (!enabled) {
    return (
      <CardShell padding="lg">
        <p className="text-[var(--text-muted)]">DH integration is not configured.</p>
      </CardShell>
    );
  }

  if (isLoading) {
    return (
      <CardShell padding="lg">
        <p className="text-[var(--text-muted)]">Loading DH status...</p>
      </CardShell>
    );
  }

  if (error && !status) {
    return (
      <CardShell padding="lg">
        <p className="text-red-400 text-sm">Failed to load DH status. Integration may not be configured.</p>
      </CardShell>
    );
  }

  const isRunning = status?.bulk_match_running ?? false;
  const pendingCount = status?.pending_count ?? 0;

  const handleBulkMatch = async () => {
    try {
      await bulkMatchMutation.mutateAsync();
      toast.success('Bulk match started — progress will update automatically.');
    } catch {
      toast.error('Failed to start bulk match');
    }
  };

  return (
    <div className="space-y-4 mt-4">
      {/* Summary Stats */}
      <div className="grid grid-cols-2 sm:grid-cols-5 gap-3">
        <SummaryCard
          label="Market Intelligence"
          value={status?.intelligence_count ?? 0}
          sub={`Last: ${formatTimestamp(status?.intelligence_last_fetch ?? '')}`}
        />
        <SummaryCard
          label="Suggestions"
          value={status?.suggestions_count ?? 0}
          sub={`Last: ${formatTimestamp(status?.suggestions_last_fetch ?? '')}`}
        />
        <SummaryCard
          label="Mapped Cards"
          value={status?.mapped_count ?? 0}
        />
        <SummaryCard
          label="Pending Push"
          value={pendingCount}
          color={pendingCount > 0 ? 'var(--info)' : undefined}
        />
        <SummaryCard
          label="Unmatched Cards"
          value={unmatchedCount}
          color={unmatchedCount > 0 ? 'var(--warning)' : undefined}
        />
      </div>

      {/* Bulk Match */}
      <CardShell padding="lg">
        <h4 className="text-sm font-semibold text-[var(--text)] mb-2">Bulk Match (Backfill)</h4>
        <p className="text-sm text-[var(--text-muted)] mb-3">
          Match unmatched inventory cards against the DH catalog. Cards with high-confidence matches will be automatically mapped.
        </p>
        <Button
          variant="secondary"
          size="sm"
          onClick={handleBulkMatch}
          loading={bulkMatchMutation.isPending}
          disabled={isRunning || bulkMatchMutation.isPending}
        >
          {isRunning ? 'Bulk Match Running...' : 'Run Bulk Match'}
        </Button>
        {isRunning && (
          <p className="mt-2 text-xs text-[var(--text-muted)]">
            Matching in progress — mapped/unmatched counts will update automatically.
          </p>
        )}
      </CardShell>

      {/* Unmatched Cards Fix UI */}
      {unmatchedCount > 0 && (
        <CardShell padding="lg">
          <h4 className="text-sm font-semibold text-[var(--text)] mb-3">
            Unmatched Cards ({unmatchedCount})
          </h4>
          {unmatchedData?.unmatched && unmatchedData.unmatched.length > 0 ? (
            <div className="overflow-x-auto">
              <table className="w-full text-left border-collapse">
                <thead>
                  <tr>
                    <th className="pb-2 px-3 text-xs font-medium text-[var(--text-muted)]">Cert</th>
                    <th className="pb-2 px-3 text-xs font-medium text-[var(--text-muted)]">Card Name</th>
                    <th className="pb-2 px-3 text-xs font-medium text-[var(--text-muted)]">Number</th>
                    <th className="pb-2 px-3 text-xs font-medium text-[var(--text-muted)]">Set</th>
                    <th className="pb-2 px-3 text-xs font-medium text-[var(--text-muted)]">Grade</th>
                    <th className="pb-2 px-3 text-xs font-medium text-[var(--text-muted)]">CL Value</th>
                    <th className="pb-2 px-3 text-xs font-medium text-[var(--text-muted)]">Fix</th>
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
      )}
    </div>
  );
}
