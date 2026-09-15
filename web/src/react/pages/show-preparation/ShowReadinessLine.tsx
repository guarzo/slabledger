import type { useShowReadiness } from '../../queries/useShowReadiness';
import { Button } from '../../ui';

/** Coverage is not a progress task and never offers source acquisition. */
export default function ShowReadinessLine({ readiness: r }: { readiness: ReturnType<typeof useShowReadiness> }) {
  const unavailable = r.counts.running + r.counts.interrupted + r.counts.failed + r.counts.invalid + r.counts.unavailable - r.needsMatchingCount;
  return <div className="show-readiness show-actions" aria-label="Comp data coverage">
    <span role="status">{r.currentCount}/{r.cohortCount} cards with current evidence</span>
    {r.counts.not_checked > 0 && <span>{r.counts.not_checked} no comp data</span>}
    {r.counts.stale > 0 && <span>{r.counts.stale} out of date</span>}
    {unavailable > 0 && <span>{unavailable} data unavailable</span>}
    {r.needsMatchingCount > 0 && <span>{r.needsMatchingCount} needs matching</span>}
    {r.counts.unknown > 0 && <span>{r.counts.unknown} evaluations unavailable</span>}
    {r.missingPriceCount > 0 && <span>{r.missingPriceCount} no DH price</span>}
    {r.observationError && <><span role="alert">{r.observationError}</span><Button size="sm" variant="secondary" disabled={r.observing} onClick={() => void r.retryObservation()}>Retry price support read</Button></>}
  </div>;
}
