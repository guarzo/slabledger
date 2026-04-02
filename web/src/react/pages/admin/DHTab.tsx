import { useDHStatus, useTriggerDHBulkMatch } from '../../queries/useAdminQueries';
import { useToast } from '../../contexts/ToastContext';
import { CardShell } from '../../ui/CardShell';
import { SummaryCard } from './shared';
import Button from '../../ui/Button';

function formatTimestamp(ts: string): string {
  if (!ts) return 'Never';
  const d = new Date(ts);
  if (isNaN(d.getTime())) return ts;
  return d.toLocaleString();
}

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
      {/* Summary Stats */}
      <div className="grid grid-cols-2 sm:grid-cols-4 gap-3">
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
          label="Unmatched Cards"
          value={status?.unmatched_count ?? 0}
          color={status?.unmatched_count ? 'var(--warning)' : undefined}
        />
      </div>

      {/* Bulk Match */}
      <CardShell padding="lg">
        <h4 className="text-sm font-semibold text-[var(--text)] mb-2">Bulk Match</h4>
        <p className="text-sm text-[var(--text-muted)] mb-3">
          Match unmatched inventory cards against the DH catalog. Cards with high-confidence matches will be automatically mapped.
        </p>
        <Button
          variant="secondary"
          size="sm"
          onClick={handleBulkMatch}
          loading={bulkMatchMutation.isPending}
          disabled={isRunning}
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
