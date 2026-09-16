import { useRequestShowPrepRun, useShowPrepWorkerStatus } from '../../queries/useShowPrepWorker';
import type { ShowPrepWorkerStatus } from '../../../types/showprepWorker';
import { Button, StatusPill } from '../../ui';
import { formatAdminDate } from './adminUtils';

function stateLabel(s: ShowPrepWorkerStatus): string {
  switch (s.state) {
    case 'disabled': return 'Disabled';
    case 'unconfigured': return 'Unconfigured';
    case 'auth_hold': return 'Authentication hold';
    case 'running': return 'Running';
    case 'failed': return 'Completed with errors';
    default:
      if (s.eligibleCards === 0) return 'No inventory';
      return s.currentCards === s.eligibleCards ? 'Current coverage' : 'Idle, coverage incomplete';
  }
}

export function ShowPrepWorkerPanel({ enabled = true }: { enabled?: boolean }) {
  const query = useShowPrepWorkerStatus(enabled);
  const request = useRequestShowPrepRun();
  const s = query.data;
  const unavailable = !s || !s.enabled || !s.configured || query.isError;
  const requestRun = (retry: boolean) => request.mutate(retry);
  const tone = !s || ['disabled', 'unconfigured', 'auth_hold', 'failed'].includes(s.state) ? 'warning' : 'neutral';

  return <section aria-labelledby="show-evidence-heading" className="space-y-3">
    <div className="flex flex-wrap items-center justify-between gap-2">
      <h3 id="show-evidence-heading" className="text-base font-semibold text-[var(--text)]">Show evidence</h3>
      {s && <StatusPill tone={tone}>{stateLabel(s)}</StatusPill>}
    </div>
    <p className="text-xs text-[var(--text-muted)] max-w-prose">Server-managed CardLadder sales evidence. Independent of value refresh and collection sync.</p>
    {query.isLoading && <p className="text-sm text-[var(--text-muted)]" role="status">Loading evidence coverage…</p>}
    {query.isError && <p role="alert" className="text-sm text-[var(--warning)]">Evidence worker status unavailable. Coverage below may be out of date.</p>}
    {s && <>
      <div className="flex flex-wrap gap-x-6 gap-y-1 text-xs text-[var(--text-muted)]">
        <span>{s.enabled ? 'Enabled' : 'Disabled by SHOW_PREP_REFRESH_ENABLED'}</span>
        <span>{s.configured ? 'Credentials configured' : 'Credentials unavailable'}</span>
      </div>
      <div className="space-y-1 text-sm tabular-nums">
        <p>{s.currentIdentities}/{s.eligibleIdentities} identities current</p>
        <p className="text-xs text-[var(--text-muted)]">{s.missingIdentities} missing · {s.staleIdentities} stale · {s.failedIdentities} failed identities</p>
        <p>{s.currentCards}/{s.eligibleCards} cards with current evidence</p>
        <p className="text-xs text-[var(--text-muted)]">{s.unresolvedCards} unresolved {s.unresolvedCards === 1 ? 'card' : 'cards'}</p>
      </div>
      <dl className="text-xs text-[var(--text-muted)] space-y-1">
        <div className="flex flex-wrap gap-x-2"><dt>Last sweep finished</dt><dd>{s.lastSweepAt ? formatAdminDate(s.lastSweepAt) : 'Not yet'}</dd></div>
        <div className="flex flex-wrap gap-x-2"><dt>Next retry / window</dt><dd>{s.retryAt ? formatAdminDate(s.retryAt) : 'No scheduled retry'}</dd></div>
      </dl>
      {s.error && <p className="text-xs text-[var(--warning)]">{s.error}</p>}
      {s.state === 'auth_hold' && <p className="text-xs text-[var(--warning)] max-w-prose">Update Card Ladder credentials, or use Retry failed to resume after repair. Run now does not clear this hold.</p>}
      {s.state === 'unconfigured' && <p className="text-xs text-[var(--text-muted)]">Save credentials in Card Ladder to activate collection. If unavailable, check the server encryption key.</p>}
    </>}
    <div className="flex flex-wrap gap-2">
      <Button size="sm" variant="secondary" disabled={unavailable || request.isPending || s?.state === 'running'}
        onClick={() => requestRun(false)}>{request.isPending && !request.variables ? 'Requesting…' : 'Run now'}</Button>
      <Button size="sm" variant="secondary" disabled={unavailable || request.isPending || (s?.state !== 'auth_hold' && !s?.failedIdentities)}
        onClick={() => requestRun(true)}>{request.isPending && request.variables ? 'Requesting…' : 'Retry failed'}</Button>
    </div>
    {request.isSuccess && <p role="status" className="text-xs text-[var(--text-muted)]">Request accepted. Background coverage will update separately.</p>}
    {request.isError && <p role="alert" className="text-xs text-[var(--danger)]">Cannot confirm request acceptance. Check worker status before explicitly retrying.</p>}
  </section>;
}
