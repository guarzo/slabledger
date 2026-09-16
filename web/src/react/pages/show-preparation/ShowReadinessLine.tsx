import type { useShowReadiness } from '../../queries/useShowReadiness';
import type { ShowPrepWorkerStatus } from '../../../types/showprepWorker';
import { Button } from '../../ui';

type Coverage = Pick<ShowPrepWorkerStatus, 'eligibleIdentities' | 'currentIdentities' | 'missingIdentities' | 'staleIdentities' | 'failedIdentities' | 'eligibleCards' | 'currentCards' | 'unresolvedCards'>;

/** Quiet, server-counted fleet coverage, not a selected-cohort progress task. */
export default function ShowReadinessLine({ readiness: r, coverage: c, coverageError = false }: {
  readiness: ReturnType<typeof useShowReadiness>;
  coverage?: Coverage;
  coverageError?: boolean;
}) {
  const incomplete = c && c.currentCards < c.eligibleCards;
  if (!incomplete && !coverageError && !r.missingPriceCount && !r.observationError) return null;
  return <div className="show-readiness show-actions" aria-label="Comp data coverage">
    {coverageError ? <span>Evidence coverage unavailable</span> : incomplete && <>
      <span>All inventory: {c.currentIdentities}/{c.eligibleIdentities} identities current</span>
      <span>{c.currentCards}/{c.eligibleCards} cards with current evidence</span>
      <span>{c.missingIdentities} missing · {c.staleIdentities} stale · {c.failedIdentities} failed identities</span>
      {c.unresolvedCards > 0 && <span>{c.unresolvedCards} unresolved {c.unresolvedCards === 1 ? 'card' : 'cards'}</span>}
    </>}
    {r.missingPriceCount > 0 && <span>{r.missingPriceCount} no DH price in this view</span>}
    {r.observationError && <><span role="alert">{r.observationError}</span><Button size="sm" variant="secondary" disabled={r.observing} onClick={() => void r.retryObservation()}>Retry price support read</Button></>}
  </div>;
}
