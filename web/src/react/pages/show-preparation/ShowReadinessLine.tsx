import type { useShowReadiness } from '../../queries/useShowReadiness';
import { Button } from '../../ui';

/** Compact functional controls, independent of displayed matches and selection. */
export default function ShowReadinessLine({ readiness: r, selectedCount }: { readiness: ReturnType<typeof useShowReadiness>; selectedCount: number }) {
  const summary = r.busy ? `Checking comps ${r.done}/${r.total} identities · ${r.cohortCount} cards in scope`
    : r.counts.running ? `Checking comps · ${r.counts.running} cards awaiting server observation`
    : r.incomplete ? `${r.counts.not_checked === r.cohortCount ? 'Not checked' : 'Check incomplete'} · ${r.currentCount}/${r.cohortCount} cards current`
    : `Check complete · ${r.cohortCount} cards current`;
  return <div className="show-actions text-sm" aria-label="Comps readiness">
    <span role="status">{summary}{selectedCount > 0 && r.incomplete ? ' · Automatic checking paused for selection' : ''}</span>
    {r.busy ? <Button size="sm" variant="ghost" onClick={r.cancel}>Cancel checking</Button>
      : <Button size="sm" variant="secondary" disabled={r.blocked || r.cohortCount === 0} onClick={() => void r.check()}>{r.error || r.retry ? 'Retry / Continue checking' : 'Check comps'}</Button>}
    {r.missingPriceCount > 0 && <span>{r.missingPriceCount} without a listed price; manual checking remains available.</span>}
    {r.counts.unknown > 0 && <span>Automatic checking unavailable for {r.counts.unknown} cards without readiness metadata.</span>}
    {r.counts.interrupted > 0 && <span>Interrupted check; retry explicitly.</span>}
    {r.error && <span role="alert">{r.error}</span>}
  </div>;
}
