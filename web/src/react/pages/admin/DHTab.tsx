import { useDHStatus, useTriggerDHBulkMatch, useDHPushConfig } from '../../queries/useAdminQueries';
import { useToast } from '../../contexts/ToastContext';
import { CardShell } from '../../ui/CardShell';
import Button from '../../ui/Button';
import { DHPushConfigCard } from './DHPushConfigCard';
import { DHListingsPauseControl } from './DHListingsPauseControl';

export function DHTab({ enabled = true }: { enabled?: boolean }) {
  const { data: status, isLoading, error } = useDHStatus({ enabled });
  const bulkMatchMutation = useTriggerDHBulkMatch();
  const { data: pushConfig } = useDHPushConfig({ enabled });
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
        <p className="text-[var(--text-muted)]">Loading DH status…</p>
      </CardShell>
    );
  }

  if (error && !status) {
    return (
      <CardShell padding="lg">
        <div className="space-y-3">
          <div className="flex items-center gap-2">
            <span
              aria-hidden="true"
              className="inline-block w-2 h-2 rounded-full bg-[var(--danger)]"
            />
            <p className="text-sm font-semibold text-[var(--text)]">
              DoubleHolo not responding · check credentials and restart
            </p>
          </div>
          <details className="text-sm text-[var(--text-muted)]">
            <summary className="cursor-pointer text-[var(--brand-400)] hover:underline select-none">
              How to fix
            </summary>
            <div className="mt-2 space-y-1.5 pl-4 border-l border-[var(--surface-2)]">
              <p>
                DH credentials are set on the server, not in the UI. Ask the operator to verify the following environment variables in the backend:
              </p>
              <ul className="list-disc list-inside space-y-1">
                <li>
                  <code className="px-1 py-0.5 rounded bg-[var(--surface-2)] text-[var(--text)] text-xs">DH_ENTERPRISE_API_KEY</code>
                </li>
                <li>
                  <code className="px-1 py-0.5 rounded bg-[var(--surface-2)] text-[var(--text)] text-xs">DH_API_BASE_URL</code>
                </li>
              </ul>
              <p>Then restart the service.</p>
            </div>
          </details>
        </div>
      </CardShell>
    );
  }

  const isRunning = status?.bulk_match_running ?? false;
  const apiHealth = status?.api_health;
  const successRate = apiHealth ? `${(apiHealth.success_rate * 100).toFixed(0)}%` : '—';
  const healthy = !!apiHealth && apiHealth.success_rate >= 0.95;

  const handleBulkMatch = async () => {
    try {
      await bulkMatchMutation.mutateAsync();
      toast.success('Bulk match started. Progress will update automatically.');
    } catch {
      toast.error('Failed to start bulk match');
    }
  };

  return (
    <div className="space-y-3">
      <CardShell padding="lg">
        <div className="flex items-center justify-between gap-4 mb-3">
          <div className="flex items-center gap-2">
            <span className={`w-2 h-2 rounded-full shrink-0 ${healthy ? 'bg-[var(--success)]' : 'bg-[var(--warning)]'}`} />
            <span className="text-sm font-semibold text-[var(--text)]">{healthy ? 'Healthy' : 'Degraded'}</span>
            <span className="text-xs text-[var(--text-muted)]">· API success: {successRate}</span>
          </div>
          <DHListingsPauseControl enabled={enabled} />
        </div>

        {pushConfig?.listingsPaused && (
          <div
            role="status"
            className="mb-3 rounded-lg border border-[var(--warning-border)] bg-[var(--warning-bg)] px-3 py-2 text-xs font-medium text-[var(--warning)]"
          >
            Listings are currently paused. New and pending purchases stay in inventory and accumulate as pending until this is turned off.
          </div>
        )}

        <div className="flex flex-col gap-5 sm:flex-row sm:items-start sm:gap-10 mb-3">
          <div className="sm:w-44 sm:shrink-0">
            <div className="text-[10px] font-semibold uppercase tracking-wider text-[var(--text-muted)]">
              Mapped cards
            </div>
            <div className="text-3xl font-semibold tabular-nums leading-tight text-[var(--text)]">
              {status?.mapped_count ?? 0}
            </div>
            {((status?.unmatched_count ?? 0) > 0 || (status?.dismissed_count ?? 0) > 0) && (
              <p className="mt-1 text-xs text-[var(--warning)]">
                {(status?.unmatched_count ?? 0) > 0 && `${status?.unmatched_count} still unmatched`}
                {(status?.unmatched_count ?? 0) > 0 && (status?.dismissed_count ?? 0) > 0 && ' · '}
                {(status?.dismissed_count ?? 0) > 0 && `${status?.dismissed_count} dismissed`}
              </p>
            )}
          </div>

          <div className="grid flex-1 grid-cols-2 gap-x-6 gap-y-3 sm:grid-cols-3">
            <Stat
              label="Unmatched"
              value={status?.unmatched_count ?? 0}
              tone={(status?.unmatched_count ?? 0) > 0 ? 'warning' : 'default'}
            />
            <Stat
              label="Pending push"
              value={status?.pending_count ?? 0}
              tone={(status?.pending_count ?? 0) > 0 ? 'warning' : 'default'}
            />
            <Stat label="DH listings" value={status?.dh_listings_count ?? 0} />
          </div>
        </div>

        <div className="flex items-center gap-2 pt-3 border-t border-[var(--surface-2)]">
          <Button
            variant="secondary"
            size="sm"
            onClick={handleBulkMatch}
            loading={bulkMatchMutation.isPending}
            disabled={isRunning || bulkMatchMutation.isPending}
          >
            {isRunning ? 'Bulk match running…' : 'Run bulk match'}
          </Button>
          {isRunning && (
            <span className="text-xs text-[var(--text-muted)]">Matching in progress; counts update automatically.</span>
          )}
        </div>

        <div className="mt-4 pt-3 border-t border-[var(--surface-2)]">
          <DHPushConfigCard />
        </div>
      </CardShell>

      {status?.bulk_match_error && (
        <div className="rounded-xl border border-[var(--danger-border)] bg-[var(--danger-bg)] p-4">
          <h4 className="text-sm font-semibold text-[var(--danger)] mb-1">Bulk match stopped</h4>
          <p className="text-sm text-[var(--danger)]">{status.bulk_match_error}</p>
        </div>
      )}
    </div>
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
