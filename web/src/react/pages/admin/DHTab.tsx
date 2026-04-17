import { useDHStatus, useTriggerDHBulkMatch } from '../../queries/useAdminQueries';
import { useToast } from '../../contexts/ToastContext';
import { CardShell } from '../../ui/CardShell';
import Button from '../../ui/Button';
import { DHPushConfigCard } from './DHPushConfigCard';

export function DHTab({ enabled = true }: { enabled?: boolean }) {
  const { data: status, isLoading, error } = useDHStatus({ enabled });
  const bulkMatchMutation = useTriggerDHBulkMatch();
  const toast = useToast();

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
        <p className="text-[var(--danger)] text-sm">Failed to load DH status. Integration may not be configured.</p>
      </CardShell>
    );
  }

  const isRunning = status?.bulk_match_running ?? false;
  const apiHealth = status?.api_health;
  const successRate = apiHealth ? `${(apiHealth.success_rate * 100).toFixed(0)}%` : '—';

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
      {/* Status card */}
      <CardShell padding="lg">
        {/* Header row */}
        <div className="flex items-center justify-between mb-3">
          <div className="flex items-center gap-2">
            <span className={`w-2 h-2 rounded-full shrink-0 ${apiHealth && apiHealth.success_rate >= 0.95 ? 'bg-[var(--success)]' : 'bg-[var(--warning)]'}`} />
            <span className="text-sm font-semibold text-[var(--text)]">
              {apiHealth && apiHealth.success_rate >= 0.95 ? 'Healthy' : 'Degraded'}
            </span>
          </div>
          <span className="text-xs text-[var(--text-muted)]">API success: {successRate}</span>
        </div>

        {/* Info rows */}
        <div className="space-y-1 mb-3">
          {status?.mapped_count !== undefined && (
            <p className="text-xs text-[var(--text-muted)]">
              Mapped: <span className="text-[var(--text)]">{status.mapped_count}</span>
            </p>
          )}
          {status?.unmatched_count !== undefined && (
            <p className="text-xs text-[var(--text-muted)]">
              Unmatched: <span className={status.unmatched_count > 0 ? 'text-[var(--warning)]' : 'text-[var(--text)]'}>{status.unmatched_count}</span>
              {(status.dismissed_count ?? 0) > 0 && (
                <span className="text-[var(--text-muted)]"> ({status.dismissed_count} dismissed)</span>
              )}
            </p>
          )}
          {status?.pending_count !== undefined && status.pending_count > 0 && (
            <p className="text-sm font-medium text-[var(--text-muted)]">
              Pending push: <span className="inline-flex items-center justify-center px-2 py-0.5 rounded-full bg-[var(--warning-bg)] text-[var(--warning)] font-semibold text-xs">{status.pending_count}</span>
            </p>
          )}
        </div>

        {/* Collapsible push config */}
        <details>
          <summary className="text-xs text-[var(--brand-400)] cursor-pointer select-none">Listing push safety rules</summary>
          <div className="mt-3">
            <DHPushConfigCard />
          </div>
        </details>
      </CardShell>

      {/* Bulk Match Error */}
      {status?.bulk_match_error && (
        <div className="rounded-xl border border-[var(--danger-border)] bg-[var(--danger-bg)] p-4">
          <h4 className="text-sm font-semibold text-[var(--danger)] mb-1">Bulk Match Stopped</h4>
          <p className="text-sm text-[var(--danger)]">{status.bulk_match_error}</p>
        </div>
      )}

      {/* Bulk Match — separate action card */}
      <CardShell padding="lg">
        <h3 className="text-base font-semibold text-[var(--text)] mb-2">Bulk Match (Backfill)</h3>
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
    </div>
  );
}
