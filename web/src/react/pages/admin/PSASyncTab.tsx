import { usePSASyncStatus, useTriggerPSASyncRefresh } from '../../queries/useAdminQueries';
import { useToast } from '../../contexts/ToastContext';
import { CardShell } from '../../ui/CardShell';
import Button from '../../ui/Button';
import { formatAdminDate } from './adminUtils';
import type { PSASyncLastRun } from '../../../types/admin';

function formatDuration(ms: number): string {
  if (ms < 1000) return `${ms}ms`;
  return `${(ms / 1000).toFixed(1)}s`;
}

function LastRunStats({ stats }: { stats: PSASyncLastRun }) {
  return (
    <div className="mt-3 p-3 rounded-lg bg-[var(--surface-1)] text-sm space-y-1">
      <p className="text-[var(--text-muted)]">
        Last run: <span className="text-[var(--text)]">{formatAdminDate(stats.lastRunAt)}</span>{' '}
        ({formatDuration(stats.durationMs)})
      </p>
      <div className="grid grid-cols-3 gap-2 mt-2">
        <Stat label="Allocated" value={stats.allocated} />
        <Stat label="Updated" value={stats.updated} />
        <Stat label="Refunded" value={stats.refunded} />
        <Stat label="Unmatched" value={stats.unmatched} color="text-orange-400" />
        <Stat label="Ambiguous" value={stats.ambiguous} color="text-yellow-400" />
        <Stat label="Skipped" value={stats.skipped} />
        <Stat label="Failed" value={stats.failed} color={stats.failed > 0 ? 'text-red-400' : undefined} />
        <Stat label="Total Rows" value={stats.totalRows} />
        <Stat label="Parse Errors" value={stats.parseErrors} color={stats.parseErrors > 0 ? 'text-orange-400' : undefined} />
      </div>
    </div>
  );
}

function Stat({ label, value, color }: { label: string; value: number; color?: string }) {
  return (
    <div>
      <span className="text-[var(--text-muted)]">{label}: </span>
      <span className={color ?? 'text-[var(--text)]'}>{value}</span>
    </div>
  );
}

export function PSASyncTab({ enabled = true }: { enabled?: boolean }) {
  const { data } = usePSASyncStatus({ enabled });
  const refreshMutation = useTriggerPSASyncRefresh();
  const toast = useToast();

  if (!data) return null;

  const handleRefresh = async () => {
    try {
      await refreshMutation.mutateAsync();
      toast.success('PSA Sheets sync complete');
    } catch {
      toast.error('PSA Sheets sync failed');
    }
  };

  return (
    <div className="space-y-4 mt-4">
      {data.configured && (
        <CardShell padding="lg">
          <h3 className="text-base font-semibold text-[var(--text)] mb-2">Configuration</h3>
          <div className="text-sm text-[var(--text-muted)]">
            <p>Sheet: <span className="text-[var(--text)] font-mono text-xs">{data.spreadsheetId && data.spreadsheetId.length > 12 ? data.spreadsheetId.slice(0, 12) + '...' : data.spreadsheetId}</span></p>
            <p>Interval: <span className="text-[var(--text)]">{data.interval}</span></p>
          </div>

          {data.lastRun && <LastRunStats stats={data.lastRun} />}

          {data.pendingCount != null && data.pendingCount > 0 && (
            <div className="text-sm mt-3">
              <span className="text-orange-400 font-medium">{data.pendingCount} pending items</span>
              <span className="text-[var(--text-muted)]"> need review in Operations tab</span>
            </div>
          )}
        </CardShell>
      )}

      {!data.configured && (
        <CardShell padding="lg">
          <p className="text-sm text-[var(--text-muted)]">
            PSA Sheets sync is not configured. Set GOOGLE_SHEETS_SPREADSHEET_ID and service account credentials.
          </p>
        </CardShell>
      )}

      {data.configured && (
        <CardShell padding="lg">
          <h3 className="text-base font-semibold text-[var(--text)] mb-2">Manual Refresh</h3>
          <p className="text-sm text-[var(--text-muted)] mb-3">
            Trigger a PSA Sheets sync. This fetches the configured Google Sheet and imports new or updated rows.
          </p>
          <Button variant="secondary" size="sm" onClick={handleRefresh} loading={refreshMutation.isPending}>
            Trigger Sync
          </Button>
        </CardShell>
      )}
    </div>
  );
}
