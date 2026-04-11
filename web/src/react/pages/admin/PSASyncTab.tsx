import type { ReactNode } from 'react';
import { usePSASyncStatus, useTriggerPSASyncRefresh } from '../../queries/useAdminQueries';
import { useToast } from '../../contexts/ToastContext';
import { CardShell } from '../../ui/CardShell';
import Button from '../../ui/Button';
import { formatAdminDate } from './adminUtils';
import { isAPIError } from '../../../js/api/client';

export function PSASyncTab({ enabled = true }: { enabled?: boolean }) {
  const { data } = usePSASyncStatus({ enabled });
  const refreshMutation = useTriggerPSASyncRefresh();
  const toast = useToast();

  if (!data) return null;

  const handleRefresh = async () => {
    try {
      await refreshMutation.mutateAsync();
      toast.success('PSA Sheets sync complete');
    } catch (err) {
      if (isAPIError(err) && err.status === 409) {
        toast.error('Sync already in progress — try again in a moment');
      } else {
        toast.error('PSA Sheets sync failed');
      }
    }
  };

  if (!data.configured) {
    return (
      <CardShell padding="lg">
        <div className="flex items-center gap-2 mb-3">
          <span className="w-2 h-2 rounded-full bg-gray-500 shrink-0" />
          <span className="text-sm font-semibold text-[var(--text)]">Not configured</span>
        </div>
        <p className="text-xs text-[var(--text-muted)]">
          Set <code>GOOGLE_SHEETS_SPREADSHEET_ID</code> and service account credentials to enable PSA Sheets sync.
        </p>
      </CardShell>
    );
  }

  const lastRun = data.lastRun;

  return (
    <div className="space-y-4 mt-4">
      <CardShell padding="lg">
        {/* Header row */}
        <div className="flex items-center justify-between mb-3">
          <div className="flex items-center gap-2">
            <span className="w-2 h-2 rounded-full bg-emerald-400 shrink-0" />
            <span className="text-sm font-semibold text-[var(--text)]">Configured</span>
          </div>
          <span className="text-xs text-[var(--text-muted)] font-mono">
            {data.spreadsheetId && data.spreadsheetId.length > 12 ? `${data.spreadsheetId.slice(0, 12)}...` : data.spreadsheetId}
          </span>
        </div>

        {/* Info rows */}
        <div className="space-y-1 mb-3">
          <p className="text-xs text-[var(--text-muted)]">Interval: {data.interval}</p>
          {data.pendingCount != null && data.pendingCount > 0 && (
            <p className="text-xs">
              <span className="text-orange-400 font-medium">{data.pendingCount} pending items</span>
              <span className="text-[var(--text-muted)]"> need review in Operations tab</span>
            </p>
          )}
        </div>

        {/* Last Refresh block */}
        {lastRun && (
          <div className="mt-4 pt-4 border-t border-[var(--surface-2)] space-y-1">
            <p className="text-xs font-semibold text-[var(--text-muted)] uppercase tracking-wide">Last Refresh</p>
            <p className="text-xs text-[var(--text-muted)]">
              Ran at {formatAdminDate(lastRun.lastRunAt)} · {Number.isFinite(lastRun.durationMs) ? (lastRun.durationMs / 1000).toFixed(1) : '?'}s
            </p>
            <p className="text-xs text-[var(--text-muted)]">
              {lastRun.allocated > 0
                ? <span className="text-[var(--success)]">{lastRun.allocated} allocated</span>
                : <span>0 allocated</span>} · {lastRun.updated} updated · {lastRun.refunded} refunded · {lastRun.totalRows} total
            </p>
            {(lastRun.unmatched > 0 || lastRun.ambiguous > 0 || lastRun.failed > 0 || lastRun.parseErrors > 0) && (
              <p className="text-xs text-[var(--text-muted)]">
                {[
                  lastRun.unmatched > 0 && <span key="unmatched" className="text-orange-400">{lastRun.unmatched} unmatched</span>,
                  lastRun.ambiguous > 0 && <span key="ambiguous" className="text-yellow-400">{lastRun.ambiguous} ambiguous</span>,
                  lastRun.failed > 0 && <span key="failed" className="text-red-400">{lastRun.failed} failed</span>,
                  lastRun.parseErrors > 0 && <span key="parseErrors" className="text-orange-400">{lastRun.parseErrors} parse errors</span>,
                ]
                  .filter(Boolean)
                  .reduce<ReactNode[]>((acc, el, i) => (i === 0 ? [el] : [...acc, ' · ', el]), [])}
              </p>
            )}
          </div>
        )}
      </CardShell>

      {/* Manual Refresh — separate action card */}
      <CardShell padding="lg">
        <h3 className="text-base font-semibold text-[var(--text)] mb-2">Manual Refresh</h3>
        <p className="text-sm text-[var(--text-muted)] mb-3">
          Trigger a PSA Sheets sync. This fetches the configured Google Sheet and imports new or updated rows.
        </p>
        <Button variant="secondary" size="sm" onClick={handleRefresh} loading={refreshMutation.isPending}>
          Trigger Sync
        </Button>
      </CardShell>
    </div>
  );
}
