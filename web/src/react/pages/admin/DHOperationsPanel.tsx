import {
  useDHStatus,
  useTriggerDHReconcile,
  useDHTombstoneCount,
  useClearDHTombstones,
} from '../../queries/useAdminQueries';
import { CardShell } from '../../ui/CardShell';
import Button from '../../ui/Button';
import { useToast } from '../../contexts/ToastContext';

/**
 * Operational controls for the DoubleHolo integration: the two manual
 * actions an operator can take, plus the one alarm that means DH sold
 * something we cannot match back to inventory.
 *
 * Deliberately not a metric grid — DHTab already renders the DH counts.
 * What lives here is (a) things you press and (b) the orphan alarm, which
 * gets visual primacy only when it is non-zero.
 */
export function DHOperationsPanel({ enabled = true }: { enabled?: boolean }) {
  const { data: status } = useDHStatus({ enabled });
  const reconcileMutation = useTriggerDHReconcile();
  const { data: tombstoneData } = useDHTombstoneCount({ enabled });
  const clearTombstonesMutation = useClearDHTombstones();
  const toast = useToast();

  const handleReconcile = async () => {
    try {
      const result = await reconcileMutation.mutateAsync();
      const errorCount = result.errors?.length ?? 0;
      // Errors first: a partial success with failures shouldn't masquerade as a clean run.
      if (errorCount > 0) {
        // Surface the actual first error rather than referring the operator to logs
        // they have no way to open from here. A dead-end "check logs" referral is
        // the failure mode this rework exists to remove.
        const first = result.errors?.[0] ?? 'unknown error';
        toast.warning(
          errorCount === 1
            ? `${result.reset} reset, 1 error: ${first}`
            : `${result.reset} reset, ${errorCount} errors. First: ${first}`,
        );
      } else if (result.reset > 0) {
        toast.success(`Reset ${result.reset} items removed from DH`);
      } else {
        toast.success('All DH-linked items still present on DH');
      }
    } catch {
      toast.error('DH reconcile failed');
    }
  };

  const handleClearTombstones = async () => {
    try {
      const result = await clearTombstonesMutation.mutateAsync();
      toast.success(`Cleared ${result.cleared} tombstoned DH card${result.cleared === 1 ? '' : 's'}`);
    } catch {
      toast.error('Failed to clear DH tombstones');
    }
  };

  if (!enabled) return null;

  const tombstoneCount = tombstoneData?.count ?? 0;
  const polledAt = status?.last_orders_poll_at;
  const orphan = status?.orders_orphan_count_24h ?? 0;
  const matched = status?.orders_matched_count_24h ?? 0;
  const alreadySold = status?.orders_already_sold_count_24h ?? 0;

  return (
    <div className="space-y-3 mt-3">
      {/* Orphan alarm — DH sold something we can't match to a purchase.
          Loud when non-zero, a single quiet line when clean. */}
      {polledAt && (orphan > 0 ? (
        <div
          role="alert"
          className="rounded-lg border border-[var(--danger-border)] bg-[var(--danger-bg)] px-4 py-3 flex items-start gap-3"
        >
          <span aria-hidden="true" className="text-[var(--danger)] leading-5">▴</span>
          <div className="min-w-0">
            <p className="text-sm font-semibold text-[var(--danger)]">
              {orphan} orphan DH orders in the last 24h
            </p>
            <p className="text-xs text-[var(--danger)] opacity-80 mt-0.5">
              DH reported a sale we could not match to a purchase. Reconcile, then check the DH order log.
            </p>
            <p className="text-[10px] text-[var(--text-muted)] uppercase tracking-wider tabular-nums mt-1.5">
              <span>{matched} matched</span>
              <span aria-hidden="true"> · </span>
              <span>{alreadySold} already sold</span>
              <span aria-hidden="true"> · </span>
              <span>polled {new Date(polledAt).toLocaleString()}</span>
            </p>
          </div>
        </div>
      ) : (
        <p className="text-[10px] text-[var(--text-muted)] uppercase tracking-wider tabular-nums">
          <span>0 orphan DH orders in the last 24h</span>
          <span aria-hidden="true"> · </span>
          <span>{matched} matched</span>
          <span aria-hidden="true"> · </span>
          <span>{alreadySold} already sold</span>
          <span aria-hidden="true"> · </span>
          <span>polled {new Date(polledAt).toLocaleString()}</span>
        </p>
      ))}

      <CardShell padding="md">
        <div className="space-y-3">
          <div className="flex items-center justify-between gap-3">
            <p className="text-xs text-[var(--text-muted)]">
              Sync DH to reset items that are no longer listed on DoubleHolo.
            </p>
            <Button
              variant="secondary"
              size="sm"
              onClick={handleReconcile}
              loading={reconcileMutation.isPending}
              disabled={reconcileMutation.isPending}
            >
              {reconcileMutation.isPending ? 'Syncing…' : 'Sync DH now'}
            </Button>
          </div>

          <div className="flex items-center justify-between gap-3 pt-3 border-t border-[var(--surface-2)]">
            <p className="text-xs text-[var(--text-muted)]">
              DH card IDs suppressed after repeated 404s:{' '}
              <span className="font-semibold tabular-nums text-[var(--text-secondary)]">{tombstoneCount}</span>
            </p>
            <Button
              variant="secondary"
              size="sm"
              onClick={handleClearTombstones}
              loading={clearTombstonesMutation.isPending}
              disabled={clearTombstonesMutation.isPending || tombstoneCount === 0}
            >
              {clearTombstonesMutation.isPending ? 'Clearing…' : 'Clear tombstones'}
            </Button>
          </div>
        </div>
      </CardShell>
    </div>
  );
}
