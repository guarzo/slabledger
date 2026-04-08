import { useDHStatus, useTriggerDHBulkMatch } from '../../queries/useAdminQueries';
import { useToast } from '../../contexts/ToastContext';
import { CardShell } from '../../ui/CardShell';
import Button from '../../ui/Button';

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
        <p className="text-red-400 text-sm">Failed to load DH status. Integration may not be configured.</p>
      </CardShell>
    );
  }

  const isRunning = status?.bulk_match_running ?? false;

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
      {/* Bulk Match Error */}
      {status?.bulk_match_error && (
        <div className="rounded-xl border border-red-500/40 bg-red-500/10 p-4">
          <h4 className="text-sm font-semibold text-red-400 mb-1">Bulk Match Stopped</h4>
          <p className="text-sm text-red-300">{status.bulk_match_error}</p>
        </div>
      )}

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
    </div>
  );
}
