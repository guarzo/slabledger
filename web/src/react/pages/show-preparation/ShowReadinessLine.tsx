import type { useShowReadiness } from '../../queries/useShowReadiness';
import { Button } from '../../ui';

/** Compact functional controls, independent of displayed matches and selection. */
export default function ShowReadinessLine({ readiness: r, selectedCount }: { readiness: ReturnType<typeof useShowReadiness>; selectedCount: number }) {
  const summary = r.observing ? 'Reading current price support…' : r.observationError ? 'Current price support unavailable' : r.busy ? `Checking comps ${r.done}/${r.total}`
    : r.counts.running ? `Checking comps · ${r.counts.running} ${r.counts.running === 1 ? 'card' : 'cards'} awaiting server observation`
    : r.incomplete ? `${r.counts.not_checked === r.cohortCount ? 'Not checked' : 'Check incomplete'} · ${r.currentCount}/${r.cohortCount} ${r.cohortCount === 1 ? 'card' : 'cards'} current`
    : r.cohortCount ? `Check complete · ${r.cohortCount} ${r.cohortCount === 1 ? 'card' : 'cards'} current` : 'No cards in check scope';
  return <div className="show-readiness show-actions" aria-label="Comps readiness">
    <span role="status">{summary}{r.busy ? ` identities · ${r.cohortCount} cards in scope` : ''}{selectedCount > 0 && r.incomplete ? ' · Paused for selection' : ''}</span>
    {r.observationError && <><span role="alert">{r.observationError}</span><Button size="sm" variant="secondary" disabled={r.observing} onClick={() => void r.retryObservation()}>Retry price support read</Button></>}
    {r.busy ? <Button size="sm" variant="ghost" onClick={r.cancel}>Cancel checking</Button>
      : !r.observationError && (r.incomplete || r.error) && <Button size="sm" variant="secondary" disabled={r.blocked || r.cohortCount === 0} onClick={() => void r.check()}>{r.error || r.retry ? 'Retry / Continue checking' : 'Check comps'}</Button>}
    {r.missingPriceCount > 0 && <span>{r.missingPriceCount} without a listed price. Select to check manually.</span>}
    {r.counts.unknown > 0 && <span>{r.counts.unknown} cards need manual checking.</span>}
    {r.counts.interrupted > 0 && <span>Interrupted check; retry explicitly.</span>}
    {r.error && <span role="alert">{r.error}</span>}
  </div>;
}
