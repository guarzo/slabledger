import { useState } from 'react';
import type { CardRequestSubmission } from '../../../js/api';
import { isAPIError } from '../../../js/api';
import { useCardRequests, useSubmitCardRequest, useSubmitAllCardRequests } from '../../queries/useAdminQueries';
import { Button } from '../../ui';

const unknownBadge = { bg: 'bg-[var(--surface-2)]', text: 'text-[var(--text-muted)]', label: 'Unknown' } as const;

function normalizeStatus(status: string | undefined | null): string {
  return (status ?? '').trim().toLowerCase();
}

const statusBadge: Record<string, { bg: string; text: string; label: string }> = {
  pending: { bg: 'bg-[var(--warning-bg)]', text: 'text-[var(--warning)]', label: 'Pending' },
  submitted: { bg: 'bg-[var(--brand-500)]/20', text: 'text-[var(--brand-400)]', label: 'Submitted' },
  accepted: { bg: 'bg-[var(--success-bg)]', text: 'text-[var(--success)]', label: 'Accepted' },
  rejected: { bg: 'bg-[var(--danger-bg)]', text: 'text-[var(--danger)]', label: 'Rejected' },
};

export function MissingCardsTab({ enabled = true }: { enabled?: boolean }) {
  const { data: items, error, isLoading } = useCardRequests({ enabled });
  const submitOne = useSubmitCardRequest();
  const submitAll = useSubmitAllCardRequests();
  const [submittingId, setSubmittingId] = useState<number | null>(null);
  const [submitError, setSubmitError] = useState<string | null>(null);

  if (isLoading) return <div className="text-center text-[var(--text-muted)] py-8">Loading...</div>;
  if (error) {
    if (isAPIError(error) && error.status === 404) {
      return <div className="p-3 rounded-lg bg-[var(--surface-2)] border border-[var(--border)] text-[var(--text-muted)] text-sm">Missing card tracking is not available.</div>;
    }
    return <div className="p-3 rounded-lg bg-[var(--danger-bg)] border border-[var(--danger-border)] text-[var(--danger)] text-sm">Failed to load missing cards</div>;
  }

  const pendingCount = items?.filter((i: CardRequestSubmission) => normalizeStatus(i.status) === 'pending').length ?? 0;

  return (
    <div className="space-y-4">
      {/* Summary bar */}
      <div className="flex items-center justify-between">
        <div className="text-sm text-[var(--text-muted)]">
          <span className="font-medium text-[var(--text)]">{pendingCount}</span> pending request{pendingCount !== 1 ? 's' : ''}
          {items && items.length > 0 && (
            <span className="ml-2">/ {items.length} total</span>
          )}
        </div>
        {pendingCount > 0 && (
          <Button
            onClick={() => { if (submitAll.isPending || submittingId !== null) return; setSubmitError(null); submitAll.mutate(); }}
            loading={submitAll.isPending}
            disabled={submitAll.isPending || submittingId !== null}
          >
            {submitAll.isPending ? 'Submitting...' : `Submit All (${pendingCount})`}
          </Button>
        )}
      </div>

      {submitAll.isError && (
        <div className="p-3 rounded-lg bg-[var(--danger-bg)] border border-[var(--danger-border)] text-[var(--danger)] text-sm">
          Failed to submit card requests
        </div>
      )}

      {submitError && (
        <div className="p-3 rounded-lg bg-[var(--danger-bg)] border border-[var(--danger-border)] text-[var(--danger)] text-sm">
          {submitError}
        </div>
      )}

      {/* Table */}
      <div className="glass-table max-h-[min(600px,calc(100vh-300px))] overflow-y-auto scrollbar-dark">
        {!items || items.length === 0 ? (
          <div className="px-5 py-8 text-center text-[var(--text-muted)] text-sm">
            No missing cards detected.
          </div>
        ) : (
          <table className="w-full text-sm">
            <thead className="sticky top-0 z-10">
              <tr className="glass-table-header" style={{ backdropFilter: 'blur(12px)' }}>
                <th className="glass-table-th text-left w-16">Image</th>
                <th className="glass-table-th text-left">Card</th>
                <th className="glass-table-th text-left hidden sm:table-cell">Set</th>
                <th className="glass-table-th text-left hidden md:table-cell">Number</th>
                <th className="glass-table-th text-left hidden md:table-cell">Grade</th>
                <th className="glass-table-th text-left hidden lg:table-cell">Cert</th>
                <th className="glass-table-th text-left">Status</th>
                <th className="glass-table-th w-20" scope="col" aria-label="Actions"></th>
              </tr>
            </thead>
            <tbody>
              {items.map((item: CardRequestSubmission) => {
                const badge = statusBadge[normalizeStatus(item.status)] ?? unknownBadge;
                return (
                  <tr key={item.id} className="glass-table-row">
                    <td className="glass-table-td">
                      {item.frontImageUrl ? (
                        <img
                          src={item.frontImageUrl}
                          alt={item.cardName || 'Card'}
                          className="w-12 h-16 object-contain rounded"
                        />
                      ) : (
                        <div className="w-12 h-16 rounded bg-[var(--surface-2)] flex items-center justify-center text-[var(--text-muted)] text-xs">
                          N/A
                        </div>
                      )}
                    </td>
                    <td className="glass-table-td">
                      <div className="text-[var(--text)] font-medium">{item.cardName || '-'}</div>
                      <div className="text-xs text-[var(--text-muted)] sm:hidden">{item.setName || '-'}</div>
                    </td>
                    <td className="glass-table-td text-[var(--text-muted)] hidden sm:table-cell">{item.setName || '-'}</td>
                    <td className="glass-table-td text-[var(--text-muted)] hidden md:table-cell">{item.cardNumber || '-'}</td>
                    <td className="glass-table-td text-[var(--text-muted)] hidden md:table-cell">{item.grade || '-'}</td>
                    <td className="glass-table-td text-[var(--text-muted)] hidden lg:table-cell font-mono text-xs">{item.certNumber || '-'}</td>
                    <td className="glass-table-td">
                      <span className={`px-2 py-0.5 text-xs font-medium rounded-full ${badge.bg} ${badge.text}`}>
                        {badge.label}
                      </span>
                    </td>
                    <td className="glass-table-td text-right">
                      {normalizeStatus(item.status) === 'pending' && (
                        <Button
                          size="sm"
                          onClick={() => {
                            if (submitAll.isPending || submittingId !== null) return;
                            setSubmitError(null);
                            setSubmittingId(item.id);
                            submitOne.mutate(item.id, {
                              onError: (err) => {
                                setSubmitError(`Failed to submit card #${item.id}: ${err instanceof Error ? err.message : 'unknown error'}`);
                              },
                              onSettled: () => setSubmittingId(null),
                            });
                          }}
                          loading={submittingId === item.id}
                          disabled={submitAll.isPending || (submittingId !== null && submittingId !== item.id)}
                        >
                          {submittingId === item.id ? 'Submitting...' : 'Submit'}
                        </Button>
                      )}
                    </td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        )}
      </div>
    </div>
  );
}
