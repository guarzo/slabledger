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
    <CardShell padding="lg">
      <div className="flex items-center justify-between mb-3">
        <div className="flex items-center gap-2">
          <span className="w-2 h-2 rounded-full bg-[var(--success)] shrink-0" />
          <span className="text-sm font-semibold text-[var(--text)]">Configured</span>
        </div>
        <span className="text-xs text-[var(--text-muted)] font-mono">
          {data.spreadsheetId && data.spreadsheetId.length > 12 ? `${data.spreadsheetId.slice(0, 12)}…` : data.spreadsheetId}
        </span>
      </div>

      <div className="grid grid-cols-2 sm:grid-cols-3 gap-3 mb-3">
        <Stat label="Interval" value={data.interval || '—'} />
        <Stat label="Pending review" value={data.pendingCount ?? 0} tone={(data.pendingCount ?? 0) > 0 ? 'warning' : 'default'} />
      </div>

      <div className="flex items-center gap-2 pt-3 border-t border-[var(--surface-2)]">
        <Button variant="secondary" size="sm" onClick={handleRefresh} loading={refreshMutation.isPending}>
          Sync from Sheets
        </Button>
        <span className="text-xs text-[var(--text-muted)]">Fetches the configured Google Sheet and imports new or updated rows.</span>
      </div>

      {lastRun && (
        <div className="mt-4 pt-3 border-t border-[var(--surface-2)] space-y-1">
          <p className="text-[10px] font-semibold text-[var(--text-muted)] uppercase tracking-wider">Last Refresh</p>
          <p className="text-xs text-[var(--text-muted)]">
            {formatAdminDate(lastRun.lastRunAt)} · {Number.isFinite(lastRun.durationMs) ? (lastRun.durationMs / 1000).toFixed(1) : '?'}s
          </p>
          <p className="text-xs text-[var(--text-muted)]">
            {lastRun.allocated > 0
              ? <span className="text-[var(--success)]">{lastRun.allocated} allocated</span>
              : <span>0 allocated</span>} · {lastRun.updated} updated · {lastRun.refunded} refunded · {lastRun.totalRows} total
          </p>
          {(lastRun.unmatched > 0 || lastRun.ambiguous > 0 || lastRun.failed > 0 || lastRun.parseErrors > 0) && (
            <p className="text-xs text-[var(--text-muted)]">
              {[
                lastRun.unmatched > 0 && <span key="unmatched" className="text-[var(--warning)]">{lastRun.unmatched} unmatched</span>,
                lastRun.ambiguous > 0 && <span key="ambiguous" className="text-[var(--warning)]">{lastRun.ambiguous} ambiguous</span>,
                lastRun.failed > 0 && <span key="failed" className="text-[var(--danger)]">{lastRun.failed} failed</span>,
                lastRun.parseErrors > 0 && <span key="parseErrors" className="text-[var(--danger)]">{lastRun.parseErrors} parse errors</span>,
              ]
                .filter(Boolean)
                .reduce<ReactNode[]>((acc, el, i) => (i === 0 ? [el] : [...acc, ' · ', el]), [])}
            </p>
          )}
        </div>
      )}
    </CardShell>
  );
}

function Stat({ label, value, tone = 'default' }: { label: string; value: number | string; tone?: 'default' | 'warning' }) {
  const valueColor = tone === 'warning' ? 'text-[var(--warning)]' : 'text-[var(--text)]';
  return (
    <div className="min-w-0">
      <div className="text-[10px] text-[var(--text-muted)] uppercase tracking-wider">{label}</div>
      <div className={`text-sm font-semibold tabular-nums ${valueColor}`}>{value}</div>
    </div>
  );
}
